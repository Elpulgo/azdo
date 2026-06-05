package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Selection is the persistence shape for the Trends view's chosen sprints.
type Selection struct {
	Sprints []string `json:"sprints"`
}

// DefaultSelectionPath is the standard location for the trend-view sprint
// selection (sibling to the snapshot file).
func DefaultSelectionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "azdo-tui", "metrics-selection.json"), nil
}

// LoadSelection reads the saved sprint tags. Missing file returns an empty
// slice and no error — a fresh install simply hasn't picked sprints yet.
func LoadSelection(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read selection: %w", err)
	}
	var s Selection
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse selection: %w", err)
	}
	return s.Sprints, nil
}

// SaveSelection writes the chosen sprint tags atomically. Empty selection is
// allowed — the file is rewritten, not deleted.
func SaveSelection(path string, sprints []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir selection: %w", err)
	}
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(Selection{Sprints: sprints}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode selection: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write selection tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename selection tmp: %w", err)
	}
	return nil
}

// FilterAvailable returns only those `saved` entries that still appear in
// `available`. Preserves the order of `saved`. Used when loading: a sprint
// tag that no longer exists in the snapshot should not be silently kept.
func FilterAvailable(saved, available []string) []string {
	if len(saved) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(available))
	for _, a := range available {
		set[a] = struct{}{}
	}
	out := make([]string, 0, len(saved))
	for _, s := range saved {
		if _, ok := set[s]; ok {
			out = append(out, s)
		}
	}
	return out
}
