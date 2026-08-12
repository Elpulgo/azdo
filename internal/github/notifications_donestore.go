package github

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/state"
	"gopkg.in/yaml.v3"
)

// doneFileName is the on-disk name of the local mark-done tombstone store,
// kept alongside state.yaml and azdevops's notifications.yaml via
// state.PathFor — same directory resolution, different file name. It is a
// separate file from azdevops's notifications.yaml, not a shared one: the two
// backends' local stores have different shapes (Azure tracks read AND done
// because it has no server-side inbox at all; GitHub tracks done only,
// because read state round-trips through the server just fine) and neither
// package imports the other.
const doneFileName = "github_notifications.yaml"

// doneOrphanTTL is how long a DoneEntry survives after its thread last
// appeared in a fetch before Filter prunes it. Mirrors azdevops's orphanTTL
// (30 days) in role and value.
//
// A tombstone whose thread IS still returned by GET /notifications is never
// pruned, regardless of age — pruning it while GitHub still returns the
// thread would resurrect the exact row this store exists to keep hidden. The
// TTL only applies to tombstones whose thread was absent from the current
// fetch, and absence alone is deliberately not enough either: the fetch shape
// is config-driven (participating_only, since_days), so a thread can be
// temporarily absent because the query narrowed — e.g. participating_only
// toggled on for a few sessions — and pruning on first absence would
// resurrect it the moment the query widens again. Absent AND expired is the
// only combination that is safe to forget.
const doneOrphanTTL = 30 * 24 * time.Hour

// DoneEntry is one locally persisted mark-done tombstone for a GitHub
// notification thread.
//
// This store exists because GitHub's REST API can WRITE done state but never
// READ it back: DELETE /notifications/threads/{id} marks a thread done, but
// GET /notifications?all=true — the only fetch shape NotificationsClient.List
// uses, deliberately, so merely-read rows don't vanish from the feed — keeps
// returning done threads with no field distinguishing them from live ones
// (done state exists only in GitHub's web UI / internal APIs). Without a
// local record, every mark-done row resurrects on the next full fetch — which
// is exactly the bug this store fixes.
type DoneEntry struct {
	// LastActivity is the thread's UpdatedAt as of the fetch the mark acted
	// on — a server-side stamp, compared by Filter against later fetches'
	// equally server-side UpdatedAt, so the comparison never crosses the
	// local/server clock boundary. (DoneStore.MarkDone falls back to the
	// local clock only when the id was never seen by a Filter call, which no
	// UI-driven mark can hit — see its doc comment.) A fetched UpdatedAt
	// strictly newer than this means new activity since the mark: GitHub
	// itself re-inboxes a done thread on new activity, so the tombstone is
	// dropped and the row resurfaces rather than being silently swallowed.
	LastActivity time.Time `yaml:"last_activity"`

	// MarkedAt is the local wall-clock time of the mark. It is prune
	// bookkeeping only (see doneOrphanTTL) and is never compared against a
	// thread's server-side UpdatedAt.
	MarkedAt time.Time `yaml:"marked_at"`
}

// DoneState is the full on-disk shape of github_notifications.yaml: a map
// from wire thread id (NotificationThread.ID, the same opaque decimal string
// stamped into provider.Identity.ID) to its tombstone.
type DoneState map[string]DoneEntry

// DoneStorePath returns the on-disk location of github_notifications.yaml,
// derived the same way state.yaml's path is (honoring $XDG_STATE_HOME) via
// state.PathFor, so it always sits alongside the other state files.
func DoneStorePath() (string, error) {
	return state.PathFor(doneFileName)
}

// LoadDoneState reads and parses github_notifications.yaml. A missing file is
// not an error — callers receive an empty DoneState and start fresh.
func LoadDoneState(path string) (DoneState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DoneState{}, nil
		}
		return nil, fmt.Errorf("read github notifications state: %w", err)
	}
	var s DoneState
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse github notifications state: %w", err)
	}
	if s == nil {
		s = DoneState{}
	}
	return s, nil
}

// DoneStore owns the persisted mark-done tombstones for GitHub notification
// threads (see DoneEntry for why they exist at all) plus one piece of
// in-memory-only bookkeeping: lastFetched, the UpdatedAt of every thread the
// most recent Filter calls have seen, which is what lets MarkDone stamp a
// tombstone with the thread's own server-side activity time instead of the
// local clock.
//
// Unlike azdevops.TriageStore, writes here are synchronous (marshal +
// state.WriteAtomic under mu) with no debounce machinery. The write pattern
// justifies the difference: TriageStore is written by Reconcile on every
// poll (LastSeen advances each tick), so uncoalesced writes there would hit
// disk once per poll forever. This store is written only when something
// actually changed — a user mark, a resurrection, a prune — all rare, so a
// synchronous write per change is cheap and removes the need for a
// shutdown Flush entirely: there is never a pending write to lose.
//
// mu guards state, lastFetched and writeErr. It is never held across an HTTP
// request — Filter and MarkDone are called by the Adapter strictly after
// nc.List/nc.MarkDone return — so it cannot reintroduce the fetch-vs-mark
// blocking that NotificationsClient's lock-free marker design exists to
// avoid. It IS held across the yaml marshal + atomic file write, which
// serializes writers and keeps a stale snapshot from ever landing after a
// newer one (the out-of-order rename hazard TriageStore needs a second
// writeMu for) at the cost of a millisecond-scale file write on the rare
// mutating call.
type DoneStore struct {
	path string

	mu    sync.Mutex
	state DoneState

	// lastFetched maps thread id → UpdatedAt as of the most recent Filter
	// call that saw it. Entries are merged in (never cleared wholesale): a
	// row can drop out of one fetch and still be the row a slightly stale UI
	// render marks done a moment later, and a stale stamp errs in the safe
	// direction — an older LastActivity only makes the tombstone easier to
	// resurrect, never harder. In-memory only; bounded by the number of
	// distinct threads seen this session.
	lastFetched map[string]time.Time

	writeErr error
}

