package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/workitems"
	"github.com/Elpulgo/azdo/internal/version"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatVersionInfo(t *testing.T) {
	tests := []struct {
		version string
		commit  string
		want    string
	}{
		{"1.2.3", "abc1234", "1.2.3 (abc1234)"},
		{"dev", "none", "dev"},
		{"0.1.0", "", "0.1.0"},
		{"2.0.0", "deadbeef", "2.0.0 (deadbeef)"},
	}
	for _, tt := range tests {
		got := formatVersionInfo(tt.version, tt.commit)
		if got != tt.want {
			t.Errorf("formatVersionInfo(%q, %q) = %q, want %q", tt.version, tt.commit, got, tt.want)
		}
	}
}

func TestModel_StatusBarShowsOrgProject(t *testing.T) {
	cfg := &config.Config{
		Organization:    "myorg",
		Projects:        []string{"myproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
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
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	// Send a tick message
	_, cmd := m.Update(polling.TickMsg{})

	// Should return a command (to fetch data)
	if cmd == nil {
		t.Error("expected a command after tick message")
	}
}

func TestModel_HandlesPipelineRunsUpdated_Success(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Simulate successful data fetch
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1", Definition: azdevops.PipelineDefinition{Name: "Build"}},
	}
	msg := polling.PipelineRunsUpdated{Runs: runs, Err: nil}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	// Status bar should show connected icon (● only, no text)
	view := m.View()
	if !strings.Contains(view, "●") {
		t.Error("should show connected icon ● after successful update")
	}
}

func TestModel_HandlesPipelineRunsUpdated_Error(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
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
		Projects:        []string{"testproject"},
		PollingInterval: 30,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	cmd := m.Init()

	// Should return commands for initialization
	if cmd == nil {
		t.Error("Init should return commands")
	}
}

func TestModel_DefaultTab_IsPullRequests(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	if m.activeTab != TabPullRequests {
		t.Errorf("Default tab should be TabPullRequests, got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Key1_IsPullRequests(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Switch away first, then press '1' to go to PR tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("After pressing '1', activeTab should be TabPullRequests, got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Key2_IsWorkItems(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Press '2' to switch to work items tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabWorkItems {
		t.Errorf("After pressing '2', activeTab should be TabWorkItems, got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Key3_IsPipelines(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Press '3' to switch to pipelines tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("After pressing '3', activeTab should be TabPipelines, got %d", m.activeTab)
	}
}

func TestModel_View_ShowsPullRequests_WhenActiveTab(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// PR is now the default tab (key '1'), so it should already be active
	view := m.View()

	// Should show pull requests content (empty list message or similar)
	if !strings.Contains(view, "pull request") && !strings.Contains(view, "No pull requests") {
		t.Error("View should show pull requests content when on PR tab")
	}
}

func TestModel_HelpModalShowsConfigPath(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 200
	m.height = 60

	// Update with window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open help modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	// Config path should appear in the help modal
	if !strings.Contains(view, "config.yaml") {
		t.Error("help modal should contain config file path")
	}
}

func TestModel_View_ShowsWorkItems_WhenActiveTab(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Switch to work items tab (key '2')
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	view := m.View()

	// Should show work items content (empty list message or similar)
	if !strings.Contains(view, "work item") && !strings.Contains(view, "No work items") {
		t.Error("View should show work items content when on Work Items tab")
	}
}

func TestModel_View_HasBorderedTabBar(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	// Tab bar should be wrapped in a rounded border (╭ top-left corner)
	if !strings.Contains(view, "╭") {
		t.Error("Tab bar should have rounded border (expected ╭ corner)")
	}
}

func TestModel_View_HasBorderedContent(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	// Content should be wrapped in a border — we expect at least 2 rounded borders
	// (one for tabs, one for content area)
	cornerCount := strings.Count(view, "╭")
	if cornerCount < 2 {
		t.Errorf("Expected at least 2 bordered sections (tabs + content), got %d ╭ corners", cornerCount)
	}
}

func TestModel_View_TabBarAppearsBeforeContent(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	// The first line should contain the tab bar border corner (logo is inside the tab bar)
	if len(lines) == 0 || !strings.Contains(lines[0], "╭") {
		t.Errorf("First line should contain tab bar border corner ╭, got: %q", lines[0])
	}

	// Total lines should not exceed terminal height
	if len(lines) > 30 {
		t.Errorf("View output has %d lines, should not exceed terminal height 30", len(lines))
	}
}

func TestModel_View_PipelinesWithData_FitsInTerminal(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	// Simulate window size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Switch to pipelines tab (key '3')
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	// Simulate pipeline data arriving (like from polling)
	runs := make([]azdevops.PipelineRun, 30)
	for i := range runs {
		runs[i] = azdevops.PipelineRun{
			ID:          i + 1,
			BuildNumber: fmt.Sprintf("2024.%d", i+1),
			Definition:  azdevops.PipelineDefinition{Name: fmt.Sprintf("Pipeline-%d", i+1)},
			Status:      "completed",
			Result:      "succeeded",
		}
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	t.Logf("Total lines: %d (terminal height: 40)", len(lines))
	// Count lines with actual visible content (not just whitespace/ANSI)
	nonEmptyCount := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmptyCount++
		}
		if i < 8 || i > len(lines)-6 {
			t.Logf("Line %d (bytes=%d): %.120q", i, len(line), line)
		}
	}
	t.Logf("Non-empty lines: %d", nonEmptyCount)

	// Count actual data rows (lines with "Pipeline-")
	dataRows := 0
	for _, line := range lines {
		if strings.Contains(line, "Pipeline-") {
			dataRows++
		}
	}
	t.Logf("Data rows visible: %d (sent %d runs)", dataRows, len(runs))

	if len(lines) > 40 {
		t.Errorf("View has %d lines, exceeds terminal height 40", len(lines))
	}

	// Tab bar border should be on line 0 (logo is inside the tab bar)
	if !strings.Contains(lines[0], "╭") {
		t.Errorf("Line 0 should have tab bar top border, got: %.80q", lines[0])
	}
}

func TestModel_View_ContentFillsBoxWithoutExcessPadding(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	terminalHeight := 40
	m := NewModel(nil, client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: terminalHeight})
	m = updated.(Model)

	// Switch to pipelines tab (key '3') since pipeline data is used in this test
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	// Load enough data to fill the table
	runs := make([]azdevops.PipelineRun, 50)
	for i := range runs {
		runs[i] = azdevops.PipelineRun{
			ID:          i + 1,
			BuildNumber: fmt.Sprintf("2024.%d", i+1),
			Definition:  azdevops.PipelineDefinition{Name: fmt.Sprintf("Pipeline-%d", i+1)},
			Status:      "completed",
			Result:      "succeeded",
		}
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	// Find the content box bottom border (╰)
	boxBottomLine := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "╰") {
			// The last ╰ is the status bar bottom; the second-to-last is the content box bottom
			// But we want the content box bottom which comes before the status bar
			// Content box bottom is followed by the status bar (╭)
			if i+1 < len(lines) && strings.Contains(lines[i+1], "╭") {
				boxBottomLine = i
				break
			}
		}
	}
	if boxBottomLine == -1 {
		t.Fatal("Could not find content box bottom border")
	}

	// Count empty lines inside the box (lines that are just border chars with whitespace)
	// Empty padding lines look like: "│                    │"
	emptyPaddingLines := 0
	for i := boxBottomLine - 1; i >= 0; i-- {
		line := lines[i]
		// Strip the border characters and check if content is just whitespace
		if strings.Contains(line, "│") {
			// Extract content between borders
			inner := strings.TrimPrefix(line, "│")
			inner = strings.TrimSuffix(inner, "│")
			inner = strings.TrimSpace(inner)
			if inner == "" {
				emptyPaddingLines++
			} else {
				break
			}
		}
	}

	// Allow at most 1 line of padding (for rounding). More than that indicates
	// the content view height doesn't match the box inner height.
	const maxAllowedPadding = 1
	if emptyPaddingLines > maxAllowedPadding {
		t.Errorf("Content box has %d empty padding lines at the bottom (max allowed: %d). "+
			"This indicates maxFooterRows is too conservative, causing content views to be undersized.",
			emptyPaddingLines, maxAllowedPadding)
	}

	t.Logf("Total lines: %d, box bottom at line: %d, empty padding: %d", len(lines), boxBottomLine, emptyPaddingLines)
}

func TestModel_View_OutputHeightMatchesTerminal(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	terminalHeights := []int{24, 30, 40, 50}
	for _, termHeight := range terminalHeights {
		t.Run(fmt.Sprintf("height_%d", termHeight), func(t *testing.T) {
			m := NewModel(nil, client, cfg, "dev", "")

			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: termHeight})
			m = updated.(Model)

			// Switch to pipelines tab (key '3')
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
			m = updated.(Model)

			// Load data so content fills
			runs := make([]azdevops.PipelineRun, 50)
			for i := range runs {
				runs[i] = azdevops.PipelineRun{
					ID:          i + 1,
					BuildNumber: fmt.Sprintf("2024.%d", i+1),
					Definition:  azdevops.PipelineDefinition{Name: fmt.Sprintf("Pipeline-%d", i+1)},
					Status:      "completed",
					Result:      "succeeded",
				}
			}
			updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
			m = updated.(Model)

			view := m.View()
			lines := strings.Split(view, "\n")

			t.Logf("Terminal height: %d, output lines: %d, footerRows: %d", termHeight, len(lines), m.footerRows)

			if len(lines) != termHeight {
				// Show first and last few lines for debugging
				for i, line := range lines {
					if i < 5 || i > len(lines)-5 {
						t.Logf("Line %d: %.100q", i, line)
					}
				}
				t.Errorf("Output has %d lines, want exactly %d", len(lines), termHeight)
			}
		})
	}
}

