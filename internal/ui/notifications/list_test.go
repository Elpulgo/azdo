package notifications

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/config"
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
// explicitly on the returned value, mirroring MapNotification's fallback —
// tests that need the mapper's one unfixable case (an empty repository
// payload) zero it out explicitly.
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
// a tautology. Style-object assertions are the primary form and
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

// TestColumnOrder_MatchesCellIndices pins notificationColumns and the
// cell<N> index constants in step. toRows assigns by index, so a column
// inserted into notificationColumns without a matching constant shift would
// silently move every cell after it into the wrong column — a bug that
// renders as plausible-looking garbage rather than as a panic.
func TestColumnOrder_MatchesCellIndices(t *testing.T) {
	cols := toColumns(nil)

	if len(cols) != cellCount {
		t.Fatalf("toColumns returned %d columns, want cellCount = %d", len(cols), cellCount)
	}
	for _, tc := range []struct {
		idx   int
		title string
	}{
		{cellRead, ""},
		{cellRepo, "Repo"},
		{cellReason, "Reason"},
		{cellTitle, "Title"},
		{cellUpdated, "Updated"},
	} {
		if cols[tc.idx].Title != tc.title {
			t.Errorf("column at index %d = %q, want %q", tc.idx, cols[tc.idx].Title, tc.title)
		}
	}
}

// TestToColumnsToRows_CellParity_BothShapes is convention 7's invariant over
// both feed shapes that used to select different layouts. The layout is now
// static, so these two cases are expected to be identical — which is exactly
// what makes the test worth keeping: it is what fails if the multi-scope gate
// is ever reintroduced.
func TestToColumnsToRows_CellParity_BothShapes(t *testing.T) {
	s := styles.DefaultStyles()
	for _, tc := range []struct {
		name  string
		items []provider.Notification
	}{
		{"single repo", []provider.Notification{
			mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
			mkNotification("2", "owner/repo", "PR two", provider.NotificationReasonMentioned, true, fixedNow),
		}},
		{"multiple repos", []provider.Notification{
			mkNotification("1", "owner/repo1", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
			mkNotification("2", "owner/repo2", "PR two", provider.NotificationReasonMentioned, true, fixedNow),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cols := toColumns(tc.items)
			if len(cols) != cellCount {
				t.Fatalf("%s columns = %d, want %d", tc.name, len(cols), cellCount)
			}
			if cols[cellRepo].Title != "Repo" {
				t.Errorf("column at cellRepo = %q, want %q", cols[cellRepo].Title, "Repo")
			}
			for i, row := range toRows(tc.items, s) {
				if len(row) != len(cols) {
					t.Fatalf("row %d has %d cells, want %d (== column count)", i, len(row), len(cols))
				}
			}
		})
	}
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

// TestToColumns_RepoColumnIsUnconditional pins the notifications pane's
// deliberate departure from convention 7's multi-scope gating example: the
// Repo column is present whether the feed spans one repo or several.
//
// The gate this replaces made the column vanish precisely when a filter
// narrowed the feed to a single repo — so switching on only_configured_repos
// removed the very answer ("which of my repos is this?") the filter was set
// to sharpen. Reported from real use.
//
// Asserting on both slices matters: a re-added `if multiRepo(items)` gate
// still passes the multi-repo case, so only the single-repo case can catch
// a regression here.
func TestToColumns_RepoColumnIsUnconditional(t *testing.T) {
	single := []provider.Notification{
		mkNotification("1", "owner/repo", "A", provider.NotificationReasonOther, false, fixedNow),
	}
	multi := []provider.Notification{
		mkNotification("1", "owner/repo1", "A", provider.NotificationReasonOther, false, fixedNow),
		mkNotification("2", "owner/repo2", "B", provider.NotificationReasonOther, false, fixedNow),
	}

	for _, tc := range []struct {
		name  string
		items []provider.Notification
	}{
		{"single repo", single},
		{"multiple repos", multi},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cols := toColumns(tc.items)
			if !hasColumn(cols, "Repo") {
				t.Errorf("toColumns(%s) = %+v, want a Repo column — it is unconditional in this pane", tc.name, cols)
			}
			assertWidthsSumTo100(t, cols)
		})
	}
}

// TestToRows_CellCountMatchesColumnCount is convention 7's invariant stated
// directly. table.renderRow indexes m.cols[i], so a row with more cells than
// there are columns panics; this pins the two in step for both slice shapes,
// which is what the removed multiRepo gate used to make fragile.
func TestToRows_CellCountMatchesColumnCount(t *testing.T) {
	items := []provider.Notification{
		mkNotification("1", "owner/repo1", "A", provider.NotificationReasonOther, false, fixedNow),
		mkNotification("2", "owner/repo2", "B", provider.NotificationReasonOther, true, fixedNow),
	}

	cols := toColumns(items)
	for i, row := range toRows(items, styles.DefaultStyles()) {
		if len(row) != len(cols) {
			t.Errorf("row %d has %d cells, want %d (one per column) — table.renderRow would panic", i, len(row), len(cols))
		}
	}
}

// TestToRows_UnreadGlyph_PresentOnlyForUnread pins the marker that makes read
// state legible at a glance.
//
// Before this cell existed, unread state was conveyed *only* by the Title
// cell's boldness. That is close to invisible in themes with little contrast
// between weights, which made `u` (mark read) look like a no-op: the row
// correctly stays in place when unread_only is false, so with no perceptible
// style change there was nothing at all to see. Reported from real use.
//
// The assertion is on the glyph rune rather than on rendered escape
// sequences: lipgloss resolves the Ascii profile in a test
// binary, so a styled-vs-unstyled byte comparison is vacuous. Emphasis is
// asserted separately, on the style object, in
// TestReadStyle_UnreadEmphasised_ReadPlain.
func TestToRows_UnreadGlyph_PresentOnlyForUnread(t *testing.T) {
	unread := mkNotification("1", "owner/repo", "A", provider.NotificationReasonOther, false, fixedNow)
	read := mkNotification("2", "owner/repo", "B", provider.NotificationReasonOther, true, fixedNow)

	rows := toRows([]provider.Notification{unread, read}, styles.DefaultStyles())

	if !strings.Contains(rows[0][0], unreadGlyph) {
		t.Errorf("unread row's marker cell = %q, want it to contain %q", rows[0][0], unreadGlyph)
	}
	if strings.Contains(rows[1][0], unreadGlyph) {
		t.Errorf("read row's marker cell = %q, want no unread glyph", rows[1][0])
	}
}

// TestReadStyle_UnreadEmphasised_ReadPlain asserts the marker's emphasis on
// the lipgloss.Style object, which no color profile can flatten — the same
// technique, and the same reason, as titleStyle's own test.
//
// It also pins the marker and the title to the *same* named style, so a theme
// change can never emphasise one and not the other.
func TestReadStyle_UnreadEmphasised_ReadPlain(t *testing.T) {
	s := styles.DefaultStyles()
	unread := mkNotification("1", "owner/repo", "A", provider.NotificationReasonOther, false, fixedNow)
	read := mkNotification("2", "owner/repo", "B", provider.NotificationReasonOther, true, fixedNow)

	if !readStyle(unread, s).GetBold() {
		t.Error("readStyle(unread).GetBold() = false, want true — the unread marker carries the unread emphasis")
	}
	if readStyle(read, s).GetBold() {
		t.Error("readStyle(read).GetBold() = true, want false — a read row must not be emphasised")
	}
	if readStyle(unread, s).GetForeground() != titleStyle(unread, s).GetForeground() {
		t.Error("unread marker and unread title resolve to different foregrounds, want the same named style so themes move both together")
	}
}

// hasColumn reports whether cols contains a column with the given title.
func hasColumn(cols []listview.ColumnSpec, title string) bool {
	for _, c := range cols {
		if c.Title == title {
			return true
		}
	}
	return false
}

// ─── Rendering: convention 8 (render through View() after WindowSizeMsg) ────
//
// The recover() guard sits above SetFeed, not just above View(): View returns
// the viewport's already-rendered content, so every table.renderRow call — the
// one place a column/cell divergence actually panics — happens inside
// SetFeed → listview.SetItems → setColumnsAndRows → SetRows → UpdateViewport.

func TestView_RendersAfterWindowSizeMsg_SingleRepo(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// rendered text. lipgloss resolves the Ascii profile in a test
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

	const titleCol = cellTitle
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

// ─── Empty Title renders "—" ─────────────────────────────────────────────────

// TestTitleCell_EmptyTitle_RendersDash pins the dash-before-styling
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

	const titleCol = cellTitle
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

	const updatedCol = cellUpdated
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
	// ScopeDisplay are empty on that one row. The Repo column is
	// unconditional now, so no second row is needed to keep it rendered; the
	// second row is retained anyway so the fixture still covers the mixed
	// case.
	absent := mkNotification("1", "", "Absent repo payload", provider.NotificationReasonOther, false, fixedNow)
	absent.Identity.ScopeDisplay = ""
	items := []provider.Notification{
		absent,
		mkNotification("2", "owner/repo", "Normal", provider.NotificationReasonOther, false, fixedNow),
	}

	rows := toRows(items, s)

	if rows[0][cellRepo] != "—" {
		t.Errorf("empty ScopeDisplay cell = %q, want %q", rows[0][cellRepo], "—")
	}
}

// ─── f reason filter: cycle mechanics ───────────────────────────────────────

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
			t.Fatalf("presentReasons = %v, must never include NotificationReasonUnknown", present)
		}
	}
	if len(present) != 1 || present[0] != provider.NotificationReasonOther {
		t.Fatalf("presentReasons = %v, want [Other] only", present)
	}
}

func TestCycleReasonFilter_EnumOrder_AllPositionReachable(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
	// only three positions visited, but restated explicitly.)
	if m.reasonFilter == provider.NotificationReasonUnknown && m.reasonFilterActive {
		t.Fatal("cycle must never stop on NotificationReasonUnknown")
	}
}

