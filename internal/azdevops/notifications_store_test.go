package azdevops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/state"
)

// waitFor polls a condition with a short interval, failing the test if it
// doesn't become true within the timeout. Used for debounced writes. Ported
// from internal/state/store_test.go's helper of the same name.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", desc)
}

// TestTriageEntry_YAMLRoundTrip ensures the entry shape survives a YAML
// round trip, including the two timestamp fields.
func TestTriageEntry_YAMLRoundTrip(t *testing.T) {
	original := TriageState{
		"review/pr/1234": {
			Read:         true,
			Done:         false,
			LastActivity: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
			LastSeen:     time.Date(2026, 8, 2, 9, 30, 0, 0, time.UTC),
		},
	}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var parsed TriageState
	if err := parsed.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	got := parsed["review/pr/1234"]
	want := original["review/pr/1234"]
	if !got.Read || got.Done || !got.LastActivity.Equal(want.LastActivity) || !got.LastSeen.Equal(want.LastSeen) {
		t.Errorf("round trip mismatch:\n  got  = %+v\n  want = %+v", got, want)
	}
}

func TestNotifStorePath_DiffersFromStatePathInSameDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	statePath, err := state.Path()
	if err != nil {
		t.Fatalf("state.Path() error = %v", err)
	}
	notifPath, err := NotifStorePath()
	if err != nil {
		t.Fatalf("NotifStorePath() error = %v", err)
	}

	if notifPath == statePath {
		t.Fatalf("NotifStorePath() = %q, want different path from state.Path()", notifPath)
	}
	if filepath.Dir(notifPath) != filepath.Dir(statePath) {
		t.Errorf("NotifStorePath() dir = %q, want same dir as state.Path() %q",
			filepath.Dir(notifPath), filepath.Dir(statePath))
	}
	if filepath.Base(notifPath) != "notifications.yaml" {
		t.Errorf("NotifStorePath() base = %q, want %q", filepath.Base(notifPath), "notifications.yaml")
	}
}

func TestNewTriageStore_MissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	if got := store.State(); len(got) != 0 {
		t.Errorf("State() = %+v, want empty", got)
	}
}