func TestModel_GlobalShortcutsDisabledDuringSearch(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Switch to pipelines tab (key '3') so we can test search there
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	// Load some pipeline data
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1", Definition: azdevops.PipelineDefinition{Name: "Build"}},
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	// Press 'f' to enter search mode
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)

	// Verify we're searching
	if !m.isActiveViewSearching() {
		t.Fatal("Expected active view to be searching after pressing 'f'")
	}

	// Press 't' — should NOT open theme picker (should go to search input)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)

	if m.themePicker.IsVisible() {
		t.Error("Pressing 't' during search should NOT open theme picker")
	}

	// Press '2' — should NOT switch tabs
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Error("Pressing '2' during search should NOT switch to Work Items tab")
	}

	// Press esc to exit search
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.isActiveViewSearching() {
		t.Error("Expected search to be exited after esc")
	}

	// Now '2' should work again — switches to Work Items
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabWorkItems {
		t.Error("After exiting search, '2' should switch to Work Items tab")
	}
}

func TestModel_MyItemsToggle_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	// Set up window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Switch to work items tab (key '2')
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	// Simulate work items arriving
	items := []provider.WorkItem{
		{Identity: provider.Identity{ID: "1"}, Title: "My task", WorkItemType: "Task", State: "Active"},
		{Identity: provider.Identity{ID: "2"}, Title: "Other task", WorkItemType: "Task", State: "Active"},
	}
	updated, _ = m.Update(workitems.SetWorkItemsMsg{WorkItems: items})
	m = updated.(Model)

	// Press 'm' to toggle my items filter — fires @Me fetch
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	// Verify filter is active
	if !m.workItemsView.IsMyItemsActive() {
		t.Error("expected my items filter to be active after pressing 'm'")
	}

	// Verify status bar shows "My Items" badge
	view := m.View()
	if !strings.Contains(view, "My Items") {
		t.Error("status bar should show 'My Items' badge when filter is active")
	}

	// Press 'm' again to toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	if m.workItemsView.IsMyItemsActive() {
		t.Error("expected my items filter to be deactivated after second 'm' press")
	}

	// Verify "My Items" badge is removed
	view = m.View()
	if strings.Contains(view, "My Items") {
		t.Error("status bar should NOT show 'My Items' badge when filter is inactive")
	}
}