// ─── f filter collapse: dynamic column shrink + cursor survival ─────────────
//
// The shrink direction is what matters: the expand direction passing does not
// prove it. Start multi-repo with the cursor on a non-first row, cycle until
// the feed narrows to a single repo, and assert View() does not panic and the
// cursor/selection survives — still on the *same item*.
//
// This test previously also asserted that the Repo column *disappeared* on
// the collapse; that intent is now inverted: the column is unconditional, and
// the assertion below pins it surviving. A column vanishing as a side effect of
// filtering hid the answer the filter was asked for, and was reported as a bug
// from real use.
//
// The fixture is shaped so that no accidental cursor behaviour can satisfy the
// same-item assertion: ≥2 rows survive the collapse, the pre-filter cursor
// index (2 of 5) is neither zero nor last, and the surviving row's post-filter
// index (1 of 3) is neither zero nor last either. Reset-to-0, clamp-to-last and
// keep-the-saved-index all land on a different row than the identity restore.
func TestCycleReasonFilter_Collapse_RepoColumnSurvives_CursorSurvives(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Row 0: id 1, a/repo1, Subscribed
	// Row 1: id 2, a/repo1, Mentioned
	// Row 2: id 3, a/repo1, Mentioned      <- cursor starts here
	// Row 3: id 4, b/repo2, Subscribed     <- the second scope
	// Row 4: id 5, a/repo1, Mentioned
	//
	// Mentioned precedes Subscribed in enum order, so the first `f` filters to
	// Mentioned: ids 2, 3 and 5 survive, all in a/repo1 — collapsing the feed
	// to a single scope — and the cursor's own item (id 3) lands at index 1.
	feed := []provider.Notification{
		mkNotification("1", "a/repo1", "Subscribed in repo1", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("2", "a/repo1", "Mentioned in repo1 (first)", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "a/repo1", "Mentioned in repo1 (cursor here)", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("4", "b/repo2", "Subscribed in repo2", provider.NotificationReasonSubscribed, false, fixedNow),
		mkNotification("5", "a/repo1", "Mentioned in repo1 (last)", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m = m.SetFeed(feed)

	// Sanity: starts multi-repo, Repo column present.
	if cols := toColumns(m.list.Items()); !hasColumn(cols, "Repo") {
		t.Fatalf("pre-filter columns = %+v, want a Repo column", cols)
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

	// (a) the repo column survives the collapse to a single scope
	cols := toColumns(visible)
	if len(cols) != cellCount {
		t.Fatalf("post-filter columns = %d, want %d (layout is static)", len(cols), cellCount)
	}
	if !hasColumn(cols, "Repo") {
		t.Fatalf("post-filter columns = %+v, want the Repo column to survive the collapse to one scope", cols)
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
	// without the pane's identity-based restore the saved index
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
// half of the cursor-restore behaviour: when the cursor's own row does NOT
// survive the filter there is no identity to restore to, and listview's
// positional clamp is the
// correct behaviour. The saved cursor (index 3) is past the end of the
// 2-row result, so it must clamp to the last row (index 1) — which also pins
// that the clamp is to len(rows)-1 and not to 0.
func TestCycleReasonFilter_Collapse_CursorClamped_ItemDropped(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// direction: SetFeed replaces the whole feed, and a row that
// moved (the merge sorts newest-first, so ties and new arrivals reshuffle
// constantly) must keep the cursor rather than keeping the index.
func TestSetFeed_PreservesSelectedItemAcrossReorder(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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

// ─── f filter: observability ────────────────────────────────────────────────

func TestReasonFilter_Accessor_ReportsCyclePosition(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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

// TestView_ActiveFilter_RendersIndicator_NotBareEmptyInbox: filter to a reason,
// then poll in a feed that has no rows of that reason. Without the indicator the
// pane renders the plain "no notifications" text while a user-set filter is what
// hides every row.
func TestView_ActiveFilter_RendersIndicator_NotBareEmptyInbox(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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

// ─── r now drives a real fetch ──────────────────────────────────────────────

// TestUpdate_RKey_TriggersRealFetch_ShowsLoadingThenResolves: `r` drives a real
// Fetch (fetchNotifications), so pressing `r` legitimately shows the loading
// spinner immediately (listview's own "r" handling turns it on before batching
// config.Fetch()), and resolving the returned cmd must clear it again and land
// the fetch's rows, exactly like the initial Init()-triggered fetch does.
func TestUpdate_RKey_TriggersRealFetch_ShowsLoadingThenResolves(t *testing.T) {
	marker := &fakeMarker{listItems: []provider.Notification{
		mkNotification("1", "owner/repo", "Freshly fetched", provider.NotificationReasonMentioned, false, fixedNow),
	}}
	m := NewModelWithStyles(styles.DefaultStyles(), marker, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("0", "owner/repo", "Stale row before refresh", provider.NotificationReasonMentioned, false, fixedNow),
	})

	updated, cmd := m.Update(keyRune('r'))
	m = updated

	if cmd == nil {
		t.Fatal("want a non-nil cmd from r — listview batches config.Fetch() with the spinner tick")
	}
	if view := m.View(); !strings.Contains(view, "Loading notifications") {
		t.Errorf("immediately after r the pane should show the loading state; view:\n%s", view)
	}

	// Resolve the cmd batch and feed every resulting message back in, the way
	// bubbletea would (tea.Batch's messages arrive independently).
	for _, resolved := range flattenBatch(cmd) {
		m, _ = m.Update(resolved)
	}

	view := m.View()
	if strings.Contains(view, "Loading notifications") {
		t.Errorf("after the fetch resolves the pane must not still show loading; view:\n%s", view)
	}
	if !strings.Contains(view, "Freshly fetched") {
		t.Errorf("after the fetch resolves the pane should show the fetch's rows; view:\n%s", view)
	}
	if strings.Contains(view, "Stale row before refresh") {
		t.Errorf("after the fetch resolves the pane must not still show the pre-refresh row; view:\n%s", view)
	}
	if marker.listCalls != 1 {
		t.Errorf("marker.List call count = %d, want 1", marker.listCalls)
	}
}

// TestUpdate_RKey_NilMarker_ResolvesWithoutStranding pins the nil-marker
// half: the capability-absent/disabled-pane state must not strand the spinner
// either, even though there is no real fetch to perform — fetchNotifications'
// nil-marker branch still resolves synchronously to an empty, error-free result.
func TestUpdate_RKey_NilMarker_ResolvesWithoutStranding(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed(nil)

	updated, cmd := m.Update(keyRune('r'))
	m = updated
	if cmd == nil {
		t.Fatal("want a non-nil cmd from r even with a nil marker")
	}

	for _, resolved := range flattenBatch(cmd) {
		m, _ = m.Update(resolved)
	}

	view := m.View()
	if strings.Contains(view, "Loading notifications") {
		t.Errorf("nil-marker refresh must still resolve and clear loading; view:\n%s", view)
	}
	if m.list.Err() != nil {
		t.Errorf("nil-marker refresh must not surface an error, got %v", m.list.Err())
	}
}

// TestUpdate_RKey_RealFetch_AppliesFilterNotifications pins fetchNotifications'
// call to FilterNotifications (mutation survivor S4): the closure must not
// hand the marker's raw rows straight to the pane, or a repo the config
// excludes would flash back onto screen on every refresh, undoing the
// initial-load filtering.
func TestUpdate_RKey_RealFetch_AppliesFilterNotifications(t *testing.T) {
	marker := &fakeMarker{listItems: []provider.Notification{
		mkNotification("1", "owner/repo", "Keep me", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "acme/excluded", "Filter me out", provider.NotificationReasonMentioned, false, fixedNow),
	}}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			ExcludeRepos: []string{"acme/*"},
		},
	}
	m := NewModelWithStyles(styles.DefaultStyles(), marker, cfg)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	updated, cmd := m.Update(keyRune('r'))
	m = updated
	for _, resolved := range flattenBatch(cmd) {
		m, _ = m.Update(resolved)
	}

	view := m.View()
	if !strings.Contains(view, "Keep me") {
		t.Errorf("view = %q, want the non-excluded row present", view)
	}
	if strings.Contains(view, "Filter me out") {
		t.Errorf("view = %q, must not contain the excluded row: fetchNotifications must apply FilterNotifications to the marker's raw result", view)
	}
}

// TestUpdate_RKey_RealFetch_ForwardsNotifOptsFromConfig pins fetchNotifications'
// call to NotifOptsFromConfig (mutation survivor S5): the closure must derive
// opts from cfg and forward them to marker.List, not call List with a zero
// value — otherwise participating_only/since_days/max_items configuration
// would silently stop reaching the fetch.
func TestUpdate_RKey_RealFetch_ForwardsNotifOptsFromConfig(t *testing.T) {
	marker := &fakeMarker{}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			MaxItems: 7,
			GitHub:   config.NotificationsGitHubConfig{ParticipatingOnly: true},
		},
	}
	m := NewModelWithStyles(styles.DefaultStyles(), marker, cfg)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := m.Update(keyRune('r'))
	for _, resolved := range flattenBatch(cmd) {
		m, _ = m.Update(resolved)
	}

	want := NotifOptsFromConfig(cfg)
	if marker.listOpts != want {
		t.Errorf("marker.List received opts %+v, want %+v (NotifOptsFromConfig(cfg)) — got the zero value instead", marker.listOpts, want)
	}
}

// flattenBatch runs cmd and, if it produced a tea.BatchMsg, runs every
// sub-cmd too, returning every resulting tea.Msg. bubbletea's own runtime
// does this same flattening; tests that assert on the *result* of a batched
// cmd (rather than merely that it is non-nil) need to reproduce it, since
// listview's "r" handling returns tea.Batch(config.Fetch(), spinner.Tick()).
func flattenBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, flattenBatch(sub)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// ─── The vanished-reason branch resets to "all", not to present[0] ───────────

// TestCycleReasonFilter_SelectedReasonVanished_ResetsToAll pins the `idx < 0`
// branch: a poll can replace the feed with one that has no rows of the
// currently selected reason, and the next `f`
// must then return to the "all" position rather than jumping to the first
// present reason — which would silently move the user's filter sideways.
func TestCycleReasonFilter_SelectedReasonVanished_ResetsToAll(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// presses `f`, asserting the cycle does not advance. This pane sets no
// FilterFunc, so listview's search mode is unreachable and the ViewList half of
// the guard is the only testable one.
func TestUpdate_FKey_NoopOutsideListMode(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// today all four *are* the zero value whatever the pane does. HasContextBar
// resolves through listview.Model.config.HasContextBar, which this pane leaves
// nil (always false), and the other three only return non-zero when
// `viewMode == ViewDetail && detail != nil` — unreachable here because the pane
// has no detail view, so its EnterDetail hook returns a nil DetailView.
// Replacing any of the four with a hardcoded zero literal is therefore a genuine
// equivalent today, not an unpinned regression.
//
// What this does pin is the delegation itself: any forwarder that starts
// returning something *other* than its listview counterpart fails, including
// once the pane gains a real detail view and turns these into live, non-zero
// values. The `!= m.list.X()` form is deliberate — comparing against a
// hand-written zero constant would keep passing when listview's own contract
// changes underneath.
func TestChromeForwarders_MatchUnderlyingListview(t *testing.T) {
	base := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
			// IsSearching is deliberately not covered here: FilterFunc is left
			// nil, so listview's search mode is unreachable and both
			// branches return false permanently. It is a confirmed genuine
			// equivalent, and a row asserting false == false would only look
			// like coverage.
		})
	}
}

// ─── The four render states ─────────────────────────────────────────────────
//
// Each state below is asserted by its own test, and every test checks both
// its own state's discriminating substring AND the absence of the other three
// states' discriminating substrings — giving all six pairs mutual
// distinguishability, not merely four positive assertions.

const (
	emptyInboxMarker   = "You're all caught up."
	filterEmptyMarker  = "No notifications match Filter:"
	errorMarker        = "Notifications unavailable:"
	capabilityMarker   = "not supported by this configuration"
	tokenScopeSkeleton = "GitHub token scope required: notifications"
	// expiredBodyMarker is unique to expiredTokenErrorBody: scopeErrorBody
	// never says "Generate a new", and genericErrorBody carries no remedy at all.
	expiredBodyMarker   = "Generate a new classic token"
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

// TestView_EmptyInbox_ReadsAsClear_NotError pins the first state:
// the feed itself has no rows and no `f` filter is active. It must read as
// "you're clear", never as an error, and must not tell the user to press a key
// this pane currently swallows.
func TestView_EmptyInbox_ReadsAsClear_NotError(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed(nil)

	view := m.View()

	if !strings.Contains(view, emptyInboxMarker) {
		t.Errorf("empty-inbox view = %q, want to contain %q", view, emptyInboxMarker)
	}
	if strings.Contains(view, pressRToRefreshText) {
		t.Errorf("empty-inbox view = %q, must not tell the user to press r (r is swallowed)", view)
	}
	assertOtherStatesAbsent(t, view, emptyInboxMarker)
}

// TestView_FilterEmpty_DistinctFromEmptyInbox_NamesActiveFilter pins the second,
// genuinely reachable state: the feed has rows, but the active `f` reason filter
// matches none of them. This must not render as the plain empty-inbox text.
func TestView_FilterEmpty_DistinctFromEmptyInbox_NamesActiveFilter(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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

// TestView_ActiveFilter_GenuinelyEmptyFeed_ReadsAsClear_NotFilterEmpty pins
// the `len(m.feed) > 0` conjunct in View()'s empty-state branch (mutation
// survivor S1): an active `f` reason filter with a feed that has since gone
// genuinely empty (e.g. the last row under any reason was marked done, or a
// poll returned zero rows) must still read as "you're clear", not as "a
// filter is hiding something" — the latter would be misleading since there
// is nothing left for any reason filter to hide.
func TestView_ActiveFilter_GenuinelyEmptyFeed_ReadsAsClear_NotFilterEmpty(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "A mention", provider.NotificationReasonMentioned, false, fixedNow),
	})

	m, _ = m.Update(keyRune('f')) // -> Mentioned, active
	if !m.reasonFilterActive {
		t.Fatalf("precondition: expected reasonFilterActive=true after f, got %v", m.reasonFilterActive)
	}

	// The feed itself goes genuinely empty while the filter stays active.
	m = m.SetFeed(nil)
	if len(m.feed) != 0 {
		t.Fatalf("precondition: expected len(m.feed)=0, got %d", len(m.feed))
	}

	view := m.View()

	if !strings.Contains(view, emptyInboxMarker) {
		t.Errorf("active-filter-empty-feed view = %q, want %q (feed itself has no rows)", view, emptyInboxMarker)
	}
	if strings.Contains(view, filterEmptyMarker) {
		t.Errorf("active-filter-empty-feed view = %q, must not contain %q: a genuinely empty feed has nothing for the filter to hide", view, filterEmptyMarker)
	}
	assertOtherStatesAbsent(t, view, emptyInboxMarker)
}

// TestView_Error_Generic_CarriesNilClientMessage_ButNotScopeBanner pins the
// error state for the generic-failure branch. The error is constructed from a
// real *github.Adapter with no NotificationsClient configured, so the nil-client
// message asserted here is the adapter's actual production string, not a
// hand-typed guess that could drift from it. That error is a plain fmt.Errorf,
// not a *github.APIError, so errors.As in errorBody cannot recover one — this
// generic case must never show the scope banner, since doing so here would
// misleadingly blame a missing scope for what is actually a wiring gap.
//
// The feed is seeded with a row before HandleFetchResult(nil, err) lands, to
// pin that the error state pre-empts the table view rather than rendering
// stale rows underneath it.
func TestView_Error_Generic_CarriesNilClientMessage_ButNotScopeBanner(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}
	var apiErr *github.APIError
	if errors.As(listErr, &apiErr) {
		t.Fatal("precondition: nil-client error must not be a *github.APIError")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Must not render once errored", provider.NotificationReasonMentioned, false, fixedNow),
	})

	m = m.HandleFetchResult(nil, listErr)

	view := m.View()

	if !strings.Contains(view, errorMarker) {
		t.Errorf("error view = %q, want to contain %q", view, errorMarker)
	}
	if !strings.Contains(view, nilClientMsgMarker) {
		t.Errorf("error view = %q, want to fold in the adapter's real nil-client message %q", view, nilClientMsgMarker)
	}
	if strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("error view = %q, must NOT carry the scope banner for a generic/nil-client failure", view)
	}
	if strings.Contains(view, "Must not render once errored") {
		t.Errorf("error view = %q, must not fall through to stale table rows", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestView_Error_ScopeError_WithHeaders_NamesGrantedAndRequiredScopes pins
// the 403-with-headers branch: a classic PAT's response carries
// X-Accepted-OAuth-Scopes/X-OAuth-Scopes, and this render must name both the
// granted and required scopes concretely, plus a classic-token settings URL.
func TestView_Error_ScopeError_WithHeaders_NamesGrantedAndRequiredScopes(t *testing.T) {
	apiErr := &github.APIError{
		StatusCode:     http.StatusForbidden,
		RequiredScopes: "notifications",
		GrantedScopes:  "repo,read:org",
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, apiErr)

	view := m.View()

	if !strings.Contains(view, errorMarker) {
		t.Errorf("scope-error view = %q, want to contain %q", view, errorMarker)
	}
	if !strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("scope-error view = %q, want the scope skeleton naming notifications", view)
	}
	if !strings.Contains(view, "repo,read:org") {
		t.Errorf("scope-error view = %q, want the granted scopes named concretely", view)
	}
	if !strings.Contains(view, classicTokenSettingsURL) {
		t.Errorf("scope-error view = %q, want the classic token settings URL", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestView_Error_ScopeError_Headerless_TellsUserToSwitchToAClassicToken pins
// the other, genuinely reachable 403 shape: a fine-grained PAT never sends
// X-Accepted-OAuth-Scopes/X-OAuth-Scopes at all, so both scope fields are
// empty. Their absence is the diagnosis, not an ambiguity to hedge around —
// GitHub's notifications endpoints accept classic tokens only, so a
// fine-grained token cannot be fixed, it has to be replaced.
//
// The render must NOT send the user to the fine-grained settings page. An
// earlier version did, telling them to "check the token's notifications
// permission" there; no such permission exists, so it sent them hunting
// through a list that could not contain the answer. That was a real shipped
// bug, caught by a user reading the docs and not finding the permission.
func TestView_Error_ScopeError_Headerless_TellsUserToSwitchToAClassicToken(t *testing.T) {
	apiErr := &github.APIError{StatusCode: http.StatusForbidden}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, apiErr)

	view := m.View()

	if !strings.Contains(view, errorMarker) {
		t.Errorf("headerless-scope-error view = %q, want to contain %q", view, errorMarker)
	}
	if !strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("headerless-scope-error view = %q, want the scope skeleton naming notifications", view)
	}
	if !strings.Contains(view, "fine-grained") {
		t.Errorf("headerless-scope-error view = %q, want it to name the likely cause (a fine-grained token)", view)
	}
	if !strings.Contains(view, "classic tokens only") {
		t.Errorf("headerless-scope-error view = %q, want it to state that the notifications API accepts classic tokens only", view)
	}
	if !strings.Contains(view, classicTokenSettingsURL) {
		t.Errorf("headerless-scope-error view = %q, want the classic token settings URL — the only actionable remedy", view)
	}
	if strings.Contains(view, "settings/personal-access-tokens") {
		t.Errorf("headerless-scope-error view = %q, must NOT send the user to the fine-grained settings page: no fine-grained permission enables the notifications API", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestView_Error_ExpiredToken_DistinctFromScopeError_AndGeneric pins the 401
// branch: distinct wording from both the 403 scope error (no scope banner —
// re-adding a scope to a rejected token would not fix anything) and the
// generic branch.
//
// Does NOT assert on "expired or invalid" alone: *github.APIError's own
// Error() text for a 401 already reads "token may be expired or invalid", so
// that phrase survives even through genericErrorBody's plain %v fold-in and
// cannot tell expiredTokenErrorBody's own branch apart from a mutation that
// deletes it and falls through to generic (verified directly: deleting the
// 401 case and re-running this test left it green before this URL assertion
// was added). The token-settings URLs are genuinely unique to
// expiredTokenErrorBody — genericErrorBody never includes them — so those are
// what this test pins.
func TestView_Error_ExpiredToken_DistinctFromScopeError_AndGeneric(t *testing.T) {
	apiErr := &github.APIError{StatusCode: http.StatusUnauthorized}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, apiErr)

	view := m.View()

	if !strings.Contains(view, errorMarker) {
		t.Errorf("expired-token view = %q, want to contain %q", view, errorMarker)
	}
	if !strings.Contains(view, "expired or invalid") {
		t.Errorf("expired-token view = %q, want the expired/invalid wording", view)
	}
	if !strings.Contains(view, classicTokenSettingsURL) {
		t.Errorf("expired-token view = %q, want the classic token settings URL — unique to expiredTokenErrorBody, unlike genericErrorBody", view)
	}
	if !strings.Contains(view, expiredBodyMarker) {
		t.Errorf("expired-token view = %q, want %q — unique to expiredTokenErrorBody, unlike genericErrorBody and scopeErrorBody", view, expiredBodyMarker)
	}
	if strings.Contains(view, "settings/personal-access-tokens") {
		t.Errorf("expired-token view = %q, must NOT offer a fine-grained token: it would leave this pane broken however valid it is elsewhere", view)
	}
	if strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("expired-token view = %q, must NOT carry the scope skeleton — a rejected token is not a scope gap", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestView_Error_ThreeVariants_ArePairwiseDistinguishable restates the
// mutual-distinguishability rule across the three error-body variants:
// the 403-scope render, the 401-expired render, and the generic render must
// each carry content the other two do not, even though all three share
// errorMarker and disableHint's common suffix.
func TestView_Error_ThreeVariants_ArePairwiseDistinguishable(t *testing.T) {
	newModel := func() Model {
		m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
		m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		return m
	}

	scopeView := newModel().HandleFetchResult(nil, &github.APIError{
		StatusCode: http.StatusForbidden, RequiredScopes: "notifications", GrantedScopes: "repo",
	}).View()
	expiredView := newModel().HandleFetchResult(nil, &github.APIError{StatusCode: http.StatusUnauthorized}).View()
	genericView := newModel().HandleFetchResult(nil, errors.New("boom")).View()

	// expiredOnly is deliberately NOT "expired or invalid": *github.APIError's
	// own Error() text for a 401 already contains that phrase, so it would
	// still appear even if expiredTokenErrorBody's own branch were deleted and
	// the 401 fell through to genericErrorBody's plain %v fold-in (a real
	// mutation verified directly to survive against that weaker marker).
	// Nor is it a settings URL: every actionable body now names the classic
	// page and only the classic page, since the notifications API accepts no
	// other token flavor. expiredBodyMarker is the one phrase this body alone
	// carries.
	scopeOnly := tokenScopeSkeleton
	expiredOnly := expiredBodyMarker
	genericMarkerText := "boom"

	views := map[string]string{"scope": scopeView, "expired": expiredView, "generic": genericView}
	uniques := map[string]string{"scope": scopeOnly, "expired": expiredOnly, "generic": genericMarkerText}

	for name, view := range views {
		for otherName, marker := range uniques {
			contains := strings.Contains(view, marker)
			if otherName == name {
				if !contains {
					t.Errorf("%s view = %q, want its own marker %q present", name, view, marker)
				}
				continue
			}
			if contains {
				t.Errorf("%s view = %q, must not contain %s's marker %q", name, view, otherName, marker)
			}
		}
	}
}

// TestView_Error_RateLimited403_DoesNotClaimMissingScope pins the one
// condition in errorBody's 403 branch that no other test exercises: a
// rate-limited 403 must fall through to genericErrorBody, not
// scopeErrorBody. GitHub returns 403 for both "your token lacks the
// notifications scope" and "you have exhausted your rate limit", and only
// the first is fixable by editing the token. Telling a rate-limited user to
// add a scope they already have sends them to rewrite a working token, and
// the pane keeps failing until the window resets regardless.
//
// Dropping `!apiErr.RateLimited` from that case is otherwise a surviving
// mutation: every other 403 test constructs an APIError with RateLimited
// false, so they all keep passing.
func TestView_Error_RateLimited403_DoesNotClaimMissingScope(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, &github.APIError{
		StatusCode:  http.StatusForbidden,
		RateLimited: true,
	})

	view := m.View()
	if strings.Contains(view, tokenScopeSkeleton) {
		t.Errorf("rate-limited 403 view = %q, must not claim a missing scope", view)
	}
	if strings.Contains(view, classicTokenSettingsURL) {
		t.Errorf("rate-limited 403 view = %q, must not send the user to the token settings page", view)
	}
}

// TestView_Error_TakesPriorityOverEmptyInbox_WhenFeedIsEmpty covers the
// ordering gap the row-seeded test above cannot: an errored fetch whose feed
// was never populated at all (items == 0) must still render the error state,
// not the empty-inbox text, even though both share the same
// "len(items) == 0" precondition. View()'s error check must run before its
// items-emptiness check for this to hold.
func TestView_Error_TakesPriorityOverEmptyInbox_WhenFeedIsEmpty(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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

// newDisableTestConfig builds a config.Config backed by a t.TempDir() path
// (convention 17, binding: never call Save() on a config that did not come
// from LoadFrom(<t.TempDir() path>)). preDisabled sets disabled_panes in the
// on-disk YAML before load, letting tests pin that disabling notifications
// appends to whatever is already there rather than replacing it.
func newDisableTestConfig(t *testing.T, preDisabled string) (*config.Config, string) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := "organization: test-org\nprojects:\n  - test-project\npolling_interval: 60\ntheme: dark\n"
	if preDisabled != "" {
		content += "disabled_panes: " + preDisabled + "\n"
	}
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}
	return cfg, configPath
}

// errorPane builds a Model already in the error state, wired to
// cfg, ready to receive disablePaneKey presses.
func errorPane(t *testing.T, cfg *config.Config) Model {
	t.Helper()
	m := NewModelWithStyles(styles.DefaultStyles(), nil, cfg)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.HandleFetchResult(nil, errors.New("boom"))
}

// TestDisableAction_FirstPress_ArmsConfirm_NoWriteYet pins that a single
// disablePaneKey press only arms the confirm overlay — errorBody's own
// error text is replaced by disableConfirmBody, and nothing is written to
// disk yet.
func TestDisableAction_FirstPress_ArmsConfirm_NoWriteYet(t *testing.T) {
	cfg, _ := newDisableTestConfig(t, "")
	m := errorPane(t, cfg)

	m, cmd := m.Update(keyRune('x'))
	if cmd != nil {
		t.Error("arming the confirm must not issue a tea.Cmd")
	}

	view := m.View()
	if !strings.Contains(view, "Press x again to confirm") {
		t.Errorf("view after first x = %q, want the confirm overlay", view)
	}
	if strings.Contains(view, errorMarker) {
		t.Errorf("view after first x = %q, must not still show the plain error body", view)
	}
	if len(cfg.DisabledPanes) != 0 {
		t.Errorf("DisabledPanes = %v after only the first press, want unchanged (no write yet)", cfg.DisabledPanes)
	}
}

// TestDisableAction_AnyOtherKey_CancelsArm_NoWrite pins that pressing any key
// other than disablePaneKey while armed cancels the confirm without writing.
func TestDisableAction_AnyOtherKey_CancelsArm_NoWrite(t *testing.T) {
	cfg, _ := newDisableTestConfig(t, "")
	m := errorPane(t, cfg)

	m, _ = m.Update(keyRune('x'))
	m, cmd := m.Update(keyRune('z'))
	if cmd != nil {
		t.Error("cancelling the arm must not issue a tea.Cmd")
	}

	if m.disableConfirmPending {
		t.Error("disableConfirmPending must be false after a non-x key")
	}
	view := m.View()
	if !strings.Contains(view, errorMarker) {
		t.Errorf("view after cancelling = %q, want back to the plain error body", view)
	}
	if len(cfg.DisabledPanes) != 0 {
		t.Errorf("DisabledPanes = %v after cancelling, want unchanged (no write occurred)", cfg.DisabledPanes)
	}
}

// TestDisableAction_SecondPress_WritesConfig_AppendingToExistingEntries pins
// the confirmed write: disabled_panes gains "notifications" via Config.Save()
// while preserving a pre-existing entry (round-trip preservation must not be
// bypassed by a second write path), verified by reloading the file from disk
// rather than trusting the in-memory struct alone.
func TestDisableAction_SecondPress_WritesConfig_AppendingToExistingEntries(t *testing.T) {
	cfg, configPath := newDisableTestConfig(t, "pipelines")
	m := errorPane(t, cfg)

	m, _ = m.Update(keyRune('x'))
	m, cmd := m.Update(keyRune('x'))
	if cmd != nil {
		t.Error("the confirmed disable must not issue a tea.Cmd")
	}

	if m.disableConfirmPending {
		t.Error("disableConfirmPending must be false after the confirming press")
	}
	if got := m.GetStatusMessage(); !strings.Contains(got, "disabled") {
		t.Errorf("GetStatusMessage() after disabling = %q, want it to say the pane was disabled", got)
	}

	reloaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom after save failed: %v", err)
	}
	if reloaded.IsPaneEnabled("notifications") {
		t.Error("want notifications disabled on disk after the confirmed press")
	}
	if reloaded.IsPaneEnabled("pipelines") {
		t.Error("want the pre-existing pipelines disable preserved, not replaced")
	}
}

// TestDisableAction_AlreadyDisabled_IsIdempotent_NoDuplicateEntry pins that
// confirming disable twice (e.g. a second session after a restart, or a
// stray double press before this pane's construction-time tab list catches
// up) never appends a second "notifications" entry.
func TestDisableAction_AlreadyDisabled_IsIdempotent_NoDuplicateEntry(t *testing.T) {
	cfg, configPath := newDisableTestConfig(t, "notifications")
	m := errorPane(t, cfg)

	m, _ = m.Update(keyRune('x'))
	m, _ = m.Update(keyRune('x'))

	if got := m.GetStatusMessage(); !strings.Contains(got, "already disabled") {
		t.Errorf("GetStatusMessage() = %q, want an already-disabled message", got)
	}

	reloaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}
	count := 0
	for _, p := range reloaded.DisabledPanes {
		if p == "notifications" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("disabled_panes on disk = %v, want exactly one \"notifications\" entry", reloaded.DisabledPanes)
	}
}

// TestDisableAction_SaveFailure_SurfacesVisibly_AndRollsBack pins that a
// Save() failure is never silent: DisabledPanes must roll back to its
// pre-attempt value and the failure must be visible via GetStatusMessage(),
// not look like the write succeeded. Forces the failure by pointing configPath
// at a location Save() cannot write to (a directory, not a file).
func TestDisableAction_SaveFailure_SurfacesVisibly_AndRollsBack(t *testing.T) {
	tmpDir := t.TempDir()
	unwritableDir := filepath.Join(tmpDir, "config.yaml")
	if err := os.Mkdir(unwritableDir, 0755); err != nil {
		t.Fatalf("failed to create directory standing in for the config path: %v", err)
	}
	cfg := config.NewWithPath("test-org", []string{"test-project"}, 60, "dark", unwritableDir)

	m := errorPane(t, cfg)
	m, _ = m.Update(keyRune('x'))
	m, cmd := m.Update(keyRune('x'))
	if cmd != nil {
		t.Error("a failed disable must not issue a tea.Cmd")
	}

	if len(cfg.DisabledPanes) != 0 {
		t.Errorf("DisabledPanes = %v after a failed Save(), want rolled back to empty", cfg.DisabledPanes)
	}
	got := m.GetStatusMessage()
	if !strings.Contains(got, "Failed to disable") {
		t.Errorf("GetStatusMessage() after a failed Save() = %q, want a visible failure message", got)
	}
}

// TestDisableAction_NilConfig_SurfacesVisibly_NeverPanics covers the
// hand-built-Model case (never true in production — app.go always passes its
// own config pointer — but reachable from a test that omits cfg).
func TestDisableAction_NilConfig_SurfacesVisibly_NeverPanics(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, errors.New("boom"))

	m, _ = m.Update(keyRune('x'))
	m, _ = m.Update(keyRune('x'))

	got := m.GetStatusMessage()
	if !strings.Contains(got, "no config available") {
		t.Errorf("GetStatusMessage() with a nil cfg = %q, want a visible explanation", got)
	}
}

// TestDisableAction_UnreachableOutsideErrorState pins that disablePaneKey
// does nothing when the pane is not in an error state — it is reachable only
// from the error render, not globally.
func TestDisableAction_UnreachableOutsideErrorState(t *testing.T) {
	cfg, _ := newDisableTestConfig(t, "")
	m := NewModelWithStyles(styles.DefaultStyles(), nil, cfg)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Row", provider.NotificationReasonMentioned, false, fixedNow),
	})

	m, cmd := m.Update(keyRune('x'))
	if cmd != nil {
		t.Error("x outside the error state must not issue a tea.Cmd")
	}
	if m.disableConfirmPending {
		t.Error("x outside the error state must not arm the confirm")
	}
	if len(cfg.DisabledPanes) != 0 {
		t.Errorf("DisabledPanes = %v, want unchanged", cfg.DisabledPanes)
	}
}

// TestView_CapabilityUnsupported_DistinctFromOtherThreeStates pins the third
// state at the pane level: this state is currently unreachable through the tab
// (see SetCapabilityUnsupported's doc comment), so it is asserted here by
// putting the pane in that state directly, rather than as an app-level test
// that could never fail for the right reason.
func TestView_CapabilityUnsupported_DistinctFromOtherThreeStates(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// makes one failed fetch permanent. The poller is what calls this.
func TestView_SuccessfulFeedAfterError_ClearsErrorState(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// spinner, never the "you're all caught up" text, which would be an outright
// lie while data is still on the way.
//
// The `r` message goes to m.list directly rather than through m.Update: the
// state under test here is listview's loading flag, not the pane's key
// handling.
//
// Known gap: this conjunct does NOT currently protect the *initial* fetch.
// listview.Init sets the spinner visible but never sets m.loading
// (listview.go's Init), so Loading() is false while the first fetch is in
// flight and this pane will render "you're all caught up" during startup. The
// fix is to set loading on the initial fetch, or move this pane off listview's
// flag.
func TestView_Loading_DoesNotClaimCaughtUp(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
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
// order between the first two states. Every other state pair is
// already distinguished by assertOtherStatesAbsent, but capability and error
// are the one pair no other fixture sets *together*, so swapping their two
// checks in View() is otherwise a genuine equivalent that no test can see.
// Capability wins because it describes the configuration, whereas an error
// describes an attempt that configuration should never have made.
func TestView_CapabilityUnsupported_OutranksError(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.HandleFetchResult(nil, listErr)
	m = m.SetCapabilityUnsupported()

	view := m.View()
	if !strings.Contains(view, capabilityMarker) {
		t.Errorf("view with both capability-unsupported and an error = %q, want the capability state to win", view)
	}
	assertOtherStatesAbsent(t, view, capabilityMarker)
}

// ─── u mark-read / d mark-done, optimistic update + rollback ────────────────

// fakeMarker is a test double for provider.NotificationSource. It records every
// MarkRead/MarkDone call it receives — including the exact Identity — and
// returns readErr/doneErr (nil unless a test sets them) so failure/rollback
// paths can be driven deterministically.
type fakeMarker struct {
	readCalls []provider.Identity
	doneCalls []provider.Identity
	readErr   error
	doneErr   error

	// listItems/listErr/listCalls drive fetchNotifications' List call (the
	// r-key/Init path); users that don't exercise it leave them at their zero
	// value (nil, nil, 0).
	listItems []provider.Notification
	listErr   error
	listCalls int
	// listOpts records the opts passed on the most recent List call — pins
	// fetchNotifications' NotifOptsFromConfig(cfg) forwarding (mutation
	// survivor S5).
	listOpts provider.NotifOpts
}

func (f *fakeMarker) List(opts provider.NotifOpts) ([]provider.Notification, error) {
	f.listCalls++
	f.listOpts = opts
	return f.listItems, f.listErr
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
	m := NewModelWithStyles(styles.DefaultStyles(), marker, nil)
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

	// The rollback above must not be silent — a failed u/d used to roll back
	// with nothing telling the user why. GetStatusMessage() is the pane-level
	// half; app_test.go covers the app-level status-bar half.
	got := m.GetStatusMessage()
	if !strings.Contains(got, "Mark read failed") || !strings.Contains(got, "boom") {
		t.Errorf("GetStatusMessage() after failed MarkRead = %q, want it to name the action and the error", got)
	}
}

// TestMarkRead_AlreadyRead_IsOneWay_NoSecondCall is the mutation target for
// "u turned into a toggle": pressing u a second time on a row already marked
// Read (via a prior successful u) must not issue a second MarkRead call and
// must not flip Read back to false. GitHub has no mark-unread endpoint at all,
// so a toggle here would have nothing real to call.
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
// dropping the override rather than re-inserting into m.feed (so the merge
// order is never reproduced by hand).
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

	got := m.GetStatusMessage()
	if !strings.Contains(got, "Mark done failed") || !strings.Contains(got, "boom") {
		t.Errorf("GetStatusMessage() after failed MarkDone = %q, want it to name the action and the error", got)
	}
}

// TestMarkResult_Success_ClearsAnyPriorFailureMessage pins that a success
// clears m.statusMessage rather than leaving an earlier action's failure
// message stuck on screen forever once a later action succeeds.
//
// Deliberately does NOT call HandleFetchResult between the failed mark and
// the successful one. An earlier version did, and could not discriminate the
// mutation it exists to catch: HandleFetchResult resets statusMessage
// unconditionally, so the intervening fetch — not handleMarkResult's own
// `m.statusMessage = ""` — was doing the clearing, and deleting that line
// left this test passing. Retrying straight after the failure is also the
// realistic path: a user who sees "Mark read failed" presses `u` again, they
// do not wait out a poll cycle first.
func TestMarkResult_Success_ClearsAnyPriorFailureMessage(t *testing.T) {
	marker := &fakeMarker{readErr: errors.New("boom")}
	want := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{want})

	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)
	if m.GetStatusMessage() == "" {
		t.Fatal("precondition: want a failure message set after the failed MarkRead")
	}

	marker.readErr = nil
	m, cmd = m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)

	if got := m.GetStatusMessage(); got != "" {
		t.Errorf("GetStatusMessage() after a successful mark = %q, want empty (the retry itself must clear the prior failure, without waiting for a fetch)", got)
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

// TestMarkRead_PollWithinDebounceWindow_DoesNotFlickerBack is the mutation
// target for both "dropping the override for a still-unread polled row" and
// "debounce window forced to zero": a poll landing well inside the 30s window,
// still reporting the row unread, must not un-mark it — the local intent is
// held until the server agrees or the window elapses, whichever comes first.
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
	// reports the row unread — GitHub's read state is eventually consistent.
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
// a short-lived buffer, not a second permanent source of truth.
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

// TestCanTriage_Blocks_UAndD_WhenErrored pins that a pane in the error render
// state swallows u/d rather than acting on whatever stale items
// listview.HandleFetchResult left behind (listview never clears m.items on
// its error path).
func TestCanTriage_Blocks_UAndD_WhenErrored(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil, nil)
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
// the capability-unsupported state must also block u/d.
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

// TestMarkRead_NilMarker_IsNoop pins the nil-safety requirement:
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

// TestMarkDone_CursorLandsOnAConcreteRow pins where the cursor actually ends up
// after `d`, for every position in the list plus the single-row case.
//
// It deliberately does NOT claim to pin the identity-based restore,
// and an earlier version of this test that did was vacuous: after `d` the
// removed row is no longer in Items(), so its only assertion ("the selected id
// is not the removed one") held for every possible cursor value and could not
// fail. Proof: replacing setItemsPreservingSelection's whole `if hadSelection`
// block with a no-op left it passing.
//
// The deeper reason is structural. markDone always removes the row *under the
// cursor*, so the previously selected identity is by construction absent from
// the new slice, FindIndex returns -1, and listview's positional clamp is the
// only mechanism in play — the identity restore is unreachable through `d` and
// cannot be pinned here at all. It is genuinely pinned by
// TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives and
// TestSetFeed_PreservesSelectedItemAcrossReorder, both of which the same
// no-op mutation does fail.
//
// So what this test is for is the clamp's concrete outcomes: a silent change in
// where the cursor jumps after a dismissal is a real regression (the next `d`
// would hit a different row than the user expects), and the single-row case
// pins that an emptied list yields SelectedIndex() == -1 with selectedItem
// reporting ok == false rather than panicking.
func TestMarkDone_CursorLandsOnAConcreteRow(t *testing.T) {
	tests := []struct {
		name    string
		rows    int
		cursor  int
		wantID  string // "" means: expect no selection at all
		wantIdx int
	}{
		{"middle of three, clamp keeps index 1", 3, 1, "3", 1},
		{"first of three, clamp keeps index 0", 3, 0, "2", 0},
		{"last of three, clamp drops to new last", 3, 2, "2", 1},
		{"only row, list becomes empty", 1, 0, "", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			feed := make([]provider.Notification, 0, tc.rows)
			for i := 1; i <= tc.rows; i++ {
				feed = append(feed, mkNotification(
					fmt.Sprintf("%d", i), "owner/repo", fmt.Sprintf("Row %d", i),
					provider.NotificationReasonMentioned, false, fixedNow))
			}
			m := newTriagePane(t, &fakeMarker{}, feed)
			m.list.SetCursor(tc.cursor)

			m, _ = m.Update(keyRune('d'))

			if got := m.list.SelectedIndex(); got != tc.wantIdx {
				t.Errorf("SelectedIndex() = %d, want %d (items now %d)", got, tc.wantIdx, len(m.list.Items()))
			}
			item, ok := m.selectedItem()
			if tc.wantID == "" {
				if ok {
					t.Errorf("selectedItem() returned %+v, want no selection once the list is empty", item)
				}
				return
			}
			if !ok {
				t.Fatalf("want a selection to remain after removing row %d of %d", tc.cursor, tc.rows)
			}
			if item.Identity.ID != tc.wantID {
				t.Errorf("selected id = %q, want %q", item.Identity.ID, tc.wantID)
			}
		})
	}
}

// TestMarkResult_StaleKind_DoesNotRollBackANewerAction pins that a result is
// matched against the override it actually owns, not just its key.
//
// Reachable as `u` then `d` on one row: after the `u` the row is still in
// Items() (with Read=true), and markDone's already-hidden guard only rejects a
// *live* overrideHidden, so the `d` proceeds and overwrites the same map entry
// with {hidden}. If the failing PATCH's result then rolled back by key alone, it
// would delete the entry the `d` owns and un-hide a row whose DELETE is still in
// flight — the user watches a notification they just dismissed reappear, then
// vanish again when the DELETE lands.
func TestMarkResult_StaleKind_DoesNotRollBackANewerAction(t *testing.T) {
	// The PATCH fails, the DELETE succeeds.
	marker := &fakeMarker{readErr: errors.New("patch boom")}
	target := mkNotification("1", "owner/repo", "Two actions", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target})

	m, readCmd := m.Update(keyRune('u'))
	if readCmd == nil {
		t.Fatal("want a cmd from u")
	}
	m, doneCmd := m.Update(keyRune('d'))
	if doneCmd == nil {
		t.Fatal("want a cmd from d — the row is still selectable after u")
	}
	if len(m.list.Items()) != 0 {
		t.Fatalf("precondition: Items() = %+v, want the row hidden by d", m.list.Items())
	}

	// The older, failing PATCH result lands first.
	m, _ = m.Update(readCmd())
	if len(m.list.Items()) != 0 {
		t.Fatalf("Items() = %+v, want the row to stay hidden: the failed u must not roll back the d's override, whose DELETE is still in flight", m.list.Items())
	}

	// Then the DELETE succeeds and the dismissal becomes durable.
	m, _ = m.Update(doneCmd())
	if len(m.list.Items()) != 0 {
		t.Errorf("Items() = %+v, want the row gone after the successful MarkDone", m.list.Items())
	}
	m.now = func() time.Time { return fixedNow.Add(markDebounceWindow + time.Second) }
	m, _ = m.Update(keyRune('f'))
	if len(m.list.Items()) != 0 {
		t.Errorf("Items() = %+v, want the row still gone past the window — the successful d must have been committed to the feed", m.list.Items())
	}
}

