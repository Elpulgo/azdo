package components

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const minModalWidth = 80

type ListPickerOption struct {
	Name      string
	Icon      string
	IsCurrent bool
}

type ListPickerSelectedMsg struct {
	Value string
}

type ListPicker struct {
	styles     *styles.Styles
	visible    bool
	width      int
	height     int
	title      string
	options    []ListPickerOption
	cursor     int
	showClear  bool
	allowClear bool
}

func NewListPicker(s *styles.Styles) ListPicker {
	return ListPicker{
		styles:  s,
		visible: false,
		cursor:  0,
	}
}

func (lp *ListPicker) SetConfig(title string, options []ListPickerOption, activeValue string, allowClear bool) {
	lp.title = title
	lp.allowClear = allowClear
	lp.options = nil
	lp.cursor = 0

	hasActive := activeValue != "" && allowClear

	if hasActive {
		lp.options = append(lp.options, ListPickerOption{Name: "Clear filter", Icon: "✕", IsCurrent: false})
	}

	offset := 0
	if hasActive {
		offset = 1
	}

	for i, opt := range options {
		opt.IsCurrent = opt.Name == activeValue
		lp.options = append(lp.options, opt)
		if opt.Name == activeValue {
			lp.cursor = i + offset
		}
	}
}

func (lp *ListPicker) Show() {
	lp.visible = true
}

func (lp *ListPicker) Hide() {
	lp.visible = false
}

func (lp ListPicker) IsVisible() bool {
	return lp.visible
}

func (lp *ListPicker) SetSize(width, height int) {
	lp.width = width
	lp.height = height
}

func (lp ListPicker) GetCursor() int {
	return lp.cursor
}

func (lp ListPicker) Update(msg tea.Msg) (ListPicker, tea.Cmd) {
	if !lp.visible {
		return lp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			lp.visible = false
			return lp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if lp.cursor > 0 {
				lp.cursor--
			}
			return lp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if lp.cursor < len(lp.options)-1 {
				lp.cursor++
			}
			return lp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(lp.options) == 0 {
				return lp, nil
			}
			selected := lp.options[lp.cursor]
			lp.visible = false
			value := selected.Name
			if selected.Name == "Clear filter" {
				value = ""
			}
			return lp, func() tea.Msg {
				return ListPickerSelectedMsg{Value: value}
			}
		}
	}

	return lp, nil
}

func (lp ListPicker) View() string {
	if !lp.visible {
		return ""
	}

	helpTextStr := "↑/↓: navigate • enter: select • esc/q: cancel"

	maxWidth := minModalWidth
	if len(lp.title) > maxWidth {
		maxWidth = len(lp.title)
	}
	if len(helpTextStr) > maxWidth {
		maxWidth = len(helpTextStr)
	}

	for _, opt := range lp.options {
		label := opt.Name
		if opt.IsCurrent {
			label += " (current)"
		}
		icon := opt.Icon
		if icon == "" {
			icon = "●"
		}
		lineLen := len(fmt.Sprintf("> %s %s", icon, label))
		if lineLen > maxWidth {
			maxWidth = lineLen
		}
	}

	var optionList string
	for i, opt := range lp.options {
		cursor := " "
		if i == lp.cursor {
			cursor = ">"
		}

		label := opt.Name
		if opt.IsCurrent {
			label += " (current)"
		}

		icon := opt.Icon
		if icon == "" {
			icon = "●"
		}

		line := fmt.Sprintf("%s %s %s", cursor, icon, label)

		if i == lp.cursor {
			line = lipgloss.NewStyle().
				Foreground(lp.styles.Theme.GetSelectForeground()).
				Background(lp.styles.Theme.GetSelectBackground()).
				Width(maxWidth).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(lp.styles.Theme.GetForeground()).
				Background(lp.styles.Theme.GetBackground()).
				Width(maxWidth).
				Render(line)
		}

		optionList += line + "\n"
	}

	title := lipgloss.NewStyle().
		Foreground(lp.styles.Theme.GetPrimary()).
		Background(lp.styles.Theme.GetBackground()).
		Bold(true).
		Width(maxWidth).
		Render(lp.title)

	helpText := lipgloss.NewStyle().
		Foreground(lp.styles.Theme.GetForegroundMuted()).
		Background(lp.styles.Theme.GetBackground()).
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
		BorderForeground(lp.styles.Theme.GetBorder()).
		Padding(1, 2).
		Background(lp.styles.Theme.GetBackground())

	modal := modalStyle.Render(content)

	if lp.width > 0 && lp.height > 0 {
		modal = lipgloss.Place(
			lp.width,
			lp.height,
			lipgloss.Center,
			lipgloss.Center,
			modal,
		)
	}

	return modal
}
