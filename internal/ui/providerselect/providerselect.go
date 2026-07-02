// Package providerselect provides a small standalone Bubble Tea model for
// prompting the user to choose a provider (Azure DevOps or GitHub) during
// 'azdo auth'. It mirrors the shape of patinput: exported Model, NewModel(),
// Init/Update/View, Selected() and Cancelled() getters.
package providerselect

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Provider identifies which credential provider was chosen.
type Provider int

const (
	// ProviderAzure represents Azure DevOps.
	ProviderAzure Provider = iota
	// ProviderGitHub represents GitHub.
	ProviderGitHub
)

// String returns a human-readable label for the provider.
func (p Provider) String() string {
	switch p {
	case ProviderAzure:
		return "Azure DevOps"
	case ProviderGitHub:
		return "GitHub"
	default:
		return fmt.Sprintf("Provider(%d)", int(p))
	}
}

var providers = []Provider{ProviderAzure, ProviderGitHub}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Model is the provider-selection Bubble Tea model.
type Model struct {
	cursor    int
	selected  Provider
	chosen    bool
	cancelled bool
}

// NewModel returns a Model with the cursor on Azure DevOps (index 0).
func NewModel() Model {
	return Model{
		cursor:   0,
		selected: ProviderAzure,
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles key presses: j/down moves down, k/up moves up, enter
// selects, esc/ctrl+c cancels.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(providers)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			m.selected = providers[m.cursor]
			m.chosen = true
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the provider list.
func (m Model) View() string {
	s := titleStyle.Render("Select a provider to authenticate") + "\n\n"

	for i, p := range providers {
		cursor := "  "
		if m.cursor == i {
			cursor = "> "
			s += cursor + selectedStyle.Render(p.String()) + "\n"
		} else {
			s += cursor + p.String() + "\n"
		}
	}

	s += "\n" + helpStyle.Render("↑/k up • ↓/j down • enter select • esc cancel")
	return s
}

// Selected returns the chosen provider. Only meaningful when Cancelled() is
// false and the program has finished running.
func (m Model) Selected() Provider {
	return m.selected
}

// Cancelled returns true when the user pressed esc or ctrl+c.
func (m Model) Cancelled() bool {
	return m.cancelled
}
