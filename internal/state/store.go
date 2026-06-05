package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// defaultDebounce is how long Apply waits for further changes before
// flushing to disk. Tuned to keep navigation-time writes off the hot path
// while bounding loss-on-crash to a single user action.
const defaultDebounce = 500 * time.Millisecond

// Store owns the in-memory state and coordinates debounced writes to disk.
// It is safe for concurrent use.
type Store struct {
	path     string
	debounce time.Duration

	mu       sync.Mutex
	state    State
	dirty    bool
	timer    *time.Timer
	writeErr error
}

// NewStore creates a Store seeded with the contents of the file at path.
// A missing file is treated as empty state — not an error.
func NewStore(path string) (*Store, error) {
	loaded, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Store{
		path:     path,
		debounce: defaultDebounce,
		state:    loaded,
	}, nil
}

// SetDebounce overrides the default debounce duration. Tests use this to
// keep the suite fast; production callers should leave the default.
func (s *Store) SetDebounce(d time.Duration) {
	s.mu.Lock()
	s.debounce = d
	s.mu.Unlock()
}

// State returns a snapshot of the current in-memory state.
func (s *Store) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Apply mutates the in-memory state under the lock and schedules a
// debounced write. Multiple Apply calls within the debounce window
// coalesce into a single write.
func (s *Store) Apply(mutate func(*State)) {
	s.mu.Lock()
	mutate(&s.state)
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounce, s.flushAsync)
	s.mu.Unlock()
}

// flushAsync is invoked by the debounce timer.
func (s *Store) flushAsync() {
	if err := s.Flush(); err != nil {
		s.mu.Lock()
		s.writeErr = err
		s.mu.Unlock()
	}
}

// Flush synchronously writes any pending state to disk. Safe to call
// even when there's nothing to flush — it returns nil in that case.
// Intended to be called on shutdown to guarantee durability.
func (s *Store) Flush() error {
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
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := writeAtomic(s.path, data); err != nil {
		return err
	}

	s.mu.Lock()
	s.dirty = false
	s.mu.Unlock()
	return nil
}

// LastWriteError returns the most recent error encountered by an
// asynchronous (debounced) write, or nil if none. Callers can poll this
// to surface persistence problems in the UI.
func (s *Store) LastWriteError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}

// writeAtomic writes data to path via a temp file + rename, so a crash
// mid-write cannot leave a half-written state.yaml behind.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	f, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
