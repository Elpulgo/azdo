package notifications

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

// forceTrueColor makes lipgloss actually emit ANSI escapes for the duration of
// the calling test, restoring the previous profile afterwards.
//
// Without it lipgloss resolves the Ascii profile in a test binary, so Render is
// the identity function and every styled-vs-unstyled comparison collapses into
// a tautology (decision 56). Style-object assertions are the primary form and
// need no profile; this is for the few places where only bytes are reachable —
// that titleCell applies its style at all, and that the "—" fallback is
// substituted *before* styling rather than after. Mirrors
// internal/ui/components/table/table_test.go:97.
func forceTrueColor(t *testing.T) {
	t.Helper()
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(restore) })
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

	// The specs must be normalised: unnormalised they sum to 120
	// (Repo 20 + Reason 20 + Title 60 + Updated 20), which over-widens the
	// table past the terminal with nothing else failing.
	assertWidthsSumTo100(t, cols)
}

// assertWidthsSumTo100 pins listview.NormalizeWidths having been applied to a
// column-spec slice.
func assertWidthsSumTo100(t *testing.T, cols []listview.ColumnSpec) {
	t.Helper()
	sum := 0
	for _, c := range cols {
		sum += c.WidthPct
	}
	if sum != 100 {
		t.Errorf("column WidthPct sum = %d, want 100 (listview.NormalizeWidths not applied); cols = %+v", sum, cols)
	}
}

func TestToColumns_SingleRepo_WidthsNormalized(t *testing.T) {
	items := []provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	}

	assertWidthsSumTo100(t, toColumns(items))
}

// TestMultiRepo_GatesOnScope_NotScopeDisplay pins which Identity field the
// dynamic Repo column keys off. Every other fixture sets Scope == ScopeDisplay,
// so swapping the gate to ScopeDisplay is invisible to them — yet two distinct
// repos sharing a display name would then render a disambiguating column that
// disambiguates nothing, and a single repo whose display name varies per row
// would grow a column it does not need.
func TestMultiRepo_GatesOnScope_NotScopeDisplay(t *testing.T) {
	// Distinct Scope, identical ScopeDisplay → multi-repo (gate is Scope).
	a := mkNotification("1", "owner/repo1", "A", provider.NotificationReasonOther, false, fixedNow)
	a.Identity.ScopeDisplay = "shared-display"
	b := mkNotification("2", "owner/repo2", "B", provider.NotificationReasonOther, false, fixedNow)
	b.Identity.ScopeDisplay = "shared-display"

	if !multiRepo([]provider.Notification{a, b}) {
		t.Error("multiRepo(distinct Scope, identical ScopeDisplay) = false, want true (gate must be Identity.Scope)")
	}

	// Identical Scope, distinct ScopeDisplay → single repo.
	c := mkNotification("3", "owner/repo", "C", provider.NotificationReasonOther, false, fixedNow)
	c.Identity.ScopeDisplay = "display-one"
	d := mkNotification("4", "owner/repo", "D", provider.NotificationReasonOther, false, fixedNow)
	d.Identity.ScopeDisplay = "display-two"

	if multiRepo([]provider.Notification{c, d}) {
		t.Error("multiRepo(identical Scope, distinct ScopeDisplay) = true, want false (gate must be Identity.Scope)")
	}
}

// ─── Rendering: convention 8 (render through View() after WindowSizeMsg) ────
//
// The recover() guard sits above SetFeed, not just above View(): View returns
// the viewport's already-rendered content, so every table.renderRow call — the
// one place a column/cell divergence actually panics — happens inside
// SetFeed → listview.SetItems → setColumnsAndRows → SetRows → UpdateViewport.

func TestView_RendersAfterWindowSizeMsg_SingleRepo(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetFeed/View() panicked: %v", r)
		}
	}()

	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})
	_ = m.View()
}

func TestView_RendersAfterWindowSizeMsg_MultiRepo(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetFeed/View() panicked: %v", r)
		}
	}()

	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo1", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo2", "PR two", provider.NotificationReasonMentioned, true, fixedNow),
	})
	_ = m.View()
}

// ─── Unread emphasis: convention 6 (named style, not substring) ─────────────

// TestTitleStyle_ReadVsUnread_NamedStyles is the primary convention 6
// assertion, and it deliberately inspects the *style object* rather than
// rendered text (decision 56). lipgloss resolves the Ascii profile in a test
// binary, so Render is the identity function there and styled/unstyled bytes
// are identical — which is why the previous rendered-string comparisons stayed
// green after the entire unread branch was deleted.
//
// GetBold/GetForeground are compared against styles.Styles' own fields rather
// than against literal colors, so a palette change cannot make this vacuous,
// and the final check pins that the two branches resolve to *different*
// styles: swapping either branch to the other's style fails here.
func TestTitleStyle_ReadVsUnread_NamedStyles(t *testing.T) {
	s := styles.DefaultStyles()

	unread := mkNotification("1", "owner/repo", "Unread title", provider.NotificationReasonOther, false, fixedNow)
	read := mkNotification("2", "owner/repo", "Read title", provider.NotificationReasonOther, true, fixedNow)

	unreadStyle := titleStyle(unread, s)
	readStyle := titleStyle(read, s)

	// Unread == styles.Styles.Title: bold, Primary foreground.
	if !unreadStyle.GetBold() {
		t.Errorf("titleStyle(unread).GetBold() = false, want true (unread emphasis via styles.Styles.Title)")
	}
	if got, want := unreadStyle.GetForeground(), s.Title.GetForeground(); got != want {
		t.Errorf("titleStyle(unread) foreground = %v, want %v (styles.Styles.Title)", got, want)
	}

	// Read == the empty style: no bold, and no foreground at all, so Render emits
	// nothing and the cell inherits table.renderRow's Cell/Selected styling. A
	// named foreground here would override the selection's own foreground for
	// this one cell, which is invisible on any theme where Foreground equals
	// SelectBackground (Matrix sets both to #00ff41).
	if readStyle.GetBold() {
		t.Errorf("titleStyle(read).GetBold() = true, want false (read rows carry no emphasis)")
	}
	var unset lipgloss.TerminalColor = lipgloss.NoColor{}
	if got := readStyle.GetForeground(); got != unset {
		t.Errorf("titleStyle(read) foreground = %v, want unset (%v) — read cells must not override the table's Selected foreground", got, unset)
	}

	// The two must be distinguishable at all — a single shared style would
	// satisfy neither convention 6 nor the unread-emphasis criterion.
	if unreadStyle.GetBold() == readStyle.GetBold() &&
		unreadStyle.GetForeground() == readStyle.GetForeground() {
		t.Error("titleStyle resolves read and unread to the same style; unread emphasis is not applied")
	}
}

