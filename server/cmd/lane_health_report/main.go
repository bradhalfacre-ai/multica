package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/lanehealth"
	"github.com/multica-ai/multica/server/internal/util"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	var (
		issuesFlag       stringList
		workspaceID      string
		daemonHealthPath string
		nowValue         string
	)
	fs := flag.NewFlagSet("lane_health_report", flag.ContinueOnError)
	fs.Var(&issuesFlag, "issue", "Issue id or C2-number to report; may be repeated")
	fs.StringVar(&workspaceID, "workspace-id", "", "Workspace id required for C2-number issue refs")
	fs.StringVar(&daemonHealthPath, "daemon-health-json", "", "Optional daemon /health JSON snapshot file")
	fs.StringVar(&nowValue, "now", "", "RFC3339 timestamp for deterministic reports")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(issuesFlag) == 0 {
		return errors.New("at least one --issue is required")
	}

	now := time.Now().UTC()
	if nowValue != "" {
		parsed, err := time.Parse(time.RFC3339, nowValue)
		if err != nil {
			return fmt.Errorf("parse --now: %w", err)
		}
		now = parsed
	}

	daemonHealth, err := readDaemonHealth(daemonHealthPath)
	if err != nil {
		return err
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://multica:multica@localhost:5432/multica?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	reports := make([]lanehealth.Report, 0, len(issuesFlag))
	for _, issueRef := range issuesFlag {
		snapshot, err := loadSnapshot(ctx, pool, issueRef, workspaceID)
		if err != nil {
			return err
		}
		snapshot.Now = now
		snapshot.DaemonHealth = daemonHealth
		report, err := lanehealth.BuildReport(snapshot)
		if err != nil {
			return err
		}
		reports = append(reports, report)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(reports)
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			*s = append(*s, trimmed)
		}
	}
	return nil
}

func readDaemonHealth(path string) (lanehealth.DaemonHealth, error) {
	if path == "" {
		return lanehealth.DaemonHealth{Available: false}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return lanehealth.DaemonHealth{}, fmt.Errorf("read daemon health json: %w", err)
	}
	return lanehealth.ParseDaemonHealthJSON(raw)
}

func loadSnapshot(ctx context.Context, pool *pgxpool.Pool, issueRef, workspaceID string) (lanehealth.Snapshot, error) {
	issue, err := loadIssue(ctx, pool, issueRef, workspaceID)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	tasks, err := loadTasks(ctx, pool, issue.ID)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	topology, err := loadRuntimeTopology(ctx, pool, issue.WorkspaceID, tasks)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	events, err := loadLaneEvents(ctx, pool, issue.ID)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	handoff, err := loadLatestHandoffText(ctx, pool, issue.ID)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	gates, err := loadGateChecks(issue.Metadata)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	residuals, err := loadResidualChildrenCount(ctx, pool, issue.ID)
	if err != nil {
		return lanehealth.Snapshot{}, err
	}
	return lanehealth.Snapshot{
		Issue:                 issue,
		Tasks:                 tasks,
		RuntimeTopology:       topology,
		LaneEvents:            events,
		HandoffText:           handoff,
		GateChecks:            gates,
		ResidualChildrenCount: residuals,
	}, nil
}