func TestNewTriageStore_LoadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.yaml")
	contents := `mention/wi/5678:
  read: true
  done: false
  last_activity: 2026-08-01T12:00:00Z
  last_seen: 2026-08-02T09:30:00Z
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	entry, ok := store.State()["mention/wi/5678"]
	if !ok {
		t.Fatalf("State() missing seeded key, got %+v", store.State())
	}
	if !entry.Read || entry.Done {
		t.Errorf("entry = %+v, want Read=true Done=false", entry)
	}
}

func TestTriageStore_ApplyAndFlushRoundTripsThroughAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(10 * time.Millisecond)

	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	store.Apply(func(s TriageState) {
		s["review/pr/1234"] = TriageEntry{Read: true, LastActivity: now, LastSeen: now}
	})

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	reloaded, err := LoadTriageState(path)
	if err != nil {
		t.Fatalf("LoadTriageState() error = %v", err)
	}
	entry, ok := reloaded["review/pr/1234"]
	if !ok {
		t.Fatalf("reloaded missing key, got %+v", reloaded)
	}
	if !entry.Read || !entry.LastActivity.Equal(now) {
		t.Errorf("reloaded entry = %+v, want Read=true LastActivity=%v", entry, now)
	}

	// No leftover temp file — same atomicity guarantee as state.Store.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if e.Name() != "notifications.yaml" {
			t.Errorf("leftover file in notifications dir: %s", e.Name())
		}
	}
}

func TestLoadTriageState_MissingFileReturnsEmptyNoError(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does-not-exist.yaml")

	got, err := LoadTriageState(missing)
	if err != nil {
		t.Fatalf("LoadTriageState(missing) error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("LoadTriageState(missing) = %+v, want empty", got)
	}
}

// TestLoadTriageState_EmptyFileReturnsEmptyNoError guards a zero-byte file
// (e.g. created by a crashed write, or `touch`) the same way a missing file
// is guarded — it should start fresh, not error.
func TestLoadTriageState_EmptyFileReturnsEmptyNoError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := LoadTriageState(path)
	if err != nil {
		t.Fatalf("LoadTriageState(empty) error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("LoadTriageState(empty) = %+v, want empty", got)
	}
}

// TestLoadTriageState_MalformedYAMLReturnsError guards against silently
// discarding a corrupt file's contents.
func TestLoadTriageState_MalformedYAMLReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := LoadTriageState(path); err == nil {
		t.Fatalf("LoadTriageState(malformed) error = nil, want error")
	}
}

func TestTriageStore_FlushWithoutChangesIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Flush with no changes created a file (err = %v)", err)
	}
}

// TestTriageStore_State_ReturnsCopyNotLiveMap pins the 🔴 defect: State()
// used to hand back the live map, so a caller mutating the result mutated
// the store with no lock held and without marking it dirty.
func TestTriageStore_State_ReturnsCopyNotLiveMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.Apply(func(s TriageState) {
		s["review/pr/1"] = TriageEntry{Read: true}
	})

	snapshot := store.State()
	snapshot["review/pr/1"] = TriageEntry{Read: false, Done: true}
	snapshot["review/pr/2"] = TriageEntry{Read: true}

	got := store.State()
	entry, ok := got["review/pr/1"]
	if !ok || !entry.Read || entry.Done {
		t.Errorf("store state mutated via State() result: got[review/pr/1] = %+v, ok = %v", entry, ok)
	}
	if _, ok := got["review/pr/2"]; ok {
		t.Errorf("store state gained a key injected into a State() result: %+v", got)
	}
}

// TestTriageStore_DebouncedWriteEventuallyHappens ports
// internal/state/store_test.go's test of the same name: a single Apply,
// left alone, eventually lands on disk without an explicit Flush.
func TestTriageStore_DebouncedWriteEventuallyHappens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(10 * time.Millisecond)

	store.Apply(func(s TriageState) {
		s["review/pr/1"] = TriageEntry{Read: true}
	})

	waitFor(t, 500*time.Millisecond, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, "debounced write to land on disk")

	reloaded, _ := LoadTriageState(path)
	if !reloaded["review/pr/1"].Read {
		t.Errorf("reloaded entry = %+v, want Read=true", reloaded["review/pr/1"])
	}
}

// TestTriageStore_RapidAppliesCoalesce confirms that many Apply calls in
// quick succession produce a single write — not one per call.
func TestTriageStore_RapidAppliesCoalesce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(50 * time.Millisecond)

	for i := 1; i <= 20; i++ {
		i := i
		store.Apply(func(s TriageState) {
			s["review/pr/1"] = TriageEntry{LastSeen: time.Unix(int64(i), 0)}
		})
	}

	// File should not exist yet — debounce hasn't expired.
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("file written before debounce expired")
	}

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	reloaded, _ := LoadTriageState(path)
	want := time.Unix(20, 0)
	if !reloaded["review/pr/1"].LastSeen.Equal(want) {
		t.Errorf("reloaded LastSeen = %v, want %v (latest Apply wins)", reloaded["review/pr/1"].LastSeen, want)
	}
}

// TestTriageStore_FlushAtomicallyReplacesExistingFile guards against a
// partial write leaving a half-written file behind on crash.
func TestTriageStore_FlushAtomicallyReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(1 * time.Millisecond)

	store.Apply(func(s TriageState) {
		s["review/pr/1"] = TriageEntry{Read: true}
	})
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	// No leftover temp file.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "notifications.yaml" {
			t.Errorf("leftover file in notifications dir: %s", e.Name())
		}
	}
}

// TestTriageStore_CreatesParentDirectory ensures we lazily create the
// notifications dir — the user shouldn't have to mkdir anything ahead of
// time.
func TestTriageStore_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "notifications.yaml")

	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(1 * time.Millisecond)

	store.Apply(func(s TriageState) { s["review/pr/1"] = TriageEntry{Read: true} })
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s, stat error: %v", path, err)
	}
}

// TestTriageStore_ConcurrentApplyIsSafe ports
// internal/state/store_test.go's concurrency test: many goroutines calling
// Apply concurrently must not corrupt the state.
func TestTriageStore_ConcurrentApplyIsSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(20 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.Apply(func(s TriageState) {
				s[fmt.Sprintf("review/pr/%d", i+1)] = TriageEntry{Read: true}
			})
		}(i)
	}
	wg.Wait()

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	reloaded, _ := LoadTriageState(path)
	if len(reloaded) != 100 {
		t.Errorf("reloaded has %d entries, want 100", len(reloaded))
	}
}

// TestTriageStore_ConcurrentApplyAndFlushIsSafe is the reproduction for the
// 🔴 defect: Flush used to snapshot the live map header (`snapshot :=
// s.state`) rather than deep-copying it, then release the mutex before
// calling snapshot.Marshal(), which ranges over the map. A concurrent Apply
// mutating the same map mid-range is a fatal "concurrent map iteration and
// map write" — a runtime throw that kills the process outright, not a
// race-detector-only finding, so this must fail even under `go test`
// without `-race` against the pre-fix code. Against the fix (deep copy
// under the lock in both State() and Flush()), this passes reliably under
// both `go test` and `go test -race`.
func TestTriageStore_ConcurrentApplyAndFlushIsSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(time.Millisecond)

	const applyGoroutines = 50
	const flushGoroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	for i := 0; i < applyGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				store.Apply(func(s TriageState) {
					s[fmt.Sprintf("review/pr/%d", i*iterations+j)] = TriageEntry{
						Read:     true,
						LastSeen: time.Now(),
					}
				})
			}
		}(i)
	}
	for i := 0; i < flushGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = store.Flush()
				_ = store.State()
			}
		}()
	}
	wg.Wait()

	if err := store.Flush(); err != nil {
		t.Fatalf("final Flush() error = %v", err)
	}

	reloaded, err := LoadTriageState(path)
	if err != nil {
		t.Fatalf("LoadTriageState() error = %v", err)
	}
	if len(reloaded) != applyGoroutines*iterations {
		t.Errorf("reloaded has %d entries, want %d", len(reloaded), applyGoroutines*iterations)
	}
}

