package pullrequests

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
	tea "github.com/charmbracelet/bubbletea"
)

// azureRef is a convenience for building an Azure PR identity in tests.
func azureRef(id string) provider.Identity {
	return provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: id}
}

// TestPendingDetailRestore_OpensDetailWhenItemAppears verifies the
// startup-restore handshake: the app calls WithPendingDetailRestore(ref)
// before the list is populated; once items arrive that contain that identity,
// the sub-model transitions into detail on its own.
func TestPendingDetailRestore_OpensDetailWhenItemAppears(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: expected ViewList, got %d", model.GetViewMode())
	}

	model, _ = model.Update(SetPRsMsg{PRs: []provider.PullRequest{
		{Identity: azureRef("12"), Title: "Other"},
		{
			Identity:       azureRef("99"),
			Title:          "Target",
			Status:         "active",
			CreatedByName:  "X",
			RepositoryName: "r",
		},
	}})

	if model.GetViewMode() != ViewDetail {
		t.Errorf("after items arrive, ViewMode = %d, want ViewDetail", model.GetViewMode())
	}
	ref, ok := model.DetailRef()
	if !ok || ref.ID != "99" {
		t.Errorf("DetailRef = %+v (ok=%v), want ID 99", ref, ok)
	}
}

// TestPendingDetailRestore_NoMatchStaysOnList ensures restore is a silent
// no-op when the pending ID isn't present (PR deleted, filtered out, etc.).
func TestPendingDetailRestore_NoMatchStaysOnList(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	model, _ = model.Update(SetPRsMsg{PRs: []provider.PullRequest{
		{Identity: azureRef("12"), Title: "Only this one"},
	}})

	if model.GetViewMode() != ViewList {
		t.Errorf("ViewMode = %d, want ViewList (restore should silently no-op)",
			model.GetViewMode())
	}
}

// TestPendingDetailRestore_DoesNotMatchAcrossProviders is the crux of the
// cross-provider identity fix: a pending GitHub ref must not open an Azure PR
// that happens to share the same numeric ID (and vice versa).
func TestPendingDetailRestore_DoesNotMatchAcrossProviders(t *testing.T) {
	model := NewModel(nil)
	// Pending restore targets GitHub PR #99...
	model = model.WithPendingDetailRestore(provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ID: "99"})

	// ...but only an Azure PR #99 is present. It must NOT match.
	model, _ = model.Update(SetPRsMsg{PRs: []provider.PullRequest{
		{Identity: azureRef("99"), Title: "Azure 99", Status: "active", CreatedByName: "X", RepositoryName: "r"},
	}})

	if model.GetViewMode() != ViewList {
		t.Errorf("ViewMode = %d, want ViewList (a same-ID Azure PR must not satisfy a GitHub restore)",
			model.GetViewMode())
	}
}

// TestPendingDetailRestore_IsOneShot guards against re-triggering on a
// later populate (e.g. polling refresh): once the pending intent has been
// considered, it must not fire again even if the user has since navigated.
func TestPendingDetailRestore_IsOneShot(t *testing.T) {
	model := NewModel(nil)
	model = model.WithPendingDetailRestore(azureRef("99"))

	// First populate without the target — pending intent should be consumed.
	model, _ = model.Update(SetPRsMsg{PRs: []provider.PullRequest{
		{Identity: azureRef("12")},
	}})
	if model.GetViewMode() != ViewList {
		t.Fatalf("precondition: ViewMode = %d, want ViewList", model.GetViewMode())
	}

	// Second populate now contains the target — but the user already saw
	// the list; we must NOT now hijack them into detail.
	model, _ = model.Update(SetPRsMsg{PRs: []provider.PullRequest{
		{Identity: azureRef("99"), Title: "T"},
	}})
	if model.GetViewMode() != ViewList {
		t.Errorf("second populate triggered restore unexpectedly (ViewMode = %d)",
			model.GetViewMode())
	}
}

// TestDetailRef_TracksOpenAndClose ensures the persistence-facing accessor
// reports ok=false when not in detail and the PR's identity when detail is open.
func TestDetailRef_TracksOpenAndClose(t *testing.T) {
	model := NewModel(nil)

	if _, ok := model.DetailRef(); ok {
		t.Errorf("initial DetailRef ok = true, want false")
	}

	model.list = model.list.SetItems([]provider.PullRequest{
		{
			Identity:       azureRef("42"),
			Title:          "Test PR",
			Status:         "active",
			CreatedByName:  "Test",
			RepositoryName: "repo",
		},
	})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	ref, ok := model.DetailRef()
	if !ok || ref.ID != "42" || ref.Kind != provider.KindAzure {
		t.Errorf("after entering detail, DetailRef = %+v (ok=%v), want Azure ID 42", ref, ok)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := model.DetailRef(); ok {
		t.Errorf("after esc, DetailRef ok = true, want false")
	}
}
