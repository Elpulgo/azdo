package app

import (
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/pipelines"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the root application model for the TUI
type Model struct {
	client        *azdevops.Client
	config        *config.Config
	pipelinesView pipelines.Model
	statusBar     *components.StatusBar
	poller        *polling.Poller
	errorHandler  *polling.ErrorHandler
	width         int
	height        int
	err           error
}

// NewModel creates a new application model with the given Azure DevOps client and config
func NewModel(client *azdevops.Client, cfg *config.Config) Model {
	// Create status bar with org/project info
	statusBar := components.NewStatusBar()
	statusBar.SetOrganization(cfg.Organization)
	statusBar.SetProject(cfg.Project)

	// Create poller with configured interval
	interval := time.Duration(cfg.PollingInterval) * time.Second
	if interval <= 0 {
		interval = polling.DefaultInterval
	}
	poller := polling.NewPoller(client, interval)

	return Model{
		client:        client,
		config:        cfg,
		pipelinesView: pipelines.NewModel(client),
		statusBar:     statusBar,
		poller:        poller,
		errorHandler:  polling.NewErrorHandler(),
	}
}

// Init initializes the application
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.pipelinesView.Init(),
		m.poller.StartPolling(),
	)
}

// Update handles incoming messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.poller.Stop()
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.SetWidth(msg.Width)

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

	// Delegate to pipelines view
	var cmd tea.Cmd
	m.pipelinesView, cmd = m.pipelinesView.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the application UI
func (m Model) View() string {
	if m.err != nil {
		return "Error: " + m.err.Error() + "\n\nPress q to quit."
	}

	// Reserve space for status bar (1 line)
	contentHeight := m.height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Render content and status bar
	content := m.pipelinesView.View()
	statusBar := m.statusBar.View()

	return content + "\n" + statusBar
}
