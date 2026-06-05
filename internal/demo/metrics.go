package demo

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/config"
	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
)

// Demo workflow state names. They mirror the metrics defaults so the seeded
// trend history buckets correctly against enableDemoMetrics' config.
const (
	demoStateActive = "Active"
	demoStateRFT    = "Ready for Test"
	demoStateClosed = "Closed"
)

// demoSprintTags lists the seeded sprints oldest → newest. The last one is the
// in-flight sprint; the rest have wrapped. Order is also the column order the
// Trends view renders, since it's saved as the sprint selection.
var demoSprintTags = []string{"Sprint 22", "Sprint 23", "Sprint 24"}

// demoSprint describes one seeded sprint's position on the calendar.
type demoSprint struct {
	tag       string
	startDays int  // calendar days before `now` that sprint day 0 falls on
	length    int  // number of days in the sprint window
	inFlight  bool // true → still running (items remain open today)
}

// demoMetricItem is one work item's lifecycle within a sprint, expressed in
// sprint-relative day offsets.
type demoMetricItem struct {
	assignee  int     // index into team
	project   string  // API project name
	points    float64 // story points (counted as velocity when closed)
	activeDay int     // day the item entered Active
	rftDay    int     // day it entered Ready for Test (-1 = never)
	closedDay int     // day it entered Closed (-1 = still open)
}

// enableDemoMetrics turns the metrics tab on and fills in the thresholds and
// state names. config.NewWithPath bypasses viper defaults, so these must be
// set explicitly — otherwise WIPLimit=0 marks everyone overloaded and the
// zero stale-day thresholds flag every item.
func enableDemoMetrics(cfg *config.Config) {
	cfg.Metrics = config.MetricsConfig{
		Enabled:         true,
		IntervalDays:    config.DefaultMetricsIntervalDays,
		ActiveStaleDays: config.DefaultMetricsActiveStaleDays,
		RFTStaleDays:    config.DefaultMetricsRFTStaleDays,
		WIPLimit:        config.DefaultMetricsWIPLimit,
		States: config.MetricsStates{
			Active:       config.DefaultMetricsActiveState,
			ReadyForTest: config.DefaultMetricsReadyForTestState,
			Closed:       config.DefaultMetricsClosedState,
		},
	}
}

// seedMetricsHistory writes the synthetic snapshot history and pre-selects all
// demo sprints so the Trends view renders a sprint-on-sprint comparison the
// moment the user opens it. Both files land under AZDO_CONFIG_DIR, which demo
// mode points at its temp dir, so the user's real metrics history is untouched.
func seedMetricsHistory() error {
	snapPath, err := coremetrics.DefaultSnapshotPath()
	if err != nil {
		return err
	}
	if err := coremetrics.EnsureSnapshotDir(snapPath); err != nil {
		return err
	}
	const retention = 90 * 24 * time.Hour
	if err := coremetrics.AppendSnapshots(snapPath, mockSnapshots(), retention, now); err != nil {
		return err
	}

	selPath, err := coremetrics.DefaultSelectionPath()
	if err != nil {
		return err
	}
	return coremetrics.SaveSelection(selPath, demoSprintTags)
}

