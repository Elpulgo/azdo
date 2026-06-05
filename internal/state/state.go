// Package state persists lightweight TUI navigation state between runs:
// the last active tab and (for restorable tabs) the most recently opened
// detail item. The file lives in $XDG_STATE_HOME/azdo-tui/state.yaml,
// falling back to ~/.local/state/azdo-tui/state.yaml.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	dirName  = "azdo-tui"
	fileName = "state.yaml"

	// CurrentVersion is the on-disk schema version. Bump when introducing
	// a breaking change to the YAML shape.
	CurrentVersion = 1
)

// TabID identifies a top-level tab in the persisted state. The string
// values are stable on-disk identifiers.
type TabID string

const (
	TabPullRequests TabID = "pull_requests"
	TabWorkItems    TabID = "work_items"
	TabPipelines    TabID = "pipelines"
)

// State is the persistent application state written to disk between runs.
type State struct {
	Version   int       `yaml:"version,omitempty"`
	ActiveTab TabID     `yaml:"active_tab,omitempty"`
	Tabs      TabsState `yaml:"tabs,omitempty"`
}

// TabsState holds per-tab restorable memory. Pipelines is deliberately
// absent — only the tab selection itself is restored, never a detail view.
type TabsState struct {
	PullRequests TabMemory `yaml:"pull_requests,omitempty"`
	WorkItems    TabMemory `yaml:"work_items,omitempty"`
}

// TabMemory captures the per-tab navigation state to restore on next launch.
// LastDetailID == 0 means "no detail open".
type TabMemory struct {
	LastDetailID int `yaml:"last_detail_id,omitempty"`
}

// Marshal encodes the state as YAML.
func (s State) Marshal() ([]byte, error) {
	return yaml.Marshal(s)
}

// Unmarshal parses YAML into the receiver.
func (s *State) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, s)
}

// Path returns the on-disk location of the state file, honoring
// $XDG_STATE_HOME when set and falling back to ~/.local/state/azdo-tui/.
func Path() (string, error) {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, dirName, fileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", dirName, fileName), nil
}

// Load reads and parses the state file. A missing file is not an error —
// callers receive a zero-value State and can start fresh.
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := s.Unmarshal(data); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	return s, nil
}
