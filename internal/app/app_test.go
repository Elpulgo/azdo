package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/state"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/notifications"
	"github.com/Elpulgo/azdo/internal/ui/pipelines"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/Elpulgo/azdo/internal/ui/workitems"
	"github.com/Elpulgo/azdo/internal/version"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newNotificationCapableProvider returns a provider whose sole backend is a
// GitHub adapter, satisfying Decision 11's capability check
// (hasNotificationCapability → CompositeProvider.HasNotifications) without a
// live token or network access: *github.Adapter implements
// provider.NotificationSource at compile time regardless of whether mc/nc
// are nil (see internal/github/adapter.go's NewAdapterWithNotifications doc
// comment), so nil, nil is sufficient to make the tab presence check true
// for tests that only exercise wiring/layout, never real API calls.
func newNotificationCapableProvider() provider.Provider {
	return provider.NewCompositeProvider(github.NewAdapterWithNotifications(nil, nil))
}

// newNotificationIncapableProvider returns the phase-1 Azure-only shape: a
// *provider.CompositeProvider whose sole backend is an *azdevops.Adapter, which
// implements no notifications surface at all (internal/azdevops contains zero
// occurrences of Notification).
//
// This fixture is capable-*shaped* on purpose (Decision 59). A nil
// provider.Provider is NOT an acceptable stand-in for "incapable": nil fails
// any type assertion, so a gate written as the naive
//
//	_, ok := p.(provider.NotificationSource); return ok
//
// reports false for nil and the tab is correctly hidden — the test passes while
// the gate is wrong. It is wrong because *CompositeProvider satisfies
// NotificationSource unconditionally, so the naive form reports *true* for the
// composite production actually builds (main.go's runTUI always wraps backends
// in one), shipping the notifications tab to Azure-only users. That is the
// exact outcome Decisions 1 and 11 exist to forbid. Only a real composite over
// a real incapable backend makes the correct gate
// (CompositeProvider.HasNotifications, which does the per-backend assertion)
// distinguishable from the naive one.
func newNotificationIncapableProvider() provider.Provider {
	return provider.NewCompositeProvider(azdevops.NewAdapter(nil))
}

func TestFormatVersionInfo(t *testing.T) {
	tests := []struct {
		version string
		commit  string
		want    string
	}{
		{"1.2.3", "abc1234", "1.2.3 (abc1234)"},
		{"dev", "none", "dev"},
		{"0.1.0", "", "0.1.0"},
		{"2.0.0", "deadbeef", "2.0.0 (deadbeef)"},
	}
	for _, tt := range tests {
		got := formatVersionInfo(tt.version, tt.commit)
		if got != tt.want {
			t.Errorf("formatVersionInfo(%q, %q) = %q, want %q", tt.version, tt.commit, got, tt.want)
		}
	}
}

func TestModel_StatusBarShowsOrgProject(t *testing.T) {
	cfg := &config.Config{
		Organization:    "myorg",
		Projects:        []string{"myproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	// Update with window size to initialize status bar width
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "myorg") {
		t.Error("view should contain organization name")
	}
	if !strings.Contains(view, "myproject") {
		t.Error("view should contain project name")
	}
}

func TestModel_HandlesPollingTick_NilClient_NoCmd(t *testing.T) {
	// When mc is nil (GitHub-only), a TickMsg must produce no command —
	// the poller no-ops rather than panicking on the nil Azure backend.
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	// Send a tick message — must not panic, cmd must be nil (no Azure polling).
	_, cmd := m.Update(polling.TickMsg{})
	if cmd != nil {
		t.Error("expected no command after tick message when Azure backend is nil")
	}
}

// firstLeafCmdMsg repeatedly resolves cmd, following only the first element
// of any resulting tea.BatchMsg, and returns the first non-batch message it
// reaches. Both polling.Poller.OnTick and polling.NotificationsPoller.OnTick
// construct tea.Batch(Fetch..., StartPolling()) in that exact order (Fetch
// first), so index 0 always leads to the fetch's own result and never to
// StartPolling's tea.Every-wrapped timer — which blocks for the real
// interval if invoked, exactly like calling it directly would. This helper
// must never be changed to walk any index other than 0.
func firstLeafCmdMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	for {
		if cmd == nil {
			t.Fatal("firstLeafCmdMsg: nil cmd")
		}
		msg := cmd()
		batch, ok := msg.(tea.BatchMsg)
		if !ok || len(batch) == 0 {
			return msg
		}
		cmd = batch[0]
	}
}

// TestModel_Update_NotificationsTickMsg_DrivesNotificationsPollerNotPipelinePoller
// pins Decision 69 at the app.go wiring level: internal/polling's own
// TestNotificationsPoller_StartPolling_EmitsNotificationsTickMsg pins the
// tick's own concrete type, and this pins that app.go's case for that type
// actually calls m.notificationsPoller.OnTick(), not m.poller.OnTick(). Both
// pollers produce a non-nil cmd here — github.NewAdapterWithNotifications(nil,
// nil) satisfies both PipelineClient and provider.NotificationSource
// unconditionally, each failing fast in-process with no network I/O — so this
// cannot pass by either arm trivially no-op'ing; the two are told apart by the
// concrete message type each Fetch half resolves to.
func TestModel_Update_NotificationsTickMsg_DrivesNotificationsPollerNotPipelinePoller(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	_, cmd := m.Update(polling.NotificationsTickMsg{})
	if cmd == nil {
		t.Fatal("want a non-nil cmd from polling.NotificationsTickMsg")
	}

	got := firstLeafCmdMsg(t, cmd)
	if _, ok := got.(polling.NotificationsFetchedMsg); !ok {
		t.Errorf("polling.NotificationsTickMsg resolved to %T, want polling.NotificationsFetchedMsg — app.go's case must call m.notificationsPoller.OnTick(), not m.poller.OnTick()", got)
	}
}

// TestModel_Update_NotificationsFetchedMsg_AppliesFilterAndFeed pins that the
// polling.NotificationsFetchedMsg case actually reaches the notifications
// pane (through notifications.FilterNotifications + HandleFetchResult), the
// same way this pane's own Init()/`r` fetch does. cfg.Notifications.UnreadOnly
// is set so the assertion cannot pass by the case merely handing msg.Items
// straight to the pane unfiltered — a mutation dropping the
// FilterNotifications call would leave the read row on screen.
func TestModel_Update_NotificationsFetchedMsg_AppliesFilterAndFeed(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Notifications:   config.NotificationsConfig{UnreadOnly: true},
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	items := []provider.Notification{
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
			Title:     "From the poller",
			Reason:    provider.NotificationReasonMentioned,
			Read:      false,
			UpdatedAt: time.Now(),
		},
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "2"},
			Title:     "Already read, must be filtered",
			Reason:    provider.NotificationReasonMentioned,
			Read:      true,
			UpdatedAt: time.Now(),
		},
	}
	updated, _ = m.Update(polling.NotificationsFetchedMsg{Items: items})
	m = updated.(Model)

	view := m.notificationsView.View()
	if !strings.Contains(view, "From the poller") {
		t.Errorf("notifications pane view = %q, want the unread poller-pushed row rendered", view)
	}
	if strings.Contains(view, "Already read, must be filtered") {
		t.Errorf("notifications pane view = %q, want the read row dropped by cfg.Notifications.UnreadOnly (FilterNotifications must run)", view)
	}
}

// TestModel_Update_NotificationsFetchedMsg_ErrorDoesNotClearFeed pins the
// error half of the same case: a failure must not wipe rows already on
// screen (HandleFetchResult's own contract), so a transient poll failure
// cannot make a populated inbox look empty.
func TestModel_Update_NotificationsFetchedMsg_ErrorDoesNotClearFeed(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(polling.NotificationsFetchedMsg{
		Items: []provider.Notification{
			{
				Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
				Title:     "Still here",
				Reason:    provider.NotificationReasonMentioned,
				UpdatedAt: time.Now(),
			},
		},
	})
	m = updated.(Model)

	updated, _ = m.Update(polling.NotificationsFetchedMsg{Err: errors.New("poll boom")})
	m = updated.(Model)

	view := m.notificationsView.View()
	if !strings.Contains(view, "Notifications unavailable:") {
		t.Errorf("notifications pane view = %q, want the error state after a failed poll", view)
	}
	if strings.Contains(view, notificationsPaneMarker) {
		t.Errorf("notifications pane view = %q, must not render the empty-inbox state after a failed poll", view)
	}
}

// TestModel_Update_NotificationsFetchedMsg_EmptyInboxClearsTheFeed pins
// Decision 74 as revised: Items == nil && Err == nil means the inbox is
// genuinely EMPTY and the feed must be cleared.
//
// This test replaces one that asserted the exact opposite. A review round
// read polling.NotificationsFetchedMsg's doc comment, which claimed nil/nil
// was a "nothing to update" result from a transparent 304 replay, and had the
// handler guard against it. The comment was wrong: CompositeProvider.List
// accumulates into a nil `var all []Notification` so an empty inbox IS
// nil/nil, while a real 304 replays cached threads and an unsolicited one is
// an error (Decision 28). The guard therefore stranded a cleared inbox on
// screen permanently — dismiss everything in the browser and the pane kept
// showing stale rows until restart. Both the comment and the guard are gone.
func TestModel_Update_NotificationsFetchedMsg_EmptyInboxClearsTheFeed(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(polling.NotificationsFetchedMsg{
		Items: []provider.Notification{
			{
				Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
				Title:     "Row from a populated inbox",
				Reason:    provider.NotificationReasonMentioned,
				UpdatedAt: time.Now(),
			},
		},
	})
	m = updated.(Model)

	if view := m.notificationsView.View(); !strings.Contains(view, "Row from a populated inbox") {
		t.Fatalf("precondition: notifications pane view = %q, want the row present before the empty poll", view)
	}

	updated, _ = m.Update(polling.NotificationsFetchedMsg{Items: nil, Err: nil})
	m = updated.(Model)

	view := m.notificationsView.View()
	if strings.Contains(view, "Row from a populated inbox") {
		t.Errorf("notifications pane view = %q, want the stale row gone: a nil,nil poll result means the inbox is empty, so dismissing everything in the browser must propagate", view)
	}
	if !strings.Contains(view, notificationsPaneMarker) {
		t.Errorf("notifications pane view = %q, want the empty-inbox state after an empty poll result", view)
	}
}

func TestModel_HandlesPipelineRunsUpdated_Success(t *testing.T) {
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

	// Simulate successful data fetch
	runs := []provider.PipelineRun{
		{Identity: provider.Identity{ID: "1"}, BuildNumber: "2024.1", DefinitionName: "Build"},
	}
	msg := polling.PipelineRunsUpdated{Runs: runs, Err: nil}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	// Status bar should show connected icon (● only, no text)
	view := m.View()
	if !strings.Contains(view, "●") {
		t.Error("should show connected icon ● after successful update")
	}
}

func TestModel_HandlesPipelineRunsUpdated_Error(t *testing.T) {
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

	// Simulate error
	msg := polling.PipelineRunsUpdated{Runs: nil, Err: &testError{}}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	// Status bar should show error
	view := m.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Error("should show error state after failed update")
	}
}

type testError struct{}

func (e *testError) Error() string { return "test error" }

func TestModel_Init_StartsPolling(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 30,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	cmd := m.Init()

	// Should return commands for initialization
	if cmd == nil {
		t.Error("Init should return commands")
	}
}

func TestModel_DefaultTab_IsPullRequests(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	if m.activeTab != TabPullRequests {
		t.Errorf("Default tab should be TabPullRequests, got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Key1_IsPullRequests(t *testing.T) {
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

	// Switch away first, then press '1' to go to PR tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("After pressing '1', activeTab should be TabPullRequests, got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Key2_IsWorkItems(t *testing.T) {
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

	// Press '2' to switch to work items tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabWorkItems {
		t.Errorf("After pressing '2', activeTab should be TabWorkItems, got %d", m.activeTab)
	}
}

func TestModel_TabSwitching_Key3_IsPipelines(t *testing.T) {
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

	// Press '3' to switch to pipelines tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("After pressing '3', activeTab should be TabPipelines, got %d", m.activeTab)
	}
}

func TestModel_View_ShowsPullRequests_WhenActiveTab(t *testing.T) {
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

	// PR is now the default tab (key '1'), so it should already be active
	view := m.View()

	// Should show pull requests content (empty list message or similar)
	if !strings.Contains(view, "pull request") && !strings.Contains(view, "No pull requests") {
		t.Error("View should show pull requests content when on PR tab")
	}
}

// scopeStub embeds provider.Provider so only Scopes() needs an implementation;
// the other methods are never called by displayScopes.
type scopeStub struct {
	provider.Provider
	scopes []string
}

func (s scopeStub) Scopes() []string { return s.scopes }

func TestDisplayScopes_UnionWithDisplayNames(t *testing.T) {
	cfg := &config.Config{
		Projects:     []string{"projA"},
		DisplayNames: map[string]string{"projA": "Project A"},
	}
	// A mixed setup: one Azure project (display-name mapped) and one GitHub repo
	// (passed through unchanged). Both must appear.
	p := scopeStub{scopes: []string{"projA", "octo/repo"}}

	got := displayScopes(p, cfg)
	want := []string{"Project A", "octo/repo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("displayScopes() = %v, want %v", got, want)
	}
}

func TestDisplayScopes_NilProviderFallsBackToProjects(t *testing.T) {
	cfg := &config.Config{
		Projects:     []string{"projA", "projB"},
		DisplayNames: map[string]string{"projA": "Project A"},
	}
	got := displayScopes(nil, cfg)
	want := []string{"Project A", "projB"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("displayScopes(nil) = %v, want %v (legacy cfg.Projects behavior)", got, want)
	}
}

func TestModel_HelpModalShowsConfigPath(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 200
	m.height = 60

	// Update with window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open help modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	// Config path should appear in the help modal
	if !strings.Contains(view, "config.yaml") {
		t.Error("help modal should contain config file path")
	}
}

func TestModel_HelpModal_ReflectsTermOverride(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Terms:           map[string]string{"work_items": "Tasks"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 200
	m.height = 60

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open the help modal.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	// The help dialog's Tabs line must honor the term override, not the
	// hard-coded default label.
	if !strings.Contains(view, "Tasks") {
		t.Error("help modal Tabs line should show the overridden term 'Tasks'")
	}
	if strings.Contains(view, "Work Items") {
		t.Error("help modal Tabs line should not show the default 'Work Items' once overridden")
	}
}

func TestModel_View_ShowsWorkItems_WhenActiveTab(t *testing.T) {
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

	// Switch to work items tab (key '2')
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	view := m.View()

	// Should show work items content (empty list message or similar)
	if !strings.Contains(view, "work item") && !strings.Contains(view, "No work items") {
		t.Error("View should show work items content when on Work Items tab")
	}
}

func TestModel_View_HasBorderedTabBar(t *testing.T) {
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

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	// Tab bar should be wrapped in a rounded border (╭ top-left corner)
	if !strings.Contains(view, "╭") {
		t.Error("Tab bar should have rounded border (expected ╭ corner)")
	}
}

func TestModel_View_HasBorderedContent(t *testing.T) {
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

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	// Content should be wrapped in a border — we expect at least 2 rounded borders
	// (one for tabs, one for content area)
	cornerCount := strings.Count(view, "╭")
	if cornerCount < 2 {
		t.Errorf("Expected at least 2 bordered sections (tabs + content), got %d ╭ corners", cornerCount)
	}
}

func TestModel_View_TabBarAppearsBeforeContent(t *testing.T) {
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

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	// The first line should contain the tab bar border corner (logo is inside the tab bar)
	if len(lines) == 0 || !strings.Contains(lines[0], "╭") {
		t.Errorf("First line should contain tab bar border corner ╭, got: %q", lines[0])
	}

	// Total lines should not exceed terminal height
	if len(lines) > 30 {
		t.Errorf("View output has %d lines, should not exceed terminal height 30", len(lines))
	}
}

func TestModel_View_PipelinesWithData_FitsInTerminal(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	// Simulate window size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Switch to pipelines tab (key '3')
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	// Simulate pipeline data arriving (like from polling)
	runs := make([]provider.PipelineRun, 30)
	for i := range runs {
		runs[i] = provider.PipelineRun{
			Identity:       provider.Identity{ID: fmt.Sprintf("%d", i+1)},
			BuildNumber:    fmt.Sprintf("2024.%d", i+1),
			DefinitionName: fmt.Sprintf("Pipeline-%d", i+1),
			RunStatus:      provider.RunStatusSucceeded,
		}
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	t.Logf("Total lines: %d (terminal height: 40)", len(lines))
	// Count lines with actual visible content (not just whitespace/ANSI)
	nonEmptyCount := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmptyCount++
		}
		if i < 8 || i > len(lines)-6 {
			t.Logf("Line %d (bytes=%d): %.120q", i, len(line), line)
		}
	}
	t.Logf("Non-empty lines: %d", nonEmptyCount)

	// Count actual data rows (lines with "Pipeline-")
	dataRows := 0
	for _, line := range lines {
		if strings.Contains(line, "Pipeline-") {
			dataRows++
		}
	}
	t.Logf("Data rows visible: %d (sent %d runs)", dataRows, len(runs))

	if len(lines) > 40 {
		t.Errorf("View has %d lines, exceeds terminal height 40", len(lines))
	}

	// Tab bar border should be on line 0 (logo is inside the tab bar)
	if !strings.Contains(lines[0], "╭") {
		t.Errorf("Line 0 should have tab bar top border, got: %.80q", lines[0])
	}
}

