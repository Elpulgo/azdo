package github_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// newTestDoneStore creates a DoneStore backed by a file inside t.TempDir().
// The file does not exist yet — NewDoneStore's missing-file-is-empty-state
// contract is itself pinned by TestNewDoneStore_MissingFileIsEmptyState, so
// every other test can lean on it silently.
func newTestDoneStore(t *testing.T) (*github.DoneStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "github_notifications.yaml")
	s, err := github.NewDoneStore(path)
	if err != nil {
		t.Fatalf("NewDoneStore: %v", err)
	}
	return s, path
}

func notifRow(id string, updatedAt time.Time) provider.Notification {
	return provider.Notification{
		Identity: provider.Identity{
			Kind:  provider.KindGitHub,
			Scope: "o/r",
			ID:    id,
		},
		Title:     "t-" + id,
		UpdatedAt: updatedAt,
	}
}

func rowIDs(rows []provider.Notification) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Identity.ID
	}
	return out
}

func TestNewDoneStore_MissingFileIsEmptyState(t *testing.T) {
	s, _ := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	rows := []provider.Notification{notifRow("1", now.Add(-time.Hour))}
	got := s.Filter(rows, now)
	if len(got) != 1 {
		t.Fatalf("Filter() with no tombstones dropped rows: got %v, want [1]", rowIDs(got))
	}
}

func TestNewDoneStore_UnreadableFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github_notifications.yaml")
	if err := os.WriteFile(path, []byte(":\tnot yaml"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := github.NewDoneStore(path); err == nil {
		t.Fatal("NewDoneStore() error = nil for a corrupt file, want a parse error — silently starting fresh would discard every persisted tombstone")
	}
}

// ---------------------------------------------------------------------------
// The core contract: a marked-done thread stays out of the feed on every
// later fetch that carries no new activity — the exact resurrection bug the
// store exists to fix (GitHub's all=true response keeps returning done
// threads with no field saying so).
// ---------------------------------------------------------------------------

func TestDoneStore_MarkDone_HidesRowOnLaterFetches(t *testing.T) {
	s, _ := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	updated := now.Add(-time.Hour)
	rows := []provider.Notification{notifRow("1", updated), notifRow("2", updated)}

	// The fetch the user is looking at when they press d.
	if got := s.Filter(rows, now); len(got) != 2 {
		t.Fatalf("pre-mark Filter() = %v, want both rows", rowIDs(got))
	}

	s.MarkDone("1", now)

	// The very next poll returns the thread again, stamp unchanged — GitHub's
	// all=true behaviour for a done thread with no new activity.
	got := s.Filter(rows, now.Add(time.Minute))
	if len(got) != 1 || got[0].Identity.ID != "2" {
		t.Fatalf("post-mark Filter() = %v, want [2] — the marked-done row must stay hidden", rowIDs(got))
	}
}

func TestDoneStore_TombstoneSurvivesReload(t *testing.T) {
	s, path := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	updated := now.Add(-time.Hour)
	rows := []provider.Notification{notifRow("1", updated)}

	s.Filter(rows, now)
	s.MarkDone("1", now)
	if err := s.LastWriteError(); err != nil {
		t.Fatalf("LastWriteError() after MarkDone = %v", err)
	}

	// A new store from the same path — the next app launch.
	reloaded, err := github.NewDoneStore(path)
	if err != nil {
		t.Fatalf("NewDoneStore(reload): %v", err)
	}
	got := reloaded.Filter(rows, now.Add(time.Hour))
	if len(got) != 0 {
		t.Fatalf("reloaded Filter() = %v, want [] — the tombstone must survive a restart, that is the whole point of persisting it", rowIDs(got))
	}
}

// ---------------------------------------------------------------------------
// Resurrection: new server-side activity (a strictly newer UpdatedAt) drops
// the tombstone and the row resurfaces — matching GitHub's own web behaviour
// of re-inboxing a done thread on new activity. Equal, older and zero stamps
// all stay hidden: none of them is evidence of anything new.
// ---------------------------------------------------------------------------

func TestDoneStore_Filter_ActivityStamps(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	markStamp := now.Add(-time.Hour)

	tests := []struct {
		name      string
		refetchAt time.Time
		wantShown bool
	}{
		{name: "strictly newer UpdatedAt resurfaces the row", refetchAt: markStamp.Add(time.Second), wantShown: true},
		{name: "equal UpdatedAt stays hidden", refetchAt: markStamp, wantShown: false},
		{name: "older UpdatedAt stays hidden", refetchAt: markStamp.Add(-time.Second), wantShown: false},
		{name: "zero UpdatedAt stays hidden", refetchAt: time.Time{}, wantShown: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestDoneStore(t)
			s.Filter([]provider.Notification{notifRow("1", markStamp)}, now)
			s.MarkDone("1", now)

			got := s.Filter([]provider.Notification{notifRow("1", tt.refetchAt)}, now.Add(time.Minute))
			shown := len(got) == 1
			if shown != tt.wantShown {
				t.Fatalf("Filter() shown = %v, want %v (mark stamp %v, refetched UpdatedAt %v)", shown, tt.wantShown, markStamp, tt.refetchAt)
			}
		})
	}
}