// TestMarkResult_DoesNotClearAFailedFetchsErrorState pins that an override-driven
// re-derivation cannot overwrite the error render with "you're all caught up".
//
// listview.SetItems assigns m.err = nil and m.loading = false as a side effect,
// so routing a mark result through it while a fetch has failed silently replaces
// "Notifications unavailable: …" with "No notifications found. / You're all
// caught up." — telling the user their inbox is clear when the fetch failed, and
// hiding the very error that explains why there is no data. That is the lie
// this pane exists to prevent.
//
// This is reachable today: both this pane's own Init()/`r` fetch and
// the app-level poller's push land through HandleFetchResult, either of which
// can fail while a `u`/`d` override is still settling — a poll failing
// mid-debounce, or an unsolicited 304 surfacing as an error (see
// TestMarkResult_DoesNotClearAnUnsolicitedNotModifiedError below for that exact
// shape), reach exactly this path.
func TestMarkResult_DoesNotClearAFailedFetchsErrorState(t *testing.T) {
	marker := &fakeMarker{}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target})

	m, cmd := m.Update(keyRune('d'))
	if cmd == nil {
		t.Fatal("want a cmd from d")
	}

	// A poll fails while the DELETE is still in flight.
	m = m.HandleFetchResult(nil, errors.New("fetch boom"))
	if !strings.Contains(m.View(), errorMarker) {
		t.Fatalf("precondition: want the error render, got:\n%s", m.View())
	}

	// The DELETE result lands.
	m, _ = m.Update(cmd())

	view := m.View()
	if !strings.Contains(view, errorMarker) {
		t.Errorf("after a mark result landed on a failed fetch, the error render is gone; view:\n%s", view)
	}
	if strings.Contains(view, emptyInboxMarker) {
		t.Errorf("after a mark result landed on a failed fetch, the pane claims the inbox is clear — the fetch actually failed; view:\n%s", view)
	}
}