func TestModel_View_ContentFillsBoxWithoutExcessPadding(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	terminalHeight := 40
	m := NewModel(nil, client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: terminalHeight})
	m = updated.(Model)

	// Switch to pipelines tab (key '3') since pipeline data is used in this test
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	// Load enough data to fill the table
	runs := make([]provider.PipelineRun, 50)
	for i := range runs {
		runs[i] = provider.PipelineRun{
			Identity:       provider.Identity{ID: fmt.Sprintf("%d", i+1)},
			BuildNumber:    fmt.Sprintf("2024.%d", i+1),
			DefinitionName: fmt.Sprintf("Pipeline-%d", i+1),
			RunStatus:      provider.RunStatusSucceeded,
		}
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")

	// Find the content box bottom border (╰)
	boxBottomLine := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "╰") {
			// The last ╰ is the status bar bottom; the second-to-last is the content box bottom
			// But we want the content box bottom which comes before the status bar
			// Content box bottom is followed by the status bar (╭)
			if i+1 < len(lines) && strings.Contains(lines[i+1], "╭") {
				boxBottomLine = i
				break
			}
		}
	}
	if boxBottomLine == -1 {
		t.Fatal("Could not find content box bottom border")
	}

	// Count empty lines inside the box (lines that are just border chars with whitespace)
	// Empty padding lines look like: "│                    │"
	emptyPaddingLines := 0
	for i := boxBottomLine - 1; i >= 0; i-- {
		line := lines[i]
		// Strip the border characters and check if content is just whitespace
		if strings.Contains(line, "│") {
			// Extract content between borders
			inner := strings.TrimPrefix(line, "│")
			inner = strings.TrimSuffix(inner, "│")
			inner = strings.TrimSpace(inner)
			if inner == "" {
				emptyPaddingLines++
			} else {
				break
			}
		}
	}

	// Allow at most 1 line of padding (for rounding). More than that indicates
	// the content view height doesn't match the box inner height.
	const maxAllowedPadding = 1
	if emptyPaddingLines > maxAllowedPadding {
		t.Errorf("Content box has %d empty padding lines at the bottom (max allowed: %d). "+
			"This indicates maxFooterRows is too conservative, causing content views to be undersized.",
			emptyPaddingLines, maxAllowedPadding)
	}

	t.Logf("Total lines: %d, box bottom at line: %d, empty padding: %d", len(lines), boxBottomLine, emptyPaddingLines)
}

func TestModel_View_OutputHeightMatchesTerminal(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	terminalHeights := []int{24, 30, 40, 50}
	for _, termHeight := range terminalHeights {
		t.Run(fmt.Sprintf("height_%d", termHeight), func(t *testing.T) {
			m := NewModel(nil, client, cfg, "dev", "")

			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: termHeight})
			m = updated.(Model)

			// Switch to pipelines tab (key '3')
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
			m = updated.(Model)

			// Load data so content fills
			runs := make([]provider.PipelineRun, 50)
			for i := range runs {
				runs[i] = provider.PipelineRun{
					Identity:       provider.Identity{ID: fmt.Sprintf("%d", i+1)},
					BuildNumber:    fmt.Sprintf("2024.%d", i+1),
					DefinitionName: fmt.Sprintf("Pipeline-%d", i+1),
					RunStatus:      provider.RunStatusSucceeded,
				}
			}
			updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
			m = updated.(Model)

			view := m.View()
			lines := strings.Split(view, "\n")

			t.Logf("Terminal height: %d, output lines: %d, footerRows: %d", termHeight, len(lines), m.footerRows)

			if len(lines) != termHeight {
				// Show first and last few lines for debugging
				for i, line := range lines {
					if i < 5 || i > len(lines)-5 {
						t.Logf("Line %d: %.100q", i, line)
					}
				}
				t.Errorf("Output has %d lines, want exactly %d", len(lines), termHeight)
			}
		})
	}
}

// TestModel_View_OutputHeightMatchesTerminal_AfterNotificationsPoll pins
// finding 1 of the task 16 independent review: a background poll that
// changes the unread badge's digit width must not desync m.footerRows from
// measureFooterHeight(). At width 120 — squarely inside the 116-128 band
// where a 7-row badge tips the footer onto a second line — the
// polling.NotificationsFetchedMsg handler used to return early, before ever
// reaching resizeActiveViewIfNeeded(), so View() kept emitting the old
// (shorter) footerRows and overflowed the terminal by one line.
//
// Routed through the real polling.NotificationsFetchedMsg, not by direct
// SetFeed/seedUnreadNotifications assignment — those bypass the handler
// entirely, which is exactly why none of the six TestModel_UnreadBadge_*
// tests (nor TestModel_View_OutputHeightMatchesTerminal itself, which builds
// NewModel(nil, ...) and so has no notifications tab at all) could see this
// bug.
//
// Runs the check twice: once after a poll populates the badge, once more
// after a following poll clears it back to empty (Decision 74's "nil Items
// with nil Err means genuinely empty" shape) — the reviewer's repro showed
// the overflow going both directions.
func TestModel_View_OutputHeightMatchesTerminal_AfterNotificationsPoll(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	// 130, not 120: task 17 added the `o` (open in browser) key to
	// notificationsKeybindings(), which no longer fits alongside `u`/`d`'s
	// mark-read/mark-done text on a single footer line at 120 columns.
	const termWidth = 130
	const termHeight = 40

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: termWidth, Height: termHeight})
	m = updated.(Model)

	if h := lipgloss.Height(m.View()); h != termHeight {
		t.Fatalf("precondition: View() rendered %d rows for a %d-row terminal before any poll, want %d", h, termHeight, termHeight)
	}

	items := make([]provider.Notification, 7)
	for i := range items {
		items[i] = provider.Notification{
			Identity: provider.Identity{
				Kind: provider.KindGitHub, Scope: "o/r", ScopeDisplay: "o/r",
				ID: fmt.Sprintf("%d", i+1),
			},
			Title:     fmt.Sprintf("notification %d", i+1),
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Now(),
		}
	}

	updated, _ = m.Update(polling.NotificationsFetchedMsg{Items: items, Err: nil})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "7 unread") {
		t.Fatalf("precondition: badge should show '7 unread' after the poll; view:\n%s", view)
	}
	if h := lipgloss.Height(view); h != termHeight {
		t.Errorf("after a poll populated the unread badge, View() rendered %d rows for a %d-row terminal (footerRows=%d) — the footer was not re-measured; view:\n%s",
			h, termHeight, m.footerRows, view)
	}

	// A following poll clears the inbox. nil Items with nil Err is the
	// genuinely-empty shape, not "unchanged" (Decision 74).
	updated, _ = m.Update(polling.NotificationsFetchedMsg{Items: nil, Err: nil})
	m = updated.(Model)

	view = m.View()
	if strings.Contains(view, "unread") {
		t.Fatalf("precondition: badge should be gone after the inbox is cleared; view:\n%s", view)
	}
	if h := lipgloss.Height(view); h != termHeight {
		t.Errorf("after a poll cleared the unread badge, View() rendered %d rows for a %d-row terminal (footerRows=%d) — the footer was not re-measured; view:\n%s",
			h, termHeight, m.footerRows, view)
	}
}

// --- Task 17 follow-up / independent review: `o` outcomes must be visible ---
//
// notifications.Model.GetStatusMessage() reported the right text for both of
// `o`'s failure outcomes all along (see list_test.go's TestOpenInBrowser_*
// pane-level tests), but nothing carried that text onto the status bar:
// View()'s "if hasContextBar && statusMessage != ''" guard (app.go) never
// fires for the notifications pane, since HasContextBar() is permanently
// false in phase 1 (there is no detail view to attach a context bar to). A
// test seam that reports a field is not the same as observing the behaviour
// that consumes it (decision 72) — every test below asserts through
// m.View(), never through m.notificationsView.GetStatusMessage() alone.

// withOpenURLForTesting substitutes notifications.SetOpenURLForTesting's seam
// for the duration of the calling test, mirroring list_test.go's own
// withOpenURLSpy so a second call within the same test (e.g. a failure
// followed by a retry that succeeds) still unwinds to the real browser.Open
// afterwards: t.Cleanup runs LIFO, so each call's own restore lands on top of
// the previous call's substituted function, not on the real one, until the
// very last cleanup finally reaches it.
func withOpenURLForTesting(t *testing.T, result error) {
	t.Helper()
	restore := notifications.SetOpenURLForTesting(func(string) error { return result })
	t.Cleanup(restore)
}

// openInBrowserRoutingModel builds a sized app model (same shape as task 14's
// own markRoutingModel) whose notifications pane holds one row with webURL as
// its WebURL, and returns it with that row's title. No marker is needed: `o`
// never calls MarkRead/MarkDone.
func openInBrowserRoutingModel(t *testing.T, width, height int, webURL string) (Model, string) {
	t.Helper()
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	if m.activeTab != TabNotifications {
		t.Fatalf("activeTab = %d, want TabNotifications", m.activeTab)
	}

	const title = "OPEN-ME"
	m.notificationsView = notifications.NewModelWithStyles(styles.DefaultStyles(), nil, cfg).
		SetFeed([]provider.Notification{{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
			Title:     title,
			Reason:    provider.NotificationReasonReviewRequested,
			UpdatedAt: time.Now(),
			WebURL:    webURL,
		}})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model), title
}

// dewrapFooterText collapses box-drawing border characters and whitespace
// runs in a rendered View() so a status-bar message that word-wraps across
// two physical footer lines at a given terminal width — observed for both of
// `o`'s failure messages in the 116-130 column band, the footer behaving
// correctly, not a bug under test — can still be matched as one contiguous
// phrase, without the test having to know or assert exactly where the wrap
// point falls.
func dewrapFooterText(view string) string {
	replacer := strings.NewReplacer(
		"\n", " ", "│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ", "─", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(view)), " ")
}

// openInBrowserRoutingFetchMsg returns the polling.NotificationsFetchedMsg a
// following poll would deliver for the same single row openInBrowserRoutingModel
// seeded, so a "the next fetch clears it" test can drive that fetch through
// the real handler rather than SetFeed/HandleFetchResult directly.
func openInBrowserRoutingFetchMsg(title, webURL string) polling.NotificationsFetchedMsg {
	return polling.NotificationsFetchedMsg{
		Items: []provider.Notification{{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
			Title:     title,
			Reason:    provider.NotificationReasonReviewRequested,
			UpdatedAt: time.Now(),
			WebURL:    webURL,
		}},
		Err: nil,
	}
}

// TestModel_View_ShowsMessage_WhenNotificationHasNoWebURL pins the reviewer
// finding: an empty WebURL (decision 3's "no usable repository URL at all"
// case) must render its message somewhere the user can actually see, not just
// report it via GetStatusMessage().
func TestModel_View_ShowsMessage_WhenNotificationHasNoWebURL(t *testing.T) {
	m, title := openInBrowserRoutingModel(t, 130, 40, "")
	if !strings.Contains(m.View(), title) {
		t.Fatalf("precondition: want the seeded row %q to render; view:\n%s", title, m.View())
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("want a nil cmd for an empty WebURL (nothing to launch)")
	}

	// Matched against dewrapFooterText(view), not view itself: the footer's
	// warning text word-wraps onto its own second line at this width — the
	// footer behaving correctly, not a bug this test is chasing — which
	// would make a plain Contains check against the raw view fail even
	// though the message is plainly rendered in full.
	view := m.View()
	if !strings.Contains(dewrapFooterText(view), "No URL for this notification") {
		t.Errorf("View() after o on a row with no WebURL should contain the failure message; view:\n%s", view)
	}
}

// TestModel_View_ShowsMessage_WhenBrowserLaunchFails pins the reviewer
// finding's other failure outcome: a browser.Open error must render its
// message too, not just report it via GetStatusMessage().
func TestModel_View_ShowsMessage_WhenBrowserLaunchFails(t *testing.T) {
	withOpenURLForTesting(t, fmt.Errorf("no browser configured"))

	m, title := openInBrowserRoutingModel(t, 130, 40, "https://github.com/owner/repo/pull/1")
	if !strings.Contains(m.View(), title) {
		t.Fatalf("precondition: want the seeded row %q to render; view:\n%s", title, m.View())
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("want a non-nil cmd for a non-empty WebURL")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	// Matched against dewrapFooterText(view), not view itself: at 130 columns
	// the footer's warning text word-wraps onto its own second line between
	// "browser:" and "no" — the footer behaving correctly, not a bug this
	// test is chasing — which would make a plain Contains check against the
	// raw view fail even though the message is plainly rendered in full.
	view := m.View()
	if !strings.Contains(dewrapFooterText(view), "Failed to open browser: no browser configured") {
		t.Errorf("View() after a failed o should contain the failure message; view:\n%s", view)
	}
}

// TestModel_View_SuccessfulOpen_IsSilentAndClearsAPriorFailureMessage pins
// both halves of the reviewer's "decide and pin what happens on success"
// requirement: a bare success renders no stray text of its own (the browser
// window appearing is the feedback), and — since success is also one of the
// two clearing triggers — a successful retry after a prior failure makes that
// failure's message disappear from the render, not just from
// GetStatusMessage().
func TestModel_View_SuccessfulOpen_IsSilentAndClearsAPriorFailureMessage(t *testing.T) {
	withOpenURLForTesting(t, fmt.Errorf("no browser configured"))

	m, _ := openInBrowserRoutingModel(t, 130, 40, "https://github.com/owner/repo/pull/1")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if !strings.Contains(dewrapFooterText(m.View()), "Failed to open browser: no browser configured") {
		t.Fatalf("precondition: want the failure message rendered; view:\n%s", m.View())
	}

	withOpenURLForTesting(t, nil)

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("want a non-nil cmd for a retry")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	view := dewrapFooterText(m.View())
	if strings.Contains(view, "Failed to open browser") {
		t.Errorf("View() after a successful retry should no longer show the prior failure message; view:\n%s", m.View())
	}
	if strings.Contains(view, "Opened in browser") {
		t.Errorf("a successful o must stay silent, not render its own message; view:\n%s", m.View())
	}
}

// TestModel_View_NextFetchClearsAFailureMessage pins the clearing rule's other
// trigger, the one the reviewer named explicitly: "a poll refreshes the feed"
// while a stale `o` failure message is still showing must not leave it behind.
func TestModel_View_NextFetchClearsAFailureMessage(t *testing.T) {
	withOpenURLForTesting(t, fmt.Errorf("no browser configured"))

	const webURL = "https://github.com/owner/repo/pull/1"
	m, title := openInBrowserRoutingModel(t, 130, 40, webURL)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if !strings.Contains(dewrapFooterText(m.View()), "Failed to open browser: no browser configured") {
		t.Fatalf("precondition: want the failure message rendered; view:\n%s", m.View())
	}

	updated, _ = m.Update(openInBrowserRoutingFetchMsg(title, webURL))
	m = updated.(Model)

	view := m.View()
	if strings.Contains(dewrapFooterText(view), "Failed to open browser") {
		t.Errorf("View() after a fetch should not still show a stale o failure message; view:\n%s", view)
	}
}

// TestModel_View_UnrelatedWarningMessageSurvivesClearing pins the "clear
// direction" of the shared-warningMessage-field hazard, reported by an
// independent reviewer: syncNotificationsActionMessage's edge-triggering only
// ever guarded the *set* direction (stopping it from re-asserting its own
// stale message over someone else's); nothing stopped its clear branch from
// retracting someone else's message outright the instant its own message went
// back to "". Reproduces the reviewer's exact sequence: an `o` failure sets
// the pane's own warning, an unrelated writer (standing in for
// polling.PipelineRunsUpdated's partial-project-load warning, which shares the
// same StatusBar.warningMessage field) sets its own message on top, and the
// following fetch — which clears the pane's own statusMessage back to "" —
// must leave the unrelated message alone rather than clobbering it.
func TestModel_View_UnrelatedWarningMessageSurvivesClearing(t *testing.T) {
	m, title := openInBrowserRoutingModel(t, 130, 40, "")

	// 1. `o` on a URL-less row sets the pane's own warning.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("want a nil cmd for an empty WebURL")
	}
	if !strings.Contains(dewrapFooterText(m.View()), "No URL for this notification") {
		t.Fatalf("precondition: want the o failure message rendered; view:\n%s", m.View())
	}

	// 2. An unrelated writer sets its own message on the very same shared
	// field, standing in for polling.PipelineRunsUpdated's own
	// SetWarningMessage call in production.
	const pipelineWarning = "PIPELINEWARN partial project load"
	m.statusBar.SetWarningMessage(pipelineWarning)
	if !strings.Contains(dewrapFooterText(m.View()), pipelineWarning) {
		t.Fatalf("precondition: want the unrelated warning rendered; view:\n%s", m.View())
	}

	// 3. The next fetch clears the pane's own statusMessage back to "".
	updated, _ = m.Update(openInBrowserRoutingFetchMsg(title, ""))
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(dewrapFooterText(view), pipelineWarning) {
		t.Errorf("the unrelated warning was clobbered by the notifications pane's own clearing rule; view:\n%s", view)
	}
}

// TestModel_View_OutputHeightMatchesTerminal_AfterOpenInBrowserFailure pins
// decision 77's footer-remeasurement obligation for this reviewer finding's
// fix: syncNotificationsActionMessage widens the status bar's warning message
// exactly like any other status-bar field, so a handler that carries it must
// still leave m.footerRows in sync with what View() actually renders.
//
// Sweeps the width band task 16 and its own follow-ups care about (100, 110,
// 116, 120, 128, 130), plus 150: at every width in the mandated band the
// footer already wraps to the same number of physical lines with or without
// this particular message (its own keybindings text plus badge/org/connection
// segments are long enough on their own to force a wrap there regardless), so
// while those six widths remain valuable general footer-accounting coverage,
// none of them can actually distinguish "resized" from "not resized" for
// *this* message — clearing it does not change the line count they render
// at. 150 columns is where it does: verified directly against
// resizeActiveViewIfNeeded's removal that the footer is 4 lines with the
// message showing and 3 once it clears, only correct if the clearing handler
// still resizes.
//
// Two pre-existing gaps (independent review, reproduced at commit a7de08e)
// are NOT this test's concern and are deliberately not asserted around: (a)
// the very first View() after a WindowSizeMsg over-renders by one row at
// 100/110 columns — worked around by taking that first render here (the
// pointer-mutation priming View() itself needs — it mutates m.statusBar
// through its pointer despite a value receiver) without asserting its
// height, and only asserting height on later renders; (b) a ThemeSelectedMsg
// is one row over at every width — never sent by this test.
func TestModel_View_OutputHeightMatchesTerminal_AfterOpenInBrowserFailure(t *testing.T) {
	const termHeight = 40
	const webURL = "https://github.com/owner/repo/pull/1"
	widths := []int{100, 110, 116, 120, 128, 130, 150}

	for _, width := range widths {
		width := width
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			withOpenURLForTesting(t, fmt.Errorf("no browser configured"))

			m, title := openInBrowserRoutingModel(t, width, termHeight, webURL)

			// Prime the pointer-mutation render; deliberately not asserted on
			// (see doc comment above).
			_ = m.View()
			if !strings.Contains(m.View(), title) {
				t.Fatalf("precondition: want the seeded row %q to render; view:\n%s", title, m.View())
			}

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
			m = updated.(Model)
			if cmd == nil {
				t.Fatal("want a non-nil cmd for a non-empty WebURL")
			}
			updated, _ = m.Update(cmd())
			m = updated.(Model)

			// Matched against dewrapFooterText(view), not view itself: at
			// 116/120 columns the footer's warning text wraps onto its own
			// second line mid-message, which would otherwise make an
			// exact-string Contains check against the raw view fail even
			// though the message is plainly rendered — that wrap is the
			// footer behaving correctly, not a bug this test is chasing.
			view := m.View()
			if !strings.Contains(dewrapFooterText(view), "Failed to open browser: no browser configured") {
				t.Fatalf("precondition: want the failure message rendered before measuring height; view:\n%s", view)
			}
			if h := strings.Count(view, "\n") + 1; h != termHeight {
				t.Errorf("width=%d: View() rendered %d rows for a %d-row terminal after the o failure message appeared (footerRows=%d); view:\n%s",
					width, h, termHeight, m.footerRows, view)
			}

			// Clear via the next fetch (HandleFetchResult's unconditional
			// reset, mirrored onto the status bar by
			// syncNotificationsActionMessage before polling.NotificationsFetchedMsg's
			// own resizeActiveViewIfNeeded call).
			updated, _ = m.Update(openInBrowserRoutingFetchMsg(title, webURL))
			m = updated.(Model)

			view = m.View()
			if strings.Contains(dewrapFooterText(view), "Failed to open browser") {
				t.Fatalf("precondition: want the failure message cleared after a fetch; view:\n%s", view)
			}
			if h := strings.Count(view, "\n") + 1; h != termHeight {
				t.Errorf("width=%d: View() rendered %d rows for a %d-row terminal after the o failure message cleared (footerRows=%d); view:\n%s",
					width, h, termHeight, m.footerRows, view)
			}
		})
	}
}

