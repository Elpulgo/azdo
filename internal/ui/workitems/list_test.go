package workitems

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

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

func TestFilterWorkItem_MatchesTags(t *testing.T) {
	wi := azdevops.WorkItem{
		ID: 42,
		Fields: azdevops.WorkItemFields{
			Title:        "Fix login bug",
			State:        "Active",
			WorkItemType: "Bug",
			Tags:         "Sprint 1; Backend; Urgent",
		},
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"Sprint", true},   // partial tag match
		{"sprint 1", true}, // case-insensitive tag match
		{"backend", true},  // exact tag match (case-insensitive)
		{"urgent", true},   // another tag
		{"Sprint 2", false}, // no such tag
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

func TestFilterWorkItemMulti_MatchesTags(t *testing.T) {
	wi := azdevops.WorkItem{
		ID:          42,
		Fields:      azdevops.WorkItemFields{Title: "Test", WorkItemType: "Task", Tags: "Sprint 1; Backend"},
		ProjectName: "alpha",
	}

	if !filterWorkItemMulti(wi, "backend") {
		t.Error("filterWorkItemMulti should match tag 'backend'")
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

func TestWorkItemsToRowsMulti_IncludesProjectColumn(t *testing.T) {
	s := styles.DefaultStyles()
	items := []azdevops.WorkItem{
		{
			ID:                 100,
			Fields:             azdevops.WorkItemFields{Title: "Test Item", WorkItemType: "Task", State: "Active", Priority: 2},
			ProjectName:        "alpha",
			ProjectDisplayName: "alpha",
		},
	}

	rows := workItemsToRowsMulti(items, s)
	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if len(row) != 7 {
		t.Fatalf("Expected 7 columns (with Project), got %d", len(row))
	}
	if row[0] != "alpha" {
		t.Errorf("Project column = %q, want 'alpha'", row[0])
	}
}

func TestFilterWorkItemMulti_MatchesProjectName(t *testing.T) {
	wi := azdevops.WorkItem{
		ID:          100,
		Fields:      azdevops.WorkItemFields{Title: "Test", WorkItemType: "Task"},
		ProjectName: "alpha",
	}

	if !filterWorkItemMulti(wi, "alpha") {
		t.Error("filterWorkItemMulti should match project name 'alpha'")
	}
	if filterWorkItemMulti(wi, "beta") {
		t.Error("filterWorkItemMulti should not match 'beta'")
	}
}

// --- My Items Toggle Tests ---

func TestMyItems_Toggle(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if m.myItemsOnly {
		t.Error("myItemsOnly should be false initially")
	}
	if m.IsMyItemsActive() {
		t.Error("IsMyItemsActive should be false initially")
	}

	// Add items
	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "My task"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Other task"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Press 'm' to toggle ON — fires a fetch command
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if !m.myItemsOnly {
		t.Error("myItemsOnly should be true after pressing m")
	}
	if !m.IsMyItemsActive() {
		t.Error("IsMyItemsActive should be true after pressing m")
	}
	if cmd == nil {
		t.Error("expected a fetch command when toggling on")
	}

	// Simulate @Me results arriving
	myItems := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "My task"}},
	}
	m, _ = m.Update(myWorkItemsMsg{workItems: myItems})

	if len(m.list.Items()) != 1 {
		t.Errorf("expected 1 item after @Me fetch, got %d", len(m.list.Items()))
	}

	// Press 'm' to toggle OFF — restores all items
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if m.myItemsOnly {
		t.Error("myItemsOnly should be false after second press")
	}

	// Should show all items again
	if len(m.list.Items()) != 2 {
		t.Errorf("expected 2 items after toggle off, got %d", len(m.list.Items()))
	}
}

func TestMyItems_ToggleIgnoredDuringSearch(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Enter search mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if !m.IsSearching() {
		t.Fatal("should be in search mode")
	}

	// Try to toggle 'm' - should be ignored during search
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if m.myItemsOnly {
		t.Error("m key should be ignored during search mode")
	}
}

func TestMyItems_ToggleIgnoredInDetailView(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item", WorkItemType: "Task"}},
	}
	m.list = m.list.SetItems(items)

	// Enter detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetViewMode() != ViewDetail {
		t.Fatal("should be in detail view")
	}

	// Try to toggle 'm' - should be ignored in detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if m.myItemsOnly {
		t.Error("m key should be ignored in detail view")
	}
}

