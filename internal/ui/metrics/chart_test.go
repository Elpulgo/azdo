package metrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// chartModel returns a metrics model wired into chart mode with three sprints
// and two users, sized for rendering. styled toggles real styles on/off.
func chartModel(styled bool) Model {
	m := makeModel()
	if styled {
		m = NewModelWithStyles(nil, m.config, styles.DefaultStyles())
		m.now = func() time.Time { return fixedNow }
	}

	// Enough distinct snapshot days to clear the history guard.
	for i := 0; i < 10; i++ {
		m.snapshots = append(m.snapshots, coremetrics.Snapshot{
			TS: fmt.Sprintf("2026-05-%02d", i+1),
		})
	}

	day := func(s string) time.Time {
		t, _ := time.Parse("2006-01-02", s)
		return t
	}
	m.sprintWindows = []coremetrics.SprintWindow{
		{Tag: "sprint-41", Start: day("2026-05-01"), End: day("2026-05-07")},
		{Tag: "sprint-42", Start: day("2026-05-08"), End: day("2026-05-14")},
		{Tag: "sprint-43", Start: day("2026-05-15"), End: day("2026-05-21")},
	}
	m.trendRows = []coremetrics.TrendRow{
		{User: "alice", Cells: []coremetrics.TrendCell{
			{Points: 8, AvgWIP: 2, CycleTime: 48 * time.Hour},
			{}, // absent sprint — a gap, not a zero
			{Points: 12, AvgWIP: 3, StuckCount: 1, CycleTime: 72 * time.Hour},
		}},
		{User: "bob", Cells: []coremetrics.TrendCell{
			{Points: 5, AvgWIP: 1},
			{Points: 0, AvgWIP: 1}, // present, real zero
			{Points: 7, AvgWIP: 2, CycleTime: 24 * time.Hour},
		}},
	}
	m.mode = viewTrendsChart
	return m
}

