package app

import (
	"fmt"
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

func TestModel_TabSwitching_To_WorkItems(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Press '3' to switch to work items tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	if m.activeTab != TabWorkItems {
		t.Errorf("After pressing '3', activeTab should be TabWorkItems (2), got %d", m.activeTab)
	}
}

func TestModel_View_ShowsWorkItems_WhenActiveTab(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	// Switch to work items tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	view := m.View()

	// Should show work items content (empty list message or similar)
	if !strings.Contains(view, "work item") && !strings.Contains(view, "No work items") {
		t.Error("View should show work items content when on Work Items tab")
	}
}

func TestModel_View_HasBorderedTabBar(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
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
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
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
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	// The first line should contain the top-left rounded corner of the tab border
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
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)

	// Simulate window size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
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

	// Tab bar border should be on line 0
	if !strings.Contains(lines[0], "╭") {
		t.Errorf("Line 0 should have tab bar top border, got: %.80q", lines[0])
	}
}

func TestModel_View_ContentFillsBoxWithoutExcessPadding(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	terminalHeight := 40
	m := NewModel(client, cfg)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: terminalHeight})
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
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	terminalHeights := []int{24, 30, 40, 50}
	for _, termHeight := range terminalHeights {
		t.Run(fmt.Sprintf("height_%d", termHeight), func(t *testing.T) {
			m := NewModel(client, cfg)

			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: termHeight})
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
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
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
		t.Error("Pressing '2' during search should NOT switch to PR tab")
	}

	// Press 'q' — should NOT quit (we can't easily test quit, but we can ensure model is returned)
	// Press esc to exit search
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.isActiveViewSearching() {
		t.Error("Expected search to be exited after esc")
	}

	// Now '2' should work again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Error("After exiting search, '2' should switch to PR tab")
	}
}

func TestModel_TabBar_Shows_Three_Tabs(t *testing.T) {
	cfg := &config.Config{
		Organization: "testorg",
		Project:      "testproject",
	}
	client := &azdevops.Client{}

	m := NewModel(client, cfg)
	m.width = 100
	m.height = 30

	view := m.View()

	// Should show all three tabs in the tab bar
	if !strings.Contains(view, "Pipelines") {
		t.Error("Tab bar should show Pipelines")
	}
	if !strings.Contains(view, "Pull Requests") {
		t.Error("Tab bar should show Pull Requests")
	}
	if !strings.Contains(view, "Work Items") {
		t.Error("Tab bar should show Work Items")
	}
}
