package metrics

import (
	"fmt"
	"strconv"
	"strings"

	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/wavelinechart"
	"github.com/charmbracelet/lipgloss"
)

// teamSeriesName is the dataset/label used for the aggregated team line. Chosen
// so it can't collide with a real user name.
const teamSeriesName = "Team total"

// minChartWidth / minChartHeight are the smallest canvas we'll attempt to draw.
// Below this the chart is illegible, so we fall back to a hint.
const (
	minChartWidth  = 24
	minChartHeight = 8
	// chartChromeRows is the number of body lines around the chart canvas
	// (header, blanks, sprint legend, readout, hints).
	chartChromeRows = 7
)

// renderTrendsChart produces the Trends chart sub-view: a line chart with the
// selected metric on Y and the chosen sprints on X, one line per user with the
// focused user highlighted and the rest ghosted, plus a Team total line.
func (m Model) renderTrendsChart() string {
	// Reuse the same preconditions as the table view.
	if msg, ok := m.trendsPreamble(); !ok {
		return msg
	}
	if len(m.sprintWindows) < 2 {
		return m.mutedOr("Pick at least 2 sprints (press T) to see a trend.")
	}

	metric := m.chartMetric
	series := coremetrics.BuildSeries(m.trendRows, metric)
	team, hasTeam := computeTeamTotal(m.trendRows)
	var teamSeries []coremetrics.SeriesPoint
	if hasTeam {
		teamSeries = coremetrics.BuildSeries([]coremetrics.TrendRow{team}, metric)[0].Points
	}

	// Y axis bound from the visible data (users + team).
	maxVal := coremetrics.SeriesMax(series)
	if hasTeam {
		if tm := coremetrics.SeriesMax([]coremetrics.Series{{User: teamSeriesName, Points: teamSeries}}); tm > maxVal {
			maxVal = tm
		}
	}
	yMax := coremetrics.NiceCeil(maxVal)

	nSprints := len(m.sprintWindows)
	w, h := m.chartCanvasSize()
	if w < minChartWidth || h < minChartHeight {
		return m.mutedOr("Window too small for the chart — widen the terminal or press v for the table.")
	}

	wlc := wavelinechart.New(w, h,
		wavelinechart.WithXYRange(0, float64(nSprints-1), 0, yMax),
	)
	// Pin the ranges. wavelinechart defaults to auto-range, which would rescale
	// the Y axis to the data and draw fractional, gutter-clipped tick labels.
	wlc.AutoMinX, wlc.AutoMaxX, wlc.AutoMinY, wlc.AutoMaxY = false, false, false, false
	wlc.SetXStep(1)
	wlc.SetYStep(1)
	// ntcharts derives each tick from the value at a pixel row, so labels are
	// rounded to keep them integer-clean (or one decimal for sub-3 ranges) and
	// a bounded, non-clipping width. Repeated values are auto-suppressed.
	yPrec := 0
	if yMax < 3 {
		yPrec = 1
	}
	wlc.XLabelFormatter = func(_ int, v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
	wlc.YLabelFormatter = func(_ int, v float64) string { return strconv.FormatFloat(v, 'f', yPrec, 64) }
	if m.styles != nil {
		axis := lipgloss.NewStyle().Foreground(m.styles.Theme.ForegroundMuted)
		wlc.AxisStyle = axis
		wlc.LabelStyle = axis
	}
	// Recompute the gutter/graph area now that ranges, steps and formatters are
	// set, so the Y-label column is sized to our formatter's widest label.
	wlc.UpdateGraphSizes()

	// Draw order controls z-stacking (later overwrites earlier at a cell):
	// ghosts first, team next, focused user last so it stays on top.
	var ghostNames []string
	focusName := m.focusedUserName()

	if !m.showTeamOnly {
		for _, s := range series {
			if s.User == focusName {
				continue
			}
			style := lipgloss.NewStyle()
			if m.styles != nil {
				style = style.Foreground(m.styles.Theme.ForegroundMuted)
			}
			wlc.SetDataSetStyles(s.User, runes.ThinLineStyle, style)
			plotSeries(&wlc, s.User, s.Points)
			ghostNames = append(ghostNames, s.User)
		}
	}

	drawOrder := append([]string(nil), ghostNames...)

	if hasTeam {
		teamStyle := lipgloss.NewStyle().Bold(true)
		if m.styles != nil {
			teamStyle = teamStyle.Foreground(m.styles.Theme.Accent)
		}
		wlc.SetDataSetStyles(teamSeriesName, runes.ArcLineStyle, teamStyle)
		plotSeries(&wlc, teamSeriesName, teamSeries)
		drawOrder = append(drawOrder, teamSeriesName)
	}

	if !m.showTeamOnly && focusName != "" {
		if fs, ok := seriesFor(series, focusName); ok {
			focusStyle := lipgloss.NewStyle().Bold(true)
			if m.styles != nil {
				focusStyle = focusStyle.Foreground(m.styles.Theme.Primary)
			}
			wlc.SetDataSetStyles(focusName, runes.ArcLineStyle, focusStyle)
			plotSeries(&wlc, focusName, fs.Points)
			drawOrder = append(drawOrder, focusName)
		}
	}

	wlc.DrawDataSets(drawOrder)

	var b strings.Builder
	b.WriteString(m.chartHeader(metric, focusName))
	b.WriteString("\n\n")
	b.WriteString(wlc.View())
	b.WriteString("\n")
	b.WriteString(m.sprintLegend())
	b.WriteString("\n\n")
	b.WriteString(m.chartReadout(metric, focusName, teamSeries, hasTeam))
	b.WriteString("\n\n")
	b.WriteString(m.chartHints())
	return strings.TrimRight(b.String(), "\n")
}

// trendsPreamble mirrors the guard ladder used by the table view (insufficient
// history / no sprints / no data). Returns ok=false plus the message to show.
func (m Model) trendsPreamble() (string, bool) {
	snapDays := distinctSnapshotDays(m.snapshots)
	if snapDays < minSnapshotDaysForTrends {
		return m.mutedOr(fmt.Sprintf(
			"Insufficient snapshot history (%d/%d days) — Trends becomes useful after ~2 sprints.",
			snapDays, minSnapshotDaysForTrends,
		)), false
	}
	if len(m.sprintWindows) == 0 {
		return m.mutedOr("No sprints picked. Press T to choose."), false
	}
	if len(m.trendRows) == 0 {
		return m.mutedOr("No data for the selected sprints in the snapshot file."), false
	}
	return "", true
}

// chartCanvasSize returns the width/height available to the ntcharts canvas,
// reserving rows for the surrounding chrome.
func (m Model) chartCanvasSize() (int, int) {
	w := m.viewport.Width
	if w <= 0 {
		w = m.width
	}
	h := m.viewport.Height
	if h <= 0 {
		h = m.height
	}
	h -= chartChromeRows
	if h > 22 {
		h = 22 // a very tall terminal doesn't need a giant chart
	}
	return w, h
}

// focusedUserName resolves the focused user index to a name, clamped.
func (m Model) focusedUserName() string {
	if len(m.trendRows) == 0 {
		return ""
	}
	idx := m.focusedUser
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.trendRows) {
		idx = len(m.trendRows) - 1
	}
	return m.trendRows[idx].User
}

