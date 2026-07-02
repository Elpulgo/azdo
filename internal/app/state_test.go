package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/state"
	"github.com/Elpulgo/azdo/internal/ui/pullrequests"
	"github.com/Elpulgo/azdo/internal/ui/workitems"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestModelWithStore(t *testing.T) (Model, *state.Store) {
	t.Helper()
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.yaml"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(5 * time.Millisecond)

	m := NewModel(nil, client, cfg, "dev", "")
	m.SetStateStore(store)
	m.width = 100
	m.height = 30
	return m, store
}

func TestModel_CapturesActiveTabOnKey2(t *testing.T) {
	m, store := newTestModelWithStore(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	_ = updated.(Model)

	if got := store.State().ActiveTab; got != state.TabWorkItems {
		t.Errorf("store ActiveTab = %v, want %v", got, state.TabWorkItems)
	}
}

func TestModel_CapturesActiveTabOnKey3(t *testing.T) {
	m, store := newTestModelWithStore(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	_ = updated.(Model)

	if got := store.State().ActiveTab; got != state.TabPipelines {
		t.Errorf("store ActiveTab = %v, want %v", got, state.TabPipelines)
	}
}

func TestModel_CapturesActiveTabOnArrowKeys(t *testing.T) {
	m, store := newTestModelWithStore(t)

	// Default is PullRequests; right arrow → WorkItems
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if got := store.State().ActiveTab; got != state.TabWorkItems {
		t.Errorf("after right arrow, ActiveTab = %v, want %v", got, state.TabWorkItems)
	}

	// Left arrow → back to PullRequests
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	_ = updated.(Model)
	if got := store.State().ActiveTab; got != state.TabPullRequests {
		t.Errorf("after left arrow, ActiveTab = %v, want %v", got, state.TabPullRequests)
	}
}

// TestModel_CapturesPRDetailOnEnterAndEsc walks an end-to-end navigation:
// load PRs → enter detail → assert capture → esc → assert cleared.
func TestModel_CapturesPRDetailOnEnterAndEsc(t *testing.T) {
	m, store := newTestModelWithStore(t)

	// Seed PRs so "enter" has something to open.
	updated, _ := m.Update(pullrequestsSetMsg([]provider.PullRequest{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "77"},
			Title:          "Test PR",
			Status:         "active",
			CreatedByName:  "Test",
			RepositoryName: "repo",
		},
	}))
	m = updated.(Model)

	// Enter → opens detail for PR #77.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if got := store.State().Tabs.PullRequests.LastDetailID; got != 77 {
		t.Errorf("after enter, PR LastDetailID = %d, want 77", got)
	}

	// Esc → leaves detail; the captured ID should be cleared.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = updated.(Model)
	if got := store.State().Tabs.PullRequests.LastDetailID; got != 0 {
		t.Errorf("after esc, PR LastDetailID = %d, want 0", got)
	}
}

func TestModel_CapturesWorkItemDetailOnEnterAndEsc(t *testing.T) {
	m, store := newTestModelWithStore(t)

	// Move to work items tab.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	// Seed work items.
	updated, _ = m.Update(workitems.SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{Identity: provider.Identity{ID: "4242"}, Title: "T", WorkItemType: "Task", State: "Active"},
	}})
	m = updated.(Model)

	// Enter → opens detail for #4242.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if got := store.State().Tabs.WorkItems.LastDetailID; got != 4242 {
		t.Errorf("after enter, WI LastDetailID = %d, want 4242", got)
	}

	// Esc → cleared.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = updated.(Model)
	if got := store.State().Tabs.WorkItems.LastDetailID; got != 0 {
		t.Errorf("after esc, WI LastDetailID = %d, want 0", got)
	}
}

// pullrequestsSetMsg is a small helper so the test reads cleanly without
// importing pullrequests just for a type name.
func pullrequestsSetMsg(prs []provider.PullRequest) tea.Msg {
	return pullrequests.SetPRsMsg{PRs: prs}
}

// TestApplyState_RestoresActiveTab confirms the persisted active tab is
// restored when ApplyState is called at startup.
func TestApplyState_RestoresActiveTab(t *testing.T) {
	m, _ := newTestModelWithStore(t)
	m.ApplyState(state.State{ActiveTab: state.TabWorkItems})

	if m.activeTab != TabWorkItems {
		t.Errorf("activeTab = %v, want TabWorkItems", m.activeTab)
	}
}

