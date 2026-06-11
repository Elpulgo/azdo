package metrics

import "math"

// MetricKind identifies one of the four Trends metrics rendered on the Y axis
// of the chart view. The numeric order matches the textual Trends table
// (points, wip, stuck, cycle).
type MetricKind int

const (
	MetricPoints MetricKind = iota
	MetricAvgWIP
	MetricStuck
	MetricCycle
)

// AllMetricKinds lists the metrics in display order, used to cycle through them.
var AllMetricKinds = []MetricKind{MetricPoints, MetricAvgWIP, MetricStuck, MetricCycle}

// Label is the human-readable metric name shown in the chart header.
func (k MetricKind) Label() string {
	switch k {
	case MetricPoints:
		return "Points closed"
	case MetricAvgWIP:
		return "Avg WIP"
	case MetricStuck:
		return "Stuck items"
	case MetricCycle:
		return "Cycle time (days)"
	default:
		return "?"
	}
}

// Short is the compact metric tag used in legends/readouts.
func (k MetricKind) Short() string {
	switch k {
	case MetricPoints:
		return "pts"
	case MetricAvgWIP:
		return "wip"
	case MetricStuck:
		return "stuck"
	case MetricCycle:
		return "cy"
	default:
		return "?"
	}
}

// CellValue extracts the numeric value for metric k from a cell. Cycle time is
// converted to days so it shares the Float64 plotting path with the others.
func CellValue(c TrendCell, k MetricKind) float64 {
	switch k {
	case MetricPoints:
		return c.Points
	case MetricAvgWIP:
		return c.AvgWIP
	case MetricStuck:
		return float64(c.StuckCount)
	case MetricCycle:
		return c.CycleTime.Hours() / 24
	default:
		return 0
	}
}

// CellActive reports whether the user had any signal at all in this sprint.
// TrendAggregate leaves a zero-valued TrendCell for a user with no rows in a
// window, so "all four zero" is the proxy for "absent that sprint". This lets
// the chart distinguish an absent user (gap) from a present user who simply
// closed nothing (a real zero).
func CellActive(c TrendCell) bool {
	return c.Points > 0 || c.AvgWIP > 0 || c.StuckCount > 0 || c.CycleTime > 0
}

// CellHasValue reports whether metric k should be plotted for this cell.
//   - Cycle time requires an actual measurement (CycleTime > 0); a sprint with
//     no completed items is a gap, never a misleading 0-day point.
//   - Every other metric plots whenever the user was active, so a present user
//     who closed nothing shows as a real zero rather than a gap.
func CellHasValue(c TrendCell, k MetricKind) bool {
	if k == MetricCycle {
		return c.CycleTime > 0
	}
	return CellActive(c)
}

// SeriesPoint is one (sprint, value) sample for a user's line. Present=false
// marks a gap that callers must not plot (and must not treat as zero).
type SeriesPoint struct {
	SprintIndex int
	Value       float64
	Present     bool
}

// Series is one user's line across the selected sprints, in sprint order.
type Series struct {
	User   string
	Points []SeriesPoint
}

// BuildSeries projects per-user trend rows onto a single metric, one
// SeriesPoint per sprint column with its presence flag resolved.
func BuildSeries(rows []TrendRow, k MetricKind) []Series {
	out := make([]Series, 0, len(rows))
	for _, r := range rows {
		s := Series{User: r.User, Points: make([]SeriesPoint, len(r.Cells))}
		for i, c := range r.Cells {
			s.Points[i] = SeriesPoint{
				SprintIndex: i,
				Value:       CellValue(c, k),
				Present:     CellHasValue(c, k),
			}
		}
		out = append(out, s)
	}
	return out
}

// SeriesMax returns the largest plotted value across all series, ignoring gaps.
// Returns 0 if there is nothing to plot.
func SeriesMax(series []Series) float64 {
	max := 0.0
	for _, s := range series {
		for _, p := range s.Points {
			if p.Present && p.Value > max {
				max = p.Value
			}
		}
	}
	return max
}

// NiceCeil rounds v up to the nearest "nice" axis bound (1, 2, or 5 times a
// power of ten), so the Y axis maximum lands on a readable number. Values <= 0
// return 1 to keep a sane axis even when there's no data.
func NiceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(v))
	pow := math.Pow(10, exp)
	f := v / pow // mantissa in [1, 10)
	var nice float64
	switch {
	case f <= 1:
		nice = 1
	case f <= 2:
		nice = 2
	case f <= 5:
		nice = 5
	default:
		nice = 10
	}
	return nice * pow
}