func TestDoneStore_Resurrection_DeletesTombstonePersistently(t *testing.T) {
	s, path := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	markStamp := now.Add(-time.Hour)
	s.Filter([]provider.Notification{notifRow("1", markStamp)}, now)
	s.MarkDone("1", now)

	// New activity resurrects the row...
	newer := markStamp.Add(time.Minute)
	if got := s.Filter([]provider.Notification{notifRow("1", newer)}, now.Add(time.Minute)); len(got) != 1 {
		t.Fatalf("Filter() after new activity = %v, want the row back", rowIDs(got))
	}

	// ...and the deletion is persisted: a reloaded store must not still be
	// carrying the stale tombstone, or a restart would re-hide a row the
	// session already resurrected (until its next activity bumped it again).
	reloaded, err := github.NewDoneStore(path)
	if err != nil {
		t.Fatalf("NewDoneStore(reload): %v", err)
	}
	if got := reloaded.Filter([]provider.Notification{notifRow("1", newer)}, now.Add(2*time.Minute)); len(got) != 1 {
		t.Fatalf("reloaded Filter() = %v, want the row shown — the resurrection must have been persisted", rowIDs(got))
	}
}

// ---------------------------------------------------------------------------
// The mark stamp comes from the thread's own UpdatedAt as recorded by the
// most recent Filter that saw it — a server-clock stamp — not from the local
// wall clock passed to MarkDone. The distinction is what keeps the comparison
// on one clock: with a local clock behind GitHub's, stamping local time would
// make the very next unchanged fetch look like "new activity" and resurrect
// the row immediately — reintroducing the original bug for clock-behind users.
// ---------------------------------------------------------------------------

func TestDoneStore_MarkDone_StampsThreadUpdatedAtNotLocalClock(t *testing.T) {
	s, _ := newTestDoneStore(t)

	// Local clock runs 2 minutes behind the server: the thread's UpdatedAt is
	// "in the future" relative to every local now.
	serverUpdated := time.Date(2026, 8, 1, 12, 5, 0, 0, time.UTC)
	localNow := time.Date(2026, 8, 1, 12, 3, 0, 0, time.UTC)

	s.Filter([]provider.Notification{notifRow("1", serverUpdated)}, localNow)
	s.MarkDone("1", localNow)

	// Next poll: same thread, same UpdatedAt, no new activity. Were the
	// tombstone stamped with localNow, serverUpdated.After(localNow) would
	// resurrect it here.
	got := s.Filter([]provider.Notification{notifRow("1", serverUpdated)}, localNow.Add(30*time.Second))
	if len(got) != 0 {
		t.Fatalf("Filter() = %v, want [] — the tombstone must be stamped with the thread's own UpdatedAt, not the (skewed) local clock", rowIDs(got))
	}
}

func TestDoneStore_MarkDone_UnseenIDFallsBackToNow(t *testing.T) {
	s, _ := newTestDoneStore(t)

	// No Filter call has ever seen id "1" — unreachable through the UI, but
	// the store must still behave sanely: the fallback stamps now, so a fetch
	// with an UpdatedAt at-or-before now stays hidden.
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.MarkDone("1", now)

	got := s.Filter([]provider.Notification{notifRow("1", now.Add(-time.Hour))}, now.Add(time.Minute))
	if len(got) != 0 {
		t.Fatalf("Filter() = %v, want [] — an unseen id's tombstone falls back to stamping now", rowIDs(got))
	}
}

func TestDoneStore_MarkDone_EmptyIDIsRejected(t *testing.T) {
	s, path := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.MarkDone("", now)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(%q) err = %v, want not-exist — an empty id must write nothing", path, err)
	}
}