// TestTitleCell_RendersThroughTitleStyle pins that titleCell actually applies
// titleStyle's style rather than returning the bare Title, which the
// style-object test above cannot see. This is the one place bytes are needed,
// so it forces a color profile (the internal/ui/components/table/table_test.go
// precedent) — under the default Ascii profile the styled and unstyled forms
// are byte-identical.
func TestTitleCell_RendersThroughTitleStyle(t *testing.T) {
	forceTrueColor(t)

	s := styles.DefaultStyles()

	unread := mkNotification("1", "owner/repo", "Unread title", provider.NotificationReasonOther, false, fixedNow)
	got := titleCell(unread, s)
	if want := titleStyle(unread, s).Render(unread.Title); got != want {
		t.Errorf("titleCell(unread) = %q, want %q (titleStyle applied)", got, want)
	}
	if got == unread.Title {
		t.Errorf("titleCell(unread) = %q, want ANSI-styled output, not the bare Title", got)
	}

	read := mkNotification("2", "owner/repo", "Read title", provider.NotificationReasonOther, true, fixedNow)
	gotRead := titleCell(read, s)
	// The read branch is the empty style, so this asserts the *absence* of any
	// escape sequence even with TrueColor forced — a named style slipped into
	// that branch would fail here, which is the regression this pins.
	if gotRead != read.Title {
		t.Errorf("titleCell(read) = %q, want the bare Title %q — read cells must emit no escape sequence", gotRead, read.Title)
	}
	if gotRead == got {
		t.Errorf("read and unread title cells are byte-identical (%q); unread emphasis is not rendered", got)
	}
}