func TestModel_PRTab_StatusBarShowsMyItemsKeybinding(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// On PR tab (default), status bar should advertise 'm' for my PRs and 'f' for search
	view := m.View()
	if !strings.Contains(view, "my PRs") {
		t.Error("PR tab status bar should show 'm my PRs' keybinding")
	}
	if !strings.Contains(view, "search") {
		t.Error("PR tab status bar should show 'f search' keybinding")
	}
}

func TestModel_MyPRsToggle_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// We're on PR tab by default — press 'm' to toggle
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	if !m.pullRequestsView.IsMyPRsActive() {
		t.Error("expected my PRs filter to be active after pressing 'm'")
	}

	view := m.View()
	if !strings.Contains(view, "My PRs") {
		t.Error("status bar should show 'My PRs' badge when filter is active")
	}

	// Toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	if m.pullRequestsView.IsMyPRsActive() {
		t.Error("expected my PRs filter to be off after second 'm' press")
	}

	view = m.View()
	if strings.Contains(view, "My PRs") {
		t.Error("status bar should NOT show 'My PRs' badge when filter is inactive")
	}
}

func TestModel_AsReviewerToggle_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// On PR tab, press 'A' to toggle as-reviewer
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = updated.(Model)

	if !m.pullRequestsView.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be active after pressing 'A'")
	}

	view := m.View()
	if !strings.Contains(view, "Reviewer") {
		t.Error("status bar should show 'As Reviewer' badge")
	}

	// Toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = updated.(Model)

	if m.pullRequestsView.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be off after second 'A'")
	}
	view = m.View()
	if strings.Contains(view, "Reviewer") {
		t.Error("status bar should NOT show 'As Reviewer' badge when off")
	}
}

