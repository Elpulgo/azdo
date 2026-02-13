package app

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/pipelines"
	"github.com/Elpulgo/azdo/internal/ui/pullrequests"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/Elpulgo/azdo/internal/ui/workitems"
	tea "github.com/charmbracelet/bubbletea"
)

// ThemeNotFoundError represents an error when a requested theme is not found.
type ThemeNotFoundError struct {
	ThemeName  string
	ThemesPath string
}

func (e *ThemeNotFoundError) Error() string {
	availableThemes := styles.ListAvailableThemes()
	return fmt.Sprintf("Theme '%s' not found. Using default theme. Available themes: %v. Custom themes can be placed in: %s",
		e.ThemeName, availableThemes, e.ThemesPath)
}

// Tab represents the active tab in the application
type Tab int

const (
	TabPipelines    Tab = iota // Pipelines tab (key '1')
	TabPullRequests            // Pull Requests tab (key '2')
	TabWorkItems               // Work Items tab (key '3')
)

// Model is the root application model for the TUI
type Model struct {
	client           *azdevops.Client
	config           *config.Config
	styles           *styles.Styles
	activeTab        Tab
	pipelinesView    pipelines.Model
	pullRequestsView pullrequests.Model
	workItemsView    workitems.Model
	statusBar        *components.StatusBar
	contextBar       *components.ContextBar
	helpModal        *components.HelpModal
	themePicker      components.ThemePicker
	poller           *polling.Poller
	errorHandler     *polling.ErrorHandler
	width            int
	height           int
	err              error
}

// NewModel creates a new application model with the given Azure DevOps client and config
func NewModel(client *azdevops.Client, cfg *config.Config) Model {
	// Load custom themes from themes directory
	if themesDir, err := styles.GetThemesDirectoryPath(); err == nil {
		// Silently load custom themes - errors are ignored
		_, _ = styles.LoadCustomThemesFromDirectory(themesDir)
	}

	// Try to load the requested theme
	requestedTheme := cfg.GetTheme()
	theme, themeErr := styles.GetThemeByName(requestedTheme)
	if themeErr != nil {
		// Fall back to default theme
		theme = styles.GetDefaultTheme()
	}

	appStyles := styles.NewStyles(theme)

	// Create status bar with org/project info
	statusBar := components.NewStatusBar(appStyles)
	statusBar.SetOrganization(cfg.Organization)
	statusBar.SetProject(cfg.Project)

	// Set config path if available
	if configPath, err := config.GetPath(); err == nil {
		statusBar.SetConfigPath(configPath)
	}

	// Create context bar for view-specific info
	contextBar := components.NewContextBar(appStyles)

	// Create help modal
	helpModal := components.NewHelpModal(appStyles)

	// Create theme picker
	availableThemes := styles.ListAvailableThemes()
	themePicker := components.NewThemePicker(appStyles, availableThemes, cfg.GetTheme())

	// Create poller with configured interval
	interval := time.Duration(cfg.PollingInterval) * time.Second
	if interval <= 0 {
		interval = polling.DefaultInterval
	}
	poller := polling.NewPoller(client, interval)

	// Create error handler
	errorHandler := polling.NewErrorHandler()

	// If theme was not found, set a friendly error message
	if themeErr != nil {
		themesDir, _ := styles.GetThemesDirectoryPath()
		themeNotFoundErr := &ThemeNotFoundError{
			ThemeName:  requestedTheme,
			ThemesPath: themesDir,
		}
		errorHandler.SetError(themeNotFoundErr)
	}

	return Model{
		client:           client,
		config:           cfg,
		styles:           appStyles,
		activeTab:        TabPipelines,
		pipelinesView:    pipelines.NewModelWithStyles(client, appStyles),
		pullRequestsView: pullrequests.NewModelWithStyles(client, appStyles),
		workItemsView:    workitems.NewModelWithStyles(client, appStyles),
		statusBar:        statusBar,
		contextBar:       contextBar,
		helpModal:        helpModal,
		themePicker:      themePicker,
		poller:           poller,
		errorHandler:     errorHandler,
	}
}

// Init initializes the application
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.poller.FetchPipelineRuns(), // Initial fetch - updates connection state
		m.poller.StartPolling(),      // Start polling timer
	)
}

