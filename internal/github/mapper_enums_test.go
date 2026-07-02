package github_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// --- StateCategory mapping ---

func TestMapStateCategory(t *testing.T) {
	tests := []struct {
		name        string
		state       string
		stateReason string
		want        provider.StateCategory
	}{
		// open → Active
		{name: "open", state: "open", stateReason: "", want: provider.StateCategoryActive},
		{name: "open mixed case", state: "Open", stateReason: "", want: provider.StateCategoryActive},
		{name: "open with irrelevant reason", state: "open", stateReason: "completed", want: provider.StateCategoryActive},

		// closed + completed → ClosedDone
		{name: "closed completed", state: "closed", stateReason: "completed", want: provider.StateCategoryClosedDone},
		{name: "closed completed mixed case", state: "Closed", stateReason: "completed", want: provider.StateCategoryClosedDone},
		{name: "closed empty reason", state: "closed", stateReason: "", want: provider.StateCategoryClosedDone},

		// closed + not_planned → Removed
		{name: "closed not_planned", state: "closed", stateReason: "not_planned", want: provider.StateCategoryRemoved},
		{name: "closed not_planned mixed case", state: "closed", stateReason: "NOT_PLANNED", want: provider.StateCategoryRemoved},

		// unknown / reopened → Active (safe default)
		{name: "empty state", state: "", stateReason: "", want: provider.StateCategoryActive},
		{name: "unknown state", state: "merged", stateReason: "", want: provider.StateCategoryActive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := github.MapStateCategory(tt.state, tt.stateReason)
			if got != tt.want {
				t.Errorf("MapStateCategory(%q, %q) = %v, want %v", tt.state, tt.stateReason, got, tt.want)
			}
		})
	}
}

// --- VoteKind mapping ---

func TestMapVoteKind(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  provider.VoteKind
	}{
		{name: "APPROVED", input: "APPROVED", want: provider.VoteKindApproved},
		{name: "approved lowercase", input: "approved", want: provider.VoteKindApproved},
		{name: "CHANGES_REQUESTED", input: "CHANGES_REQUESTED", want: provider.VoteKindRejected},
		{name: "changes_requested lowercase", input: "changes_requested", want: provider.VoteKindRejected},
		// non-vote states all collapse to NoVote
		{name: "COMMENTED", input: "COMMENTED", want: provider.VoteKindNoVote},
		{name: "PENDING", input: "PENDING", want: provider.VoteKindNoVote},
		{name: "DISMISSED", input: "DISMISSED", want: provider.VoteKindNoVote},
		{name: "empty", input: "", want: provider.VoteKindNoVote},
		{name: "unknown", input: "SOMETHING_ELSE", want: provider.VoteKindNoVote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := github.MapVoteKind(tt.input)
			if got != tt.want {
				t.Errorf("MapVoteKind(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- RunStatus mapping ---

func TestMapRunStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		conclusion string
		want       provider.RunStatus
	}{
		// In-flight statuses — conclusion is irrelevant
		{name: "queued", status: "queued", conclusion: "", want: provider.RunStatusQueued},
		{name: "requested", status: "requested", conclusion: "", want: provider.RunStatusQueued},
		{name: "pending", status: "pending", conclusion: "", want: provider.RunStatusQueued},
		{name: "queued mixed case", status: "Queued", conclusion: "", want: provider.RunStatusQueued},
		{name: "waiting", status: "waiting", conclusion: "", want: provider.RunStatusPending},
		{name: "waiting mixed case", status: "Waiting", conclusion: "", want: provider.RunStatusPending},
		{name: "in_progress", status: "in_progress", conclusion: "", want: provider.RunStatusRunning},
		{name: "in_progress mixed case", status: "In_Progress", conclusion: "", want: provider.RunStatusRunning},

		// status takes priority over conclusion
		{name: "in_progress ignores success conclusion", status: "in_progress", conclusion: "success", want: provider.RunStatusRunning},

		// completed + each conclusion
		{name: "completed success", status: "completed", conclusion: "success", want: provider.RunStatusSucceeded},
		{name: "completed Success mixed", status: "completed", conclusion: "Success", want: provider.RunStatusSucceeded},
		{name: "completed failure", status: "completed", conclusion: "failure", want: provider.RunStatusFailed},
		{name: "completed timed_out", status: "completed", conclusion: "timed_out", want: provider.RunStatusFailed},
		{name: "completed startup_failure", status: "completed", conclusion: "startup_failure", want: provider.RunStatusFailed},
		{name: "completed cancelled", status: "completed", conclusion: "cancelled", want: provider.RunStatusCanceled},
		{name: "completed Cancelled mixed", status: "completed", conclusion: "Cancelled", want: provider.RunStatusCanceled},

		// completed + non-decisive conclusions → Unknown
		{name: "completed skipped", status: "completed", conclusion: "skipped", want: provider.RunStatusUnknown},
		{name: "completed neutral", status: "completed", conclusion: "neutral", want: provider.RunStatusUnknown},
		{name: "completed stale", status: "completed", conclusion: "stale", want: provider.RunStatusUnknown},
		{name: "completed action_required", status: "completed", conclusion: "action_required", want: provider.RunStatusUnknown},
		{name: "completed unknown conclusion", status: "completed", conclusion: "something_new", want: provider.RunStatusUnknown},
		{name: "completed empty conclusion", status: "completed", conclusion: "", want: provider.RunStatusUnknown},

		// unknown status → Unknown
		{name: "empty status and conclusion", status: "", conclusion: "", want: provider.RunStatusUnknown},
		{name: "unknown status", status: "postponed", conclusion: "", want: provider.RunStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := github.MapRunStatus(tt.status, tt.conclusion)
			if got != tt.want {
				t.Errorf("MapRunStatus(%q, %q) = %v, want %v", tt.status, tt.conclusion, got, tt.want)
			}
		})
	}
}
