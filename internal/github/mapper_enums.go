package github

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapStateCategory translates a GitHub issue state + state_reason pair into a
// neutral provider.StateCategory.
//
// GitHub issues have two states ("open", "closed") and a state_reason
// ("completed", "not_planned", or null/empty).
//
// Decision (Unknown #4): the two state_reason values map to distinct categories:
//   - "open" / unknown          → StateCategoryActive
//   - "closed" + "not_planned"  → StateCategoryRemoved  (issue was triaged away)
//   - "closed" + "completed"    → StateCategoryClosedDone (happy-path terminal)
//   - "closed" + "" / null      → StateCategoryClosedDone (legacy or API omission)
//
// This mirrors how Azure DevOps maps "removed"→Removed vs "closed"→ClosedDone,
// giving the UI meaningful color differentiation between won't-fix and done issues.
// Keeping them distinct (rather than collapsing both into ClosedDone) lets the
// StateCategoryRemoved glyph convey the "explicitly deprioritised" signal.
func MapStateCategory(state, stateReason string) provider.StateCategory {
	switch strings.ToLower(state) {
	case "open":
		return provider.StateCategoryActive
	case "closed":
		if strings.ToLower(stateReason) == "not_planned" {
			return provider.StateCategoryRemoved
		}
		return provider.StateCategoryClosedDone
	default:
		// Unknown or future states (e.g. "reopened") default to Active — the safe
		// fallback that does not hide items from the user.
		return provider.StateCategoryActive
	}
}

// MapVoteKind translates a GitHub pull request review state string into a
// neutral provider.VoteKind.
//
// GitHub review states: APPROVED, CHANGES_REQUESTED, COMMENTED, PENDING, DISMISSED.
//
// Note: GitHub has no "approved with suggestions" or "waiting for author" concept.
// Those provider.VoteKind variants (VoteKindApprovedWithSuggestions,
// VoteKindWaitingForAuthor) are Azure DevOps-only and remain unused for GitHub.
// COMMENTED, PENDING, and DISMISSED all map to VoteKindNoVote: none of them
// express a definitive approve or reject, so the reviewer is treated as not yet
// voted.
func MapVoteKind(reviewState string) provider.VoteKind {
	switch strings.ToUpper(reviewState) {
	case "APPROVED":
		return provider.VoteKindApproved
	case "CHANGES_REQUESTED":
		return provider.VoteKindRejected
	default:
		// COMMENTED, PENDING, DISMISSED, and any unknown future value → no vote.
		return provider.VoteKindNoVote
	}
}

// MapRunStatus translates a GitHub Actions status+conclusion pair into a
// neutral provider.RunStatus. Status is checked before conclusion because
// in-flight statuses (in_progress, queued, waiting) are authoritative
// regardless of any stale conclusion field from a previous attempt.
//
// GitHub Actions status values: queued, requested, pending, waiting, in_progress,
// completed.
// GitHub Actions conclusion values: success, failure, timed_out, startup_failure,
// cancelled (British spelling), skipped, neutral, stale, action_required.
//
// Mapping rationale:
//   - "queued"/"requested"/"pending" → RunStatusQueued (not yet dispatched)
//   - "waiting" → RunStatusPending (waiting on a deployment/approval gate,
//     analogous to Azure DevOps "pending" for approval gates)
//   - "in_progress" → RunStatusRunning
//   - "completed"+"success" → RunStatusSucceeded
//   - "completed"+"failure"/"timed_out"/"startup_failure" → RunStatusFailed
//   - "completed"+"cancelled" → RunStatusCanceled
//     (GitHub spells it "cancelled"; the neutral enum uses "Canceled")
//   - "completed"+"skipped"/"neutral"/"stale"/"action_required"/unknown
//     → RunStatusUnknown (non-decisive; not a success, not a user-visible failure)
func MapRunStatus(status, conclusion string) provider.RunStatus {
	switch strings.ToLower(status) {
	case "queued", "requested", "pending":
		return provider.RunStatusQueued
	case "waiting":
		return provider.RunStatusPending
	case "in_progress":
		return provider.RunStatusRunning
	case "completed":
		switch strings.ToLower(conclusion) {
		case "success":
			return provider.RunStatusSucceeded
		case "failure", "timed_out", "startup_failure":
			return provider.RunStatusFailed
		case "cancelled":
			return provider.RunStatusCanceled
		default:
			// skipped, neutral, stale, action_required, empty, or any future value.
			return provider.RunStatusUnknown
		}
	default:
		return provider.RunStatusUnknown
	}
}
