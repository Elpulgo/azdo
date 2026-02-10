package patinput

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

// PATSubmittedMsg is sent when a PAT has been successfully submitted
type PATSubmittedMsg struct {
	PAT string
}

// Model represents the PAT input view model
type Model struct {
	textInput textinput.Model
	err       string
	submitted bool
}

// NewModel creates a new PAT input model
func NewModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Enter your Azure DevOps Personal Access Token"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 60
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'

	return Model{
		textInput: ti,
		err:       "",
		submitted: false,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Validate PAT is not empty
			pat := m.textInput.Value()
			if pat == "" {
				m.err = "PAT cannot be empty"
				return m, nil
			}

			// Mark as submitted and return the PAT
			m.submitted = true
			m.err = ""
			return m, func() tea.Msg {
				return PATSubmittedMsg{PAT: pat}
			}

		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		}
	}

	// Update text input
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the PAT input view
func (m Model) View() string {
	var s string

	s += titleStyle.Render("Azure DevOps Personal Access Token Setup") + "\n\n"
	s += "No PAT found in keyring. Please enter your Personal Access Token:\n\n"
	s += m.textInput.View() + "\n\n"

	if m.err != "" {
		s += errorStyle.Render("Error: "+m.err) + "\n\n"
	}

	s += helpStyle.Render("Press Enter to submit • Esc to quit")

	return s
}

// GetPAT returns the entered PAT value
func (m Model) GetPAT() string {
	return m.textInput.Value()
}
