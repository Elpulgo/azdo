package metrics

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/ui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fixedNow returns a deterministic "now" used in tests.
var fixedNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// makeModel constructs a Model with deterministic clock and default thresholds
// suitable for tests.
func makeModel() Model {
	cfg := &config.Config{
		Metrics: config.MetricsConfig{
			Enabled:         true,
			IntervalDays:    14,
			ActiveStaleDays: 3,
			RFTStaleDays:    2,
			WIPLimit:        4,
		},
	}
	m := NewModelWithStyles(nil, cfg, nil)
	m.now = func() time.Time { return fixedNow }
	return m
}

func mkItem(id int, state, user, project string, points float64, changedDaysAgo int, tags ...string) azdevops.WorkItem {
	wi := azdevops.WorkItem{
		ID:                 id,
		ProjectName:        project,
		ProjectDisplayName: project,
	}
	wi.Fields.State = state
	wi.Fields.Title = "item-" + strings.ToLower(state)
	wi.Fields.StoryPoints = points
	wi.Fields.Tags = strings.Join(tags, "; ")
	wi.Fields.StateChangeDate = fixedNow.AddDate(0, 0, -changedDaysAgo)
	if state == "Closed" {
		wi.Fields.ClosedDate = fixedNow.AddDate(0, 0, -changedDaysAgo)
	}
	if user != "" {
		wi.Fields.AssignedTo = &azdevops.Identity{DisplayName: user}
	}
	return wi
}

// TestModel_HandleFetchResult_PopulatesAggregate verifies that after a
// metricsLoadedMsg with items, the model contains aggregated user rows.
func TestModel_HandleFetchResult_PopulatesAggregate(t *testing.T) {
	m := makeModel()

	items := []azdevops.WorkItem{
		mkItem(1, "Active", "Alice", "proj", 3, 5),         // active-stale (5d > 3d)
		mkItem(2, "Ready for Test", "Alice", "proj", 2, 1), // not stale
		mkItem(3, "Closed", "Bob", "proj", 5, 1),           // points in interval
	}

	m, _ = m.Update(metricsLoadedMsg{items: items, fetchedAt: fixedNow})

	if got, want := len(m.userRows), 2; got != want {
		t.Fatalf("userRows: got %d, want %d", got, want)
	}
	if len(m.flags) != 1 {
		t.Fatalf("flags: got %d, want 1", len(m.flags))
	}
	if m.loading {
		t.Errorf("loading should be false after data arrived")
	}
	if !m.lastUpdated.Equal(fixedNow) {
		t.Errorf("lastUpdated = %v, want %v", m.lastUpdated, fixedNow)
	}
}

// TestModel_TagFilter_ReAggregates verifies the tag-filter path re-runs the
// pure aggregate against a filtered subset — no fetch involved.
func TestModel_TagFilter_ReAggregates(t *testing.T) {
	m := makeModel()

	items := []azdevops.WorkItem{
		mkItem(1, "Active", "Alice", "proj", 1, 1, "sprint-42"),
		mkItem(2, "Active", "Bob", "proj", 1, 1), // no tag
	}
	m, _ = m.Update(metricsLoadedMsg{items: items, fetchedAt: fixedNow})

	if len(m.userRows) != 2 {
		t.Fatalf("pre-filter rows = %d, want 2", len(m.userRows))
	}

	m, _ = m.Update(components.TagSelectedMsg{Tag: "sprint-42"})

	if got := m.activeTag; got != "sprint-42" {
		t.Errorf("activeTag = %q, want sprint-42", got)
	}
	if len(m.userRows) != 1 {
		t.Fatalf("post-filter rows = %d, want 1", len(m.userRows))
	}
	if m.userRows[0].User != "Alice" {
		t.Errorf("user = %q, want Alice", m.userRows[0].User)
	}
}

// TestModel_TagFilter_Clear restores the unfiltered aggregate when an empty
// tag is selected.
func TestModel_TagFilter_Clear(t *testing.T) {
	m := makeModel()
	items := []azdevops.WorkItem{
		mkItem(1, "Active", "Alice", "proj", 1, 1, "sprint-42"),
		mkItem(2, "Active", "Bob", "proj", 1, 1),
	}
	m, _ = m.Update(metricsLoadedMsg{items: items, fetchedAt: fixedNow})
	m, _ = m.Update(components.TagSelectedMsg{Tag: "sprint-42"})
	m, _ = m.Update(components.TagSelectedMsg{Tag: ""})

	if m.activeTag != "" {
		t.Errorf("activeTag = %q, want empty", m.activeTag)
	}
	if len(m.userRows) != 2 {
		t.Errorf("rows after clear = %d, want 2", len(m.userRows))
	}
}

