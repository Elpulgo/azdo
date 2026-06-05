package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

// When AZDO_CONFIG_DIR is set, all three default-path helpers must resolve
// under it. This is what lets demo mode redirect snapshot/selection/marker
// I/O at a temp dir instead of the user's real ~/.config/azdo-tui.
func TestDefaultPaths_HonorConfigDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AZDO_CONFIG_DIR", dir)

	cases := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{"snapshot", DefaultSnapshotPath, filepath.Join(dir, "metrics.jsonl")},
		{"selection", DefaultSelectionPath, filepath.Join(dir, "metrics-selection.json")},
		{"backfill", DefaultBackfillMarkerPath, filepath.Join(dir, ".metrics-backfill-done")},
	}
	for _, tc := range cases {
		got, err := tc.fn()
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Without the override the helpers fall back to ~/.config/azdo-tui so existing
// users see no behavior change.
func TestDefaultPaths_FallBackToHomeConfig(t *testing.T) {
	t.Setenv("AZDO_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	base := filepath.Join(home, ".config", "azdo-tui")

	got, err := DefaultSnapshotPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(base, "metrics.jsonl"); got != want {
		t.Errorf("DefaultSnapshotPath = %q, want %q", got, want)
	}
}