// unsolicitedNotModifiedErr builds the exact error shape
// internal/github/notifications.go's client produces for an *unsolicited* 304
// — one with no matching cached validator, so there is nothing to replay
// transparently and the response surfaces as a failure rather than the
// existing list being left intact. *github.APIError is the concrete type
// errors.As recovers downstream; the pane itself only ever reads it
// as a plain error, which is exactly what this fixture exercises.
func unsolicitedNotModifiedErr() error {
	return fmt.Errorf("github: notifications: %w", &github.APIError{
		StatusCode: http.StatusNotModified,
		Message:    "Not Modified",
	})
}

// TestView_UnsolicitedNotModified_RendersErrorState_NotEmptyInbox widens 304
// handling beyond "a matching-cache 304 leaves the list intact". An
// *unsolicited* 304 reaches this pane exactly like any other failed fetch —
// through HandleFetchResult's error argument — and must render the error state,
// never the empty-inbox text a naive reading of "304 Not Modified" might suggest
// ("nothing changed" is not "no notifications").
func TestView_UnsolicitedNotModified_RendersErrorState_NotEmptyInbox(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Must not render once errored", provider.NotificationReasonMentioned, false, fixedNow),
	})

	m = m.HandleFetchResult(nil, unsolicitedNotModifiedErr())

	view := m.View()
	if !strings.Contains(view, errorMarker) {
		t.Errorf("unsolicited-304 view = %q, want the error state (%q)", view, errorMarker)
	}
	if strings.Contains(view, emptyInboxMarker) {
		t.Errorf("unsolicited-304 view = %q, must not render as an empty inbox — a 304 is a failed fetch, not zero notifications", view)
	}
	assertOtherStatesAbsent(t, view, errorMarker)
}

