package demo

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/config"
	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
)

func demoThresholds() coremetrics.Thresholds {
	return coremetrics.Thresholds{
		ActiveStaleDays: config.DefaultMetricsActiveStaleDays,
		RFTStaleDays:    config.DefaultMetricsRFTStaleDays,
		WIPLimit:        config.DefaultMetricsWIPLimit,
		States:          coremetrics.DefaultStates(),
	}
}

func TestMockSnapshots_CoversAllDemoSprints(t *testing.T) {
	snaps := mockSnapshots()
	if len(snaps) == 0 {
		t.Fatal("mockSnapshots returned no rows")
	}

	seen := map[string]int{}
	for _, s := range snaps {
		if s.AssignedTo == "" {
			t.Errorf("snapshot id=%d has empty AssignedTo", s.ID)
		}
		if len(s.Tags) == 0 {
			t.Errorf("snapshot id=%d has no tags", s.ID)
		}
		for _, tag := range s.Tags {
			seen[tag]++
		}
	}

	for _, tag := range demoSprintTags {
		if seen[tag] == 0 {
			t.Errorf("no snapshot rows tagged %q", tag)
		}
	}
}

func TestMockSnapshots_SprintWindowsInFlightVsWrapped(t *testing.T) {
	snaps := mockSnapshots()
	states := coremetrics.DefaultStates()

	// The newest sprint is still running: its window end is pinned to `now`.
	w, ok := coremetrics.DeriveSprintWindow(snaps, "Sprint 24", now, states)
	if !ok {
		t.Fatal("Sprint 24 window not derived")
	}
	if !w.End.Equal(now) {
		t.Errorf("Sprint 24 should be in flight (End == now); End=%v now=%v", w.End, now)
	}

	// Older sprints have wrapped: their window end is strictly before now.
	for _, tag := range []string{"Sprint 22", "Sprint 23"} {
		w, ok := coremetrics.DeriveSprintWindow(snaps, tag, now, states)
		if !ok {
			t.Fatalf("%s window not derived", tag)
		}
		if !w.End.Before(now) {
			t.Errorf("%s should be wrapped (End < now); End=%v now=%v", tag, w.End, now)
		}
		if !w.Start.Before(w.End) {
			t.Errorf("%s window Start should precede End; Start=%v End=%v", tag, w.Start, w.End)
		}
	}
}

func TestMockSnapshots_TrendAggregateProducesRows(t *testing.T) {
	snaps := mockSnapshots()
	states := coremetrics.DefaultStates()

	var windows []coremetrics.SprintWindow
	for _, tag := range demoSprintTags {
		w, ok := coremetrics.DeriveSprintWindow(snaps, tag, now, states)
		if !ok {
			t.Fatalf("%s window not derived", tag)
		}
		windows = append(windows, w)
	}

	rows := coremetrics.TrendAggregate(snaps, windows, demoThresholds(), now)
	if len(rows) == 0 {
		t.Fatal("TrendAggregate produced no rows")
	}

	// At least one wrapped sprint should show closed story points (velocity).
	var anyPoints bool
	for _, r := range rows {
		for _, c := range r.Cells {
			if c.Points > 0 {
				anyPoints = true
			}
		}
	}
	if !anyPoints {
		t.Error("no sprint cell shows closed story points")
	}
}

// The live dashboard's stuck-items digest is built by Aggregate from the mock
// work items' current-state dwell. This guards against the regression where
// StateChangeDate was unset (dwell always 0 → an empty stuck list).
func TestMockWorkItems_LiveAggregateHasFlagsAndBuckets(t *testing.T) {
	items := mockWorkItems()
	intervalStart := now.AddDate(0, 0, -config.DefaultMetricsIntervalDays)

	rows, flags := coremetrics.Aggregate(items, intervalStart, now, demoThresholds())

	if len(flags) == 0 {
		t.Fatal("Aggregate produced no stuck-item flags; the digest would be empty")
	}

	var activeStale, rftStale bool
	for _, f := range flags {
		switch f.Reason {
		case "active-stale":
			activeStale = true
		case "rft-stale":
			rftStale = true
		}
		if f.Dwell <= 0 {
			t.Errorf("flag %d has non-positive dwell %v", f.ID, f.Dwell)
		}
	}
	if !activeStale {
		t.Error("expected at least one active-stale flag")
	}
	if !rftStale {
		t.Error("expected at least one rft-stale flag")
	}

	var anyRFT, anyClosedPoints bool
	for _, r := range rows {
		if r.RFTCount > 0 {
			anyRFT = true
		}
		if r.PointsClosed > 0 {
			anyClosedPoints = true
		}
	}
	if !anyRFT {
		t.Error("expected at least one user with a Ready for Test item")
	}
	if !anyClosedPoints {
		t.Error("expected at least one user with closed story points")
	}
}

func TestEnableDemoMetrics_TabBecomesValid(t *testing.T) {
	cfg := config.NewWithPath("contoso", []string{"p"}, 3600, "dracula", "/tmp/x.yaml")
	enableDemoMetrics(cfg)

	if !cfg.Metrics.Enabled {
		t.Error("metrics not enabled")
	}
	if cfg.Metrics.IntervalDays <= 0 {
		t.Errorf("IntervalDays = %d, want > 0", cfg.Metrics.IntervalDays)
	}
	if cfg.Metrics.WIPLimit <= 0 {
		t.Errorf("WIPLimit = %d, want > 0", cfg.Metrics.WIPLimit)
	}
	if cfg.Metrics.States.Active == "" || cfg.Metrics.States.ReadyForTest == "" || cfg.Metrics.States.Closed == "" {
		t.Errorf("metrics states not fully populated: %+v", cfg.Metrics.States)
	}
	// An enabled metrics block must satisfy the real validator.
	if err := cfg.Validate(); err != nil {
		t.Errorf("demo config with metrics failed Validate(): %v", err)
	}
}
