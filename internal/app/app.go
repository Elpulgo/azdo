package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/state"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/metrics"
	"github.com/Elpulgo/azdo/internal/ui/pipelines"
	"github.com/Elpulgo/azdo/internal/ui/pullrequests"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/Elpulgo/azdo/internal/ui/workitems"
	"github.com/Elpulgo/azdo/internal/version"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ThemeNotFoundError represents an error when a requested theme is not found.
type ThemeNotFoundError struct {
	ThemeName  string
	ThemesPath string
}

func (e *ThemeNotFoundError) Error() string {
	defaultTheme := styles.GetDefaultTheme()
	return fmt.Sprintf("Theme '%s' not found, using '%s'. Press 't' to select a theme.",
		e.ThemeName, defaultTheme.Name)
}

// Tab represents the active tab in the application
type Tab int

const (
	TabPullRequests Tab = iota // Pull Requests tab (key '1')
	TabWorkItems               // Work Items tab (key '2')
	TabPipelines               // Pipelines tab (key '3')
	TabMetrics                 // Metrics dashboard tab (opt-in via metrics.enabled)
)

// Layout constants for the bordered content area.
const (
	// borderWidth is the horizontal space consumed by box side borders (left + right).
	borderWidth = 2

	// boxBorderRows is the vertical space consumed by the box border itself:
	// top border (1) + bottom border (1) = 2.
	boxBorderRows = 2

	// tabBarRows is the vertical space consumed by the bordered tab bar
	// (which includes the logo): top border (1) + 3 content rows + bottom border (1) = 5.
	tabBarRows = 5
)

// updateCheckMsg is sent when the background version check completes.
type updateCheckMsg struct {
	info *version.UpdateInfo
}

// Model is the root application model for the TUI
type Model struct {
	// client is the backend-neutral provider used by the three main views
	// (pull requests, work items, pipelines). Stored now; views migrate to
	// consume it in tasks 7-9.
	client provider.Provider

	// metricsClient is the concrete Azure DevOps client kept for the metrics
	// view, which calls Azure-only methods not covered by provider.Provider
	// (Decision 5). Nullable: nil when metrics is disabled or the concrete
	// client was not supplied.
	metricsClient *azdevops.MultiClient

	config           *config.Config
	styles           *styles.Styles
	activeTab        Tab
	enabledTabs      []Tab // ordered list of enabled tabs
	pipelinesView    pipelines.Model
	pullRequestsView pullrequests.Model
	workItemsView    workitems.Model
	metricsView      metrics.Model
	logo             *components.Logo
	statusBar        *components.StatusBar
	helpModal        *components.HelpModal
	errorModal       *components.ErrorModal
	themePicker      components.ThemePicker
	poller           *polling.Poller
	errorHandler     *polling.ErrorHandler
	currentVersion   string
	commitHash       string
	width            int
	height           int
	footerRows       int
	err              error
	stateStore       *state.Store // optional; nil when persistence is disabled
}

// SetStateStore attaches a state store to the model so navigation changes
// (active tab, PR / work-item detail) get persisted between runs. Wired
// up by cmd/azdo-tui; tests may omit it.
func (m *Model) SetStateStore(s *state.Store) {
	m.stateStore = s
}

// tabIDForTab maps the internal Tab iota to the on-disk TabID.
func tabIDForTab(t Tab) state.TabID {
	switch t {
	case TabPullRequests:
		return state.TabPullRequests
	case TabWorkItems:
		return state.TabWorkItems
	case TabPipelines:
		return state.TabPipelines
	}
	return ""
}

// tabFromID resolves a persisted TabID back to the internal Tab iota.
// Unknown IDs return (0, false) so the caller can fall back gracefully.
func tabFromID(id state.TabID) (Tab, bool) {
	switch id {
	case state.TabPullRequests:
		return TabPullRequests, true
	case state.TabWorkItems:
		return TabWorkItems, true
	case state.TabPipelines:
		return TabPipelines, true
	}
	return 0, false
}

// ApplyState seeds the model from persisted state at startup: it sets the
// active tab (when enabled and recognised) and queues per-tab pending
// detail restores on the sub-models. Detail restores fire on the first
// list populate; if the persisted ID isn't found the restore silently
// no-ops — the user lands on the list, never in a broken state.
func (m *Model) ApplyState(s state.State) {
	if t, ok := tabFromID(s.ActiveTab); ok && m.isTabEnabled(t) {
		m.activeTab = t
	}
	if id := s.Tabs.PullRequests.LastDetailID; id != 0 {
		m.pullRequestsView = m.pullRequestsView.WithPendingDetailRestore(id)
	}
	if id := s.Tabs.WorkItems.LastDetailID; id != 0 {
		m.workItemsView = m.workItemsView.WithPendingDetailRestore(id)
	}
}

// recordActiveTab is a no-op when no store is attached.
func (m Model) recordActiveTab() {
	if m.stateStore == nil {
		return
	}
	id := tabIDForTab(m.activeTab)
	m.stateStore.Apply(func(s *state.State) {
		s.Version = state.CurrentVersion
		s.ActiveTab = id
	})
}