// NewDoneStore creates a DoneStore seeded with the contents of the file at
// path. A missing file is treated as empty state — not an error.
func NewDoneStore(path string) (*DoneStore, error) {
	loaded, err := LoadDoneState(path)
	if err != nil {
		return nil, err
	}
	return &DoneStore{
		path:        path,
		state:       loaded,
		lastFetched: map[string]time.Time{},
	}, nil
}

// Filter applies the tombstones to a freshly fetched-and-mapped set of rows:
// rows with no tombstone pass through; a tombstoned row whose UpdatedAt is
// strictly newer than the tombstone's LastActivity has new activity since the
// mark, so the tombstone is deleted and the row resurfaces (matching GitHub's
// own web behaviour of re-inboxing a done thread on new activity); every
// other tombstoned row — equal stamp, older stamp (clock-skewed or reordered
// server data), or a zero UpdatedAt carrying no activity information at all —
// stays hidden. It also records every row's UpdatedAt into lastFetched (see
// that field's doc comment) and prunes tombstones that are both absent from
// this fetch and older than doneOrphanTTL (see the constant's doc comment for
// why both conditions are required).
//
// The returned slice is always freshly allocated and non-nil, and rows is
// never mutated — matching Adapter.List's existing contract that an empty
// inbox is an empty-but-non-nil slice, never nil (a nil result reads as an
// error/skipped-fetch shape downstream, not "you're clear"). An inbox whose
// every row is tombstoned legitimately filters down to that same empty
// non-nil shape.
//
// Changes to the tombstone map (resurrections, prunes) are persisted before
// returning; a call that changes nothing writes nothing.
func (s *DoneStore) Filter(rows []provider.Notification, now time.Time) []provider.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()

	present := make(map[string]bool, len(rows))
	for _, row := range rows {
		present[row.Identity.ID] = true
		s.lastFetched[row.Identity.ID] = row.UpdatedAt
	}

	changed := false
	out := make([]provider.Notification, 0, len(rows))
	for _, row := range rows {
		entry, done := s.state[row.Identity.ID]
		if !done {
			out = append(out, row)
			continue
		}
		if row.UpdatedAt.After(entry.LastActivity) {
			delete(s.state, row.Identity.ID)
			changed = true
			out = append(out, row)
			continue
		}
		// Still done: equal or older stamp means no new activity since the
		// mark (an older or zero stamp carries no evidence of any), so the
		// row stays hidden.
	}

	for id, entry := range s.state {
		if !present[id] && now.Sub(entry.MarkedAt) > doneOrphanTTL {
			delete(s.state, id)
			changed = true
		}
	}

	if changed {
		s.persistLocked()
	}
	return out
}

// MarkDone records a tombstone for id, stamped with the thread's own
// server-side UpdatedAt as last recorded by Filter. The Adapter calls this
// only AFTER the server's DELETE succeeded — a failed server mark must not
// write a tombstone, or the row would be hidden locally while GitHub still
// counts it as live inbox state.
//
// The lastFetched lookup can only miss for a caller marking an id no Filter
// call has ever seen — unreachable through the UI, which only marks rows it
// rendered from a List (and every List routes through Filter). The fallback
// for that case stamps the local clock instead, accepting the cross-clock
// comparison the primary path avoids: with a local clock behind GitHub's, a
// pre-mark update can look "newer than the mark" and resurrect the row one
// extra time (safe direction — a stray extra row, not a swallowed one).
//
// An id that already carries a tombstone with this same stamp is a no-op:
// nothing changes in memory and nothing is written, so a caller repeating the
// mark (the UI already debounces, but nothing here relies on that) cannot
// churn the file. An empty id is rejected outright — it can never have come
// from a successful server mark (nc.MarkDone validates the id shape before
// any request), and writing it would create a single entry every malformed
// caller collapses into.
func (s *DoneStore) MarkDone(id string, now time.Time) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	stamp := s.lastFetched[id]
	if stamp.IsZero() {
		stamp = now
	}
	if entry, exists := s.state[id]; exists && entry.LastActivity.Equal(stamp) {
		return
	}
	s.state[id] = DoneEntry{LastActivity: stamp, MarkedAt: now}
	s.persistLocked()
}

// LastWriteError returns the most recent error encountered writing the store
// to disk, or nil if the last write succeeded. Writes are best-effort by
// design: a failed persist leaves the in-memory tombstones fully effective
// for the rest of the session (the mark already succeeded server-side, so
// failing the user-visible action over a local bookkeeping write would be
// wrong), and the next successful write carries the full current state.
// Mirrors azdevops.TriageStore.LastWriteError in shape and intent.
func (s *DoneStore) LastWriteError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}

// persistLocked marshals and atomically writes the current state. Must be
// called with s.mu held — holding it across the write is what serializes
// snapshots and prevents an older one renaming into place after a newer one
// (see the type-level doc comment). Errors are recorded on writeErr rather
// than returned; see LastWriteError for why they do not propagate.
func (s *DoneStore) persistLocked() {
	data, err := yaml.Marshal(s.state)
	if err != nil {
		s.writeErr = fmt.Errorf("marshal github notifications state: %w", err)
		return
	}
	if err := state.WriteAtomic(s.path, data); err != nil {
		s.writeErr = err
		return
	}
	s.writeErr = nil
}