// TestMarkResult_DoesNotClearAnUnsolicitedNotModifiedError is
// TestMarkResult_DoesNotClearAFailedFetchsErrorState's companion using an
// unsolicited 304's exact error shape rather than a generic fetch error, so
// that specific failure mode has its own direct regression test rather than
// relying on a generic error string to stand in for it.
func TestMarkResult_DoesNotClearAnUnsolicitedNotModifiedError(t *testing.T) {
	marker := &fakeMarker{}
	target := mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{target})

	m, cmd := m.Update(keyRune('d'))
	if cmd == nil {
		t.Fatal("want a cmd from d")
	}

	// An unsolicited 304 surfaces as an error while the DELETE is still in flight.
	m = m.HandleFetchResult(nil, unsolicitedNotModifiedErr())
	if !strings.Contains(m.View(), errorMarker) {
		t.Fatalf("precondition: want the error render, got:\n%s", m.View())
	}

	// The DELETE result lands — successfully. The mark-rollback path
	// (handleMarkResult) must decide purely from res.err (nil here), never
	// from the concurrent 304-shaped fetch error the pane is still displaying,
	// which arrived on an entirely different message type
	// (notificationsFetchMsg / polling.NotificationsFetchedMsg, never
	// MarkResultMsg) and therefore cannot be "read" as this mark's own result.
	m, _ = m.Update(cmd())

	view := m.View()
	if !strings.Contains(view, errorMarker) {
		t.Errorf("after a successful mark landed on an unsolicited-304 fetch error, the error render is gone; view:\n%s", view)
	}
	if strings.Contains(view, emptyInboxMarker) {
		t.Errorf("after a successful mark landed on an unsolicited-304 fetch error, the pane claims the inbox is clear; view:\n%s", view)
	}

	// The commit must still have landed in the held feed (commitOverride),
	// even though refreshItems' guard kept the error rendered on screen: once
	// the fetch error clears, the mark must not have reverted.
	found := false
	for _, n := range m.feed {
		if n.Identity.SameItem(target.Identity) {
			found = true
		}
	}
	if found {
		t.Errorf("feed = %+v, want the marked-done row committed out of the held feed despite the concurrent fetch error", m.feed)
	}
}

