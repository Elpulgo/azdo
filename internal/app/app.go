package app

import (
	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/pipelines"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the root application model for the TUI
type Model struct {
	client        *azdevops.Client
	pipelinesView pipelines.Model
	width         int
	height        int
	err           error
}

// NewModel creates a new application model with the given Azure DevOps client
func NewModel(client *azdevops.Client) Model {
	return Model{
		client:        client,
		pipelinesView: pipelines.NewModel(client),
	}
}

// Init initializes the application
func (m Model) Init() tea.Cmd {
	return m.pipelinesView.Init()
}

// Update handles incoming messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	// Delegate to pipelines view
	m.pipelinesView, cmd = m.pipelinesView.Update(msg)
	return m, cmd
}

// View renders the application UI
func (m Model) View() string {
	if m.err != nil {
		return "Error: " + m.err.Error() + "\n\nPress q to quit."
	}

	return m.pipelinesView.View()
}
