package display_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/display"
)

// ─── StateCategory ───────────────────────────────────────────────────────────

func TestStateGlyph(t *testing.T) {
	tests := []struct {
		name     string
		cat      provider.StateCategory
		expected string
	}{
		{"Unknown", provider.StateCategoryUnknown, "○"},
		{"New", provider.StateCategoryNew, "○"},
		{"Active", provider.StateCategoryActive, "◐"},
		{"Resolved", provider.StateCategoryResolved, "●"},
		{"ReadyForTest", provider.StateCategoryReadyForTest, "●"},
		{"ClosedDone", provider.StateCategoryClosedDone, "✓"},
		{"Removed", provider.StateCategoryRemoved, "✗"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.StateGlyph(tc.cat)
			if got != tc.expected {
				t.Errorf("StateGlyph(%v) = %q, want %q", tc.cat, got, tc.expected)
			}
		})
	}
}

func TestStateLabel(t *testing.T) {
	tests := []struct {
		name     string
		cat      provider.StateCategory
		expected string
	}{
		// Unknown and ReadyForTest return "" (caller uses raw state string)
		{"Unknown", provider.StateCategoryUnknown, ""},
		{"New", provider.StateCategoryNew, "New"},
		{"Active", provider.StateCategoryActive, "Active"},
		{"Resolved", provider.StateCategoryResolved, "Resolved"},
		{"ReadyForTest", provider.StateCategoryReadyForTest, ""},
		{"ClosedDone", provider.StateCategoryClosedDone, "Closed"},
		{"Removed", provider.StateCategoryRemoved, "Removed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.StateLabel(tc.cat)
			if got != tc.expected {
				t.Errorf("StateLabel(%v) = %q, want %q", tc.cat, got, tc.expected)
			}
		})
	}
}

// ─── ItemType ────────────────────────────────────────────────────────────────

func TestItemTypeLabel(t *testing.T) {
	tests := []struct {
		name     string
		itemType provider.ItemType
		expected string
	}{
		{"Unknown", provider.ItemTypeUnknown, "Item"},
		{"Bug", provider.ItemTypeBug, "Bug"},
		{"Task", provider.ItemTypeTask, "Task"},
		{"UserStory", provider.ItemTypeUserStory, "Story"},
		{"Feature", provider.ItemTypeFeature, "Feature"},
		{"Epic", provider.ItemTypeEpic, "Epic"},
		{"Issue", provider.ItemTypeIssue, "Issue"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.ItemTypeLabel(tc.itemType)
			if got != tc.expected {
				t.Errorf("ItemTypeLabel(%v) = %q, want %q", tc.itemType, got, tc.expected)
			}
		})
	}
}

// ─── VoteKind ────────────────────────────────────────────────────────────────

func TestVoteGlyph(t *testing.T) {
	tests := []struct {
		name     string
		vote     provider.VoteKind
		expected string
	}{
		{"NoVote", provider.VoteKindNoVote, "○"},
		{"Approved", provider.VoteKindApproved, "✓"},
		{"ApprovedWithSuggestions", provider.VoteKindApprovedWithSuggestions, "~"},
		{"WaitingForAuthor", provider.VoteKindWaitingForAuthor, "◐"},
		{"Rejected", provider.VoteKindRejected, "✗"},
		// Sentinel: out-of-range value falls through to default
		{"OutOfRange", provider.VoteKind(99), "?"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.VoteGlyph(tc.vote)
			if got != tc.expected {
				t.Errorf("VoteGlyph(%v) = %q, want %q", tc.vote, got, tc.expected)
			}
		})
	}
}

func TestVoteLabel(t *testing.T) {
	tests := []struct {
		name     string
		vote     provider.VoteKind
		expected string
	}{
		{"NoVote", provider.VoteKindNoVote, "No vote"},
		{"Approved", provider.VoteKindApproved, "Approved"},
		{"ApprovedWithSuggestions", provider.VoteKindApprovedWithSuggestions, "Approved with suggestions"},
		{"WaitingForAuthor", provider.VoteKindWaitingForAuthor, "Waiting for author"},
		{"Rejected", provider.VoteKindRejected, "Rejected"},
		{"OutOfRange", provider.VoteKind(99), "Unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.VoteLabel(tc.vote)
			if got != tc.expected {
				t.Errorf("VoteLabel(%v) = %q, want %q", tc.vote, got, tc.expected)
			}
		})
	}
}

// ─── RunStatus ───────────────────────────────────────────────────────────────

func TestRunStatusGlyph(t *testing.T) {
	tests := []struct {
		name     string
		status   provider.RunStatus
		expected string
	}{
		// RunStatusUnknown → safe fallback "?" (raw "status/result" not representable)
		{"Unknown", provider.RunStatusUnknown, "?"},
		{"Running", provider.RunStatusRunning, "●"},
		{"Queued", provider.RunStatusQueued, "○"},
		{"Canceling", provider.RunStatusCanceling, "⊘"},
		{"Succeeded", provider.RunStatusSucceeded, "✓"},
		{"Failed", provider.RunStatusFailed, "✗"},
		{"Canceled", provider.RunStatusCanceled, "○"},
		{"PartiallySucceeded", provider.RunStatusPartiallySucceeded, "◐"},
		// Detail-view-only statuses
		{"Pending", provider.RunStatusPending, "○"},
		{"SucceededWithIssues", provider.RunStatusSucceededWithIssues, "◐"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.RunStatusGlyph(tc.status)
			if got != tc.expected {
				t.Errorf("RunStatusGlyph(%v) = %q, want %q", tc.status, got, tc.expected)
			}
		})
	}
}

func TestRunStatusLabel(t *testing.T) {
	tests := []struct {
		name     string
		status   provider.RunStatus
		expected string
	}{
		{"Unknown", provider.RunStatusUnknown, ""},
		{"Running", provider.RunStatusRunning, "Running"},
		{"Queued", provider.RunStatusQueued, "Queued"},
		{"Canceling", provider.RunStatusCanceling, "Cancel"},
		{"Succeeded", provider.RunStatusSucceeded, "Success"},
		{"Failed", provider.RunStatusFailed, "Failed"},
		{"Canceled", provider.RunStatusCanceled, "Cancel"},
		{"PartiallySucceeded", provider.RunStatusPartiallySucceeded, "Partial"},
		// Detail-view-only statuses have no list label
		{"Pending", provider.RunStatusPending, ""},
		{"SucceededWithIssues", provider.RunStatusSucceededWithIssues, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.RunStatusLabel(tc.status)
			if got != tc.expected {
				t.Errorf("RunStatusLabel(%v) = %q, want %q", tc.status, got, tc.expected)
			}
		})
	}
}
