package notifications

import (
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// mkNotification builds a fixture row with just the fields the pane cares
// about. scopeDisplay is left equal to scope unless the caller sets it
// explicitly on the returned value, mirroring MapNotification's fallback
// (Decision 35) — tests that need the mapper's one unfixable case (an empty
// repository payload) zero it out explicitly.
func mkNotification(id, scope, title string, reason provider.NotificationReason, read bool, updatedAt time.Time) provider.Notification {
	return provider.Notification{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scope,
			ID:           id,
		},
		Title:     title,
		Reason:    reason,
		Read:      read,
		UpdatedAt: updatedAt,
	}
}

var fixedNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

// keyRune builds a plain rune key message the way listview/table expect it
// (mirrors the runeKeyMsg helper used in internal/ui/metrics/list_test.go).
func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// ─── toColumns / toRows: convention 7 (column/cell parity) ──────────────────

func TestToColumnsToRows_SingleRepo_NoRepoColumn(t *testing.T) {
	s := styles.DefaultStyles()
	items := []provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo", "PR two", provider.NotificationReasonMentioned, true, fixedNow),
	}

	cols := toColumns(items)
	rows := toRows(items, s)

	if len(cols) != 3 {
		t.Fatalf("single-repo columns = %d, want 3 (no Repo column)", len(cols))
	}
	for i, row := range rows {
		if len(row) != len(cols) {
			t.Fatalf("row %d has %d cells, want %d (== column count)", i, len(row), len(cols))
		}
	}
}

func TestToColumnsToRows_MultiRepo_RepoColumnFirst(t *testing.T) {
	s := styles.DefaultStyles()
	items := []provider.Notification{
		mkNotification("1", "owner/repo1", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo2", "PR two", provider.NotificationReasonMentioned, true, fixedNow),
	}

	cols := toColumns(items)
	rows := toRows(items, s)

	const wantCols = 4 // Repo + Reason + Title + Updated
	if len(cols) != wantCols {
		t.Fatalf("multi-repo columns = %d, want %d", len(cols), wantCols)
	}
	if cols[0].Title != "Repo" {
		t.Errorf("first column = %q, want %q", cols[0].Title, "Repo")
	}
	for i, row := range rows {
		if len(row) != len(cols) {
			t.Fatalf("row %d has %d cells, want %d (== column count)", i, len(row), len(cols))
		}
	}
	if !strings.Contains(rows[0][0], "owner/repo1") {
		t.Errorf("row 0 repo cell = %q, want to contain %q", rows[0][0], "owner/repo1")
	}
}

// ─── Rendering: convention 8 (render through View() after WindowSizeMsg) ────

func TestView_RendersAfterWindowSizeMsg_SingleRepo(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles())
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View() panicked: %v", r)
		}
	}()
	_ = m.View()
}

func TestView_RendersAfterWindowSizeMsg_MultiRepo(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles())
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo1", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo2", "PR two", provider.NotificationReasonMentioned, true, fixedNow),
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View() panicked: %v", r)
		}
	}()
	_ = m.View()
}

// ─── Unread emphasis: convention 6 (named style, not substring) ─────────────