// TestApplyState_IgnoresDisabledTab — if the persisted tab is currently
// disabled by config, the app must not jump onto it.
func TestApplyState_IgnoresDisabledTab(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"workitems"},
	}
	var client *azdevops.MultiClient
	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	m.ApplyState(state.State{ActiveTab: state.TabWorkItems})

	if m.activeTab != TabPullRequests {
		t.Errorf("activeTab = %v, want TabPullRequests (work items disabled)", m.activeTab)
	}
}

// TestApplyState_IgnoresUnknownTab guards against on-disk schema drift —
// an unknown TabID must not crash or set an out-of-range tab.
func TestApplyState_IgnoresUnknownTab(t *testing.T) {
	m, _ := newTestModelWithStore(t)
	m.ApplyState(state.State{ActiveTab: state.TabID("future_tab")})

	if m.activeTab != TabPullRequests {
		t.Errorf("activeTab = %v, want TabPullRequests (unknown tab ignored)", m.activeTab)
	}
}

// TestApplyState_RestoresPRDetailOnFirstPopulate is the user-visible win:
// quit while reading PR #7, relaunch, land on PR #7's detail.
func TestApplyState_RestoresPRDetailOnFirstPopulate(t *testing.T) {
	m, _ := newTestModelWithStore(t)
	m.ApplyState(state.State{
		ActiveTab: state.TabPullRequests,
		Tabs:      state.TabsState{PullRequests: state.TabMemory{LastDetailID: 7}},
	})

	updated, _ := m.Update(pullrequests.SetPRsMsg{PRs: []provider.PullRequest{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "7"},
			Title:          "Target",
			Status:         "active",
			CreatedByName:  "X",
			RepositoryName: "r",
		},
	}})
	m = updated.(Model)

	if got := m.pullRequestsView.DetailItemID(); got != 7 {
		t.Errorf("PR DetailItemID after first populate = %d, want 7", got)
	}
}

// TestApplyState_RestoresWIDetailOnFirstPopulate symmetrical case for WI.
func TestApplyState_RestoresWIDetailOnFirstPopulate(t *testing.T) {
	m, _ := newTestModelWithStore(t)
	m.ApplyState(state.State{
		ActiveTab: state.TabWorkItems,
		Tabs:      state.TabsState{WorkItems: state.TabMemory{LastDetailID: 4242}},
	})

	updated, _ := m.Update(workitems.SetWorkItemsMsg{WorkItems: []provider.WorkItem{
		{Identity: provider.Identity{ID: "4242"}, Title: "T", WorkItemType: "Task", State: "Active"},
	}})
	m = updated.(Model)

	if got := m.workItemsView.DetailItemID(); got != 4242 {
		t.Errorf("WI DetailItemID after first populate = %d, want 4242", got)
	}
}

// TestApplyState_MissingItemDoesNotEnterDetail covers the deletion case:
// the persisted ID is gone (PR closed and gc'd, item moved out of view),
// so we should stay on the list with no error.
func TestApplyState_MissingItemDoesNotEnterDetail(t *testing.T) {
	m, _ := newTestModelWithStore(t)
	m.ApplyState(state.State{
		ActiveTab: state.TabPullRequests,
		Tabs:      state.TabsState{PullRequests: state.TabMemory{LastDetailID: 999}},
	})

	updated, _ := m.Update(pullrequests.SetPRsMsg{PRs: []provider.PullRequest{
		{Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "12"}, Title: "Different PR"},
	}})
	m = updated.(Model)

	if got := m.pullRequestsView.DetailItemID(); got != 0 {
		t.Errorf("DetailItemID = %d, want 0 (target PR not present)", got)
	}
}

// TestModel_NoStoreDoesNotPanic confirms the app works without a store
// wired up (the default in tests and any caller that doesn't opt in).
func TestModel_NoStoreDoesNotPanic(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient
	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Should not panic even though no store is attached
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)
	if m.activeTab != TabWorkItems {
		t.Errorf("tab switching should still work without a store, got %d", m.activeTab)
	}
}