// TestToRows_TitleCell_UsesTitleStyleForBothStates pins that toRows routes the
// Title column through titleCell (and therefore titleStyle) for read and
// unread rows alike, again with a forced color profile so the two are
// distinguishable at the byte level.
func TestToRows_TitleCell_UsesTitleStyleForBothStates(t *testing.T) {
	forceTrueColor(t)

	s := styles.DefaultStyles()
	items := []provider.Notification{
		mkNotification("1", "owner/repo", "Same title", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo", "Same title", provider.NotificationReasonReviewRequested, true, fixedNow),
	}

	rows := toRows(items, s)

	const titleCol = 1 // single-repo layout: [Reason][Title][Updated]
	if got, want := rows[0][titleCol], s.Title.Render("Same title"); got != want {
		t.Errorf("unread title cell = %q, want %q (styles.Styles.Title)", got, want)
	}
	if got, want := rows[1][titleCol], "Same title"; got != want {
		t.Errorf("read title cell = %q, want the bare title %q (empty style, no escape sequence)", got, want)
	}
	if rows[0][titleCol] == rows[1][titleCol] {
		t.Errorf("read and unread cells for the same title are identical (%q); unread emphasis is missing from toRows", rows[0][titleCol])
	}
}

// ─── Empty Title renders "—" (decision 58) ───────────────────────────────────

// TestTitleCell_EmptyTitle_RendersDash pins decision 58's dash-before-styling
// order: an untitled *unread* row must still show the dash, so dashIfEmpty has
// to run before the style is applied rather than after.
//
// The forced color profile is what makes the ordering observable. Under the
// default Ascii profile, dashIfEmpty(Render("")) and Render(dashIfEmpty(""))
// are both "—"; with real escapes the wrong order yields an ANSI-wrapped empty
// string, which dashIfEmpty then sees as non-empty and leaves alone — a blank
// cell.
func TestTitleCell_EmptyTitle_RendersDash(t *testing.T) {
	forceTrueColor(t)

	s := styles.DefaultStyles()

	unread := mkNotification("1", "owner/repo", "", provider.NotificationReasonOther, false, fixedNow)
	if got, want := titleCell(unread, s), titleStyle(unread, s).Render("—"); got != want {
		t.Errorf("titleCell(untitled unread) = %q, want %q (dashed before styling)", got, want)
	}

	read := mkNotification("2", "owner/repo", "", provider.NotificationReasonOther, true, fixedNow)
	if got, want := titleCell(read, s), titleStyle(read, s).Render("—"); got != want {
		t.Errorf("titleCell(untitled read) = %q, want %q", got, want)
	}
}

func TestToRows_EmptyTitle_RendersDash(t *testing.T) {
	forceTrueColor(t)

	s := styles.DefaultStyles()
	items := []provider.Notification{
		mkNotification("1", "owner/repo", "", provider.NotificationReasonOther, false, fixedNow),
	}

	rows := toRows(items, s)

	const titleCol = 1 // single-repo layout: [Reason][Title][Updated]
	if !strings.Contains(rows[0][titleCol], "—") {
		t.Errorf("empty Title cell = %q, want to contain %q", rows[0][titleCol], "—")
	}
	if strings.TrimSpace(rows[0][titleCol]) == "" {
		t.Errorf("empty Title cell = %q, must never render blank (reads as a rendering bug)", rows[0][titleCol])
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
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
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
// the cursor/selection survives — still on the *same item*, per decision 55.
//
// The fixture is shaped so that no accidental cursor behaviour can satisfy the
// same-item assertion: ≥2 rows survive the collapse, the pre-filter cursor
// index (2 of 5) is neither zero nor last, and the surviving row's post-filter
// index (1 of 3) is neither zero nor last either. Reset-to-0, clamp-to-last and
// keep-the-saved-index all land on a different row than the identity restore.
func TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Row 0: id 1, a/repo1, Subscribed
	// Row 1: id 2, a/repo1, Mentioned
	// Row 2: id 3, a/repo1, Mentioned      <- cursor starts here
	// Row 3: id 4, b/repo2, Subscribed     <- the second scope
	// Row 4: id 5, a/repo1, Mentioned
	//
	// Mentioned precedes Subscribed in enum order, so the first `f` filters to
	// Mentioned: ids 2, 3 and 5 survive, all in a/repo1 — so the Repo column
	// collapses — and the cursor's own item (id 3) lands at index 1.
	feed := []provider.Notification{
		mkNotification("1", "a/repo1", "Subscribed in repo1", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("2", "a/repo1", "Mentioned in repo1 (first)", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "a/repo1", "Mentioned in repo1 (cursor here)", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("4", "b/repo2", "Subscribed in repo2", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("5", "a/repo1", "Mentioned in repo1 (last)", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m = m.SetFeed(feed)

	// Sanity: starts multi-repo (Repo column present).
	if cols := toColumns(m.list.Items()); len(cols) != 4 {
		t.Fatalf("pre-filter columns = %d, want 4 (multi-repo)", len(cols))
	}

	m.list.SetCursor(2) // id "3": neither the first nor the last row
	if got := m.list.SelectedIndex(); got != 2 {
		t.Fatalf("cursor after SetCursor(2) = %d, want 2", got)
	}

	m, _ = m.Update(keyRune('f'))
	if !m.reasonFilterActive || m.reasonFilter != provider.NotificationReasonMentioned {
		t.Fatalf("after 1st f: active=%v reason=%v, want Mentioned", m.reasonFilterActive, m.reasonFilter)
	}

	visible := m.list.Items()
	if len(visible) != 3 {
		t.Fatalf("post-filter items = %d, want 3 (ids 2, 3, 5 survive)", len(visible))
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

	// (c) the cursor is still on the *same item* (id "3"), not merely in
	// range: listview.setColumnsAndRows restores the cursor positionally, so
	// without the pane's identity-based restore (decision 55) the saved index
	// 2 would now point at id "5".
	idx := m.list.SelectedIndex()
	if idx < 0 || idx >= len(visible) {
		t.Fatalf("cursor after collapse = %d, out of range for %d items", idx, len(visible))
	}
	if idx != 1 {
		t.Errorf("cursor after collapse = %d, want 1 (id \"3\"'s new index)", idx)
	}
	if got := m.list.Items()[idx].Identity.ID; got != "3" {
		t.Fatalf("cursor after collapse points at ID %q, want %q (same item survived)", got, "3")
	}
}

// TestCycleReasonFilter_Collapse_CursorClamped_ItemDropped covers the other
// half of decision 55: when the cursor's own row does NOT survive the filter
// there is no identity to restore to, and listview's positional clamp is the
// correct behaviour. The saved cursor (index 3) is past the end of the
// 2-row result, so it must clamp to the last row (index 1) — which also pins
// that the clamp is to len(rows)-1 and not to 0.
func TestCycleReasonFilter_Collapse_CursorClamped_ItemDropped(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	feed := []provider.Notification{
		mkNotification("1", "a/repo1", "Mentioned", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "a/repo1", "Mentioned too", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "b/repo2", "Subscribed", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("4", "b/repo2", "Subscribed (cursor here, dropped by filter)", provider.NotificationReasonSubscribed, false, fixedNow),
	}
	m = m.SetFeed(feed)

	m.list.SetCursor(3) // id "4", which the Mentioned filter drops entirely
	if got := m.list.SelectedIndex(); got != 3 {
		t.Fatalf("cursor after SetCursor(3) = %d, want 3", got)
	}

	m, _ = m.Update(keyRune('f')) // -> Mentioned (first present reason in enum order)

	visible := m.list.Items()
	if len(visible) != 2 || visible[0].Identity.ID != "1" || visible[1].Identity.ID != "2" {
		t.Fatalf("post-filter items = %v, want ids 1 and 2", visible)
	}

	idx := m.list.SelectedIndex()
	if idx < 0 || idx >= len(visible) {
		t.Fatalf("cursor after collapse = %d, out of range for %d items", idx, len(visible))
	}
	if idx != len(visible)-1 {
		t.Errorf("cursor after collapse = %d, want %d (clamped to the last surviving row)", idx, len(visible)-1)
	}
}

// TestSetFeed_PreservesSelectedItemAcrossReorder covers the poll-refresh
// direction of decision 55: SetFeed replaces the whole feed, and a row that
// moved (the merge sorts newest-first, so ties and new arrivals reshuffle
// constantly) must keep the cursor rather than keeping the index.
func TestSetFeed_PreservesSelectedItemAcrossReorder(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "a/repo1", "One", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "a/repo1", "Two", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "a/repo1", "Three", provider.NotificationReasonMentioned, false, fixedNow),
	})
	m.list.SetCursor(1) // id "2"

	// A poll lands with a new newest row, pushing id "2" from index 1 to 2.
	m = m.SetFeed([]provider.Notification{
		mkNotification("9", "a/repo1", "Brand new", provider.NotificationReasonMentioned, false, fixedNow.Add(time.Minute)),
		mkNotification("1", "a/repo1", "One", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "a/repo1", "Two", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "a/repo1", "Three", provider.NotificationReasonMentioned, false, fixedNow),
	})

	idx := m.list.SelectedIndex()
	if got := m.list.Items()[idx].Identity.ID; got != "2" {
		t.Errorf("cursor after refresh points at ID %q (index %d), want %q", got, idx, "2")
	}
}

// ─── f filter: observability (decision 57) ──────────────────────────────────

func TestReasonFilter_Accessor_ReportsCyclePosition(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "B", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	if _, active := m.ReasonFilter(); active {
		t.Fatal("ReasonFilter() reports active on the initial 'all' position")
	}

	m, _ = m.Update(keyRune('f'))
	reason, active := m.ReasonFilter()
	if !active || reason != provider.NotificationReasonMentioned {
		t.Errorf("ReasonFilter() = (%v, %v), want (Mentioned, true)", reason, active)
	}

	m, _ = m.Update(keyRune('f'))
	m, _ = m.Update(keyRune('f')) // back to "all"
	if _, active := m.ReasonFilter(); active {
		t.Error("ReasonFilter() reports active after cycling back to 'all'")
	}
}

// TestView_ActiveFilter_RendersIndicator_NotBareEmptyInbox is decision 57's
// measured failure: filter to a reason, then poll in a feed that has no rows of
// that reason. Without the indicator the pane renders the plain "no
// notifications" text while a user-set filter is what hides every row.
func TestView_ActiveFilter_RendersIndicator_NotBareEmptyInbox(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A mention", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	// Baseline: no filter active, no indicator.
	if got := m.View(); strings.Contains(got, "Filter:") {
		t.Errorf("View() on the 'all' position renders a Filter indicator:\n%s", got)
	}

	m, _ = m.Update(keyRune('f')) // -> Mentioned
	if got := m.View(); !strings.Contains(got, "Filter: "+display.NotificationReasonLabel(provider.NotificationReasonMentioned)) {
		t.Errorf("View() with the Mentioned filter active does not render the indicator:\n%s", got)
	}

	// A poll brings a feed with no Mentioned rows at all: zero rows visible.
	m = m.SetFeed([]provider.Notification{
		mkNotification("3", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})
	if got := len(m.list.Items()); got != 0 {
		t.Fatalf("post-refresh visible items = %d, want 0 (no Mentioned rows in the new feed)", got)
	}

	view := m.View()
	if !strings.Contains(view, "Filter: "+display.NotificationReasonLabel(provider.NotificationReasonMentioned)) {
		t.Errorf("an empty *filtered* feed must not render as a bare empty inbox; View() =\n%s", view)
	}
}

// ─── r must not strand the pane on a spinner (decision 58) ──────────────────

// TestUpdate_RKey_DoesNotStrandSpinner pins the stopgap: listview's `r` sets
// loading = true and shows the spinner before batching config.Fetch(), and this
// pane's Fetch is still a stub returning nil, so nothing would ever call
// HandleFetchResult. Task 15 removes both the stopgap and this test's reason to
// exist when the real fetch lands.
func TestUpdate_RKey_DoesNotStrandSpinner(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Still here after r", provider.NotificationReasonMentioned, false, fixedNow),
	})

	m, _ = m.Update(keyRune('r'))

	view := m.View()
	if strings.Contains(view, "Loading notifications") {
		t.Errorf("after r the pane renders the loading spinner and nothing will ever clear it:\n%s", view)
	}
	if !strings.Contains(view, "Still here after r") {
		t.Errorf("after r the pane no longer renders its rows:\n%s", view)
	}
}

// ─── The vanished-reason branch resets to "all", not to present[0] ───────────

// TestCycleReasonFilter_SelectedReasonVanished_ResetsToAll pins the `idx < 0`
// branch: a poll can replace the feed with one that has no rows of the
// currently selected reason (decision 57's measured scenario), and the next `f`
// must then return to the "all" position rather than jumping to the first
// present reason — which would silently move the user's filter sideways.
func TestCycleReasonFilter_SelectedReasonVanished_ResetsToAll(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A mention", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	m, _ = m.Update(keyRune('f')) // -> Mentioned
	if !m.reasonFilterActive || m.reasonFilter != provider.NotificationReasonMentioned {
		t.Fatalf("after 1st f: active=%v reason=%v, want Mentioned", m.reasonFilterActive, m.reasonFilter)
	}

	// The refreshed feed has no Mentioned rows: Subscribed is the only reason
	// present, so present[0] == Subscribed. The cycle must NOT land there.
	m = m.SetFeed([]provider.Notification{
		mkNotification("3", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("4", "owner/repo", "More subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	m, _ = m.Update(keyRune('f'))

	if m.reasonFilterActive {
		t.Fatalf("after f with the selected reason gone: active=%v reason=%v, want the 'all' position", m.reasonFilterActive, m.reasonFilter)
	}
	if got := len(m.list.Items()); got != 2 {
		t.Errorf("visible items = %d, want 2 (whole feed restored by the 'all' position)", got)
	}
}

// ─── listview.ViewMode sanity: `f` is a no-op while a detail view is open ───

// TestUpdate_FKey_NoopOutsideListMode drives the pane into ViewDetail and then
// presses `f`, asserting the cycle does not advance. Per decision 57 this pane
// sets no FilterFunc, so listview's search mode is unreachable in phase 1 and
// the ViewList half of the guard is the only testable one.
func TestUpdate_FKey_NoopOutsideListMode(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "B", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	if m.list.GetViewMode() != listview.ViewList {
		t.Fatalf("precondition: expected ViewList, got %v", m.list.GetViewMode())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.list.GetViewMode() != listview.ViewDetail {
		t.Fatalf("precondition: expected ViewDetail after enter, got %v", m.list.GetViewMode())
	}

	m, _ = m.Update(keyRune('f'))

	if _, active := m.ReasonFilter(); active {
		t.Error("f cycled the reason filter while the pane was not in list mode")
	}
}

// ─── app-chrome forwarders ───────────────────────────────────────────────────

// TestChromeForwarders_MatchUnderlyingListview pins the four accessors that
// exist solely to feed app.Model's chrome — GetContextItems, GetScrollPercent,
// GetStatusMessage and HasContextBar — against the listview values they are
// meant to forward, on a seeded feed and in both view modes the pane can reach.
//
// Honest limitation, stated so nobody reads more into this test than it gives:
// in phase 1 all four *are* the zero value whatever the pane does. HasContextBar
// resolves through listview.Model.config.HasContextBar, which this pane leaves
// nil (always false), and the other three only return non-zero when
// `viewMode == ViewDetail && detail != nil` — unreachable here because decision
// 3 gives the pane no detail view, so its EnterDetail hook returns a nil
// DetailView. Replacing any of the four with a hardcoded zero literal is
// therefore a genuine equivalent today, not an unpinned regression.
//
// What this does pin is the delegation itself: any forwarder that starts
// returning something *other* than its listview counterpart fails, including
// after task 13 gives the pane a real detail view and turns these into live,
// non-zero values. The `!= m.list.X()` form is deliberate — comparing against a
// hand-written zero constant would keep passing when listview's own contract
// changes underneath.
func TestChromeForwarders_MatchUnderlyingListview(t *testing.T) {
	base := NewModelWithStyles(styles.DefaultStyles(), nil)
	base.list, _ = base.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	base = base.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "other/repo", "B", provider.NotificationReasonSubscribed, true, fixedNow),
	})

	detail := base
	detail, _ = detail.Update(tea.KeyMsg{Type: tea.KeyEnter})

	modes := []struct {
		name string
		m    Model
	}{
		{"list mode", base},
		{"after enter", detail},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			m := mode.m

			if got, want := m.GetContextItems(), m.list.GetContextItems(); len(got) != len(want) {
				t.Errorf("GetContextItems() returned %d items, underlying listview has %d", len(got), len(want))
			} else {
				for i := range got {
					if got[i] != want[i] {
						t.Errorf("GetContextItems()[%d] = %+v, listview has %+v", i, got[i], want[i])
					}
				}
			}
			if got, want := m.GetScrollPercent(), m.list.GetScrollPercent(); got != want {
				t.Errorf("GetScrollPercent() = %v, listview has %v", got, want)
			}
			if got, want := m.GetStatusMessage(), m.list.GetStatusMessage(); got != want {
				t.Errorf("GetStatusMessage() = %q, listview has %q", got, want)
			}
			if got, want := m.HasContextBar(), m.list.HasContextBar(); got != want {
				t.Errorf("HasContextBar() = %v, listview has %v", got, want)
			}
			// IsSearching is deliberately not covered here: decision 57 leaves
			// FilterFunc nil, so listview's search mode is unreachable and both
			// branches return false permanently. It is a confirmed genuine
			// equivalent, and a row asserting false == false would only look
			// like coverage.
		})
	}
}

// ─── Task 13: the four render states (decisions 11, 17, 57, 58, 63) ─────────
//
// Each state below is asserted by its own test, and every test checks both
// its own state's discriminating substring AND the absence of the other three
// states' discriminating substrings — giving all six pairs mutual
// distinguishability, not merely four positive assertions.

const (
	emptyInboxMarker    = "You're all caught up."
	filterEmptyMarker   = "No notifications match Filter:"
	errorMarker         = "Notifications unavailable:"
	capabilityMarker    = "not supported by this configuration"
	tokenScopeSkeleton  = "GitHub token scope required: notifications"
	nilClientMsgMarker  = "no notifications client configured"
	pressRToRefreshText = "Press r"
)

// assertOtherStatesAbsent fails if view contains any of the three markers
// that do not belong to the state under test.
func assertOtherStatesAbsent(t *testing.T, view string, own string) {
	t.Helper()
	for _, marker := range []string{emptyInboxMarker, filterEmptyMarker, errorMarker, capabilityMarker} {
		if marker == own {
			continue
		}
		if strings.Contains(view, marker) {
			t.Errorf("view for state %q also contains state marker %q; states are not mutually distinguishable:\n%s", own, marker, view)
		}
	}
}

// TestView_EmptyInbox_ReadsAsClear_NotError pins decision 63's first state:
// the feed itself has no rows and no `f` filter is active. It must read as
// "you're clear", never as an error, and per decision 58's `r` stopgap must
// not tell the user to press a key this pane currently swallows.
func TestView_EmptyInbox_ReadsAsClear_NotError(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed(nil)

	view := m.View()

	if !strings.Contains(view, emptyInboxMarker) {
		t.Errorf("empty-inbox view = %q, want to contain %q", view, emptyInboxMarker)
	}
	if strings.Contains(view, pressRToRefreshText) {
		t.Errorf("empty-inbox view = %q, must not tell the user to press r (decision 58: r is swallowed)", view)
	}
	assertOtherStatesAbsent(t, view, emptyInboxMarker)
}

// TestView_FilterEmpty_DistinctFromEmptyInbox_NamesActiveFilter pins decision
// 63's second, genuinely reachable state (decision 57's measured bug): the
// feed has rows, but the active `f` reason filter matches none of them. This
// must not render as the plain empty-inbox text.
func TestView_FilterEmpty_DistinctFromEmptyInbox_NamesActiveFilter(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A mention", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	m, _ = m.Update(keyRune('f')) // -> Mentioned

	// A poll lands with no Mentioned rows at all: zero rows visible under the
	// active filter, while the feed itself is non-empty.
	m = m.SetFeed([]provider.Notification{
		mkNotification("3", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})
	if got := len(m.list.Items()); got != 0 {
		t.Fatalf("precondition: visible items = %d, want 0 (no Mentioned rows in the new feed)", got)
	}

	view := m.View()

	wantLabel := "Filter: " + display.NotificationReasonLabel(provider.NotificationReasonMentioned)
	if !strings.Contains(view, filterEmptyMarker) || !strings.Contains(view, wantLabel) {
		t.Errorf("filter-empty view = %q, want to contain %q and %q", view, filterEmptyMarker, wantLabel)
	}
	if !strings.Contains(view, "Press f to cycle back to all reasons.") {
		t.Errorf("filter-empty view = %q, want to name how to clear the filter", view)
	}
	assertOtherStatesAbsent(t, view, filterEmptyMarker)
}

// TestView_Error_CarriesTokenScopeSkeleton_AndTakesPriorityOverRows pins
// decision 63's error state and decision 17's token-scope skeleton. The error
// is constructed from a real *github.Adapter with no NotificationsClient
// configured, so the nil-client message asserted here is the adapter's
// actual production string, not a hand-typed guess that could drift from it.
//
// The feed is seeded with a row before HandleFetchResult(nil, err) lands, to
// pin that the error state pre-empts the table view rather than rendering
// stale rows underneath it.
func TestView_Error_CarriesTokenScopeSkeleton_AndTakesPriorityOverRows(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Must not render once errored", provider.NotificationReasonMentioned, false, fixedNow),
	})

	m = m.HandleFetchResult(nil, listErr)

	view := m.View()

	if !strings.Contains(view, errorMarker) {
		t.Errorf("error view = %q, want to contain %q", view, errorMarker)
	}
	if !strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("error view = %q, want to contain decision 17's token-scope skeleton %q", view, tokenScopeSkeleton)
	}
	if !strings.Contains(view, nilClientMsgMarker) {
		t.Errorf("error view = %q, want to fold in the adapter's real nil-client message %q", view, nilClientMsgMarker)
	}
	if strings.Contains(view, "Must not render once errored") {
		t.Errorf("error view = %q, must not fall through to stale table rows", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestView_Error_TakesPriorityOverEmptyInbox_WhenFeedIsEmpty covers the
// ordering gap the row-seeded test above cannot: an errored fetch whose feed
// was never populated at all (items == 0) must still render the error state,
// not decision 63's empty-inbox text, even though both share the same
// "len(items) == 0" precondition. View()'s error check must run before its
// items-emptiness check for this to hold.
func TestView_Error_TakesPriorityOverEmptyInbox_WhenFeedIsEmpty(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// No SetFeed call at all: items start at zero.

	m = m.HandleFetchResult(nil, listErr)

	view := m.View()
	if !strings.Contains(view, errorMarker) {
		t.Errorf("error-with-empty-feed view = %q, want to contain %q", view, errorMarker)
	}
	if strings.Contains(view, emptyInboxMarker) {
		t.Errorf("error-with-empty-feed view = %q, must not render the empty-inbox text", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestView_CapabilityUnsupported_DistinctFromOtherThreeStates pins decision
// 63's third state at the pane level, per its own instruction: this state is
// unreachable through the tab in phase 1 (see SetCapabilityUnsupported's doc
// comment), so it is asserted here by putting the pane in that state
// directly, rather than as an app-level test that could never fail for the
// right reason.
func TestView_CapabilityUnsupported_DistinctFromOtherThreeStates(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetCapabilityUnsupported()

	view := m.View()

	if !strings.Contains(view, capabilityMarker) {
		t.Errorf("capability-unsupported view = %q, want to contain %q", view, capabilityMarker)
	}
	assertOtherStatesAbsent(t, view, capabilityMarker)
}

// TestView_SuccessfulFeedAfterError_ClearsErrorState pins the recovery path,
// which is the pane's only protection against a pre-existing listview bug and
// is otherwise held by nothing.
//
// listview.HandleFetchResult's success path never assigns m.err = nil (it
// returns early on the error path and leaves the field alone otherwise) while
// listview.viewList short-circuits on m.err != nil — so a pane that recovers
// through it stays pinned to the error render forever. This pane escapes that
// only because HandleFetchResult routes its *success* path through SetFeed ->
// SetItems, and SetItems does clear the field. Rewriting that forward as
// `m.list = m.list.HandleFetchResult(items, nil)` looks like a harmless
// simplification, keeps the whole suite green without this test, and silently
// makes one failed fetch permanent. Task 15's poller is what calls this.
func TestView_SuccessfulFeedAfterError_ClearsErrorState(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, listErr)
	if !strings.Contains(m.View(), errorMarker) {
		t.Fatalf("precondition: view = %q, want the error state before recovery", m.View())
	}

	m = m.HandleFetchResult([]provider.Notification{
		mkNotification("1", "owner/repo", "Recovered row", provider.NotificationReasonMentioned, false, fixedNow),
	}, nil)

	view := m.View()
	if strings.Contains(view, errorMarker) {
		t.Errorf("view after a successful feed = %q, must not still render the error state", view)
	}
	if strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("view after a successful feed = %q, must not still carry the token-scope skeleton", view)
	}
	if !strings.Contains(view, "Recovered row") {
		t.Errorf("view after a successful feed = %q, want the recovered row rendered", view)
	}
}

// TestView_Loading_DoesNotClaimCaughtUp pins View()'s !m.list.Loading()
// conjunct: a fetch in flight with zero rows so far must show listview's
// spinner, never decision 63's "you're all caught up" text, which would be an
// outright lie while data is still on the way.
//
// The `r` message goes to m.list directly rather than through m.Update,
// because the pane deliberately swallows `r` at its own level while the Fetch
// hook is a stub (decision 58) — the state under test here is listview's
// loading flag, not the pane's key handling.
//
// FORWARD: task 15 — this conjunct does NOT currently protect the *initial*
// fetch. listview.Init sets the spinner visible but never sets m.loading
// (listview.go's Init), so Loading() is false while the first fetch is in
// flight and this pane will render "you're all caught up" during startup once
// a real Fetch replaces the stub. Task 15 must set loading on the initial
// fetch, or move this pane off listview's flag.
func TestView_Loading_DoesNotClaimCaughtUp(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed(nil)
	if !strings.Contains(m.View(), emptyInboxMarker) {
		t.Fatalf("precondition: view = %q, want the empty-inbox state before the refresh", m.View())
	}

	m.list, _ = m.list.Update(keyRune('r'))
	if !m.list.Loading() {
		t.Fatal("precondition: listview must be loading after r")
	}

	view := m.View()
	if strings.Contains(view, emptyInboxMarker) {
		t.Errorf("view while loading = %q, must not claim the user is caught up mid-fetch", view)
	}
	assertOtherStatesAbsent(t, view, "")
}

// TestView_CapabilityUnsupported_OutranksError pins the documented priority
// order between decision 63's first two states. Every other state pair is
// already distinguished by assertOtherStatesAbsent, but capability and error
// are the one pair no other fixture sets *together*, so swapping their two
// checks in View() is otherwise a genuine equivalent that no test can see.
// Capability wins because it describes the configuration, whereas an error
// describes an attempt that configuration should never have made.
func TestView_CapabilityUnsupported_OutranksError(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, listErr)
	m = m.SetCapabilityUnsupported()

	view := m.View()
	if !strings.Contains(view, capabilityMarker) {
		t.Errorf("view with both capability-unsupported and an error = %q, want the capability state to win", view)
	}
	assertOtherStatesAbsent(t, view, capabilityMarker)
}

// ─── task 14: u mark-read / d mark-done, optimistic update + rollback ───────

// fakeMarker is a test double for provider.NotificationSource (task 14). It
// records every MarkRead/MarkDone call it receives — including the exact
// Identity — and returns readErr/doneErr (nil unless a test sets them) so
// failure/rollback paths can be driven deterministically.
type fakeMarker struct {
	readCalls []provider.Identity
	doneCalls []provider.Identity
	readErr   error
	doneErr   error
}

func (f *fakeMarker) List(provider.NotifOpts) ([]provider.Notification, error) {
	return nil, nil
}

func (f *fakeMarker) MarkRead(id provider.Identity) error {
	f.readCalls = append(f.readCalls, id)
	return f.readErr
}

func (f *fakeMarker) MarkDone(id provider.Identity) error {
	f.doneCalls = append(f.doneCalls, id)
	return f.doneErr
}

// runMarkCmd runs cmd synchronously (the way bubbletea itself would, just
// without the goroutine) and feeds the resulting message back into m.Update,
// mirroring how markCmd's notificationMarkResultMsg actually reaches the
// pane in production.
func runMarkCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("want a non-nil tea.Cmd from the mark action")
	}
	msg := cmd()
	m, _ = m.Update(msg)
	return m
}

func newTriagePane(t *testing.T, marker provider.NotificationSource, feed []provider.Notification) Model {
	t.Helper()
	m := NewModelWithStyles(styles.DefaultStyles(), marker)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.now = func() time.Time { return fixedNow }
	return m.SetFeed(feed)
}

// TestMarkRead_Success_AppliesOptimisticallyAndSurvivesSuccess pins the
// happy path: `u` marks the row Read immediately (before the API call
// resolves), issues exactly one MarkRead call for the selected row's own
// Identity, and a successful result leaves the optimistic mark in place.
func TestMarkRead_Success_AppliesOptimisticallyAndSurvivesSuccess(t *testing.T) {
	marker := &fakeMarker{}
	want := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{want})

	m, cmd := m.Update(keyRune('u'))

	// The optimistic mark is applied synchronously inside Update, before the
	// tea.Cmd — and therefore the actual MarkRead call — ever runs.
	item, ok := m.selectedItem()
	if !ok || !item.Read {
		t.Fatalf("after u, selected item = %+v, ok=%v, want Read=true applied optimistically before the API call resolves", item, ok)
	}
	if len(marker.readCalls) != 0 {
		t.Fatalf("MarkRead calls = %d before the cmd runs, want 0 (the call happens inside the tea.Cmd)", len(marker.readCalls))
	}

	m = runMarkCmd(t, m, cmd)

	if len(marker.readCalls) != 1 {
		t.Fatalf("MarkRead calls = %d, want exactly 1", len(marker.readCalls))
	}
	if !marker.readCalls[0].SameItem(want.Identity) {
		t.Errorf("MarkRead called with Identity = %+v, want %+v", marker.readCalls[0], want.Identity)
	}

	item, ok = m.selectedItem()
	if !ok || !item.Read {
		t.Errorf("after a successful MarkRead result, item = %+v, ok=%v, want Read to remain true", item, ok)
	}
}

// TestMarkRead_Failure_RollsBackOptimisticUpdate is the mutation target for
// "rollback made a no-op": a failed MarkRead must un-mark the row, since
// markRead never touches m.feed itself — only dropOverride can undo it.
func TestMarkRead_Failure_RollsBackOptimisticUpdate(t *testing.T) {
	marker := &fakeMarker{readErr: errors.New("boom")}
	want := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{want})

	m, cmd := m.Update(keyRune('u'))
	if item, ok := m.selectedItem(); !ok || !item.Read {
		t.Fatalf("precondition: item = %+v, ok=%v, want Read=true right after u", item, ok)
	}

	m = runMarkCmd(t, m, cmd)

	item, ok := m.selectedItem()
	if !ok || item.Read {
		t.Errorf("after a failed MarkRead result, item = %+v, ok=%v, want Read rolled back to false", item, ok)
	}
}

// TestMarkRead_AlreadyRead_IsOneWay_NoSecondCall is the mutation target for
// "u turned into a toggle": pressing u a second time on a row already marked
// Read (via a prior successful u) must not issue a second MarkRead call and
// must not flip Read back to false. Decision 13 gives GitHub no mark-unread
// endpoint at all, so a toggle here would have nothing real to call.
func TestMarkRead_AlreadyRead_IsOneWay_NoSecondCall(t *testing.T) {
	marker := &fakeMarker{}
	want := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{want})

	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)
	if len(marker.readCalls) != 1 {
		t.Fatalf("precondition: MarkRead calls = %d, want exactly 1 after the first u", len(marker.readCalls))
	}

	m, cmd = m.Update(keyRune('u'))

	if cmd != nil {
		t.Error("a second u on an already-Read row must not issue another tea.Cmd")
	}
	if len(marker.readCalls) != 1 {
		t.Errorf("MarkRead calls after a second u = %d, want still exactly 1", len(marker.readCalls))
	}
	if item, ok := m.selectedItem(); !ok || !item.Read {
		t.Errorf("after a second u, item = %+v, ok=%v, want Read to remain true (no toggle back to unread)", item, ok)
	}
}

