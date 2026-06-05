package metrics

import (
	"fmt"
	"os"
	"path/filepath"
)

// configDir returns the base directory for metrics persistence files
// (snapshots, sprint selection, backfill marker).
//
// AZDO_CONFIG_DIR, when set, wins outright — demo mode points it at a temp
// directory so showcasing the metrics tab never reads or writes the user's
// real history. Otherwise the standard ~/.config/azdo-tui location is used,
// preserving existing behavior for everyone else.
func configDir() (string, error) {
	if dir := os.Getenv("AZDO_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "azdo-tui"), nil
}