// TestTriageStore_SetDebounce_RejectsNonPositive pins convention 11's `<=
// 0` guard shape with convention 13's boundary rows.
func TestTriageStore_SetDebounce_RejectsNonPositive(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want time.Duration
	}{
		{"negative", -1, defaultTriageDebounce},
		{"zero", 0, defaultTriageDebounce},
		{"one_nanosecond", time.Nanosecond, time.Nanosecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notifications.yaml")
			store, err := NewTriageStore(path)
			if err != nil {
				t.Fatalf("NewTriageStore() error = %v", err)
			}

			store.SetDebounce(tt.d)

			store.mu.Lock()
			got := store.debounce
			store.mu.Unlock()
			if got != tt.want {
				t.Errorf("debounce after SetDebounce(%v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

// TestTriageStore_LastWriteError_ClearsOnLaterSuccess pins the 🟡 defect:
// writeErr was never cleared on a later successful write, so one transient
// failure pinned a persistence error in the UI forever.
func TestTriageStore_LastWriteError_ClearsOnLaterSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}

	store.mu.Lock()
	store.writeErr = errors.New("boom (simulated prior failure)")
	store.mu.Unlock()

	store.Apply(func(s TriageState) { s["review/pr/1"] = TriageEntry{Read: true} })
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	if err := store.LastWriteError(); err != nil {
		t.Errorf("LastWriteError() = %v, want nil after a successful flush", err)
	}
}

// TestTriageStore_Flush_RearmsTimerOnWriteFailure pins the 🟡 defect: Flush
// stopped and nil'd the timer before writing, so a failed write left dirty
// true with no timer armed — nothing would ever retry.
func TestTriageStore_Flush_RearmsTimerOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(10 * time.Millisecond)

	// Point the store at a path nested under a regular file, so
	// os.MkdirAll (inside state.WriteAtomic) fails with ENOTDIR. The store
	// was already constructed successfully against a valid path above, so
	// this only breaks the write, not NewTriageStore's initial load.
	blocker := filepath.Join(dir, "blocker-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	store.mu.Lock()
	store.path = filepath.Join(blocker, "notifications.yaml")
	store.mu.Unlock()

	store.Apply(func(s TriageState) {
		s["review/pr/1"] = TriageEntry{Read: true}
	})

	if err := store.Flush(); err == nil {
		t.Fatalf("Flush() error = nil, want error writing under a non-directory path")
	}

	store.mu.Lock()
	dirty := store.dirty
	timerArmed := store.timer != nil
	writeErr := store.writeErr
	store.mu.Unlock()

	if !dirty {
		t.Errorf("dirty = false after failed write, want true so a retry can happen")
	}
	if !timerArmed {
		t.Errorf("no timer armed after failed write, want a re-armed retry timer")
	}
	if writeErr == nil {
		t.Errorf("writeErr = nil after failed write, want the write error recorded")
	}
}

// TestTriageStore_Replace_SwapsMapAndPersists exercises the Replace API
// added for task 3's pure Reconcile(rows, state) -> (rows, state) shape:
// a caller computing a fresh map (e.g. after TTL pruning deletes keys) can
// hand it to the store directly rather than clearing and re-copying a live
// map through Apply.
// TestTriageStore_Swap_HoldsLockAcrossComputeAndWrite pins task 8 review's
// 🟡 finding 5: Swap must hold mu across the whole
// clone-then-compute-then-store sequence, not release it between steps the
// way a naive State() -> compute -> Replace() call chain would (three
// separate lock acquisitions, with an unlocked gap between each). A
// concurrent Apply landing in that gap would be silently discarded the
// moment Replace's full-map overwrite runs, computed from an
// already-stale snapshot that never saw the Apply's write.
//
// This is deliberately not a -race-detector-only reproduction (this repo's
// sandbox cannot build with -race — see this package's other concurrency
// tests): the assertion is on the *result*, not a race report. fn blocks on
// proceed until the test has had a bounded window to attempt the concurrent
// Apply; if that Apply completes inside that window, Swap did not hold the
// lock across fn's execution, and the test fails outright rather than
// relying on timing to merely make corruption more likely.
func TestTriageStore_Swap_HoldsLockAcrossComputeAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(time.Hour)

	fnStarted := make(chan struct{})
	proceed := make(chan struct{})
	swapDone := make(chan struct{})

	go func() {
		store.Swap(func(current TriageState) TriageState {
			close(fnStarted)
			<-proceed
			next := current.clone()
			next["swap/written"] = TriageEntry{Read: true}
			return next
		})
		close(swapDone)
	}()

	<-fnStarted

	applyDone := make(chan struct{})
	go func() {
		store.Apply(func(s TriageState) {
			s["apply/written"] = TriageEntry{Read: true}
		})
		close(applyDone)
	}()

	select {
	case <-applyDone:
		t.Fatal("concurrent Apply completed while Swap's fn was still in flight — Swap is not holding the lock across the whole compute-and-write sequence")
	case <-time.After(100 * time.Millisecond):
		// Expected: Apply is blocked behind Swap's held lock.
	}

	close(proceed)
	<-swapDone
	<-applyDone

	got := store.State()
	if _, ok := got["swap/written"]; !ok {
		t.Errorf("state = %+v, want the entry Swap's fn computed and stored", got)
	}
	if _, ok := got["apply/written"]; !ok {
		t.Errorf("state = %+v, want the entry the concurrent Apply wrote (it should land after Swap releases the lock, not be lost underneath Swap's write)", got)
	}
}

func TestTriageStore_Replace_SwapsMapAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}
	store.SetDebounce(10 * time.Millisecond)

	store.Apply(func(s TriageState) {
		s["review/pr/1"] = TriageEntry{Read: true}
		s["review/pr/2"] = TriageEntry{Read: true}
	})

	// Simulate a Reconcile pass that pruned "review/pr/2".
	store.Replace(TriageState{
		"review/pr/1": {Read: true, Done: true},
	})

	got := store.State()
	if len(got) != 1 {
		t.Fatalf("State() after Replace = %+v, want exactly one entry", got)
	}
	if _, ok := got["review/pr/2"]; ok {
		t.Errorf("State() still has pruned key review/pr/2: %+v", got)
	}
	if !got["review/pr/1"].Done {
		t.Errorf("State()[review/pr/1] = %+v, want Done=true", got["review/pr/1"])
	}

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	reloaded, err := LoadTriageState(path)
	if err != nil {
		t.Fatalf("LoadTriageState() error = %v", err)
	}
	if len(reloaded) != 1 {
		t.Errorf("reloaded = %+v, want exactly one entry", reloaded)
	}
}

