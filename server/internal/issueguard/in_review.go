package issueguard

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RepoVisibilityOverrideKey = "repo_visibility_override"

	repoVisibilityFreshnessWindow = 24 * time.Hour
	repoVisibilityFutureSkew      = 5 * time.Minute
)

var AgentProtectedMetadataKeys = map[string]struct{}{
	RepoVisibilityOverrideKey:                  {},
	"repo_visibility_override_actor_type":      {},
	"repo_visibility_override_actor_id":        {},
	"repo_visibility_override_reason":          {},
	"repo_visibility_override_at":              {},
	"repo_visibility_attestation":              {},
	"repo_visibility_branch":                   {},
	"repo_visibility_commit":                   {},
	"repo_visibility_attested_branch":          {},
	"repo_visibility_attested_commit":          {},
	"repo_visibility_attested_at":              {},
	"repo_visibility_attestation_checked_at":   {},
	"repo_visibility_attestation_checked_by":   {},
	"repo_visibility_attestation_checked_type": {},
}

var (
	ErrRepoVisibilityAttestationRequired = errors.New("repo visibility attestation required before moving this issue to in_review")
	ErrRepoVisibilityOverrideRejected    = errors.New("repo visibility override is not authorized for this transition")
)

type InReviewGuardResult struct {
	Required       bool
	OverrideUsed   bool
	OverrideReason string
	OverrideActor  string
	OverrideAt     string
}

func EvaluateInReviewTransition(prevStatus, nextStatus string, metadata map[string]any, actorType, actorID string, now time.Time) (InReviewGuardResult, error) {
	if nextStatus != "in_review" || prevStatus == "in_review" {
		return InReviewGuardResult{}, nil
	}
	if !metadataTruthy(metadata, "forgepilot_required") && !metadataTruthy(metadata, "source_writing") {
		return InReviewGuardResult{}, nil
	}

	result := InReviewGuardResult{Required: true}
	if hasRepoVisibilityOverride(metadata) {
		ok, reason, actor, at := authorizedRepoVisibilityOverride(metadata, actorType, actorID)
		if !ok {
			return result, ErrRepoVisibilityOverrideRejected
		}
		result.OverrideUsed = true
		result.OverrideReason = reason
		result.OverrideActor = actor
		result.OverrideAt = at
		return result, nil
	}

	if validRepoVisibilityAttestation(metadata, now) {
		return result, nil
	}
	return result, ErrRepoVisibilityAttestationRequired
}

func IsAgentProtectedMetadataKey(key string) bool {
	_, ok := AgentProtectedMetadataKeys[key]
	return ok
}

func hasRepoVisibilityOverride(metadata map[string]any) bool {
	return metadataTruthy(metadata, RepoVisibilityOverrideKey)
}

func authorizedRepoVisibilityOverride(metadata map[string]any, requestActorType, requestActorID string) (bool, string, string, string) {
	metadataActorType := strings.ToLower(strings.TrimSpace(metadataString(metadata, "repo_visibility_override_actor_type")))
	metadataActorID := strings.TrimSpace(metadataString(metadata, "repo_visibility_override_actor_id"))
	requestActorType = strings.ToLower(strings.TrimSpace(requestActorType))
	requestActorID = strings.TrimSpace(requestActorID)
	reason := strings.TrimSpace(metadataString(metadata, "repo_visibility_override_reason"))
	at := strings.TrimSpace(metadataString(metadata, "repo_visibility_override_at"))

	if requestActorType == "agent" || requestActorType == "" || requestActorID == "" || reason == "" || at == "" {
		return false, "", "", ""
	}
	if requestActorType != "member" && requestActorType != "orchestrator" && requestActorType != "system" {
		return false, "", "", ""
	}
	if metadataActorType == "" || metadataActorID == "" {
		return false, "", "", ""
	}
	if metadataActorType != requestActorType || metadataActorID != requestActorID {
		return false, "", "", ""
	}
	if _, err := time.Parse(time.RFC3339, at); err != nil {
		return false, "", "", ""
	}
	return true, reason, fmt.Sprintf("%s:%s", requestActorType, requestActorID), at
}

func validRepoVisibilityAttestation(metadata map[string]any, now time.Time) bool {
	if !metadataTruthy(metadata, "repo_visibility_attestation") {
		return false
	}

	pinnedBranch := firstMetadataString(metadata, "pinned_branch", "forgepilot_branch", "repo_branch", "branch")
	pinnedCommit := firstMetadataString(metadata, "pinned_commit", "forgepilot_commit", "repo_commit", "commit")
	attestedBranch := firstMetadataString(metadata, "repo_visibility_branch", "repo_visibility_attested_branch")
	attestedCommit := firstMetadataString(metadata, "repo_visibility_commit", "repo_visibility_attested_commit")
	attestedAt := strings.TrimSpace(metadataString(metadata, "repo_visibility_attested_at"))

	if pinnedBranch == "" || pinnedCommit == "" || attestedBranch == "" || attestedCommit == "" || attestedAt == "" {
		return false
	}
	if pinnedBranch != attestedBranch || pinnedCommit != attestedCommit {
		return false
	}

	t, err := time.Parse(time.RFC3339, attestedAt)
	if err != nil {
		return false
	}
	if t.After(now.Add(repoVisibilityFutureSkew)) {
		return false
	}
	return now.Sub(t) <= repoVisibilityFreshnessWindow
}

func firstMetadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadataString(metadata, key)); value != "" {
			return value
		}
	}
	return ""
}

func metadataTruthy(metadata map[string]any, key string) bool {
	v, ok := metadata[key]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "required", "valid", "passed", "approved":
			return true
		default:
			return false
		}
	case float64:
		return x != 0
	default:
		return false
	}
}

func metadataString(metadata map[string]any, key string) string {
	v, ok := metadata[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
