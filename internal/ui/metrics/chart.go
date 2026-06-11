package metrics

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/charmbracelet/lipgloss"
)

// minChartWidth / minChartHeight are the smallest canvas we'll attempt to draw.
// Below this the chart is illegible, so we fall back to a hint.
const (
	minChartWidth  = 24
	minChartHeight = 8
	// chartChromeRows is the number of body lines around the chart canvas
	// (header, blanks, sprint legend, colour key, readout, glossary, hints).
	chartChromeRows = 11
)

// barPalette is the per-user colour cycle (Nord accents): cool, calm, muted
// tones that stay distinguishable on a dark terminal without being harsh;
// indices wrap for teams larger than the palette.
var barPalette = []string{
	"#88c0d0", // frost cyan
	"#a3be8c", // green
	"#ebcb8b", // yellow
	"#bf616a", // red
	"#b48ead", // purple
	"#81a1c1", // blue
	"#d08770", // orange
	"#8fbcbb", // teal
}

// userColor returns the bar colour for the i-th user (wrapping the palette).
func userColor(i int) lipgloss.Color {
	return lipgloss.Color(barPalette[i%len(barPalette)])
}

// focusedIndex returns the highlighted user's index, or -1 when no user is
// focused or the stored index is stale (out of range for nUsers).
func (m Model) focusedIndex(nUsers int) int {
	if m.focusedUser < 0 || m.focusedUser >= nUsers {
		return -1
	}
	return m.focusedUser
}

// userStyle returns the bar/label style for user u given the current focus:
// the focused user (or every user when nothing is focused) keeps its colour;
// the rest are dimmed so a single trend stands out.
func (m Model) userStyle(u, focused int) lipgloss.Style {
	if focused >= 0 && u != focused {
		s := lipgloss.NewStyle()
		if m.styles != nil {
			s = s.Foreground(m.styles.Theme.ForegroundMuted)
		}
		return s
	}
	return lipgloss.NewStyle().Foreground(userColor(u))
}

// canvasPoint aliases the ntcharts canvas point so the rest of this file (and
// its tests) don't need to reference the canvas package directly.
type canvasPoint = canvas.Point

// renderTrendsChart produces the Trends chart sub-view: a grouped bar chart with
// the selected metric on Y and the chosen sprints on X. Each sprint is a cluster
// of bars, one per user, each user a fixed colour, shown for every sprint.
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
	yMax := coremetrics.NiceCeil(coremetrics.SeriesMax(series))

	nSprints := len(m.sprintWindows)
	w, h := m.chartCanvasSize()
	if w < minChartWidth || h < minChartHeight {
		return m.mutedOr("Window too small for the chart — widen the terminal or press v for the table.")
	}

	chart := m.renderBarCanvas(newChartGeom(w, h, nSprints, yMax), series)

	var b strings.Builder
	b.WriteString(m.chartHeader(metric))
	b.WriteString("\n\n")
	b.WriteString(chart)
	b.WriteString("\n")
	b.WriteString(m.sprintLegend())
	b.WriteString("\n")
	b.WriteString(m.userLegend(series))
	b.WriteString("\n\n")
	b.WriteString(m.chartReadout(metric, series))
	b.WriteString("\n\n")
	b.WriteString(m.metricsGlossary())
	b.WriteString("\n")
	b.WriteString(m.chartHints())
	return strings.TrimRight(b.String(), "\n")
}

// chartGeom maps data coordinates (sprint index, metric value) onto canvas cell
// coordinates. (0,0) is the top-left of the canvas; Y increases downward.
type chartGeom struct {
	w, h       int
	gutterW    int // columns reserved for right-aligned Y labels (0..gutterW-1)
	axisX      int // column of the vertical Y axis bar
	plotLeft   int // first plottable column (axisX+1)
	plotRight  int // last plottable column (w-1)
	plotTop    int // row for value == yMax
	plotBottom int // row for value == 0 (also the X axis row)
	n          int // number of sprints
	yMax       float64
}

