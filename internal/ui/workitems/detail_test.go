package workitems

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

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
