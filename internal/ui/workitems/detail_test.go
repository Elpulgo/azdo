package workitems

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestDetailView_ShowsTags(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 300,
		Fields: azdevops.WorkItemFields{
			Title:        "Tagged work item",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Tags:         "Sprint 5; Frontend; Critical",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	if !strings.Contains(view, "Tags") {
		t.Error("Expected view to contain 'Tags' label when work item has tags")
	}
	if !strings.Contains(view, "Sprint 5") {
		t.Error("Expected view to contain tag 'Sprint 5'")
	}
	if !strings.Contains(view, "Frontend") {
		t.Error("Expected view to contain tag 'Frontend'")
	}
	if !strings.Contains(view, "Critical") {
		t.Error("Expected view to contain tag 'Critical'")
	}
}

func TestDetailView_NoTagsSectionWhenEmpty(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 301,
		Fields: azdevops.WorkItemFields{
			Title:        "No tags item",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	// "Tags" label should NOT appear when there are no tags
	// We need to be careful: "Tags" could appear in other content.
	// Check that the specific "Tags:" label pattern is absent.
	if strings.Contains(view, "Tags:") {
		t.Error("Expected 'Tags:' label to NOT appear when work item has no tags")
	}
}

func TestDetailView_TagsAppearBeforeDescription(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 302,
		Fields: azdevops.WorkItemFields{
			Title:        "Ordering test",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Tags:         "Backend",
			Description:  "Some description text",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 40)

	view := m.View()

	tagsIdx := strings.Index(view, "Backend")
	descIdx := strings.Index(view, "Description")

	if tagsIdx == -1 {
		t.Fatal("Expected tag 'Backend' in view")
	}
	if descIdx == -1 {
		t.Fatal("Expected 'Description' in view")
	}
	if tagsIdx >= descIdx {
		t.Errorf("Expected tags (pos %d) to appear before description (pos %d)", tagsIdx, descIdx)
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

func TestDetailView_ShowsLastChangedTimestamp(t *testing.T) {
	changedAt := time.Date(2026, 3, 1, 14, 30, 0, 0, time.UTC)

	wi := azdevops.WorkItem{
		ID: 500,
		Fields: azdevops.WorkItemFields{
			Title:        "Timestamped item",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			ChangedDate:  changedAt,
			Description:  "Some description",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 40)

	view := m.View()

	// The user should see when the item was last changed
	if !strings.Contains(view, "2026-03-01 14:30") {
		t.Error("Expected detail view to show formatted ChangedDate timestamp '2026-03-01 14:30'")
	}
	if !strings.Contains(view, "Last changed") {
		t.Error("Expected detail view to show 'Last changed' label")
	}
}

func TestDetailView_TimestampAppearsBeforeDescription(t *testing.T) {
	changedAt := time.Date(2026, 2, 15, 9, 0, 0, 0, time.UTC)

	wi := azdevops.WorkItem{
		ID: 501,
		Fields: azdevops.WorkItemFields{
			Title:        "Ordering check",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			ChangedDate:  changedAt,
			Description:  "Description content here",
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 40)

	view := m.View()

	tsIdx := strings.Index(view, "2026-02-15")
	descIdx := strings.Index(view, "Description")

	if tsIdx == -1 {
		t.Fatal("Expected timestamp to be present in view")
	}
	if descIdx == -1 {
		t.Fatal("Expected 'Description' to be present in view")
	}
	if tsIdx >= descIdx {
		t.Errorf("Expected timestamp (pos %d) to appear before description (pos %d)", tsIdx, descIdx)
	}
}

func TestDetailView_ZeroChangedDateIsHidden(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 502,
		Fields: azdevops.WorkItemFields{
			Title:        "No timestamp item",
			State:        "New",
			WorkItemType: "Task",
			Priority:     3,
			// ChangedDate is zero value
		},
	}

	m := NewDetailModel(nil, wi)
	m.SetSize(100, 30)

	view := m.View()

	if strings.Contains(view, "Last changed") {
		t.Error("Expected 'Last changed' label to NOT appear when ChangedDate is zero")
	}
}

func TestDetailView_LongDescriptionWrapsWithinViewWidth(t *testing.T) {
	// A single long line that far exceeds any reasonable terminal width.
	// This is a realistic scenario: Azure DevOps descriptions often come as
	// a single run of HTML text that, once stripped, becomes one huge line.
	longLine := strings.Repeat("word ", 80) // ~400 chars, well over 80-col terminal
	wi := azdevops.WorkItem{
		ID: 700,
		Fields: azdevops.WorkItemFields{
			Title:        "Overflow test",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Description:  longLine,
		},
	}

	viewWidth := 80
	m := NewDetailModel(nil, wi)
	m.SetSize(viewWidth, 40)

	view := m.View()
	lines := strings.Split(view, "\n")

	for i, line := range lines {
		w := lipgloss.Width(line)
		if w > viewWidth {
			t.Errorf("line %d exceeds view width %d (visual width %d): %q",
				i+1, viewWidth, w, truncateForTest(line, 60))
		}
	}
}

func TestDetailView_LongHTMLDescriptionWrapsWithinViewWidth(t *testing.T) {
	// Realistic Azure DevOps content: HTML paragraphs that become long lines
	// after stripping tags.
	htmlDesc := "<p>" + strings.Repeat("This is a detailed description of the work item that explains the requirements in full. ", 10) + "</p>"
	wi := azdevops.WorkItem{
		ID: 701,
		Fields: azdevops.WorkItemFields{
			Title:        "HTML overflow test",
			State:        "New",
			WorkItemType: "Bug",
			Priority:     1,
			ReproSteps:   htmlDesc,
		},
	}

	viewWidth := 60
	m := NewDetailModel(nil, wi)
	m.SetSize(viewWidth, 30)

	view := m.View()
	lines := strings.Split(view, "\n")

	for i, line := range lines {
		w := lipgloss.Width(line)
		if w > viewWidth {
			t.Errorf("line %d exceeds view width %d (visual width %d): %q",
				i+1, viewWidth, w, truncateForTest(line, 60))
		}
	}
}

func TestDetailView_WrappedDescriptionPreservesContent(t *testing.T) {
	// When we wrap a long description, the text should still be fully present
	// in the output — just broken across multiple lines, not truncated.
	description := "The quick brown fox jumps over the lazy dog and then continues running across the field until reaching the distant forest"
	wi := azdevops.WorkItem{
		ID: 702,
		Fields: azdevops.WorkItemFields{
			Title:        "Content preservation test",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Description:  description,
		},
	}

	// Narrow width forces wrapping
	m := NewDetailModel(nil, wi)
	m.SetSize(40, 30)

	view := m.View()

	// Every word from the description should still appear in the rendered output
	for _, word := range strings.Fields(description) {
		if !strings.Contains(view, word) {
			t.Errorf("word %q from description is missing in the rendered view — wrapping may be truncating content", word)
		}
	}
}

func TestDetailView_DescriptionRespectsResizedWidth(t *testing.T) {
	// When the terminal is resized, the description wrapping should adapt
	// to the new width.
	longLine := strings.Repeat("alpha beta gamma delta ", 20)
	wi := azdevops.WorkItem{
		ID: 703,
		Fields: azdevops.WorkItemFields{
			Title:        "Resize test",
			State:        "Active",
			WorkItemType: "Task",
			Priority:     2,
			Description:  longLine,
		},
	}

	m := NewDetailModel(nil, wi)

	// First render at 100 columns
	m.SetSize(100, 30)

	// Now shrink to 50 columns
	m.SetSize(50, 30)

	view := m.View()
	lines := strings.Split(view, "\n")

	for i, line := range lines {
		w := lipgloss.Width(line)
		if w > 50 {
			t.Errorf("after resize to 50 cols, line %d still exceeds width (visual width %d): %q",
				i+1, w, truncateForTest(line, 50))
		}
	}
}

// truncateForTest shortens a string for readable test output
func truncateForTest(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
