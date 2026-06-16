package lanehealth

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	VerdictHealthy   = "HEALTHY"
	VerdictUnhealthy = "UNHEALTHY"
)

var handoffFields = []string{
	"task_id:",
	"objective:",
	"risk_tier:",
	"mode:",
	"route:",
	"scope:",
	"out_of_scope:",
	"files_changed:",
	"acceptance_criteria:",
	"security_relevance:",
	"data_relevance:",
	"tenant_rbac_relevance:",
	"tests_required:",
	"commands_run:",
	"forgepilot:",
	"evidence:",
	"open_questions:",
	"residual_risks:",
	"recommendation:",
}

var forgePilotFields = []string{"contract", "run_id", "branch", "commit", "checks"}

type Snapshot struct {
	Issue                 IssueSnapshot
	Tasks                 []TaskSnapshot
	RuntimeTopology       RuntimeTopology
	LaneEvents            []LaneEvent
	HandoffText           string
	GateChecks            []GateCheck
	ResidualChildrenCount int
	DaemonHealth          DaemonHealth
	Now                   time.Time
}

type IssueSnapshot struct {
	ID           string         `json:"id"`
	Identifier   string         `json:"identifier"`
	WorkspaceID  string         `json:"workspace_id"`
	Status       string         `json:"status"`
	AssigneeType string         `json:"assignee_type,omitempty"`
	AssigneeID   string         `json:"assignee_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type TaskSnapshot struct {
	ID            string     `json:"id"`
	AgentID       string     `json:"agent_id"`
	RuntimeID     string     `json:"runtime_id"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	DispatchedAt  *time.Time `json:"dispatched_at,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Attempt       int        `json:"attempt"`
	ParentTaskID  string     `json:"parent_task_id,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
}

type RuntimeTopology struct {
	RegisteredRuntimeCount int                   `json:"registered_runtime_count"`
	MaxConcurrentTasks     int                   `json:"max_concurrent_tasks"`
	RuntimeLastSeenAgeSec  *int64                `json:"runtime_last_seen_age_seconds,omitempty"`
	QueueCountByRuntime    map[string]int        `json:"queue_count_by_runtime"`
	RuntimeStatusByID      map[string]string     `json:"runtime_status_by_id"`
	RuntimeLastSeenAtByID  map[string]*time.Time `json:"runtime_last_seen_at_by_id,omitempty"`
	AgentMaxConcurrency    map[string]int        `json:"agent_max_concurrency,omitempty"`
}

type LaneEvent struct {
	From      string    `json:"from,omitempty"`
	To        string    `json:"to"`
	CreatedAt time.Time `json:"created_at"`
}

type GateCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type DaemonHealth struct {
	Available       bool   `json:"available"`
	Status          string `json:"status,omitempty"`
	ActiveTaskCount *int64 `json:"active_task_count,omitempty"`
}

type Report struct {
	IssueID     string        `json:"issue_id"`
	Identifier  string        `json:"identifier"`
	WorkspaceID string        `json:"workspace_id"`
	Status      string        `json:"status"`
	Verdict     string        `json:"verdict"`
	Drivers     []string      `json:"drivers"`
	Metrics     Metrics       `json:"metrics"`
	Context     ReportContext `json:"context"`
}

type Metrics struct {
	TimeInLaneSeconds            int64       `json:"time_in_lane_seconds"`
	QueuedAgeSeconds             int64       `json:"queued_age_seconds"`
	MissingForgePilotFieldsCount int         `json:"missing_forgepilot_fields_count"`
	MissingHandoffFieldsCount    int         `json:"missing_handoff_fields_count"`
	RuntimeRecoveryCount         int         `json:"runtime_recovery_count"`
	RetryCount                   int         `json:"retry_count"`
	AssignmentRuntimeMismatch    bool        `json:"assignment_runtime_mismatch"`
	GateFailureReason            []GateCheck `json:"gate_failure_reason"`
	ResidualChildrenCount        int         `json:"residual_children_count"`
}

type ReportContext struct {
	DispatchLatencySeconds     *int64          `json:"dispatch_latency_seconds,omitempty"`
	StartLatencySeconds        *int64          `json:"start_latency_seconds,omitempty"`
	StaleCancelledResumesCount int             `json:"stale_cancelled_resumes_count"`
	RuntimeTopology            RuntimeTopology `json:"runtime_topology"`
	DaemonHealth               DaemonHealth    `json:"daemon_health"`
}