// recordDetailState captures the currently open detail (if any) for the
// active tab into the persistent state. Called after delegating to a
// sub-model in Update, so the snapshot reflects the post-update view mode.
// Only PR and Work Items tabs participate — Pipelines detail is not
// restorable by design.
func (m Model) recordDetailState() {
	if m.stateStore == nil {
		return
	}
	switch m.activeTab {
	case TabPullRequests:
		id := m.pullRequestsView.DetailItemID()
		m.stateStore.Apply(func(s *state.State) {
			s.Version = state.CurrentVersion
			s.Tabs.PullRequests.LastDetailID = id
		})
	case TabWorkItems:
		id := m.workItemsView.DetailItemID()
		m.stateStore.Apply(func(s *state.State) {
			s.Version = state.CurrentVersion
			s.Tabs.WorkItems.LastDetailID = id
		})
	}
}

// isTabEnabled returns true if the given tab is in the enabledTabs list.
func (m Model) isTabEnabled(tab Tab) bool {
	for _, t := range m.enabledTabs {
		if t == tab {
			return true
		}
	}
	return false
}

// nextTab returns the next enabled tab after the current one (wrapping).
func (m Model) nextTab() Tab {
	for i, t := range m.enabledTabs {
		if t == m.activeTab {
			return m.enabledTabs[(i+1)%len(m.enabledTabs)]
		}
	}
	return m.enabledTabs[0]
}

// prevTab returns the previous enabled tab before the current one (wrapping).
func (m Model) prevTab() Tab {
	for i, t := range m.enabledTabs {
		if t == m.activeTab {
			prev := i - 1
			if prev < 0 {
				prev = len(m.enabledTabs) - 1
			}
			return m.enabledTabs[prev]
		}
	}
	return m.enabledTabs[0]
}

// equalSlices reports whether two string slices have identical contents.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildEnabledTabs returns the list of enabled tabs based on config.
// azurePresent must be true when a live Azure MultiClient is available;
// the metrics tab requires both cfg.Metrics.Enabled AND azurePresent.
func buildEnabledTabs(cfg *config.Config, azurePresent bool) []Tab {
	tabs := []Tab{TabPullRequests} // always enabled
	if cfg.IsPaneEnabled("workitems") {
		tabs = append(tabs, TabWorkItems)
	}
	if cfg.IsPaneEnabled("pipelines") {
		tabs = append(tabs, TabPipelines)
	}
	if cfg.Metrics.Enabled && azurePresent {
		tabs = append(tabs, TabMetrics)
	}
	return tabs
}

// initTabCmd returns the Init command for the given tab, or nil for pipelines
// (which is populated by the poller).
func (m Model) initTabCmd(tab Tab) tea.Cmd {
	switch tab {
	case TabPullRequests:
		return m.pullRequestsView.Init()
	case TabWorkItems:
		return m.workItemsView.Init()
	case TabMetrics:
		return m.metricsView.Init()
	}
	return nil
}

// formatVersionInfo formats the version and commit hash for display.
// For example: "1.2.3 (abc1234)" or "dev (none)".
func formatVersionInfo(version, commit string) string {
	if commit != "" && commit != "none" {
		return fmt.Sprintf("%s (%s)", version, commit)
	}
	return version
}

