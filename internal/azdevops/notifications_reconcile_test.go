package azdevops

import (
	"reflect"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// TestNotifKey table-tests the "<source>/<entity>/<entity_id>" key shape
// from decision 2, including the convention-11 guard: a non-positive id
// (<= 0, not just == 0) is rejected rather than producing a
// malformed-but-non-empty key.
func TestNotifKey(t *testing.T) {
	tests := []struct {
		name   string
		source string
		entity string
		id     int
		want   string
	}{
		{name: "positive id produces the source/entity/id shape", source: "review", entity: "pr", id: 1234, want: "review/pr/1234"},
		{name: "zero id rejected", source: "mention", entity: "wi", id: 0, want: ""},
		{name: "negative id rejected", source: "mention", entity: "wi", id: -5, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NotifKey(tt.source, tt.entity, tt.id)
			if got != tt.want {
				t.Errorf("NotifKey(%q, %q, %d) = %q, want %q", tt.source, tt.entity, tt.id, got, tt.want)
			}
		})
	}
}

// entriesEqual compares two TriageEntry values field by field, using
// time.Time.Equal for the timestamps so values with different (but
// equivalent) monotonic/location internals still compare equal.
func entriesEqual(a, b TriageEntry) bool {
	return a.Read == b.Read &&
		a.Done == b.Done &&
		a.LastActivity.Equal(b.LastActivity) &&
		a.LastSeen.Equal(b.LastSeen)
}

// row builds a minimal provider.Notification identified solely by key and
// stamped with updatedAt, which is all Reconcile inspects.
func row(key string, updatedAt time.Time) provider.Notification {
	return provider.Notification{
		Identity:  provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: key},
		UpdatedAt: updatedAt,
	}
}

// TestReconcile table-tests decision 2's pure (rows, state) -> (rows,
// state) reconcile step across every branch task 3 calls out: unseen,
// unchanged stamp, newer stamp, older stamp (a separate row from unchanged,
// since > and >= only differ on the equal case), done-row dropping, and TTL
// pruning with a boundary row exactly at the TTL (convention 13's shape).
func TestReconcile(t *testing.T) {
	base := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		rows      []provider.Notification
		state     map[string]TriageEntry
		now       time.Time
		wantKeys  []string
		wantState map[string]TriageEntry
	}{
		{
			name:     "unseen subject surfaces unread with a fresh entry",
			rows:     []provider.Notification{row("review/pr/1", base)},
			state:    map[string]TriageEntry{},
			now:      base,
			wantKeys: []string{"review/pr/1"},
			wantState: map[string]TriageEntry{
				"review/pr/1": {LastActivity: base, LastSeen: base},
			},
		},
		{
			name: "seen subject, unchanged stamp: stored read/done applied and LastSeen advances",
			rows: []provider.Notification{row("review/pr/1", base)},
			state: map[string]TriageEntry{
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base.Add(-24 * time.Hour)},
			},
			now:      base,
			wantKeys: []string{"review/pr/1"},
			wantState: map[string]TriageEntry{
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base},
			},
		},
		{
			name: "seen subject, newer stamp: read/done cleared and stamp advances",
			rows: []provider.Notification{row("review/pr/1", base.Add(time.Hour))},
			state: map[string]TriageEntry{
				"review/pr/1": {Read: true, Done: true, LastActivity: base, LastSeen: base},
			},
			now:      base.Add(time.Hour),
			wantKeys: []string{"review/pr/1"},
			wantState: map[string]TriageEntry{
				"review/pr/1": {Read: false, Done: false, LastActivity: base.Add(time.Hour), LastSeen: base.Add(time.Hour)},
			},
		},
		{
			name: "seen subject, older stamp: state left untouched, not cleared",
			rows: []provider.Notification{row("review/pr/1", base.Add(-time.Hour))},
			state: map[string]TriageEntry{
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base.Add(-2 * time.Hour)},
			},
			now:      base,
			wantKeys: []string{"review/pr/1"},
			wantState: map[string]TriageEntry{
				// Completely untouched: LastActivity and LastSeen keep
				// their pre-reconcile values rather than advancing to now,
				// distinguishing this row from the unchanged-stamp case
				// above.
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base.Add(-2 * time.Hour)},
			},
		},
		{
			name: "done rows are dropped from the returned slice but kept in state",
			rows: []provider.Notification{row("review/pr/1", base)},
			state: map[string]TriageEntry{
				"review/pr/1": {Done: true, LastActivity: base, LastSeen: base.Add(-time.Hour)},
			},
			now:      base,
			wantKeys: nil,
			wantState: map[string]TriageEntry{
				"review/pr/1": {Done: true, LastActivity: base, LastSeen: base},
			},
		},
		{
			name: "orphan exactly at the TTL boundary is kept",
			rows: nil,
			state: map[string]TriageEntry{
				"review/pr/1": {LastActivity: base, LastSeen: base.Add(-orphanTTL)},
			},
			now:      base,
			wantKeys: nil,
			wantState: map[string]TriageEntry{
				"review/pr/1": {LastActivity: base, LastSeen: base.Add(-orphanTTL)},
			},
		},
		{
			name: "orphan older than the TTL is pruned",
			rows: nil,
			state: map[string]TriageEntry{
				"review/pr/1": {LastActivity: base, LastSeen: base.Add(-orphanTTL - time.Nanosecond)},
			},
			now:       base,
			wantKeys:  nil,
			wantState: map[string]TriageEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRows, gotState := Reconcile(tt.rows, tt.state, tt.now)

			var gotKeys []string
			for _, r := range gotRows {
				gotKeys = append(gotKeys, r.Identity.ID)
			}
			if !reflect.DeepEqual(gotKeys, tt.wantKeys) {
				t.Errorf("returned row keys = %v, want %v", gotKeys, tt.wantKeys)
			}

			if len(gotState) != len(tt.wantState) {
				t.Fatalf("returned state = %+v, want %+v", gotState, tt.wantState)
			}
			for k, want := range tt.wantState {
				got, ok := gotState[k]
				if !ok {
					t.Errorf("returned state missing key %q", k)
					continue
				}
				if !entriesEqual(got, want) {
					t.Errorf("returned state[%q] = %+v, want %+v", k, got, want)
				}
			}
		})
	}
}

