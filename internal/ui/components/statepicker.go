package components

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StateSelectedMsg is sent when a state option is selected
type StateSelectedMsg struct {
	State string
}

// stateOption represents a single state choice in the picker
type stateOption struct {
	Name      string
	Icon      string
	IsCurrent bool
}

// StatePicker is a modal component for selecting a work item state
type StatePicker struct {
	styles       *styles.Styles
	visible      bool
	width        int
	height       int
	options      []stateOption
	cursor       int
	currentState string
}

// NewStatePicker creates a new state picker
func NewStatePicker(s *styles.Styles) StatePicker {
	return StatePicker{
		styles:  s,
		visible: false,
		cursor:  0,
	}
}

// SetStates sets the available states and positions the cursor on the current state
func (sp *StatePicker) SetStates(states []azdevops.WorkItemTypeState, currentState string) {
	sp.currentState = currentState
	sp.options = make([]stateOption, len(states))
	sp.cursor = 0

	for i, s := range states {
		sp.options[i] = stateOption{
			Name:      s.Name,
			Icon:      stateIcon(s.Category),
			IsCurrent: s.Name == currentState,
		}
		if s.Name == currentState {
			sp.cursor = i
		}
	}
}

// Show makes the state picker visible
func (sp *StatePicker) Show() {
	sp.visible = true
}

// Hide makes the state picker invisible
func (sp *StatePicker) Hide() {
	sp.visible = false
}

// IsVisible returns whether the state picker is visible
func (sp StatePicker) IsVisible() bool {
	return sp.visible
}

// SetSize sets the dimensions for centering
func (sp *StatePicker) SetSize(width, height int) {
	sp.width = width
	sp.height = height
}

// GetCursor returns the current cursor position
func (sp StatePicker) GetCursor() int {
	return sp.cursor
}

// Update handles messages
func (sp StatePicker) Update(msg tea.Msg) (StatePicker, tea.Cmd) {
	if !sp.visible {
		return sp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			sp.visible = false
			return sp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if sp.cursor > 0 {
				sp.cursor--
			}
			return sp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if sp.cursor < len(sp.options)-1 {
				sp.cursor++
			}
			return sp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(sp.options) == 0 {
				return sp, nil
			}
			selected := sp.options[sp.cursor]
			sp.visible = false
			return sp, func() tea.Msg {
				return StateSelectedMsg{State: selected.Name}
			}
		}
	}

	return sp, nil
}

// View renders the state picker
func (sp StatePicker) View() string {
	if !sp.visible {
		return ""
	}

	titleText := "Change Work Item State"
	helpTextStr := "↑/↓: navigate • enter: select • esc/q: cancel"

	maxWidth := minModalWidth
	if len(titleText) > maxWidth {
		maxWidth = len(titleText)
	}
	if len(helpTextStr) > maxWidth {
		maxWidth = len(helpTextStr)
	}

	for _, opt := range sp.options {
		label := opt.Name
		if opt.IsCurrent {
			label += " (current)"
		}
		lineLen := len(fmt.Sprintf("> %s %s", opt.Icon, label))
		if lineLen > maxWidth {
			maxWidth = lineLen
		}
	}

	var optionList string
	for i, opt := range sp.options {
		cursor := " "
		if i == sp.cursor {
			cursor = ">"
		}

		label := opt.Name
		if opt.IsCurrent {
			label += " (current)"
		}

		line := fmt.Sprintf("%s %s %s", cursor, opt.Icon, label)

		if i == sp.cursor {
			line = lipgloss.NewStyle().
				Foreground(sp.styles.Theme.GetSelectForeground()).
				Background(sp.styles.Theme.GetSelectBackground()).
				Width(maxWidth).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(sp.styles.Theme.GetForeground()).
				Background(sp.styles.Theme.GetBackground()).
				Width(maxWidth).
				Render(line)
		}

		optionList += line + "\n"
	}

	title := lipgloss.NewStyle().
		Foreground(sp.styles.Theme.GetPrimary()).
		Background(sp.styles.Theme.GetBackground()).
		Bold(true).
		Width(maxWidth).
		Render(titleText)

	helpText := lipgloss.NewStyle().
		Foreground(sp.styles.Theme.GetForegroundMuted()).
		Background(sp.styles.Theme.GetBackground()).
		Width(maxWidth).
		Render(helpTextStr)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		optionList,
		helpText,
	)

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(sp.styles.Theme.GetBorder()).
		Padding(1, 2).
		Background(sp.styles.Theme.GetBackground())

	modal := modalStyle.Render(content)

	if sp.width > 0 && sp.height > 0 {
		modal = lipgloss.Place(
			sp.width,
			sp.height,
			lipgloss.Center,
			lipgloss.Center,
			modal,
		)
	}

	return modal
}

// stateIcon returns an icon for the work item state category
func stateIcon(category string) string {
	switch category {
	case "Proposed":
		return "○"
	case "InProgress":
		return "◐"
	case "Resolved":
		return "●"
	case "Completed":
		return "✓"
	default:
		return "○"
	}
}