func TestModel_PRTab_StatusBarShowsAsReviewerKeybinding(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "as reviewer") {
		t.Error("PR tab status bar should show 'A as reviewer' keybinding")
	}
}

func TestModel_View_ShowsLogo(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	// The logo should appear in the view (contains box-drawing chars from ASCII art)
	if !strings.Contains(view, "╔═╗") {
		t.Error("View should contain the ASCII art logo")
	}
}

func TestModel_TabBar_Shows_Three_Tabs(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	// Should show all three tabs with new ordering: 1=PR, 2=Work Items, 3=Pipelines
	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("Tab bar should show '1: Pull Requests'")
	}
	if !strings.Contains(view, "2: Work Items") {
		t.Error("Tab bar should show '2: Work Items'")
	}
	if !strings.Contains(view, "3: Pipelines") {
		t.Error("Tab bar should show '3: Pipelines'")
	}
}

func TestModel_UpdateCheckMsg_ShowsNotification(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "1.0.0", "")
	m.width = 120
	m.height = 30

	// Simulate receiving an update check result
	msg := updateCheckMsg{
		info: &version.UpdateInfo{
			CurrentVersion:  "1.0.0",
			LatestVersion:   "v2.0.0",
			UpdateAvailable: true,
			ReleaseURL:      "https://github.com/Elpulgo/azdo/releases/tag/v2.0.0",
		},
	}

	updated, _ := m.Update(msg)
	updatedModel := updated.(Model)

	view := updatedModel.View()
	if !strings.Contains(view, "Update available") {
		t.Error("expected update notification in view")
	}
}

func TestModel_CriticalErrorMsg_ShowsErrorModal(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Send a CriticalErrorMsg
	msg := components.CriticalErrorMsg{
		Title:   "Configuration Error",
		Message: "The API returned 'not found'.",
		Hint:    "Check your config file.",
	}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	if !m.errorModal.IsVisible() {
		t.Error("error modal should be visible after CriticalErrorMsg")
	}

	// View should render the error modal overlay
	view := m.View()
	if !strings.Contains(view, "Configuration Error") {
		t.Error("view should show error modal with title")
	}
}

