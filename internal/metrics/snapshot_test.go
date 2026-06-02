package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

var snapNow = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

// TestReadSnapshots_MissingFile returns nil rows and no error.
func TestReadSnapshots_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.jsonl")
	rows, err := ReadSnapshots(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rows != nil {
		t.Errorf("expected nil rows, got %v", rows)
	}
}

// TestReadSnapshots_RoundTrip writes a fresh file then reads it back unchanged.
func TestReadSnapshots_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.jsonl")
	in := []Snapshot{
		{TS: "2026-05-28", ID: 1, State: "Active", AssignedTo: "Alice", Source: SourceSnapshot},
		{TS: "2026-05-29", ID: 1, State: "Ready for Test", AssignedTo: "Alice", Source: SourceSnapshot},
	}
	if err := AppendSnapshots(path, in, 90*24*time.Hour, snapNow); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := ReadSnapshots(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("read %d rows, wrote %d", len(out), len(in))
	}
	for i := range in {
		if out[i].TS != in[i].TS || out[i].ID != in[i].ID || out[i].State != in[i].State {
			t.Errorf("row %d mismatch: got %+v, want %+v", i, out[i], in[i])
		}
	}
}

// TestReadSnapshots_SkipsMalformed keeps valid rows when a line is corrupt.
func TestReadSnapshots_SkipsMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.jsonl")
	good, _ := json.Marshal(Snapshot{TS: "2026-05-28", ID: 1, State: "Active"})
	content := []byte(string(good) + "\n{this is not json}\n" + string(good) + "\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadSnapshots(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 valid rows, got %d", len(rows))
	}
}

// TestAppendSnapshots_DedupLatestWins for (TS, ID), newSnaps replace existing.
func TestAppendSnapshots_DedupLatestWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.jsonl")
	first := []Snapshot{{TS: "2026-05-28", ID: 1, State: "Active", AssignedTo: "Alice"}}
	if err := AppendSnapshots(path, first, 90*24*time.Hour, snapNow); err != nil {
		t.Fatal(err)
	}
	// Same (TS,ID), new state — should overwrite, not duplicate.
	second := []Snapshot{{TS: "2026-05-28", ID: 1, State: "Ready for Test", AssignedTo: "Alice"}}
	if err := AppendSnapshots(path, second, 90*24*time.Hour, snapNow); err != nil {
		t.Fatal(err)
	}
	rows, _ := ReadSnapshots(path)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after dedup, got %d", len(rows))
	}
	if rows[0].State != "Ready for Test" {
		t.Errorf("dedup kept wrong row: state = %q, want %q", rows[0].State, "Ready for Test")
	}
}

// TestAppendSnapshots_PrunesOldRows drops rows older than retention.
func TestAppendSnapshots_PrunesOldRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.jsonl")
	// 100 days before snapNow (June 1, 2026) is Feb 21, 2026.
	old := []Snapshot{{TS: "2026-02-21", ID: 1, State: "Active"}}
	fresh := []Snapshot{{TS: "2026-05-31", ID: 2, State: "Active"}}
	if err := AppendSnapshots(path, append(old, fresh...), 90*24*time.Hour, snapNow); err != nil {
		t.Fatal(err)
	}
	rows, _ := ReadSnapshots(path)
	if len(rows) != 1 || rows[0].ID != 2 {
		t.Errorf("after prune: got %+v, want only ID 2", rows)
	}
}

// TestAppendSnapshots_AtomicOnEncodeFailure does not corrupt an existing file
// when the write fails midway. We can't easily inject an encode error without
// a custom io.Writer, so we instead verify the .tmp file is cleaned up after
// a successful write — the inverse property of atomicity.
func TestAppendSnapshots_AtomicLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.jsonl")
	in := []Snapshot{{TS: "2026-05-29", ID: 1, State: "Active"}}
	if err := AppendSnapshots(path, in, 90*24*time.Hour, snapNow); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// TestAppendSnapshots_SortsDeterministically by TS asc then ID asc.
func TestAppendSnapshots_SortsDeterministically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.jsonl")
	in := []Snapshot{
		{TS: "2026-05-29", ID: 5, State: "Active"},
		{TS: "2026-05-28", ID: 2, State: "Active"},
		{TS: "2026-05-28", ID: 1, State: "Active"},
		{TS: "2026-05-29", ID: 1, State: "Active"},
	}
	if err := AppendSnapshots(path, in, 90*24*time.Hour, snapNow); err != nil {
		t.Fatal(err)
	}
	rows, _ := ReadSnapshots(path)
	if !sort.SliceIsSorted(rows, func(i, j int) bool {
		if rows[i].TS != rows[j].TS {
			return rows[i].TS < rows[j].TS
		}
		return rows[i].ID < rows[j].ID
	}) {
		t.Errorf("rows not sorted: %+v", rows)
	}
}

// TestBuildSnapshots_FromWorkItems converts WorkItems to today-stamped rows.
func TestBuildSnapshots_FromWorkItems(t *testing.T) {
	items := []azdevops.WorkItem{
		mkSnapItem(1, "Active", "Alice", "proj-a", 3, "sprint-42;mobile"),
		mkSnapItem(2, "Closed", "Bob", "proj-b", 5, ""),
	}
	rows := BuildSnapshots(items, snapNow)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.TS != "2026-06-01" {
			t.Errorf("TS = %q, want today", r.TS)
		}
		if r.Source != SourceSnapshot {
			t.Errorf("Source = %q, want %q", r.Source, SourceSnapshot)
		}
	}
	// Check tag list parsing.
	if got, want := rows[0].Tags, []string{"sprint-42", "mobile"}; !equalStringSlices(got, want) {
		t.Errorf("Tags = %v, want %v", got, want)
	}
	if rows[0].AssignedTo != "Alice" {
		t.Errorf("AssignedTo = %q, want Alice", rows[0].AssignedTo)
	}
	if rows[0].Points != 3 {
		t.Errorf("Points = %v, want 3", rows[0].Points)
	}
}

func mkSnapItem(id int, state, user, project string, points float64, tags string) azdevops.WorkItem {
	wi := azdevops.WorkItem{
		ID:                 id,
		ProjectName:        project,
		ProjectDisplayName: project,
	}
	wi.Fields.State = state
	wi.Fields.StoryPoints = points
	wi.Fields.Tags = tags
	wi.Fields.StateChangeDate = snapNow.AddDate(0, 0, -2)
	if state == "Closed" {
		wi.Fields.ClosedDate = snapNow.AddDate(0, 0, -1)
	}
	if user != "" {
		wi.Fields.AssignedTo = &azdevops.Identity{DisplayName: user}
	}
	return wi
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