func loadIssue(ctx context.Context, pool *pgxpool.Pool, issueRef, workspaceID string) (lanehealth.IssueSnapshot, error) {
	var (
		row pgx.Row
	)
	if strings.HasPrefix(strings.ToUpper(issueRef), "C2-") {
		number, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(issueRef), "C2-"))
		if err != nil {
			return lanehealth.IssueSnapshot{}, fmt.Errorf("parse issue ref %q: %w", issueRef, err)
		}
		if workspaceID == "" {
			return lanehealth.IssueSnapshot{}, errors.New("--workspace-id is required when --issue uses a C2-number")
		}
		row = pool.QueryRow(ctx, `
SELECT id, workspace_id, number, status, COALESCE(assignee_type, ''), COALESCE(assignee_id::text, ''), metadata
FROM issue
WHERE workspace_id = $1::uuid AND number = $2`, workspaceID, number)
	} else {
		row = pool.QueryRow(ctx, `
SELECT id, workspace_id, number, status, COALESCE(assignee_type, ''), COALESCE(assignee_id::text, ''), metadata
FROM issue
WHERE id = $1::uuid`, issueRef)
	}

	var id, wsID pgtype.UUID
	var number int
	var status, assigneeType, assigneeID string
	var metadataRaw []byte
	if err := row.Scan(&id, &wsID, &number, &status, &assigneeType, &assigneeID, &metadataRaw); err != nil {
		return lanehealth.IssueSnapshot{}, fmt.Errorf("load issue %q: %w", issueRef, err)
	}
	metadata := map[string]any{}
	if len(metadataRaw) > 0 {
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
			return lanehealth.IssueSnapshot{}, fmt.Errorf("parse issue metadata: %w", err)
		}
	}
	return lanehealth.IssueSnapshot{
		ID:           uuidString(id),
		Identifier:   fmt.Sprintf("C2-%d", number),
		WorkspaceID:  uuidString(wsID),
		Status:       status,
		AssigneeType: assigneeType,
		AssigneeID:   assigneeID,
		Metadata:     metadata,
	}, nil
}

func loadTasks(ctx context.Context, pool *pgxpool.Pool, issueID string) ([]lanehealth.TaskSnapshot, error) {
	rows, err := pool.Query(ctx, `
SELECT id, agent_id, runtime_id, status, created_at, dispatched_at, started_at, completed_at,
       attempt, COALESCE(parent_task_id::text, ''), COALESCE(failure_reason, '')
FROM agent_task_queue
WHERE issue_id = $1::uuid
ORDER BY created_at ASC`, issueID)
	if err != nil {
		return nil, fmt.Errorf("load tasks: %w", err)
	}
	defer rows.Close()

	var tasks []lanehealth.TaskSnapshot
	for rows.Next() {
		var id, agentID, runtimeID pgtype.UUID
		var status, parentID, failureReason string
		var created, dispatched, started, completed pgtype.Timestamptz
		var attempt int
		if err := rows.Scan(&id, &agentID, &runtimeID, &status, &created, &dispatched, &started, &completed, &attempt, &parentID, &failureReason); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, lanehealth.TaskSnapshot{
			ID:            uuidString(id),
			AgentID:       uuidString(agentID),
			RuntimeID:     uuidString(runtimeID),
			Status:        status,
			CreatedAt:     created.Time.UTC(),
			DispatchedAt:  timestamptzPtr(dispatched),
			StartedAt:     timestamptzPtr(started),
			CompletedAt:   timestamptzPtr(completed),
			Attempt:       attempt,
			ParentTaskID:  parentID,
			FailureReason: failureReason,
		})
	}
	return tasks, rows.Err()
}

func loadRuntimeTopology(ctx context.Context, pool *pgxpool.Pool, workspaceID string, tasks []lanehealth.TaskSnapshot) (lanehealth.RuntimeTopology, error) {
	rows, err := pool.Query(ctx, `
SELECT ar.id, ar.status, ar.last_seen_at, COALESCE(MAX(a.max_concurrent_tasks), 0)::int
FROM agent_runtime ar
LEFT JOIN agent a ON a.runtime_id = ar.id AND a.archived_at IS NULL
WHERE ar.workspace_id = $1::uuid
GROUP BY ar.id, ar.status, ar.last_seen_at`, workspaceID)
	if err != nil {
		return lanehealth.RuntimeTopology{}, fmt.Errorf("load runtime topology: %w", err)
	}
	defer rows.Close()

	topology := lanehealth.RuntimeTopology{
		QueueCountByRuntime:   map[string]int{},
		RuntimeStatusByID:     map[string]string{},
		RuntimeLastSeenAtByID: map[string]*time.Time{},
		AgentMaxConcurrency:   map[string]int{},
	}
	for _, task := range tasks {
		if task.Status == "queued" && task.RuntimeID != "" {
			topology.QueueCountByRuntime[task.RuntimeID]++
		}
	}
	for rows.Next() {
		var runtimeID pgtype.UUID
		var status string
		var lastSeen pgtype.Timestamptz
		var maxConcurrent int
		if err := rows.Scan(&runtimeID, &status, &lastSeen, &maxConcurrent); err != nil {
			return lanehealth.RuntimeTopology{}, fmt.Errorf("scan runtime topology: %w", err)
		}
		id := uuidString(runtimeID)
		topology.RegisteredRuntimeCount++
		topology.RuntimeStatusByID[id] = status
		topology.AgentMaxConcurrency[id] = maxConcurrent
		topology.MaxConcurrentTasks += maxConcurrent
		topology.RuntimeLastSeenAtByID[id] = timestamptzPtr(lastSeen)
	}
	return topology, rows.Err()
}

