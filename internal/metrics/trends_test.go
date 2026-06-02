package metrics

import (
	"sort"
	"testing"
	"time"
)

// helper: snapshot row builder for trend tests
func snap(ts string, id int, state, user string, points float64, tags ...string) Snapshot {
	return Snapshot{
		TS:         ts,
		ID:         id,
		State:      state,
		AssignedTo: user,
		Points:     points,
		Tags:       tags,
		Source:     SourceSnapshot,
	}
}

func TestDeriveSprintWindow_StartEnd(t *testing.T) {
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Active", "Alice", 3, "sprint-42"),
		snap("2026-05-11", 1, "Active", "Alice", 3, "sprint-42"),
		snap("2026-05-12", 1, "Ready for Test", "Alice", 3, "sprint-42"),
		snap("2026-05-13", 1, "Closed", "Alice", 3, "sprint-42"),
		// Unrelated row from another sprint, must be ignored.
		snap("2026-06-01", 2, "Active", "Bob", 3, "sprint-43"),
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	w, ok := DeriveSprintWindow(snaps, "sprint-42", now)
	if !ok {
		t.Fatal("expected window, got !ok")
	}
	if got, want := w.Start.Format("2006-01-02"), "2026-05-10"; got != want {
		t.Errorf("Start = %q, want %q", got, want)
	}
	// Last non-Closed observation is day 12 (RFT). Closed on day 13 ends the
	// in-flight window — items closed today still count toward velocity in
	// TrendAggregate, but the window itself stops at the last non-Closed day.
	if got, want := w.End.Format("2006-01-02"), "2026-05-12"; got != want {
		t.Errorf("End = %q, want %q", got, want)
	}
}

func TestDeriveSprintWindow_OngoingExtendsToNow(t *testing.T) {
	snaps := []Snapshot{
		snap("2026-05-25", 1, "Active", "Alice", 3, "sprint-42"),
		snap("2026-05-31", 1, "Active", "Alice", 3, "sprint-42"),
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	w, ok := DeriveSprintWindow(snaps, "sprint-42", now)
	if !ok {
		t.Fatal("expected window, got !ok")
	}
	if !w.End.Equal(now) {
		t.Errorf("ongoing sprint End = %v, want %v (now)", w.End, now)
	}
}

func TestDeriveSprintWindow_TagNeverSeen(t *testing.T) {
	snaps := []Snapshot{snap("2026-05-10", 1, "Active", "Alice", 3, "sprint-42")}
	if _, ok := DeriveSprintWindow(snaps, "sprint-99", time.Now()); ok {
		t.Error("expected !ok for tag never seen")
	}
}

func TestTrendAggregate_PointsClosed(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Active", "Alice", 3, "sprint-42"),
		snap("2026-05-12", 1, "Closed", "Alice", 3, "sprint-42"),
		snap("2026-05-13", 2, "Closed", "Alice", 5, "sprint-42"), // closed inside window
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-14")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{ActiveStaleDays: 3, RFTStaleDays: 2, WIPLimit: 4}, now)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if got, want := rows[0].Cells[0].Points, 8.0; got != want {
		t.Errorf("Points = %v, want %v", got, want)
	}
}

// TestTrendAggregate_SkipsUnassigned drops rows whose AssignedTo is "-" or
// empty — unassigned work items are not a developer signal and pollute the
// per-user grid with a row no one owns.
func TestTrendAggregate_SkipsUnassigned(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Closed", "Alice", 3, "sprint-42"),
		snap("2026-05-11", 2, "Closed", "-", 5, "sprint-42"),
		snap("2026-05-12", 3, "Closed", "", 7, "sprint-42"),
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-13")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{ActiveStaleDays: 3, RFTStaleDays: 2, WIPLimit: 4}, now)

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (only Alice); got users: %v", len(rows), rowNames(rows))
	}
	if rows[0].User != "Alice" {
		t.Errorf("row user = %q, want Alice", rows[0].User)
	}
}

func rowNames(rows []TrendRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.User
	}
	return out
}

// TestTrendAggregate_PointsClosed_NotDoubleCountedAcrossDays guards against a
// bug where each daily Closed snapshot row added its points to the total.
// With 90 days of backfill, a 5-pt item closed once could contribute 5*N to
// the sprint, inflating "pts" by an order of magnitude.
func TestTrendAggregate_PointsClosed_NotDoubleCountedAcrossDays(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Item 1 worth 5 pts is closed on day 1 of the window and stays Closed
	// for 5 days. Item 2 worth 8 pts is closed on day 4 and stays Closed for
	// 2 days. Each item should contribute its own points exactly once.
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Closed", "Alice", 5, "sprint-42"),
		snap("2026-05-11", 1, "Closed", "Alice", 5, "sprint-42"),
		snap("2026-05-12", 1, "Closed", "Alice", 5, "sprint-42"),
		snap("2026-05-13", 1, "Closed", "Alice", 5, "sprint-42"),
		snap("2026-05-14", 1, "Closed", "Alice", 5, "sprint-42"),
		snap("2026-05-13", 2, "Closed", "Alice", 8, "sprint-42"),
		snap("2026-05-14", 2, "Closed", "Alice", 8, "sprint-42"),
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-15")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{ActiveStaleDays: 3, RFTStaleDays: 2, WIPLimit: 4}, now)
	if got, want := rows[0].Cells[0].Points, 13.0; got != want {
		t.Errorf("Points = %v, want %v (5 + 8, not 5*5 + 8*2 = 41)", got, want)
	}
}

