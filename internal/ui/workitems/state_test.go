package workitems

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
	tea "github.com/charmbracelet/bubbletea"
)

// azureRef is a convenience for building an Azure work-item identity in tests.
func azureRef(id string) provider.Identity {
	return provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: id}
}

func TestPendingDetailRestore_OpensDetailWhenItemAppears(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: expected ViewList, got %d", model.GetViewMode())
	}

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{Identity: azureRef("12"), Title: "Other", WorkItemType: "Task"},
		{Identity: azureRef("99"), Title: "Target", WorkItemType: "Task", State: "Active"},
	}})

	if model.GetViewMode() != ViewDetail {
		t.Errorf("after items arrive, ViewMode = %d, want ViewDetail", model.GetViewMode())
	}
	ref, ok := model.DetailRef()
	if !ok || ref.ID != "99" {
		t.Errorf("DetailRef = %+v (ok=%v), want ID 99", ref, ok)
	}
}

func TestPendingDetailRestore_NoMatchStaysOnList(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{Identity: azureRef("12"), Title: "Only", WorkItemType: "Task"},
	}})

	if model.GetViewMode() != ViewList {
		t.Errorf("ViewMode = %d, want ViewList (restore should silently no-op)",
			model.GetViewMode())
	}
}

// TestPendingDetailRestore_DoesNotMatchAcrossProviders is the crux of the
// cross-provider identity fix: a pending Azure ref must not open a GitHub
// issue that shares the same numeric ID.
func TestPendingDetailRestore_DoesNotMatchAcrossProviders(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	// Only a GitHub issue #99 is present — different backend, same number.
	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{
			Identity:     provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ID: "99"},
			Title:        "GitHub 99",
			WorkItemType: "Issue",
			State:        "Active",
		},
	}})

	if model.GetViewMode() != ViewList {
		t.Errorf("ViewMode = %d, want ViewList (a same-ID GitHub issue must not satisfy an Azure restore)",
			model.GetViewMode())
	}
}

func TestPendingDetailRestore_IsOneShot(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{Identity: azureRef("12"), Title: "A", WorkItemType: "Task"},
	}})
	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: ViewMode = %d, want ViewList", model.GetViewMode())
	}

	model, _ = model.Update(SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{Identity: azureRef("99"), Title: "Target", WorkItemType: "Task"},
	}})
	if model.GetViewMode() != ViewList {
		t.Errorf("second populate triggered restore unexpectedly (ViewMode = %d)",
			model.GetViewMode())
	}
}

// TestDetailRef_TracksOpenAndClose ensures the persistence-facing accessor
// reports ok=false when not in detail and the work item's identity when open.
func TestDetailRef_TracksOpenAndClose(t *testing.T) {
	model := NewModel(nil)

	if _, ok := model.DetailRef(); ok {
		t.Errorf("initial DetailRef ok = true, want false")
	}

	model.list = model.list.SetItems([]provider.WorkItem{
		{Identity: azureRef("1337"), Title: "Test", WorkItemType: "Task", State: "Active"},
	})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	ref, ok := model.DetailRef()
	if !ok || ref.ID != "1337" || ref.Kind != provider.KindAzure {
		t.Errorf("after entering detail, DetailRef = %+v (ok=%v), want Azure ID 1337", ref, ok)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := model.DetailRef(); ok {
		t.Errorf("after esc, DetailRef ok = true, want false")
	}
}