// TestModel_FlagFilter_Cycles verifies pressing "f" cycles through
// All -> Active-stale -> RFT-stale -> All.
func TestModel_FlagFilter_Cycles(t *testing.T) {
	m := makeModel()

	if m.flagFilter != flagFilterAll {
		t.Fatalf("initial flagFilter = %v, want %v", m.flagFilter, flagFilterAll)
	}

	m, _ = m.Update(runeKeyMsg('f'))
	if m.flagFilter != flagFilterActiveStale {
		t.Errorf("after 1 cycle = %v, want %v", m.flagFilter, flagFilterActiveStale)
	}

	m, _ = m.Update(runeKeyMsg('f'))
	if m.flagFilter != flagFilterRFTStale {
		t.Errorf("after 2 cycles = %v, want %v", m.flagFilter, flagFilterRFTStale)
	}

	m, _ = m.Update(runeKeyMsg('f'))
	if m.flagFilter != flagFilterAll {
		t.Errorf("after 3 cycles = %v, want %v", m.flagFilter, flagFilterAll)
	}
}

// TestModel_FlagFilter_DropsNonMatching verifies the visible flags slice
// respects the active filter without affecting underlying `flags`.
func TestModel_FlagFilter_DropsNonMatching(t *testing.T) {
	m := makeModel()
	items := []azdevops.WorkItem{
		mkItem(1, "Active", "Alice", "proj", 1, 5),        // active-stale
		mkItem(2, "Ready for Test", "Bob", "proj", 1, 10), // rft-stale
	}
	m, _ = m.Update(metricsLoadedMsg{items: items, fetchedAt: fixedNow})

	if len(m.flags) != 2 {
		t.Fatalf("flags = %d, want 2", len(m.flags))
	}

	// Cycle to Active-stale only.
	m, _ = m.Update(runeKeyMsg('f'))
	if vis := m.visibleFlags(); len(vis) != 1 || vis[0].Reason != "active-stale" {
		t.Errorf("active-stale visible = %v", vis)
	}

	// Cycle to RFT-stale only.
	m, _ = m.Update(runeKeyMsg('f'))
	if vis := m.visibleFlags(); len(vis) != 1 || vis[0].Reason != "rft-stale" {
		t.Errorf("rft-stale visible = %v", vis)
	}

	// Back to All.
	m, _ = m.Update(runeKeyMsg('f'))
	if vis := m.visibleFlags(); len(vis) != 2 {
		t.Errorf("all visible = %d, want 2", len(vis))
	}
}