func TestTrendAggregate_AvgWIPAcrossDays(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Window: 3 days (May 10-12). Alice has 1 in-flight on day 10, 3 on day 11, 2 on day 12.
	// Avg = (1+3+2)/3 = 2.0
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Active", "Alice", 1, "sprint-42"),

		snap("2026-05-11", 1, "Active", "Alice", 1, "sprint-42"),
		snap("2026-05-11", 2, "Active", "Alice", 1, "sprint-42"),
		snap("2026-05-11", 3, "Ready for Test", "Alice", 1, "sprint-42"),

		snap("2026-05-12", 1, "Active", "Alice", 1, "sprint-42"),
		snap("2026-05-12", 2, "Ready for Test", "Alice", 1, "sprint-42"),
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-12")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{WIPLimit: 4}, now)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if got, want := rows[0].Cells[0].AvgWIP, 2.0; got != want {
		t.Errorf("AvgWIP = %v, want %v", got, want)
	}
}

func TestTrendAggregate_StuckCount_DedupedPerItem(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// item 1 stays Active 5 consecutive days → stuck (> 3 days). Should count once.
	// item 2 only 2 days → not stuck.
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-11", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-12", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-13", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-14", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-10", 2, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-11", 2, "Active", "Alice", 0, "sprint-42"),
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-14")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{ActiveStaleDays: 3, WIPLimit: 4}, now)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if got := rows[0].Cells[0].StuckCount; got != 1 {
		t.Errorf("StuckCount = %d, want 1 (item 2 not stuck; item 1 counted once not 5x)", got)
	}
}

func TestTrendAggregate_CycleTime(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Item 1: Active day 10 → Closed day 14 = 4 days cycle time.
	// Item 2: Active day 11 → Closed day 13 = 2 days.
	// Average = 3 days.
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Active", "Alice", 3, "sprint-42"),
		snap("2026-05-14", 1, "Closed", "Alice", 3, "sprint-42"),
		snap("2026-05-11", 2, "Active", "Alice", 5, "sprint-42"),
		snap("2026-05-13", 2, "Closed", "Alice", 5, "sprint-42"),
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-14")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{}, now)
	if got, want := rows[0].Cells[0].CycleTime, 3*24*time.Hour; got != want {
		t.Errorf("CycleTime = %v, want %v", got, want)
	}
}

func TestTrendAggregate_OverloadedAnyDay(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// WIPLimit=2. Alice peaks at 3 in-flight on day 11. Flag should fire.
	snaps := []Snapshot{
		snap("2026-05-10", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-11", 1, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-11", 2, "Active", "Alice", 0, "sprint-42"),
		snap("2026-05-11", 3, "Ready for Test", "Alice", 0, "sprint-42"),
		snap("2026-05-12", 1, "Active", "Alice", 0, "sprint-42"),
	}
	w := SprintWindow{Tag: "sprint-42", Start: mustDate("2026-05-10"), End: mustDate("2026-05-12")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{WIPLimit: 2}, now)
	if !rows[0].Cells[0].OverloadedAnyDay {
		t.Errorf("expected OverloadedAnyDay=true at peak 3 vs limit 2")
	}
}

func TestTrendAggregate_MultiSprintMultiUser(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	snaps := []Snapshot{
		snap("2026-05-01", 1, "Closed", "Alice", 3, "sprint-40"),
		snap("2026-05-08", 2, "Closed", "Bob", 5, "sprint-40"),
		snap("2026-05-15", 3, "Closed", "Alice", 8, "sprint-41"),
	}
	windows := []SprintWindow{
		{Tag: "sprint-40", Start: mustDate("2026-05-01"), End: mustDate("2026-05-10")},
		{Tag: "sprint-41", Start: mustDate("2026-05-11"), End: mustDate("2026-05-20")},
	}

	rows := TrendAggregate(snaps, windows, Thresholds{}, now)

	// Should produce one row per user, with one cell per window.
	byUser := make(map[string][]TrendCell)
	for _, r := range rows {
		byUser[r.User] = r.Cells
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (Alice + Bob)", len(rows))
	}
	if byUser["Alice"][0].Points != 3 {
		t.Errorf("Alice sprint-40 pts = %v, want 3", byUser["Alice"][0].Points)
	}
	if byUser["Alice"][1].Points != 8 {
		t.Errorf("Alice sprint-41 pts = %v, want 8", byUser["Alice"][1].Points)
	}
	if byUser["Bob"][0].Points != 5 {
		t.Errorf("Bob sprint-40 pts = %v, want 5", byUser["Bob"][0].Points)
	}
	if byUser["Bob"][1].Points != 0 {
		t.Errorf("Bob sprint-41 pts = %v, want 0 (no Bob items in window)", byUser["Bob"][1].Points)
	}
}

func TestTrendAggregate_RowsSortedByName(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	snaps := []Snapshot{
		snap("2026-05-01", 1, "Active", "Zach", 0, "sprint-40"),
		snap("2026-05-01", 2, "Active", "Alice", 0, "sprint-40"),
		snap("2026-05-01", 3, "Active", "Mara", 0, "sprint-40"),
	}
	w := SprintWindow{Tag: "sprint-40", Start: mustDate("2026-05-01"), End: mustDate("2026-05-01")}

	rows := TrendAggregate(snaps, []SprintWindow{w}, Thresholds{WIPLimit: 4}, now)
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.User
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("rows not sorted alphabetically by user: %v", names)
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