// Update handles incoming messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

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
		case "1":
			m.activeTab = TabPipelines
			return m, nil
		case "2":
			if m.activeTab != TabPullRequests {
				m.activeTab = TabPullRequests
				// Trigger initial load when switching to PR tab
				return m, m.pullRequestsView.Init()
			}
			return m, nil
		case "3":
			if m.activeTab != TabWorkItems {
				m.activeTab = TabWorkItems
				// Trigger initial load when switching to Work Items tab
				return m, m.workItemsView.Init()
			}
			return m, nil
		}

	case components.ThemeSelectedMsg:
		// Update theme in config and save
		if err := m.config.UpdateTheme(msg.ThemeName); err != nil {
			// Handle error - could set an error message in status bar
			m.errorHandler.SetError(fmt.Errorf("failed to save theme: %w", err))
			return m, nil
		}

		// Load the new theme
		theme, err := styles.GetThemeByName(msg.ThemeName)
		if err != nil {
			// Handle error - this shouldn't happen as we selected from available themes
			m.errorHandler.SetError(fmt.Errorf("failed to load theme: %w", err))
			return m, nil
		}

		// Create new styles with the selected theme
		m.styles = styles.NewStyles(theme)

		// Update all components with new styles
		m.statusBar = components.NewStatusBar(m.styles)
		m.statusBar.SetOrganization(m.config.Organization)
		m.statusBar.SetProject(m.config.Project)
		m.statusBar.SetWidth(m.width)
		if configPath, err := config.GetPath(); err == nil {
			m.statusBar.SetConfigPath(configPath)
		}

		m.contextBar = components.NewContextBar(m.styles)
		m.contextBar.SetWidth(m.width)

		m.helpModal = components.NewHelpModal(m.styles)
		m.helpModal.SetSize(m.width, m.height)

		// Update theme picker with new styles and current theme
		availableThemes := styles.ListAvailableThemes()
		m.themePicker = components.NewThemePicker(m.styles, availableThemes, msg.ThemeName)

		// Recreate views with new styles
		m.pipelinesView = pipelines.NewModelWithStyles(m.client, m.styles)
		m.pullRequestsView = pullrequests.NewModelWithStyles(m.client, m.styles)
		m.workItemsView = workitems.NewModelWithStyles(m.client, m.styles)

		// CRITICAL: Set window size for all views before they try to render
		if m.width > 0 && m.height > 0 {
			sizeMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height}
			m.pipelinesView, _ = m.pipelinesView.Update(sizeMsg)
			m.pullRequestsView, _ = m.pullRequestsView.Update(sizeMsg)
			m.workItemsView, _ = m.workItemsView.Update(sizeMsg)
		}

		// Re-initialize views to fetch data again
		cmds = append(cmds, m.pipelinesView.Init())
		if m.activeTab == TabPullRequests {
			cmds = append(cmds, m.pullRequestsView.Init())
		}
		if m.activeTab == TabWorkItems {
			cmds = append(cmds, m.workItemsView.Init())
		}

		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.SetWidth(msg.Width)
		m.contextBar.SetWidth(msg.Width)
		m.helpModal.SetSize(msg.Width, msg.Height)
		m.themePicker.SetSize(msg.Width, msg.Height)
		// Pass size to all views so they're ready when switched to
		m.pipelinesView, _ = m.pipelinesView.Update(msg)
		m.pullRequestsView, _ = m.pullRequestsView.Update(msg)
		m.workItemsView, _ = m.workItemsView.Update(msg)

	case polling.TickMsg:
		// Time to poll for updates
		cmds = append(cmds, m.poller.OnTick())

	case polling.PipelineRunsUpdated:
		// Process the update through error handler
		runs, hasError := m.errorHandler.ProcessUpdate(msg)

		if hasError {
			m.statusBar.SetState(polling.StateError)
		} else {
			m.statusBar.SetState(polling.StateConnected)
		}

		// Update pipelines view with the runs
		if runs != nil {
			pipelineMsg := pipelines.SetRunsMsg{Runs: runs}
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
	default:
		m.pipelinesView, cmd = m.pipelinesView.Update(msg)
	}
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// Note: Tab styles are now provided by the styles package and accessed via m.styles

// renderTabBar renders the tab header
func (m Model) renderTabBar() string {
	var tab1, tab2, tab3 string

	switch m.activeTab {
	case TabPipelines:
		tab1 = m.styles.TabActive.Render("1: Pipelines")
		tab2 = m.styles.TabInactive.Render("2: Pull Requests")
		tab3 = m.styles.TabInactive.Render("3: Work Items")
	case TabPullRequests:
		tab1 = m.styles.TabInactive.Render("1: Pipelines")
		tab2 = m.styles.TabActive.Render("2: Pull Requests")
		tab3 = m.styles.TabInactive.Render("3: Work Items")
	case TabWorkItems:
		tab1 = m.styles.TabInactive.Render("1: Pipelines")
		tab2 = m.styles.TabInactive.Render("2: Pull Requests")
		tab3 = m.styles.TabActive.Render("3: Work Items")
	}

	return m.styles.TabBar.Render(tab1+" "+tab2+" "+tab3) + "\n"
}

// View renders the application UI
func (m Model) View() string {
	if m.err != nil {
		return "Error: " + m.err.Error() + "\n\nPress q to quit."
	}

	// If help modal is visible, show it as overlay
	if m.helpModal.IsVisible() {
		return m.helpModal.View()
	}

	// If theme picker is visible, show it as overlay
	if m.themePicker.IsVisible() {
		return m.themePicker.View()
	}

	// Render tab bar
	tabBar := m.renderTabBar()

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
	default:
		content = m.pipelinesView.View()
		hasContextBar = m.pipelinesView.HasContextBar()
		contextItems = m.pipelinesView.GetContextItems()
		scrollPercent = m.pipelinesView.GetScrollPercent()
		statusMessage = m.pipelinesView.GetStatusMessage()
	}

	// Build footer section
	var footer string

	// Show context bar above footer when in detail/log views
	if hasContextBar {
		m.contextBar.Clear()
		m.contextBar.SetItems(contextItems)
		m.contextBar.ShowScrollPercent(true)
		m.contextBar.SetScrollPercent(scrollPercent)

		if statusMessage != "" {
			m.contextBar.SetStatus(statusMessage)
		}

		m.statusBar.ShowScrollPercent(false)
		footer = m.contextBar.View() + "\n" + m.statusBar.View()
	} else {
		// Show scroll percent in status bar for views without context bar
		// (e.g., PR detail view which has scrollable content)
		if scrollPercent > 0 {
			m.statusBar.ShowScrollPercent(true)
			m.statusBar.SetScrollPercent(scrollPercent)
		} else {
			m.statusBar.ShowScrollPercent(false)
		}
		footer = m.statusBar.View()
	}

	return tabBar + content + "\n" + footer
}