func BuildReport(snapshot Snapshot) (Report, error) {
	if snapshot.Issue.ID == "" {
		return Report{}, fmt.Errorf("issue id is required")
	}
	now := snapshot.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	normalizeSnapshot(&snapshot)

	metrics := Metrics{
		TimeInLaneSeconds:            timeInLaneSeconds(snapshot.Issue.Status, snapshot.LaneEvents, now),
		QueuedAgeSeconds:             queuedAgeSeconds(snapshot.Tasks, now),
		MissingForgePilotFieldsCount: missingForgePilotFieldsCount(snapshot.Issue.Metadata),
		MissingHandoffFieldsCount:    missingHandoffFieldsCount(snapshot.HandoffText),
		RuntimeRecoveryCount:         runtimeRecoveryCount(snapshot.Tasks),
		RetryCount:                   retryCount(snapshot.Tasks),
		AssignmentRuntimeMismatch:    assignmentRuntimeMismatch(snapshot.Issue, snapshot.Tasks),
		GateFailureReason:            failedGateChecks(snapshot.GateChecks),
		ResidualChildrenCount:        snapshot.ResidualChildrenCount,
	}

	context := ReportContext{
		DispatchLatencySeconds:     maxDispatchLatencySeconds(snapshot.Tasks),
		StartLatencySeconds:        maxStartLatencySeconds(snapshot.Tasks),
		StaleCancelledResumesCount: staleCancelledResumesCount(snapshot.Tasks),
		RuntimeTopology:            snapshot.RuntimeTopology,
		DaemonHealth:               snapshot.DaemonHealth,
	}

	drivers := verdictDrivers(metrics)
	verdict := VerdictHealthy
	if len(drivers) > 0 {
		verdict = VerdictUnhealthy
	}

	return Report{
		IssueID:     snapshot.Issue.ID,
		Identifier:  snapshot.Issue.Identifier,
		WorkspaceID: snapshot.Issue.WorkspaceID,
		Status:      snapshot.Issue.Status,
		Verdict:     verdict,
		Drivers:     drivers,
		Metrics:     metrics,
		Context:     context,
	}, nil
}