func TestModel_GlobalShortcutsDisabledDuringSearch(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Switch to pipelines tab (key '3') so we can test search there
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	// Load some pipeline data
	runs := []provider.PipelineRun{
		{Identity: provider.Identity{ID: "1"}, BuildNumber: "2024.1", DefinitionName: "Build"},
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	// Press 'f' to enter search mode
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)

	// Verify we're searching
	if !m.isActiveViewSearching() {
		t.Fatal("Expected active view to be searching after pressing 'f'")
	}

	// Press 't' — should NOT open theme picker (should go to search input)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)

	if m.themePicker.IsVisible() {
		t.Error("Pressing 't' during search should NOT open theme picker")
	}

	// Press '2' — should NOT switch tabs
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Error("Pressing '2' during search should NOT switch to Work Items tab")
	}

	// Press esc to exit search
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.isActiveViewSearching() {
		t.Error("Expected search to be exited after esc")
	}

	// Now '2' should work again — switches to Work Items
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabWorkItems {
		t.Error("After exiting search, '2' should switch to Work Items tab")
	}
}

func TestModel_MyItemsToggle_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	// Set up window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Switch to work items tab (key '2')
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	// Simulate work items arriving
	items := []provider.WorkItem{
		{Identity: provider.Identity{ID: "1"}, Title: "My task", WorkItemType: "Task", State: "Active"},
		{Identity: provider.Identity{ID: "2"}, Title: "Other task", WorkItemType: "Task", State: "Active"},
	}
	updated, _ = m.Update(workitems.SetWorkItemsMsg{WorkItems: items})
	m = updated.(Model)

	// Press 'm' to toggle my items filter — fires @Me fetch
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	// Verify filter is active
	if !m.workItemsView.IsMyItemsActive() {
		t.Error("expected my items filter to be active after pressing 'm'")
	}

	// Verify status bar shows "My Items" badge
	view := m.View()
	if !strings.Contains(view, "My Items") {
		t.Error("status bar should show 'My Items' badge when filter is active")
	}

	// Press 'm' again to toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	if m.workItemsView.IsMyItemsActive() {
		t.Error("expected my items filter to be deactivated after second 'm' press")
	}

	// Verify "My Items" badge is removed
	view = m.View()
	if strings.Contains(view, "My Items") {
		t.Error("status bar should NOT show 'My Items' badge when filter is inactive")
	}
}

func TestModel_PRTab_StatusBarShowsMyItemsKeybinding(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// On PR tab (default), status bar should advertise 'm' for my PRs and 'f' for search
	view := m.View()
	if !strings.Contains(view, "my PRs") {
		t.Error("PR tab status bar should show 'm my PRs' keybinding")
	}
	if !strings.Contains(view, "search") {
		t.Error("PR tab status bar should show 'f search' keybinding")
	}
}

func TestModel_MyPRsToggle_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// We're on PR tab by default — press 'm' to toggle
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	if !m.pullRequestsView.IsMyPRsActive() {
		t.Error("expected my PRs filter to be active after pressing 'm'")
	}

	view := m.View()
	if !strings.Contains(view, "My PRs") {
		t.Error("status bar should show 'My PRs' badge when filter is active")
	}

	// Toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)

	if m.pullRequestsView.IsMyPRsActive() {
		t.Error("expected my PRs filter to be off after second 'm' press")
	}

	view = m.View()
	if strings.Contains(view, "My PRs") {
		t.Error("status bar should NOT show 'My PRs' badge when filter is inactive")
	}
}

func TestModel_AsReviewerToggle_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// On PR tab, press 'A' to toggle as-reviewer
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = updated.(Model)

	if !m.pullRequestsView.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be active after pressing 'A'")
	}

	view := m.View()
	if !strings.Contains(view, "Reviewer") {
		t.Error("status bar should show 'As Reviewer' badge")
	}

	// Toggle off
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = updated.(Model)

	if m.pullRequestsView.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be off after second 'A'")
	}
	view = m.View()
	if strings.Contains(view, "Reviewer") {
		t.Error("status bar should NOT show 'As Reviewer' badge when off")
	}
}

func TestModel_PRTab_StatusBarShowsAsReviewerKeybinding(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "as reviewer") {
		t.Error("PR tab status bar should show 'A as reviewer' keybinding")
	}
}

func TestModel_View_ShowsLogo(t *testing.T) {
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

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()

	// The logo should appear in the view (contains box-drawing chars from ASCII art)
	if !strings.Contains(view, "╔═╗") {
		t.Error("View should contain the ASCII art logo")
	}
}

func TestModel_TabBar_Shows_Three_Tabs(t *testing.T) {
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

	view := m.View()

	// Should show all three tabs with new ordering: 1=PR, 2=Work Items, 3=Pipelines
	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("Tab bar should show '1: Pull Requests'")
	}
	if !strings.Contains(view, "2: Work Items") {
		t.Error("Tab bar should show '2: Work Items'")
	}
	if !strings.Contains(view, "3: Pipelines") {
		t.Error("Tab bar should show '3: Pipelines'")
	}
}

func TestModel_UpdateCheckMsg_ShowsNotification(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "1.0.0", "")
	m.width = 120
	m.height = 30

	// Simulate receiving an update check result
	msg := updateCheckMsg{
		info: &version.UpdateInfo{
			CurrentVersion:  "1.0.0",
			LatestVersion:   "v2.0.0",
			UpdateAvailable: true,
			ReleaseURL:      "https://github.com/Elpulgo/azdo/releases/tag/v2.0.0",
		},
	}

	updated, _ := m.Update(msg)
	updatedModel := updated.(Model)

	view := updatedModel.View()
	if !strings.Contains(view, "Update available") {
		t.Error("expected update notification in view")
	}
}

func TestModel_CriticalErrorMsg_ShowsErrorModal(t *testing.T) {
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

	// Send a CriticalErrorMsg
	msg := components.CriticalErrorMsg{
		Title:   "Configuration Error",
		Message: "The API returned 'not found'.",
		Hint:    "Check your config file.",
	}

	updated, _ := m.Update(msg)
	m = updated.(Model)

	if !m.errorModal.IsVisible() {
		t.Error("error modal should be visible after CriticalErrorMsg")
	}

	// View should render the error modal overlay
	view := m.View()
	if !strings.Contains(view, "Configuration Error") {
		t.Error("view should show error modal with title")
	}
}

func TestModel_ThemeSwitch_PreservesConnectionState(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	// Simulate successful data fetch to set status to "connected"
	runs := []provider.PipelineRun{
		{Identity: provider.Identity{ID: "1"}, BuildNumber: "2024.1", DefinitionName: "Build"},
	}
	updated, _ = m.Update(polling.PipelineRunsUpdated{Runs: runs, Err: nil})
	m = updated.(Model)

	// Verify we're connected
	if m.statusBar.GetState() != polling.StateConnected {
		t.Fatal("Expected connected state before theme switch")
	}

	// Switch theme
	updated, _ = m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	// Status bar should still show connected, not connecting
	if m.statusBar.GetState() != polling.StateConnected {
		t.Errorf("Expected state to remain 'connected' after theme switch, got %q", m.statusBar.GetState())
	}

	view := m.View()
	if strings.Contains(view, "connecting") {
		t.Error("Status bar should not show 'connecting' after theme switch when already connected")
	}
	// Connected state shows icon only (●), not the word "connected"
	if !strings.Contains(view, "●") {
		t.Error("Status bar should show connected icon ● after theme switch")
	}
}

// collectMsgTypes runs a command tree (descending into tea.Batch) and returns
// the reflect type name of every leaf message it emits. Each leaf is run under
// a recover guard so a command that panics on a nil client (offline test) is
// skipped rather than failing the collection.
func collectMsgTypes(cmd tea.Cmd) []string {
	var out []string
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		var msg tea.Msg
		func() {
			defer func() { _ = recover() }()
			msg = c()
		}()
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, cc := range batch {
				run(cc)
			}
			return
		}
		out = append(out, reflect.TypeOf(msg).String())
	}
	run(cmd)
	return out
}

// TestModel_ThemeSwitch_DoesNotRefetchMetrics guards the theme-change bug: when
// the metrics tab is active, switching theme must re-style the metrics view in
// place (SetStyles) WITHOUT re-initializing it. A re-init re-runs the async
// fetch/snapshot load, whose completion message blanks the loaded trends data
// — the symptom the user reported ("changed theme and the config was blown
// away again"). We detect the regression by asserting the theme handler emits
// no metrics.* command for the active metrics tab.
func TestModel_ThemeSwitch_DoesNotRefetchMetrics(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	// Pretend the user is on the metrics tab when they change theme.
	m.activeTab = TabMetrics

	updated, cmd := m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	for _, ty := range collectMsgTypes(cmd) {
		if strings.HasPrefix(ty, "metrics.") {
			t.Errorf("theme switch re-fetched metrics (emitted %s); the view should be"+
				" re-styled in place via SetStyles, not re-initialized", ty)
		}
	}
}

func TestModel_ThemeSwitch_PreservesWarningMessage(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	// Set a warning message
	m.statusBar.SetWarningMessage("Some projects failed")

	// Switch theme
	updated, _ = m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	// Warning message should be preserved
	view := m.View()
	if !strings.Contains(view, "Some projects failed") {
		t.Error("Warning message should be preserved after theme switch")
	}
}

func TestModel_HelpModalShowsVersionInfo(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "1.5.0", "abc1234")
	m.width = 200
	m.height = 60

	// Update with window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open help modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "1.5.0") {
		t.Error("help modal should contain version number")
	}
	if !strings.Contains(view, "abc1234") {
		t.Error("help modal should contain commit hash")
	}
}

func TestModel_HelpModalShowsDevVersion(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "none")
	m.width = 200
	m.height = 60

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open help modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "dev") {
		t.Error("help modal should show 'dev' version")
	}
}

func TestModel_UpdateCheckMsg_NoUpdateAvailable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "2.0.0", "")
	m.width = 120
	m.height = 30

	msg := updateCheckMsg{
		info: &version.UpdateInfo{
			CurrentVersion:  "2.0.0",
			LatestVersion:   "v2.0.0",
			UpdateAvailable: false,
		},
	}

	updated, _ := m.Update(msg)
	updatedModel := updated.(Model)

	view := updatedModel.View()
	if strings.Contains(view, "Update available") {
		t.Error("should not show update notification when already on latest")
	}
}

func TestModel_DisabledPanes_PipelinesDisabled_TabBarHidesPipelines(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pipelines"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("Tab bar should show '1: Pull Requests'")
	}
	if !strings.Contains(view, "2: Work Items") {
		t.Error("Tab bar should show '2: Work Items'")
	}
	if strings.Contains(view, "Pipelines") {
		t.Error("Tab bar should NOT show Pipelines when disabled")
	}
}

func TestModel_DisabledPanes_WorkItemsDisabled_TabBarHidesWorkItems(t *testing.T) {
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

	view := m.View()

	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("Tab bar should show '1: Pull Requests'")
	}
	if strings.Contains(view, "Work Items") {
		t.Error("Tab bar should NOT show Work Items when disabled")
	}
	if !strings.Contains(view, "2: Pipelines") {
		t.Error("Tab bar should show '2: Pipelines' (renumbered)")
	}
}

func TestModel_DisabledPanes_Key2_GoesToPipelines_WhenWorkItemsDisabled(t *testing.T) {
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

	// Press '2' — should go to pipelines (since it's the 2nd enabled tab)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("After pressing '2' with workitems disabled, expected TabPipelines, got %d", m.activeTab)
	}
}

func TestModel_DisabledPanes_Key3_Noop_WhenOnlyTwoTabs(t *testing.T) {
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

	// Press '3' — should be a no-op (only 2 tabs)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("Pressing '3' with only 2 tabs should be no-op, got tab %d", m.activeTab)
	}
}

func TestModel_DisabledPanes_ArrowKeys_SkipDisabledTabs(t *testing.T) {
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

	// Right arrow from PR → should go to Pipelines (skipping WorkItems)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("Right arrow should skip disabled WorkItems tab, got %d", m.activeTab)
	}

	// Right arrow from Pipelines → should wrap to PR
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	if m.activeTab != TabPullRequests {
		t.Errorf("Right arrow should wrap to PullRequests, got %d", m.activeTab)
	}

	// Left arrow from PR → should wrap to Pipelines
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(Model)

	if m.activeTab != TabPipelines {
		t.Errorf("Left arrow should wrap to Pipelines, got %d", m.activeTab)
	}
}

func TestModel_TabBar_DefaultLabels_Unchanged(t *testing.T) {
	// With no Terms configured the tab bar must show the built-in default labels
	// byte-for-byte — "Pull Requests", "Work Items", "Pipelines".
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		// Terms is nil — no overrides
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	for _, want := range []string{"1: Pull Requests", "2: Work Items", "3: Pipelines"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected tab bar to contain %q with default Terms (nil)", want)
		}
	}
}

func TestModel_TabBar_TermOverride_ReplacesLabel(t *testing.T) {
	// When Terms["work_items"] is set the tab bar must show the override instead
	// of the default "Work Items", while the other tabs keep their defaults.
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Terms:           map[string]string{"work_items": "Tasks"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	// Overridden label must appear with its positional number
	if !strings.Contains(view, "2: Tasks") {
		t.Error("expected tab bar to contain '2: Tasks' when Terms[\"work_items\"] = \"Tasks\"")
	}
	// Default label must be gone for that slot
	if strings.Contains(view, "Work Items") {
		t.Error("tab bar must NOT contain 'Work Items' when it is overridden to 'Tasks'")
	}
	// Other tabs must keep their default labels
	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("expected tab bar to contain '1: Pull Requests' (not overridden)")
	}
	if !strings.Contains(view, "3: Pipelines") {
		t.Error("expected tab bar to contain '3: Pipelines' (not overridden)")
	}
}

func TestModel_DisabledPanes_EnabledTabs_AllEnabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}

	tabs := buildEnabledTabs(cfg, true, false)
	if len(tabs) != 3 {
		t.Fatalf("expected 3 enabled tabs, got %d", len(tabs))
	}
	if tabs[0] != TabPullRequests || tabs[1] != TabWorkItems || tabs[2] != TabPipelines {
		t.Errorf("unexpected tab order: %v", tabs)
	}
}

func TestModel_DisabledPanes_EnabledTabs_BothDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pipelines", "workitems"},
	}

	tabs := buildEnabledTabs(cfg, true, false)
	if len(tabs) != 1 {
		t.Fatalf("expected 1 enabled tab, got %d", len(tabs))
	}
	if tabs[0] != TabPullRequests {
		t.Errorf("expected TabPullRequests, got %d", tabs[0])
	}
}

func TestModel_DisabledPanes_PullRequestsDisabled_TabBarHidesPullRequests(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pullrequests"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	view := m.View()

	if strings.Contains(view, "Pull Requests") {
		t.Error("Tab bar should NOT show Pull Requests when disabled")
	}
	if !strings.Contains(view, "1: Work Items") {
		t.Error("Tab bar should show '1: Work Items' (renumbered)")
	}
	if !strings.Contains(view, "2: Pipelines") {
		t.Error("Tab bar should show '2: Pipelines' (renumbered)")
	}
}

func TestModel_DisabledPanes_PullRequestsDisabled_DefaultTabIsWorkItems(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pullrequests"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")

	if m.activeTab != TabWorkItems {
		t.Errorf("with pullrequests disabled, expected default activeTab to be TabWorkItems (first enabled), got %d", m.activeTab)
	}
}

func TestModel_DisabledPanes_EnabledTabs_PullRequestsDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pullrequests"},
	}

	tabs := buildEnabledTabs(cfg, false, false)
	if len(tabs) != 2 {
		t.Fatalf("expected 2 enabled tabs, got %d", len(tabs))
	}
	if tabs[0] != TabWorkItems || tabs[1] != TabPipelines {
		t.Errorf("unexpected tab order: %v", tabs)
	}
}

// openTagPickerOnWorkItemsTab returns a Model with the work items tab active
// and the tag picker open, ready for keypress tests.
func openTagPickerOnWorkItemsTab(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)

	items := []provider.WorkItem{
		{Identity: provider.Identity{ID: "1"}, Title: "A", Tags: "Spring"},
	}
	updated, _ = m.Update(workitems.SetWorkItemsMsg{WorkItems: items})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = updated.(Model)

	if !m.workItemsView.IsTagPickerVisible() {
		t.Fatal("precondition failed: tag picker should be visible")
	}
	return m
}

