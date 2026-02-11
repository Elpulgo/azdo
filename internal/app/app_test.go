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

func TestModel_DefaultTab_IsPipelines(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)

	if m.activeTab != TabPipelines {
		t.Errorf("Default tab should be TabPipelines (0), got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_To_PullRequests(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Press '2' to switch to pull requests tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("After pressing '2', activeTab should be TabPullRequests (1), got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Back_To_Pipelines(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Switch to pull requests tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	// Press '1' to switch back to pipelines tab
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("After pressing '1', activeTab should be TabPipelines (0), got %d", m.activeTab)
	}
}

func TestModel_View_ShowsPullRequests_WhenActiveTab(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Switch to pull requests tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	view := m.View()

	// Should show pull requests content (empty list message or similar)
	if !strings.Contains(view, "pull request") && !strings.Contains(view, "No pull requests") {
		t.Error("View should show pull requests content when on PR tab")
	}
}

func TestModel_StatusBarShowsConfigPath(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 200
	m.height = 30

	// Update with window size to initialize status bar width
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m = updated.(Model)

	view := m.View()

	// Should contain config.yaml somewhere in the view
	if !strings.Contains(view, "config.yaml") {
		t.Error("view should contain config file path")
	}
}