// TestMarkRead_AlreadyReadInFeed_NeverCallsMarkRead covers the same one-way
// rule for a row the feed itself already reports as Read (no override
// involved at all): u must be a pure no-op.
func TestMarkRead_AlreadyReadInFeed_NeverCallsMarkRead(t *testing.T) {
	marker := &fakeMarker{}
	already := mkNotification("1", "owner/repo", "Already read", provider.NotificationReasonMentioned, true, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{already})

	m, cmd := m.Update(keyRune('u'))

	if cmd != nil {
		t.Error("u on a row the feed already reports Read must not issue a tea.Cmd")
	}
	if len(marker.readCalls) != 0 {
		t.Errorf("MarkRead calls = %d, want 0", len(marker.readCalls))
	}
}

// TestMarkDone_Success_RemovesRowAndStaysRemoved pins the happy path for `d`:
// the row disappears from Items() immediately (optimistic), and a successful
// result leaves it gone.
func TestMarkDone_Success_RemovesRowAndStaysRemoved(t *testing.T) {
	marker := &fakeMarker{}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonReviewRequested, false, fixedNow)
	other := mkNotification("2", "owner/repo", "Leave me", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target, other})

	m, cmd := m.Update(keyRune('d'))

	// Optimistic removal happens synchronously inside Update, before the
	// tea.Cmd — and therefore the actual MarkDone call — ever runs.
	if len(m.list.Items()) != 1 || m.list.Items()[0].Identity.ID != "2" {
		t.Fatalf("after d, Items() = %+v, want only the other row left", m.list.Items())
	}
	if len(marker.doneCalls) != 0 {
		t.Fatalf("MarkDone calls = %+v before the cmd runs, want none (the call happens inside the tea.Cmd)", marker.doneCalls)
	}

	m = runMarkCmd(t, m, cmd)

	if len(marker.doneCalls) != 1 || !marker.doneCalls[0].SameItem(target.Identity) {
		t.Fatalf("MarkDone calls = %+v, want exactly one call for %+v", marker.doneCalls, target.Identity)
	}
	if len(m.list.Items()) != 1 || m.list.Items()[0].Identity.ID != "2" {
		t.Errorf("after a successful MarkDone result, Items() = %+v, want the row to stay removed", m.list.Items())
	}
}