// TestNewModel_NilAzureBackend_NoInitPanic is a smoke test for the GitHub-only
// path (mc == nil). It verifies that constructing the model, calling Init(), and
// processing a WindowSizeMsg through Update() all complete without panicking.
// Before the fix, NewModel passed the typed-nil *azdevops.MultiClient directly
// into NewPoller, boxing it into a non-nil PipelineClient interface value. The
// first FetchPipelineRuns call would then dispatch through the interface to a nil
// MultiClient, dereferencing mc.clients and panicking at startup.
func TestNewModel_NilAzureBackend_NoInitPanic(t *testing.T) {
	cfg := &config.Config{
		Organization:    "githubuser",
		Projects:        []string{},
		PollingInterval: 30,
		Theme:           "dark",
	}

	// Explicitly typed nil — no Azure backend (GitHub-only user).
	var mc *azdevops.MultiClient

	// Must not panic during construction.
	m := NewModel(nil, mc, cfg, "dev", "")

	// Init() must not panic (previously panicked via FetchPipelineRuns → nil deref).
	_ = m.Init()

	// Pin the regression directly: the poller command that previously panicked is
	// FetchPipelineRuns. Execute it WITHOUT a recover wrapper. The fix guards on a
	// genuinely-nil client and returns a nil cmd; if the typed-nil *MultiClient is
	// ever boxed back into a non-nil interface, p.client == nil is false and this
	// closure dereferences the nil MultiClient — panicking and failing the test.
	if cmd := m.poller.FetchPipelineRuns(); cmd != nil {
		t.Errorf("FetchPipelineRuns() = non-nil cmd, want nil no-op for nil Azure backend; executing it: %v", cmd())
	}
	// Same guarantee on the periodic-tick path.
	if cmd := m.poller.OnTick(); cmd != nil {
		t.Errorf("OnTick() = non-nil cmd, want nil no-op for nil Azure backend; executing it: %v", cmd())
	}

	// WindowSizeMsg through Update must also be panic-free.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Basic sanity: model is usable.
	if m.activeTab != TabPullRequests {
		t.Errorf("expected default tab to be TabPullRequests, got %d", m.activeTab)
	}
}

func TestModel_GlobalShortcutsDisabledWhenTagPickerOpen(t *testing.T) {
	cases := []struct {
		name string
		key  rune
	}{
		{"q_does_not_quit", 'q'},
		{"t_does_not_open_theme_picker", 't'},
		{"help_does_not_open", '?'},
		{"digit_1_does_not_switch_tab", '1'},
		{"digit_3_does_not_switch_tab", '3'},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := openTagPickerOnWorkItemsTab(t)

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
			m = updated.(Model)

			if m.themePicker.IsVisible() {
				t.Error("theme picker opened while tag picker was active")
			}
			if m.helpModal.IsVisible() {
				t.Error("help modal opened while tag picker was active")
			}
			if m.activeTab != TabWorkItems {
				t.Errorf("active tab changed while tag picker was active, got %d", m.activeTab)
			}
			if !m.workItemsView.IsTagPickerVisible() {
				t.Error("tag picker was dismissed when it should have stayed open")
			}
			if got := m.workItemsView.TagPickerSearchQuery(); got != string(tc.key) {
				t.Errorf("expected key %q to be typed into tag search, got %q", tc.key, got)
			}
		})
	}
}

// --- Task 12: notifications tab registration (Decisions 6, 9, 11) ---------

// notificationsPaneMarker is the string that discriminates "the notifications
// pane rendered" from "some other pane rendered in the notifications tab's
// slot" (Decision 60). listview.viewList formats its empty state as
// "No <EntityName> found.", and notifications.NewModelWithStyles sets
// EntityName to "notifications", so an empty notifications pane emits this and
// no sibling pane can ("pipeline runs", "pull requests", "work items").
//
// The tab strip's "1: Notifications" label is NOT sufficient on its own:
// deleting `case TabNotifications:` from View()'s content switch makes the tab
// render the *pipelines* pane while the label stays put, so a label-only
// assertion passes on a perfectly valid render of the wrong pane. Convention 8
// is satisfied either way — the test does drive View() after a WindowSizeMsg —
// which is exactly why the content has to be pinned too.
const notificationsPaneMarker = "No notifications found."

// TestBuildEnabledTabs_NotificationsFirst_WhenCapable pins Decision 6:
// notifications lands at enabledTabs[0] whenever it is present at all.
func TestBuildEnabledTabs_NotificationsFirst_WhenCapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}

	tabs := buildEnabledTabs(cfg, false, true)
	if len(tabs) != 4 {
		t.Fatalf("expected 4 enabled tabs, got %d: %v", len(tabs), tabs)
	}
	if tabs[0] != TabNotifications {
		t.Fatalf("expected TabNotifications first, got %v", tabs[0])
	}
	if tabs[1] != TabPullRequests || tabs[2] != TabWorkItems || tabs[3] != TabPipelines {
		t.Errorf("unexpected tab order after notifications: %v", tabs)
	}
}

// TestBuildEnabledTabs_NotificationsAbsent_WhenIncapable pins Decision 11:
// the tab is gated on capability, never on config alone — an Azure-only
// provider (notifCapable=false) must never surface it even though
// "notifications" is not in DisabledPanes.
func TestBuildEnabledTabs_NotificationsAbsent_WhenIncapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}

	tabs := buildEnabledTabs(cfg, false, false)
	for _, tab := range tabs {
		if tab == TabNotifications {
			t.Fatalf("expected no TabNotifications when incapable, got tabs: %v", tabs)
		}
	}
	if len(tabs) != 3 {
		t.Fatalf("expected 3 enabled tabs, got %d: %v", len(tabs), tabs)
	}
}

// TestBuildEnabledTabs_NotificationsAbsent_WhenPaneDisabled confirms
// disabled_panes still applies on top of capability, same as every other pane.
func TestBuildEnabledTabs_NotificationsAbsent_WhenPaneDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}

	tabs := buildEnabledTabs(cfg, false, true)
	for _, tab := range tabs {
		if tab == TabNotifications {
			t.Fatalf("expected no TabNotifications when pane disabled, got tabs: %v", tabs)
		}
	}
}

// TestHasNotificationCapability_AzureOnlyComposite_False pins Decision 59
// directly on the gate function: the production provider shape for an
// Azure-only config is a *provider.CompositeProvider wrapping an
// *azdevops.Adapter, and it must report false.
//
// This is the assertion the naive `_, ok := p.(provider.NotificationSource)`
// form fails: *CompositeProvider satisfies NotificationSource unconditionally,
// so the assertion succeeds and capability is reported for a config with no
// GitHub backend at all. Only calling HasNotifications() — which performs the
// real per-backend assertion — gets this right.
func TestHasNotificationCapability_AzureOnlyComposite_False(t *testing.T) {
	azureOnly := newNotificationIncapableProvider()

	// Guard the fixture itself: it must be capable-*shaped*, i.e. it must
	// satisfy provider.NotificationSource, or it cannot tell the correct gate
	// from the naive one and the assertion below goes vacuous.
	if _, ok := azureOnly.(provider.NotificationSource); !ok {
		t.Fatal("fixture is not capable-shaped: *CompositeProvider must satisfy provider.NotificationSource, otherwise this test cannot distinguish the capability gate from a bare type assertion")
	}

	if hasNotificationCapability(azureOnly) {
		t.Error("hasNotificationCapability(Azure-only composite) = true, want false — the gate must ask CompositeProvider.HasNotifications(), not assert provider.NotificationSource on the composite (Decisions 11, 59)")
	}

	// The capable shape must still report true, so the gate is not simply
	// stuck at false.
	if !hasNotificationCapability(newNotificationCapableProvider()) {
		t.Error("hasNotificationCapability(GitHub composite) = false, want true")
	}
}

// TestNotificationsPollerClient_ReturnsNilWhenIncapable pins the review fix's
// belt-and-braces half at the poller-construction call site: passing nil as
// the client whenever the provider is not notification-capable, even though
// notificationMarker(p) alone would still return a non-nil
// *provider.CompositeProvider (notificationMarker deliberately does not call
// HasNotifications(), by its own doc comment — correct for the pane's mark
// routing, wrong for a poller that must never arm its timer against an
// incapable backend). This is independent of and in addition to Init()'s own
// isTabEnabled gate: a capable-shaped composite over an Azure-only backend
// must still yield a nil client here, regardless of what any caller checks
// before constructing the poller.
func TestNotificationsPollerClient_ReturnsNilWhenIncapable(t *testing.T) {
	azureOnly := newNotificationIncapableProvider()

	if _, ok := azureOnly.(provider.NotificationSource); !ok {
		t.Fatal("fixture is not capable-shaped: this test needs a composite that satisfies provider.NotificationSource unconditionally, to distinguish the capability-checked helper from a bare notificationMarker call")
	}

	if got := notificationsPollerClient(azureOnly); got != nil {
		t.Errorf("notificationsPollerClient(Azure-only composite) = %v, want nil — the poller must never be constructed with a client for a non-notification-capable provider", got)
	}

	if got := notificationsPollerClient(newNotificationCapableProvider()); got == nil {
		t.Error("notificationsPollerClient(GitHub composite) = nil, want a non-nil client")
	}
}

// hintingProviderStub embeds provider.Provider so only
// NotificationsPollInterval needs an implementation, mirroring scopeStub's
// pattern above. It exists solely to make notificationsIntervalHinter
// satisfiable in tests without a full fake Provider.
type hintingProviderStub struct {
	provider.Provider
	hint time.Duration
}

func (h hintingProviderStub) NotificationsPollInterval() time.Duration { return h.hint }

// TestNotificationsPollIntervalHint_NonHintingProvider_ReturnsZero pins the
// "no hint available" branch of notificationsPollIntervalHint: a provider
// that does not implement notificationsIntervalHinter (e.g. the Azure-only
// composite, which is capable-shaped for NotificationSource but has no
// NotificationsPollInterval method) must yield 0, never panic or fall back
// to some other value.
func TestNotificationsPollIntervalHint_NonHintingProvider_ReturnsZero(t *testing.T) {
	p := newNotificationIncapableProvider()
	if got := notificationsPollIntervalHint(p); got != 0 {
		t.Errorf("notificationsPollIntervalHint(non-hinting provider) = %v, want 0", got)
	}
}

// TestNotificationsPollIntervalHint_HintingProvider_ReturnsHint pins the
// positive branch: a provider implementing notificationsIntervalHinter must
// have its value returned verbatim.
func TestNotificationsPollIntervalHint_HintingProvider_ReturnsHint(t *testing.T) {
	p := hintingProviderStub{hint: 90 * time.Second}
	if got := notificationsPollIntervalHint(p); got != 90*time.Second {
		t.Errorf("notificationsPollIntervalHint(hinting provider) = %v, want 90s", got)
	}
}

// TestNotificationsConfiguredInterval_NilConfig_DefaultInterval pins the nil
// guard.
func TestNotificationsConfiguredInterval_NilConfig_DefaultInterval(t *testing.T) {
	if got := notificationsConfiguredInterval(nil); got != polling.DefaultInterval {
		t.Errorf("notificationsConfiguredInterval(nil) = %v, want %v", got, polling.DefaultInterval)
	}
}

// TestNotificationsConfiguredInterval_NotificationsSpecific_TakesPrecedence
// pins that cfg.Notifications.PollInterval outranks cfg.PollingInterval when
// both are set, matching the doc comment's stated precedence order.
func TestNotificationsConfiguredInterval_NotificationsSpecific_TakesPrecedence(t *testing.T) {
	cfg := &config.Config{
		PollingInterval: 60,
		Notifications:   config.NotificationsConfig{PollInterval: 120},
	}
	want := 120 * time.Second
	if got := notificationsConfiguredInterval(cfg); got != want {
		t.Errorf("notificationsConfiguredInterval() = %v, want %v", got, want)
	}
}

// TestNotificationsConfiguredInterval_FallsBackToPollingInterval pins the
// fallback when notifications.poll_interval is unset (0).
func TestNotificationsConfiguredInterval_FallsBackToPollingInterval(t *testing.T) {
	cfg := &config.Config{PollingInterval: 45}
	want := 45 * time.Second
	if got := notificationsConfiguredInterval(cfg); got != want {
		t.Errorf("notificationsConfiguredInterval() = %v, want %v", got, want)
	}
}

// TestNotificationsConfiguredInterval_FallsBackToDefaultInterval pins the
// final fallback when both cfg.Notifications.PollInterval and
// cfg.PollingInterval are 0.
func TestNotificationsConfiguredInterval_FallsBackToDefaultInterval(t *testing.T) {
	cfg := &config.Config{}
	if got := notificationsConfiguredInterval(cfg); got != polling.DefaultInterval {
		t.Errorf("notificationsConfiguredInterval() = %v, want %v", got, polling.DefaultInterval)
	}
}

// TestNotificationsPollInterval_HintWinsWhenLarger pins Decision 8's cadence
// formula max(configured, hint) for the case where the server hint exceeds
// the configured interval.
func TestNotificationsPollInterval_HintWinsWhenLarger(t *testing.T) {
	cfg := &config.Config{PollingInterval: 30}
	p := hintingProviderStub{hint: 300 * time.Second}
	want := 300 * time.Second
	if got := notificationsPollInterval(p, cfg); got != want {
		t.Errorf("notificationsPollInterval() = %v, want %v (hint must win)", got, want)
	}
}

// TestNotificationsPollInterval_ConfiguredWinsWhenHintSmallerOrAbsent pins
// the other half of Decision 8's formula: the configured interval must never
// be shrunk by a smaller (or absent, i.e. zero) hint.
func TestNotificationsPollInterval_ConfiguredWinsWhenHintSmallerOrAbsent(t *testing.T) {
	cfg := &config.Config{PollingInterval: 120}
	want := 120 * time.Second

	// Hint smaller than configured.
	small := hintingProviderStub{hint: 10 * time.Second}
	if got := notificationsPollInterval(small, cfg); got != want {
		t.Errorf("notificationsPollInterval() = %v, want %v (configured must win over a smaller hint)", got, want)
	}

	// No hint at all (provider does not implement notificationsIntervalHinter).
	incapable := newNotificationIncapableProvider()
	if got := notificationsPollInterval(incapable, cfg); got != want {
		t.Errorf("notificationsPollInterval() = %v, want %v (configured must win when no hint is available)", got, want)
	}
}

// TestModel_NotificationsTab_Absent_WhenIncapable exercises the real NewModel
// path with the Azure-only composite of Decision 59 (never a nil provider —
// see newNotificationIncapableProvider): the tab bar must not mention
// notifications at all, no notifications pane body may render, and Pull
// Requests must keep its "1:" slot exactly as before this task.
func TestModel_NotificationsTab_Absent_WhenIncapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationIncapableProvider(), client, cfg, "dev", "")
	m.width = 100
	m.height = 30
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	if m.isTabEnabled(TabNotifications) {
		t.Error("TabNotifications must not be in enabledTabs for an Azure-only composite")
	}
	if m.activeTab != TabPullRequests {
		t.Errorf("activeTab = %v, want TabPullRequests when notifications is capability-absent", m.activeTab)
	}

	view := m.View()
	if strings.Contains(view, "Notifications") {
		t.Error("tab bar should not mention Notifications when no backend implements provider.NotificationSource")
	}
	if strings.Contains(view, notificationsPaneMarker) {
		t.Errorf("notifications pane body must not render when the tab is capability-absent, view:\n%s", view)
	}
	if !strings.Contains(view, "1: Pull Requests") {
		t.Error("expected '1: Pull Requests' to remain the first tab when notifications is absent")
	}
}

// TestModel_NotificationsTab_Absent_WhenPaneDisabled_ButCapable covers the one
// combination Decision 61 found rendered nowhere: a capable provider with
// `disabled_panes: [notifications]`. buildEnabledTabs' unit tests cover the
// predicate, but nothing rendered it, which is why dropping the IsPaneEnabled
// conjunct from NewModel's second copy of that predicate survived — the tab
// strip and the help modal were free to disagree.
//
// Asserting both surfaces here is the point: they now derive from the same
// computed enabledTabs slice, so a divergence is a test failure rather than a
// silent inconsistency.
func TestModel_NotificationsTab_Absent_WhenPaneDisabled_ButCapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 200
	m.height = 60
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	if m.isTabEnabled(TabNotifications) {
		t.Error("TabNotifications must not be enabled when disabled_panes lists notifications, even with a capable provider")
	}
	if m.activeTab != TabPullRequests {
		t.Errorf("activeTab = %v, want TabPullRequests", m.activeTab)
	}

	// Tab strip: no Notifications, and PR keeps slot 1.
	view := m.View()
	if strings.Contains(view, "Notifications") {
		t.Errorf("tab strip must not list Notifications when the pane is disabled, view:\n%s", view)
	}
	if !strings.Contains(view, "1: Pull Requests") {
		t.Errorf("expected '1: Pull Requests' when notifications is disabled, view:\n%s", view)
	}
	if strings.Contains(view, notificationsPaneMarker) {
		t.Errorf("notifications pane body must not render when the pane is disabled, view:\n%s", view)
	}

	// Help modal: the Tabs line must agree with the strip.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)
	helpView := m.View()
	if strings.Contains(helpView, "Notifications") {
		t.Errorf("help modal Tabs line must not list Notifications when the pane is disabled, view:\n%s", helpView)
	}
	if !strings.Contains(helpView, "PR / Work Items / Pipelines") {
		t.Errorf("help modal Tabs line should read 'PR / Work Items / Pipelines' when notifications is disabled, view:\n%s", helpView)
	}
}