// TestSetFeed_PrunesExpiredOverrides_ButKeepsLiveOnes pins prunedOverrides
// against being deleted outright, which the two poll-debounce tests do not
// catch: they kill the *inverted* form (dropping live entries) because that
// resurrects a flicker, but a no-op prunedOverrides is behaviourally invisible
// to them — visibleItems and markDone both skip an expired entry anyway, so
// nothing on screen depends on the map having been swept. The only observable
// is the map itself, hence the direct read of m.overrides (same package).
//
// Absent this, the map grows once per u/d for the life of the session, since
// a *successful* override's entry is kept and only a failure removes one. Both
// halves are asserted: the expired entry goes, the live one stays —
// a prune that clears the map wholesale would pass the first check alone and is
// exactly the mutation that reintroduces the flicker the debounce window exists
// to prevent.
func TestSetFeed_PrunesExpiredOverrides_ButKeepsLiveOnes(t *testing.T) {
	marker := &fakeMarker{}
	old := mkNotification("1", "owner/repo", "stale mark", provider.NotificationReasonReviewRequested, false, fixedNow)
	fresh := mkNotification("2", "owner/repo", "recent mark", provider.NotificationReasonMentioned, false, fixedNow)
	m := newTriagePane(t, marker, []provider.Notification{old, fresh})

	// Mark row 1 read at fixedNow, then row 2 read a full window later, so the
	// first entry is expired and the second is live at the moment SetFeed runs.
	m, cmd := m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)

	m.now = func() time.Time { return fixedNow.Add(markDebounceWindow + time.Second) }
	m.list.SetCursor(1)
	m, cmd = m.Update(keyRune('u'))
	m = runMarkCmd(t, m, cmd)

	if len(m.overrides) != 2 {
		t.Fatalf("precondition: overrides = %d, want 2 (one expired, one live) before SetFeed", len(m.overrides))
	}

	m = m.SetFeed([]provider.Notification{old, fresh})

	if _, ok := m.overrides[keyOf(old.Identity)]; ok {
		t.Errorf("the expired override for %q survived SetFeed — prunedOverrides is not sweeping, so the map grows for the life of the session", old.Identity.ID)
	}
	if _, ok := m.overrides[keyOf(fresh.Identity)]; !ok {
		t.Errorf("the LIVE override for %q was pruned — a poll reporting stale unread will now flicker the row back", fresh.Identity.ID)
	}
}