// newChartGeom computes the layout, sizing the Y-label gutter to the widest tick
// label (top / mid / bottom).
func newChartGeom(w, h, n int, yMax float64) chartGeom {
	gutterW := 1
	for _, v := range []float64{yMax, yMax / 2, 0} {
		if l := len(fmtAxisVal(v)); l > gutterW {
			gutterW = l
		}
	}
	axisX := gutterW
	return chartGeom{
		w:          w,
		h:          h,
		gutterW:    gutterW,
		axisX:      axisX,
		plotLeft:   axisX + 1,
		plotRight:  w - 1,
		plotTop:    0,
		plotBottom: h - 1,
		n:          n,
		yMax:       yMax,
	}
}

// xFor maps a sprint index to a canvas column (the centre of its cluster).
func (g chartGeom) xFor(i int) int {
	if g.n <= 1 {
		return (g.plotLeft + g.plotRight) / 2
	}
	span := g.plotRight - g.plotLeft
	return g.plotLeft + int(math.Round(float64(i)/float64(g.n-1)*float64(span)))
}

// yFor maps a metric value to a canvas row (clamped to the plot area).
func (g chartGeom) yFor(v float64) int {
	rows := g.plotBottom - g.plotTop
	if g.yMax <= 0 || rows <= 0 {
		return g.plotBottom
	}
	frac := v / g.yMax
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return g.plotBottom - int(math.Round(frac*float64(rows)))
}

// barSpan is the inclusive column range [x0,x1] occupied by one user's bar.
type barSpan struct{ x0, x1 int }

// barLayout lays out grouped bars: for each sprint, one bar per user, clustered
// and centred within an evenly divided slot. Returns spans indexed [sprint][user].
func barLayout(plotLeft, plotRight, nSprints, nUsers int) [][]barSpan {
	if nSprints < 1 || nUsers < 1 {
		return nil
	}
	plotW := plotRight - plotLeft + 1
	slotW := plotW / nSprints
	if slotW < 1 {
		slotW = 1
	}
	inner := slotW - 2 // leave a one-column gutter on each side of a cluster
	if inner < nUsers {
		inner = nUsers
	}
	barW := inner / nUsers
	if barW < 1 {
		barW = 1
	}
	groupW := barW * nUsers

	out := make([][]barSpan, nSprints)
	for s := 0; s < nSprints; s++ {
		slotStart := plotLeft + s*slotW
		pad := (slotW - groupW) / 2
		if pad < 0 {
			pad = 0
		}
		gx := slotStart + pad
		group := make([]barSpan, nUsers)
		for u := 0; u < nUsers; u++ {
			x0 := gx + u*barW
			group[u] = barSpan{x0: x0, x1: x0 + barW - 1}
		}
		out[s] = group
	}
	return out
}

// renderBarCanvas draws the axes, Y labels and grouped per-user bars onto a
// fresh canvas and returns its rendered string.
func (m Model) renderBarCanvas(g chartGeom, series []coremetrics.Series) string {
	cv := canvas.New(g.w, g.h)
	cv.Fill(canvas.NewCell(' '))

	axisStyle := lipgloss.NewStyle()
	if m.styles != nil {
		axisStyle = axisStyle.Foreground(m.styles.Theme.ForegroundMuted)
	}

	// Axes first, so bars draw on top.
	for y := g.plotTop; y <= g.plotBottom; y++ {
		cv.SetRuneWithStyle(canvasPoint{X: g.axisX, Y: y}, '│', axisStyle)
	}
	for x := g.axisX; x <= g.plotRight; x++ {
		cv.SetRuneWithStyle(canvasPoint{X: x, Y: g.plotBottom}, '─', axisStyle)
	}
	cv.SetRuneWithStyle(canvasPoint{X: g.axisX, Y: g.plotBottom}, '└', axisStyle)

	m.drawYLabel(&cv, g, g.plotTop, g.yMax, axisStyle)
	m.drawYLabel(&cv, g, (g.plotTop+g.plotBottom)/2, g.yMax/2, axisStyle)
	m.drawYLabel(&cv, g, g.plotBottom, 0, axisStyle)

	m.drawBars(&cv, g, series)
	return cv.View()
}