// TestModel_NotificationsTab_PresentButEmpty_WhenCapable exercises NewModel
// with a capable provider (Decision 11): the tab must appear first (Decision
// 6), the *notifications pane* must be what renders in it (Decision 60 — see
// notificationsPaneMarker for why the tab label alone is not enough), and
// rendering after a WindowSizeMsg must not panic (convention 8).
//
// The pane renders its loading state here, not notificationsPaneMarker's
// empty-inbox state: NewModel constructs the pane with SetLoading(true)
// (Decision 64) and this test never calls Init() nor resolves any fetch, so
// `m.loading` is never cleared. That is deliberately the regression this test
// now guards instead: before task 15, listview.Init set the spinner visible
// but never assigned m.loading, so the empty-inbox marker rendered during the
// window between construction and the first fetch's resolution.
func TestModel_NotificationsTab_PresentButEmpty_WhenCapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	if m.activeTab != TabNotifications {
		t.Fatalf("expected default activeTab to be TabNotifications (first enabled), got %d", m.activeTab)
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View() // must not panic
	if !strings.Contains(view, "1: Notifications") {
		t.Errorf("expected tab bar to contain '1: Notifications', view:\n%s", view)
	}
	if !strings.Contains(view, "Loading notifications...") {
		t.Errorf("expected the notifications pane's loading state (no fetch has resolved yet), view:\n%s", view)
	}
	if strings.Contains(view, notificationsPaneMarker) {
		t.Errorf("notifications pane must not render the empty-inbox marker before its first fetch resolves; view:\n%s", view)
	}
	// Negative half: the sibling the content switch falls through to must NOT
	// be what rendered. Without this, a `default:`-branch render of the
	// pipelines pane would only be caught by the assertion above going absent.
	if strings.Contains(view, "No pipeline runs found.") {
		t.Errorf("notifications tab rendered the pipelines pane — View()'s content switch fell through; view:\n%s", view)
	}
}

// TestModel_PerTabChrome pins the five per-tab `case` arms that route the
// active view's chrome, all of which deleted clean before this test existed
// (Decision 61's neighbourhood: app.go's initTabCmd, resizeActiveViewIfNeeded,
// syncStatusBarContext, the keybindings if/else chain, and the WindowSizeMsg
// sizing loop).
//
// Everything is asserted through View() (convention 8), because the status bar
// is populated as a side effect of rendering: View() calls
// statusBar.SetKeybindings(m.<tab>Keybindings()) and reads the active view's
// GetContextItems/GetScrollPercent/GetStatusMessage from the same switch. A
// per-tab keybindings string is therefore the cheapest observable that
// distinguishes "the arm for this tab ran" from "the default: arm ran", and
// notificationsKeybindings() in particular had zero coverage.
func TestModel_PerTabChrome(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Metrics:         config.MetricsConfig{Enabled: true},
	}
	client, err := azdevops.NewMultiClient("testorg", []string{"testproject"}, "dummy-pat", nil)
	if err != nil {
		t.Fatalf("NewMultiClient() error = %v", err)
	}

	tests := []struct {
		name string
		tab  Tab
		// wantKeys are substrings unique to that tab's keybindings string.
		wantKeys []string
		// notWantKeys are substrings owned by a *different* tab's keybindings
		// string, so a fall-through to the wrong arm is caught positively
		// rather than only as a missing expectation.
		notWantKeys []string
		// wantPane is the active pane's own body text, pinning the View()
		// content switch and (via the empty-state render) that the pane was
		// sized by the WindowSizeMsg handler rather than left at width 0.
		wantPane string
	}{
		{
			name:        "notifications",
			tab:         TabNotifications,
			wantKeys:    []string{"f filter reason", "o open"},
			notWantKeys: []string{"S status", "m my items", "m my PRs", "v live/trends"},
			// NewModel constructs the pane with SetLoading(true) (Decision 64)
			// and this test never resolves a fetch, so the pane renders its
			// loading state rather than notificationsPaneMarker's empty-inbox
			// state.
			wantPane: "Loading notifications...",
		},
		{
			name:        "pullrequests",
			tab:         TabPullRequests,
			wantKeys:    []string{"m my PRs", "A as reviewer"},
			notWantKeys: []string{"f filter reason", "S status"},
			wantPane:    "No pull requests found.",
		},
		{
			name:        "workitems",
			tab:         TabWorkItems,
			wantKeys:    []string{"m my items", "s state"},
			notWantKeys: []string{"f filter reason", "S status"},
			wantPane:    "No work items found.",
		},
		{
			name:        "pipelines",
			tab:         TabPipelines,
			wantKeys:    []string{"S status"},
			notWantKeys: []string{"f filter reason", "m my PRs"},
			wantPane:    "No pipeline runs found.",
		},
		{
			name:        "metrics",
			tab:         TabMetrics,
			wantKeys:    []string{"v live/trends"},
			notWantKeys: []string{"f filter reason", "S status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = updated.(Model)

			if !m.isTabEnabled(tt.tab) {
				t.Fatalf("precondition failed: tab %v not enabled; enabledTabs = %v", tt.tab, m.enabledTabs)
			}
			m.activeTab = tt.tab
			// The real tab-switch path re-syncs the status bar's context items
			// and resizes the newly active view; drive it so the
			// syncStatusBarContext and resizeActiveViewIfNeeded arms run too.
			m.resizeActiveViewIfNeeded()

			view := m.View()

			for _, want := range tt.wantKeys {
				if !strings.Contains(view, want) {
					t.Errorf("tab %v: keybindings missing %q — View()'s per-tab arm did not run; view:\n%s", tt.tab, want, view)
				}
			}
			for _, notWant := range tt.notWantKeys {
				if strings.Contains(view, notWant) {
					t.Errorf("tab %v: view contains %q, which belongs to a different tab's keybindings — the wrong arm ran; view:\n%s", tt.tab, notWant, view)
				}
			}
			if tt.wantPane != "" && !strings.Contains(view, tt.wantPane) {
				t.Errorf("tab %v: expected pane body %q; view:\n%s", tt.tab, tt.wantPane, view)
			}
		})
	}
}

// TestModel_WindowSizeMsg_SizesNotificationsPane pins the notifications line in
// app.go's tea.WindowSizeMsg handler, which deleted clean: the pane kept
// listview's construction-time defaults and rendered a narrow, short table
// inside a full-width box, with nothing failing.
//
// Sizing is asserted by comparison against a reference pane sized directly with
// the Model's own contentViewSize() and fed the same rows — exact, and immune to
// the box-arithmetic constants. The anti-vacuity half compares against an
// unsized reference so the test cannot pass by both panes happening to be
// unsized.
func TestModel_WindowSizeMsg_SizesNotificationsPane(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	feed := []provider.Notification{
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"},
			Title:     "A notification title",
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		},
	}

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.notificationsView = m.notificationsView.SetFeed(feed)

	got := m.notificationsView.View()

	sized := notifications.NewModelWithStyles(m.styles, nil, nil)
	sized, _ = sized.Update(m.contentViewSize())
	sized = sized.SetFeed(feed)
	want := sized.View()

	unsized := notifications.NewModelWithStyles(m.styles, nil, nil)
	unsized = unsized.SetFeed(feed)
	reference := unsized.View()

	if lipgloss.Width(want) == lipgloss.Width(reference) && lipgloss.Height(want) == lipgloss.Height(reference) {
		t.Fatalf("fixture is vacuous: a sized pane (%dx%d) renders identically to an unsized one",
			lipgloss.Width(want), lipgloss.Height(want))
	}
	if lipgloss.Width(got) != lipgloss.Width(want) || lipgloss.Height(got) != lipgloss.Height(want) {
		t.Errorf("notifications pane rendered %dx%d, want %dx%d (contentViewSize %dx%d); an unsized pane renders %dx%d — the WindowSizeMsg handler did not size it",
			lipgloss.Width(got), lipgloss.Height(got),
			lipgloss.Width(want), lipgloss.Height(want),
			m.contentViewSize().Width, m.contentViewSize().Height,
			lipgloss.Width(reference), lipgloss.Height(reference))
	}
}

// TestModel_SwitchToNotificationsTab_ResizesPaneAndAccountsFooter pins the two
// remaining unpinned TabNotifications arms that both live on the tab-switch
// path: resizeActiveViewIfNeeded's (app.go's switch on m.activeTab after the
// footer height changed) and syncStatusBarContext's.
//
// Neither is observable from a plain tab switch, which is why both deleted
// clean. resizeActiveViewIfNeeded only resizes when the footer height actually
// changes, and syncStatusBarContext's notifications arm only differs from the
// default arm when the pipelines pane happens to have a context bar — the
// notifications pane's own HasContextBar() is structurally false in phase 1
// (decision 57 leaves listview's HasContextBar hook nil), so it is the *stale
// other pane* that has to supply the difference.
//
// The scenario builds exactly that state:
//
//	a. size the window on the notifications tab (footer 3 rows)
//	b. open pipelines and press enter to enter detail mode, which turns its
//	   context bar on and grows the footer to 4 rows
//	c. re-send WindowSizeMsg while that detail view is open — its handler
//	   sizes *every* pane, so the notifications pane is now one row short
//	d. switch back to notifications
//
// At (d) the footer must shrink back to 3 rows, which requires
// syncStatusBarContext to read the notifications pane (not fall through to
// pipelines' still-open context bar), and the newly active pane must be
// re-measured, which requires resizeActiveViewIfNeeded's notifications arm.
//
// Deleting the resize arm leaves the pane at (c)'s height while the layout
// reserves (d)'s; deleting the sync arm leaves pipelines' context items on the
// status bar, so measureFooterHeight reports 4 rows while View() — whose own
// switch is a separate copy — renders 3, and the whole frame comes out a row
// shorter than the terminal.
func TestModel_SwitchToNotificationsTab_ResizesPaneAndAccountsFooter(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	// Enough rows that the pane fills whatever height it is given, so its
	// rendered height reflects the last size it was told about.
	feed := make([]provider.Notification, 0, 60)
	for i := 0; i < 60; i++ {
		feed = append(feed, provider.Notification{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ScopeDisplay: "o/r", ID: fmt.Sprintf("%d", i)},
			Title:     fmt.Sprintf("notification %d", i),
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		})
	}

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	// 140 columns, not 120: wide enough that the unread badge (task 16,
	// decision 21) — which now renders on every tab, including pipelines'
	// detail view entered in step (b) below — does not by itself tip the
	// *notifications* tab's single-line footer onto a second line, while
	// still leaving pipelines' longer detail-context keybindings line (with
	// the same badge) wrapped onto two. At 120 columns the badge's added
	// width wraps both alike, at 150+ neither wraps (task 17's `o` key
	// lengthened notificationsKeybindings() enough that the old 130-column
	// calibration no longer cleared the badge's width at all), and either
	// way the two footer heights this test differentiates would come out
	// equal — a vacuous fixture that can't exercise the actual resize/sync
	// logic.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.notificationsView = m.notificationsView.SetFeed(feed)

	// (b) pipelines detail mode.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	m = updated.(Model)
	if m.activeTab != TabPipelines {
		t.Fatalf("precondition failed: activeTab is %v after pressing 4, want TabPipelines", m.activeTab)
	}
	updated, _ = m.Update(pipelines.SetRunsMsg{Runs: []provider.PipelineRun{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "testproject", ID: "1"},
			DefinitionName: "build",
			BuildNumber:    "42",
			Status:         "completed",
			Result:         "succeeded",
			QueueTime:      time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		},
	}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.pipelinesView.HasContextBar() {
		t.Fatalf("precondition failed: pipelines pane has no context bar after enter, so the fallthrough arm cannot be distinguished")
	}

	// (c) resize while the detail context bar is open: the WindowSizeMsg handler
	// sizes every pane, so the notifications pane picks up the shorter height.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	staleHeight := lipgloss.Height(m.notificationsView.View())

	// (d) back to notifications.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = updated.(Model)
	if m.activeTab != TabNotifications {
		t.Fatalf("precondition failed: activeTab is %v after pressing 1, want TabNotifications", m.activeTab)
	}

	view := m.View()

	// syncStatusBarContext: the footer must be measured for the notifications
	// tab, not for pipelines' still-open detail context bar. A footer measured
	// one row too tall costs the frame a row of terminal height.
	if h := lipgloss.Height(view); h != m.height {
		t.Errorf("View() rendered %d rows for a %d-row terminal (footerRows=%d) — the footer was measured against another pane's context bar; view:\n%s",
			h, m.height, m.footerRows, view)
	}

	// resizeActiveViewIfNeeded: the pane must be re-measured for the shorter
	// footer instead of keeping (c)'s height.
	sized := notifications.NewModelWithStyles(m.styles, nil, nil)
	sized, _ = sized.Update(m.contentViewSize())
	sized = sized.SetFeed(feed)
	wantHeight := lipgloss.Height(sized.View())

	if wantHeight == staleHeight {
		t.Fatalf("fixture is vacuous: the pane's height before (%d) and after (%d) the footer shrank are equal, so a missing resize is unobservable",
			staleHeight, wantHeight)
	}
	if got := lipgloss.Height(m.notificationsView.View()); got != wantHeight {
		t.Errorf("notifications pane rendered %d rows after the tab switch, want %d (contentViewSize height %d); it still has the %d rows it had while pipelines' detail view was open — the active view was not resized",
			got, wantHeight, m.contentViewSize().Height, staleHeight)
	}
}

// TestModel_NotificationsPane_ConstructedEvenWhenTabAbsent pins Decision 61's
// second half: the pane is constructed unconditionally, so no zero-value
// notifications.Model is ever reachable from a Model built by NewModel.
//
// The comment this replaces claimed the zero value is never reached. It is:
// app.go's tea.WindowSizeMsg handler calls m.notificationsView.Update
// unconditionally and ThemeSelectedMsg reconstructs the pane unconditionally.
// And it is not inert — listview.Init does m.spinner.SetVisible(true) on a nil
// *components.LoadingIndicator and panics. Nothing routes a message to a
// disabled pane today, so this is latent rather than live, but tasks 15 and 16
// deliver messages to this pane from the top-level switch and a future
// implementer would have trusted that comment.
//
// Init() and SetFeed are asserted directly, without a recover wrapper
// (convention 16): a helper that recovers would swallow the very panic this
// test exists to guard and would pass against the unfixed code too.
func TestModel_NotificationsPane_ConstructedEvenWhenTabAbsent(t *testing.T) {
	cases := []struct {
		name     string
		cfg      *config.Config
		provider provider.Provider
	}{
		{
			name: "capability absent",
			cfg: &config.Config{
				Organization: "testorg", Projects: []string{"testproject"},
				PollingInterval: 60, Theme: "dark",
			},
			provider: newNotificationIncapableProvider(),
		},
		{
			name: "pane disabled",
			cfg: &config.Config{
				Organization: "testorg", Projects: []string{"testproject"},
				PollingInterval: 60, Theme: "dark",
				DisabledPanes: []string{"notifications"},
			},
			provider: newNotificationCapableProvider(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var client *azdevops.MultiClient
			m := NewModel(tc.provider, client, tc.cfg, "dev", "")

			if m.isTabEnabled(TabNotifications) {
				t.Fatalf("precondition failed: TabNotifications should be absent for %q", tc.name)
			}

			// Both of these dereference the pane's *LoadingIndicator and panic
			// on a zero-value notifications.Model.
			if cmd := m.notificationsView.Init(); cmd == nil {
				t.Error("notificationsView.Init() returned nil — the pane looks unconstructed")
			}
			m.notificationsView = m.notificationsView.SetFeed([]provider.Notification{
				{
					Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"},
					Title:     "Something",
					Reason:    provider.NotificationReasonMentioned,
					UpdatedAt: time.Now(),
				},
			})
		})
	}
}

// TestModel_InitTabCmd_Notifications pins app.go's initTabCmd arm for the
// notifications tab: without it the pane's Init is never dispatched, which is
// silent today (the Fetch hook is task 11's stub) and a visible bug the moment
// task 15 wires the real fetch. Asserted as a non-nil cmd, matching how the
// sibling arms are observable.
func TestModel_InitTabCmd_Notifications(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	if cmd := m.initTabCmd(TabNotifications); cmd == nil {
		t.Error("initTabCmd(TabNotifications) = nil, want the pane's Init cmd — without the case arm the notifications pane is never initialised")
	}
	// Pipelines is the tab initTabCmd deliberately returns nil for (the poller
	// populates it), so this confirms the assertion above is not just "every
	// tab returns non-nil".
	if cmd := m.initTabCmd(TabPipelines); cmd != nil {
		t.Error("initTabCmd(TabPipelines) should stay nil (populated by the poller)")
	}
}

// TestModel_DigitKeys_MapToNewOrder_AllFiveTabs presses "1".."5" with every
// tab enabled (notifications, PR, work items, pipelines, metrics) and
// confirms each digit lands on the tab at its purely positional index in
// enabledTabs — pinning that notifications-first (Decision 6) shifts every
// other tab's number without any dedicated per-tab key-mapping logic.
func TestModel_DigitKeys_MapToNewOrder_AllFiveTabs(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Metrics:         config.MetricsConfig{Enabled: true},
	}
	client, err := azdevops.NewMultiClient("testorg", []string{"testproject"}, "dummy-pat", nil)
	if err != nil {
		t.Fatalf("NewMultiClient() error = %v", err)
	}

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 100
	m.height = 30

	want := []Tab{TabNotifications, TabPullRequests, TabWorkItems, TabPipelines, TabMetrics}
	if len(m.enabledTabs) != len(want) {
		t.Fatalf("expected %d enabled tabs, got %d: %v", len(want), len(m.enabledTabs), m.enabledTabs)
	}

	for i, tab := range want {
		key := string(rune('1' + i))
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = updated.(Model)
		if m.activeTab != tab {
			t.Errorf("after pressing %q, activeTab = %v, want %v", key, m.activeTab, tab)
		}
	}
}

// TestModel_HelpModal_ReflectsNotificationsFirst pins the correctness fix
// called out alongside Decision 6: components/help.go seeds the Tabs section
// with the hard-coded default "1/2/3 — PR / Work Items / Pipelines"
// (internal/ui/components/help.go's NewHelpModal), which would otherwise go
// stale the moment notifications is registered first. NewModel's
// UpdateTabsBinding call must overwrite it to include Notifications in slot
// 1 whenever the tab is capability-present.
func TestModel_HelpModal_ReflectsNotificationsFirst(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 200
	m.height = 60

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	// Open the help modal.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	if !strings.Contains(view, "1/2/3/4") {
		t.Error("help modal Tabs line should list keys '1/2/3/4' once notifications is enabled")
	}
	if !strings.Contains(view, "Notifications / PR / Work Items / Pipelines") {
		t.Errorf("help modal Tabs line should read 'Notifications / PR / Work Items / Pipelines', view:\n%s", view)
	}
}