func TestRenderTrendsChart_DoesNotPanicAndShowsMetric(t *testing.T) {
	m := chartModel(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	out := m.View()
	if !strings.Contains(out, coremetrics.MetricPoints.Label()) {
		t.Errorf("chart header missing metric label %q in:\n%s", coremetrics.MetricPoints.Label(), out)
	}
	// Sprint legend should list every tag.
	for _, tag := range []string{"sprint-41", "sprint-42", "sprint-43"} {
		if !strings.Contains(out, tag) {
			t.Errorf("chart output missing sprint tag %q", tag)
		}
	}
}

func TestRenderTrendsChart_EmptyAndSmallWindow(t *testing.T) {
	// No sprints selected → guidance message, no panic.
	m := makeModel()
	m.mode = viewTrendsChart
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if out := m.View(); out == "" {
		t.Error("expected a non-empty guidance message with no data")
	}

	// One sprint → "need at least 2" hint.
	m2 := chartModel(false)
	m2.sprintWindows = m2.sprintWindows[:1]
	m2.trendRows[0].Cells = m2.trendRows[0].Cells[:1]
	m2.trendRows[1].Cells = m2.trendRows[1].Cells[:1]
	m2, _ = m2.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if out := m2.renderTrendsChart(); !strings.Contains(out, "at least 2 sprints") {
		t.Errorf("expected 2-sprint hint, got:\n%s", out)
	}
}

func TestRenderTrendsChart_Bars(t *testing.T) {
	// Grouped bars: every user is always shown (no focus/ghost), so the legend
	// must name each user and the canvas must contain block runes (the bars).
	m := chartModel(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	out := m.renderTrendsChart()
	if !strings.ContainsRune(out, '█') {
		t.Errorf("expected bar (block) runes in chart, got:\n%s", out)
	}
	for _, u := range []string{"alice", "bob"} {
		if !strings.Contains(out, u) {
			t.Errorf("legend missing user %q in:\n%s", u, out)
		}
	}
	// Sprint legend is 1-based, not 0-based.
	if !strings.Contains(out, "1 sprint-41") {
		t.Errorf("expected 1-based sprint legend (\"1 sprint-41\") in:\n%s", out)
	}
	if strings.Contains(out, "0 sprint-41") {
		t.Errorf("sprint legend should not be 0-based in:\n%s", out)
	}
}

func TestChartFocus_CycleKey(t *testing.T) {
	m := chartModel(false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	if m.focusedUser != -1 {
		t.Fatalf("initial focusedUser = %d, want -1 (no focus)", m.focusedUser)
	}

	// Two users (alice, bob): pressing f walks all → 0 → 1 → all.
	m, _ = m.Update(runeKeyMsg('f'))
	if m.focusedUser != 0 {
		t.Errorf("after 1st f, focusedUser = %d, want 0", m.focusedUser)
	}
	m, _ = m.Update(runeKeyMsg('f'))
	if m.focusedUser != 1 {
		t.Errorf("after 2nd f, focusedUser = %d, want 1", m.focusedUser)
	}
	m, _ = m.Update(runeKeyMsg('f'))
	if m.focusedUser != -1 {
		t.Errorf("after 3rd f, focusedUser = %d, want -1 (wrap back to all)", m.focusedUser)
	}

	// The focus key is a no-op outside chart mode.
	live := makeModel()
	live.mode = viewLive
	live, _ = live.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	live, _ = live.Update(runeKeyMsg('f'))
	if live.focusedUser != -1 {
		t.Error("focus key must not act in Live mode")
	}
}

func TestUserLegend_MarksFocusedUser(t *testing.T) {
	m := chartModel(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	series := coremetrics.BuildSeries(m.trendRows, m.chartMetric)

	// No focus → no marker, every user shown plainly.
	if strings.Contains(m.userLegend(series), "▸") {
		t.Errorf("unfocused legend should not contain a focus marker:\n%s", m.userLegend(series))
	}

	// Focus bob (index 1): the marker must appear, and sit on bob (after alice).
	m.focusedUser = 1
	leg := m.userLegend(series)
	if !strings.Contains(leg, "▸") {
		t.Fatalf("focused legend missing marker:\n%s", leg)
	}
	if strings.Index(leg, "▸") < strings.Index(leg, "alice") {
		t.Errorf("focus marker should sit on bob (after alice), got:\n%s", leg)
	}
}

func TestChartHints_IncludeFocus(t *testing.T) {
	m := chartModel(true)
	if !strings.Contains(m.chartHints(), "focus") {
		t.Errorf("chart hints should mention the focus key, got: %q", m.chartHints())
	}
}

func TestMetricsGlossary_InBothViews(t *testing.T) {
	m := chartModel(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	chart := m.renderTrendsChart()
	m.mode = viewTrends
	table := m.renderTrends()

	for name, out := range map[string]string{"chart": chart, "table": table} {
		if !strings.Contains(out, "Legend:") || !strings.Contains(out, "Cycle time") {
			t.Errorf("%s view missing the metric glossary:\n%s", name, out)
		}
	}
}

func TestSetStyles_PreservesState(t *testing.T) {
	m := chartModel(true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.selectedSprints = []string{"sprint-41", "sprint-42", "sprint-43"}
	m.sprintCursor = 2

	snaps, sprints, rows := len(m.snapshots), len(m.sprintWindows), len(m.trendRows)
	mode, cursor, sel := m.mode, m.sprintCursor, len(m.selectedSprints)

	newStyles := styles.DefaultStyles()
	m.SetStyles(newStyles)

	if m.styles != newStyles {
		t.Error("SetStyles did not swap the styles pointer")
	}
	if len(m.snapshots) != snaps || len(m.sprintWindows) != sprints || len(m.trendRows) != rows ||
		m.mode != mode || m.sprintCursor != cursor || len(m.selectedSprints) != sel {
		t.Errorf("SetStyles erased state: snaps %d→%d sprints %d→%d rows %d→%d mode %v→%v cursor %d→%d sel %d→%d",
			snaps, len(m.snapshots), sprints, len(m.sprintWindows), rows, len(m.trendRows),
			mode, m.mode, cursor, m.sprintCursor, sel, len(m.selectedSprints))
	}
	if out := m.View(); !strings.Contains(out, "sprint-41") {
		t.Errorf("after SetStyles the metrics section is blank:\n%s", out)
	}
}

func TestChartGeom_DiscreteAndMonotonic(t *testing.T) {
	g := newChartGeom(60, 12, 3, 20)

	// First sprint pins to the left plot edge, last to the right edge, and all
	// columns are distinct.
	if x := g.xFor(0); x != g.plotLeft {
		t.Errorf("xFor(0) = %d, want plotLeft %d", x, g.plotLeft)
	}
	if x := g.xFor(2); x != g.plotRight {
		t.Errorf("xFor(2) = %d, want plotRight %d", x, g.plotRight)
	}
	if g.xFor(0) == g.xFor(1) || g.xFor(1) == g.xFor(2) {
		t.Errorf("sprint columns not distinct: %d %d %d", g.xFor(0), g.xFor(1), g.xFor(2))
	}

	// Y maps value 0 to the baseline and yMax to the top, monotonically.
	if y := g.yFor(0); y != g.plotBottom {
		t.Errorf("yFor(0) = %d, want plotBottom %d", y, g.plotBottom)
	}
	if y := g.yFor(20); y != g.plotTop {
		t.Errorf("yFor(max) = %d, want plotTop %d", y, g.plotTop)
	}
	if !(g.yFor(20) < g.yFor(10) && g.yFor(10) < g.yFor(0)) {
		t.Errorf("yFor not monotonic: %d %d %d", g.yFor(20), g.yFor(10), g.yFor(0))
	}
}

func TestBarLayout_GroupedNonOverlapping(t *testing.T) {
	// plotLeft..plotRight = 3..62 (width 60), 3 sprints, 2 users.
	spans := barLayout(3, 62, 3, 2)
	if len(spans) != 3 {
		t.Fatalf("want 3 sprint groups, got %d", len(spans))
	}
	var prevX1 int = -1
	for s, group := range spans {
		if len(group) != 2 {
			t.Fatalf("sprint %d: want 2 user bars, got %d", s, len(group))
		}
		for u, span := range group {
			if span.x0 < 3 || span.x1 > 62 {
				t.Errorf("sprint %d user %d span %v out of plot bounds [3,62]", s, u, span)
			}
			if span.x1 < span.x0 {
				t.Errorf("sprint %d user %d inverted span %v", s, u, span)
			}
			// Bars are strictly left-to-right with no overlap across the whole chart.
			if span.x0 <= prevX1 {
				t.Errorf("sprint %d user %d span %v overlaps previous (x1=%d)", s, u, span, prevX1)
			}
			prevX1 = span.x1
		}
	}
}

func TestChartKeys_OnlyActInChartMode(t *testing.T) {
	m := chartModel(false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Metric switch: 'l' advances, 'h' goes back.
	if m.chartMetric != coremetrics.MetricPoints {
		t.Fatalf("initial metric = %v", m.chartMetric)
	}
	m, _ = m.Update(runeKeyMsg('l'))
	if m.chartMetric != coremetrics.MetricAvgWIP {
		t.Errorf("after l, metric = %v, want AvgWIP", m.chartMetric)
	}
	m, _ = m.Update(runeKeyMsg('h'))
	if m.chartMetric != coremetrics.MetricPoints {
		t.Errorf("after h, metric = %v, want Points", m.chartMetric)
	}

	// Sprint cursor clamps at the ends.
	m, _ = m.Update(runeKeyMsg(','))
	if m.sprintCursor != 0 {
		t.Errorf("sprintCursor underflowed to %d", m.sprintCursor)
	}
	m, _ = m.Update(runeKeyMsg('.'))
	m, _ = m.Update(runeKeyMsg('.'))
	m, _ = m.Update(runeKeyMsg('.'))
	if m.sprintCursor != 2 {
		t.Errorf("sprintCursor = %d, want clamped to 2", m.sprintCursor)
	}

	// In Live mode, the same keys are no-ops.
	live := makeModel()
	live.mode = viewLive
	live, _ = live.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	live, _ = live.Update(runeKeyMsg('l'))
	if live.chartMetric != coremetrics.MetricPoints {
		t.Error("chart keys must not mutate state in Live mode")
	}
}