// drawBars draws one block-rune column per user per sprint. Absent sprints and
// real zeros produce no bar (their values are reported in the readout instead).
func (m Model) drawBars(cv *canvas.Model, g chartGeom, series []coremetrics.Series) {
	nUsers := len(series)
	spans := barLayout(g.plotLeft, g.plotRight, g.n, nUsers)
	rows := float64(g.plotBottom) // cell rows above the axis (0..plotBottom-1)
	focused := m.focusedIndex(nUsers)

	for s := 0; s < g.n; s++ {
		for u := 0; u < nUsers; u++ {
			if s >= len(series[u].Points) {
				continue
			}
			p := series[u].Points[s]
			if !p.Present {
				continue
			}
			frac := 0.0
			if g.yMax > 0 {
				frac = p.Value / g.yMax
			}
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			hCells := frac * rows
			if hCells <= 0 {
				continue
			}
			style := m.userStyle(u, focused)
			span := spans[s][u]
			for x := span.x0; x <= span.x1 && x <= g.plotRight; x++ {
				graph.DrawColumnBottomToTop(cv, canvasPoint{X: x, Y: g.plotBottom - 1}, hCells, style)
			}
		}
	}
}

// drawYLabel right-aligns a tick value within the gutter at the given row.
func (m Model) drawYLabel(cv *canvas.Model, g chartGeom, row int, v float64, style lipgloss.Style) {
	if row < g.plotTop || row > g.plotBottom {
		return
	}
	s := fmtAxisVal(v)
	if len(s) > g.gutterW {
		s = s[:g.gutterW]
	}
	cv.SetStringWithStyle(canvasPoint{X: g.gutterW - len(s), Y: row}, s, style)
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

func (m Model) chartHeader(metric coremetrics.MetricKind) string {
	left := fmt.Sprintf("Trends · chart · %s", metric.Label())
	if m.styles != nil {
		return m.styles.Header.Render(left)
	}
	return left
}

// userLegend is the colour key mapping each user to their bar colour. When a
// user is focused, that entry is marked with ▸ and the rest are dimmed so the
// highlighted trend is easy to pick out.
func (m Model) userLegend(series []coremetrics.Series) string {
	focused := m.focusedIndex(len(series))
	parts := make([]string, 0, len(series))
	for i, s := range series {
		swatch := m.userStyle(i, focused).Render("█")
		entry := swatch + " " + s.User
		switch {
		case i == focused:
			entry = "▸" + entry
			if m.styles != nil {
				entry = m.styles.Selected.Render(entry)
			}
		case focused >= 0 && m.styles != nil:
			entry = m.styles.Muted.Render(entry)
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, "   ")
}

// sprintLegend maps each X index to its sprint tag and marks the cursor sprint.
func (m Model) sprintLegend() string {
	parts := make([]string, 0, len(m.sprintWindows))
	for i, w := range m.sprintWindows {
		label := fmt.Sprintf("%d %s", i+1, w.Tag)
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

// chartReadout shows the exact value at the cursor sprint for every user — the
// precision the bars themselves can't convey.
func (m Model) chartReadout(metric coremetrics.MetricKind, series []coremetrics.Series) string {
	idx := m.clampSprintCursor()
	w := m.sprintWindows[idx]
	head := fmt.Sprintf("%s (%s–%s)", w.Tag, w.Start.Format("Jan 2"), w.End.Format("Jan 2"))
	if m.styles != nil {
		head = m.styles.Value.Render(head)
	}

	focused := m.focusedIndex(len(series))
	parts := []string{head}
	for i, s := range series {
		if idx >= len(s.Points) {
			continue
		}
		chunk := fmt.Sprintf("%s %s", s.User, readoutVal(metric, s.Points[idx]))
		if m.styles != nil {
			chunk = m.userStyle(i, focused).Render(chunk)
		}
		parts = append(parts, chunk)
	}
	return strings.Join(parts, "   ")
}

func (m Model) chartHints() string {
	hint := "h/l metric · ,/. sprint · f focus user · v back to table"
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