// notificationsHelpSection extracts the slice of view between the
// "Notifications tab" section heading and the next section heading ("Info",
// which SetScopes/SetVersionInfo/SetConfigPath always populate for these
// fixtures), so assertions can be scoped to just this section's own bindings
// rather than the whole modal body.
func notificationsHelpSection(t *testing.T, view string) string {
	t.Helper()
	start := strings.Index(view, "Notifications tab")
	if start < 0 {
		t.Fatalf("view does not contain a 'Notifications tab' section:\n%s", view)
	}
	rest := view[start:]
	end := strings.Index(rest, "Info")
	if end < 0 {
		t.Fatalf("could not find the 'Info' section following 'Notifications tab':\n%s", view)
	}
	return rest[:end]
}

// TestModel_HelpModal_NotificationsSection_PresentWhenEnabled pins task 17's
// core criterion: a "Notifications tab" help section is added when the tab is
// enabled (capable provider, not in disabled_panes), listing exactly the keys
// internal/ui/notifications/list.go's Update switch actually handles —
// `f`, `u`, `d`, `o` — and nothing else. `o` (open in browser) is present:
// list.go now wires a real "o" case (openInBrowser, decision 3), so
// advertising it here matches the pane's actual behaviour rather than
// promising a binding that does not exist. `enter`/`esc` remain absent:
// EnterDetail is still a no-op stub.
func TestModel_HelpModal_NotificationsSection_PresentWhenEnabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 200
	m.height = 60
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()
	section := notificationsHelpSection(t, view)

	for _, want := range []string{"f", "Cycle reason filter", "u", "Mark read", "d", "Mark done", "o", "Open in browser"} {
		if !strings.Contains(section, want) {
			t.Errorf("Notifications tab section should contain %q, section:\n%s", want, section)
		}
	}
	if strings.Contains(section, "enter") || strings.Contains(section, "esc") {
		t.Errorf("Notifications tab section should not advertise 'enter'/'esc' — EnterDetail is a no-op stub, section:\n%s", section)
	}
}

// TestModel_HelpModal_NotificationsSection_AbsentWhenPaneDisabled confirms the
// section is removed (never added at all, in this implementation) when
// disabled_panes lists notifications, mirroring how the tab itself disappears
// (TestModel_NotificationsTab_Absent_WhenPaneDisabled_ButCapable).
func TestModel_HelpModal_NotificationsSection_AbsentWhenPaneDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 200
	m.height = 60
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "Notifications tab") {
		t.Errorf("help modal should not contain a 'Notifications tab' section when disabled_panes lists notifications, view:\n%s", view)
	}
}

// TestModel_HelpModal_NotificationsSection_AbsentWhenIncapable confirms the
// section is removed when no configured backend implements
// provider.NotificationSource (Decision 11), using the capable-*shaped*
// Azure-only fixture per Decision 59 rather than a nil provider.
func TestModel_HelpModal_NotificationsSection_AbsentWhenIncapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationIncapableProvider(), client, cfg, "dev", "")
	m.width = 200
	m.height = 60
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "Notifications tab") {
		t.Errorf("help modal should not contain a 'Notifications tab' section for an incapable provider, view:\n%s", view)
	}
}

// TestModel_HelpModal_FLine_SurvivesWorkItemsDisabled pins the substring
// collision task 17 was told to check: RemoveBindingsByDescription("work
// items") / ("work item") strips the Actions section's `T`/`s` lines when
// workitems is disabled, and — before this test — would also have deleted the
// global `f` line if its parenthetical scope had used the literal words "work
// items" (decision 70's first-proposed wording). The wording actually shipped
// ("work-items", hyphenated) contains neither substring, so the line must
// still be present.
func TestModel_HelpModal_FLine_SurvivesWorkItemsDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"workitems"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 200
	m.height = 60
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	// Sanity: confirm the disable path actually ran and did remove a
	// work-items-scoped line, or this test would pass vacuously.
	if strings.Contains(view, "Filter by tag (work items)") {
		t.Fatalf("precondition failed: 'Filter by tag (work items)' should have been removed when workitems is disabled, view:\n%s", view)
	}

	if !strings.Contains(view, "Search / filter (PRs / work-items / pipeline runs)") {
		t.Errorf("the 'f' line should survive workitems being disabled, view:\n%s", view)
	}
}

// TestModel_HelpModal_FLine_SurvivesPipelinesDisabled is
// TestModel_HelpModal_FLine_SurvivesWorkItemsDisabled's twin for the
// "pipelines" substring: RemoveBindingsByDescription("pipelines") must not
// remove the `f` line, which is why its wording says "pipeline runs" rather
// than the bare plural "pipelines".
func TestModel_HelpModal_FLine_SurvivesPipelinesDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"pipelines"},
	}
	var client *azdevops.MultiClient

	m := NewModel(nil, client, cfg, "dev", "")
	m.width = 200
	m.height = 60
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(Model)

	view := m.View()

	// Sanity: confirm the disable path actually ran.
	if strings.Contains(view, "Filter by status (pipelines)") {
		t.Fatalf("precondition failed: 'Filter by status (pipelines)' should have been removed when pipelines is disabled, view:\n%s", view)
	}

	if !strings.Contains(view, "Search / filter (PRs / work-items / pipeline runs)") {
		t.Errorf("the 'f' line should survive pipelines being disabled, view:\n%s", view)
	}
}

// TestModel_NotificationsTab_KeyDelegatesToNotificationsView pins that, once
// notifications is the active tab, key messages actually reach
// notificationsView.Update rather than silently falling through a
// switch-on-activeTab's `default:` branch (which every other such switch in
// this file routes to pipelinesView). Pressing 'f' (decision 57's reason
// filter cycle) is used as the observable signal: it only advances
// notificationsView's own ReasonFilter() state, never pipelinesView's.
func TestModel_NotificationsTab_KeyDelegatesToNotificationsView(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 100
	m.height = 30
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	if m.activeTab != TabNotifications {
		t.Fatalf("expected default activeTab TabNotifications, got %d", m.activeTab)
	}

	// Seed a feed with a reason present, directly via the exported SetFeed
	// method (Update-message-based population is a later task's concern).
	m.notificationsView = m.notificationsView.SetFeed([]provider.Notification{
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"},
			Title:     "Something",
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Now(),
		},
	})
	if _, active := m.notificationsView.ReasonFilter(); active {
		t.Fatal("precondition failed: reason filter should start inactive")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)

	if reason, active := m.notificationsView.ReasonFilter(); !active || reason != provider.NotificationReasonMentioned {
		t.Errorf("after 'f' on notifications tab, ReasonFilter() = (%v, %v), want (Mentioned, true) — key was not delegated to notificationsView", reason, active)
	}
}

// TestModel_TabID_NotificationsRoundTripsThroughState confirms
// state.TabNotifications round-trips through state.yaml *on disk*: switching to
// the notifications tab persists it via the state store, the file that lands is
// reloadable by state.Load, and a fresh, equally-capable model restores onto the
// tab from that reloaded state (Decision 9).
//
// The disk hop is the point. store.Apply only mutates memory and schedules a
// debounced flushAsync, so asserting store.State() — as this test originally
// did — never serialises anything: renaming the on-disk TabID literal to
// something else passed. Flush() forces the write and state.Load(path) reads it
// back through the YAML marshal/unmarshal that the criterion ("round-trips
// through state.yaml") actually names.
//
// This is state.Store on a t.TempDir() path, never config.Config.Save(), so
// convention 17 is not at issue here.
func TestModel_TabID_NotificationsRoundTripsThroughState(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	statePath := filepath.Join(t.TempDir(), "state.yaml")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.SetDebounce(5 * time.Millisecond)

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.SetStateStore(store)
	m.width = 100
	m.height = 30

	// Move to PR tab, then back to notifications, so the persisted value is
	// genuinely written by the tab-switch path rather than being the
	// zero-value default already sitting in the store.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)

	if got := store.State().ActiveTab; got != state.TabNotifications {
		t.Fatalf("in-memory store ActiveTab = %v, want %v", got, state.TabNotifications)
	}

	// Force the debounced write, then read the file back from scratch.
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	reloaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("state.Load(%q) error = %v", statePath, err)
	}
	if reloaded.ActiveTab != state.TabNotifications {
		t.Fatalf("reloaded-from-disk ActiveTab = %q, want %q", reloaded.ActiveTab, state.TabNotifications)
	}
	// Pin the serialised literal itself: state.TabID is a plain string with no
	// load-time validation, so a renamed constant would round-trip happily
	// while silently invalidating every existing user's state.yaml.
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", statePath, err)
	}
	if !strings.Contains(string(raw), "active_tab: notifications") {
		t.Errorf("state.yaml should contain `active_tab: notifications`, got:\n%s", raw)
	}

	// A fresh model (simulating relaunch) with the same capability restores
	// onto the tab loaded from disk — not from the live in-memory store.
	fresh := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	fresh.width = 100
	fresh.height = 30
	fresh.ApplyState(reloaded)

	if fresh.activeTab != TabNotifications {
		t.Errorf("after ApplyState(reloaded), activeTab = %v, want TabNotifications", fresh.activeTab)
	}
}

// ─── Task 13: render states through the full app (decisions 17, 46, 57, 63) ─
//
// Decision 63 assigns app-level coverage to the empty-inbox, filter-empty and
// error states (TestModel_NotificationsTab_PresentButEmpty_WhenCapable above
// already pins the empty-inbox render); capability-unsupported is asserted
// pane-level only, in internal/ui/notifications, since nothing in production
// ever puts the real app into that state (see notifications.Model's
// SetCapabilityUnsupported doc comment).

// TestModel_NotificationsTab_FilterEmpty_DistinctFromEmptyInbox drives the
// real app through NewModel, seeds a feed, activates the `f` reason filter via
// a genuine key message (not SetFeed alone), then lands a refreshed feed with
// none of the filtered reason present — pinning that the full View() renders
// decision 63's filter-empty text, not the bare empty-inbox marker.
func TestModel_NotificationsTab_FilterEmpty_DistinctFromEmptyInbox(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 100
	m.height = 30
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	m.notificationsView = m.notificationsView.SetFeed([]provider.Notification{
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"},
			Title:     "A mention",
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Now(),
		},
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "2"},
			Title:     "Subscribed noise",
			Reason:    provider.NotificationReasonSubscribed,
			UpdatedAt: time.Now(),
		},
	})

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}) // -> Mentioned
	m = updated.(Model)
	if reason, active := m.notificationsView.ReasonFilter(); !active || reason != provider.NotificationReasonMentioned {
		t.Fatalf("precondition: ReasonFilter() = (%v, %v), want (Mentioned, true)", reason, active)
	}

	// A refresh lands with no Mentioned rows at all.
	m.notificationsView = m.notificationsView.SetFeed([]provider.Notification{
		{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "3"},
			Title:     "Subscribed noise",
			Reason:    provider.NotificationReasonSubscribed,
			UpdatedAt: time.Now(),
		},
	})

	view := m.View()
	if !strings.Contains(view, "No notifications match Filter:") {
		t.Errorf("expected the filter-empty render in the full app View(); view:\n%s", view)
	}
	if strings.Contains(view, "You're all caught up.") {
		t.Errorf("filter-empty state rendered the empty-inbox text instead; view:\n%s", view)
	}
}

// TestModel_NotificationsTab_Error_RendersThroughFullView drives the real app
// through NewModel and lands a fetch failure via notificationsView's exported
// HandleFetchResult, using the real error internal/github's Adapter returns
// with no NotificationsClient configured — pinning that the full app View()
// renders decision 63's error state and the adapter's real nil-client
// message, and not the empty-inbox text.
//
// Task 19 recovers *github.APIError via errors.As to differentiate a 403
// missing-scope response from other failures; the adapter's nil-client error
// here is a plain fmt.Errorf, not a *github.APIError, so it falls into the
// generic branch, which must NOT carry decision 17's scope banner — showing
// it here would misleadingly blame a missing scope for what is actually a
// wiring gap. See internal/ui/notifications/list_test.go's own
// TestView_Error_Generic_CarriesNilClientMessage_ButNotScopeBanner for the
// pane-level twin of this same assertion.
func TestModel_NotificationsTab_Error_RendersThroughFullView(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 100
	m.height = 30
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	adapter := github.NewAdapterWithNotifications(nil, nil)
	_, listErr := adapter.List(provider.NotifOpts{})
	if listErr == nil {
		t.Fatal("precondition: github.Adapter.List with no NotificationsClient must return an error")
	}

	m.notificationsView = m.notificationsView.HandleFetchResult(nil, listErr)

	view := m.View()
	if !strings.Contains(view, "Notifications unavailable:") {
		t.Errorf("expected the error render in the full app View(); view:\n%s", view)
	}
	if !strings.Contains(view, "no notifications client configured") {
		t.Errorf("expected the adapter's real nil-client message in the full app View(); view:\n%s", view)
	}
	if strings.Contains(view, "GitHub token scope required: notifications") {
		t.Errorf("expected NO scope banner for a generic/nil-client failure; view:\n%s", view)
	}
	if strings.Contains(view, "You're all caught up.") {
		t.Errorf("error state rendered the empty-inbox text instead; view:\n%s", view)
	}
}

// TestNotificationsTabContent_PopulatedWarnings_RendersBanner pins decision
// 46: a populated Config.Warnings renders ahead of the pane's own content.
func TestNotificationsTabContent_PopulatedWarnings_RendersBanner(t *testing.T) {
	got := notificationsTabContent("PANE BODY", []string{"some warning"})

	if !strings.Contains(got, "some warning") {
		t.Errorf("notificationsTabContent = %q, want to contain the warning text", got)
	}
	if !strings.Contains(got, "PANE BODY") {
		t.Errorf("notificationsTabContent = %q, want to still contain the pane body", got)
	}
	if strings.Index(got, "some warning") > strings.Index(got, "PANE BODY") {
		t.Errorf("notificationsTabContent = %q, want the warning banner ahead of the pane body", got)
	}
}

// TestNotificationsTabContent_EmptyWarnings_NoStrayBlankLine pins decision
// 46's other half: an empty (or nil) Warnings slice must never reserve a
// blank line ahead of the pane's content — the output must be exactly the
// pane body, unchanged.
func TestNotificationsTabContent_EmptyWarnings_NoStrayBlankLine(t *testing.T) {
	if got := notificationsTabContent("PANE BODY", nil); got != "PANE BODY" {
		t.Errorf("notificationsTabContent(nil warnings) = %q, want exactly %q (no stray blank line)", got, "PANE BODY")
	}
	if got := notificationsTabContent("PANE BODY", []string{}); got != "PANE BODY" {
		t.Errorf("notificationsTabContent(empty warnings) = %q, want exactly %q (no stray blank line)", got, "PANE BODY")
	}
}

// TestModel_NotificationsTab_Warnings_RenderInFullView pins decision 46 end to
// end: a Config populated with Warnings at construction renders the banner
// through the real app's View(), not merely through the pure helper above.
func TestModel_NotificationsTab_Warnings_RenderInFullView(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Warnings:        []string{"notifications.exclude_reasons: unrecognised value \"bogus\" ignored"},
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.width = 100
	m.height = 30
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "notifications.exclude_reasons") {
		t.Errorf("expected Config.Warnings to render in the notifications tab; view:\n%s", view)
	}
}

// --- Task 14 / decision 65: MarkResultMsg routing ------------------------

// appMarkerStub is a provider.NotificationSource for the app-level mark-routing
// tests. markErr is returned by both mutators so the *failure* path can be
// driven, which is what makes TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive
// clock-free — see its doc comment.
type appMarkerStub struct {
	markErr   error
	doneCalls []provider.Identity
}

func (s *appMarkerStub) List(provider.NotifOpts) ([]provider.Notification, error) {
	return nil, nil
}

func (s *appMarkerStub) MarkRead(provider.Identity) error { return s.markErr }

func (s *appMarkerStub) MarkDone(id provider.Identity) error {
	s.doneCalls = append(s.doneCalls, id)
	return s.markErr
}

// markRoutingModel builds a sized app model whose notifications pane holds one
// unread row wired to marker, and returns it with that row's title.
func markRoutingModel(t *testing.T, marker provider.NotificationSource) (Model, string) {
	t.Helper()
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	if m.activeTab != TabNotifications {
		t.Fatalf("activeTab = %d, want TabNotifications", m.activeTab)
	}

	const title = "ROUTE-ME-HOME"
	m.notificationsView = notifications.NewModelWithStyles(styles.DefaultStyles(), marker, cfg).
		SetFeed([]provider.Notification{{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
			Title:     title,
			Reason:    provider.NotificationReasonReviewRequested,
			UpdatedAt: time.Now(),
		}})

	// 130, not 120: task 17 added the `o` key to notificationsKeybindings(),
	// which no longer fits on a single footer line at 120 columns alongside
	// `u`/`d`'s mark-read/mark-done text.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	return updated.(Model), title
}

// TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive is the regression
// test for the 🔴 that survived decision 65's first fix: notifications.MarkResultMsg
// matched no case in the top-level switch, so it reached the pane only via
// `case TabNotifications:` in the delegate-to-active-view switch. A mark is
// issued from the notifications tab but its result lands one HTTP round trip
// later, and tab switching is handled earlier and returns early — so pressing 2
// mid-flight handed the result to the pull-requests pane, which discarded it.
//
// The override was then the sole holder of the mark, which is exactly the
// pre-decision-65 state: 30s later any re-derivation resurrects a row the server
// already accepted as done.
//
// The assertion uses the *failure* path deliberately. A dropped success and an
// applied success are indistinguishable inside the debounce window (both leave
// the row hidden), and the pane's clock seam is unexported so this package
// cannot advance it. A failure is different: routing it produces a visible
// rollback, so "the row came back" proves delivery with no clock involved.
func TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive(t *testing.T) {
	marker := &appMarkerStub{markErr: errors.New("403 missing scope")}
	m, title := markRoutingModel(t, marker)

	if !strings.Contains(m.View(), title) {
		t.Fatalf("precondition: want the seeded row %q to render; view:\n%s", title, m.View())
	}

	// d hides the row optimistically and returns the cmd carrying the API call.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("want a non-nil cmd from d")
	}
	if strings.Contains(m.View(), title) {
		t.Fatalf("precondition: want %q hidden immediately after d; view:\n%s", title, m.View())
	}

	// The user switches to another tab while the DELETE is still in flight.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)
	if m.activeTab == TabNotifications {
		t.Fatal("precondition: want a non-notifications tab active before the result lands")
	}

	// The API call resolves and bubbletea delivers the result.
	msg := cmd()
	if _, ok := msg.(notifications.MarkResultMsg); !ok {
		t.Fatalf("cmd produced %T, want notifications.MarkResultMsg", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if len(marker.doneCalls) != 1 {
		t.Fatalf("MarkDone calls = %d, want 1", len(marker.doneCalls))
	}

	// Back to notifications: the failed mark must have been rolled back, which
	// can only have happened if the result actually reached the pane.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)
	if m.activeTab != TabNotifications {
		t.Fatalf("activeTab = %d, want TabNotifications", m.activeTab)
	}

	if !strings.Contains(m.View(), title) {
		t.Errorf("row %q is still hidden after a FAILED MarkDone whose result landed on another tab — the result was routed to the wrong pane and discarded, leaving the override as the sole holder of the mark (decision 65); view:\n%s", title, m.View())
	}
}

// TestModel_View_OutputHeightMatchesTerminal_AfterMarkResultRollback pins the
// notifications.MarkResultMsg half of finding 1 of the task 16 independent
// review: a failed mark's rollback re-raises the unread count exactly the
// same way a poll can, and the handler had the same early return that
// skipped resizeActiveViewIfNeeded.
//
// At width 120 with a single seeded row: pressing 'd' hides it optimistically
// (the row falls out of applyOverrides entirely, so the badge disappears and
// the footer correctly shrinks by one row via the ordinary delegate-to-
// active-tab path). The failed MarkDone then rolls the override back,
// restoring the row and re-raising the count to 1 — and 120 sits in the band
// where that reappearance tips the footer back onto a second line, so a
// stale m.footerRows overflows the 40-row terminal by one line.
func TestModel_View_OutputHeightMatchesTerminal_AfterMarkResultRollback(t *testing.T) {
	marker := &appMarkerStub{markErr: errors.New("403 missing scope")}
	m, title := markRoutingModel(t, marker)

	const termHeight = 40
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("want a non-nil cmd from d")
	}
	if strings.Contains(m.View(), "unread") {
		t.Fatalf("precondition: badge should be gone right after the optimistic hide; view:\n%s", m.View())
	}
	if h := lipgloss.Height(m.View()); h != termHeight {
		t.Fatalf("precondition: View() rendered %d rows for a %d-row terminal right after the optimistic hide, want %d", h, termHeight, termHeight)
	}

	msg := cmd()
	if _, ok := msg.(notifications.MarkResultMsg); !ok {
		t.Fatalf("cmd produced %T, want notifications.MarkResultMsg", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, title) || !strings.Contains(view, "1 unread") {
		t.Fatalf("precondition: want the row restored and the badge back to '1 unread' after the failed MarkDone rolled back; view:\n%s", view)
	}
	if h := lipgloss.Height(view); h != termHeight {
		t.Errorf("after MarkResultMsg's failure rollback re-raised the unread count, View() rendered %d rows for a %d-row terminal (footerRows=%d) — the footer was not re-measured; view:\n%s",
			h, termHeight, m.footerRows, view)
	}
}

// TestModel_MarkResultMsg_FailureMessage_ReachesStatusBar_ThroughView pins
// task 19 Part C's decision-81 compliance: a failed mark's message must be
// observable through app.Model's own View(), not merely through the pane's
// GetStatusMessage() (which internal/ui/notifications/list_test.go already
// covers, but per decision 81 that field is not itself a render surface).
// This is what proves the new m.syncNotificationsActionMessage() call in the
// MarkResultMsg case is actually wired, not just present in the diff.
func TestModel_MarkResultMsg_FailureMessage_ReachesStatusBar_ThroughView(t *testing.T) {
	marker := &appMarkerStub{markErr: errors.New("403 missing scope")}
	m, title := markRoutingModel(t, marker)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("want a non-nil cmd from d")
	}

	msg := cmd()
	if _, ok := msg.(notifications.MarkResultMsg); !ok {
		t.Fatalf("cmd produced %T, want notifications.MarkResultMsg", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)

	view := dewrapFooterText(m.View())
	if !strings.Contains(view, "Mark done failed") || !strings.Contains(view, "403 missing scope") {
		t.Errorf("view after a failed MarkDone = %q, want it to name the action and the error via the status bar; row title %q", view, title)
	}
}

// TestModel_View_OutputHeightMatchesTerminal_AfterMarkFailureMessage pins
// decision 77's footer-remeasurement obligation for the new
// m.syncNotificationsActionMessage() call added to the MarkResultMsg case: it
// widens the status bar's warning message exactly like `o`'s own failure
// message does, so the handler must still leave m.footerRows in sync with
// what View() actually renders.
//
// Sweeps the same width band TestModel_View_OutputHeightMatchesTerminal_AfterOpenInBrowserFailure
// does, but this message's own width-to-wrap behavior was instrumented
// directly rather than assumed to match that test's: with
// resizeActiveViewIfNeeded's call removed from the MarkResultMsg branch
// (verified by temporarily deleting it and re-running this exact sweep before
// writing this test), 100/110/116/120 do NOT discriminate — the message's
// wrap happens to land on a footer-row count the stale m.footerRows already
// matched by coincidence at those widths — while 128/130/150 do, overflowing
// the terminal by one row when the resize call is missing. 130 doubles as
// markRoutingModel's own fixed construction width, so it is included even
// though a second, independently-built model is used per sub-test here for
// the wider band.
func TestModel_View_OutputHeightMatchesTerminal_AfterMarkFailureMessage(t *testing.T) {
	const termHeight = 40
	widths := []int{100, 110, 116, 120, 128, 130, 150}

	for _, width := range widths {
		width := width
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			marker := &appMarkerStub{markErr: errors.New("403 missing scope")}
			m, title := markRoutingModelAtWidth(t, marker, width)

			// Prime the pointer-mutation render; deliberately not asserted on
			// (mirrors TestModel_View_OutputHeightMatchesTerminal_AfterOpenInBrowserFailure's
			// own workaround for the same first-render-after-WindowSizeMsg gap).
			_ = m.View()
			if !strings.Contains(m.View(), title) {
				t.Fatalf("precondition: want the seeded row %q to render; view:\n%s", title, m.View())
			}

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
			m = updated.(Model)
			if cmd == nil {
				t.Fatal("want a non-nil cmd from d")
			}
			msg := cmd()
			updated, _ = m.Update(msg)
			m = updated.(Model)

			view := m.View()
			if !strings.Contains(dewrapFooterText(view), "Mark done failed") {
				t.Fatalf("precondition: want the failure message rendered before measuring height; view:\n%s", view)
			}
			if h := strings.Count(view, "\n") + 1; h != termHeight {
				t.Errorf("width=%d: View() rendered %d rows for a %d-row terminal after the mark-failure message appeared (footerRows=%d); view:\n%s",
					width, h, termHeight, m.footerRows, view)
			}
		})
	}
}

// markRoutingModelAtWidth is markRoutingModel with a caller-supplied width,
// so TestModel_View_OutputHeightMatchesTerminal_AfterMarkFailureMessage can
// sweep the footer width band without re-deriving the seeded row/marker
// wiring at each width.
func markRoutingModelAtWidth(t *testing.T, marker provider.NotificationSource, width int) (Model, string) {
	t.Helper()
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	const title = "ROUTE-ME-HOME"
	m.notificationsView = notifications.NewModelWithStyles(styles.DefaultStyles(), marker, cfg).
		SetFeed([]provider.Notification{{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
			Title:     title,
			Reason:    provider.NotificationReasonReviewRequested,
			UpdatedAt: time.Now(),
		}})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return updated.(Model), title
}

// TestModel_NotificationMarker_IsWiredToThePane closes the gap the task-14
// re-validation flagged: every other app-level notifications test passes a nil
// marker, so mutating notificationMarker to `return nil` left the whole suite
// green. This pins the wiring by observing a real API call reaching the marker.
//
// It goes through NewModel rather than asserting on notificationMarker directly,
// because the defect being guarded is the pane being built *without* the marker,
// not the helper computing the wrong value.
func TestModel_NotificationMarker_IsWiredToThePane(t *testing.T) {
	marker := &appMarkerStub{}
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	// A provider that IS the marker, so NewModel's own notificationMarker(p)
	// type assertion is what has to find it — not a hand-injected pane.
	m := NewModel(provider.NewCompositeProvider(github.NewAdapterWithNotifications(nil, nil)), client, cfg, "dev", "")
	if got := notificationMarker(m.client); got == nil {
		t.Fatal("notificationMarker returned nil for a capable provider — the pane would silently no-op on u/d")
	}

	assertPaneHasAMarker(t, m, "NewModel")

	// The marker stub is unused on the NewModel path on purpose: substituting a
	// pane built around it is exactly what made the earlier version of this
	// test vacuous. It stays only so the two nil-argument sites can be checked
	// by the same helper without it.
	_ = marker
}

// assertPaneHasAMarker drives `d` against the pane the Model already holds and
// requires the row to disappear. It never replaces m.notificationsView — the
// whole point is to exercise the pane that app *built*, so passing nil for
// notificationMarker(...) at a construction site is caught. Feeding the
// existing pane via SetFeed supplies data without discarding its marker.
//
// The discriminator is the row vanishing rather than the marker stub being
// called, because on this path the marker is whatever app resolved from the
// real provider and the test has no handle on it. markDone's nil-marker guard
// (internal/ui/notifications/list.go) returns `m, nil` before recording any
// override, so a markerless pane leaves the row on screen; a wired one hides
// it optimistically. Rendered through View() after a WindowSizeMsg per
// convention 8.
func assertPaneHasAMarker(t *testing.T, m Model, site string) {
	t.Helper()

	const title = "MARKER-WIRED"
	m.notificationsView = m.notificationsView.SetFeed([]provider.Notification{{
		Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
		Title:     title,
		Reason:    provider.NotificationReasonMentioned,
		UpdatedAt: time.Now(),
	}})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	if !strings.Contains(m.View(), title) {
		t.Fatalf("%s: row %q not rendered before d — fixture is wrong, not the wiring", site, title)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)

	if strings.Contains(m.View(), title) {
		t.Errorf("%s: row %q still visible after d — the pane was built with a nil marker, so u/d silently no-op", site, title)
	}
	if cmd == nil {
		t.Errorf("%s: d produced no cmd — the pane has no marker", site)
	}
}

// TestModel_ThemeChange_KeepsTheMarkerWiredToTheRebuiltPane covers the second
// construction site. ThemeSelectedMsg rebuilds notificationsView from scratch,
// so dropping the marker there kills u/d permanently after the user picks a
// theme — a state no test reached while only NewModel's site was covered.
func TestModel_ThemeChange_KeepsTheMarkerWiredToTheRebuiltPane(t *testing.T) {
	// NewWithPath + t.TempDir per convention 17, and here it is load-bearing
	// twice over. The handler's first statement is m.config.UpdateTheme, which
	// calls Save(); with a bare &config.Config{} literal configPath is "" and
	// Save() resolves GetPath() — the developer's real ~/.config/azdo-tui/
	// config.yaml. It also makes the test vacuous: Save() fails, the handler
	// takes its early `return m, nil`, and the pane rebuild below is never
	// reached, so the nil-marker mutation at that site survives. The sibling
	// TestModel_ThemeSwitch_* tests already do it this way.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(provider.NewCompositeProvider(github.NewAdapterWithNotifications(nil, nil)), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	assertPaneHasAMarker(t, m, "ThemeSelectedMsg")
}

// mutableHintProviderStub is a hinter whose hint can change between model
// construction and a later fetch. That is the whole point: NewModel computes
// max(configured, hint) up front too, so a stub with a constant hint makes the
// handler's recompute redundant and the wiring untestable. GitHub only sends
// X-Poll-Interval on an actual response, so 0-at-construction becoming a real
// value after the first fetch is the real sequence, not a contrived one.
type mutableHintProviderStub struct {
	provider.Provider
	hint time.Duration
}

func (h *mutableHintProviderStub) NotificationsPollInterval() time.Duration { return h.hint }

// TestModel_NotificationsFetchedMsg_AppliesTheNewPollIntervalHintToThePoller
// pins the single SetInterval call in the NotificationsFetchedMsg handler,
// which is the entirety of task 15's "cadence is max(X-Poll-Interval,
// configured)" criterion at the app level. Deleting that line leaves the
// configured interval in force forever while the hint is computed and thrown
// away — and the suite stayed green, because the pure interval-arithmetic
// functions are not what breaks.
//
// Asserted through polling.NotificationsPoller.Interval(), added as a test
// seam for exactly this. The hint must be raised *after* NewModel has run, or
// construction has already applied it and the assertion passes with the
// handler's call deleted.
func TestModel_NotificationsFetchedMsg_AppliesTheNewPollIntervalHintToThePoller(t *testing.T) {
	const configured = 30
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: configured,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	// No hint yet: GitHub has sent no response, so the poller starts on the
	// configured cadence. The real capable provider is embedded rather than a
	// nil interface because NewModel calls displayScopes(p, cfg) -> p.Scopes().
	p := &mutableHintProviderStub{Provider: newNotificationCapableProvider(), hint: 0}
	m := NewModel(p, client, cfg, "dev", "")

	if got, want := m.notificationsPoller.Interval(), configured*time.Second; got != want {
		t.Fatalf("precondition: interval at construction = %v, want the configured %v", got, want)
	}

	// The first response carries X-Poll-Interval: 300, which the adapter now
	// reports. GitHub asking us to slow down must win over the configured 30s.
	p.hint = 300 * time.Second

	// The result is deliberately a real, non-empty one rather than the nil/nil
	// shape this test used to dispatch. A guard on nil/nil briefly lived in this
	// handler, and while it stood this test silently migrated to pinning that
	// guard's branch instead of the branch a live GitHub response takes, leaving
	// the headline cadence criterion unobserved on the only path that matters.
	updated, _ := m.Update(polling.NotificationsFetchedMsg{
		Items: []provider.Notification{{
			Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "hint-1"},
			Title:     "A real fetch result",
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Now(),
		}},
	})
	m = updated.(Model)

	if got, want := m.notificationsPoller.Interval(), 300*time.Second; got != want {
		t.Errorf("interval after a fetch that raised the hint = %v, want %v — the hint is being computed and discarded, so GitHub's rate-limit request is ignored", got, want)
	}
}

// TestModel_NotificationsFetchedMsg_EmptyInbox_ReDerivesOptsFromConfig pins
// Decision 75 for the nil,nil ("nothing changed") branch: NotifOpts must be
// re-derived once per fetch, not frozen at poller construction (which is
// what NewModel's own notifications.NotifOptsFromConfig(cfg) call would
// otherwise leave in place forever). cfg.Notifications.MaxItems is mutated
// through the same *config.Config pointer m.config aliases *after*
// construction, so a poller whose opts were only ever set once at startup
// would still report the old value here; only the handler's own SetOpts
// call turns the config change into an observable difference.
func TestModel_NotificationsFetchedMsg_EmptyInbox_ReDerivesOptsFromConfig(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Notifications:   config.NotificationsConfig{MaxItems: 10},
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	if got, want := m.notificationsPoller.Opts().Max, 10; got != want {
		t.Fatalf("precondition: opts.Max at construction = %d, want %d", got, want)
	}

	cfg.Notifications.MaxItems = 99

	updated, _ := m.Update(polling.NotificationsFetchedMsg{Items: nil, Err: nil})
	m = updated.(Model)

	if got, want := m.notificationsPoller.Opts().Max, 99; got != want {
		t.Errorf("opts.Max after a nil,nil fetch following a config change = %d, want %d — SetOpts must be called alongside SetInterval in the nil,nil branch too", got, want)
	}
}

// TestModel_NotificationsFetchedMsg_RealResult_ReDerivesOptsFromConfig is
// TestModel_NotificationsFetchedMsg_EmptyInbox_ReDerivesOptsFromConfig's sibling
// for the ordinary (non-nil,nil) branch, which has its own separate SetOpts
// call — deleting that one leaves the nil,nil branch's call intact and the
// sibling test above green, so this needs its own, independent assertion.
func TestModel_NotificationsFetchedMsg_RealResult_ReDerivesOptsFromConfig(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		Notifications:   config.NotificationsConfig{MaxItems: 10},
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	if got, want := m.notificationsPoller.Opts().Max, 10; got != want {
		t.Fatalf("precondition: opts.Max at construction = %d, want %d", got, want)
	}

	cfg.Notifications.MaxItems = 99

	updated, _ := m.Update(polling.NotificationsFetchedMsg{
		Items: []provider.Notification{
			{
				Identity:  provider.Identity{Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo", ID: "1"},
				Title:     "Real result",
				Reason:    provider.NotificationReasonMentioned,
				UpdatedAt: time.Now(),
			},
		},
	})
	m = updated.(Model)

	if got, want := m.notificationsPoller.Opts().Max, 99; got != want {
		t.Errorf("opts.Max after a real fetch result following a config change = %d, want %d — SetOpts must be called alongside SetInterval in the ordinary branch too", got, want)
	}
}

// TestModel_QuitKey_StopsNotificationsPollerToo pins the review fix: quitting
// must stop both pollers, not just the pipeline one. Before this fix,
// m.notificationsPoller kept running (IsStopped() stayed false forever),
// which is invisible in practice only because the whole process exits
// straight after — but any in-flight cmd (e.g. a fetch already scheduled)
// would still complete and could still call back into a torn-down model in
// a longer-lived host.
func TestModel_QuitKey_StopsNotificationsPollerToo(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if m.notificationsPoller.IsStopped() {
		t.Fatal("precondition: notificationsPoller must not be stopped yet")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("want tea.Quit cmd from q")
	}

	if !m.notificationsPoller.IsStopped() {
		t.Error("notificationsPoller.IsStopped() = false after q, want true — quitting must stop both pollers")
	}
}

// TestModel_NotificationsKeybindings_IncludesMarkReadAndMarkDone pins the
// review fix: task 14 already ships the `u` (mark read) and `d` (mark done)
// keys in the pane's own Update switch, but the status bar's
// notificationsKeybindings() never listed them, so the pane's own keys were
// undiscoverable from the chrome every other tab uses for this.
func TestModel_NotificationsKeybindings_IncludesMarkReadAndMarkDone(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.activeTab = TabNotifications
	m.resizeActiveViewIfNeeded()

	view := m.View()
	if !strings.Contains(view, "u mark read") {
		t.Errorf("notifications tab view = %q, want the u mark-read keybinding listed", view)
	}
	if !strings.Contains(view, "d mark done") {
		t.Errorf("notifications tab view = %q, want the d mark-done keybinding listed", view)
	}
}

// ─── Init()'s notifications-poller-timer gate (mutation survivors S2, S3) ──

// TestModel_NotificationsStartPollingCmd_EnabledAndCapable_ArmsTimer and
// TestModel_NotificationsStartPollingCmd_DisabledOrIncapable_ReturnsNil pin
// the fix-1 gate as a pair, distinguishing polling.NotificationsTickMsg from
// polling.TickMsg via NotificationsPoller's every seam (SetEveryForTesting) so
// the arm case can be resolved without waiting on a real tea.Every timer or
// invoking any other, unsafe leaf of Init()'s batch.

func TestModel_NotificationsStartPollingCmd_EnabledAndCapable_ArmsTimer(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	if !m.isTabEnabled(TabNotifications) {
		t.Fatal("precondition: notifications tab must be enabled+capable")
	}

	var everyCalled bool
	m.notificationsPoller.SetEveryForTesting(func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		everyCalled = true
		return func() tea.Msg { return fn(time.Now()) }
	})

	cmd := m.notificationsStartPollingCmd()
	if cmd == nil {
		t.Fatal("want a non-nil cmd when the notifications tab is enabled and capable")
	}

	msg := cmd()
	if !everyCalled {
		t.Fatal("want the injected every seam to have been invoked")
	}
	if _, ok := msg.(polling.NotificationsTickMsg); !ok {
		t.Errorf("notificationsStartPollingCmd() resolved to %T, want polling.NotificationsTickMsg", msg)
	}
	if _, ok := msg.(polling.TickMsg); ok {
		t.Fatal("notificationsStartPollingCmd() must never resolve to polling.TickMsg (Decision 69)")
	}
}