// TestReconcile_DoesNotMutateInputStateMap proves Reconcile is pure on the
// state side: the caller's map must be untouched after the call, since a
// previous defect in this store aliased a caller's map into live state
// (see TestTriageStore_Replace_DoesNotAliasCallerMap) and Reconcile's
// returned map is exactly what callers hand to Replace.
func TestReconcile_DoesNotMutateInputStateMap(t *testing.T) {
	base := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	input := map[string]TriageEntry{
		"review/pr/1": {Read: true, LastActivity: base, LastSeen: base},
	}
	rows := []provider.Notification{row("review/pr/1", base.Add(time.Hour))}

	_, gotState := Reconcile(rows, input, base.Add(time.Hour))

	want := TriageEntry{Read: true, LastActivity: base, LastSeen: base}
	if got := input["review/pr/1"]; !entriesEqual(got, want) {
		t.Errorf("input state map was mutated by Reconcile: got %+v, want %+v", got, want)
	}
	if len(input) != 1 {
		t.Errorf("input state map gained/lost keys: %+v", input)
	}

	// The returned map must actually differ from the input for this case
	// (newer stamp clears Read) — otherwise the "not mutated" assertion
	// above would be vacuous.
	if got := gotState["review/pr/1"]; got.Read {
		t.Errorf("returned state[review/pr/1].Read = true, want false (newer stamp should clear it)")
	}
}

// TestReconcile_DoesNotMutateInputRowsSlice proves Reconcile does not write
// through the input rows slice: the returned slice is fresh, so a caller
// still holding the original rows (e.g. a source that reuses a backing
// array across polls) cannot observe Reconcile's Read/Done fold-in.
func TestReconcile_DoesNotMutateInputRowsSlice(t *testing.T) {
	base := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	rows := []provider.Notification{row("review/pr/1", base)}
	state := map[string]TriageEntry{
		"review/pr/1": {Read: true, Done: true, LastActivity: base, LastSeen: base},
	}

	gotRows, _ := Reconcile(rows, state, base)

	if rows[0].Read || rows[0].Done {
		t.Errorf("input rows slice was mutated by Reconcile: %+v", rows[0])
	}
	// Done row is dropped from the result, confirming the read/done values
	// really were folded in somewhere rather than the test being vacuous.
	if len(gotRows) != 0 {
		t.Errorf("gotRows = %+v, want empty (done row should be dropped)", gotRows)
	}
}