// NewModel creates a new application model.
//
// p is the backend-neutral provider used by the three main views. It is stored
// on Model.client for future use by tasks 7-9. Pass nil to disable provider-
// backed features (e.g. in tests that don't exercise view fetching).
//
// mc is the concrete Azure DevOps multi-client. It is passed directly to the
// view initializers (which still accept *azdevops.MultiClient until tasks 7-9
// migrate them) and stored on Model.metricsClient for the metrics view, which
// requires Azure-specific methods not covered by provider.Provider (Decision 5).
func NewModel(p provider.Provider, mc *azdevops.MultiClient, cfg *config.Config, currentVersion string, commitHash string) Model {
	// Create error handler early to capture initialization errors
	errorHandler := polling.NewErrorHandler()

	// Load custom themes from themes directory
	if themesDir, err := styles.GetThemesDirectoryPath(); err != nil {
		// Failed to get themes directory path
		errorHandler.SetError(fmt.Errorf("failed to access themes directory: %w", err))
	} else {
		// Try to load custom themes
		_, err := styles.LoadCustomThemesFromDirectory(themesDir)
		if err != nil {
			// Failed to load custom themes - set error but continue
			errorHandler.SetError(fmt.Errorf("failed to load custom themes from %s: %w", themesDir, err))
		}
	}

	// Try to load the requested theme
	requestedTheme := cfg.GetTheme()
	theme, themeErr := styles.GetThemeByName(requestedTheme)
	if themeErr != nil {
		// Fall back to default theme
		theme = styles.GetDefaultTheme()
	}

	appStyles := styles.NewStyles(theme)

	// Create logo
	logo := components.NewLogo(appStyles)

	// Create status bar with org/project info
	statusBar := components.NewStatusBar(appStyles)
	statusBar.SetOrganization(cfg.Organization)
	scopes := make([]string, len(cfg.Projects))
	for i, p := range cfg.Projects {
		scopes[i] = cfg.DisplayNameFor(p)
	}
	statusBar.SetScopes(scopes)

	// Gate metrics on both the config flag and a live Azure backend.
	// The metrics view uses Azure-specific APIs not available via provider.Provider,
	// so it must only be enabled when a concrete MultiClient is present.
	metricsEnabled := cfg.Metrics.Enabled && mc != nil

	// Create help modal
	helpModal := components.NewHelpModal(appStyles)

	// Configure help modal based on disabled panes
	if !cfg.IsPaneEnabled("workitems") {
		helpModal.RemoveBindingsByDescription("work items")
		helpModal.RemoveBindingsByDescription("work item")
	}
	if !cfg.IsPaneEnabled("pipelines") {
		helpModal.RemoveSection("Log Viewer (pipelines)")
		helpModal.RemoveBindingsByDescription("pipelines")
	}

	// Update tab description in help modal based on enabled tabs.
	// Labels resolve through TermFor (same lowercase keys as renderTabBar) so a
	// configured term override shows up in the help dialog too, not just the tab
	// bar. The fallbacks are the help line's abbreviated defaults ("PR").
	enabledTabNames := []string{cfg.TermFor("pull_requests", "PR")}
	if cfg.IsPaneEnabled("workitems") {
		enabledTabNames = append(enabledTabNames, cfg.TermFor("work_items", "Work Items"))
	}
	if cfg.IsPaneEnabled("pipelines") {
		enabledTabNames = append(enabledTabNames, cfg.TermFor("pipelines", "Pipelines"))
	}
	if metricsEnabled {
		enabledTabNames = append(enabledTabNames, cfg.TermFor("metrics", "Metrics"))
	}
	// Rebuild the tabs help line whenever the set differs from the default
	// "1/2/3 — PR / Work Items / Pipelines" (e.g. a pane disabled, metrics
	// enabled, or both).
	defaultTabs := []string{"PR", "Work Items", "Pipelines"}
	if !equalSlices(enabledTabNames, defaultTabs) {
		keys := make([]string, len(enabledTabNames))
		for i := range enabledTabNames {
			keys[i] = fmt.Sprintf("%d", i+1)
		}
		helpModal.UpdateTabsBinding(
			strings.Join(keys, "/"),
			strings.Join(enabledTabNames, " / "),
		)
	}
	if metricsEnabled {
		helpModal.AddSection("Metrics tab", []components.HelpBinding{
			{Key: "v", Description: "Toggle Live ↔ Trends sub-view"},
			{Key: "Tab", Description: "Switch focus between stuck-items and per-user pane (Live)"},
			{Key: "↑/↓", Description: "Live: move cursor; Trends: scroll"},
			{Key: "pgup/pgdn", Description: "Scroll dashboard body"},
			{Key: "f", Description: "Cycle flag filter — Live only (All / Active-stale / RFT-stale)"},
			{Key: "T", Description: "Live: filter by tag. Trends: multi-select sprint picker"},
			{Key: "esc", Description: "Clear tag filter (Live)"},
			{Key: "o", Description: "Open focused item in browser (Live)"},
			{Key: "r", Description: "Refresh metrics + reload snapshot file"},
		})
	}

	// Set version info in help modal
	helpModal.SetVersionInfo(formatVersionInfo(currentVersion, commitHash))

	// Set config path in help modal
	if configPath, err := config.GetPath(); err == nil {
		helpModal.SetConfigPath(configPath)
	}

	// Create error modal
	errorModal := components.NewErrorModal(appStyles)

	// Create theme picker
	availableThemes := styles.ListAvailableThemes()
	themePicker := components.NewThemePicker(appStyles, availableThemes, cfg.GetTheme())

	// Create poller with configured interval.
	// When mc is nil (GitHub-only user) we must pass an untyped-nil PipelineClient
	// rather than mc directly. Passing mc (a typed *azdevops.MultiClient nil) would
	// box it into a non-nil interface value, defeating the p.client == nil guards in
	// the poller and causing a nil-deref panic on the first fetch.
	interval := time.Duration(cfg.PollingInterval) * time.Second
	if interval <= 0 {
		interval = polling.DefaultInterval
	}
	var pc polling.PipelineClient
	if mc != nil {
		pc = mc
	}
	poller := polling.NewPoller(pc, interval)

	// If theme was not found, set a friendly error message
	if themeErr != nil {
		themesDir, _ := styles.GetThemesDirectoryPath()
		themeNotFoundErr := &ThemeNotFoundError{
			ThemeName:  requestedTheme,
			ThemesPath: themesDir,
		}
		errorHandler.SetError(themeNotFoundErr)
	}

	enabledTabs := buildEnabledTabs(cfg, mc != nil)

	var mv metrics.Model
	if metricsEnabled {
		mv = metrics.NewModelWithStyles(mc, cfg, appStyles)
	}

	return Model{
		client:        p,
		metricsClient: mc,
		config:        cfg,
		styles:        appStyles,
		activeTab:     TabPullRequests,
		enabledTabs:   enabledTabs,
		logo:          logo,
		// pullRequestsView, workItemsView, and pipelinesView all consume provider.Provider (tasks 7-9).
		pipelinesView:    pipelines.NewModelWithStyles(p, appStyles),
		pullRequestsView: pullrequests.NewModelWithStyles(p, appStyles),
		workItemsView:    workitems.NewModelWithStyles(p, appStyles),
		metricsView:      mv,
		statusBar:        statusBar,
		helpModal:        helpModal,
		errorModal:       errorModal,
		themePicker:      themePicker,
		poller:           poller,
		errorHandler:     errorHandler,
		currentVersion:   currentVersion,
		commitHash:       commitHash,
	}
}

// Init initializes the application
func (m Model) Init() tea.Cmd {
	// Check for any startup errors (e.g., theme not found)
	// Use warning message so it persists even after successful polling
	if m.errorHandler.ShouldShowError() {
		m.statusBar.SetWarningMessage(m.errorHandler.ErrorMessage())
	}

	initCmds := []tea.Cmd{
		m.poller.FetchPipelineRuns(), // Initial fetch - updates connection state
		m.poller.StartPolling(),      // Start polling timer
		checkForUpdate(m.currentVersion),
	}

	// Initialize the active tab's view. After ApplyState, this may be a
	// non-default tab restored from persisted state.
	if cmd := m.initTabCmd(m.activeTab); cmd != nil {
		initCmds = append(initCmds, cmd)
	}
	// PR is the canonical default; ensure its data is preloaded even when
	// state restored a different tab so switching back is instant.
	if m.activeTab != TabPullRequests {
		initCmds = append(initCmds, m.pullRequestsView.Init())
	}

	return tea.Batch(initCmds...)
}

