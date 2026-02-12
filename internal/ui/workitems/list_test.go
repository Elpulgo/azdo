package workitems

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	m := NewModel(nil)

	// Check initial state
	if m.viewMode != ViewList {
		t.Errorf("Expected viewMode to be ViewList, got %v", m.viewMode)
	}
	if m.loading {
		t.Error("Expected loading to be false initially")
	}
	if m.err != nil {
		t.Errorf("Expected err to be nil, got %v", m.err)
	}
	if len(m.workItems) != 0 {
		t.Errorf("Expected empty workItems, got %d", len(m.workItems))
	}
}

func TestUpdate_SetWorkItemsMsg(t *testing.T) {
	m := NewModel(nil)
	m.loading = true
	m.width = 100
	m.height = 30

	// Simulate receiving work items
	workItems := []azdevops.WorkItem{
		{
			ID:     123,
			Fields: azdevops.WorkItemFields{Title: "Fix bug", State: "Active", WorkItemType: "Bug"},
		},
		{
			ID:     456,
			Fields: azdevops.WorkItemFields{Title: "Add feature", State: "New", WorkItemType: "Task"},
		},
	}

	msg := SetWorkItemsMsg{WorkItems: workItems}
	m, _ = m.Update(msg)

	if m.loading {
		t.Error("Expected loading to be false after SetWorkItemsMsg")
	}
	if len(m.workItems) != 2 {
		t.Errorf("Expected 2 work items, got %d", len(m.workItems))
	}
	if m.workItems[0].ID != 123 {
		t.Errorf("Expected first work item ID to be 123, got %d", m.workItems[0].ID)
	}
}

func TestUpdate_Error(t *testing.T) {
	m := NewModel(nil)
	m.loading = true

	msg := workItemsMsg{err: tea.ErrInterrupted}
	m, _ = m.Update(msg)

	if m.loading {
		t.Error("Expected loading to be false after error")
	}
	if m.err == nil {
		t.Error("Expected err to be set after error message")
	}
}

func TestViewMode_Navigation(t *testing.T) {
	m := NewModel(nil)
	m.width = 100
	m.height = 30

	// Add some work items
	m.workItems = []azdevops.WorkItem{
		{
			ID:     123,
			Fields: azdevops.WorkItemFields{Title: "Fix bug", State: "Active", WorkItemType: "Bug"},
		},
	}
	m.table.SetRows(m.workItemsToRows())

	// Simulate pressing Enter to go to detail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.viewMode != ViewDetail {
		t.Errorf("Expected ViewDetail after Enter, got %v", m.viewMode)
	}
	if m.detail == nil {
		t.Error("Expected detail model to be set")
	}

	// Simulate pressing Esc to go back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.viewMode != ViewList {
		t.Errorf("Expected ViewList after Esc, got %v", m.viewMode)
	}
}

func TestView_Loading(t *testing.T) {
	m := NewModel(nil)
	m.loading = true
	m.spinner.SetVisible(true)
	m.width = 100
	m.height = 30

	view := m.View()

	// Check that loading state shows spinner content (which includes "Loading work items" message)
	if !strings.Contains(view, "work items") && !strings.Contains(view, "quit") {
		t.Error("Expected view to show loading message or quit instruction")
	}
}

func TestView_Error(t *testing.T) {
	m := NewModel(nil)
	m.err = tea.ErrInterrupted
	m.width = 100
	m.height = 30

	view := m.View()

	if !strings.Contains(view, "Error") {
		t.Error("Expected view to show error message")
	}
}

func TestView_Empty(t *testing.T) {
	m := NewModel(nil)
	m.workItems = []azdevops.WorkItem{}
	m.width = 100
	m.height = 30

	view := m.View()

	if !strings.Contains(view, "No work items") {
		t.Error("Expected view to show empty message")
	}
}

func TestWorkItemsToRows(t *testing.T) {
	m := NewModel(nil)
	m.workItems = []azdevops.WorkItem{
		{
			ID: 123,
			Fields: azdevops.WorkItemFields{
				Title:        "Fix critical bug",
				State:        "Active",
				WorkItemType: "Bug",
				Priority:     1,
				AssignedTo:   &azdevops.Identity{DisplayName: "John Doe"},
			},
		},
		{
			ID: 456,
			Fields: azdevops.WorkItemFields{
				Title:        "Add new feature",
				State:        "New",
				WorkItemType: "Task",
				Priority:     2,
				AssignedTo:   nil,
			},
		},
	}

	rows := m.workItemsToRows()

	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}

	// Check first row
	row := rows[0]
	if row[1] != "123" {
		t.Errorf("Expected ID '123', got '%s'", row[1])
	}
	if row[2] != "Fix critical bug" {
		t.Errorf("Expected title 'Fix critical bug', got '%s'", row[2])
	}
	// Row[5] is assigned to
	if row[5] != "John Doe" {
		t.Errorf("Expected assigned to 'John Doe', got '%s'", row[5])
	}

	// Check second row - nil assignee should show "-"
	row2 := rows[1]
	if row2[5] != "-" {
		t.Errorf("Expected assigned to '-' for nil, got '%s'", row2[5])
	}
}

func TestListModel_GetContextItems_ListMode(t *testing.T) {
	m := NewModel(nil)
	m.viewMode = ViewList

	items := m.GetContextItems()
	if items != nil {
		t.Error("Expected nil context items for list mode")
	}
}

func TestListModel_HasContextBar(t *testing.T) {
	m := NewModel(nil)

	// List mode should not have context bar
	m.viewMode = ViewList
	if m.HasContextBar() {
		t.Error("Expected no context bar for list mode")
	}
}

func TestListModel_GetScrollPercent_ListMode(t *testing.T) {
	m := NewModel(nil)
	m.viewMode = ViewList

	percent := m.GetScrollPercent()
	if percent != 0 {
		t.Errorf("Expected 0 scroll percent for list mode, got %f", percent)
	}
}

func TestListModel_GetStatusMessage(t *testing.T) {
	m := NewModel(nil)

	msg := m.GetStatusMessage()
	if msg != "" {
		t.Errorf("Expected empty status message, got %s", msg)
	}
}