// TestTriageStore_Replace_DoesNotAliasCallerMap mirrors
// TestTriageStore_State_ReturnsCopyNotLiveMap for the write side: Replace
// used to store the caller's map by reference, so a caller mutating the map
// it just handed to Replace corrupted the store's live state without going
// through any lock — the exact hole that previously caused a "concurrent
// map iteration and map write" fatal error when Reconcile's returned map
// was mutated by its caller after being passed to Replace.
func TestTriageStore_Replace_DoesNotAliasCallerMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.yaml")
	store, err := NewTriageStore(path)
	if err != nil {
		t.Fatalf("NewTriageStore() error = %v", err)
	}

	callerMap := TriageState{
		"review/pr/1": {Read: true},
	}
	store.Replace(callerMap)

	// Mutate the caller's map after handing it to Replace.
	callerMap["review/pr/1"] = TriageEntry{Read: false, Done: true}
	callerMap["review/pr/2"] = TriageEntry{Read: true}

	got := store.State()
	entry, ok := got["review/pr/1"]
	if !ok || !entry.Read || entry.Done {
		t.Errorf("store state mutated via caller's map after Replace(): got[review/pr/1] = %+v, ok = %v", entry, ok)
	}
	if _, ok := got["review/pr/2"]; ok {
		t.Errorf("store state gained a key injected into the caller's map after Replace(): %+v", got)
	}
}
