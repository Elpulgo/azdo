package azdevops_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
)

// --- StateCategory mapping ---

func TestMapStateCategory(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  provider.StateCategory
	}{
		{name: "new lowercase", input: "new", want: provider.StateCategoryNew},
		{name: "new mixed case", input: "New", want: provider.StateCategoryNew},
		{name: "new uppercase", input: "NEW", want: provider.StateCategoryNew},
		{name: "active", input: "active", want: provider.StateCategoryActive},
		{name: "Active mixed", input: "Active", want: provider.StateCategoryActive},
		{name: "resolved", input: "resolved", want: provider.StateCategoryResolved},
		{name: "Resolved mixed", input: "Resolved", want: provider.StateCategoryResolved},
		{name: "ready for test", input: "ready for test", want: provider.StateCategoryReadyForTest},
		{name: "Ready For Test", input: "Ready For Test", want: provider.StateCategoryReadyForTest},
		{name: "ready for review", input: "ready for review", want: provider.StateCategoryReadyForTest},
		{name: "closed", input: "closed", want: provider.StateCategoryClosedDone},
		{name: "Closed", input: "Closed", want: provider.StateCategoryClosedDone},
		{name: "removed", input: "removed", want: provider.StateCategoryRemoved},
		{name: "Removed", input: "Removed", want: provider.StateCategoryRemoved},
		{name: "empty", input: "", want: provider.StateCategoryUnknown},
		{name: "unknown state", input: "someCustomState", want: provider.StateCategoryUnknown},
		// PR status strings
		{name: "completed (PR)", input: "completed", want: provider.StateCategoryClosedDone},
		{name: "abandoned (PR)", input: "abandoned", want: provider.StateCategoryRemoved},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azdevops.MapStateCategory(tt.input)
			if got != tt.want {
				t.Errorf("MapStateCategory(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- ItemType mapping ---

func TestMapItemType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  provider.ItemType
	}{
		{name: "Bug", input: "Bug", want: provider.ItemTypeBug},
		{name: "bug lowercase", input: "bug", want: provider.ItemTypeBug},
		{name: "Task", input: "Task", want: provider.ItemTypeTask},
		{name: "task lowercase", input: "task", want: provider.ItemTypeTask},
		{name: "User Story", input: "User Story", want: provider.ItemTypeUserStory},
		{name: "user story lowercase", input: "user story", want: provider.ItemTypeUserStory},
		{name: "Feature", input: "Feature", want: provider.ItemTypeFeature},
		{name: "feature lowercase", input: "feature", want: provider.ItemTypeFeature},
		{name: "Epic", input: "Epic", want: provider.ItemTypeEpic},
		{name: "epic lowercase", input: "epic", want: provider.ItemTypeEpic},
		{name: "Issue", input: "Issue", want: provider.ItemTypeIssue},
		{name: "issue lowercase", input: "issue", want: provider.ItemTypeIssue},
		{name: "empty", input: "", want: provider.ItemTypeUnknown},
		{name: "custom type", input: "Test Case", want: provider.ItemTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azdevops.MapItemType(tt.input)
			if got != tt.want {
				t.Errorf("MapItemType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- VoteKind mapping ---

func TestMapVoteKind(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  provider.VoteKind
	}{
		{name: "approved (10)", input: 10, want: provider.VoteKindApproved},
		{name: "approved with suggestions (5)", input: 5, want: provider.VoteKindApprovedWithSuggestions},
		{name: "no vote (0)", input: 0, want: provider.VoteKindNoVote},
		{name: "waiting for author (-5)", input: -5, want: provider.VoteKindWaitingForAuthor},
		{name: "rejected (-10)", input: -10, want: provider.VoteKindRejected},
		{name: "unknown positive", input: 99, want: provider.VoteKindNoVote},
		{name: "unknown negative", input: -99, want: provider.VoteKindNoVote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azdevops.MapVoteKind(tt.input)
			if got != tt.want {
				t.Errorf("MapVoteKind(%d) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- RunStatus mapping ---

func TestMapRunStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		result string
		want   provider.RunStatus
	}{
		{name: "inProgress running", status: "inProgress", result: "", want: provider.RunStatusRunning},
		{name: "inprogress lowercase", status: "inprogress", result: "", want: provider.RunStatusRunning},
		{name: "notStarted queued", status: "notStarted", result: "", want: provider.RunStatusQueued},
		{name: "notstarted lowercase", status: "notstarted", result: "", want: provider.RunStatusQueued},
		{name: "canceling", status: "canceling", result: "", want: provider.RunStatusCanceling},
		{name: "Canceling mixed", status: "Canceling", result: "", want: provider.RunStatusCanceling},
		{name: "completed succeeded", status: "completed", result: "succeeded", want: provider.RunStatusSucceeded},
		{name: "completed failed", status: "completed", result: "failed", want: provider.RunStatusFailed},
		{name: "completed canceled", status: "completed", result: "canceled", want: provider.RunStatusCanceled},
		{name: "completed partiallySucceeded", status: "completed", result: "partiallySucceeded", want: provider.RunStatusPartiallySucceeded},
		{name: "partiallysucceeded lowercase", status: "completed", result: "partiallysucceeded", want: provider.RunStatusPartiallySucceeded},
		{name: "empty status and result", status: "", result: "", want: provider.RunStatusUnknown},
		{name: "unknown status only", status: "postponed", result: "", want: provider.RunStatusUnknown},
		{name: "unknown both", status: "other", result: "other", want: provider.RunStatusUnknown},
		// status takes priority over result
		{name: "inprogress ignores result", status: "inprogress", result: "succeeded", want: provider.RunStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azdevops.MapRunStatus(tt.status, tt.result)
			if got != tt.want {
				t.Errorf("MapRunStatus(%q, %q) = %v, want %v", tt.status, tt.result, got, tt.want)
			}
		})
	}
}