// TestModel_PaneFocus_TogglesOnTab verifies Tab switches focus between flags
// and user table.
func TestModel_PaneFocus_TogglesOnTab(t *testing.T) {
	m := makeModel()
	if m.focusedPane != paneFlags {
		t.Fatalf("initial focus = %v, want %v", m.focusedPane, paneFlags)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedPane != paneUsers {
		t.Errorf("after tab = %v, want %v", m.focusedPane, paneUsers)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedPane != paneFlags {
		t.Errorf("after 2 tabs = %v, want %v", m.focusedPane, paneFlags)
	}
}

// TestModel_HandleFetchResult_PartialError surfaces a status message but
// keeps the data.
func TestModel_HandleFetchResult_PartialError(t *testing.T) {
	m := makeModel()
	items := []azdevops.WorkItem{mkItem(1, "Active", "Alice", "proj", 1, 1)}
	pe := &azdevops.PartialError{Failed: 1, Total: 3}

	m, _ = m.Update(metricsLoadedMsg{items: items, err: pe, fetchedAt: fixedNow})

	if len(m.userRows) != 1 {
		t.Errorf("rows = %d, want 1 even with partial error", len(m.userRows))
	}
	msg := m.GetStatusMessage()
	if !strings.Contains(strings.ToLower(msg), "partial") &&
		!strings.Contains(msg, "1 of 3") {
		t.Errorf("status message = %q, expected a partial-data hint", msg)
	}
	var partialErr *azdevops.PartialError
	if !errors.As(pe, &partialErr) {
		t.Fatal("PartialError type assertion failed (sanity check)")
	}
}

// TestModel_HandleFetchResult_FatalError clears items and surfaces an error
// message.
func TestModel_HandleFetchResult_FatalError(t *testing.T) {
	m := makeModel()
	m, _ = m.Update(metricsLoadedMsg{err: errors.New("boom"), fetchedAt: fixedNow})

	if len(m.userRows) != 0 {
		t.Errorf("rows = %d, want 0 on fatal error", len(m.userRows))
	}
	if m.GetStatusMessage() == "" {
		t.Errorf("expected non-empty status message on fatal error")
	}
}

// TestModel_View_RendersHeaderInfo checks the rendered output contains
// interval, thresholds, and tag context — the spec mandates the header is
// the only place filter state is announced.
func TestModel_View_RendersHeaderInfo(t *testing.T) {
	m := makeModel()
	m, _ = m.Update(metricsLoadedMsg{items: nil, fetchedAt: fixedNow})
	m.width, m.height = 120, 30

	out := m.View()
	if !strings.Contains(out, "14d") {
		t.Errorf("header missing interval marker. Output:\n%s", out)
	}
	if !strings.Contains(out, "3d") {
		t.Errorf("header missing active-stale threshold. Output:\n%s", out)
	}
	if !strings.Contains(out, "2d") {
		t.Errorf("header missing rft-stale threshold. Output:\n%s", out)
	}
}

// runeKeyMsg constructs a tea.KeyMsg for a single-character rune key.
func runeKeyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// makeManyUserItems generates `n` Active items each assigned to a unique user
// so the resulting per-user table overflows a small viewport.
func makeManyUserItems(n int) []azdevops.WorkItem {
	items := make([]azdevops.WorkItem, n)
	for i := 0; i < n; i++ {
		items[i] = mkItem(1000+i, "Active", "user"+strconv.Itoa(i), "proj", 1, 1)
	}
	return items
}

// TestModel_RenderFlagsPane_AlignsColumns verifies the stuck-items rows
// stay column-aligned even when work item IDs differ in width (e.g. 4 vs
// 7 digits) and users/projects have very different name lengths.
func TestModel_RenderFlagsPane_AlignsColumns(t *testing.T) {
	m := makeModel()
	items := []azdevops.WorkItem{
		mkItem(6887, "Active", "Al", "proj", 1, 5),
		mkItem(101054, "Active", "Veronica-Longname", "very-long-proj", 1, 7),
		mkItem(42, "Ready for Test", "Bob", "p", 1, 5),
	}
	m, _ = m.Update(metricsLoadedMsg{items: items, fetchedAt: fixedNow})

	out := m.renderFlagsPane()
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected title + 3 rows, got %d lines:\n%s", len(lines), out)
	}
	// All data rows (index 1..3) should have identical display widths.
	wantWidth := lipgloss.Width(lines[1])
	for i, line := range lines[1:] {
		if got := lipgloss.Width(line); got != wantWidth {
			t.Errorf("row %d width = %d, want %d (matched row 0). Row:\n%q", i+1, got, wantWidth, line)
		}
	}
}

// TestModel_RenderUsersPane_AlignsColumns verifies the per-user rows align
// across users with mixed name lengths and Overloaded flags.
func TestModel_RenderUsersPane_AlignsColumns(t *testing.T) {
	m := makeModel()
	items := []azdevops.WorkItem{
		// Alice: 5 in-flight (overloaded), some stale
		mkItem(1, "Active", "Alice", "proj", 1, 5),
		mkItem(2, "Active", "Alice", "proj", 1, 1),
		mkItem(3, "Active", "Alice", "proj", 1, 1),
		mkItem(4, "Active", "Alice", "proj", 1, 1),
		mkItem(5, "Ready for Test", "Alice", "proj", 1, 1),
		// Bob: short row
		mkItem(6, "Active", "Bob", "proj", 1, 1),
		// Veronica-Longname: tests truncation
		mkItem(7, "Active", "Veronica-Longname-Person", "proj", 1, 5),
	}
	m, _ = m.Update(metricsLoadedMsg{items: items, fetchedAt: fixedNow})

	out := m.renderUsersPane()
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected title + header + 3 rows, got %d:\n%s", len(lines), out)
	}
	// All data rows (skip title at 0 and column header at 1) should share width.
	wantWidth := lipgloss.Width(lines[2])
	for i, line := range lines[2:] {
		if got := lipgloss.Width(line); got != wantWidth {
			t.Errorf("user row %d width = %d, want %d. Row:\n%q", i+2, got, wantWidth, line)
		}
	}
}

