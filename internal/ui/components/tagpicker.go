package components

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TagSelectedMsg is sent when a tag option is selected.
// An empty Tag means "clear filter".
type TagSelectedMsg struct {
	Tag string
}

// tagOption represents a single tag choice in the picker
type tagOption struct {
	Name    string
	IsClear bool // true for the "Clear filter" option
}

// TagPicker is a modal component for selecting a tag to filter by
type TagPicker struct {
	styles    *styles.Styles
	visible   bool
	width     int
	height    int
	options   []tagOption
	cursor    int
	activeTag string
}

// NewTagPicker creates a new tag picker
func NewTagPicker(s *styles.Styles) TagPicker {
	return TagPicker{
		styles:  s,
		visible: false,
		cursor:  0,
	}
}

// SetTags sets the available tags and positions the cursor on the active tag.
// When activeTag is non-empty, a "Clear filter" option is prepended.
func (tp *TagPicker) SetTags(tags []string, activeTag string) {
	tp.activeTag = activeTag
	tp.cursor = 0

	tp.options = nil

	if activeTag != "" {
		tp.options = append(tp.options, tagOption{Name: "Clear filter", IsClear: true})
	}

	for i, tag := range tags {
		tp.options = append(tp.options, tagOption{Name: tag})
		if tag == activeTag {
			// +1 offset if "Clear filter" is present
			offset := 0
			if activeTag != "" {
				offset = 1
			}
			tp.cursor = i + offset
		}
	}
}

// Show makes the tag picker visible
func (tp *TagPicker) Show() {
	tp.visible = true
}

// Hide makes the tag picker invisible
func (tp *TagPicker) Hide() {
	tp.visible = false
}

// IsVisible returns whether the tag picker is visible
func (tp TagPicker) IsVisible() bool {
	return tp.visible
}

// SetSize sets the dimensions for centering
func (tp *TagPicker) SetSize(width, height int) {
	tp.width = width
	tp.height = height
}

// GetCursor returns the current cursor position
func (tp TagPicker) GetCursor() int {
	return tp.cursor
}

// Update handles messages
func (tp TagPicker) Update(msg tea.Msg) (TagPicker, tea.Cmd) {
	if !tp.visible {
		return tp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			tp.visible = false
			return tp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if tp.cursor > 0 {
				tp.cursor--
			}
			return tp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if tp.cursor < len(tp.options)-1 {
				tp.cursor++
			}
			return tp, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(tp.options) == 0 {
				return tp, nil
			}
			selected := tp.options[tp.cursor]
			tp.visible = false
			tag := selected.Name
			if selected.IsClear {
				tag = ""
			}
			return tp, func() tea.Msg {
				return TagSelectedMsg{Tag: tag}
			}
		}
	}

	return tp, nil
}

// View renders the tag picker
func (tp TagPicker) View() string {
	if !tp.visible {
		return ""
	}

	titleText := "Filter by Tag"
	helpTextStr := "↑/↓: navigate • enter: select • esc/q: cancel"

	maxWidth := minModalWidth
	if len(titleText) > maxWidth {
		maxWidth = len(titleText)
	}
	if len(helpTextStr) > maxWidth {
		maxWidth = len(helpTextStr)
	}

	for _, opt := range tp.options {
		lineLen := len(fmt.Sprintf("> ● %s", opt.Name))
		if lineLen > maxWidth {
			maxWidth = lineLen
		}
	}

	var optionList string
	for i, opt := range tp.options {
		cursor := " "
		if i == tp.cursor {
			cursor = ">"
		}

		icon := "●"
		if opt.IsClear {
			icon = "✕"
		}

		line := fmt.Sprintf("%s %s %s", cursor, icon, opt.Name)

		if i == tp.cursor {
			line = lipgloss.NewStyle().
				Foreground(tp.styles.Theme.GetSelectForeground()).
				Background(tp.styles.Theme.GetSelectBackground()).
				Width(maxWidth).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(tp.styles.Theme.GetForeground()).
				Background(tp.styles.Theme.GetBackground()).
				Width(maxWidth).
				Render(line)
		}

		optionList += line + "\n"
	}

	title := lipgloss.NewStyle().
		Foreground(tp.styles.Theme.GetPrimary()).
		Background(tp.styles.Theme.GetBackground()).
		Bold(true).
		Width(maxWidth).
		Render(titleText)

	helpText := lipgloss.NewStyle().
		Foreground(tp.styles.Theme.GetForegroundMuted()).
		Background(tp.styles.Theme.GetBackground()).
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
		BorderForeground(tp.styles.Theme.GetBorder()).
		Padding(1, 2).
		Background(tp.styles.Theme.GetBackground())

	modal := modalStyle.Render(content)

	if tp.width > 0 && tp.height > 0 {
		modal = lipgloss.Place(
			tp.width,
			tp.height,
			lipgloss.Center,
			lipgloss.Center,
			modal,
		)
	}

	return modal
}
