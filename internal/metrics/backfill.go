package metrics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultBackfillMarkerPath returns the standard marker location
// (~/.config/azdo-tui/.metrics-backfill-done). The marker's presence means
// the one-shot backfill has already been run; absence (with the config flag
// on) means it should run on the next launch.
func DefaultBackfillMarkerPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".metrics-backfill-done"), nil
}

// BackfillAlreadyDone reports whether the marker file exists. A missing file
// is not an error (fresh install).
func BackfillAlreadyDone(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat backfill marker: %w", err)
}

// MarkBackfillDone creates the marker file (idempotent). Creates any missing
// parent directories. Writing-then-erroring is fine: BackfillAlreadyDone only
// cares that the file exists.
func MarkBackfillDone(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create marker dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create backfill marker: %w", err)
	}
	return f.Close()
}
