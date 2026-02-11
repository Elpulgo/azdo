package app

import (
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/pipelines"
	"github.com/Elpulgo/azdo/internal/ui/pullrequests"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab represents the active tab in the application
type Tab int

const (
	TabPipelines     Tab = iota // Pipelines tab (key '1')
	TabPullRequests             // Pull Requests tab (key '2')
)

// Model is the root application model for the TUI
type Model struct {
	client           *azdevops.Client
	config           *config.Config
	activeTab        Tab
	pipelinesView    pipelines.Model
	pullRequestsView pullrequests.Model
	statusBar        *components.StatusBar
	contextBar       *components.ContextBar
	helpModal        *components.HelpModal
	poller           *polling.Poller
	errorHandler     *polling.ErrorHandler
	width            int
	height           int
	err              error
}

// NewModel creates a new application model with the given Azure DevOps client and config
func NewModel(client *azdevops.Client, cfg *config.Config) Model {
	// Create status bar with org/project info
	statusBar := components.NewStatusBar()
	statusBar.SetOrganization(cfg.Organization)
	statusBar.SetProject(cfg.Project)

	// Create context bar for view-specific info
	contextBar := components.NewContextBar()

	// Create help modal
	helpModal := components.NewHelpModal()

	// Create poller with configured interval
	interval := time.Duration(cfg.PollingInterval) * time.Second
	if interval <= 0 {
		interval = polling.DefaultInterval
	}
	poller := polling.NewPoller(client, interval)

	return Model{
		client:           client,
		config:           cfg,
		activeTab:        TabPipelines,
		pipelinesView:    pipelines.NewModel(client),
		pullRequestsView: pullrequests.NewModel(client),
		statusBar:        statusBar,
		contextBar:       contextBar,
		helpModal:        helpModal,
		poller:           poller,
		errorHandler:     polling.NewErrorHandler(),
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
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.SetWidth(msg.Width)
		m.contextBar.SetWidth(msg.Width)
		m.helpModal.SetSize(msg.Width, msg.Height)
		// Pass size to both views so they're ready when switched to
		m.pipelinesView, _ = m.pipelinesView.Update(msg)
		m.pullRequestsView, _ = m.pullRequestsView.Update(msg)

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
	default:
		m.pipelinesView, cmd = m.pipelinesView.Update(msg)
	}
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// Tab header styles
var (
	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 2).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("236")).
				Padding(0, 2)

	tabBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236"))
)

// renderTabBar renders the tab header
func (m Model) renderTabBar() string {
	var tab1, tab2 string

	if m.activeTab == TabPipelines {
		tab1 = activeTabStyle.Render("1: Pipelines")
		tab2 = inactiveTabStyle.Render("2: Pull Requests")
	} else {
		tab1 = inactiveTabStyle.Render("1: Pipelines")
		tab2 = activeTabStyle.Render("2: Pull Requests")
	}

	return tabBarStyle.Render(tab1 + " " + tab2) + "\n"
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

		footer = m.contextBar.View() + "\n" + m.statusBar.View()
	} else {
		footer = m.statusBar.View()
	}

	return tabBar + content + "\n" + footer
}
