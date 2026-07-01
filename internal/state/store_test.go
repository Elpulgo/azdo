package state

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// waitFor polls a condition with a short interval, failing the test if it
// doesn't become true within the timeout. Used for debounced writes.
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

func TestNewStore_MissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if store.State() != (State{}) {
		t.Errorf("State() = %+v, want zero", store.State())
	}
}

func TestNewStore_LoadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	contents := `version: 2
active_tab: work_items
tabs:
  work_items:
    last_detail:
      kind: azure
      scope: proj
      id: "99"
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if store.State().ActiveTab != TabWorkItems {
		t.Errorf("ActiveTab = %v, want %v", store.State().ActiveTab, TabWorkItems)
	}
	if store.State().Tabs.WorkItems.LastDetail.ID != "99" {
		t.Errorf("LastDetail.ID = %q, want \"99\"", store.State().Tabs.WorkItems.LastDetail.ID)
	}
}

func TestStore_ApplyAndFlushPersistsToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(10 * time.Millisecond)

	store.Apply(func(s *State) {
		s.Version = CurrentVersion
		s.ActiveTab = TabPullRequests
		s.Tabs.PullRequests.LastDetail = DetailRef{Kind: "azure", Scope: "proj", ID: "7"}
	})

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if reloaded.ActiveTab != TabPullRequests {
		t.Errorf("reloaded ActiveTab = %v, want %v", reloaded.ActiveTab, TabPullRequests)
	}
	if reloaded.Tabs.PullRequests.LastDetail.ID != "7" {
		t.Errorf("reloaded LastDetail.ID = %q, want \"7\"", reloaded.Tabs.PullRequests.LastDetail.ID)
	}
}

func TestStore_DebouncedWriteEventuallyHappens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(10 * time.Millisecond)

	store.Apply(func(s *State) {
		s.ActiveTab = TabPipelines
	})

	waitFor(t, 500*time.Millisecond, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, "debounced write to land on disk")

	reloaded, _ := Load(path)
	if reloaded.ActiveTab != TabPipelines {
		t.Errorf("reloaded ActiveTab = %v, want %v", reloaded.ActiveTab, TabPipelines)
	}
}

// TestStore_RapidAppliesCoalesce confirms that many Apply calls in quick
// succession produce a single write (the debounce purpose) — not one per call.
func TestStore_RapidAppliesCoalesce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(50 * time.Millisecond)

	for i := 1; i <= 20; i++ {
		i := i
		store.Apply(func(s *State) {
			s.Tabs.PullRequests.LastDetail = DetailRef{Kind: "azure", Scope: "proj", ID: strconv.Itoa(i)}
		})
	}

	// File should not exist yet — debounce hasn't expired.
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("file written before debounce expired")
	}

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	reloaded, _ := Load(path)
	if reloaded.Tabs.PullRequests.LastDetail.ID != "20" {
		t.Errorf("reloaded LastDetail.ID = %q, want \"20\" (latest Apply wins)",
			reloaded.Tabs.PullRequests.LastDetail.ID)
	}
}

func TestStore_FlushWithoutChangesIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Flush with no changes created a file (err = %v)", err)
	}
}

// TestStore_ConcurrentApplyIsSafe runs many goroutines that call Apply
// concurrently. The race detector + a final reload check guard the invariant.
func TestStore_ConcurrentApplyIsSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(20 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.Apply(func(s *State) {
				s.Tabs.PullRequests.LastDetail = DetailRef{Kind: "azure", Scope: "proj", ID: strconv.Itoa(i + 1)}
			})
		}(i)
	}
	wg.Wait()

	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	reloaded, _ := Load(path)
	if reloaded.Tabs.PullRequests.LastDetail.IsZero() {
		t.Errorf("expected some LastDetail written, got zero ref")
	}
}

// TestStore_FlushAtomicallyReplacesExistingFile guards against a partial
// write leaving a half-written file behind on crash.
func TestStore_FlushAtomicallyReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte("version: 0\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(1 * time.Millisecond)

	store.Apply(func(s *State) {
		s.Version = CurrentVersion
		s.ActiveTab = TabPullRequests
	})
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	// No leftover temp file.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "state.yaml" {
			t.Errorf("leftover file in state dir: %s", e.Name())
		}
	}
}

// TestStore_StateRoundTripsAcrossSessions simulates a complete launch
// cycle: a "first run" persists state and flushes; a fresh Store on the
// same path reads it back identically.
func TestStore_StateRoundTripsAcrossSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")

	first, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore (first session): %v", err)
	}
	first.SetDebounce(1 * time.Millisecond)
	first.Apply(func(s *State) {
		s.Version = CurrentVersion
		s.ActiveTab = TabWorkItems
		s.Tabs.PullRequests.LastDetail = DetailRef{Kind: "azure", Scope: "proj", ID: "7"}
		s.Tabs.WorkItems.LastDetail = DetailRef{Kind: "github", Scope: "owner/repo", ID: "42"}
	})
	if err := first.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	second, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore (second session): %v", err)
	}
	got := second.State()
	want := State{
		Version:   CurrentVersion,
		ActiveTab: TabWorkItems,
		Tabs: TabsState{
			PullRequests: TabMemory{LastDetail: DetailRef{Kind: "azure", Scope: "proj", ID: "7"}},
			WorkItems:    TabMemory{LastDetail: DetailRef{Kind: "github", Scope: "owner/repo", ID: "42"}},
		},
	}
	if got != want {
		t.Errorf("second session state = %+v, want %+v", got, want)
	}
}

// TestStore_CreatesParentDirectory ensures we lazily create the state dir
// — the user shouldn't have to mkdir anything ahead of time.
func TestStore_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "state.yaml")

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(1 * time.Millisecond)

	store.Apply(func(s *State) { s.ActiveTab = TabWorkItems })
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s, stat error: %v", path, err)
	}
}
