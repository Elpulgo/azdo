package pullrequests

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

// TestPendingDetailRestore_OpensDetailWhenItemAppears verifies the
// startup-restore handshake: the app calls WithPendingDetailRestore(id)
// before the list is populated; once items arrive that contain that ID,
// the sub-model transitions into detail on its own.
func TestPendingDetailRestore_OpensDetailWhenItemAppears(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(99)

	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: expected ViewList, got %d", model.GetViewMode())
	}

	model, _ = model.Update(SetPRsMsg{PRs: []azdevops.PullRequest{
		{ID: 12, Title: "Other"},
		{ID: 99, Title: "Target", Status: "active",
			CreatedBy:  azdevops.Identity{DisplayName: "X"},
			Repository: azdevops.Repository{Name: "r"}},
	}})

	if model.GetViewMode() != ViewDetail {
		t.Errorf("after items arrive, ViewMode = %d, want ViewDetail", model.GetViewMode())
	}
	if got := model.DetailItemID(); got != 99 {
		t.Errorf("DetailItemID = %d, want 99", got)
	}
}

// TestPendingDetailRestore_NoMatchStaysOnList ensures restore is a silent
// no-op when the pending ID isn't present (PR deleted, filtered out, etc.).
func TestPendingDetailRestore_NoMatchStaysOnList(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(99)

	model, _ = model.Update(SetPRsMsg{PRs: []azdevops.PullRequest{
		{ID: 12, Title: "Only this one"},
	}})

	if model.GetViewMode() != ViewList {
		t.Errorf("ViewMode = %d, want ViewList (restore should silently no-op)",
			model.GetViewMode())
	}
}

// TestPendingDetailRestore_IsOneShot guards against re-triggering on a
// later populate (e.g. polling refresh): once the pending intent has been
// considered, it must not fire again even if the user has since navigated.
func TestPendingDetailRestore_IsOneShot(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(99)

	// First populate without the target — pending intent should be consumed.
	model, _ = model.Update(SetPRsMsg{PRs: []azdevops.PullRequest{{ID: 12}}})
	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: ViewMode = %d, want ViewList", model.GetViewMode())
	}

	// Second populate now contains the target — but the user already saw
	// the list; we must NOT now hijack them into detail.
	model, _ = model.Update(SetPRsMsg{PRs: []azdevops.PullRequest{{ID: 99, Title: "T"}}})
	if model.GetViewMode() != ViewList {
		t.Errorf("second populate triggered restore unexpectedly (ViewMode = %d)",
			model.GetViewMode())
	}
}

// TestDetailItemID_TracksOpenAndClose ensures the persistence-facing accessor
// returns 0 when not in detail and the PR's ID when detail is open.
func TestDetailItemID_TracksOpenAndClose(t *testing.T) {
	model := NewModel(nil)

	if got := model.DetailItemID(); got != 0 {
		t.Errorf("initial DetailItemID = %d, want 0", got)
	}

	model.list = model.list.SetItems([]azdevops.PullRequest{
		{ID: 42, Title: "Test PR", Status: "active",
			CreatedBy:  azdevops.Identity{DisplayName: "Test"},
			Repository: azdevops.Repository{Name: "repo"}},
	})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := model.DetailItemID(); got != 42 {
		t.Errorf("after entering detail, DetailItemID = %d, want 42", got)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := model.DetailItemID(); got != 0 {
		t.Errorf("after esc, DetailItemID = %d, want 0", got)
	}
}
