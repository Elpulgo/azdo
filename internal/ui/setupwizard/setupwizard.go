package setupwizard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type wizardStep int

const (
	stepOrganization    wizardStep = iota
	stepProjects                   // comma-separated
	stepPollingInterval            // pre-filled with default
	stepTheme                      // cursor-based selection
	stepConfirm                    // summary + save
)

const totalSteps = 5

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	stepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Bold(true)
)

// Model is the Bubbletea model for the setup wizard.
type Model struct {
	step      wizardStep
	orgInput  textinput.Model
	projInput textinput.Model
	pollInput textinput.Model

	themes      []string
	themeCursor int

	err       string
	cancelled bool
	done      bool

	// collected values
	organization    string
	projects        []string
	pollingInterval int
	theme           string
}

// NewModel creates a new setup wizard model with themes from the style system.
func NewModel() Model {
	orgInput := textinput.New()
	orgInput.Placeholder = "e.g. my-organization"
	orgInput.Focus()
	orgInput.CharLimit = 200
	orgInput.Width = 60

	projInput := textinput.New()
	projInput.Placeholder = "e.g. project-a, project-b"
	projInput.CharLimit = 500
	projInput.Width = 60

	pollInput := textinput.New()
	pollInput.Placeholder = "seconds"
	pollInput.CharLimit = 10
	pollInput.Width = 20
	pollInput.SetValue(strconv.Itoa(config.DefaultPollingInterval))

	return Model{
		step:      stepOrganization,
		orgInput:  orgInput,
		projInput: projInput,
		pollInput: pollInput,
		themes:    styles.ListAvailableThemes(),
	}
}

// Init starts the text cursor blink.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles key messages and drives the wizard state machine.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global cancel
		if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			return m, tea.Quit
		}

		switch m.step {
		case stepOrganization:
			return m.updateOrganization(msg)
		case stepProjects:
			return m.updateProjects(msg)
		case stepPollingInterval:
			return m.updatePollingInterval(msg)
		case stepTheme:
			return m.updateTheme(msg)
		case stepConfirm:
			return m.updateConfirm(msg)
		}
	}

	return m, nil
}

func (m Model) updateOrganization(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.orgInput.Value())
		if val == "" {
			m.err = "Organization cannot be empty"
			return m, nil
		}
		m.organization = val
		m.err = ""
		m.step = stepProjects
		m.projInput.Focus()
		m.orgInput.Blur()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.orgInput, cmd = m.orgInput.Update(msg)
	return m, cmd
}

func (m Model) updateProjects(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.projInput.Value())
		if val == "" {
			m.err = "Projects cannot be empty"
			return m, nil
		}
		parts := strings.Split(val, ",")
		projects := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				projects = append(projects, trimmed)
			}
		}
		if len(projects) == 0 {
			m.err = "At least one project is required"
			return m, nil
		}
		m.projects = projects
		m.err = ""
		m.step = stepPollingInterval
		m.projInput.Blur()
		m.pollInput.Focus()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.projInput, cmd = m.projInput.Update(msg)
	return m, cmd
}

func (m Model) updatePollingInterval(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.pollInput.Value())
		n, err := strconv.Atoi(val)
		if err != nil {
			m.err = "Polling interval must be a number"
			return m, nil
		}
		if n <= 0 {
			m.err = "Polling interval must be greater than 0"
			return m, nil
		}
		m.pollingInterval = n
		m.err = ""
		m.step = stepTheme
		m.pollInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.pollInput, cmd = m.pollInput.Update(msg)
	return m, cmd
}

func (m Model) updateTheme(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.themeCursor > 0 {
			m.themeCursor--
		}
	case tea.KeyDown:
		if m.themeCursor < len(m.themes)-1 {
			m.themeCursor++
		}
	case tea.KeyEnter:
		m.theme = m.themes[m.themeCursor]
		m.err = ""
		m.step = stepConfirm
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.done = true
		return m, tea.Quit
	case tea.KeyRunes:
		if string(msg.Runes) == "b" {
			m.step = stepOrganization
			m.orgInput.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

// View renders the current wizard step.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(strings.Join(components.LogoArt, "\n")))
	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render("Setup Wizard"))
	b.WriteString("\n")
	b.WriteString(stepStyle.Render(fmt.Sprintf("Step %d of %d", int(m.step)+1, totalSteps)))
	b.WriteString("\n\n")

	switch m.step {
	case stepOrganization:
		b.WriteString(labelStyle.Render("Organization"))
		b.WriteString("\n")
		b.WriteString("Enter your Azure DevOps organization name:\n\n")
		b.WriteString(m.orgInput.View())
	case stepProjects:
		b.WriteString(labelStyle.Render("Projects"))
		b.WriteString("\n")
		b.WriteString("Enter project names (comma-separated):\n\n")
		b.WriteString(m.projInput.View())
	case stepPollingInterval:
		b.WriteString(labelStyle.Render("Polling Interval"))
		b.WriteString("\n")
		b.WriteString("How often to refresh data (in seconds):\n\n")
		b.WriteString(m.pollInput.View())
	case stepTheme:
		b.WriteString(labelStyle.Render("Theme"))
		b.WriteString("\n")
		b.WriteString("Select a color theme:\n\n")
		for i, t := range m.themes {
			if i == m.themeCursor {
				b.WriteString(selectedStyle.Render("▸ " + t))
			} else {
				b.WriteString("  " + t)
			}
			b.WriteString("\n")
		}
	case stepConfirm:
		b.WriteString(labelStyle.Render("Confirm"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  Organization:     %s\n", m.organization))
		b.WriteString(fmt.Sprintf("  Projects:         %s\n", strings.Join(m.projects, ", ")))
		b.WriteString(fmt.Sprintf("  Polling interval: %ds\n", m.pollingInterval))
		b.WriteString(fmt.Sprintf("  Theme:            %s\n", m.theme))
	}

	b.WriteString("\n\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: " + m.err))
		b.WriteString("\n\n")
	}

	switch m.step {
	case stepConfirm:
		b.WriteString(helpStyle.Render("Enter to save • b to go back • Esc to cancel"))
	case stepTheme:
		b.WriteString(helpStyle.Render("↑/↓ to navigate • Enter to select • Esc to cancel"))
	default:
		b.WriteString(helpStyle.Render("Enter to continue • Esc to cancel"))
	}

	return b.String()
}

// GetConfig returns the collected configuration after the wizard completes.
// Returns nil if the wizard was cancelled or hasn't completed yet.
func (m Model) GetConfig() *config.Config {
	if m.cancelled || !m.done {
		return nil
	}

	configPath, err := config.GetPath()
	if err != nil {
		return nil
	}

	return config.NewWithPath(
		m.organization,
		m.projects,
		m.pollingInterval,
		m.theme,
		configPath,
	)
}

// Cancelled returns true if the user cancelled the wizard.
func (m Model) Cancelled() bool {
	return m.cancelled
}
