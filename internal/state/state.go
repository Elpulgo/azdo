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
	//
	// v2 replaced the bare integer LastDetailID with a DetailRef keyed on
	// (Kind, Scope, ID). A bare ID collides across backends once a merged
	// Azure + GitHub list is in play (GitHub issue #42 and Azure work item
	// 42 coexist). Old v1 files simply fail to restore a detail and land the
	// user on the list — the restore path already no-ops on a miss.
	CurrentVersion = 2
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
// A zero-value LastDetail (empty ID) means "no detail open".
type TabMemory struct {
	LastDetail DetailRef `yaml:"last_detail,omitempty"`
}

// DetailRef identifies a detail item across backends. A bare numeric ID is
// not unique once Azure and GitHub items are merged into one list, so the
// persisted key carries the origin Kind and Scope alongside the ID. Kind is
// the stable string form of provider.Kind ("azure"/"github"); Scope is the
// project/repo API name; ID is the backend's string ID.
type DetailRef struct {
	Kind  string `yaml:"kind,omitempty"`
	Scope string `yaml:"scope,omitempty"`
	ID    string `yaml:"id,omitempty"`
}

// IsZero reports whether the ref points at no detail (nothing to restore).
func (r DetailRef) IsZero() bool {
	return r.ID == ""
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