// checkForUpdate returns a tea.Cmd that checks GitHub for a newer version.
// Failures are silently ignored.
func checkForUpdate(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		checker := version.NewChecker(currentVersion)
		info, err := checker.CheckForUpdate()
		if err != nil {
			// Silently ignore errors — don't disrupt the user
			return nil
		}
		return updateCheckMsg{info: info}
	}
}

// Update handles incoming messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// If error modal is visible, handle its input first (highest priority)
	if m.errorModal.IsVisible() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			m.errorModal, _ = m.errorModal.Update(msg)
			return m, nil
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.errorModal.SetSize(msg.Width, msg.Height)
			m.statusBar.SetWidth(msg.Width)
			return m, nil
		}
		return m, nil
	}

	// If help modal is visible, handle its input first
	if m.helpModal.IsVisible() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			m.helpModal, _ = m.helpModal.Update(msg)
			return m, nil
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.helpModal.SetSize(msg.Width, msg.Height)
			m.statusBar.SetWidth(msg.Width)
			return m, nil
		}
		return m, nil
	}

	// If theme picker is visible, handle its input first
	if m.themePicker.IsVisible() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			var cmd tea.Cmd
			m.themePicker, cmd = m.themePicker.Update(msg)
			return m, cmd
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.themePicker.SetSize(msg.Width, msg.Height)
			return m, nil
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// When a child view is in search mode or has a modal picker open,
		// skip all global shortcuts so keystrokes reach the input instead.
		if m.isActiveViewCapturingInput() {
			break
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.poller.Stop()
			return m, tea.Quit
		case "?":
			m.helpModal.SetSize(m.width, m.height)
			m.helpModal.Show()
			return m, nil
		case "t":
			m.themePicker.SetSize(m.width, m.height)
			m.themePicker.Show()
			return m, nil
		case "1", "2", "3", "4":
			idx := int(msg.String()[0]-'0') - 1 // "1"→0, "2"→1, "3"→2, "4"→3
			if idx >= 0 && idx < len(m.enabledTabs) {
				target := m.enabledTabs[idx]
				if target != m.activeTab {
					m.activeTab = target
					m.resizeActiveViewIfNeeded()
					m.recordActiveTab()
					return m, m.initTabCmd(target)
				}
			}
			return m, nil
		case "left":
			prev := m.prevTab()
			if prev != m.activeTab {
				m.activeTab = prev
				m.resizeActiveViewIfNeeded()
				m.recordActiveTab()
				return m, m.initTabCmd(prev)
			}
			return m, nil
		case "right":
			next := m.nextTab()
			if next != m.activeTab {
				m.activeTab = next
				m.resizeActiveViewIfNeeded()
				m.recordActiveTab()
				return m, m.initTabCmd(next)
			}
			return m, nil
		}

	case components.ThemeSelectedMsg:
		// Update theme in config and save
		if err := m.config.UpdateTheme(msg.ThemeName); err != nil {
			// Handle error - set error message in status bar
			m.errorHandler.SetError(fmt.Errorf("failed to save theme: %w", err))
			m.statusBar.SetState(polling.StateError)
			m.statusBar.SetErrorMessage(fmt.Sprintf("Failed to save theme setting: %v", err))
			return m, nil
		}

		// Load the new theme
		theme, err := styles.GetThemeByName(msg.ThemeName)
		if err != nil {
			// Handle error - this shouldn't happen as we selected from available themes
			m.errorHandler.SetError(fmt.Errorf("failed to load theme: %w", err))
			m.statusBar.SetState(polling.StateError)
			m.statusBar.SetErrorMessage(fmt.Sprintf("Failed to load theme '%s': %v", msg.ThemeName, err))
			return m, nil
		}

		// Create new styles with the selected theme
		m.styles = styles.NewStyles(theme)

		// Preserve transient status bar state before rebuilding
		previousState := m.statusBar.GetState()
		previousWarning := m.statusBar.GetWarningMessage()

		// Update all components with new styles
		m.statusBar = components.NewStatusBar(m.styles)
		m.statusBar.SetState(previousState)
		if previousWarning != "" {
			m.statusBar.SetWarningMessage(previousWarning)
		}
		m.statusBar.SetOrganization(m.config.Organization)
		scopes := make([]string, len(m.config.Projects))
		for i, p := range m.config.Projects {
			scopes[i] = m.config.DisplayNameFor(p)
		}
		m.statusBar.SetScopes(scopes)
		m.statusBar.SetWidth(m.width)

		m.logo = components.NewLogo(m.styles)

		m.helpModal = components.NewHelpModal(m.styles)
		m.helpModal.SetVersionInfo(formatVersionInfo(m.currentVersion, m.commitHash))
		if configPath, err := config.GetPath(); err == nil {
			m.helpModal.SetConfigPath(configPath)
		}
		m.helpModal.SetSize(m.width, m.height)

		m.errorModal = components.NewErrorModal(m.styles)
		m.errorModal.SetSize(m.width, m.height)

		// Update theme picker with new styles and current theme
		availableThemes := styles.ListAvailableThemes()
		m.themePicker = components.NewThemePicker(m.styles, availableThemes, msg.ThemeName)

		// Recreate views with new styles.
		// pullRequestsView, workItemsView, and pipelinesView all use provider.Provider (tasks 7-9).
		m.pipelinesView = pipelines.NewModelWithStyles(m.client, m.styles)
		m.pullRequestsView = pullrequests.NewModelWithStyles(m.client, m.styles)
		m.workItemsView = workitems.NewModelWithStyles(m.client, m.styles)
		// Re-style the metrics view in place rather than reconstructing it —
		// recreating would erase its loaded snapshots, sprint selection and
		// fetched rows, blanking the section on theme change.
		m.metricsView.SetStyles(m.styles)

		// CRITICAL: Set window size for all views before they try to render
		// Subtract border space (2 width for sides, 2 height for top/bottom borders)
		if m.width > 0 && m.height > 0 {
			m.footerRows = m.measureFooterHeight()
			contentSize := m.contentViewSize()
			m.pipelinesView, _ = m.pipelinesView.Update(contentSize)
			m.pullRequestsView, _ = m.pullRequestsView.Update(contentSize)
			m.workItemsView, _ = m.workItemsView.Update(contentSize)
			m.metricsView, _ = m.metricsView.Update(contentSize)
		}

		// Re-initialize views to fetch data again
		cmds = append(cmds, m.pipelinesView.Init())
		if m.activeTab == TabPullRequests {
			cmds = append(cmds, m.pullRequestsView.Init())
		}
		if m.activeTab == TabWorkItems {
			cmds = append(cmds, m.workItemsView.Init())
		}
		// The metrics view is re-styled in place (SetStyles above), not
		// recreated, so it must NOT be re-initialized here — re-running its
		// async fetch/snapshot load would blank the already-loaded trends data
		// on theme change. The normal tab-activation path re-inits it when
		// needed.

		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.SetWidth(msg.Width)
		m.errorModal.SetSize(msg.Width, msg.Height)
		m.helpModal.SetSize(msg.Width, msg.Height)
		m.themePicker.SetSize(msg.Width, msg.Height)
		// Measure actual footer height at current width
		m.footerRows = m.measureFooterHeight()
		contentSize := m.contentViewSize()
		m.pipelinesView, _ = m.pipelinesView.Update(contentSize)
		m.pullRequestsView, _ = m.pullRequestsView.Update(contentSize)
		m.workItemsView, _ = m.workItemsView.Update(contentSize)
		m.metricsView, _ = m.metricsView.Update(contentSize)
		return m, nil

	case updateCheckMsg:
		if msg.info != nil && msg.info.UpdateAvailable {
			m.statusBar.SetUpdateMessage(
				fmt.Sprintf("Update available: %s → %s", msg.info.CurrentVersion, msg.info.LatestVersion),
			)
		}
		return m, nil

	case polling.TickMsg:
		// Time to poll for updates
		cmds = append(cmds, m.poller.OnTick())

	case components.CriticalErrorMsg:
		m.errorModal.SetSize(m.width, m.height)
		m.errorModal.Show(msg.Title, msg.Message, msg.Hint)
		return m, nil

	case polling.PipelineRunsUpdated:
		// Process the update through error handler
		runs, hasError := m.errorHandler.ProcessUpdate(msg)

		if hasError {
			m.statusBar.SetState(polling.StateError)
			// Check if this is a critical error that should show the modal
			if errInfo := components.ClassifyError(msg.Err); errInfo != nil {
				m.errorModal.SetSize(m.width, m.height)
				m.errorModal.Show(errInfo.Title, errInfo.Message, errInfo.Hint)
			}
			// Display user-friendly error message in status bar too
			if m.errorHandler.ShouldShowError() {
				m.statusBar.SetErrorMessage(m.errorHandler.RecoveryMessage())
			}
		} else {
			m.statusBar.SetState(polling.StateConnected)
			m.statusBar.ClearErrorMessage()

			// Check for partial project load warning
			if warning := m.errorHandler.PartialWarning(); warning != "" {
				m.statusBar.SetWarningMessage(warning)
			} else {
				m.statusBar.ClearWarningMessage()
			}
		}

		// Update pipelines view with the runs, mapping wire types to neutral types.
		if runs != nil {
			providerRuns := make([]provider.PipelineRun, len(runs))
			for i, r := range runs {
				providerRuns[i] = azdevops.MapPipelineRun(r, r.ProjectName, r.ProjectDisplayName)
			}
			pipelineMsg := pipelines.SetRunsMsg{Runs: providerRuns}
			var cmd tea.Cmd
			m.pipelinesView, cmd = m.pipelinesView.Update(pipelineMsg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	// Delegate to active view
	var cmd tea.Cmd
	switch m.activeTab {
	case TabPullRequests:
		m.pullRequestsView, cmd = m.pullRequestsView.Update(msg)
	case TabWorkItems:
		m.workItemsView, cmd = m.workItemsView.Update(msg)
	case TabMetrics:
		m.metricsView, cmd = m.metricsView.Update(msg)
	default:
		m.pipelinesView, cmd = m.pipelinesView.Update(msg)
	}
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Re-measure footer height after view update — if the view switched
	// between list/detail mode, the footer height changes and we need to
	// resize the active view to match.
	m.resizeActiveViewIfNeeded()

	// Persist any change to the currently open detail view (e.g. user
	// pressed Enter on a row, or Esc to leave detail). No-op if the
	// store isn't attached.
	m.recordDetailState()

	return m, tea.Batch(cmds...)
}

// isActiveViewSearching returns true if the currently active tab's view is in search mode.
func (m Model) isActiveViewSearching() bool {
	switch m.activeTab {
	case TabPipelines:
		return m.pipelinesView.IsSearching()
	case TabPullRequests:
		return m.pullRequestsView.IsSearching()
	case TabWorkItems:
		return m.workItemsView.IsSearching()
	case TabMetrics:
		return m.metricsView.IsSearching()
	}
	return false
}

// isActiveViewCapturingInput returns true when the active view is either in
// search mode or has a modal picker open — in both cases global shortcuts
// must yield so keystrokes land in the view's own input field.
func (m Model) isActiveViewCapturingInput() bool {
	if m.isActiveViewSearching() {
		return true
	}
	switch m.activeTab {
	case TabWorkItems:
		return m.workItemsView.IsTagPickerVisible() ||
			m.workItemsView.IsStatePickerVisible() ||
			m.workItemsView.IsCommentFormVisible()
	case TabPipelines:
		return m.pipelinesView.IsStatusPickerVisible()
	case TabMetrics:
		return m.metricsView.IsTagPickerVisible()
	}
	return false
}

// Note: Tab styles are now provided by the styles package and accessed via m.styles

// contentViewSize returns the size available for content views inside the
// bordered content box. It subtracts all chrome from the terminal height:
// the tab bar (with its own border), the content box borders, and the footer.
//
// The "\n" joins between tabBar, contentBox, and footer in View() do not
// consume extra lines — they only prevent adjacent components from merging
// on the same line (e.g. "╰──╯╭──╮" becoming one line instead of two).
func (m Model) contentViewSize() tea.WindowSizeMsg {
	const minContentWidth = 20
	const minContentHeight = 5

	width := m.width - borderWidth
	if width < minContentWidth {
		width = minContentWidth
	}

	height := m.height - tabBarRows - boxBorderRows - m.footerRows
	if height < minContentHeight {
		height = minContentHeight
	}

	return tea.WindowSizeMsg{
		Width:  width,
		Height: height,
	}
}

// resizeActiveViewIfNeeded re-measures the footer height and resizes
// the active content view if it changed (e.g., after tab switch or
// view mode change).
func (m *Model) resizeActiveViewIfNeeded() {
	// Sync context items on the status bar BEFORE measuring, so the
	// footer height reflects the current view mode (list vs detail).
	m.syncStatusBarContext()
	newFooterRows := m.measureFooterHeight()
	if newFooterRows == m.footerRows {
		return
	}
	m.footerRows = newFooterRows
	contentSize := m.contentViewSize()
	switch m.activeTab {
	case TabPullRequests:
		m.pullRequestsView, _ = m.pullRequestsView.Update(contentSize)
	case TabWorkItems:
		m.workItemsView, _ = m.workItemsView.Update(contentSize)
	case TabMetrics:
		m.metricsView, _ = m.metricsView.Update(contentSize)
	default:
		m.pipelinesView, _ = m.pipelinesView.Update(contentSize)
	}
}

// syncStatusBarContext reads context items from the active view and updates
// the status bar. This ensures measureFooterHeight uses the correct state
// during Update, not stale state from the previous View call.
func (m *Model) syncStatusBarContext() {
	var hasContextBar bool
	var contextItems []components.ContextItem

	switch m.activeTab {
	case TabPullRequests:
		hasContextBar = m.pullRequestsView.HasContextBar()
		contextItems = m.pullRequestsView.GetContextItems()
	case TabWorkItems:
		hasContextBar = m.workItemsView.HasContextBar()
		contextItems = m.workItemsView.GetContextItems()
	case TabMetrics:
		hasContextBar = m.metricsView.HasContextBar()
		contextItems = m.metricsView.GetContextItems()
	default:
		hasContextBar = m.pipelinesView.HasContextBar()
		contextItems = m.pipelinesView.GetContextItems()
	}

	if hasContextBar {
		m.statusBar.SetContextItems(contextItems)
	} else {
		m.statusBar.ClearContextItems()
	}
}

// workItemsKeybindings returns the keybindings string for the work items list view.
func (m Model) workItemsKeybindings() string {
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.styles.Theme.Border))
	sep := sepStyle.Render(" • ")

	return m.styles.Key.Render("r") + m.styles.Description.Render(" refresh") + sep +
		m.styles.Key.Render("↑↓") + m.styles.Description.Render(" navigate") + sep +
		m.styles.Key.Render("enter") + m.styles.Description.Render(" details") + sep +
		m.styles.Key.Render("f") + m.styles.Description.Render(" search") + sep +
		m.styles.Key.Render("m") + m.styles.Description.Render(" my items") + sep +
		m.styles.Key.Render("T") + m.styles.Description.Render(" tags") + sep +
		m.styles.Key.Render("s") + m.styles.Description.Render(" state") + sep +
		m.styles.Key.Render("esc") + m.styles.Description.Render(" back") + sep +
		m.styles.Key.Render("?") + m.styles.Description.Render(" help") + sep +
		m.styles.Key.Render("q") + m.styles.Description.Render(" quit")
}

