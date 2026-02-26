package components

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// VoteSelectedMsg is sent when a vote option is selected
type VoteSelectedMsg struct {
	Vote int
}

// VoteOption represents a single vote choice
type VoteOption struct {
	Label string
	Icon  string
	Vote  int
}

// VotePicker is a modal component for selecting a PR vote
type VotePicker struct {
	styles  *styles.Styles
	visible bool
	width   int
	height  int
	options []VoteOption
	cursor  int
}

// NewVotePicker creates a new vote picker
func NewVotePicker(s *styles.Styles) VotePicker {
	return VotePicker{
		styles:  s,
		visible: false,
		options: []VoteOption{
			{Label: "Approve", Icon: "✓", Vote: azdevops.VoteApprove},
			{Label: "Approve with suggestions", Icon: "~", Vote: azdevops.VoteApproveWithSuggestions},
			{Label: "Wait for author", Icon: "◐", Vote: azdevops.VoteWaitForAuthor},
			{Label: "Reject", Icon: "✗", Vote: azdevops.VoteReject},
			{Label: "Reset feedback", Icon: "○", Vote: azdevops.VoteNoVote},
		},
		cursor: 0,
	}
}

// Show makes the vote picker visible
func (v *VotePicker) Show() {
	v.visible = true
}

// Hide makes the vote picker invisible
func (v *VotePicker) Hide() {
	v.visible = false
}

// IsVisible returns whether the vote picker is visible
func (v VotePicker) IsVisible() bool {
	return v.visible
}

// SetSize sets the dimensions for centering
func (v *VotePicker) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// GetCursor returns the current cursor position
func (v VotePicker) GetCursor() int {
	return v.cursor
}

// Update handles messages
func (v VotePicker) Update(msg tea.Msg) (VotePicker, tea.Cmd) {
	if !v.visible {
		return v, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			v.visible = false
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if v.cursor > 0 {
				v.cursor--
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if v.cursor < len(v.options)-1 {
				v.cursor++
			}
			return v, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			selected := v.options[v.cursor]
			v.visible = false
			return v, func() tea.Msg {
				return VoteSelectedMsg{Vote: selected.Vote}
			}
		}
	}

	return v, nil
}

// View renders the vote picker
func (v VotePicker) View() string {
	if !v.visible {
		return ""
	}

	titleText := "Vote on Pull Request"
	helpTextStr := "↑/↓: navigate • enter: select • esc/q: cancel"

	maxWidth := minModalWidth
	if len(titleText) > maxWidth {
		maxWidth = len(titleText)
	}
	if len(helpTextStr) > maxWidth {
		maxWidth = len(helpTextStr)
	}

	for _, opt := range v.options {
		lineLen := len(fmt.Sprintf("> %s %s", opt.Icon, opt.Label))
		if lineLen > maxWidth {
			maxWidth = lineLen
		}
	}

	var optionList string
	for i, opt := range v.options {
		cursor := " "
		if i == v.cursor {
			cursor = ">"
		}

		line := fmt.Sprintf("%s %s %s", cursor, opt.Icon, opt.Label)

		if i == v.cursor {
			line = lipgloss.NewStyle().
				Foreground(v.styles.Theme.GetSelectForeground()).
				Background(v.styles.Theme.GetSelectBackground()).
				Width(maxWidth).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(v.styles.Theme.GetForeground()).
				Background(v.styles.Theme.GetBackground()).
				Width(maxWidth).
				Render(line)
		}

		optionList += line + "\n"
	}

	title := lipgloss.NewStyle().
		Foreground(v.styles.Theme.GetPrimary()).
		Background(v.styles.Theme.GetBackground()).
		Bold(true).
		Width(maxWidth).
		Render(titleText)

	helpText := lipgloss.NewStyle().
		Foreground(v.styles.Theme.GetForegroundMuted()).
		Background(v.styles.Theme.GetBackground()).
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
		BorderForeground(v.styles.Theme.GetBorder()).
		Padding(1, 2).
		Background(v.styles.Theme.GetBackground())

	modal := modalStyle.Render(content)

	if v.width > 0 && v.height > 0 {
		modal = lipgloss.Place(
			v.width,
			v.height,
			lipgloss.Center,
			lipgloss.Center,
			modal,
		)
	}

	return modal
}