// TestMarkDone_Failure_RestoresRow is the mutation target for "rollback made
// a no-op" on the d path: a failed MarkDone must bring the row back, and by
// dropping the override rather than re-inserting into m.feed (so decision
// 45's merge order is never reproduced by hand).
func TestMarkDone_Failure_RestoresRow(t *testing.T) {
	marker := &fakeMarker{doneErr: errors.New("boom")}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonReviewRequested, false, fixedNow)
	other := mkNotification("2", "owner/repo", "Leave me", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target, other})

	m, cmd := m.Update(keyRune('d'))
	if len(m.list.Items()) != 1 {
		t.Fatalf("precondition: Items() = %+v, want the target row optimistically removed", m.list.Items())
	}

	m = runMarkCmd(t, m, cmd)

	if len(m.list.Items()) != 2 {
		t.Fatalf("after a failed MarkDone result, Items() = %+v, want both rows restored", m.list.Items())
	}
	foundTarget := false
	for _, n := range m.list.Items() {
		if n.Identity.SameItem(target.Identity) {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Errorf("Items() = %+v, want the rolled-back row's own Identity restored", m.list.Items())
	}
}

// TestMarkRead_ExactIdentity_SelectsRowUnderCursor_NotFirstRow is the
// mutation target for "MarkRead firing on the wrong Identity": with the
// cursor moved onto the second row, u must call MarkRead for that row's
// Identity, never the first row's.
func TestMarkRead_ExactIdentity_SelectsRowUnderCursor_NotFirstRow(t *testing.T) {
	marker := &fakeMarker{}
	first := mkNotification("1", "owner/repo", "First", provider.NotificationReasonReviewRequested, false, fixedNow)
	second := mkNotification("2", "owner/repo", "Second", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{first, second})
	m.list.SetCursor(1)

	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)

	if len(marker.readCalls) != 1 {
		t.Fatalf("MarkRead calls = %d, want exactly 1", len(marker.readCalls))
	}
	if !marker.readCalls[0].SameItem(second.Identity) {
		t.Errorf("MarkRead called with Identity = %+v, want the cursor row's own Identity %+v", marker.readCalls[0], second.Identity)
	}
}