func ParseDaemonHealthJSON(raw []byte) (DaemonHealth, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return DaemonHealth{}, nil
	}
	var payload struct {
		Status          string `json:"status"`
		ActiveTaskCount *int64 `json:"active_task_count"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return DaemonHealth{}, fmt.Errorf("parse daemon health json: %w", err)
	}
	return DaemonHealth{
		Available:       true,
		Status:          payload.Status,
		ActiveTaskCount: payload.ActiveTaskCount,
	}, nil
}

func normalizeSnapshot(snapshot *Snapshot) {
	if snapshot.Issue.Metadata == nil {
		snapshot.Issue.Metadata = map[string]any{}
	}
	if snapshot.RuntimeTopology.QueueCountByRuntime == nil {
		snapshot.RuntimeTopology.QueueCountByRuntime = map[string]int{}
	}
	if snapshot.RuntimeTopology.RuntimeStatusByID == nil {
		snapshot.RuntimeTopology.RuntimeStatusByID = map[string]string{}
	}
}

func timeInLaneSeconds(status string, events []LaneEvent, now time.Time) int64 {
	var latest *time.Time
	for i := range events {
		if events[i].To != status {
			continue
		}
		created := events[i].CreatedAt
		if latest == nil || created.After(*latest) {
			latest = &created
		}
	}
	if latest == nil {
		return 0
	}
	return secondsBetween(*latest, now)
}

func queuedAgeSeconds(tasks []TaskSnapshot, now time.Time) int64 {
	var maxAge int64
	for _, task := range tasks {
		if task.Status == "queued" {
			maxAge = max(maxAge, secondsBetween(task.CreatedAt, now))
		}
	}
	return maxAge
}

func missingForgePilotFieldsCount(metadata map[string]any) int {
	required, _ := metadata["forgepilot_required"].(bool)
	if !required {
		return 0
	}
	count := 0
	for _, field := range forgePilotFields {
		if !hasMetadataAlias(metadata, field) {
			count++
		}
	}
	return count
}

func hasMetadataAlias(metadata map[string]any, field string) bool {
	aliases := map[string][]string{
		"contract": {"forgepilot_contract", "contract"},
		"run_id":   {"forgepilot_run_id", "run_id"},
		"branch":   {"repo_branch", "branch"},
		"commit":   {"repo_commit", "commit", "merged_commit"},
		"checks":   {"checks", "forgepilot_checks"},
	}
	for _, key := range aliases[field] {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
}

func missingHandoffFieldsCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return len(handoffFields)
	}
	lower := strings.ToLower(text)
	count := 0
	for _, field := range handoffFields {
		if !strings.Contains(lower, field) {
			count++
		}
	}
	return count
}

func runtimeRecoveryCount(tasks []TaskSnapshot) int {
	count := 0
	for _, task := range tasks {
		if task.FailureReason == "runtime_recovery" {
			count++
		}
	}
	return count
}

func retryCount(tasks []TaskSnapshot) int {
	count := 0
	for _, task := range tasks {
		if task.Attempt > 1 {
			count += task.Attempt - 1
			continue
		}
		if task.ParentTaskID != "" {
			count++
		}
	}
	return count
}

func assignmentRuntimeMismatch(issue IssueSnapshot, tasks []TaskSnapshot) bool {
	if issue.AssigneeType != "agent" || issue.AssigneeID == "" {
		return false
	}
	intended := strings.TrimSpace(fmt.Sprint(issue.Metadata["intended_runtime_id"]))
	if intended == "" {
		intended = strings.TrimSpace(fmt.Sprint(issue.Metadata["runtime_id"]))
	}
	if intended == "" {
		return false
	}
	for _, task := range tasks {
		if task.AgentID == issue.AssigneeID && task.RuntimeID != "" && task.RuntimeID != intended {
			return true
		}
	}
	return false
}

func failedGateChecks(checks []GateCheck) []GateCheck {
	failures := make([]GateCheck, 0)
	for _, check := range checks {
		status := strings.ToUpper(strings.TrimSpace(check.Status))
		if status == "" {
			continue
		}
		if slices.Contains([]string{"FAIL", "FAILED", "ERROR"}, status) ||
			strings.HasPrefix(status, "FAIL:") ||
			strings.HasPrefix(status, "ERROR:") {
			failures = append(failures, GateCheck{Name: check.Name, Status: "FAIL"})
		}
	}
	return failures
}

func maxDispatchLatencySeconds(tasks []TaskSnapshot) *int64 {
	var result *int64
	for _, task := range tasks {
		if task.DispatchedAt == nil {
			continue
		}
		seconds := secondsBetween(task.CreatedAt, *task.DispatchedAt)
		result = maxPointer(result, seconds)
	}
	return result
}

func maxStartLatencySeconds(tasks []TaskSnapshot) *int64 {
	var result *int64
	for _, task := range tasks {
		if task.DispatchedAt == nil || task.StartedAt == nil {
			continue
		}
		seconds := secondsBetween(*task.DispatchedAt, *task.StartedAt)
		result = maxPointer(result, seconds)
	}
	return result
}

func staleCancelledResumesCount(tasks []TaskSnapshot) int {
	count := 0
	for _, task := range tasks {
		if task.Status == "cancelled" && task.ParentTaskID != "" {
			count++
		}
	}
	return count
}

func verdictDrivers(metrics Metrics) []string {
	var drivers []string
	if metrics.TimeInLaneSeconds > 24*60*60 {
		drivers = append(drivers, "time_in_lane_seconds")
	}
	if metrics.QueuedAgeSeconds > 120 {
		drivers = append(drivers, "queued_age_seconds")
	}
	if metrics.MissingForgePilotFieldsCount > 0 {
		drivers = append(drivers, "missing_forgepilot_fields_count")
	}
	if metrics.MissingHandoffFieldsCount > 0 {
		drivers = append(drivers, "missing_handoff_fields_count")
	}
	if metrics.RuntimeRecoveryCount > 0 {
		drivers = append(drivers, "runtime_recovery_count")
	}
	if metrics.RetryCount > 2 {
		drivers = append(drivers, "retry_count")
	}
	if metrics.AssignmentRuntimeMismatch {
		drivers = append(drivers, "assignment_runtime_mismatch")
	}
	if len(metrics.GateFailureReason) > 0 {
		drivers = append(drivers, "gate_failure_reason")
	}
	return drivers
}

func secondsBetween(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return int64(end.Sub(start).Seconds())
}

func maxPointer(current *int64, candidate int64) *int64 {
	if current == nil || candidate > *current {
		value := candidate
		return &value
	}
	return current
}
