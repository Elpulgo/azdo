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
type TriageStore struct {
	path     string
	debounce time.Duration

	mu       sync.Mutex
	state    TriageState
	dirty    bool
	timer    *time.Timer
	writeErr error
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
// keep the suite fast; production callers should leave the default.
func (s *TriageStore) SetDebounce(d time.Duration) {
	s.mu.Lock()
	s.debounce = d
	s.mu.Unlock()
}

// State returns a snapshot of the current in-memory state.
func (s *TriageStore) State() TriageState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
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
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounce, s.flushAsync)
	s.mu.Unlock()
}

// flushAsync is invoked by the debounce timer.
func (s *TriageStore) flushAsync() {
	if err := s.Flush(); err != nil {
		s.mu.Lock()
		s.writeErr = err
		s.mu.Unlock()
	}
}

// Flush synchronously writes any pending state to disk. Safe to call even
// when there's nothing to flush — it returns nil in that case. Intended to
// be called on shutdown to guarantee durability.
func (s *TriageStore) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	snapshot := s.state
	s.mu.Unlock()

	data, err := snapshot.Marshal()
	if err != nil {
		return fmt.Errorf("marshal notifications state: %w", err)
	}

	if err := state.WriteAtomic(s.path, data); err != nil {
		return err
	}

	s.mu.Lock()
	s.dirty = false
	s.mu.Unlock()
	return nil
}

// LastWriteError returns the most recent error encountered by an
// asynchronous (debounced) write, or nil if none. Callers can poll this to
// surface persistence problems in the UI.
func (s *TriageStore) LastWriteError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}