// TestMarkRead_PollWithinDebounceWindow_DoesNotFlickerBack is the core task
// 14 criterion from the spec's Unknowns section, and the mutation target for
// both "dropping the override for a still-unread polled row" and "debounce
// window forced to zero": a poll landing well inside the 30s window, still
// reporting the row unread, must not un-mark it — the local intent is held
// until the server agrees or the window elapses, whichever comes first.
func TestMarkRead_PollWithinDebounceWindow_DoesNotFlickerBack(t *testing.T) {
	marker := &fakeMarker{}
	stale := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{stale})

	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)
	if item, ok := m.selectedItem(); !ok || !item.Read {
		t.Fatalf("precondition: item = %+v, ok=%v, want Read=true after the successful mark", item, ok)
	}

	// A poll lands 1s later (well inside the 30s debounce window) and still
	// reports the row unread — GitHub's read state is eventually consistent
	// (spec's Unknowns section).
	m.now = func() time.Time { return fixedNow.Add(1 * time.Second) }
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})

	item, ok := m.selectedItem()
	if !ok || !item.Read {
		t.Errorf("after a stale poll inside the debounce window, item = %+v, ok=%v, want Read to stay true (no flicker)", item, ok)
	}
}

// TestMarkRead_PollAfterDebounceWindowExpires_TrustsPoll is the mutation
// target for "debounce window forced to infinite": once the window has
// genuinely elapsed, a poll that still disagrees must win — the override is
// a short-lived buffer (decision 5), not a second permanent source of truth.
func TestMarkRead_PollAfterDebounceWindowExpires_TrustsPoll(t *testing.T) {
	marker := &fakeMarker{}
	stale := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{stale})

	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)

	// A poll lands well past the 30s debounce window, still reporting unread.
	m.now = func() time.Time { return fixedNow.Add(10 * time.Minute) }
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})

	item, ok := m.selectedItem()
	if !ok || item.Read {
		t.Errorf("after a poll past the debounce window, item = %+v, ok=%v, want Read=false (the poll wins)", item, ok)
	}
}