// ─── o open in browser ──────────────────────────────────────────────────────

// withOpenURLSpy substitutes the package-level openURL seam with a spy that
// records the URL(s) it was called with and returns result, restoring the
// original on test cleanup. Mirrors internal/ui/metrics/list_test.go's own
// seam-substitution pattern — never launches a real browser.
func withOpenURLSpy(t *testing.T, result error) *[]string {
	t.Helper()
	var calls []string
	restore := openURL
	openURL = func(u string) error {
		calls = append(calls, u)
		return result
	}
	t.Cleanup(func() { openURL = restore })
	return &calls
}

// TestOpenInBrowser_OpensSelectedRowsWebURL pins the happy path: `o` issues
// exactly one openURL call, for the selected row's own WebURL, and a
// successful result is silent (reviewer finding): the browser
// window appearing is the feedback, and a persistent "Opened in browser"
// that never cleared was rejected as its own defect during review.
func TestOpenInBrowser_OpensSelectedRowsWebURL(t *testing.T) {
	item := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	item.WebURL = "https://github.com/owner/repo/pull/1"
	m := newTriagePane(t, nil, []provider.Notification{item})

	calls := withOpenURLSpy(t, nil)

	m, cmd := m.Update(keyRune('o'))
	if cmd == nil {
		t.Fatal("want a non-nil tea.Cmd from o")
	}
	m = runMarkCmd(t, m, cmd)

	if len(*calls) != 1 || (*calls)[0] != item.WebURL {
		t.Errorf("openURL calls = %v, want exactly one call with %q", *calls, item.WebURL)
	}
	if got := m.GetStatusMessage(); got != "" {
		t.Errorf("GetStatusMessage() = %q, want \"\" (silent success)", got)
	}
}

// TestOpenInBrowser_SuccessClearsAPriorFailureMessage pins the other half of
// the clearing rule: a successful `o` clears whatever failure text
// a previous `o` attempt left behind, so retrying after a transient browser
// error does not leave stale text on screen once it works.
func TestOpenInBrowser_SuccessClearsAPriorFailureMessage(t *testing.T) {
	item := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	item.WebURL = "https://github.com/owner/repo/pull/1"
	m := newTriagePane(t, nil, []provider.Notification{item})

	withOpenURLSpy(t, fmt.Errorf("no browser configured"))
	m, cmd := m.Update(keyRune('o'))
	m = runMarkCmd(t, m, cmd)
	if got := m.GetStatusMessage(); got == "" {
		t.Fatal("precondition: a failed o must leave a status message")
	}

	// A fresh substitution for the retry: openURL now succeeds.
	withOpenURLSpy(t, nil)
	m, cmd = m.Update(keyRune('o'))
	m = runMarkCmd(t, m, cmd)

	if got := m.GetStatusMessage(); got != "" {
		t.Errorf("GetStatusMessage() after a successful retry = %q, want \"\" (the prior failure must be cleared)", got)
	}
}

// TestOpenInBrowser_FailedOpen_SurfacesStatusMessage pins the failure path:
// openURL erroring must not panic and must surface a message rather than
// silently doing nothing.
func TestOpenInBrowser_FailedOpen_SurfacesStatusMessage(t *testing.T) {
	item := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	item.WebURL = "https://github.com/owner/repo/pull/1"
	m := newTriagePane(t, nil, []provider.Notification{item})

	withOpenURLSpy(t, errors.New("no browser found"))

	m, cmd := m.Update(keyRune('o'))
	m = runMarkCmd(t, m, cmd)

	if got := m.GetStatusMessage(); !strings.Contains(got, "no browser found") {
		t.Errorf("GetStatusMessage() = %q, want it to mention the underlying error", got)
	}
}

// TestOpenInBrowser_EmptyWebURL_DoesNotCallOpenURL is the mutation target for
// "the empty-WebURL guard removed": types.go's WebURL doc comment is explicit
// that "" means nothing to open, so o must not hand it to openURL at all.
func TestOpenInBrowser_EmptyWebURL_DoesNotCallOpenURL(t *testing.T) {
	item := mkNotification("1", "owner/repo", "No URL", provider.NotificationReasonReviewRequested, false, fixedNow)
	// WebURL left "" deliberately — the fallback chain is exhausted.
	m := newTriagePane(t, nil, []provider.Notification{item})

	calls := withOpenURLSpy(t, nil)

	m, cmd := m.Update(keyRune('o'))

	if cmd != nil {
		t.Error("o with an empty WebURL must not issue a tea.Cmd")
	}
	if len(*calls) != 0 {
		t.Errorf("openURL calls = %v, want none for an empty WebURL", *calls)
	}
	if got := m.GetStatusMessage(); got == "" {
		t.Error(`GetStatusMessage() = "", want a message explaining why nothing opened`)
	}
}

// TestOpenInBrowser_NoRows_DoesNotCallOpenURL is the mutation target for "the
// no-selection/empty-feed guard removed": an empty feed leaves canTriage
// (and therefore o) a no-op, mirroring u/d's own TestCanTriage_Blocks_UAndD_WhenNoRows.
func TestOpenInBrowser_NoRows_DoesNotCallOpenURL(t *testing.T) {
	m := newTriagePane(t, nil, nil)

	calls := withOpenURLSpy(t, nil)

	_, cmd := m.Update(keyRune('o'))

	if cmd != nil {
		t.Error("o with no rows must not issue a tea.Cmd")
	}
	if len(*calls) != 0 {
		t.Errorf("openURL calls = %v, want none with no rows", *calls)
	}
}

// TestOpenInBrowser_Blocks_WhenErrored mirrors TestCanTriage_Blocks_UAndD_WhenErrored:
// o is gated by the same canTriage() as u/d, so a pane in the error
// render state must swallow o rather than acting on whatever stale items
// listview.HandleFetchResult left behind — even though selectedItem() itself
// has no opinion about m.list.Err() and would happily return the stale row.
func TestOpenInBrowser_Blocks_WhenErrored(t *testing.T) {
	adapter := github.NewAdapterWithNotifications(nil, nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	item := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	item.WebURL = "url-1"
	m := newTriagePane(t, nil, []provider.Notification{item})
	m = m.HandleFetchResult(nil, listErr)

	calls := withOpenURLSpy(t, nil)

	if _, cmd := m.Update(keyRune('o')); cmd != nil {
		t.Error("o while errored must not issue a tea.Cmd")
	}
	if len(*calls) != 0 {
		t.Errorf("openURL calls = %v, want none while errored", *calls)
	}
}

// TestOpenInBrowser_Blocks_WhenCapabilityUnsupported mirrors
// TestCanTriage_Blocks_UAndD_WhenCapabilityUnsupported: the
// capability-unsupported state must also block o, for the same reason it
// blocks u/d — canTriage is the single shared gate.
func TestOpenInBrowser_Blocks_WhenCapabilityUnsupported(t *testing.T) {
	item := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	item.WebURL = "url-1"
	m := newTriagePane(t, nil, []provider.Notification{item})
	m = m.SetCapabilityUnsupported()

	calls := withOpenURLSpy(t, nil)

	if _, cmd := m.Update(keyRune('o')); cmd != nil {
		t.Error("o while capability-unsupported must not issue a tea.Cmd")
	}
	if len(*calls) != 0 {
		t.Errorf("openURL calls = %v, want none while capability-unsupported", *calls)
	}
}

// TestOpenInBrowser_ActiveFilter_OpensHighlightedRow_NotRawFeedIndex is the
// mutation target for "o reads m.feed instead of the filtered/visible list":
// with the `f` reason filter narrowing the feed and the cursor at a non-zero
// filtered index, o must open the row actually highlighted — not
// m.feed[cursor], which sits at a different row once the raw feed order and
// the filtered order diverge.
func TestOpenInBrowser_ActiveFilter_OpensHighlightedRow_NotRawFeedIndex(t *testing.T) {
	mentioned1 := mkNotification("1", "owner/repo", "Mentioned one", provider.NotificationReasonMentioned, false, fixedNow)
	mentioned1.WebURL = "url-mentioned-1"
	review1 := mkNotification("2", "owner/repo", "Review one", provider.NotificationReasonReviewRequested, false, fixedNow)
	review1.WebURL = "url-review-1"
	review2 := mkNotification("3", "owner/repo", "Review two", provider.NotificationReasonReviewRequested, false, fixedNow)
	review2.WebURL = "url-review-2"
	mentioned2 := mkNotification("4", "owner/repo", "Mentioned two", provider.NotificationReasonMentioned, false, fixedNow)
	mentioned2.WebURL = "url-mentioned-2"

	// Raw feed order deliberately interleaves the two reasons, so the
	// post-filter order (review1, review2) diverges from m.feed's own index
	// order at index 1: m.feed[1] is review1, but the filtered list's index 1
	// is review2.
	m := newTriagePane(t, nil, []provider.Notification{mentioned1, review1, review2, mentioned2})

	m, _ = m.Update(keyRune('f'))
	if !m.reasonFilterActive || m.reasonFilter != provider.NotificationReasonReviewRequested {
		t.Fatalf("precondition: after 1st f, active=%v reason=%v, want ReviewRequested", m.reasonFilterActive, m.reasonFilter)
	}
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("precondition: filtered items = %d, want 2 (review1, review2)", got)
	}

	m.list.SetCursor(1)
	if item, ok := m.selectedItem(); !ok || item.Identity.ID != review2.Identity.ID {
		t.Fatalf("precondition: selected item = %+v, ok=%v, want review2 under the cursor", item, ok)
	}

	calls := withOpenURLSpy(t, nil)

	m, cmd := m.Update(keyRune('o'))
	if cmd == nil {
		t.Fatal("want a non-nil tea.Cmd from o")
	}
	m = runMarkCmd(t, m, cmd)

	if len(*calls) != 1 || (*calls)[0] != review2.WebURL {
		t.Errorf("openURL calls = %v, want exactly one call with %q (the highlighted row, not m.feed[1] = %q)", *calls, review2.WebURL, review1.WebURL)
	}
}

