package metrics

import (
	"math"
	"testing"
	"time"
)

func TestNiceCeil(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0, 1},
		{-5, 1},
		{0.5, 0.5},
		{0.6, 1},
		{1, 1},
		{1.4, 2},
		{2, 2},
		{2.0001, 5},
		{4.9, 5},
		{5, 5},
		{6, 10},
		{10, 10},
		{12, 20},
		{18, 20},
		{21, 50},
	}
	for _, c := range cases {
		got := NiceCeil(c.in)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("NiceCeil(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCellValue_CycleInDays(t *testing.T) {
	c := TrendCell{CycleTime: 48 * time.Hour}
	if got := CellValue(c, MetricCycle); math.Abs(got-2) > 1e-9 {
		t.Errorf("CellValue cycle = %v, want 2 days", got)
	}
	c = TrendCell{Points: 8, AvgWIP: 2.5, StuckCount: 3}
	if got := CellValue(c, MetricPoints); got != 8 {
		t.Errorf("points = %v, want 8", got)
	}
	if got := CellValue(c, MetricAvgWIP); got != 2.5 {
		t.Errorf("wip = %v, want 2.5", got)
	}
	if got := CellValue(c, MetricStuck); got != 3 {
		t.Errorf("stuck = %v, want 3", got)
	}
}

func TestCellHasValue_GapVsZero(t *testing.T) {
	// Fully empty cell (user absent that sprint): no value for any metric.
	empty := TrendCell{}
	for _, k := range AllMetricKinds {
		if CellHasValue(empty, k) {
			t.Errorf("empty cell should have no value for %v", k.Label())
		}
	}

	// Active but closed nothing: points is a *real* zero (present), but cycle
	// time is undefined (gap) because no item completed.
	active := TrendCell{AvgWIP: 1.5}
	if !CellHasValue(active, MetricPoints) {
		t.Error("active cell should plot a real zero for points")
	}
	if CellValue(active, MetricPoints) != 0 {
		t.Error("points value should be 0")
	}
	if CellHasValue(active, MetricCycle) {
		t.Error("cycle time with no completed item must be a gap, not 0")
	}

	// A cell with a real cycle measurement plots it.
	done := TrendCell{AvgWIP: 1, CycleTime: 72 * time.Hour}
	if !CellHasValue(done, MetricCycle) {
		t.Error("cell with cycle measurement should have a value")
	}
}

func TestBuildSeries_AndMax(t *testing.T) {
	rows := []TrendRow{
		{User: "alice", Cells: []TrendCell{
			{Points: 8, AvgWIP: 2},
			{}, // absent sprint 1
			{Points: 12, AvgWIP: 3},
		}},
		{User: "bob", Cells: []TrendCell{
			{Points: 5, AvgWIP: 1},
			{Points: 0, AvgWIP: 1}, // present, real zero points
			{Points: 7, AvgWIP: 2},
		}},
	}

	series := BuildSeries(rows, MetricPoints)
	if len(series) != 2 {
		t.Fatalf("got %d series, want 2", len(series))
	}
	if series[0].User != "alice" || len(series[0].Points) != 3 {
		t.Fatalf("alice series malformed: %+v", series[0])
	}
	// alice sprint 1 is a gap.
	if series[0].Points[1].Present {
		t.Error("alice sprint 1 should be a gap")
	}
	// bob sprint 1 is a present zero.
	if !series[1].Points[1].Present {
		t.Error("bob sprint 1 should be present")
	}
	if series[1].Points[1].Value != 0 {
		t.Error("bob sprint 1 value should be 0")
	}

	if got := SeriesMax(series); got != 12 {
		t.Errorf("SeriesMax = %v, want 12", got)
	}
}