// TestMarkDone_PollWithinDebounceWindow_RowStaysHidden mirrors the read-side
// debounce test for the hidden (d) path: a poll landing inside the window
// that still includes the row must not bring it back.
func TestMarkDone_PollWithinDebounceWindow_RowStaysHidden(t *testing.T) {
	marker := &fakeMarker{}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target})

	m, cmd := m.Update(keyRune('d'))
	m = runMarkCmd(t, m, cmd)

	m.now = func() time.Time { return fixedNow.Add(1 * time.Second) }
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonReviewRequested, false, fixedNow),
	})

	if len(m.list.Items()) != 0 {
		t.Errorf("Items() = %+v, want the row to stay hidden inside the debounce window", m.list.Items())
	}
}

// TestMarkDone_Success_SurvivesWindowExpiry_WithoutPoll is the regression test
// for a confirmed mark-done coming back on its own. It is deliberately
// poll-free: the only inputs after the successful result are the clock moving
// past markDebounceWindow and one purely local re-render (`f`).
//
// The bug this pins: while success left the override as the sole holder of the
// dismissal, visibleItems re-derived from an m.feed that still contained the
// row, so the instant the window elapsed the row the server had already
// accepted as done reappeared — no stale poll needed, just 30 seconds and any
// keypress that re-derives. That is data resurrection, not a flicker, which is
// why the fix commits the mark into the feed (commitOverride) rather than
// widening the window.
//
// Every row shares one reason so the `f` cycle re-derives without also
// filtering rows out: what the assertion sees is the override/feed interaction
// alone.
func TestMarkDone_Success_SurvivesWindowExpiry_WithoutPoll(t *testing.T) {
	marker := &fakeMarker{}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonMentioned, false, fixedNow)
	second := mkNotification("2", "owner/repo", "Keep me", provider.NotificationReasonMentioned, false, fixedNow)
	third := mkNotification("3", "owner/repo", "Keep me too", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target, second, third})

	m, cmd := m.Update(keyRune('d'))
	m = runMarkCmd(t, m, cmd)
	if len(m.list.Items()) != 2 {
		t.Fatalf("precondition: Items() = %+v, want the target row gone after a successful MarkDone", m.list.Items())
	}

	// No poll: the clock simply advances past the debounce window and the user
	// presses f, which re-derives the visible rows from the held feed.
	m.now = func() time.Time { return fixedNow.Add(markDebounceWindow + time.Second) }
	m, _ = m.Update(keyRune('f'))

	if len(m.list.Items()) != 2 {
		t.Fatalf("Items() = %+v, want 2 rows — the successfully-dismissed row must not return once the override expires", m.list.Items())
	}
	for _, n := range m.list.Items() {
		if n.Identity.SameItem(target.Identity) {
			t.Errorf("Items() = %+v, want %+v to stay gone: MarkDone succeeded, so the row must never be re-derived from the feed", m.list.Items(), target.Identity)
		}
	}
}