func TestMyItems_PollingWhileFilterActive_DoesNotChangeVisible(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Set initial items
	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Mine"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Theirs"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Toggle on — fires fetch command
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Simulate @Me results
	m, _ = m.Update(myWorkItemsMsg{workItems: []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Mine"}},
	}})

	if len(m.list.Items()) != 1 {
		t.Fatalf("expected 1 @Me item, got %d", len(m.list.Items()))
	}

	// New items arrive via polling while filter is active
	newItems := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Mine"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Title: "New item"}},
		{ID: 4, Fields: azdevops.WorkItemFields{Title: "Another new"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: newItems})

	// Visible list should not change (still showing @Me results)
	if !m.myItemsOnly {
		t.Error("myItemsOnly should still be true")
	}
	if len(m.list.Items()) != 1 {
		t.Errorf("expected 1 visible item (unchanged), got %d", len(m.list.Items()))
	}

	// But allItems should be updated
	if len(m.allItems) != 3 {
		t.Errorf("expected 3 allItems after polling, got %d", len(m.allItems))
	}

	// Toggle off → should show updated allItems
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if len(m.list.Items()) != 3 {
		t.Errorf("expected 3 items after toggle off, got %d", len(m.list.Items()))
	}
}

func TestMyItems_FetchError_FallsBack(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Toggle on
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Simulate fetch error
	m, _ = m.Update(myWorkItemsMsg{err: tea.ErrInterrupted})

	// Should fall back: myItemsOnly should be false
	if m.myItemsOnly {
		t.Error("myItemsOnly should be false after fetch error")
	}
}

func TestRefresh_WhileMyItemsActive_ClearsLoading(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Set initial items and toggle my-items filter on
	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Mine"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Theirs"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = m.Update(myWorkItemsMsg{workItems: []azdevops.WorkItem{items[0]}})

	if !m.myItemsOnly {
		t.Fatal("myItemsOnly should be true")
	}

	// Press 'r' to refresh — this sets loading=true on the listview
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Simulate the all-items fetch returning (workItemsMsg)
	newItems := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Mine (updated)"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Theirs"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Title: "New item"}},
	}
	m, cmd := m.Update(workItemsMsg{workItems: newItems})

	// allItems should be updated
	if len(m.allItems) != 3 {
		t.Errorf("expected 3 allItems, got %d", len(m.allItems))
	}

	// A follow-up command should be returned to fetch my items
	if cmd == nil {
		t.Fatal("expected a command to fetch my items")
	}

	// Simulate my-items fetch returning
	m, _ = m.Update(myWorkItemsMsg{workItems: []azdevops.WorkItem{newItems[0]}})

	// View should NOT be stuck on "Loading work items..."
	view := m.View()
	if strings.Contains(view, "Loading work items") {
		t.Error("view should not be stuck on loading after refresh with my-items active")
	}
}

func TestRefresh_AfterStateChange_UpdatesList(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Bug", State: "Active", WorkItemType: "Bug"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Enter detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.GetViewMode() != ViewDetail {
		t.Fatal("expected detail view")
	}

	// Simulate a successful state change in detail view
	m, cmd := m.Update(WorkItemStateChangedMsg{})

	// Should return a command to re-fetch work items
	if cmd == nil {
		t.Fatal("expected a refresh command after state change")
	}
}

func TestRefresh_AfterStateChange_WithMyItems_UpdatesList(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Bug", State: "Active", WorkItemType: "Bug"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Task", State: "New", WorkItemType: "Task"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Toggle my-items on
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = m.Update(myWorkItemsMsg{workItems: []azdevops.WorkItem{items[0]}})

	// Enter detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Simulate a successful state change
	m, cmd := m.Update(WorkItemStateChangedMsg{})

	// Should return a refresh command even with my-items filter active
	if cmd == nil {
		t.Fatal("expected a refresh command after state change with my-items active")
	}
}

func TestMyItems_FetchError_ClearsLoading(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Toggle my-items on, then press 'r' to refresh
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = m.Update(myWorkItemsMsg{workItems: items})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// All-items fetch returns, chains to my-items fetch
	m, _ = m.Update(workItemsMsg{workItems: items})

	// My-items fetch errors
	m, _ = m.Update(myWorkItemsMsg{err: tea.ErrInterrupted})

	// Should fall back to all items and NOT be stuck loading
	if m.myItemsOnly {
		t.Error("myItemsOnly should be false after fetch error")
	}
	view := m.View()
	if strings.Contains(view, "Loading work items") {
		t.Error("view should not be stuck on loading after my-items fetch error")
	}
}