func TestModel_NotificationsStartPollingCmd_DisabledOrIncapable_ReturnsNil(t *testing.T) {
	tests := []struct {
		name string
		p    provider.Provider
		cfg  *config.Config
	}{
		{
			name: "incapable provider",
			p:    newNotificationIncapableProvider(),
			cfg: &config.Config{
				Organization:    "testorg",
				Projects:        []string{"testproject"},
				PollingInterval: 60,
				Theme:           "dark",
			},
		},
		{
			name: "capable provider, pane disabled",
			p:    newNotificationCapableProvider(),
			cfg: &config.Config{
				Organization:    "testorg",
				Projects:        []string{"testproject"},
				PollingInterval: 60,
				Theme:           "dark",
				DisabledPanes:   []string{"notifications"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *azdevops.MultiClient
			m := NewModel(tt.p, client, tt.cfg, "dev", "")
			if m.isTabEnabled(TabNotifications) {
				t.Fatal("precondition: notifications tab must be absent")
			}

			cmd := m.notificationsStartPollingCmd()
			if cmd != nil {
				t.Error("notificationsStartPollingCmd() must return nil when the notifications tab is disabled or incapable")
			}
		})
	}
}

// batchLen returns the number of leaves cmd's tea.BatchMsg holds without
// invoking any of them, by resolving cmd exactly one level. bubbletea's own
// tea.Batch (see compactCmds in commands.go) never calls any sub-cmd to
// build the BatchMsg it returns, so this one call is always safe — even
// when, as in Init()'s own batch, some leaves are unresolvable real timers
// or real network calls. A single non-nil cmd (len(validCmds) == 1) is
// returned directly rather than wrapped in a BatchMsg, which is why this
// helper is only used where the batch is known to hold more than one
// command (Init()'s batch always does).
func batchLen(cmd tea.Cmd) int {
	if cmd == nil {
		return 0
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		return len(batch)
	}
	return 1
}

// TestModel_Init_NotificationsTimerGate_WiredIn pins that Init() actually
// includes notificationsStartPollingCmd()'s result: comparing the batch
// length between an enabled+capable model and a disabled one isolates
// exactly the one extra (or missing) cmd fix 1 gates, without invoking any
// other leaf of Init()'s batch (m.poller.StartPolling() is a real,
// un-gated tea.Every timer, and checkForUpdate performs real network I/O —
// both are out of scope here and unsafe to resolve).
//
// activeTab is forced to TabNotifications on both models before Init() runs
// so Init()'s separate notifications preload append (`m.activeTab !=
// TabNotifications`, gated by the same isTabEnabled predicate but pinned
// independently by TestModel_Init_PreloadsNotifications_WhenNotActiveTab)
// evaluates false either way — isolating the diff to exactly the timer-arm
// cmd this test targets. Forcing activeTab is safe here: batchLen only
// resolves Init()'s outer tea.Batch one level (per its own doc comment), so
// whatever m.initTabCmd(TabNotifications) itself returns is never invoked.
func TestModel_Init_NotificationsTimerGate_WiredIn(t *testing.T) {
	var client *azdevops.MultiClient

	enabledCfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	mEnabled := NewModel(newNotificationCapableProvider(), client, enabledCfg, "dev", "")
	if !mEnabled.isTabEnabled(TabNotifications) {
		t.Fatal("precondition: notifications tab must be enabled+capable")
	}
	mEnabled.activeTab = TabNotifications

	disabledCfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}
	mDisabled := NewModel(newNotificationCapableProvider(), client, disabledCfg, "dev", "")
	if mDisabled.isTabEnabled(TabNotifications) {
		t.Fatal("precondition: notifications tab must be disabled")
	}
	mDisabled.activeTab = TabNotifications

	gotEnabled := batchLen(mEnabled.Init())
	gotDisabled := batchLen(mDisabled.Init())

	if gotEnabled != gotDisabled+1 {
		t.Errorf("Init() batch length enabled=%d disabled=%d, want exactly one more cmd when the notifications tab is enabled+capable (notificationsStartPollingCmd wired in)", gotEnabled, gotDisabled)
	}
}

// TestModel_Init_PreloadsNotifications_WhenNotActiveTab pins mutation
// survivor S3: Init()'s notifications preload
// (m.notificationsPoller.FetchNotifications()) fires even when notifications
// is not the active tab, so switching to it later is instant. m.activeTab is
// forced to TabPullRequests before Init() runs (mirroring the same
// direct-field-assignment trick other tests in this file already use to force
// a scenario), which guarantees FetchNotifications()'s cmd is the very last
// non-nil entry appended to initCmds — the one safe leaf to invoke, since
// every earlier entry may be a real timer or real network call.
func TestModel_Init_PreloadsNotifications_WhenNotActiveTab(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m.activeTab = TabPullRequests
	if !m.isTabEnabled(TabNotifications) {
		t.Fatal("precondition: notifications tab must be enabled+capable")
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("want a non-nil cmd from Init()")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatal("want Init() to resolve to a non-empty tea.BatchMsg")
	}

	last := batch[len(batch)-1]
	got := last()
	if _, ok := got.(polling.NotificationsFetchedMsg); !ok {
		t.Errorf("Init()'s last batch entry resolved to %T, want polling.NotificationsFetchedMsg — the notifications preload must still fire when notifications is not the active tab", got)
	}
}

// ─── task 16: unread-count footer badge (decision 21, decision 68) ─────────

// seedUnreadNotifications wires n unread rows into m's notifications pane,
// wholly replacing its feed via SetFeed (the pane's marker, if any, is left
// intact).
func seedUnreadNotifications(m Model, n int) Model {
	feed := make([]provider.Notification, 0, n)
	for i := 0; i < n; i++ {
		feed = append(feed, provider.Notification{
			Identity: provider.Identity{
				Kind: provider.KindGitHub, Scope: "owner/repo", ScopeDisplay: "owner/repo",
				ID: fmt.Sprintf("%d", i+1),
			},
			Title:     fmt.Sprintf("Row %d", i+1),
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Now(),
		})
	}
	m.notificationsView = m.notificationsView.SetFeed(feed)
	return m
}

// TestModel_UnreadBadge_VisibleFromNotificationsTab is the base case: with
// the notifications tab active (Decision 6's default landing tab) and two
// unread rows seeded, the footer shows the badge.
func TestModel_UnreadBadge_VisibleFromNotificationsTab(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient
	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	if m.activeTab != TabNotifications {
		t.Fatalf("precondition: activeTab = %d, want TabNotifications", m.activeTab)
	}
	m = seedUnreadNotifications(m, 2)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "2 unread") {
		t.Errorf("view should contain the unread badge '2 unread' while on the notifications tab; view:\n%s", view)
	}
}

// TestModel_UnreadBadge_VisibleFromNonNotificationsTab pins decision 21's
// "visible from every tab" half: the badge must still render in the footer
// after switching away to another tab, since the footer is shared across all
// tabs rather than being part of notificationsKeybindings (which is per-tab
// and never runs for another active tab).
func TestModel_UnreadBadge_VisibleFromNonNotificationsTab(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient
	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m = seedUnreadNotifications(m, 3)

	// Wide enough that the PR tab's longer keybindings line plus the badge,
	// org and connection state do not wrap onto a second line — this test
	// asserts presence of the badge text, not the footer's line-wrap
	// behaviour under width pressure.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	m = updated.(Model)

	// Switch away from notifications (key "2" is the tab immediately after
	// notifications in enabledTabs — PullRequests, since notifications is
	// always enabledTabs[0] per Decision 6).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(Model)
	if m.activeTab == TabNotifications {
		t.Fatal("precondition: want a non-notifications tab active")
	}

	view := m.View()
	if !strings.Contains(view, "3 unread") {
		t.Errorf("view should contain the unread badge '3 unread' even while a different tab (%d) is active; view:\n%s", m.activeTab, view)
	}
}

// TestModel_UnreadBadge_AbsentWhenZero pins decision 21's other half: hidden
// entirely at zero, asserted as the absence of the badge's own word rather
// than merely the digit "0" (a stray "0 unread" residue would still fail the
// literal criterion even though it contains no bare "0" check).
func TestModel_UnreadBadge_AbsentWhenZero(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient
	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m = seedUnreadNotifications(m, 0)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "unread") {
		t.Errorf("view should NOT contain the unread badge when the count is 0; view:\n%s", view)
	}
}

// TestModel_UnreadBadge_AbsentWhenPaneDisabled pins the capability/pane-
// disabled gate: even with a capable provider and an unread row seeded
// directly into the (unconstructed-into-a-tab) pane, disabling the pane via
// config must hide the badge, exactly like it hides the tab itself.
func TestModel_UnreadBadge_AbsentWhenPaneDisabled(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}
	var client *azdevops.MultiClient
	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	if m.isTabEnabled(TabNotifications) {
		t.Fatal("precondition: TabNotifications should be disabled")
	}
	m = seedUnreadNotifications(m, 5)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "unread") {
		t.Errorf("view should NOT contain the unread badge when the notifications pane is disabled; view:\n%s", view)
	}
}

// TestModel_UnreadBadge_ClearedWhenThePaneIsDisabledAfterRendering pins the
// `else { SetUnreadCount(0) }` half of syncStatusBarContext's gate, which the
// two tests above cannot reach: both build a model whose pane is already
// absent, so the status bar's count is still at its zero value and deleting
// the else branch changes nothing. Measured — with that branch removed the
// whole suite stayed green.
//
// The count lives on the status bar, not the pane, so once a non-zero value
// has been pushed it persists until something overwrites it. Disabling the
// pane must therefore actively clear it, not merely stop refreshing it.
//
// The branch is dead today (enabledTabs is fixed at construction) but is
// about to become live: task 19's in-view disable action writes
// disabled_panes through Config.Save(). Pinning it now is Decision 64's
// shape — a currently-dead guard must still be pinned, or it will be
// "simplified" away exactly when it starts to matter.
func TestModel_UnreadBadge_ClearedWhenThePaneIsDisabledAfterRendering(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient
	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	m = seedUnreadNotifications(m, 5)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if view := m.View(); !strings.Contains(view, "unread") {
		t.Fatalf("precondition: want the badge rendered while the pane is enabled; view:\n%s", view)
	}

	// Disable the pane the way task 19's action will: drop it from the
	// enabled set. The stale count is still sitting on the status bar.
	filtered := make([]Tab, 0, len(m.enabledTabs))
	for _, tab := range m.enabledTabs {
		if tab != TabNotifications {
			filtered = append(filtered, tab)
		}
	}
	m.enabledTabs = filtered
	m.activeTab = TabPullRequests

	if view := m.View(); strings.Contains(view, "unread") {
		t.Errorf("want the badge gone once the pane is disabled — a stale count must be cleared, not merely left unrefreshed; view:\n%s", view)
	}
}

// TestModel_UnreadBadge_AbsentWhenProviderIncapable mirrors the disabled-pane
// case for the other half of isTabEnabled's predicate: an Azure-only
// provider that implements no notifications capability at all.
func TestModel_UnreadBadge_AbsentWhenProviderIncapable(t *testing.T) {
	cfg := &config.Config{
		Organization:    "testorg",
		Projects:        []string{"testproject"},
		PollingInterval: 60,
		Theme:           "dark",
	}
	var client *azdevops.MultiClient
	m := NewModel(newNotificationIncapableProvider(), client, cfg, "dev", "")
	if m.isTabEnabled(TabNotifications) {
		t.Fatal("precondition: TabNotifications should be absent for an incapable provider")
	}
	m = seedUnreadNotifications(m, 5)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "unread") {
		t.Errorf("view should NOT contain the unread badge when the provider implements no notifications capability; view:\n%s", view)
	}
}

// TestModel_UnreadBadge_ReflectsConfigFilteredCount pins decision 21's
// headline criterion — "count is unread *after config filters*" — with an
// actual assertion (task 16 independent-review finding 4). Every other
// app-level badge test seeds the pane by direct assignment
// (seedUnreadNotifications -> SetFeed), which bypasses the production route
// entirely: polling.NotificationsFetchedMsg -> notifications.FilterNotifications
// (msg.Items, m.config) -> HandleFetchResult. So the post-filter claim rested
// on prose alone until now.
//
// exclude_reasons drops the four "subscribed" rows, leaving 3 unread; the
// badge must show "3 unread", never the raw pre-filter "7 unread". This test
// would also have caught finding 1 (it routes through the same handler the
// fix touches), which the reviewer flagged as a good sign it targets the
// right seam.
func TestModel_UnreadBadge_ReflectsConfigFilteredCount(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	cfg.Notifications.ExcludeReasons = []string{"subscribed"}

	var client *azdevops.MultiClient
	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	items := make([]provider.Notification, 0, 7)
	for i := 0; i < 3; i++ {
		items = append(items, provider.Notification{
			Identity: provider.Identity{
				Kind: provider.KindGitHub, Scope: "o/r", ScopeDisplay: "o/r",
				ID: fmt.Sprintf("mentioned-%d", i),
			},
			Title:     fmt.Sprintf("mentioned %d", i),
			Reason:    provider.NotificationReasonMentioned,
			UpdatedAt: time.Now(),
		})
	}
	for i := 0; i < 4; i++ {
		items = append(items, provider.Notification{
			Identity: provider.Identity{
				Kind: provider.KindGitHub, Scope: "o/r", ScopeDisplay: "o/r",
				ID: fmt.Sprintf("subscribed-%d", i),
			},
			Title:     fmt.Sprintf("subscribed %d", i),
			Reason:    provider.NotificationReasonSubscribed,
			UpdatedAt: time.Now(),
		})
	}

	updated, _ = m.Update(polling.NotificationsFetchedMsg{Items: items, Err: nil})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "7 unread") {
		t.Fatalf("badge shows the raw pre-filter count '7 unread'; exclude_reasons was not applied before counting; view:\n%s", view)
	}
	if !strings.Contains(view, "3 unread") {
		t.Errorf("badge should show '3 unread' (post-filter: exclude_reasons drops the 4 'subscribed' rows); view:\n%s", view)
	}
}

// TestModel_UnreadBadge_ResetToZeroOnThemeChange pins the current, deliberate
// behaviour (task 16 independent-review finding 5, Oscar's decision): a
// theme change rebuilds m.notificationsView from scratch (app.go's
// ThemeSelectedMsg handler), which drops the fed-in feed and, with it, the
// badge count — visible from every tab until the next NotificationsTickMsg
// repopulates it.
//
// The count is deliberately NOT carried across the rebuild. Carrying the
// count without also carrying the feed would let the badge and the pane
// disagree (a "4 unread" badge over an empty inbox), which is worse than
// both being momentarily empty; the reset is transient and self-heals on the
// next poll. This test exists to make that a recorded choice, not an
// accident a future refactor silently reverses.
//
// Uses config.NewWithPath, never a bare &config.Config{} (convention 17):
// ThemeSelectedMsg reaches Config.Save(), and Save() resolves a bare
// literal's GetPath() to the developer's real ~/.config/azdo-tui/config.yaml
// -- this already happened once and overwrote a live config. A bare literal
// is also the *unobservant* choice here specifically: Save() would fail
// against a path that can't be created, the handler would early-return
// before ever reaching the rebuild this test means to exercise, and the
// test would pass for the wrong reason.
func TestModel_UnreadBadge_ResetToZeroOnThemeChange(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.NewWithPath("testorg", []string{"testproject"}, 60, "dark", cfgPath)
	var client *azdevops.MultiClient

	m := NewModel(newNotificationCapableProvider(), client, cfg, "dev", "")
	if m.activeTab != TabNotifications {
		t.Fatalf("precondition: activeTab = %d, want TabNotifications", m.activeTab)
	}
	m = seedUnreadNotifications(m, 4)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if view := m.View(); !strings.Contains(view, "4 unread") {
		t.Fatalf("precondition: want the badge showing '4 unread' before the theme switch; view:\n%s", view)
	}

	updated, _ = m.Update(components.ThemeSelectedMsg{ThemeName: "catppuccin"})
	m = updated.(Model)

	if got := m.notificationsView.UnreadCount(); got != 0 {
		t.Errorf("notificationsView.UnreadCount() = %d after theme switch, want 0 — the rebuilt pane should start with an empty feed", got)
	}
	if view := m.View(); strings.Contains(view, "unread") {
		t.Errorf("want the badge gone immediately after a theme switch (until the next poll repopulates it); view:\n%s", view)
	}
}