// TestMarkRead_Success_SurvivesWindowExpiry_WithoutPoll is the read-side mirror
// of TestMarkDone_Success_SurvivesWindowExpiry_WithoutPoll: a confirmed `u`
// must not silently revert to unread once the override expires, with no poll
// having contradicted it. Separate from the mark-done case because the two
// commit paths differ — overrideRead rewrites a field on a retained row, while
// overrideHidden drops the row — so one can regress without the other.
func TestMarkRead_Success_SurvivesWindowExpiry_WithoutPoll(t *testing.T) {
	marker := &fakeMarker{}
	target := mkNotification("1", "owner/repo", "Read me", provider.NotificationReasonMentioned, false, fixedNow)
	second := mkNotification("2", "owner/repo", "Untouched", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target, second})

	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)
	if item, ok := m.selectedItem(); !ok || !item.Read {
		t.Fatalf("precondition: item = %+v, ok=%v, want Read=true after a successful MarkRead", item, ok)
	}

	m.now = func() time.Time { return fixedNow.Add(markDebounceWindow + time.Second) }
	m, _ = m.Update(keyRune('f'))

	items := m.list.Items()
	if len(items) != 2 {
		t.Fatalf("Items() = %+v, want both rows still present", items)
	}
	for _, n := range items {
		if n.Identity.SameItem(target.Identity) && !n.Read {
			t.Errorf("Items() = %+v, want %+v to stay Read: MarkRead succeeded, so expiry must not revert it", items, target.Identity)
		}
		// The untouched row must not be swept up by the commit — a commit that
		// matched on something looser than the full identity key would mark the
		// whole feed read and still pass the assertion above.
		if n.Identity.SameItem(second.Identity) && n.Read {
			t.Errorf("Items() = %+v, want %+v to stay unread: only the marked row may be committed", items, second.Identity)
		}
	}
}

// TestMarkDone_Failure_ThenWindowExpiry_DoesNotResurrectTwice pins that the
// commit-on-success path did not quietly become commit-on-every-result: a
// *failed* MarkDone must leave m.feed untouched, so the restored row is still
// there after the window elapses and a later local re-render happens. Without
// this, committing unconditionally in handleMarkResult would pass every other
// mark test in this file.
func TestMarkDone_Failure_ThenWindowExpiry_DoesNotResurrectTwice(t *testing.T) {
	marker := &fakeMarker{doneErr: errors.New("boom")}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonMentioned, false, fixedNow)
	second := mkNotification("2", "owner/repo", "Keep me", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target, second})

	m, cmd := m.Update(keyRune('d'))
	m = runMarkCmd(t, m, cmd)
	if len(m.list.Items()) != 2 {
		t.Fatalf("precondition: Items() = %+v, want the row restored after a failed MarkDone", m.list.Items())
	}

	m.now = func() time.Time { return fixedNow.Add(markDebounceWindow + time.Second) }
	m, _ = m.Update(keyRune('f'))

	if len(m.list.Items()) != 2 {
		t.Fatalf("Items() = %+v, want the rolled-back row to still be present past the window", m.list.Items())
	}
	foundTarget := false
	for _, n := range m.list.Items() {
		if n.Identity.SameItem(target.Identity) {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Errorf("Items() = %+v, want %+v present: a failed mark must never be committed to the feed", m.list.Items(), target.Identity)
	}
}

// TestCanTriage_Blocks_UAndD_WhenNoRows pins that u/d are no-ops with an
// empty feed: canTriage must gate on more than "a marker is set".
func TestCanTriage_Blocks_UAndD_WhenNoRows(t *testing.T) {
	marker := &fakeMarker{}
	m := newTriagePane(t, marker, nil)

	if _, cmd := m.Update(keyRune('u')); cmd != nil {
		t.Error("u with no rows must not issue a tea.Cmd")
	}
	if _, cmd := m.Update(keyRune('d')); cmd != nil {
		t.Error("d with no rows must not issue a tea.Cmd")
	}
	if len(marker.readCalls) != 0 || len(marker.doneCalls) != 0 {
		t.Errorf("marker calls = read:%d done:%d, want none", len(marker.readCalls), len(marker.doneCalls))
	}
}

// TestCanTriage_Blocks_UAndD_WhenErrored pins that a pane in decision 63's
// error render state swallows u/d rather than acting on whatever stale items
// listview.HandleFetchResult left behind (listview never clears m.items on
// its error path).
func TestCanTriage_Blocks_UAndD_WhenErrored(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	marker := &fakeMarker{}
	m := newTriagePane(t, marker, []provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})
	m = m.HandleFetchResult(nil, listErr)

	if _, cmd := m.Update(keyRune('u')); cmd != nil {
		t.Error("u while errored must not issue a tea.Cmd")
	}
	if _, cmd := m.Update(keyRune('d')); cmd != nil {
		t.Error("d while errored must not issue a tea.Cmd")
	}
	if len(marker.readCalls) != 0 || len(marker.doneCalls) != 0 {
		t.Errorf("marker calls = read:%d done:%d, want none", len(marker.readCalls), len(marker.doneCalls))
	}
}

// TestCanTriage_Blocks_UAndD_WhenCapabilityUnsupported pins the third gate:
// decision 63's capability-unsupported state must also block u/d.
func TestCanTriage_Blocks_UAndD_WhenCapabilityUnsupported(t *testing.T) {
	marker := &fakeMarker{}
	m := newTriagePane(t, marker, []provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})
	m = m.SetCapabilityUnsupported()

	if _, cmd := m.Update(keyRune('u')); cmd != nil {
		t.Error("u while capability-unsupported must not issue a tea.Cmd")
	}
	if _, cmd := m.Update(keyRune('d')); cmd != nil {
		t.Error("d while capability-unsupported must not issue a tea.Cmd")
	}
	if len(marker.readCalls) != 0 || len(marker.doneCalls) != 0 {
		t.Errorf("marker calls = read:%d done:%d, want none", len(marker.readCalls), len(marker.doneCalls))
	}
}

// TestMarkRead_NilMarker_IsNoop pins decision 61's nil-safety requirement:
// a pane built with no marker (the capability-absent / nil provider case)
// must swallow u/d without panicking rather than dereferencing a nil
// interface.
func TestMarkRead_NilMarker_IsNoop(t *testing.T) {
	m := newTriagePane(t, nil, []provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
	})

	m, cmd := m.Update(keyRune('u'))
	if cmd != nil {
		t.Error("u with a nil marker must not issue a tea.Cmd")
	}
	if item, ok := m.selectedItem(); !ok || item.Read {
		t.Errorf("item = %+v, ok=%v, want Read unchanged with a nil marker", item, ok)
	}

	m, cmd = m.Update(keyRune('d'))
	if cmd != nil {
		t.Error("d with a nil marker must not issue a tea.Cmd")
	}
	if len(m.list.Items()) != 1 {
		t.Errorf("Items() = %+v, want the row still present with a nil marker", m.list.Items())
	}
}

// TestMarkDone_CursorSurvives_OnPreviouslySelectedNeighbor pins decision 55's
// identity-based cursor restore for d's own removal: with the cursor on the
// middle row of three, marking it done must land the cursor on the row that
// was its neighbor, not wherever listview's positional clamp would leave it
// by accident.
func TestMarkDone_CursorSurvives_OnPreviouslySelectedNeighbor(t *testing.T) {
	marker := &fakeMarker{}
	first := mkNotification("1", "owner/repo", "First", provider.NotificationReasonReviewRequested, false, fixedNow)
	middle := mkNotification("2", "owner/repo", "Middle", provider.NotificationReasonMentioned, false, fixedNow)
	last := mkNotification("3", "owner/repo", "Last", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{first, middle, last})
	m.list.SetCursor(1)

	m, _ = m.Update(keyRune('d'))

	item, ok := m.selectedItem()
	if !ok {
		t.Fatal("want a selection to remain after removing the middle row")
	}
	if item.Identity.ID == "2" {
		t.Fatalf("selected item = %+v, the removed row must not still be selectable", item)
	}
}