// mockSnapshots builds three sprints of daily snapshot rows across the team:
// two wrapped sprints with rising velocity and one in-flight sprint with open
// work. The shape is designed to exercise every Trends column — velocity,
// average WIP, cycle time, and stuck counts.
func mockSnapshots() []coremetrics.Snapshot {
	sprints := []demoSprint{
		{tag: "Sprint 22", startDays: 41, length: 14, inFlight: false},
		{tag: "Sprint 23", startDays: 27, length: 14, inFlight: false},
		{tag: "Sprint 24", startDays: 13, length: 14, inFlight: true},
	}

	items := map[string][]demoMetricItem{
		"Sprint 22": {
			{assignee: 0, project: projectNexus, points: 5, activeDay: 0, rftDay: 6, closedDay: 9},
			{assignee: 1, project: projectNexus, points: 3, activeDay: 0, rftDay: 4, closedDay: 7},
			{assignee: 2, project: projectHorizon, points: 8, activeDay: 1, rftDay: 8, closedDay: 12},
			{assignee: 3, project: projectHorizon, points: 2, activeDay: 2, rftDay: 5, closedDay: 8},
			{assignee: 4, project: projectNexus, points: 3, activeDay: 0, rftDay: 7, closedDay: 11},
			{assignee: 5, project: projectHorizon, points: 5, activeDay: 3, rftDay: 9, closedDay: 13},
			{assignee: 0, project: projectNexus, points: 2, activeDay: 5, rftDay: 9, closedDay: 12},
		},
		"Sprint 23": {
			{assignee: 0, project: projectNexus, points: 8, activeDay: 0, rftDay: 7, closedDay: 11},
			{assignee: 1, project: projectNexus, points: 5, activeDay: 0, rftDay: 5, closedDay: 9},
			{assignee: 1, project: projectHorizon, points: 2, activeDay: 4, rftDay: 8, closedDay: 12},
			{assignee: 2, project: projectHorizon, points: 3, activeDay: 1, rftDay: 6, closedDay: 10},
			{assignee: 3, project: projectNexus, points: 5, activeDay: 0, rftDay: -1, closedDay: 13},
			{assignee: 4, project: projectHorizon, points: 3, activeDay: 2, rftDay: 9, closedDay: 13},
			{assignee: 5, project: projectNexus, points: 8, activeDay: 0, rftDay: 10, closedDay: 13},
			{assignee: 5, project: projectHorizon, points: 2, activeDay: 6, rftDay: 9, closedDay: 12},
		},
		"Sprint 24": {
			{assignee: 0, project: projectNexus, points: 5, activeDay: 0, rftDay: 6, closedDay: 10},
			{assignee: 0, project: projectHorizon, points: 3, activeDay: 4, rftDay: -1, closedDay: -1},
			{assignee: 1, project: projectNexus, points: 8, activeDay: 0, rftDay: 9, closedDay: -1},
			{assignee: 2, project: projectHorizon, points: 2, activeDay: 2, rftDay: -1, closedDay: -1},
			{assignee: 3, project: projectNexus, points: 5, activeDay: 0, rftDay: 5, closedDay: 9},
			{assignee: 3, project: projectHorizon, points: 3, activeDay: 7, rftDay: -1, closedDay: -1},
			{assignee: 4, project: projectNexus, points: 2, activeDay: 3, rftDay: 11, closedDay: -1},
			{assignee: 5, project: projectHorizon, points: 5, activeDay: 0, rftDay: 8, closedDay: 12},
			{assignee: 5, project: projectNexus, points: 2, activeDay: 9, rftDay: -1, closedDay: -1},
		},
	}

	var out []coremetrics.Snapshot
	id := 9000
	for _, s := range sprints {
		for _, it := range items[s.tag] {
			id++
			out = appendItemSnapshots(out, s, it, id)
		}
	}
	return out
}

// appendItemSnapshots emits one daily snapshot row per day of an item's life
// within its sprint, walking Active → Ready for Test → Closed at the item's
// configured transition days.
func appendItemSnapshots(out []coremetrics.Snapshot, s demoSprint, it demoMetricItem, id int) []coremetrics.Snapshot {
	lastDay := s.length - 1

	// In a wrapped sprint we keep emitting Closed rows through the final day so
	// the latest observation is Closed (DeriveSprintWindow then treats it as
	// wrapped). In the in-flight sprint we stop Closed rows one day early so the
	// open items remain the newest observation and the window pins to `now`.
	closedFill := lastDay
	if s.inFlight {
		closedFill = lastDay - 1
	}

	assignee := team[it.assignee].DisplayName

	for d := it.activeDay; d <= lastDay; d++ {
		state := demoStateActive
		sinceDay := it.activeDay
		if it.rftDay >= 0 && d >= it.rftDay {
			state = demoStateRFT
			sinceDay = it.rftDay
		}
		if it.closedDay >= 0 && d >= it.closedDay {
			state = demoStateClosed
			sinceDay = it.closedDay
		}
		if state == demoStateClosed && d > closedFill {
			break
		}

		ts := demoDayStart(s.startDays - d)
		since := demoDayStart(s.startDays - sinceDay)
		out = append(out, coremetrics.Snapshot{
			TS:         ts.Format("2006-01-02"),
			ID:         id,
			Project:    it.project,
			State:      state,
			AssignedTo: assignee,
			Points:     it.points,
			Tags:       []string{s.tag},
			Iteration:  fmt.Sprintf("%s\\%s", it.project, s.tag),
			StateSince: since.Format(time.RFC3339),
			Source:     coremetrics.SourceSnapshot,
		})
	}
	return out
}

// demoDayStart returns midnight, `daysAgo` days before the demo's `now` anchor.
func demoDayStart(daysAgo int) time.Time {
	y, m, d := now.Date()
	base := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	return base.AddDate(0, 0, -daysAgo)
}
