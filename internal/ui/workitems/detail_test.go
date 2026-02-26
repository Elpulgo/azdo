package workitems

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDetailModel_ViewportUsesFullAvailableHeight(t *testing.T) {
	// The height passed to SetSize is already the content area (after app-level
	// borders and footer are subtracted). The work item detail view should only
	// subtract its own header lines (title + type/state + separator = 3 lines).
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "Active",
			WorkItemType: "Bug",
			Priority:     1,
			Description:  strings.Repeat("Long description text. ", 50),
		},
	}
	model := NewDetailModel(nil, wi)

	height := 30
	model.SetSize(80, height)

	view := model.View()
	lines := strings.Split(view, "\n")

	// Total output lines should equal the height passed in.
	if len(lines) != height {
		t.Errorf("Work item detail view output has %d lines, want %d (height passed to SetSize). "+
			"Viewport is not using full available height.", len(lines), height)
	}
}

func TestDetailModel_HasStyles(t *testing.T) {
	wi := azdevops.WorkItem{ID: 123}
	m := NewDetailModel(nil, wi)

	if m.styles == nil {
		t.Error("Expected detail model to have styles initialized")
	}
}

func TestDetailModel_WithStyles(t *testing.T) {
	wi := azdevops.WorkItem{ID: 123}
	customStyles := styles.NewStyles(styles.GetThemeByNameWithFallback("nord"))
	m := NewDetailModelWithStyles(nil, wi, customStyles)

	if m.styles != customStyles {
		t.Error("Expected detail model to use provided custom styles")
	}
}

func TestNewDetailModel(t *testing.T) {
	wi := azdevops.WorkItem{
		ID:     123,
		Fields: azdevops.WorkItemFields{Title: "Test item", State: "Active"},
	}

	m := NewDetailModel(nil, wi)

	if m.workItem.ID != 123 {
		t.Errorf("Expected work item ID 123, got %d", m.workItem.ID)
	}
	if m.workItem.Fields.Title != "Test item" {
		t.Errorf("Expected title 'Test item', got '%s'", m.workItem.Fields.Title)
	}
}

