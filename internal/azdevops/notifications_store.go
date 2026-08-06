package azdevops

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/state"
	"gopkg.in/yaml.v3"
)

// notifFileName is the on-disk name of the local triage store, kept
// alongside state.yaml but owned by its own state.Store-shaped instance
// (decision 1 of the phase-2 notifications spec): navigation state is
// disposable, triage state is not, so the two live in different files with
// different lifetimes.
const notifFileName = "notifications.yaml"

// TriageEntry is the locally tracked read/done state for one synthetic
// notification identity (decision 2's key), plus the activity stamp used to
// decide whether a subject's current activity is new enough to resurrect it.
type TriageEntry struct {
	Read bool `yaml:"read"`
	Done bool `yaml:"done"`

	// LastActivity is the subject's activity timestamp as of the last time
	// it was reconciled (e.g. a PR's last-update time, a mention comment's
	// createdDate). Advancing it is what clears Read/Done on new activity.
	LastActivity time.Time `yaml:"last_activity"`

	// LastSeen is when this entry was last observed in a poll, independent
	// of whether its activity changed. Used for TTL pruning of orphaned
	// entries whose subject has stopped matching any source's query.
	LastSeen time.Time `yaml:"last_seen"`
}

// TriageState is the full on-disk shape of notifications.yaml: a map from
// identity key (Identity.ID, see NotifKey) to its triage entry.
type TriageState map[string]TriageEntry

// Marshal encodes the state as YAML.
func (s TriageState) Marshal() ([]byte, error) {
	return yaml.Marshal(s)
}

// Unmarshal parses YAML into the receiver.
func (s *TriageState) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, s)
}

// clone returns a deep copy of s. TriageEntry is all value types (two
// bools, two time.Time), so a per-key copy into a fresh map is a true
// snapshot — the caller cannot observe or cause mutation of the original
// via the result.
func (s TriageState) clone() TriageState {
	if s == nil {
		return nil
	}
	c := make(TriageState, len(s))
	for k, v := range s {
		c[k] = v
	}
	return c
}

// NotifStorePath returns the on-disk location of notifications.yaml. It is
// derived the same way state.yaml's path is — same directory resolution
// (honoring $XDG_STATE_HOME), different file name — via state.PathFor, so
// the two files always sit side by side.
func NotifStorePath() (string, error) {
	return state.PathFor(notifFileName)
}

// LoadTriageState reads and parses notifications.yaml. A missing file is
// not an error — callers receive an empty TriageState and start fresh.
func LoadTriageState(path string) (TriageState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TriageState{}, nil
		}
		return nil, fmt.Errorf("read notifications state: %w", err)
	}
	var s TriageState
	if err := s.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("parse notifications state: %w", err)
	}
	if s == nil {
		s = TriageState{}
	}
	return s, nil
}

// defaultTriageDebounce mirrors state.Store's debounce window.
const defaultTriageDebounce = 500 * time.Millisecond

// TriageStore owns the in-memory local triage state and coordinates
// debounced writes to notifications.yaml. It mirrors state.Store's shape and
// atomicity guarantee (via state.WriteAtomic) but is its own instance,
// keyed on TriageState rather than state.State — a sibling store, not a
// shared one, per decision 1.
//
// Unlike state.State (a struct, copied by value on every assignment),
// TriageState is a map — a reference type. Every place that would be a safe
// value-copy for state.Store (State(), the Flush snapshot) has to be an
// explicit deep copy here, or callers and the debounced writer end up
// sharing the same live map. gen tracks how many times the in-memory state
// has changed since the last successful write, so a Flush that raced an
// Apply mid-write can tell whether the copy it just persisted is still
// current before it clears dirty.
type TriageStore struct {
	path     string
	debounce time.Duration

	mu       sync.Mutex
	state    TriageState
	dirty    bool
	gen      uint64
	timer    *time.Timer
	writeErr error

	// writeMu serializes the snapshot-and-write sequence in Flush across
	// concurrent Flush calls. mu alone is not enough: two overlapping
	// Flush calls each take a snapshot and write it to the same path
	// without holding mu across the write, so their os.Rename calls can
	// land out of order — a stale, smaller snapshot finishing (and
	// renaming) after a newer, larger one would silently regress the file
	// on disk even though neither individual write corrupted the
	// in-memory map. Always acquired before mu, never the reverse.
	writeMu sync.Mutex
}

// NewTriageStore creates a TriageStore seeded with the contents of the file
// at path. A missing file is treated as empty state — not an error.
func NewTriageStore(path string) (*TriageStore, error) {
	loaded, err := LoadTriageState(path)
	if err != nil {
		return nil, err
	}
	return &TriageStore{
		path:     path,
		debounce: defaultTriageDebounce,
		state:    loaded,
	}, nil
}

