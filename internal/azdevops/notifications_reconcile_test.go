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
// since > and >= only differ on the equal case, and proven to survive
// orphanTTL pruning while still present in the feed -- the review's 🔴),
// zero UpdatedAt (no activity information), an empty Identity.ID row
// alongside a real one, done-row dropping, and TTL pruning with a boundary
// row exactly at the TTL (convention 13's shape).
func TestReconcile(t *testing.T) {
	base := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		rows      []provider.Notification
		state     TriageState
		now       time.Time
		wantKeys  []string
		wantState TriageState
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
			name: "seen subject, older stamp: triage state left untouched, LastSeen still advances",
			rows: []provider.Notification{row("review/pr/1", base.Add(-time.Hour))},
			state: map[string]TriageEntry{
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base.Add(-2 * time.Hour)},
			},
			now:      base,
			wantKeys: []string{"review/pr/1"},
			wantState: map[string]TriageEntry{
				// Triage state (Read/Done/LastActivity) keeps its
				// pre-reconcile values rather than clearing, distinguishing
				// this row from the newer-stamp case above. LastSeen is
				// presence bookkeeping, not triage, and DOES advance to
				// now — a regressed-stamp row that keeps appearing in
				// every poll must not freeze LastSeen, or the prune loop
				// below TTL-deletes it while the source keeps returning
				// it (the 🔴 review defect this test now pins).
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base},
			},
		},
		{
			name: "regressed-stamp row still present in the feed survives past orphanTTL",
			rows: []provider.Notification{row("review/pr/1", base.Add(-time.Hour))},
			state: map[string]TriageEntry{
				// LastSeen is already older than orphanTTL relative to
				// `now` below — if the older-stamp branch failed to
				// advance LastSeen (the reverted 🔴 defect), the prune
				// loop would delete this entry even though the row is
				// present in this very poll's rows, and the next poll
				// would resurrect it as a fresh, unread subject.
				"review/pr/1": {Done: true, LastActivity: base, LastSeen: base.Add(-orphanTTL - time.Hour)},
			},
			now: base,
			// Done, so dropped from the returned rows -- but it must
			// survive in state, not be pruned as orphaned.
			wantKeys: nil,
			wantState: map[string]TriageEntry{
				"review/pr/1": {Done: true, LastActivity: base, LastSeen: base},
			},
		},
		{
			name: "zero UpdatedAt: no activity information, stamp comparison skipped, LastSeen still advances",
			rows: []provider.Notification{row("review/pr/1", time.Time{})},
			state: map[string]TriageEntry{
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base.Add(-time.Hour)},
			},
			now:      base,
			wantKeys: []string{"review/pr/1"},
			wantState: map[string]TriageEntry{
				// Stored Read/Done/LastActivity are untouched -- a zero
				// stamp must not be read as "regressed to the beginning
				// of time" (which would still coincidentally work via the
				// older-stamp branch) nor as a spurious resurrection.
				// LastSeen still advances like every other branch.
				"review/pr/1": {Read: true, LastActivity: base, LastSeen: base},
			},
		},
		{
			name: "empty Identity.ID rows are dropped entirely, and do not disturb an unrelated real row",
			rows: []provider.Notification{
				row("", base),
				row("review/pr/1", base),
			},
			state: map[string]TriageEntry{
				"review/pr/1": {Read: true, Done: true, LastActivity: base, LastSeen: base.Add(-time.Hour)},
			},
			now: base,
			// The malformed row is dropped from the result and never
			// tracked in state; the real row's stored Done applies and
			// also drops it from the result (for the unrelated reason that
			// it's done) -- proving the two rows did not collapse into a
			// shared "" entry along the way.
			wantKeys: nil,
			wantState: map[string]TriageEntry{
				"review/pr/1": {Read: true, Done: true, LastActivity: base, LastSeen: base},
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
// previous defect in this store aliased a caller's map into live state (a
// since-removed Replace method — task 9 review's 🟡 finding 4 deleted it
// once Swap made it redundant everywhere) and Reconcile's returned map is
// exactly what Adapter.list hands to Swap.
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
