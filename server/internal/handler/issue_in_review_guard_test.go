package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func createInReviewGuardIssue(t *testing.T, metadata map[string]any) string {
	t.Helper()

	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var issueID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO issue (
			workspace_id, title, status, priority, creator_type, creator_id, number, metadata
		)
		VALUES (
			$1, $2, 'in_progress', 'high', 'member', $3,
			(SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1),
			$4::jsonb
		)
		RETURNING id
	`, testWorkspaceID, "in review guard test "+time.Now().Format(time.RFC3339Nano), testUserID, string(rawMetadata)).Scan(&issueID); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
	})
	return issueID
}

func attachForgePilotRequiredLabel(t *testing.T, issueID string) {
	t.Helper()

	var labelID string
	err := testPool.QueryRow(context.Background(), `
		SELECT id FROM issue_label
		WHERE workspace_id = $1 AND LOWER(name) = LOWER('forgepilot:required')
		LIMIT 1
	`, testWorkspaceID).Scan(&labelID)
	if err != nil {
		if err := testPool.QueryRow(context.Background(), `
			INSERT INTO issue_label (workspace_id, name, color)
			VALUES ($1, 'forgepilot:required', '#2563eb')
			RETURNING id
		`, testWorkspaceID).Scan(&labelID); err != nil {
			t.Fatalf("create forgepilot label: %v", err)
		}
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO issue_to_label (issue_id, label_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, issueID, labelID); err != nil {
		t.Fatalf("attach forgepilot label: %v", err)
	}
}

func updateIssueStatusForGuard(t *testing.T, issueID string, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.UpdateIssue(w, withURLParam(req, "id", issueID))
	return w
}

func withGuardURLParams(req *http.Request, kv ...string) *http.Request {
	rctx := chi.NewRouteContext()
	for i := 0; i+1 < len(kv); i += 2 {
		rctx.URLParams.Add(kv[i], kv[i+1])
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestUpdateIssueInReviewGuardRejectsMissingAttestation(t *testing.T) {
	issueID := createInReviewGuardIssue(t, map[string]any{
		"forgepilot_required": true,
		"pinned_branch":       "agent/test",
		"pinned_commit":       "abc123",
	})

	w := updateIssueStatusForGuard(t, issueID, newRequest("PUT", "/api/issues/"+issueID, map[string]any{
		"status": "in_review",
	}))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for missing attestation, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "repo visibility attestation required") {
		t.Fatalf("expected attestation error, got %s", w.Body.String())
	}
}

func TestUpdateIssueInReviewGuardTreatsForgePilotRequiredLabelAsGuarded(t *testing.T) {
	issueID := createInReviewGuardIssue(t, map[string]any{
		"pinned_branch": "agent/test",
		"pinned_commit": "abc123",
	})
	attachForgePilotRequiredLabel(t, issueID)

	w := updateIssueStatusForGuard(t, issueID, newRequest("PUT", "/api/issues/"+issueID, map[string]any{
		"status": "in_review",
	}))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for label-only guarded issue without attestation, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateIssueInReviewGuardAcceptsMatchingAttestation(t *testing.T) {
	issueID := createInReviewGuardIssue(t, map[string]any{
		"forgepilot_required":         true,
		"pinned_branch":               "agent/test",
		"pinned_commit":               "abc123",
		"repo_visibility_attestation": "passed",
		"repo_visibility_branch":      "agent/test",
		"repo_visibility_commit":      "abc123",
		"repo_visibility_attested_at": time.Now().UTC().Format(time.RFC3339),
	})

	w := updateIssueStatusForGuard(t, issueID, newRequest("PUT", "/api/issues/"+issueID, map[string]any{
		"status": "in_review",
	}))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching attestation, got %d: %s", w.Code, w.Body.String())
	}
	var resp IssueResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "in_review" {
		t.Fatalf("expected status in_review, got %q", resp.Status)
	}
}

func TestUpdateIssueInReviewGuardAcceptsAuthorizedOverrideAndAudits(t *testing.T) {
	overrideAt := time.Now().UTC().Format(time.RFC3339)
	issueID := createInReviewGuardIssue(t, map[string]any{
		"forgepilot_required":                    true,
		"pinned_branch":                          "agent/test",
		"pinned_commit":                          "abc123",
		"repo_visibility_override":               "approved",
		"repo_visibility_override_actor_type":    "member",
		"repo_visibility_override_actor_id":      testUserID,
		"repo_visibility_override_reason":        "human verified remote branch and commit",
		"repo_visibility_override_at":            overrideAt,
		"repo_visibility_attestation":            "",
		"repo_visibility_attestation_checked_at": overrideAt,
	})

	w := updateIssueStatusForGuard(t, issueID, newRequest("PUT", "/api/issues/"+issueID, map[string]any{
		"status": "in_review",
	}))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for authorized override, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	if err := testPool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM activity_log
		WHERE issue_id = $1
		  AND action = 'repo_visibility_override_used'
		  AND details->>'override_reason' = 'human verified remote branch and commit'
	`, issueID).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one repo visibility override audit row, got %d", count)
	}
}

func TestSetIssueMetadataRejectsAgentAuthoredRepoVisibilityOverride(t *testing.T) {
	issueID := createInReviewGuardIssue(t, map[string]any{
		"forgepilot_required": true,
	})
	agentID := createHandlerTestAgent(t, "repo-visibility-override-agent", []byte(`{}`))
	taskID := createHandlerTestTaskForAgent(t, agentID)

	req := newRequest("PUT", "/api/issues/"+issueID+"/metadata/repo_visibility_override", json.RawMessage(`{"value":"approved"}`))
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	w := httptest.NewRecorder()
	testHandler.SetIssueMetadataKey(w, withGuardURLParams(req, "id", issueID, "key", "repo_visibility_override"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for agent-authored override, got %d: %s", w.Code, w.Body.String())
	}
}
