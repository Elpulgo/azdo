// Package display maps neutral provider enums to glyphs, labels, and lipgloss
// styles. All theming stays here; view code passes enum values and receives
// ready-to-render strings.
package display

import (
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

// ─── StateCategory ───────────────────────────────────────────────────────────

// StateGlyph returns the icon used for a work-item state category.
func StateGlyph(cat provider.StateCategory) string {
	switch cat {
	case provider.StateCategoryNew:
		return "○"
	case provider.StateCategoryActive:
		return "◐"
	case provider.StateCategoryResolved, provider.StateCategoryReadyForTest:
		return "●"
	case provider.StateCategoryClosedDone:
		return "✓"
	case provider.StateCategoryRemoved:
		return "✗"
	default: // StateCategoryUnknown
		return "○"
	}
}

// StateLabel returns the display label for a work-item state category.
// StateLabel returns the display label for a work-item state category.
// For StateCategoryUnknown and StateCategoryReadyForTest the raw state string
// is used by the caller; this returns "" as a signal that the raw value should
// be used instead.
func StateLabel(cat provider.StateCategory) string {
	switch cat {
	case provider.StateCategoryNew:
		return "New"
	case provider.StateCategoryActive:
		return "Active"
	case provider.StateCategoryResolved:
		return "Resolved"
	case provider.StateCategoryReadyForTest:
		// The original view renders the raw state string for ready-variants.
		// Return "" so callers fall back to the raw string.
		return ""
	case provider.StateCategoryClosedDone:
		return "Closed"
	case provider.StateCategoryRemoved:
		return "Removed"
	default: // StateCategoryUnknown
		return ""
	}
}

// StateStyle returns the lipgloss style for a work-item state category.
// Mirrors stateTextWithStyles in internal/ui/workitems/list.go.
func StateStyle(cat provider.StateCategory, s *styles.Styles) lipgloss.Style {
	switch cat {
	case provider.StateCategoryNew:
		return s.Muted
	case provider.StateCategoryActive:
		return s.Info
	case provider.StateCategoryResolved:
		return s.Warning
	case provider.StateCategoryReadyForTest:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(s.Theme.Secondary))
	case provider.StateCategoryClosedDone:
		return s.Success
	case provider.StateCategoryRemoved:
		return s.Error
	default: // StateCategoryUnknown
		return s.Muted
	}
}

// ─── ItemType ────────────────────────────────────────────────────────────────

// ItemTypeLabel returns the short display label for a work-item type.
// Mirrors typeIconWithStyles in internal/ui/workitems/list.go.
func ItemTypeLabel(t provider.ItemType) string {
	switch t {
	case provider.ItemTypeBug:
		return "Bug"
	case provider.ItemTypeTask:
		return "Task"
	case provider.ItemTypeUserStory:
		return "Story"
	case provider.ItemTypeFeature:
		return "Feature"
	case provider.ItemTypeEpic:
		return "Epic"
	case provider.ItemTypeIssue:
		return "Issue"
	default: // ItemTypeUnknown
		return "Item"
	}
}

// ItemTypeStyle returns the lipgloss style for a work-item type label.
// Mirrors typeIconWithStyles in internal/ui/workitems/list.go.
func ItemTypeStyle(t provider.ItemType, s *styles.Styles) lipgloss.Style {
	switch t {
	case provider.ItemTypeBug:
		return s.Error
	case provider.ItemTypeTask:
		return s.Info
	case provider.ItemTypeUserStory:
		return s.Success
	case provider.ItemTypeFeature:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(s.Theme.Accent))
	case provider.ItemTypeEpic:
		return s.Warning
	case provider.ItemTypeIssue:
		return s.Error
	default: // ItemTypeUnknown
		return s.Muted
	}
}

// ─── Kind ────────────────────────────────────────────────────────────────────

// KindGlyph returns the provider-origin icon for a backend Kind.
// Used when a list mixes entities from different backends and a per-row marker
// is needed. Returns "?" for the zero/unknown Kind and any unrecognised value.
func KindGlyph(k provider.Kind) string {
	switch k {
	case provider.KindAzure:
		return "⬡"
	case provider.KindGitHub:
		return "⑂"
	default: // KindUnknown (zero) and future/unrecognised values
		return "?"
	}
}

// MixedKinds reports whether the given kinds span more than one distinct
// provider Kind. Returns false for an empty slice and for a slice where all
// elements share the same Kind.
func MixedKinds(kinds []provider.Kind) bool {
	if len(kinds) == 0 {
		return false
	}
	first := kinds[0]
	for _, k := range kinds[1:] {
		if k != first {
			return true
		}
	}
	return false
}

// KindStyle returns the lipgloss style for a provider-kind glyph cell.
// All kinds — including KindAzure and KindGitHub — use a muted/neutral style
// so the glyph reads as secondary metadata rather than a status indicator.
func KindStyle(k provider.Kind, s *styles.Styles) lipgloss.Style {
	switch k {
	case provider.KindAzure:
		return s.Muted
	case provider.KindGitHub:
		return s.Muted
	default: // KindUnknown (zero) and future/unrecognised values
		return s.Muted
	}
}

