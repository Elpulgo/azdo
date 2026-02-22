package workitems

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel_HasStyles(t *testing.T) {
	m := NewModel(nil)

	if m.styles == nil {
		t.Error("Expected model to have styles initialized")
	}
}

func TestNewModelWithStyles(t *testing.T) {
	customStyles := styles.NewStyles(styles.GetThemeByNameWithFallback("gruvbox"))
	m := NewModelWithStyles(nil, customStyles)

	if m.styles != customStyles {
		t.Error("Expected model to use provided custom styles")
	}
}

func TestTypeIconWithStyles(t *testing.T) {
	themes := []string{"dark", "gruvbox", "nord", "dracula"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			tests := []struct {
				workItemType string
				wantContains string
			}{
				{"Bug", "Bug"},
				{"Task", "Task"},
				{"User Story", "Story"},
				{"Feature", "Feature"},
			}

			for _, tt := range tests {
				got := typeIconWithStyles(tt.workItemType, s)
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("typeIconWithStyles(%q) with theme %s = %q, want to contain %q",
						tt.workItemType, themeName, got, tt.wantContains)
				}
			}
		})
	}
}

func TestStateTextWithStyles(t *testing.T) {
	themes := []string{"dark", "nord"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			tests := []struct {
				state        string
				wantContains string
			}{
				{"New", "New"},
				{"Active", "Active"},
				{"Closed", "Closed"},
			}

			for _, tt := range tests {
				got := stateTextWithStyles(tt.state, s)
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("stateTextWithStyles(%q) with theme %s = %q, want to contain %q",
						tt.state, themeName, got, tt.wantContains)
				}
			}
		})
	}
}

func TestPriorityTextWithStyles(t *testing.T) {
	themes := []string{"dark", "gruvbox"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			tests := []struct {
				priority     int
				wantContains string
			}{
				{1, "P1"},
				{2, "P2"},
				{3, "P3"},
				{4, "P4"},
			}

			for _, tt := range tests {
				got := priorityTextWithStyles(tt.priority, s)
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("priorityTextWithStyles(%d) with theme %s = %q, want to contain %q",
						tt.priority, themeName, got, tt.wantContains)
				}
			}
		})
	}
}

func TestNewModel(t *testing.T) {
	m := NewModel(nil)

	// Check initial state
	if m.GetViewMode() != ViewList {
		t.Errorf("Expected viewMode to be ViewList, got %v", m.GetViewMode())
	}
	if len(m.list.Items()) != 0 {
		t.Errorf("Expected empty workItems, got %d", len(m.list.Items()))
	}
}

func TestUpdate_SetWorkItemsMsg(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

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

	if len(m.list.Items()) != 2 {
		t.Errorf("Expected 2 work items, got %d", len(m.list.Items()))
	}
	if m.list.Items()[0].ID != 123 {
		t.Errorf("Expected first work item ID to be 123, got %d", m.list.Items()[0].ID)
	}
}

func TestUpdate_Error(t *testing.T) {
	m := NewModel(nil)

	msg := workItemsMsg{err: tea.ErrInterrupted}
	m, _ = m.Update(msg)

	// View should show error
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := m.View()
	if !strings.Contains(view, "Error") {
		t.Error("Expected view to show error message")
	}
}

func TestViewMode_Navigation(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Add some work items via SetItems
	workItems := []azdevops.WorkItem{
		{
			ID:     123,
			Fields: azdevops.WorkItemFields{Title: "Fix bug", State: "Active", WorkItemType: "Bug"},
		},
	}
	m.list = m.list.SetItems(workItems)

	// Simulate pressing Enter to go to detail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetViewMode() != ViewDetail {
		t.Errorf("Expected ViewDetail after Enter, got %v", m.GetViewMode())
	}
	if m.list.Detail() == nil {
		t.Error("Expected detail model to be set")
	}

	// Simulate pressing Esc to go back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.GetViewMode() != ViewList {
		t.Errorf("Expected ViewList after Esc, got %v", m.GetViewMode())
	}
}

func TestView_Loading(t *testing.T) {
	m := NewModel(nil)
	// Init triggers loading
	m.Init()
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()

	// Check that loading state shows spinner content
	if !strings.Contains(view, "work items") && !strings.Contains(view, "quit") {
		t.Error("Expected view to show loading message or quit instruction")
	}
}

func TestView_Error(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m, _ = m.Update(workItemsMsg{err: tea.ErrInterrupted})

	view := m.View()
	if !strings.Contains(view, "Error") {
		t.Error("Expected view to show error message")
	}
}

func TestView_Empty(t *testing.T) {
	m := NewModel(nil)
	m.list = m.list.SetItems([]azdevops.WorkItem{})
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	if !strings.Contains(view, "No work items") {
		t.Error("Expected view to show empty message")
	}
}

func TestWorkItemsToRows(t *testing.T) {
	s := styles.DefaultStyles()
	items := []azdevops.WorkItem{
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

	rows := workItemsToRows(items, s)

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

	items := m.GetContextItems()
	if items != nil {
		t.Error("Expected nil context items for list mode")
	}
}

func TestListModel_HasContextBar(t *testing.T) {
	m := NewModel(nil)

	if m.HasContextBar() {
		t.Error("Expected no context bar for list mode")
	}
}

func TestListModel_GetScrollPercent_ListMode(t *testing.T) {
	m := NewModel(nil)

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

func TestFilterWorkItem(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 42,
		Fields: azdevops.WorkItemFields{
			Title:        "Fix critical login bug",
			State:        "Active",
			WorkItemType: "Bug",
			AssignedTo:   &azdevops.Identity{DisplayName: "Jane Smith"},
		},
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"login", true},        // matches title
		{"LOGIN", true},        // case-insensitive
		{"42", true},           // matches ID
		{"active", true},       // matches state
		{"jane", true},         // matches assigned to
		{"Bug", true},          // matches type
		{"nonexistent", false}, // no match
		{"", true},             // empty matches all
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := filterWorkItem(wi, tt.query)
			if got != tt.want {
				t.Errorf("filterWorkItem(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestFilterWorkItem_NilAssignedTo(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 10,
		Fields: azdevops.WorkItemFields{
			Title:        "Unassigned task",
			State:        "New",
			WorkItemType: "Task",
			AssignedTo:   nil,
		},
	}

	// Should match on title but not crash on nil AssignedTo
	if !filterWorkItem(wi, "unassigned") {
		t.Error("Expected match on title")
	}
	if filterWorkItem(wi, "jane") {
		t.Error("Expected no match on nonexistent assignee")
	}
}