func (m Model) chartHeader(metric coremetrics.MetricKind, focus string) string {
	left := fmt.Sprintf("Trends · chart · %s", metric.Label())
	if m.showTeamOnly {
		left += " · team only"
	} else if focus != "" {
		left += " · focus: " + focus
	}
	if m.styles != nil {
		return m.styles.Header.Render(left)
	}
	return left
}

// sprintLegend maps each X index to its sprint tag and marks the cursor sprint.
func (m Model) sprintLegend() string {
	parts := make([]string, 0, len(m.sprintWindows))
	for i, w := range m.sprintWindows {
		label := fmt.Sprintf("%d %s", i, w.Tag)
		if i == m.clampSprintCursor() {
			label = "▸" + label + "◂"
			if m.styles != nil {
				label = m.styles.Selected.Render(label)
			}
		} else if m.styles != nil {
			label = m.styles.Muted.Render(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

// chartReadout shows the exact value at the cursor sprint for the focused user
// and the team — the precision the chart itself can't convey.
func (m Model) chartReadout(metric coremetrics.MetricKind, focus string, teamPts []coremetrics.SeriesPoint, hasTeam bool) string {
	idx := m.clampSprintCursor()
	w := m.sprintWindows[idx]
	rng := fmt.Sprintf("%s–%s", w.Start.Format("Jan 2"), w.End.Format("Jan 2"))
	head := fmt.Sprintf("%s (%s)", w.Tag, rng)

	parts := []string{head}
	if focus != "" && !m.showTeamOnly {
		if fs, ok := seriesFor(coremetrics.BuildSeries(m.trendRows, metric), focus); ok {
			parts = append(parts, fmt.Sprintf("%s %s", focus, readoutVal(metric, fs.Points[idx])))
		}
	}
	if hasTeam && idx < len(teamPts) {
		parts = append(parts, fmt.Sprintf("%s %s", teamSeriesName, readoutVal(metric, teamPts[idx])))
	}
	line := strings.Join(parts, "   ")
	if m.styles != nil {
		return m.styles.Value.Render(line)
	}
	return line
}

func (m Model) chartHints() string {
	hint := "h/l metric · ,/. sprint · n/p user · a team-only · v back to table"
	if m.styles != nil {
		return m.styles.Muted.Render(hint)
	}
	return hint
}

// clampSprintCursor keeps the cursor within the selected-sprint range.
func (m Model) clampSprintCursor() int {
	idx := m.sprintCursor
	if idx < 0 {
		return 0
	}
	if idx >= len(m.sprintWindows) {
		return len(m.sprintWindows) - 1
	}
	return idx
}

// mutedOr renders s muted when styles are present, else returns it raw.
func (m Model) mutedOr(s string) string {
	if m.styles != nil {
		return m.styles.Muted.Render(s)
	}
	return s
}

// plotSeries plots only the present points of a series onto the named dataset.
// Gaps (absent sprints / undefined cycle time) are skipped so they don't read
// as a dip to zero.
func plotSeries(wlc *wavelinechart.Model, name string, pts []coremetrics.SeriesPoint) {
	for _, p := range pts {
		if !p.Present {
			continue
		}
		wlc.PlotDataSet(name, canvas.Float64Point{X: float64(p.SprintIndex), Y: p.Value})
	}
}

func seriesFor(series []coremetrics.Series, user string) (coremetrics.Series, bool) {
	for _, s := range series {
		if s.User == user {
			return s, true
		}
	}
	return coremetrics.Series{}, false
}

// readoutVal formats a single point's value for the readout, marking gaps.
func readoutVal(metric coremetrics.MetricKind, p coremetrics.SeriesPoint) string {
	if !p.Present {
		return metric.Short() + ":—"
	}
	switch metric {
	case coremetrics.MetricStuck:
		return fmt.Sprintf("%s:%d", metric.Short(), int(p.Value+0.5))
	case coremetrics.MetricCycle:
		return fmt.Sprintf("%s:%sd", metric.Short(), fmtAxisVal(p.Value))
	default:
		return fmt.Sprintf("%s:%s", metric.Short(), fmtAxisVal(p.Value))
	}
}

// fmtAxisVal formats a float for axis labels / readouts: integers print clean,
// fractions keep one decimal.
func fmtAxisVal(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