// SetDebounce overrides the default debounce duration. Tests use this to
// keep the suite fast; production callers should leave the default. A
// non-positive duration is rejected and leaves the existing debounce
// untouched — time.AfterFunc(0 or negative, ...) fires immediately, which
// would silently disable the coalescing Apply exists to provide (convention
// 11's `<= 0` guard shape).
func (s *TriageStore) SetDebounce(d time.Duration) {
	if d <= 0 {
		return
	}
	s.mu.Lock()
	s.debounce = d
	s.mu.Unlock()
}

// State returns a snapshot of the current in-memory state. The returned map
// is a copy: mutating it does not affect the store, and does not mark it
// dirty. Callers that want to persist a change must go through Apply or
// Replace.
func (s *TriageStore) State() TriageState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.clone()
}

// Apply mutates the in-memory state under the lock and schedules a
// debounced write. Multiple Apply calls within the debounce window coalesce
// into a single write.
func (s *TriageStore) Apply(mutate func(TriageState)) {
	s.mu.Lock()
	if s.state == nil {
		s.state = TriageState{}
	}
	mutate(s.state)
	s.markDirtyLocked()
	s.mu.Unlock()
}

// Replace atomically swaps the entire in-memory state with newState and
// schedules a debounced write. Intended for callers — like the identity-key
// reconcile function — that compute a fresh map (e.g. dropping pruned or
// done entries) rather than mutating one in place: writing that back through
// Apply would force a hand-rolled clear-then-copy over the live map, which
// is exactly the mutation-in-place shape a pure reconcile function exists to
// avoid.
func (s *TriageStore) Replace(newState TriageState) {
	if newState == nil {
		newState = TriageState{}
	}
	s.mu.Lock()
	// Store a deep copy, not the caller's live map. Reconcile hands back a
	// freshly built map, but aliasing it here would let the caller's later
	// mutation of that map race the store's own reads/writes of s.state —
	// exactly the "concurrent map iteration and map write" fatal error a
	// previous pass of this store already fixed for Flush's snapshot.
	s.state = newState.clone()
	s.markDirtyLocked()
	s.mu.Unlock()
}

// markDirtyLocked marks the state dirty, bumps the generation counter and
// (re-)arms the debounce timer. Must be called with s.mu held.
func (s *TriageStore) markDirtyLocked() {
	s.dirty = true
	s.gen++
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounce, s.flushAsync)
}

// flushAsync is invoked by the debounce timer. Flush already records any
// write error on the store, so the return value is intentionally ignored
// here.
func (s *TriageStore) flushAsync() {
	_ = s.Flush()
}

// Flush synchronously writes any pending state to disk. Safe to call even
// when there's nothing to flush — it returns nil in that case. Intended to
// be called on shutdown to guarantee durability.
//
// The snapshot taken here is a deep copy made under the lock, not an alias
// of the live map — TriageState is a reference type, so handing yaml.Marshal
// the live map and only then releasing the lock would let a concurrent
// Apply mutate the map mid-iteration (a fatal "concurrent map iteration and
// map write", not merely a data race). gen records how many changes the
// in-memory state has seen as of the snapshot; if an Apply/Replace lands
// while the write is in flight, gen has moved on by the time Flush
// re-acquires the lock, so dirty is deliberately left set (that Apply/
// Replace call has already re-armed the timer to retry) rather than being
// cleared out from under the update it raced.
func (s *TriageStore) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	dirty := s.dirty
	s.mu.Unlock()
	if !dirty {
		return nil
	}

	// Serialize the snapshot-and-write sequence: see writeMu's doc comment
	// on the struct for why mu alone doesn't prevent an out-of-order write.
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.Lock()
	if !s.dirty {
		// A Flush call that acquired writeMu ahead of us already
		// persisted everything this call would have written.
		s.mu.Unlock()
		return nil
	}
	snapshotGen := s.gen
	snapshot := s.state.clone()
	s.mu.Unlock()

	data, err := snapshot.Marshal()
	if err != nil {
		werr := fmt.Errorf("marshal notifications state: %w", err)
		s.mu.Lock()
		s.writeErr = werr
		s.rearmLocked()
		s.mu.Unlock()
		return werr
	}

	if err := state.WriteAtomic(s.path, data); err != nil {
		s.mu.Lock()
		s.writeErr = err
		s.rearmLocked()
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	if s.gen == snapshotGen {
		s.dirty = false
	}
	s.writeErr = nil
	s.mu.Unlock()
	return nil
}

// rearmLocked re-arms the debounce timer after a failed write, so a
// transient failure (e.g. a full disk) gets retried instead of silently
// dropping the pending change on the floor. Must be called with s.mu held;
// dirty is left true by the caller (it was true on entry to Flush and the
// write did not succeed).
func (s *TriageStore) rearmLocked() {
	s.timer = time.AfterFunc(s.debounce, s.flushAsync)
}

// LastWriteError returns the most recent error encountered by an
// asynchronous (debounced) write, or nil if none. Callers can poll this to
// surface persistence problems in the UI.
func (s *TriageStore) LastWriteError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}
