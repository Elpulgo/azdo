package metrics

import (
	"testing"
	"time"
)

func TestClassifyTransition(t *testing.T) {
	cases := []struct {
		name string
		prev string
		curr string
		want GapAction
	}{
		{"first observation", "", "Active", GapWrite},
		{"first observation closed", "", "Closed", GapWrite},
		{"same state active", "Active", "Active", GapWrite},
		{"same state rft", "Ready for Test", "Ready for Test", GapWrite},
		{"active -> rft legal", "Active", "Ready for Test", GapWrite},
		{"rft -> closed legal", "Ready for Test", "Closed", GapWrite},
		{"active -> closed (skipped rft)", "Active", "Closed", GapNeedsFallback},
		{"closed -> active reopen", "Closed", "Active", GapNeedsFallback},
		{"closed -> rft backward", "Closed", "Ready for Test", GapNeedsFallback},
		{"rft -> active backward", "Ready for Test", "Active", GapNeedsFallback},
		{"case-insensitive match", "ACTIVE", "ready for test", GapWrite},
		{"unknown forward jump", "Active", "Resolved", GapNeedsFallback},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyTransition(c.prev, c.curr); got != c.want {
				t.Errorf("ClassifyTransition(%q, %q) = %v, want %v", c.prev, c.curr, got, c.want)
			}
		})
	}
}

func TestSynthesizeGapRows_FillsIntermediateDays(t *testing.T) {
	// Item went Active on day 1, RFT on day 3, Closed on day 5. Caller wants
	// the gap between day 1 (already have a snapshot) and day 5 (today's
	// observation), i.e. days 2, 3, 4 synthesized.
	day := func(d int) time.Time {
		return time.Date(2026, 5, d, 12, 0, 0, 0, time.UTC)
	}
	transitions := []StateTransition{
		{State: "Active", At: day(1).Add(10 * time.Hour)},
		{State: "Ready for Test", At: day(3).Add(10 * time.Hour)},
		{State: "Closed", At: day(5).Add(10 * time.Hour)},
	}
	template := Snapshot{ID: 42, Project: "p", AssignedTo: "Alice", Points: 3}
	rows := SynthesizeGapRows(transitions, day(1), day(5), template)

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (days 2,3,4)", len(rows))
	}

	want := []struct {
		ts    string
		state string
	}{
		{"2026-05-02", "Active"},
		{"2026-05-03", "Ready for Test"},
		{"2026-05-04", "Ready for Test"},
	}
	for i, w := range want {
		if rows[i].TS != w.ts {
			t.Errorf("row %d TS = %q, want %q", i, rows[i].TS, w.ts)
		}
		if rows[i].State != w.state {
			t.Errorf("row %d state = %q, want %q (full row: %+v)", i, rows[i].State, w.state, rows[i])
		}
		if rows[i].Source != SourceUpdates {
			t.Errorf("row %d Source = %q, want %q", i, rows[i].Source, SourceUpdates)
		}
		if rows[i].ID != template.ID || rows[i].AssignedTo != template.AssignedTo {
			t.Errorf("row %d template fields not copied: %+v", i, rows[i])
		}
	}
}

func TestSynthesizeGapRows_HandlesUnsortedInput(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2026, 5, d, 12, 0, 0, 0, time.UTC)
	}
	transitions := []StateTransition{
		{State: "Closed", At: day(5).Add(10 * time.Hour)},
		{State: "Active", At: day(1).Add(10 * time.Hour)},
		{State: "Ready for Test", At: day(3).Add(10 * time.Hour)},
	}
	rows := SynthesizeGapRows(transitions, day(1), day(5), Snapshot{ID: 1})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[1].State != "Ready for Test" {
		t.Errorf("day 3 state = %q, want RFT (unsorted input not normalized)", rows[1].State)
	}
}

func TestSynthesizeGapRows_NoGap(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2026, 5, d, 12, 0, 0, 0, time.UTC)
	}
	// since = day 1, until = day 2 — no days between them.
	transitions := []StateTransition{{State: "Active", At: day(1)}}
	rows := SynthesizeGapRows(transitions, day(1), day(2), Snapshot{ID: 1})
	if len(rows) != 0 {
		t.Errorf("expected 0 rows when no full day fits in the gap, got %d", len(rows))
	}
}

func TestSynthesizeGapRows_EmptyTransitions(t *testing.T) {
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows := SynthesizeGapRows(nil, day, day.AddDate(0, 0, 5), Snapshot{})
	if len(rows) != 0 {
		t.Errorf("expected 0 rows for empty input, got %d", len(rows))
	}
}

func TestSynthesizeGapRows_SkipsDaysBeforeFirstTransition(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2026, 5, d, 12, 0, 0, 0, time.UTC)
	}
	// Item first activated on day 10 at 10am. Caller asks for rows 5..15 —
	// days 6..9 are skipped (item didn't exist yet); days 10..14 emitted.
	transitions := []StateTransition{
		{State: "Active", At: day(10).Add(10 * time.Hour)},
	}
	rows := SynthesizeGapRows(transitions, day(5), day(15), Snapshot{ID: 1})
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5 (days 10-14)", len(rows))
	}
	if rows[0].TS != "2026-05-10" {
		t.Errorf("first emitted day = %q, want 2026-05-10", rows[0].TS)
	}
	if rows[len(rows)-1].TS != "2026-05-14" {
		t.Errorf("last emitted day = %q, want 2026-05-14", rows[len(rows)-1].TS)
	}
}
