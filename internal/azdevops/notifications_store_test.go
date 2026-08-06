package azdevops

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/state"
)

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
