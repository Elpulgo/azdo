package display_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
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
		// RunStatusUnknown → "○" matching the detail view's default/unknown case
		{"Unknown", provider.RunStatusUnknown, "○"},
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

// ─── Kind ────────────────────────────────────────────────────────────────────

func TestKindGlyph(t *testing.T) {
	tests := []struct {
		name     string
		kind     provider.Kind
		expected string
	}{
		// Zero/unknown Kind returns a neutral fallback; no constant exists for it yet.
		{"Zero", provider.Kind(0), "?"},
		{"Azure", provider.KindAzure, "⬡"},
		{"GitHub", provider.KindGitHub, "⑂"},
		// Sentinel: out-of-range value falls through to default
		{"OutOfRange", provider.Kind(99), "?"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.KindGlyph(tc.kind)
			if got != tc.expected {
				t.Errorf("KindGlyph(%v) = %q, want %q", tc.kind, got, tc.expected)
			}
		})
	}
}

func TestKindLabel(t *testing.T) {
	tests := []struct {
		name     string
		kind     provider.Kind
		expected string
	}{
		// Zero/unknown Kind returns "" (caller decides how to handle unknown origin).
		{"Zero", provider.Kind(0), ""},
		{"Azure", provider.KindAzure, "Azure"},
		{"GitHub", provider.KindGitHub, "GitHub"},
		// Sentinel: out-of-range value falls through to default
		{"OutOfRange", provider.Kind(99), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.KindLabel(tc.kind)
			if got != tc.expected {
				t.Errorf("KindLabel(%v) = %q, want %q", tc.kind, got, tc.expected)
			}
		})
	}
}

func TestKindStyle(t *testing.T) {
	s := styles.DefaultStyles()
	th := s.Theme
	tests := []struct {
		name   string
		kind   provider.Kind
		wantFg lipgloss.Color
	}{
		// All kinds (including zero/unknown and KindAzure) use Muted so the glyph
		// reads as secondary metadata rather than a status indicator.
		{"Zero", provider.Kind(0), th.ForegroundMuted},
		{"Azure", provider.KindAzure, th.ForegroundMuted},
		{"GitHub", provider.KindGitHub, th.ForegroundMuted},
		// Sentinel: out-of-range value also returns Muted
		{"OutOfRange", provider.Kind(99), th.ForegroundMuted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.KindStyle(tc.kind, s).GetForeground()
			if got != tc.wantFg {
				t.Errorf("KindStyle(%v) foreground = %v, want %v", tc.kind, got, tc.wantFg)
			}
		})
	}
}

// ─── MixedKinds ──────────────────────────────────────────────────────────────

func TestMixedKinds(t *testing.T) {
	tests := []struct {
		name     string
		kinds    []provider.Kind
		expected bool
	}{
		// empty → false
		{"Empty", []provider.Kind{}, false},
		// single element, single kind → false
		{"SingleKind_OneElement", []provider.Kind{provider.KindAzure}, false},
		// multiple elements, all same kind → false
		{"SingleKind_MultipleElements", []provider.Kind{provider.KindAzure, provider.KindAzure, provider.KindAzure}, false},
		// two distinct kinds → true
		{"TwoDistinctKinds", []provider.Kind{provider.KindAzure, provider.Kind(2)}, true},
		// more than two distinct kinds → true
		{"ThreeDistinctKinds", []provider.Kind{provider.KindAzure, provider.Kind(2), provider.Kind(3)}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.MixedKinds(tc.kinds)
			if got != tc.expected {
				t.Errorf("MixedKinds(%v) = %v, want %v", tc.kinds, got, tc.expected)
			}
		})
	}
}

// ─── Style function tests ─────────────────────────────────────────────────────