// TestModel_Viewport_ScrollsOnPageDown verifies that after loading enough
// content to overflow a small viewport, pressing PgDown moves YOffset
// downward.
func TestModel_Viewport_ScrollsOnPageDown(t *testing.T) {
	m := makeModel()
	// Force a small viewport so 20 users overflow.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m, _ = m.Update(metricsLoadedMsg{items: makeManyUserItems(20), fetchedAt: fixedNow})

	before := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	after := m.viewport.YOffset

	if after <= before {
		t.Errorf("PgDown did not scroll: YOffset before=%d, after=%d", before, after)
	}
}

// TestModel_Viewport_CursorAutoScrolls verifies that pressing ↓ enough times
// to walk the cursor past the visible viewport area triggers auto-scroll.
func TestModel_Viewport_CursorAutoScrolls(t *testing.T) {
	m := makeModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m, _ = m.Update(metricsLoadedMsg{items: makeManyUserItems(20), fetchedAt: fixedNow})

	// Move focus to users pane, then walk down.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	startOffset := m.viewport.YOffset
	for i := 0; i < 19; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	if m.viewport.YOffset <= startOffset {
		t.Errorf("walking cursor down did not scroll viewport: offset stayed at %d", m.viewport.YOffset)
	}
	if m.userCursor != 19 {
		t.Errorf("userCursor = %d, want 19", m.userCursor)
	}
}

// TestModel_VToggle_SwitchesMode ensures pressing 'v' cycles the metrics tab
// through Live → Trends (table) → Trends (chart) → Live.
func TestModel_VToggle_SwitchesMode(t *testing.T) {
	m := makeModel()
	if m.mode != viewLive {
		t.Fatalf("initial mode = %v, want viewLive", m.mode)
	}
	m, _ = m.Update(runeKeyMsg('v'))
	if m.mode != viewTrends {
		t.Errorf("after 1st v = %v, want viewTrends", m.mode)
	}
	m, _ = m.Update(runeKeyMsg('v'))
	if m.mode != viewTrendsChart {
		t.Errorf("after 2nd v = %v, want viewTrendsChart", m.mode)
	}
	m, _ = m.Update(runeKeyMsg('v'))
	if m.mode != viewLive {
		t.Errorf("after 3rd v = %v, want viewLive", m.mode)
	}
}

// TestModel_TagsSelectedMsg_UpdatesSelection feeds a TagsSelectedMsg and
// verifies the model adopts the selection and recomputes trends.
func TestModel_TagsSelectedMsg_UpdatesSelection(t *testing.T) {
	m := makeModel()
	m, _ = m.Update(components.TagsSelectedMsg{Tags: []string{"sprint-42"}})
	if len(m.selectedSprints) != 1 || m.selectedSprints[0] != "sprint-42" {
		t.Errorf("selectedSprints = %v, want [sprint-42]", m.selectedSprints)
	}
}

// TestModel_BackfillDoneMsg_SuccessSetsStatus verifies the status footer
// communicates the backfill result and includes the disable-hint.
func TestModel_BackfillDoneMsg_SuccessSetsStatus(t *testing.T) {
	m := makeModel()
	m, _ = m.Update(backfillDoneMsg{total: 47, saved: 1234, skipped: 2})
	if !strings.Contains(m.statusMessage, "1234") {
		t.Errorf("statusMessage = %q, want it to mention saved=1234", m.statusMessage)
	}
	if !strings.Contains(m.statusMessage, "run_one_shot_backfill") {
		t.Errorf("statusMessage = %q, want it to hint about disabling run_one_shot_backfill", m.statusMessage)
	}
}

// TestModel_BackfillDoneMsg_ErrorSurfaced verifies a failure is shown to the
// user (so they know to retry / investigate).
func TestModel_BackfillDoneMsg_ErrorSurfaced(t *testing.T) {
	m := makeModel()
	m, _ = m.Update(backfillDoneMsg{err: errors.New("HTTP 503")})
	if !strings.Contains(m.statusMessage, "503") {
		t.Errorf("statusMessage = %q, want it to surface the error", m.statusMessage)
	}
}

// TestModel_BackfillDoneMsg_AlreadyDoneIsQuiet keeps the footer clean on the
// common case (backfill ran previously, marker is present).
func TestModel_BackfillDoneMsg_AlreadyDoneIsQuiet(t *testing.T) {
	m := makeModel()
	m.statusMessage = ""
	m, _ = m.Update(backfillDoneMsg{alreadyDone: true})
	if m.statusMessage != "" {
		t.Errorf("statusMessage = %q, want empty for alreadyDone", m.statusMessage)
	}
}