func TestStatePickerEscClosesPickerNotDetailView(t *testing.T) {
	m := NewModel(nil)

	// Set up work items
	m.list = m.list.SetItems([]azdevops.WorkItem{
		{
			ID: 123,
			Fields: azdevops.WorkItemFields{
				Title:        "Test WI",
				State:        "Active",
				WorkItemType: "Bug",
			},
		},
	})

	// Enter detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.GetViewMode() != ViewDetail {
		t.Fatalf("Expected ViewDetail, got %d", m.GetViewMode())
	}

	// Simulate states loaded (which opens the state picker)
	if adapter, ok := m.list.Detail().(*detailAdapter); ok {
		adapter.model, _ = adapter.model.Update(statesLoadedMsg{
			states: []azdevops.WorkItemTypeState{
				{Name: "New", Color: "b2b2b2", Category: "Proposed"},
				{Name: "Active", Color: "007acc", Category: "InProgress"},
				{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
			},
		})
		if !adapter.model.statePicker.IsVisible() {
			t.Fatal("State picker should be visible after states loaded")
		}
	} else {
		t.Fatal("Expected detailAdapter")
	}

	// Press Esc to close state picker (not the detail view)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Should still be in detail view (Esc closed the picker, not the view)
	if m.GetViewMode() != ViewDetail {
		t.Error("Esc should close state picker, not exit detail view")
	}

	if adapter, ok := m.list.Detail().(*detailAdapter); ok {
		if adapter.model.statePicker.IsVisible() {
			t.Error("State picker should be hidden after Esc")
		}
	}

	// Now pressing Esc again should exit detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.GetViewMode() != ViewList {
		t.Error("Second Esc should exit detail view back to list")
	}
}

func TestHasContextBar_DetailView(t *testing.T) {
	m := NewModel(nil)

	// In list mode, no context bar
	if m.HasContextBar() {
		t.Error("Expected no context bar in list mode")
	}

	// Set up items and enter detail
	m.list = m.list.SetItems([]azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Test", WorkItemType: "Bug"}},
	})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// In detail mode, context bar should be shown
	if !m.HasContextBar() {
		t.Error("Expected context bar in detail mode")
	}
}

// --- Tag Filter Tests ---

func TestTagFilter_ApplyAndClear(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item 1", Tags: "Sprint 1; Backend"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Item 2", Tags: "Sprint 1; Frontend"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Title: "Item 3", Tags: "Sprint 2; Backend"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	if m.IsTagFilterActive() {
		t.Error("tag filter should not be active initially")
	}

	// Apply tag filter for "Backend"
	m, _ = m.Update(TagSelectedMsg{Tag: "Backend"})

	if !m.IsTagFilterActive() {
		t.Error("tag filter should be active after selecting a tag")
	}
	if m.ActiveTag() != "Backend" {
		t.Errorf("expected active tag 'Backend', got %q", m.ActiveTag())
	}
	if len(m.list.Items()) != 2 {
		t.Errorf("expected 2 items with 'Backend' tag, got %d", len(m.list.Items()))
	}

	// Clear tag filter
	m, _ = m.Update(TagSelectedMsg{Tag: ""})

	if m.IsTagFilterActive() {
		t.Error("tag filter should not be active after clearing")
	}
	if len(m.list.Items()) != 3 {
		t.Errorf("expected 3 items after clearing tag filter, got %d", len(m.list.Items()))
	}
}

func TestTagFilter_ComposesWithMyItems(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "My Backend", Tags: "Backend"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "My Frontend", Tags: "Frontend"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Title: "Other Backend", Tags: "Backend"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Toggle my items on
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Simulate @Me results (items 1 and 2 are mine)
	myItems := []azdevops.WorkItem{items[0], items[1]}
	m, _ = m.Update(myWorkItemsMsg{workItems: myItems})

	if len(m.list.Items()) != 2 {
		t.Fatalf("expected 2 my items, got %d", len(m.list.Items()))
	}

	// Apply tag filter for "Backend" — should intersect with my items
	m, _ = m.Update(TagSelectedMsg{Tag: "Backend"})

	if len(m.list.Items()) != 1 {
		t.Errorf("expected 1 item (my + backend), got %d", len(m.list.Items()))
	}
	if m.list.Items()[0].ID != 1 {
		t.Errorf("expected item ID 1, got %d", m.list.Items()[0].ID)
	}
}