// KindLabel returns the human-readable provider name for a backend Kind.
// Returns "" for the zero/unknown Kind so callers can apply their own fallback.
func KindLabel(k provider.Kind) string {
	switch k {
	case provider.KindAzure:
		return "Azure"
	case provider.KindGitHub:
		return "GitHub"
	default: // KindUnknown (zero) and future/unrecognised values
		return ""
	}
}

// ─── VoteKind ────────────────────────────────────────────────────────────────

// VoteGlyph returns the icon for a reviewer vote kind.
// Mirrors reviewerVoteIconWithStyles in internal/ui/pullrequests/detail.go and
// voteIconWithStyles in internal/ui/pullrequests/list.go.
func VoteGlyph(v provider.VoteKind) string {
	switch v {
	case provider.VoteKindApproved:
		return "✓"
	case provider.VoteKindApprovedWithSuggestions:
		return "~"
	case provider.VoteKindNoVote:
		return "○"
	case provider.VoteKindWaitingForAuthor:
		return "◐"
	case provider.VoteKindRejected:
		return "✗"
	default:
		return "?"
	}
}

// VoteLabel returns the human-readable description of a reviewer vote kind.
// Mirrors reviewerVoteDescription in internal/ui/pullrequests/detail.go.
func VoteLabel(v provider.VoteKind) string {
	switch v {
	case provider.VoteKindApproved:
		return "Approved"
	case provider.VoteKindApprovedWithSuggestions:
		return "Approved with suggestions"
	case provider.VoteKindNoVote:
		return "No vote"
	case provider.VoteKindWaitingForAuthor:
		return "Waiting for author"
	case provider.VoteKindRejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// VoteStyle returns the lipgloss style for a reviewer vote kind.
// Mirrors reviewerVoteIconWithStyles in internal/ui/pullrequests/detail.go.
func VoteStyle(v provider.VoteKind, s *styles.Styles) lipgloss.Style {
	switch v {
	case provider.VoteKindApproved:
		return s.Success
	case provider.VoteKindApprovedWithSuggestions:
		return s.Warning
	case provider.VoteKindNoVote:
		return s.Muted
	case provider.VoteKindWaitingForAuthor:
		return s.Warning
	case provider.VoteKindRejected:
		return s.Error
	default:
		return s.Muted
	}
}

// ─── RunStatus ───────────────────────────────────────────────────────────────

// RunStatusGlyph returns the icon for a pipeline run status.
// Mirrors statusIconWithStyles in internal/ui/pipelines/list.go and
// recordIconWithStyles in internal/ui/pipelines/detail.go.
//
// RunStatusUnknown returns "○" to reproduce the detail view's default/unknown
// case (detail.go:586-607 renders Muted "○" for skipped/abandoned/unrecognised
// values). The adapter maps skipped/abandoned/unrecognised wire values to
// RunStatusUnknown, so returning "○" here preserves the existing behavior.
func RunStatusGlyph(r provider.RunStatus) string {
	switch r {
	case provider.RunStatusRunning:
		return "●"
	case provider.RunStatusQueued:
		return "○"
	case provider.RunStatusCanceling:
		return "⊘"
	case provider.RunStatusSucceeded:
		return "✓"
	case provider.RunStatusFailed:
		return "✗"
	case provider.RunStatusCanceled:
		return "○"
	case provider.RunStatusPartiallySucceeded:
		return "◐"
	case provider.RunStatusPending:
		return "○"
	case provider.RunStatusSucceededWithIssues:
		return "◐"
	default: // RunStatusUnknown — skipped/abandoned/unrecognised → Muted "○"
		return "○"
	}
}

// RunStatusLabel returns the short display label for a pipeline run status.
// Mirrors statusIconWithStyles in internal/ui/pipelines/list.go.
//
// RunStatusPending and RunStatusSucceededWithIssues are detail-view-only
// statuses; they return "" here because the list view has no label for them.
func RunStatusLabel(r provider.RunStatus) string {
	switch r {
	case provider.RunStatusRunning:
		return "Running"
	case provider.RunStatusQueued:
		return "Queued"
	case provider.RunStatusCanceling:
		return "Cancel"
	case provider.RunStatusSucceeded:
		return "Success"
	case provider.RunStatusFailed:
		return "Failed"
	case provider.RunStatusCanceled:
		return "Cancel"
	case provider.RunStatusPartiallySucceeded:
		return "Partial"
	case provider.RunStatusPending:
		return ""
	case provider.RunStatusSucceededWithIssues:
		return ""
	default: // RunStatusUnknown
		return ""
	}
}

// RunStatusStyle returns the lipgloss style for a pipeline run status.
// Mirrors statusIconWithStyles in internal/ui/pipelines/list.go and
// recordIconWithStyles in internal/ui/pipelines/detail.go.
func RunStatusStyle(r provider.RunStatus, s *styles.Styles) lipgloss.Style {
	switch r {
	case provider.RunStatusRunning:
		return s.Info
	case provider.RunStatusQueued:
		return s.Info
	case provider.RunStatusCanceling:
		return s.Warning
	case provider.RunStatusSucceeded:
		return s.Success
	case provider.RunStatusFailed:
		return s.Error
	case provider.RunStatusCanceled:
		return s.Muted
	case provider.RunStatusPartiallySucceeded:
		return s.Warning
	case provider.RunStatusPending:
		return s.Muted
	case provider.RunStatusSucceededWithIssues:
		return s.Warning
	default: // RunStatusUnknown
		return s.Muted
	}
}
