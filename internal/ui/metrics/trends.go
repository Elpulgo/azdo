package metrics

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	"github.com/charmbracelet/lipgloss"
)

// minSnapshotDaysForTrends is the threshold below which Trends renders the
// "Insufficient history" fallback instead of an empty grid that pretends to
// know things it doesn't.
const minSnapshotDaysForTrends = 7

// Trends grid column widths.
const (
	trendUserColW = 18
	trendCellW    = 22 // fits "pts:99 wip:9.9⚠" / "stuck:99 cy:99d" comfortably
	trendCellGap  = 2
)

// renderTrends produces the Trends sub-view content. Called by View() when the
// metrics tab is in trends mode.
func (m Model) renderTrends() string {
	snapDays := distinctSnapshotDays(m.snapshots)

	if snapDays < minSnapshotDaysForTrends {
		msg := fmt.Sprintf(
			"Insufficient snapshot history (%d/%d days) — Trends view becomes useful after ~2 sprints. Run backfill by setting configuration parameter runOneShotBackfill for immediate history.",
			snapDays, minSnapshotDaysForTrends,
		)
		if m.styles != nil {
			return m.styles.Muted.Render(msg)
		}
		return msg
	}

	if len(m.sprintWindows) == 0 {
		msg := "No sprints picked. Press T to choose."
		if m.styles != nil {
			return m.styles.Muted.Render(msg)
		}
		return msg
	}

	if len(m.trendRows) == 0 {
		msg := "No data for the selected sprints in the snapshot file."
		if m.styles != nil {
			return m.styles.Muted.Render(msg)
		}
		return msg
	}

	var b strings.Builder

	// Sub-header: tag list + date label
	subhead := fmt.Sprintf("Trends · %d sprints · %d days collected · Updated %s",
		len(m.sprintWindows), snapDays, m.lastUpdatedLabel())
	if m.styles != nil {
		subhead = m.styles.Muted.Render(subhead)
	}
	b.WriteString(subhead)
	b.WriteString("\n\n")

	gap := strings.Repeat(" ", trendCellGap)

	// Column header line 1: sprint tags
	tagLine := padCol("", trendUserColW)
	for _, w := range m.sprintWindows {
		tagLine += gap + padCol(w.Tag, trendCellW)
	}
	// Column header line 2: date ranges
	rangeLine := padCol("", trendUserColW)
	for _, w := range m.sprintWindows {
		rng := fmt.Sprintf("(%s – %s)", w.Start.Format("Jan 2"), w.End.Format("Jan 2"))
		rangeLine += gap + padCol(rng, trendCellW)
	}
	if m.styles != nil {
		tagLine = m.styles.Header.Render(tagLine)
		rangeLine = m.styles.Muted.Render(rangeLine)
	}
	b.WriteString(tagLine)
	b.WriteString("\n")
	b.WriteString(rangeLine)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", lipglossSafeWidth(rangeLine)))
	b.WriteString("\n")

	// Per-user rows, with a blank line between them so the 4-line block per
	// user reads as one group.
	for i, row := range m.trendRows {
		if i > 0 {
			b.WriteString("\n")
		}
		m.appendTrendRow(&b, row.User, row.Cells)
	}

	// Team total row
	if total, ok := computeTeamTotal(m.trendRows); ok {
		b.WriteString(strings.Repeat("─", lipglossSafeWidth(rangeLine)))
		b.WriteString("\n")
		m.appendTrendRow(&b, total.User, total.Cells)
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m Model) appendTrendRow(b *strings.Builder, user string, cells []coremetrics.TrendCell) {
	gap := strings.Repeat(" ", trendCellGap)

	// One line per metric so each value gets its own row — easier to scan than
	// the previous 2x2 layout.
	userCell := padCol(user, trendUserColW)
	if m.styles != nil {
		userCell = m.styles.Header.Render(userCell)
	}
	indent := padCol("", trendUserColW)

	linePts := userCell
	lineWIP := indent
	lineStuck := indent
	lineCy := indent

	for _, c := range cells {
		wipMark := ""
		if c.OverloadedAnyDay {
			wipMark = "⚠"
		}
		ptsText := fmt.Sprintf("pts:%s", fmtPoints(c.Points))
		wipText := fmt.Sprintf("wip:%s%s", fmtFloat1(c.AvgWIP), wipMark)
		stuckText := fmt.Sprintf("stuck:%d", c.StuckCount)
		cyText := fmt.Sprintf("cy:%s", fmtDwell(c.CycleTime))

		linePts += gap + padCol(m.colorPoints(ptsText, c.Points), trendCellW)
		lineWIP += gap + padCol(m.colorWIP(wipText, c), trendCellW)
		lineStuck += gap + padCol(m.colorStuck(stuckText, c.StuckCount), trendCellW)
		lineCy += gap + padCol(m.colorCycle(cyText, c.CycleTime), trendCellW)
	}

	b.WriteString(linePts)
	b.WriteString("\n")
	b.WriteString(lineWIP)
	b.WriteString("\n")
	b.WriteString(lineStuck)
	b.WriteString("\n")
	b.WriteString(lineCy)
	b.WriteString("\n")
}

// colorPoints: green when there's something to celebrate, muted at zero.
func (m Model) colorPoints(s string, pts float64) string {
	if m.styles == nil {
		return s
	}
	if pts > 0 {
		return m.styles.Success.Render(s)
	}
	return m.styles.Muted.Render(s)
}

// colorWIP: yellow if the user was overloaded on any day in the window
// (the ⚠ marker), neutral when in flight, muted at zero.
func (m Model) colorWIP(s string, c coremetrics.TrendCell) string {
	if m.styles == nil {
		return s
	}
	if c.OverloadedAnyDay {
		return m.styles.Warning.Render(s)
	}
	if c.AvgWIP > 0 {
		return m.styles.Value.Render(s)
	}
	return m.styles.Muted.Render(s)
}

// colorStuck: red whenever a single stuck item is in the window, muted at zero.
func (m Model) colorStuck(s string, n int) string {
	if m.styles == nil {
		return s
	}
	if n > 0 {
		return m.styles.Error.Render(s)
	}
	return m.styles.Muted.Render(s)
}

// colorCycle: neutral. No clear universal threshold for "too slow" — we leave
// the value uncoloured rather than guessing.
func (m Model) colorCycle(s string, d time.Duration) string {
	if m.styles == nil {
		return s
	}
	if d > 0 {
		return m.styles.Value.Render(s)
	}
	return m.styles.Muted.Render(s)
}

// computeTeamTotal aggregates per-user cells into a team-total row.
// Points are summed; AvgWIP is averaged across users (mean of means);
// StuckCount summed; CycleTime is the simple mean of users' cycle times.
// Returns ok=false if input is empty.
func computeTeamTotal(rows []coremetrics.TrendRow) (coremetrics.TrendRow, bool) {
	if len(rows) == 0 || len(rows[0].Cells) == 0 {
		return coremetrics.TrendRow{}, false
	}
	nCells := len(rows[0].Cells)
	cells := make([]coremetrics.TrendCell, nCells)
	for c := 0; c < nCells; c++ {
		var sumPts, sumWIP float64
		var sumCy time.Duration
		sumStuck, cyN, overloadedN := 0, 0, 0
		for _, r := range rows {
			sumPts += r.Cells[c].Points
			sumWIP += r.Cells[c].AvgWIP
			sumStuck += r.Cells[c].StuckCount
			if r.Cells[c].CycleTime > 0 {
				sumCy += r.Cells[c].CycleTime
				cyN++
			}
			if r.Cells[c].OverloadedAnyDay {
				overloadedN++
			}
		}
		cells[c] = coremetrics.TrendCell{
			Points:           sumPts,
			AvgWIP:           sumWIP / float64(len(rows)),
			StuckCount:       sumStuck,
			OverloadedAnyDay: overloadedN > 0,
		}
		if cyN > 0 {
			cells[c].CycleTime = sumCy / time.Duration(cyN)
		}
	}
	return coremetrics.TrendRow{User: "Team total", Cells: cells}, true
}

func distinctSnapshotDays(snaps []coremetrics.Snapshot) int {
	set := make(map[string]struct{})
	for _, s := range snaps {
		set[s.TS] = struct{}{}
	}
	return len(set)
}

func fmtFloat1(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
}

// lipglossSafeWidth wraps lipgloss.Width with a cap so an exotic terminal
// can't produce a separator wider than the screen.
func lipglossSafeWidth(s string) int {
	w := lipgloss.Width(s)
	if w > 200 {
		w = 200
	}
	return w
}