func TestTagFilter_PollingRespectsActiveFilter(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item 1", Tags: "Sprint 1"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Item 2", Tags: "Sprint 2"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Apply tag filter
	m, _ = m.Update(TagSelectedMsg{Tag: "Sprint 1"})

	if len(m.list.Items()) != 1 {
		t.Fatalf("expected 1 item with Sprint 1, got %d", len(m.list.Items()))
	}

	// Polling arrives with new items
	newItems := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item 1", Tags: "Sprint 1"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Title: "Item 2", Tags: "Sprint 2"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Title: "Item 3", Tags: "Sprint 1"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: newItems})

	// allItems updated but visible list should apply tag filter
	if len(m.allItems) != 3 {
		t.Errorf("expected 3 allItems, got %d", len(m.allItems))
	}
	if len(m.list.Items()) != 2 {
		t.Errorf("expected 2 visible items (Sprint 1 filtered), got %d", len(m.list.Items()))
	}
}

func TestTagFilter_IgnoredDuringSearch(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item", Tags: "Sprint 1"}},
	}
	m, _ = m.Update(SetWorkItemsMsg{WorkItems: items})

	// Enter search mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if !m.IsSearching() {
		t.Fatal("should be in search mode")
	}

	// Try to press T - should be ignored during search
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})

	if m.IsTagFilterActive() {
		t.Error("T key should be ignored during search mode")
	}
}

func TestTagFilter_IgnoredInDetailView(t *testing.T) {
	m := NewModel(nil)
	m.list, _ = m.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Title: "Item", Tags: "Sprint 1", WorkItemType: "Task"}},
	}
	m.list = m.list.SetItems(items)

	// Enter detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetViewMode() != ViewDetail {
		t.Fatal("should be in detail view")
	}

	// Try to press T - should be ignored in detail view
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})

	if m.IsTagFilterActive() {
		t.Error("T key should be ignored in detail view")
	}
}

func TestCollectUniqueTags(t *testing.T) {
	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Tags: "Sprint 1; Backend"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Tags: "Sprint 1; Frontend"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Tags: "Sprint 2; Backend"}},
		{ID: 4, Fields: azdevops.WorkItemFields{Tags: ""}},
	}

	tags := collectUniqueTags(items)

	// Should have 4 unique tags: Sprint 1, Backend, Frontend, Sprint 2
	if len(tags) != 4 {
		t.Errorf("expected 4 unique tags, got %d: %v", len(tags), tags)
	}

	// Check all expected tags are present
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	for _, expected := range []string{"Sprint 1", "Backend", "Frontend", "Sprint 2"} {
		if !tagSet[expected] {
			t.Errorf("expected tag %q to be present", expected)
		}
	}
}

func TestCollectUniqueTags_Sorted(t *testing.T) {
	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Tags: "Zebra; Alpha; Middle"}},
	}

	tags := collectUniqueTags(items)

	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
	if tags[0] != "Alpha" || tags[1] != "Middle" || tags[2] != "Zebra" {
		t.Errorf("expected sorted tags [Alpha Middle Zebra], got %v", tags)
	}
}

func TestApplyTagFilter(t *testing.T) {
	items := []azdevops.WorkItem{
		{ID: 1, Fields: azdevops.WorkItemFields{Tags: "Sprint 1; Backend"}},
		{ID: 2, Fields: azdevops.WorkItemFields{Tags: "Sprint 1; Frontend"}},
		{ID: 3, Fields: azdevops.WorkItemFields{Tags: "Sprint 2; Backend"}},
	}

	filtered := applyTagFilter(items, "Backend")
	if len(filtered) != 2 {
		t.Errorf("expected 2 items with Backend tag, got %d", len(filtered))
	}

	filtered = applyTagFilter(items, "Sprint 1")
	if len(filtered) != 2 {
		t.Errorf("expected 2 items with Sprint 1 tag, got %d", len(filtered))
	}

	filtered = applyTagFilter(items, "Nonexistent")
	if len(filtered) != 0 {
		t.Errorf("expected 0 items with Nonexistent tag, got %d", len(filtered))
	}

	// Empty tag = no filter
	filtered = applyTagFilter(items, "")
	if len(filtered) != 3 {
		t.Errorf("expected 3 items with empty filter, got %d", len(filtered))
	}
}

func TestUpdate_WorkItemsMsg_CriticalErrorNotShownInline(t *testing.T) {
	model := NewModel(nil)
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	criticalErr := fmt.Errorf("all projects failed: [HTTP request failed with status 400]")
	model, cmd := model.Update(workItemsMsg{workItems: nil, err: criticalErr})

	if cmd == nil {
		t.Fatal("Expected a command to be returned for critical error, got nil")
	}
	msg := cmd()
	if _, ok := msg.(components.CriticalErrorMsg); !ok {
		t.Errorf("Expected CriticalErrorMsg, got %T", msg)
	}

	// Critical error should NOT show inline
	view := model.View()
	if strings.Contains(view, "Error loading") {
		t.Error("Critical error should not be displayed inline in the list view")
	}
}