func TestModel_ThemeSwitch_PreservesConnectionState(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	// Simulate successful data fetch to set status to "connected"
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1", Definition: azdevops.PipelineDefinition{Name: "Build"}},
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	// Verify we're connected
	if m.statusBar.GetState() != polling.StateConnected {
		t.Fatal("Expected connected state before theme switch")
	}

	// Switch theme
	updated, _ = m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	// Status bar should still show connected, not connecting
	if m.statusBar.GetState() != polling.StateConnected {
		t.Errorf("Expected state to remain 'connected' after theme switch, got %q", m.statusBar.GetState())
	}

	view := m.View()
	if strings.Contains(view, "connecting") {
		t.Error("Status bar should not show 'connecting' after theme switch when already connected")
	}
	// Connected state shows icon only (●), not the word "connected"
	if !strings.Contains(view, "●") {
		t.Error("Status bar should show connected icon ● after theme switch")
	}
}

// collectMsgTypes runs a command tree (descending into tea.Batch) and returns
// the reflect type name of every leaf message it emits. Each leaf is run under
// a recover guard so a command that panics on a nil client (offline test) is
// skipped rather than failing the collection.
func collectMsgTypes(cmd tea.Cmd) []string {
	var out []string
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		var msg tea.Msg
		func() {
			defer func() { _ = recover() }()
			msg = c()
		}()
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, cc := range batch {
				run(cc)
			}
			return
		}
		out = append(out, reflect.TypeOf(msg).String())
	}
	run(cmd)
	return out
}

// TestModel_ThemeSwitch_DoesNotRefetchMetrics guards the theme-change bug: when
// the metrics tab is active, switching theme must re-style the metrics view in
// place (SetStyles) WITHOUT re-initializing it. A re-init re-runs the async
// fetch/snapshot load, whose completion message blanks the loaded trends data
// — the symptom the user reported ("changed theme and the config was blown
// away again"). We detect the regression by asserting the theme handler emits
// no metrics.* command for the active metrics tab.
func TestModel_ThemeSwitch_DoesNotRefetchMetrics(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	// Pretend the user is on the metrics tab when they change theme.
	m.activeTab = TabMetrics

	updated, cmd := m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	for _, ty := range collectMsgTypes(cmd) {
		if strings.HasPrefix(ty, "metrics.") {
			t.Errorf("theme switch re-fetched metrics (emitted %s); the view should be"+
				" re-styled in place via SetStyles, not re-initialized", ty)
		}
	}
}

func TestModel_ThemeSwitch_PreservesWarningMessage(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	// Set a warning message
	m.statusBar.SetWarningMessage("Some projects failed")

	// Switch theme
	updated, _ = m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	// Warning message should be preserved
	view := m.View()
	if !strings.Contains(view, "Some projects failed") {
		t.Error("Warning message should be preserved after theme switch")
	}
}

func TestModel_HelpModalShowsVersionInfo(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "1.5.0", "abc1234")
	m.width = 200
	m.height = 60

	// Update with window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open help modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "1.5.0") {
		t.Error("help modal should contain version number")
	}
	if !strings.Contains(view, "abc1234") {
		t.Error("help modal should contain commit hash")
	}
}

func TestModel_HelpModalShowsDevVersion(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "none")
	m.width = 200
	m.height = 60

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open help modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "dev") {
		t.Error("help modal should show 'dev' version")
	}
}

func TestModel_UpdateCheckMsg_NoUpdateAvailable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "2.0.0", "")
	m.width = 120
	m.height = 30

	msg := updateCheckMsg{
		info: &version.UpdateInfo{
			CurrentVersion:  "2.0.0",
			LatestVersion:   "v2.0.0",
			UpdateAvailable: false,
		},
	}

	updated, _ := m.Update(msg)
	updatedModel := updated.(Model)

	view := updatedModel.View()
	if strings.Contains(view, "Update available") {
		t.Error("should not show update notification when already on latest")
	}
}

func TestModel_DisabledPanes_PipelinesDisabled_TabBarHidesPipelines(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pipelines"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("Tab bar should show '1: Pull Requests'")
	}
	if !strings.Contains(view, "2: Work Items") {
		t.Error("Tab bar should show '2: Work Items'")
	}
	if strings.Contains(view, "Pipelines") {
		t.Error("Tab bar should NOT show Pipelines when disabled")
	}
}