func TestStateStyle(t *testing.T) {
	s := styles.DefaultStyles()
	th := s.Theme
	tests := []struct {
		name   string
		cat    provider.StateCategory
		wantFg lipgloss.Color
	}{
		{"Unknown", provider.StateCategoryUnknown, th.ForegroundMuted},
		{"New", provider.StateCategoryNew, th.ForegroundMuted},
		{"Active", provider.StateCategoryActive, th.Info},
		{"Resolved", provider.StateCategoryResolved, th.Warning},
		{"ReadyForTest", provider.StateCategoryReadyForTest, th.Secondary},
		{"ClosedDone", provider.StateCategoryClosedDone, th.Success},
		{"Removed", provider.StateCategoryRemoved, th.Error},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.StateStyle(tc.cat, s).GetForeground()
			if got != tc.wantFg {
				t.Errorf("StateStyle(%v) foreground = %v, want %v", tc.cat, got, tc.wantFg)
			}
		})
	}
}

func TestItemTypeStyle(t *testing.T) {
	s := styles.DefaultStyles()
	th := s.Theme
	tests := []struct {
		name     string
		itemType provider.ItemType
		wantFg   lipgloss.Color
	}{
		{"Unknown", provider.ItemTypeUnknown, th.ForegroundMuted},
		{"Bug", provider.ItemTypeBug, th.Error},
		{"Task", provider.ItemTypeTask, th.Info},
		{"UserStory", provider.ItemTypeUserStory, th.Success},
		{"Feature", provider.ItemTypeFeature, th.Accent},
		{"Epic", provider.ItemTypeEpic, th.Warning},
		{"Issue", provider.ItemTypeIssue, th.Error},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.ItemTypeStyle(tc.itemType, s).GetForeground()
			if got != tc.wantFg {
				t.Errorf("ItemTypeStyle(%v) foreground = %v, want %v", tc.itemType, got, tc.wantFg)
			}
		})
	}
}

func TestVoteStyle(t *testing.T) {
	s := styles.DefaultStyles()
	th := s.Theme
	tests := []struct {
		name   string
		vote   provider.VoteKind
		wantFg lipgloss.Color
	}{
		{"NoVote", provider.VoteKindNoVote, th.ForegroundMuted},
		{"Approved", provider.VoteKindApproved, th.Success},
		{"ApprovedWithSuggestions", provider.VoteKindApprovedWithSuggestions, th.Warning},
		{"WaitingForAuthor", provider.VoteKindWaitingForAuthor, th.Warning},
		{"Rejected", provider.VoteKindRejected, th.Error},
		{"OutOfRange", provider.VoteKind(99), th.ForegroundMuted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.VoteStyle(tc.vote, s).GetForeground()
			if got != tc.wantFg {
				t.Errorf("VoteStyle(%v) foreground = %v, want %v", tc.vote, got, tc.wantFg)
			}
		})
	}
}

func TestRunStatusStyle(t *testing.T) {
	s := styles.DefaultStyles()
	th := s.Theme
	tests := []struct {
		name   string
		status provider.RunStatus
		wantFg lipgloss.Color
	}{
		{"Unknown", provider.RunStatusUnknown, th.ForegroundMuted},
		{"Running", provider.RunStatusRunning, th.Info},
		{"Queued", provider.RunStatusQueued, th.Info},
		{"Canceling", provider.RunStatusCanceling, th.Warning},
		{"Succeeded", provider.RunStatusSucceeded, th.Success},
		{"Failed", provider.RunStatusFailed, th.Error},
		{"Canceled", provider.RunStatusCanceled, th.ForegroundMuted},
		{"PartiallySucceeded", provider.RunStatusPartiallySucceeded, th.Warning},
		{"Pending", provider.RunStatusPending, th.ForegroundMuted},
		{"SucceededWithIssues", provider.RunStatusSucceededWithIssues, th.Warning},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := display.RunStatusStyle(tc.status, s).GetForeground()
			if got != tc.wantFg {
				t.Errorf("RunStatusStyle(%v) foreground = %v, want %v", tc.status, got, tc.wantFg)
			}
		})
	}
}
