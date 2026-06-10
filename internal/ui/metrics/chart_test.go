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

	// Focus user wraps.
	if m.focusedUser != 0 {
		t.Fatalf("initial focusedUser = %d", m.focusedUser)
	}
	m, _ = m.Update(runeKeyMsg('n'))
	if m.focusedUser != 1 {
		t.Errorf("after n, focusedUser = %d, want 1", m.focusedUser)
	}
	m, _ = m.Update(runeKeyMsg('n'))
	if m.focusedUser != 0 {
		t.Errorf("after wrap, focusedUser = %d, want 0", m.focusedUser)
	}

	// Team-only toggles.
	m, _ = m.Update(runeKeyMsg('a'))
	if !m.showTeamOnly {
		t.Error("a should enable team-only")
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