// pullRequestsKeybindings returns the keybindings string for the PR list view.
func (m Model) pullRequestsKeybindings() string {
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.styles.Theme.Border))
	sep := sepStyle.Render(" • ")

	return m.styles.Key.Render("r") + m.styles.Description.Render(" refresh") + sep +
		m.styles.Key.Render("↑↓") + m.styles.Description.Render(" navigate") + sep +
		m.styles.Key.Render("enter") + m.styles.Description.Render(" details") + sep +
		m.styles.Key.Render("f") + m.styles.Description.Render(" search") + sep +
		m.styles.Key.Render("m") + m.styles.Description.Render(" my PRs") + sep +
		m.styles.Key.Render("A") + m.styles.Description.Render(" as reviewer") + sep +
		m.styles.Key.Render("esc") + m.styles.Description.Render(" back") + sep +
		m.styles.Key.Render("?") + m.styles.Description.Render(" help") + sep +
		m.styles.Key.Render("q") + m.styles.Description.Render(" quit")
}

// metricsKeybindings returns the keybindings string for the metrics dashboard.
func (m Model) metricsKeybindings() string {
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.styles.Theme.Border))
	sep := sepStyle.Render(" • ")

	return m.styles.Key.Render("r") + m.styles.Description.Render(" refresh") + sep +
		m.styles.Key.Render("v") + m.styles.Description.Render(" live/trends") + sep +
		m.styles.Key.Render("Tab") + m.styles.Description.Render(" focus pane") + sep +
		m.styles.Key.Render("↑↓") + m.styles.Description.Render(" navigate") + sep +
		m.styles.Key.Render("T") + m.styles.Description.Render(" tag") + sep +
		m.styles.Key.Render("f") + m.styles.Description.Render(" flag filter") + sep +
		m.styles.Key.Render("o") + m.styles.Description.Render(" open") + sep +
		m.styles.Key.Render("esc") + m.styles.Description.Render(" clear tag") + sep +
		m.styles.Key.Render("?") + m.styles.Description.Render(" help") + sep +
		m.styles.Key.Render("q") + m.styles.Description.Render(" quit")
}

