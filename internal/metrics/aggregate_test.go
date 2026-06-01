package metrics

import (
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// fixed reference points so dwell math is deterministic
var (
	now      = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	interval = now.Add(-14 * 24 * time.Hour) // 14-day window
)

// item builds a WorkItem with the fields Aggregate reads. Keeps tests terse.
func item(id int, state, user string, daysInState int, closedDaysAgo int, points float64) azdevops.WorkItem {
	wi := azdevops.WorkItem{
		ID:                 id,
		ProjectDisplayName: "proj",
		Fields: azdevops.WorkItemFields{
			Title:           "item",
			State:           state,
			StateChangeDate: now.Add(-time.Duration(daysInState) * 24 * time.Hour),
			StoryPoints:     points,
		},
	}
	if user != "" {
		wi.Fields.AssignedTo = &azdevops.Identity{DisplayName: user}
	}
	if closedDaysAgo >= 0 {
		wi.Fields.ClosedDate = now.Add(-time.Duration(closedDaysAgo) * 24 * time.Hour)
	}
	return wi
}

func defaultThresholds() Thresholds {
	return Thresholds{
		ActiveStaleDays: 3,
		RFTStaleDays:    2,
		WIPLimit:        4,
	}
}

func TestAggregate_BucketsByState(t *testing.T) {
	items := []azdevops.WorkItem{
		item(1, "Active", "Alice", 1, -1, 3),
		item(2, "Active", "Alice", 1, -1, 5),
		item(3, "Ready for Test", "Alice", 1, -1, 8),
		item(4, "Closed", "Alice", 1, 5, 13),
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 1 {
		t.Fatalf("expected 1 user row, got %d", len(rows))
	}
	r := rows[0]
	if r.User != "Alice" {
		t.Errorf("User = %q, want Alice", r.User)
	}
	if r.ActiveCount != 2 {
		t.Errorf("ActiveCount = %d, want 2", r.ActiveCount)
	}
	if r.RFTCount != 1 {
		t.Errorf("RFTCount = %d, want 1", r.RFTCount)
	}
	if r.InFlight != 3 {
		t.Errorf("InFlight = %d, want 3", r.InFlight)
	}
	if r.PointsClosed != 13 {
		t.Errorf("PointsClosed = %v, want 13", r.PointsClosed)
	}
}

func TestAggregate_PointsClosed_RespectsIntervalWindow(t *testing.T) {
	// 14-day window. Item closed 5 days ago is in; 30 days ago is out.
	items := []azdevops.WorkItem{
		item(1, "Closed", "Alice", 5, 5, 3),
		item(2, "Closed", "Alice", 30, 30, 100), // outside window
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 1 {
		t.Fatalf("expected 1 user row, got %d", len(rows))
	}
	if rows[0].PointsClosed != 3 {
		t.Errorf("PointsClosed = %v, want 3 (the 100-pt item is outside the interval)", rows[0].PointsClosed)
	}
}

func TestAggregate_FlagsStaleActive(t *testing.T) {
	// Threshold: active-stale > 3d. Item at 5d should be flagged.
	items := []azdevops.WorkItem{
		item(1, "Active", "Bob", 5, -1, 0),
		item(2, "Active", "Bob", 1, -1, 0),
	}

	rows, flags := Aggregate(items, interval, now, defaultThresholds())

	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
	if flags[0].ID != 1 {
		t.Errorf("flag ID = %d, want 1", flags[0].ID)
	}
	if flags[0].Reason != "active-stale" {
		t.Errorf("flag Reason = %q, want active-stale", flags[0].Reason)
	}
	if rows[0].Stalled != 1 {
		t.Errorf("Stalled = %d, want 1", rows[0].Stalled)
	}
	if rows[0].OldestActive != 5*24*time.Hour {
		t.Errorf("OldestActive = %v, want 5 days", rows[0].OldestActive)
	}
}

func TestAggregate_FlagsStaleRFT(t *testing.T) {
	// Threshold: rft-stale > 2d. Item at 4d in RFT should be flagged.
	items := []azdevops.WorkItem{
		item(1, "Ready for Test", "Carol", 4, -1, 0),
		item(2, "Ready for Test", "Carol", 1, -1, 0),
	}

	rows, flags := Aggregate(items, interval, now, defaultThresholds())

	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
	if flags[0].Reason != "rft-stale" {
		t.Errorf("flag Reason = %q, want rft-stale", flags[0].Reason)
	}
	if rows[0].Stalled != 1 {
		t.Errorf("Stalled = %d, want 1", rows[0].Stalled)
	}
	if rows[0].OldestRFT != 4*24*time.Hour {
		t.Errorf("OldestRFT = %v, want 4 days", rows[0].OldestRFT)
	}
}

func TestAggregate_OverloadedFlag(t *testing.T) {
	// WIPLimit = 4. Five in-flight items must set Overloaded.
	var items []azdevops.WorkItem
	for i := 0; i < 5; i++ {
		items = append(items, item(i+1, "Active", "Dave", 1, -1, 0))
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 1 {
		t.Fatalf("expected 1 user row, got %d", len(rows))
	}
	if !rows[0].Overloaded {
		t.Error("Overloaded = false, want true")
	}
	if rows[0].InFlight != 5 {
		t.Errorf("InFlight = %d, want 5", rows[0].InFlight)
	}
}

func TestAggregate_NotOverloadedAtLimit(t *testing.T) {
	// Limit = 4. Exactly 4 in-flight is not over.
	var items []azdevops.WorkItem
	for i := 0; i < 4; i++ {
		items = append(items, item(i+1, "Active", "Eve", 1, -1, 0))
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if rows[0].Overloaded {
		t.Error("Overloaded = true at the limit; want false (strictly greater)")
	}
}

func TestAggregate_SortByStalledThenInFlight(t *testing.T) {
	items := []azdevops.WorkItem{
		// Alice: 2 in-flight, 0 stalled
		item(1, "Active", "Alice", 1, -1, 0),
		item(2, "Active", "Alice", 1, -1, 0),
		// Bob: 1 in-flight, 1 stalled (5d > 3d)
		item(3, "Active", "Bob", 5, -1, 0),
		// Carol: 3 in-flight, 0 stalled
		item(4, "Active", "Carol", 1, -1, 0),
		item(5, "Active", "Carol", 1, -1, 0),
		item(6, "Active", "Carol", 1, -1, 0),
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 3 {
		t.Fatalf("expected 3 user rows, got %d", len(rows))
	}
	// Stalled wins → Bob first
	if rows[0].User != "Bob" {
		t.Errorf("rows[0].User = %q, want Bob (most stalled)", rows[0].User)
	}
	// Tied stalled = 0, in-flight breaks: Carol(3) > Alice(2)
	if rows[1].User != "Carol" {
		t.Errorf("rows[1].User = %q, want Carol (more in-flight)", rows[1].User)
	}
	if rows[2].User != "Alice" {
		t.Errorf("rows[2].User = %q, want Alice", rows[2].User)
	}
}

func TestAggregate_FlagsSortedWorstFirst(t *testing.T) {
	items := []azdevops.WorkItem{
		item(1, "Active", "Alice", 4, -1, 0),         // 4d, stale
		item(2, "Active", "Bob", 10, -1, 0),          // 10d, stale (worst)
		item(3, "Ready for Test", "Carol", 6, -1, 0), // 6d, stale
	}

	_, flags := Aggregate(items, interval, now, defaultThresholds())

	if len(flags) != 3 {
		t.Fatalf("expected 3 flags, got %d", len(flags))
	}
	// Worst dwell first
	if flags[0].ID != 2 {
		t.Errorf("flags[0].ID = %d, want 2 (10d, worst)", flags[0].ID)
	}
	if flags[1].ID != 3 {
		t.Errorf("flags[1].ID = %d, want 3 (6d)", flags[1].ID)
	}
	if flags[2].ID != 1 {
		t.Errorf("flags[2].ID = %d, want 1 (4d)", flags[2].ID)
	}
}

func TestAggregate_IgnoresNewAndUnknownStates(t *testing.T) {
	// Spec excludes New from the WIQL. Defensively: Aggregate ignores
	// anything that isn't Active / Ready for Test / Closed.
	items := []azdevops.WorkItem{
		item(1, "New", "Alice", 1, -1, 0),
		item(2, "Removed", "Alice", 1, -1, 0),
		item(3, "InReview", "Alice", 1, -1, 0),
	}

	rows, flags := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 0 {
		t.Errorf("expected 0 user rows for ignored states, got %d", len(rows))
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags for ignored states, got %d", len(flags))
	}
}

func TestAggregate_UnassignedUser(t *testing.T) {
	// AssignedToName returns "-" for nil assignments; Aggregate should
	// roll them into a single "-" bucket rather than dropping them.
	items := []azdevops.WorkItem{
		item(1, "Active", "", 1, -1, 0),
		item(2, "Active", "", 1, -1, 0),
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 1 {
		t.Fatalf("expected 1 row for unassigned bucket, got %d", len(rows))
	}
	if rows[0].User != "-" {
		t.Errorf("User = %q, want \"-\"", rows[0].User)
	}
	if rows[0].InFlight != 2 {
		t.Errorf("InFlight = %d, want 2", rows[0].InFlight)
	}
}

func TestAggregate_CaseInsensitiveStateMatching(t *testing.T) {
	// State strings may come back in either case in practice; Aggregate
	// folds them so it doesn't silently miss anything.
	items := []azdevops.WorkItem{
		item(1, "active", "Alice", 1, -1, 0),
		item(2, "READY FOR TEST", "Alice", 1, -1, 0),
		item(3, "closed", "Alice", 5, 5, 7),
	}

	rows, _ := Aggregate(items, interval, now, defaultThresholds())

	if len(rows) != 1 {
		t.Fatalf("expected 1 user row, got %d", len(rows))
	}
	if rows[0].ActiveCount != 1 || rows[0].RFTCount != 1 {
		t.Errorf("counts wrong: Active=%d RFT=%d", rows[0].ActiveCount, rows[0].RFTCount)
	}
	if rows[0].PointsClosed != 7 {
		t.Errorf("PointsClosed = %v, want 7", rows[0].PointsClosed)
	}
}