// TestOpenInBrowser_NextFetchClearsAFailureMessage pins the clearing rule's
// other trigger (reviewer finding): "a poll refreshes the feed"
// must clear a stale `o` failure message just as reliably as a subsequent
// successful `o` does, so a message about a browser launch that failed
// minutes ago does not survive an inbox refresh the user has moved on to.
func TestOpenInBrowser_NextFetchClearsAFailureMessage(t *testing.T) {
	item := mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow)
	item.WebURL = "https://github.com/owner/repo/pull/1"
	m := newTriagePane(t, nil, []provider.Notification{item})

	withOpenURLSpy(t, fmt.Errorf("no browser configured"))
	m, cmd := m.Update(keyRune('o'))
	m = runMarkCmd(t, m, cmd)
	if got := m.GetStatusMessage(); got == "" {
		t.Fatal("precondition: a failed o must leave a status message")
	}

	m = m.HandleFetchResult([]provider.Notification{item}, nil)

	if got := m.GetStatusMessage(); got != "" {
		t.Errorf("GetStatusMessage() after a fetch = %q, want \"\" (the next fetch must clear a stale o message)", got)
	}
}

// ─── UnreadCount() footer badge accessor ────────────────────────────────────

// TestUnreadCount_CountsUnreadRowsInFeed pins the base case: three rows, two
// unread, one read — UnreadCount reports 2, not len(feed) and not the read
// count.
func TestUnreadCount_CountsUnreadRowsInFeed(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Unread one", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo", "Unread two", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "owner/repo", "Already read", provider.NotificationReasonSubscribed, true, fixedNow),
	})

	if got := m.UnreadCount(); got != 2 {
		t.Errorf("UnreadCount() = %d, want 2", got)
	}
}

// TestUnreadCount_ZeroWhenEverythingRead pins the hidden-at-zero source: an
// all-read feed reports 0, not len(feed) and not a negative/garbage value.
func TestUnreadCount_ZeroWhenEverythingRead(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Read one", provider.NotificationReasonReviewRequested, true, fixedNow),
		mkNotification("2", "owner/repo", "Read two", provider.NotificationReasonMentioned, true, fixedNow),
	})

	if got := m.UnreadCount(); got != 0 {
		t.Errorf("UnreadCount() = %d, want 0", got)
	}
}

// TestUnreadCount_ZeroOnEmptyFeed covers the zero-row case distinctly from
// the all-read case above, since a mutant returning len(feed) instead of the
// unread subset would still pass an all-read fixture that happens to have
// zero rows, but would fail this one only if it also mishandled an empty
// slice — belt and suspenders alongside the all-read test.
func TestUnreadCount_ZeroOnEmptyFeed(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m = m.SetFeed(nil)

	if got := m.UnreadCount(); got != 0 {
		t.Errorf("UnreadCount() = %d, want 0 for an empty feed", got)
	}
}

// TestUnreadCount_ReflectsOptimisticMarkRead_BeforeAnyPoll is the badge's
// first half: `u` applies the mark synchronously inside Update, so
// UnreadCount must decrement immediately — before the tea.Cmd carrying the
// actual MarkRead call ever runs, let alone before any poll lands. A version
// of UnreadCount that reads m.feed's raw Read field (ignoring
// Model.overrides) would still report the pre-mark count here.
func TestUnreadCount_ReflectsOptimisticMarkRead_BeforeAnyPoll(t *testing.T) {
	marker := &fakeMarker{}
	feed := []provider.Notification{
		mkNotification("1", "owner/repo", "PR one", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo", "PR two", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m := newTriagePane(t, marker, feed)

	if got := m.UnreadCount(); got != 2 {
		t.Fatalf("precondition: UnreadCount() = %d, want 2 before any mark", got)
	}

	m, cmd := m.Update(keyRune('u'))

	if len(marker.readCalls) != 0 {
		t.Fatalf("MarkRead calls = %d before the cmd runs, want 0 (the call happens inside the tea.Cmd)", len(marker.readCalls))
	}
	if got := m.UnreadCount(); got != 1 {
		t.Errorf("UnreadCount() = %d immediately after u (before the API call resolves), want 1 — the optimistic mark must be reflected before any poll", got)
	}

	// Resolving the cmd must not change the count again: it was already
	// applied optimistically.
	m = runMarkCmd(t, m, cmd)
	if got := m.UnreadCount(); got != 1 {
		t.Errorf("UnreadCount() = %d after MarkRead resolves, want still 1", got)
	}
}

// TestUnreadCount_ReflectsOptimisticMarkDone mirrors the mark-read case for
// `d`: a row removed via mark-done must stop counting toward the unread
// total immediately, even though it is still present, unread, in m.feed
// itself (the count applies overrides, not m.feed read raw).
func TestUnreadCount_ReflectsOptimisticMarkDone(t *testing.T) {
	marker := &fakeMarker{}
	feed := []provider.Notification{
		mkNotification("1", "owner/repo", "Done me", provider.NotificationReasonReviewRequested, false, fixedNow),
		mkNotification("2", "owner/repo", "Leave me", provider.NotificationReasonMentioned, false, fixedNow),
	}
	m := newTriagePane(t, marker, feed)

	if got := m.UnreadCount(); got != 2 {
		t.Fatalf("precondition: UnreadCount() = %d, want 2 before any mark", got)
	}

	m, cmd := m.Update(keyRune('d'))

	// Asserted BEFORE the cmd resolves, mirroring the mark-read test above.
	// This ordering is the whole point: commitOverride folds a server-
	// confirmed `d` into m.feed itself, physically removing the row, so an
	// assertion taken after runMarkCmd passes even when UnreadCount ignores
	// the overrides map entirely — the commit does the work the override was
	// supposed to be pinning. Measured: with `for _, n := range m.feed`
	// substituted for `m.applyOverrides(m.feed)`, the post-commit form of
	// this test stayed green while the mark-read one failed.
	if len(marker.doneCalls) != 0 {
		t.Fatalf("MarkDone calls = %d before the cmd runs, want 0 (the call happens inside the tea.Cmd)", len(marker.doneCalls))
	}
	if got := m.UnreadCount(); got != 1 {
		t.Errorf("UnreadCount() = %d immediately after d (before the API call resolves), want 1 — the optimistic mark must be reflected before any poll", got)
	}

	// Resolving the cmd must not change the count again: commitOverride
	// removes the row from m.feed, which the override was already hiding.
	m = runMarkCmd(t, m, cmd)
	if got := m.UnreadCount(); got != 1 {
		t.Errorf("UnreadCount() = %d after MarkDone resolves, want still 1", got)
	}
}

// TestUnreadCount_IgnoresReasonFilter_CountsOverWholeFeed is the badge's
// second half, and the one most likely to regress: cycling `f` to narrow the
// visible rows to a single reason must NOT change UnreadCount. A version
// counting over reasonFiltered() instead of m.feed would drop to the
// per-reason subtotal the moment `f` is pressed, which this test would catch.
func TestUnreadCount_IgnoresReasonFilter_CountsOverWholeFeed(t *testing.T) {
	m := NewModelWithStyles(styles.DefaultStyles(), nil, nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = m.SetFeed([]provider.Notification{
		mkNotification("1", "owner/repo", "Mentioned one", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("2", "owner/repo", "Mentioned two", provider.NotificationReasonMentioned, false, fixedNow),
		mkNotification("3", "owner/repo", "Subscribed noise", provider.NotificationReasonSubscribed, false, fixedNow),
	})

	before := m.UnreadCount()
	if before != 3 {
		t.Fatalf("precondition: UnreadCount() = %d, want 3 before cycling f", before)
	}

	// Cycle f to narrow onto a single reason — the interactive `f` filter,
	// not a config filter.
	m, _ = m.Update(keyRune('f'))
	if _, active := m.ReasonFilter(); !active {
		t.Fatalf("precondition: f did not activate the reason filter")
	}
	// reasonFiltered() must actually have narrowed, or this test proves
	// nothing about UnreadCount ignoring it.
	if got := len(m.reasonFiltered()); got == len(m.feed) {
		t.Fatalf("precondition: reasonFiltered() did not narrow (len=%d), f-cycle setup is broken", got)
	}

	if got := m.UnreadCount(); got != before {
		t.Errorf("UnreadCount() = %d after pressing f, want unchanged %d — the badge must ignore the interactive f cycle and count over the whole (config-filtered) feed", got, before)
	}
}