func TestDetailView_ShowsTitle(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 456,
		Fields: azdevops.WorkItemFields{
			Title:        "Important bug fix",
			State:        "Active",
			WorkItemType: "Bug",
			Priority:     1,
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	if !strings.Contains(view, "456") {
		t.Error("Expected view to contain work item ID")
	}
	if !strings.Contains(view, "Important bug fix") {
		t.Error("Expected view to contain work item title")
	}
	if !strings.Contains(view, "Active") {
		t.Error("Expected view to contain work item state")
	}
	if !strings.Contains(view, "Bug") {
		t.Error("Expected view to contain work item type in state line")
	}
}

func TestDetailView_BugShowsReproSteps(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 100,
		Fields: azdevops.WorkItemFields{
			Title:        "Login crash",
			State:        "Active",
			WorkItemType: "Bug",
			Priority:     1,
			ReproSteps:   "1. Open app\n2. Click login\n3. App crashes",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	if !strings.Contains(view, "Open app") {
		t.Error("Expected Bug detail view to show ReproSteps content, but it was missing")
	}
	if strings.Contains(view, "No description") {
		t.Error("Bug with ReproSteps should not show 'No description'")
	}
}

func TestDetailView_BugWithoutReproStepsFallsBackToDescription(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 101,
		Fields: azdevops.WorkItemFields{
			Title:        "Minor issue",
			State:        "New",
			WorkItemType: "Bug",
			Priority:     3,
			Description:  "This is a bug description fallback",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	if !strings.Contains(view, "bug description fallback") {
		t.Error("Bug without ReproSteps should fall back to Description")
	}
}

func TestDetailView_TaskShowsDescription(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 102,
		Fields: azdevops.WorkItemFields{
			Title:        "Implement feature",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Description:  "Task description content here",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	if !strings.Contains(view, "Task description content here") {
		t.Error("Task should show Description field content")
	}
}

func TestDetailView_NoTypeIconInTitle(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 789,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "New",
			WorkItemType: "Bug",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	// Title line should not contain emoji icons
	if strings.Contains(view, "🐛") {
		t.Error("Title should not contain bug emoji icon")
	}
	if strings.Contains(view, "📋") {
		t.Error("Title should not contain task emoji icon")
	}
}

func TestDetailView_Scrolling(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 789,
		Fields: azdevops.WorkItemFields{
			Title:       "Long description item",
			Description: strings.Repeat("This is a long description. ", 100),
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(80, 20)

	// Test scroll down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	// View should still render without error
	view := m.View()
	if view == "" {
		t.Error("Expected view to render after scrolling")
	}
}

func TestGetScrollPercent(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title: "Test",
		},
	}

	m := NewDetailModel(nil, wi)

	// Before SetSize, should return 0
	percent := m.GetScrollPercent()
	if percent != 0 {
		t.Errorf("Expected 0 scroll percent before ready, got %f", percent)
	}

	// After SetSize, should return valid percent
	m.SetSize(80, 20)
	percent = m.GetScrollPercent()
	// Scroll percent could be 0 or higher depending on content
	if percent < 0 {
		t.Errorf("Expected non-negative scroll percent, got %f", percent)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<div>Hello <b>World</b></div>", "Hello World"},
		{"Plain text", "Plain text"},
		{"&nbsp;spaces&nbsp;", "spaces"},
		{"&lt;not&gt; tags", "<not> tags"},
		{"&amp;&quot;&#39;", "&\"'"},
		{"<p>Line 1</p><p>Line 2</p>", "Line 1\nLine 2"},
		{"Hello<br>World", "Hello\nWorld"},
		{"Hello<br/>World", "Hello\nWorld"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.expected {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShortenIterationPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Project\\Sprint 1", "Project\\Sprint 1"},
		{"Project\\Release 1\\Sprint 1", "Release 1\\Sprint 1"},
		{"Very\\Long\\Path\\Sprint 1", "Path\\Sprint 1"},
		{"Single", "Single"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shortenIterationPath(tt.input)
			if got != tt.expected {
				t.Errorf("shortenIterationPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildWorkItemURL(t *testing.T) {
	tests := []struct {
		org     string
		project string
		id      int
		want    string
	}{
		{"myorg", "myproject", 123, "https://dev.azure.com/myorg/myproject/_workitems/edit/123"},
		{"", "project", 123, ""},
		{"org", "", 123, ""},
	}

	for _, tt := range tests {
		got := buildWorkItemURL(tt.org, tt.project, tt.id)
		if got != tt.want {
			t.Errorf("buildWorkItemURL(%q, %q, %d) = %q, want %q", tt.org, tt.project, tt.id, got, tt.want)
		}
	}
}

func TestDetailModel_GetContextItems(t *testing.T) {
	wi := azdevops.WorkItem{ID: 123}
	m := NewDetailModel(nil, wi)

	items := m.GetContextItems()
	if len(items) == 0 {
		t.Error("Expected context items to be non-empty")
	}
}

func TestDetailModel_GetStatusMessage(t *testing.T) {
	wi := azdevops.WorkItem{ID: 123}
	m := NewDetailModel(nil, wi)

	msg := m.GetStatusMessage()
	if msg != "" {
		t.Errorf("Expected empty status message, got %s", msg)
	}
}

func TestGetWorkItem(t *testing.T) {
	wi := azdevops.WorkItem{ID: 999, Fields: azdevops.WorkItemFields{Title: "Test WI"}}
	m := NewDetailModel(nil, wi)

	got := m.GetWorkItem()
	if got.ID != 999 {
		t.Errorf("Expected ID 999, got %d", got.ID)
	}
	if got.Fields.Title != "Test WI" {
		t.Errorf("Expected title 'Test WI', got '%s'", got.Fields.Title)
	}
}

func TestDetailModel_SKeyStartsLoading(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "Active",
			WorkItemType: "Bug",
		},
	}
	m := NewDetailModel(nil, wi)
	m.SetSize(80, 30)

	// Press 's' should start loading states
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	if !m.loading {
		t.Error("Expected loading to be true after pressing 's'")
	}
	if cmd == nil {
		t.Error("Expected command to be returned for fetching states")
	}
}

func TestDetailModel_StatesLoadedOpensStatePicker(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "Active",
			WorkItemType: "Bug",
		},
	}
	m := NewDetailModel(nil, wi)
	m.SetSize(80, 30)

	// Simulate states being loaded
	m, _ = m.Update(statesLoadedMsg{
		states: []azdevops.WorkItemTypeState{
			{Name: "New", Color: "b2b2b2", Category: "Proposed"},
			{Name: "Active", Color: "007acc", Category: "InProgress"},
			{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
			{Name: "Closed", Color: "339933", Category: "Completed"},
		},
	})

	if !m.statePicker.IsVisible() {
		t.Error("Expected state picker to be visible after states loaded")
	}
}

func TestDetailModel_StateUpdateSuccess(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "Active",
			WorkItemType: "Bug",
		},
	}
	m := NewDetailModel(nil, wi)
	m.SetSize(80, 30)

	// Simulate successful state update
	m, _ = m.Update(stateUpdateResultMsg{newState: "Resolved"})

	if m.workItem.Fields.State != "Resolved" {
		t.Errorf("Expected state 'Resolved', got %q", m.workItem.Fields.State)
	}
	if m.statusMessage == "" {
		t.Error("Expected status message after state update")
	}
}

func TestDetailModel_StateUpdateError(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "Active",
			WorkItemType: "Bug",
		},
	}
	m := NewDetailModel(nil, wi)
	m.SetSize(80, 30)

	// Simulate failed state update
	m, _ = m.Update(stateUpdateResultMsg{err: fmt.Errorf("access denied")})

	if m.workItem.Fields.State != "Active" {
		t.Errorf("Expected state to remain 'Active' after error, got %q", m.workItem.Fields.State)
	}
	if !strings.Contains(m.statusMessage, "Error") {
		t.Error("Expected error in status message")
	}
}

func TestDetailModel_StatePickerRoutesInput(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 123,
		Fields: azdevops.WorkItemFields{
			Title:        "Test item",
			State:        "Active",
			WorkItemType: "Bug",
		},
	}
	m := NewDetailModel(nil, wi)
	m.SetSize(80, 30)

	// Open state picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	// Navigate down in the picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Escape should close the picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.statePicker.IsVisible() {
		t.Error("Expected state picker to be hidden after escape")
	}
}

func TestDetailView_LinkBeforeDescription(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 200,
		Fields: azdevops.WorkItemFields{
			Title:        "Test ordering",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Description:  "This is the description text",
		},
	}

	client, _ := azdevops.NewClient("myorg", "myproject", "fake-pat")
	m := NewDetailModel(client, wi)
	m.SetSize(100, 40)

	view := m.View()

	linkIdx := strings.Index(view, "Open in browser")
	descIdx := strings.Index(view, "Description")

	if linkIdx == -1 {
		t.Fatal("Expected 'Open in browser' link in view")
	}
	if descIdx == -1 {
		t.Fatal("Expected 'Description' label in view")
	}
	if linkIdx >= descIdx {
		t.Errorf("Expected 'Open in browser' (pos %d) to appear before 'Description' (pos %d)", linkIdx, descIdx)
	}
}

func TestDetailModel_GetContextItemsIncludesStateChange(t *testing.T) {
	wi := azdevops.WorkItem{ID: 123}
	m := NewDetailModel(nil, wi)

	items := m.GetContextItems()
	found := false
	for _, item := range items {
		if item.Key == "s" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected context items to include 's' keybinding for state change")
	}
}
