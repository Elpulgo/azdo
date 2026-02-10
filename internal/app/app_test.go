package app

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel_WithConfig(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)

	if m.config != cfg {
		t.Error("expected config to be set")
	}
	if m.statusBar == nil {
		t.Error("expected status bar to be initialized")
	}
}

func TestModel_StatusBarShowsOrgProject(t *testing.T) {
	cfg := &config.Config{
		Organization: "myorg",
		Project:      "myproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Update with window size to initialize status bar width
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "myorg") {
		t.Error("view should contain organization name")
	}
	if !strings.Contains(view, "myproject") {
		t.Error("view should contain project name")
	}
}

func TestModel_HandlesPollingTick(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)

	// Send a tick message
	_, cmd := m.Update(polling.TickMsg{})

	// Should return a command (to fetch data)
	if cmd == nil {
		t.Error("expected a command after tick message")
	}
}

func TestModel_HandlesPipelineRunsUpdated_Success(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Simulate successful data fetch
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1", Definition: azdevops.PipelineDefinition{Name: "Build"}},
	}
	msg := polling.PipelineRunsUpdated{Runs: runs, Err: nil}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	// Status bar should show connected
	view := m.View()
	if !strings.Contains(strings.ToLower(view), "connected") {
		t.Error("should show connected state after successful update")
	}
}

func TestModel_HandlesPipelineRunsUpdated_Error(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Simulate error
	msg := polling.PipelineRunsUpdated{Runs: nil, Err: &testError{}}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	// Status bar should show error
	view := m.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Error("should show error state after failed update")
	}
}

type testError struct{}

func (e *testError) Error() string { return "test error" }

func TestModel_Init_StartsPolling(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Project:         "testproject",
		PollingInterval: 30,
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	cmd := m.Init()

	// Should return commands for initialization
	if cmd == nil {
		t.Error("Init should return commands")
	}
}