// pipelinesKeybindings returns the keybindings string for the pipelines list view.
func (m Model) pipelinesKeybindings() string {
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.styles.Theme.Border))
	sep := sepStyle.Render(" • ")

	return m.styles.Key.Render("r") + m.styles.Description.Render(" refresh") + sep +
		m.styles.Key.Render("↑↓") + m.styles.Description.Render(" navigate") + sep +
		m.styles.Key.Render("enter") + m.styles.Description.Render(" details") + sep +
		m.styles.Key.Render("f") + m.styles.Description.Render(" search") + sep +
		m.styles.Key.Render("S") + m.styles.Description.Render(" status") + sep +
		m.styles.Key.Render("esc") + m.styles.Description.Render(" back") + sep +
		m.styles.Key.Render("?") + m.styles.Description.Render(" help") + sep +
		m.styles.Key.Render("q") + m.styles.Description.Render(" quit")
}

// measureFooterHeight measures the actual footer height. The footer is always
// just the status bar (context items are now rendered inline in the status bar).
func (m Model) measureFooterHeight() int {
	return strings.Count(m.statusBar.View(), "\n") + 1
}

// renderTabBar renders the tab header content wrapped in its own bordered box,
// with tabs on the left and the ASCII art logo on the right.
func (m Model) renderTabBar(innerWidth int) string {
	// tabLabel pairs each tab with its config key and default display string.
	// Keys MUST be lowercase — viper lowercases all config keys on load, so
	// Terms map keys arrive lowercase. A capitalised lookup key would never match.
	type tabLabel struct{ key, def string }
	tabLabels := map[Tab]tabLabel{
		TabPullRequests: {"pull_requests", "Pull Requests"},
		TabWorkItems:    {"work_items", "Work Items"},
		TabPipelines:    {"pipelines", "Pipelines"},
		TabMetrics:      {"metrics", "Metrics"},
	}

	var renderedTabs []string
	for i, tab := range m.enabledTabs {
		tl := tabLabels[tab]
		label := fmt.Sprintf("%d: %s", i+1, m.config.TermFor(tl.key, tl.def))
		if tab == m.activeTab {
			renderedTabs = append(renderedTabs, m.styles.TabActive.Render(label))
		} else {
			renderedTabs = append(renderedTabs, m.styles.TabInactive.Render(label))
		}
	}

	tabs := strings.Join(renderedTabs, " ")
	logo := m.logo.View()

	// Place tabs on the left (top-aligned) and logo on the right.
	logoWidth := lipgloss.Width(logo)
	tabsWidth := innerWidth - logoWidth
	if tabsWidth < 0 {
		tabsWidth = 0
	}

	left := lipgloss.NewStyle().
		Width(tabsWidth).
		Height(m.logo.Height()).
		AlignVertical(lipgloss.Center).
		Render(tabs)

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, logo)

	return m.styles.TabBar.Width(innerWidth).Render(content)
}

