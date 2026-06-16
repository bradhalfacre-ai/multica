package lanehealth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildReportC2123RuntimeRecoveryAndStarvationUnhealthy(t *testing.T) {
	now := mustTime(t, "2026-06-16T19:10:00Z")
	queuedAt := mustTime(t, "2026-06-16T19:05:13Z")
	failedAt := mustTime(t, "2026-06-16T19:09:13Z")

	report, err := BuildReport(Snapshot{
		Issue: IssueSnapshot{
			ID:          "4e03e0fd-bb4a-4053-b486-86cb94fb39f3",
			Identifier:  "C2-123",
			WorkspaceID: "6e37bde9-b644-4371-92db-fde4b1d63f80",
			Status:      "done",
			Metadata: map[string]any{
				"forgepilot_required": true,
				"forgepilot_run_id":   "run_20260616T184004512595Z",
				"branch":              "agent/aegis-implementation-agent/5e29bd02",
				"commit":              "1a4b02e50339027285ba2e0163f6e29d37cd9ac7",
				"checks":              "2/2 PASS",
			},
		},
		Tasks: []TaskSnapshot{
			{
				ID:            "50f9d895",
				AgentID:       "security-gate-agent",
				RuntimeID:     "07a46a62",
				Status:        "failed",
				CreatedAt:     failedAt.Add(-4 * time.Minute),
				Attempt:       1,
				FailureReason: "runtime_recovery",
			},
			{
				ID:           "b847a611",
				AgentID:      "security-gate-agent",
				RuntimeID:    "07a46a62",
				Status:       "queued",
				CreatedAt:    queuedAt,
				Attempt:      2,
				ParentTaskID: "50f9d895",
			},
		},
		HandoffText: handoffFixture(),
		DaemonHealth: DaemonHealth{
			Available:       true,
			Status:          "running",
			ActiveTaskCount: int64Ptr(0),
		},
		Now: now,
	})
	if err != nil {
		t.Fatalf("BuildReport returned error: %v", err)
	}

	if report.Verdict != VerdictUnhealthy {
		t.Fatalf("verdict = %s, want %s", report.Verdict, VerdictUnhealthy)
	}
	assertDriver(t, report.Drivers, "queued_age_seconds")
	assertDriver(t, report.Drivers, "runtime_recovery_count")
	if report.Metrics.QueuedAgeSeconds != 287 {
		t.Fatalf("queued age = %d, want 287", report.Metrics.QueuedAgeSeconds)
	}
	if report.Metrics.RuntimeRecoveryCount != 1 {
		t.Fatalf("runtime recovery count = %d, want 1", report.Metrics.RuntimeRecoveryCount)
	}
	if report.Metrics.RetryCount != 1 {
		t.Fatalf("retry count = %d, want 1", report.Metrics.RetryCount)
	}
}

func TestBuildReportC2142CleanSOPCompletionHealthy(t *testing.T) {
	now := mustTime(t, "2026-06-16T17:00:00Z")
	doneAt := mustTime(t, "2026-06-16T16:59:20Z")

	report, err := BuildReport(Snapshot{
		Issue: IssueSnapshot{
			ID:          "4ad63f2d-fdc3-4485-b6f4-e69334b1e27f",
			Identifier:  "C2-142",
			WorkspaceID: "6e37bde9-b644-4371-92db-fde4b1d63f80",
			Status:      "done",
			Metadata: map[string]any{
				"repo_branch":          "main",
				"repo_commit":          "f2ea242",
				"live_multica_updated": true,
				"risk_tier":            "Tier 2 - governance SOP",
			},
		},
		Tasks: []TaskSnapshot{
			{
				ID:        "c2-142-clean-task",
				AgentID:   "implementation-agent",
				RuntimeID: "07a46a62",
				Status:    "completed",
				CreatedAt: doneAt.Add(-5 * time.Minute),
				Attempt:   1,
			},
		},
		LaneEvents:  []LaneEvent{{From: "in_review", To: "done", CreatedAt: doneAt}},
		HandoffText: handoffFixture(),
		Now:         now,
	})
	if err != nil {
		t.Fatalf("BuildReport returned error: %v", err)
	}

	if report.Verdict != VerdictHealthy {
		t.Fatalf("verdict = %s, want %s; drivers=%v", report.Verdict, VerdictHealthy, report.Drivers)
	}
	if len(report.Drivers) != 0 {
		t.Fatalf("drivers = %v, want none", report.Drivers)
	}
	if report.Metrics.MissingForgePilotFieldsCount != 0 {
		t.Fatalf("missing forgepilot fields = %d, want 0", report.Metrics.MissingForgePilotFieldsCount)
	}
	if report.Metrics.MissingHandoffFieldsCount != 0 {
		t.Fatalf("missing handoff fields = %d, want 0", report.Metrics.MissingHandoffFieldsCount)
	}
}

func TestBuildReportGateFailureOutputDoesNotIncludeFreeTextReason(t *testing.T) {
	report, err := BuildReport(Snapshot{
		Issue: IssueSnapshot{
			ID:          "issue-id",
			Identifier:  "C2-999",
			WorkspaceID: "workspace-id",
			Status:      "in_review",
		},
		HandoffText: handoffFixture(),
		GateChecks: []GateCheck{
			{Name: "implementation", Status: "FAIL: do not emit this reason"},
			{Name: "evidence", Status: "PASS"},
		},
		Now: mustTime(t, "2026-06-16T17:00:00Z"),
	})
	if err != nil {
		t.Fatalf("BuildReport returned error: %v", err)
	}

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if strings.Contains(string(raw), "do not emit this reason") {
		t.Fatalf("report leaked free-text gate reason: %s", raw)
	}
	if len(report.Metrics.GateFailureReason) != 1 {
		t.Fatalf("gate failures = %d, want 1", len(report.Metrics.GateFailureReason))
	}
	if report.Metrics.GateFailureReason[0].Status != "FAIL" {
		t.Fatalf("gate status = %q, want FAIL", report.Metrics.GateFailureReason[0].Status)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func int64Ptr(value int64) *int64 {
	return &value
}

func assertDriver(t *testing.T, drivers []string, want string) {
	t.Helper()
	if !slicesContains(drivers, want) {
		t.Fatalf("drivers = %v, want %q", drivers, want)
	}
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func handoffFixture() string {
	return `task_id: C2-142
objective: Define Aegis SOP
risk_tier: Tier 2
mode: act
route: aegis_lane_health_metrics
scope: test
out_of_scope: none
files_changed: docs
acceptance_criteria: PASS
security_relevance: none
data_relevance: none
tenant_rbac_relevance: none
tests_required: yes
commands_run: git diff --check
forgepilot: not required
evidence: done
open_questions: none
residual_risks: none
recommendation: done`
}