func loadLaneEvents(ctx context.Context, pool *pgxpool.Pool, issueID string) ([]lanehealth.LaneEvent, error) {
	rows, err := pool.Query(ctx, `
SELECT details, created_at
FROM activity_log
WHERE issue_id = $1::uuid AND action = 'status_changed'
ORDER BY created_at ASC`, issueID)
	if err != nil {
		return nil, fmt.Errorf("load lane events: %w", err)
	}
	defer rows.Close()

	var events []lanehealth.LaneEvent
	for rows.Next() {
		var raw []byte
		var created pgtype.Timestamptz
		if err := rows.Scan(&raw, &created); err != nil {
			return nil, fmt.Errorf("scan lane event: %w", err)
		}
		var details map[string]any
		if err := json.Unmarshal(raw, &details); err != nil {
			continue
		}
		to := fmt.Sprint(details["to"])
		if to == "" || to == "<nil>" {
			continue
		}
		events = append(events, lanehealth.LaneEvent{
			From:      fmt.Sprint(details["from"]),
			To:        to,
			CreatedAt: created.Time.UTC(),
		})
	}
	return events, rows.Err()
}

func loadLatestHandoffText(ctx context.Context, pool *pgxpool.Pool, issueID string) (string, error) {
	var content string
	err := pool.QueryRow(ctx, `
SELECT content
FROM comment
WHERE issue_id = $1::uuid
  AND type = 'comment'
  AND content ILIKE '%task_id:%'
  AND content ILIKE '%recommendation:%'
ORDER BY created_at DESC
LIMIT 1`, issueID).Scan(&content)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load latest handoff: %w", err)
	}
	return content, nil
}

func loadGateChecks(metadata map[string]any) ([]lanehealth.GateCheck, error) {
	runID, _ := metadata["forgepilot_run_id"].(string)
	if strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	path := ".forgepilot/runs/" + runID + "/trace.json"
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read forgepilot trace: %w", err)
	}
	return extractTraceChecks(raw), nil
}

func extractTraceChecks(raw []byte) []lanehealth.GateCheck {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	var checks []lanehealth.GateCheck
	walkJSON(payload, func(object map[string]any) {
		name := firstString(object, "name", "id", "check", "command_ref")
		status := strings.ToUpper(firstString(object, "status", "result", "outcome"))
		if name == "" || status == "" {
			return
		}
		if status == "PASS" || status == "PASSED" || status == "FAIL" || status == "FAILED" || status == "ERROR" {
			checks = append(checks, lanehealth.GateCheck{Name: name, Status: status})
		}
	})
	return checks
}

func walkJSON(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkJSON(child, visit)
		}
	}
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func loadResidualChildrenCount(ctx context.Context, pool *pgxpool.Pool, issueID string) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)::int
FROM issue
WHERE parent_issue_id = $1::uuid AND status = 'backlog'`, issueID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("load residual children count: %w", err)
	}
	return count, nil
}

func uuidString(value pgtype.UUID) string {
	return util.UUIDToString(value)
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}