// View renders the application UI
func (m Model) View() string {
	if m.err != nil {
		return "Error: " + m.err.Error() + "\n\nPress q to quit."
	}

	// If error modal is visible, show it as overlay
	if m.errorModal.IsVisible() {
		return m.errorModal.View()
	}

	// If help modal is visible, show it as overlay
	if m.helpModal.IsVisible() {
		return m.helpModal.View()
	}

	// If theme picker is visible, show it as overlay
	if m.themePicker.IsVisible() {
		return m.themePicker.View()
	}

	// If tag picker is visible, show it as overlay
	if m.activeTab == TabWorkItems && m.workItemsView.IsTagPickerVisible() {
		m.workItemsView.SetTagPickerSize(m.width, m.height)
		return m.workItemsView.TagPickerView()
	}

	// If state picker is visible, show it as overlay
	if m.activeTab == TabWorkItems && m.workItemsView.IsStatePickerVisible() {
		m.workItemsView.SetStatePickerSize(m.width, m.height)
		return m.workItemsView.StatePickerView()
	}

	// If status picker is visible, show it as overlay
	if m.activeTab == TabPipelines && m.pipelinesView.IsStatusPickerVisible() {
		m.pipelinesView.SetStatusPickerSize(m.width, m.height)
		return m.pipelinesView.StatusPickerView()
	}

	// Metrics tag picker overlay
	if m.activeTab == TabMetrics && m.metricsView.IsTagPickerVisible() {
		m.metricsView.SetTagPickerSize(m.width, m.height)
		return m.metricsView.TagPickerView()
	}

	// Render tab bar in its own bordered box
	contentSize := m.contentViewSize()
	tabBar := m.renderTabBar(contentSize.Width)

	// Render content based on active tab
	var content string
	var hasContextBar bool
	var contextItems []components.ContextItem
	var scrollPercent float64
	var statusMessage string

	switch m.activeTab {
	case TabPullRequests:
		content = m.pullRequestsView.View()
		hasContextBar = m.pullRequestsView.HasContextBar()
		contextItems = m.pullRequestsView.GetContextItems()
		scrollPercent = m.pullRequestsView.GetScrollPercent()
		statusMessage = m.pullRequestsView.GetStatusMessage()
	case TabWorkItems:
		content = m.workItemsView.View()
		hasContextBar = m.workItemsView.HasContextBar()
		contextItems = m.workItemsView.GetContextItems()
		scrollPercent = m.workItemsView.GetScrollPercent()
		statusMessage = m.workItemsView.GetStatusMessage()
	case TabMetrics:
		content = m.metricsView.View()
		hasContextBar = m.metricsView.HasContextBar()
		contextItems = m.metricsView.GetContextItems()
		scrollPercent = m.metricsView.GetScrollPercent()
		statusMessage = m.metricsView.GetStatusMessage()
	default:
		content = m.pipelinesView.View()
		hasContextBar = m.pipelinesView.HasContextBar()
		contextItems = m.pipelinesView.GetContextItems()
		scrollPercent = m.pipelinesView.GetScrollPercent()
		statusMessage = m.pipelinesView.GetStatusMessage()
	}

	// Set tab-specific keybindings on status bar
	if m.activeTab == TabPullRequests && !hasContextBar {
		m.statusBar.SetKeybindings(m.pullRequestsKeybindings())
	} else if m.activeTab == TabWorkItems && !hasContextBar {
		m.statusBar.SetKeybindings(m.workItemsKeybindings())
	} else if m.activeTab == TabPipelines && !hasContextBar {
		m.statusBar.SetKeybindings(m.pipelinesKeybindings())
	} else if m.activeTab == TabMetrics && !hasContextBar {
		m.statusBar.SetKeybindings(m.metricsKeybindings())
	} else {
		m.statusBar.SetKeybindings("")
	}

	// Update filter label badge on status bar
	if m.activeTab == TabWorkItems {
		var labels []string
		if m.workItemsView.IsMyItemsActive() {
			labels = append(labels, "My Items")
		}
		if m.workItemsView.IsTagFilterActive() {
			labels = append(labels, "Tag: "+m.workItemsView.ActiveTag())
		}
		if m.workItemsView.IsStateFilterActive() {
			labels = append(labels, "State: "+m.workItemsView.ActiveState())
		}
		if len(labels) > 0 {
			m.statusBar.SetFilterLabel(strings.Join(labels, " + "))
		} else {
			m.statusBar.ClearFilterLabel()
		}
	} else if m.activeTab == TabPipelines {
		var labels []string
		if m.pipelinesView.IsStatusFilterActive() {
			labels = append(labels, "Status: "+m.pipelinesView.ActiveStatus())
		}
		if len(labels) > 0 {
			m.statusBar.SetFilterLabel(strings.Join(labels, " + "))
		} else {
			m.statusBar.ClearFilterLabel()
		}
	} else if m.activeTab == TabPullRequests {
		switch {
		case m.pullRequestsView.IsMyPRsActive():
			m.statusBar.SetFilterLabel("My PRs")
		case m.pullRequestsView.IsAsReviewerActive():
			m.statusBar.SetFilterLabel("Reviewer")
		default:
			m.statusBar.ClearFilterLabel()
		}
	} else if m.activeTab == TabMetrics {
		if m.metricsView.IsTagFilterActive() {
			m.statusBar.SetFilterLabel("Tag: " + m.metricsView.ActiveTag())
		} else {
			m.statusBar.ClearFilterLabel()
		}
	} else {
		m.statusBar.ClearFilterLabel()
	}

	// Pass context items to status bar (replaces default keybindings in detail views)
	if hasContextBar {
		m.statusBar.SetContextItems(contextItems)
		if statusMessage != "" {
			m.statusBar.SetContextStatus(statusMessage)
		}
	} else {
		m.statusBar.ClearContextItems()
	}

	// Pass scroll percent to status bar
	if scrollPercent > 0 {
		m.statusBar.ShowScrollPercent(true)
		m.statusBar.SetScrollPercent(scrollPercent)
	} else {
		m.statusBar.ShowScrollPercent(false)
	}

	footer := m.statusBar.View()

	// Render content in its own bordered box, using the same dimensions
	// that were used to size the content views.
	contentBox := m.styles.ContentBox.
		Width(contentSize.Width).
		Height(contentSize.Height).
		Render(content)

	return tabBar + "\n" + contentBox + "\n" + footer
}