func TestToRows_UnreadRow_UsesNamedTitleStyle(t *testing.T) {
	s := styles.DefaultStyles()
	items := []provider.Notification{
		mkNotification("1", "owner/repo", "Unread title", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo", "Read title", provider.NotificationReasonReviewRequested, true, fixedNow),
	}

	rows := toRows(items, s)

	// Assert against the exact named style's Render output (styles.Styles.Title),
	// not a substring of the rendered text — this is the convention 6 shape
	// (mirrors pullrequests_test.go's KindStyle().GetForeground() assertion).
	// Note: lipgloss strips ANSI codes when no TTY/color profile is attached
	// (as in this test binary), so this equality check alone cannot detect a
	// missing style at the byte level; titleCell's read/unread branch is
	// exercised directly below and its named style is asserted structurally
	// via style-property checks, not string diffing.
	if got, want := rows[0][1], titleCell(items[0], s); got != want {
		t.Errorf("unread title cell = %q, want %q (titleCell/styles.Styles.Title output)", got, want)
	}
	if got, want := rows[1][1], items[1].Title; got != want {
		t.Errorf("read title cell = %q, want plain %q (no style applied)", got, want)
	}
}

// TestTitleCell_ReadVsUnread_StructurallyDifferentPaths pins titleCell's two
// branches directly: unread rows go through the exact named style
// (styles.Styles.Title, a real style with Bold set — not a zero-value/no-op
// style masquerading as one), read rows are the notification's Title
// verbatim with no wrapping call at all.
func TestTitleCell_ReadVsUnread_StructurallyDifferentPaths(t *testing.T) {
	s := styles.DefaultStyles()

	if !s.Title.GetBold() {
		t.Fatal("styles.Styles.Title must be Bold to carry unread emphasis — if this fails, Title's definition changed underneath this pane")
	}

	unread := mkNotification("1", "owner/repo", "Unread title", provider.NotificationReasonOther, false, fixedNow)
	if got, want := titleCell(unread, s), s.Title.Render(unread.Title); got != want {
		t.Errorf("titleCell(unread) = %q, want %q (styles.Styles.Title.Render(Title))", got, want)
	}

	read := mkNotification("2", "owner/repo", "Read title", provider.NotificationReasonOther, true, fixedNow)
	if got, want := titleCell(read, s), read.Title; got != want {
		t.Errorf("titleCell(read) = %q, want plain %q", got, want)
	}
}

// ─── Zero UpdatedAt / empty ScopeDisplay render "—" ──────────────────────────

func TestToRows_ZeroUpdatedAt_RendersDash(t *testing.T) {
	s := styles.DefaultStyles()
	items := []provider.Notification{
		mkNotification("1", "owner/repo", "No timestamp", provider.NotificationReasonOther, false, time.Time{}),
	}

	rows := toRows(items, s)

	const updatedCol = 2 // single-repo layout: [Reason][Title][Updated]
	if rows[0][updatedCol] != "—" {
		t.Errorf("zero UpdatedAt cell = %q, want %q", rows[0][updatedCol], "—")
	}
	if strings.Contains(rows[0][updatedCol], "0001") {
		t.Errorf("zero UpdatedAt cell = %q, must never render a year-0001 date", rows[0][updatedCol])
	}
}

func TestToRows_EmptyScopeDisplay_RendersDash(t *testing.T) {
	s := styles.DefaultStyles()
	// A thread whose repository payload is absent: both Scope and
	// ScopeDisplay are empty on that one row, but a second row with a
	// populated Scope keeps the feed multi-repo so the Repo column renders.
	absent := mkNotification("1", "", "Absent repo payload", provider.NotificationReasonOther, false, fixedNow)
	absent.Identity.ScopeDisplay = ""
	items := []provider.Notification{
		absent,
		mkNotification("2", "owner/repo", "Normal", provider.NotificationReasonOther, false, fixedNow),
	}

	rows := toRows(items, s)

	if rows[0][0] != "—" {
		t.Errorf("empty ScopeDisplay cell = %q, want %q", rows[0][0], "—")
	}
}

// ─── f reason filter: cycle mechanics (decision 53) ──────────────────────────

func TestPresentReasons_OnlyReasonsInFeed_NotFullEnum(t *testing.T) {
	feed := []provider.Notification{
		mkNotification("1", "owner/repo", "A", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "B", provider.NotificationReasonSubscribed, false, fixedNow),
	}

	present := presentReasons(feed)

	if len(present) != 2 {
		t.Fatalf("presentReasons = %v, want exactly 2 entries (not the full 11-value enum)", present)
	}
	if present[0] != provider.NotificationReasonMentioned || present[1] != provider.NotificationReasonSubscribed {
		t.Errorf("presentReasons = %v, want [Mentioned, Subscribed] in enum order", present)
	}
}

func TestPresentReasons_EnumOrder_NotFeedOrder(t *testing.T) {
	// Subscribed is late in the enum, Mentioned is early. The feed's newest
	// (first) row carries the late-enum reason — presentReasons must still
	// report Mentioned before Subscribed.
	feed := []provider.Notification{
		mkNotification("1", "owner/repo", "Newest, late-enum reason", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("2", "owner/repo", "Older, early-enum reason", provider.NotificationReasonMentioned, false, fixedNow.Add(-time.Hour)),
	}

	present := presentReasons(feed)

	if len(present) != 2 || present[0] != provider.NotificationReasonMentioned || present[1] != provider.NotificationReasonSubscribed {
		t.Fatalf("presentReasons = %v, want [Mentioned, Subscribed] (enum order, ignoring feed order)", present)
	}
}

func TestPresentReasons_NeverIncludesUnknown(t *testing.T) {
	feed := []provider.Notification{
		mkNotification("1", "owner/repo", "Corrupt/probe row", provider.NotificationReasonUnknown, false, fixedNow),
		mkNotification("2", "owner/repo", "Normal row", provider.NotificationReasonOther, false, fixedNow),
	}

	present := presentReasons(feed)

	for _, r := range present {
		if r == provider.NotificationReasonUnknown {
			t.Fatalf("presentReasons = %v, must never include NotificationReasonUnknown (decision 53)", present)
		}
	}
	if len(present) != 1 || present[0] != provider.NotificationReasonOther {
		t.Fatalf("presentReasons = %v, want [Other] only", present)
	}
}

func TestCycleReasonFilter_EnumOrder_AllPositionReachable(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles())
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Feed order deliberately reversed vs. enum order, and a row carrying
	// Unknown thrown in, to prove the cycle ignores both.
	feed := []provider.Notification{
		mkNotification("1", "owner/repo", "Subscribed row", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("2", "owner/repo", "Unknown row", provider.NotificationReasonUnknown, false, fixedNow),
		mkNotification("3", "owner/repo", "Mentioned row", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m = m.SetFeed(feed)

	if m.reasonFilterActive {
		t.Fatalf("initial state must be the 'all' position, got active=%v reason=%v", m.reasonFilterActive, m.reasonFilter)
	}

	m, _ = m.Update(keyRune('f'))
	if !m.reasonFilterActive || m.reasonFilter != provider.NotificationReasonMentioned {
		t.Fatalf("after 1st f: active=%v reason=%v, want Mentioned (enum order, before Subscribed)", m.reasonFilterActive, m.reasonFilter)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("after filtering to Mentioned, items = %d, want 1", got)
	}

	m, _ = m.Update(keyRune('f'))
	if !m.reasonFilterActive || m.reasonFilter != provider.NotificationReasonSubscribed {
		t.Fatalf("after 2nd f: active=%v reason=%v, want Subscribed", m.reasonFilterActive, m.reasonFilter)
	}

	m, _ = m.Update(keyRune('f'))
	if m.reasonFilterActive {
		t.Fatalf("after 3rd f (cycling past the last present reason): active=%v reason=%v, want the 'all' position", m.reasonFilterActive, m.reasonFilter)
	}
	if got := len(m.list.Items()); got != 3 {
		t.Errorf("after returning to 'all', items = %d, want 3 (full feed restored)", got)
	}

	// Never once must the cycle have stopped on Unknown.
	// (Covered structurally above since Mentioned/Subscribed/all are the
	// only three positions visited, but restated explicitly per the task's
	// criterion.)
	if m.reasonFilter == provider.NotificationReasonUnknown && m.reasonFilterActive {
		t.Fatal("cycle must never stop on NotificationReasonUnknown")
	}
}

// ─── f filter collapse: dynamic column shrink + cursor survival ─────────────
//
// This is the direction that matters most (the task's own words): the expand
// direction passing does not prove the shrink direction. Start multi-repo
// with the cursor on a non-first row, cycle until the feed narrows to a
// single repo, and assert the column collapses, View() does not panic, and
// the cursor/selection survives — clamped in range and, in this fixture,
// still on the same item.
func TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles())
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Row 0: a/repo1, Mentioned
	// Row 1: a/repo1, ReviewRequested  <- cursor starts here (non-first row)
	// Row 2: b/repo2, Mentioned
	//
	// Filtering to ReviewRequested (the first present reason in enum order)
	// leaves only row 1 — a single repo, so the Repo column must disappear —
	// and the cursor's own item (id "2") is the one that survives.
	feed := []provider.Notification{
		mkNotification("1", "a/repo1", "Mentioned in repo1", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "a/repo1", "Review requested in repo1", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("3", "b/repo2", "Mentioned in repo2", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m = m.SetFeed(feed)

	// Sanity: starts multi-repo (Repo column present).
	if cols := toColumns(m.list.Items()); len(cols) != 4 {
		t.Fatalf("pre-filter columns = %d, want 4 (multi-repo)", len(cols))
	}

	m.list.SetCursor(1) // non-first row: id "2"
	if got := m.list.SelectedIndex(); got != 1 {
		t.Fatalf("cursor after SetCursor(1) = %d, want 1", got)
	}

	m, _ = m.Update(keyRune('f'))
	if !m.reasonFilterActive || m.reasonFilter != provider.NotificationReasonReviewRequested {
		t.Fatalf("after 1st f: active=%v reason=%v, want ReviewRequested", m.reasonFilterActive, m.reasonFilter)
	}

	visible := m.list.Items()
	if len(visible) != 1 {
		t.Fatalf("post-filter items = %d, want 1", len(visible))
	}
	if visible[0].Identity.ID != "2" {
		t.Fatalf("post-filter item ID = %q, want %q (survived the filter)", visible[0].Identity.ID, "2")
	}

	// (a) the repo column is gone
	cols := toColumns(visible)
	if len(cols) != 3 {
		t.Fatalf("post-filter columns = %d, want 3 (Repo column gone)", len(cols))
	}
	for _, c := range cols {
		if c.Title == "Repo" {
			t.Fatal("Repo column must not survive the collapse")
		}
	}

	// (b) View() does not panic across the column-count change
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("View() panicked after collapse: %v", r)
			}
		}()
		_ = m.View()
	}()

	// (c) the cursor is still on a valid row — clamped in range — and in
	// this fixture still on the same item (id "2") that was under it before
	// the filter narrowed the feed. bubbles' table.SetRows does not
	// necessarily clamp a cursor that is now past the end, so this must be
	// asserted explicitly rather than assumed from SetItems succeeding.
	idx := m.list.SelectedIndex()
	if idx < 0 || idx >= len(m.list.Items()) {
		t.Fatalf("cursor after collapse = %d, out of range for %d items", idx, len(m.list.Items()))
	}
	if m.list.Items()[idx].Identity.ID != "2" {
		t.Fatalf("cursor after collapse points at ID %q, want %q (same item survived)", m.list.Items()[idx].Identity.ID, "2")
	}
}

// TestCycleReasonFilter_Collapse_CursorClamped_ItemDropped covers the case
// where the cursor's own row does NOT survive the filter (it must still
// clamp into range rather than leaving a stale/out-of-bounds cursor).
func TestCycleReasonFilter_Collapse_CursorClamped_ItemDropped(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles())
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	feed := []provider.Notification{
		mkNotification("1", "a/repo1", "Review requested", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "b/repo2", "Mentioned (cursor here, dropped by filter)", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "a/repo1", "Mentioned too", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m = m.SetFeed(feed)

	m.list.SetCursor(1) // id "2", which ReviewRequested filtering will drop entirely

	m, _ = m.Update(keyRune('f')) // -> ReviewRequested (first in enum order)

	visible := m.list.Items()
	if len(visible) != 1 || visible[0].Identity.ID != "1" {
		t.Fatalf("post-filter items = %v, want just ID 1", visible)
	}

	idx := m.list.SelectedIndex()
	if idx < 0 || idx >= len(visible) {
		t.Fatalf("cursor after collapse = %d, out of range for %d items", idx, len(visible))
	}
}

// ─── listview.ViewMode sanity: `f` is a no-op while a detail view is open ───

func TestUpdate_FKey_NoopOutsideListMode(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles())
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A", provider.NotificationReasonMentioned, false, fixedNow),
	})

	if m.list.GetViewMode() != listview.ViewList {
		t.Fatalf("precondition: expected ViewList, got %v", m.list.GetViewMode())
	}
}
