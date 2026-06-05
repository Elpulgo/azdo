package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestState_YAMLRoundTrip ensures the State type serialises and parses back
// to the same value via YAML — the on-disk format must survive a round trip.
func TestState_YAMLRoundTrip(t *testing.T) {
	original := State{
		Version:   1,
		ActiveTab: TabPullRequests,
		Tabs: TabsState{
			PullRequests: TabMemory{LastDetailID: 7},
			WorkItems:    TabMemory{LastDetailID: 42},
		},
	}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var parsed State
	if err := parsed.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if parsed != original {
		t.Errorf("round trip mismatch:\n  got  = %+v\n  want = %+v", parsed, original)
	}
}

// TestState_EmptyMarshalOmitsZeroValues ensures the zero state produces a
// minimal YAML output — no spurious detail IDs, no empty tab sections.
func TestState_EmptyMarshalOmitsZeroValues(t *testing.T) {
	s := State{Version: 1}
	data, err := s.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := string(data)
	if strings.Contains(got, "last_detail_id") {
		t.Errorf("empty state should not emit last_detail_id, got:\n%s", got)
	}
}

func TestPath_UsesXDGStateHomeWhenSet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join(tmp, "azdo-tui", "state.yaml")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPath_FallsBackToHomeWhenXDGUnset(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join(home, ".local", "state", "azdo-tui", "state.yaml")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoad_MissingFileReturnsEmptyStateNoError(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does-not-exist.yaml")

	got, err := Load(missing)
	if err != nil {
		t.Fatalf("Load(missing) error = %v, want nil", err)
	}
	if got != (State{}) {
		t.Errorf("Load(missing) = %+v, want zero State", got)
	}
}

func TestLoad_ParsesExistingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.yaml")
	contents := `version: 1
active_tab: work_items
tabs:
  pull_requests:
    last_detail_id: 7
  work_items:
    last_detail_id: 42
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := State{
		Version:   1,
		ActiveTab: TabWorkItems,
		Tabs: TabsState{
			PullRequests: TabMemory{LastDetailID: 7},
			WorkItems:    TabMemory{LastDetailID: 42},
		},
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoad_MalformedFileReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "broken.yaml")
	if err := os.WriteFile(path, []byte("not: : valid"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load(malformed) error = nil, want non-nil")
	}
}

// TestLoad_UnknownFieldsTolerated guards forward-compatibility: a state file
// written by a newer version with extra keys must still load on older builds.
func TestLoad_UnknownFieldsTolerated(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.yaml")
	contents := `version: 1
active_tab: pull_requests
future_field: surprise
tabs:
  pull_requests:
    last_detail_id: 3
    future_nested: x
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ActiveTab != TabPullRequests {
		t.Errorf("ActiveTab = %v, want TabPullRequests", got.ActiveTab)
	}
	if got.Tabs.PullRequests.LastDetailID != 3 {
		t.Errorf("PullRequests.LastDetailID = %d, want 3", got.Tabs.PullRequests.LastDetailID)
	}
}
