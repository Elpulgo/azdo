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

	// Per-user rows
	for _, row := range m.trendRows {
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

	line1 := padCol(user, trendUserColW)
	line2 := padCol("", trendUserColW)
	for _, c := range cells {
		wipMark := ""
		if c.OverloadedAnyDay {
			wipMark = "⚠"
		}
		cellL1 := fmt.Sprintf("pts:%s  wip:%s%s",
			fmtPoints(c.Points), fmtFloat1(c.AvgWIP), wipMark)
		cellL2 := fmt.Sprintf("stuck:%d  cy:%s",
			c.StuckCount, fmtDwell(c.CycleTime))
		line1 += gap + padCol(cellL1, trendCellW)
		line2 += gap + padCol(cellL2, trendCellW)
	}
	if m.styles != nil {
		line1 = m.styles.Value.Render(line1)
		line2 = m.styles.Muted.Render(line2)
	}
	b.WriteString(line1)
	b.WriteString("\n")
	b.WriteString(line2)
	b.WriteString("\n")
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
