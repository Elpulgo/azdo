package workitems

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPendingDetailRestore_OpensDetailWhenItemAppears(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(99)

	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: expected ViewList, got %d", model.GetViewMode())
	}

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []azdevops.WorkItem{
		{ID: 12, Fields: azdevops.WorkItemFields{Title: "Other", WorkItemType: "Task"}},
		{ID: 99, Fields: azdevops.WorkItemFields{Title: "Target", WorkItemType: "Task", State: "Active"}},
	}})

	if model.GetViewMode() != ViewDetail {
		t.Errorf("after items arrive, ViewMode = %d, want ViewDetail", model.GetViewMode())
	}
	if got := model.DetailItemID(); got != 99 {
		t.Errorf("DetailItemID = %d, want 99", got)
	}
}

func TestPendingDetailRestore_NoMatchStaysOnList(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(99)

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []azdevops.WorkItem{
		{ID: 12, Fields: azdevops.WorkItemFields{Title: "Only", WorkItemType: "Task"}},
	}})

	if model.GetViewMode() != ViewList {
		t.Errorf("ViewMode = %d, want ViewList (restore should silently no-op)",
			model.GetViewMode())
	}
}

func TestPendingDetailRestore_IsOneShot(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(99)

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []azdevops.WorkItem{
		{ID: 12, Fields: azdevops.WorkItemFields{Title: "A", WorkItemType: "Task"}},
	}})
	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: ViewMode = %d, want ViewList", model.GetViewMode())
	}

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []azdevops.WorkItem{
		{ID: 99, Fields: azdevops.WorkItemFields{Title: "Target", WorkItemType: "Task"}},
	}})
	if model.GetViewMode() != ViewList {
		t.Errorf("second populate triggered restore unexpectedly (ViewMode = %d)",
			model.GetViewMode())
	}
}

// TestDetailItemID_TracksOpenAndClose ensures the persistence-facing accessor
// returns 0 when not in detail and the work item's ID when detail is open.
func TestDetailItemID_TracksOpenAndClose(t *testing.T) {
	model := NewModel(nil)

	if got := model.DetailItemID(); got != 0 {
		t.Errorf("initial DetailItemID = %d, want 0", got)
	}

	model.list = model.list.SetItems([]azdevops.WorkItem{
		{ID: 1337, Fields: azdevops.WorkItemFields{Title: "Test", WorkItemType: "Task", State: "Active"}},
	})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := model.DetailItemID(); got != 1337 {
		t.Errorf("after entering detail, DetailItemID = %d, want 1337", got)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := model.DetailItemID(); got != 0 {
		t.Errorf("after esc, DetailItemID = %d, want 0", got)
	}
}