func TestModel_DisabledPanes_WorkItemsDisabled_TabBarHidesWorkItems(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"workitems"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("Tab bar should show '1: Pull Requests'")
	}
	if strings.Contains(view, "Work Items") {
		t.Error("Tab bar should NOT show Work Items when disabled")
	}
	if !strings.Contains(view, "2: Pipelines") {
		t.Error("Tab bar should show '2: Pipelines' (renumbered)")
	}
}

func TestModel_DisabledPanes_Key2_GoesToPipelines_WhenWorkItemsDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"workitems"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Press '2' — should go to pipelines (since it's the 2nd enabled tab)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("After pressing '2' with workitems disabled, expected TabPipelines, got %d", m.activeTab)
	}
}

func TestModel_DisabledPanes_Key3_Noop_WhenOnlyTwoTabs(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"workitems"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Press '3' — should be a no-op (only 2 tabs)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("Pressing '3' with only 2 tabs should be no-op, got tab %d", m.activeTab)
	}
}

func TestModel_DisabledPanes_ArrowKeys_SkipDisabledTabs(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"workitems"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Right arrow from PR → should go to Pipelines (skipping WorkItems)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("Right arrow should skip disabled WorkItems tab, got %d", m.activeTab)
	}

	// Right arrow from Pipelines → should wrap to PR
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("Right arrow should wrap to PullRequests, got %d", m.activeTab)
	}

	// Left arrow from PR → should wrap to Pipelines
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("Left arrow should wrap to Pipelines, got %d", m.activeTab)
	}
}

func TestModel_DisabledPanes_EnabledTabs_AllEnabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}

	tabs := buildEnabledTabs(cfg)
	if len(tabs) != 3 {
		t.Fatalf("expected 3 enabled tabs, got %d", len(tabs))
	}
	if tabs[0] != TabPullRequests || tabs[1] != TabWorkItems || tabs[2] != TabPipelines {
		t.Errorf("unexpected tab order: %v", tabs)
	}
}

func TestModel_DisabledPanes_EnabledTabs_BothDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pipelines", "workitems"},
	}

	tabs := buildEnabledTabs(cfg)
	if len(tabs) != 1 {
		t.Fatalf("expected 1 enabled tab, got %d", len(tabs))
	}
	if tabs[0] != TabPullRequests {
		t.Errorf("expected TabPullRequests, got %d", tabs[0])
	}
}

// openTagPickerOnWorkItemsTab returns a Model with the work items tab active
// and the tag picker open, ready for keypress tests.
func openTagPickerOnWorkItemsTab(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	items := []provider.WorkItem{
		{Identity: provider.Identity{ID: "1"}, Title: "A", Tags: "Spring"},
	}
	updated, _ = m.Update(workitems.SetWorkItemsMsg{WorkItems: items})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = updated.(Model)

	if !m.workItemsView.IsTagPickerVisible() {
		t.Fatal("precondition failed: tag picker should be visible")
	}
	return m
}

func TestModel_GlobalShortcutsDisabledWhenTagPickerOpen(t *testing.T) {
	cases := []struct {
		name string
		key  rune
	}{
		{"q_does_not_quit", 'q'},
		{"t_does_not_open_theme_picker", 't'},
		{"help_does_not_open", '?'},
		{"digit_1_does_not_switch_tab", '1'},
		{"digit_3_does_not_switch_tab", '3'},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := openTagPickerOnWorkItemsTab(t)

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
			m = updated.(Model)

			if m.themePicker.IsVisible() {
				t.Error("theme picker opened while tag picker was active")
			}
			if m.helpModal.IsVisible() {
				t.Error("help modal opened while tag picker was active")
			}
			if m.activeTab != TabWorkItems {
				t.Errorf("active tab changed while tag picker was active, got %d", m.activeTab)
			}
			if !m.workItemsView.IsTagPickerVisible() {
				t.Error("tag picker was dismissed when it should have stayed open")
			}
			if got := m.workItemsView.TagPickerSearchQuery(); got != string(tc.key) {
				t.Errorf("expected key %q to be typed into tag search, got %q", tc.key, got)
			}
		})
	}
}