// ---------------------------------------------------------------------------
// Pruning: a tombstone is forgotten only when its thread is BOTH absent from
// the current fetch AND older than the TTL. Either condition alone must keep
// it — a still-present thread's tombstone is the feature itself, and a
// recently-absent thread may only be hidden by a narrowed query shape
// (participating_only toggled, since window shifted).
// ---------------------------------------------------------------------------

func TestDoneStore_Filter_PruneRequiresAbsentAndExpired(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	markStamp := now.Add(-time.Hour)
	overTTL := now.Add(31 * 24 * time.Hour)  // past the 30-day TTL
	underTTL := now.Add(29 * 24 * time.Hour) // inside it

	tests := []struct {
		name    string
		fetchAt time.Time
		present bool
		// wantHiddenOnReturn: whether a follow-up fetch that DOES contain the
		// thread (unchanged stamp) still hides it — i.e. whether the
		// tombstone survived.
		wantSurvived bool
	}{
		{name: "absent and expired: pruned", fetchAt: overTTL, present: false, wantSurvived: false},
		{name: "absent but not expired: kept", fetchAt: underTTL, present: false, wantSurvived: true},
		{name: "present and expired: kept", fetchAt: overTTL, present: true, wantSurvived: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestDoneStore(t)
			s.Filter([]provider.Notification{notifRow("1", markStamp)}, now)
			s.MarkDone("1", now)

			var fetch []provider.Notification
			if tt.present {
				fetch = []provider.Notification{notifRow("1", markStamp)}
			}
			s.Filter(fetch, tt.fetchAt)

			// Probe: the thread reappears with its stamp unchanged. A
			// surviving tombstone hides it; a pruned one lets it through.
			got := s.Filter([]provider.Notification{notifRow("1", markStamp)}, tt.fetchAt.Add(time.Minute))
			survived := len(got) == 0
			if survived != tt.wantSurvived {
				t.Fatalf("tombstone survived = %v, want %v", survived, tt.wantSurvived)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Slice contracts, mirroring Adapter.List's own: the returned slice is always
// freshly allocated and non-nil (an all-tombstoned inbox filters down to the
// same empty-but-non-nil shape a genuinely empty inbox has), and the input
// slice is never mutated.
// ---------------------------------------------------------------------------

func TestDoneStore_Filter_AllRowsDropped_ReturnsNonNilEmptySlice(t *testing.T) {
	s, _ := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	rows := []provider.Notification{notifRow("1", now.Add(-time.Hour))}
	s.Filter(rows, now)
	s.MarkDone("1", now)

	got := s.Filter(rows, now.Add(time.Minute))
	if got == nil {
		t.Fatal("Filter() = nil, want an empty-but-non-nil slice — nil reads as an error/skipped-fetch shape downstream, not \"you're clear\"")
	}
	if len(got) != 0 {
		t.Fatalf("Filter() = %v, want []", rowIDs(got))
	}
}

func TestDoneStore_Filter_DoesNotMutateInput(t *testing.T) {
	s, _ := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	rows := []provider.Notification{
		notifRow("1", now.Add(-time.Hour)),
		notifRow("2", now.Add(-time.Hour)),
	}
	s.Filter(rows, now)
	s.MarkDone("1", now)

	// "1" is dropped; an aliasing filter (rows[:0]-style) would shift "2"
	// into rows[0] here.
	got := s.Filter(rows, now.Add(time.Minute))
	if len(got) != 1 || got[0].Identity.ID != "2" {
		t.Fatalf("Filter() = %v, want [2]", rowIDs(got))
	}
	if rows[0].Identity.ID != "1" || rows[1].Identity.ID != "2" {
		t.Fatalf("input slice was mutated: %v, want [1 2]", rowIDs(rows))
	}
}

func TestDoneStore_MarkDone_RepeatedSameStampDoesNotRewriteFile(t *testing.T) {
	s, path := newTestDoneStore(t)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.Filter([]provider.Notification{notifRow("1", now.Add(-time.Hour))}, now)
	s.MarkDone("1", now)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("os.Stat after first MarkDone: %v — the first mark must have written the file", err)
	}

	// Same id, same recorded stamp: a no-op that must not touch the file.
	// Deleting the file first makes "did it write?" directly observable
	// rather than inferred from mtime granularity.
	if err := os.Remove(path); err != nil {
		t.Fatalf("os.Remove: %v", err)
	}
	s.MarkDone("1", now.Add(time.Minute))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("repeated MarkDone rewrote the file (stat err = %v), want no write for an unchanged tombstone", err)
	}
}
