package components

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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
	styles      *styles.Styles
	visible     bool
	width       int
	height      int
	options     []tagOption
	cursor      int
	activeTag   string
	searchInput textinput.Model
}

// NewTagPicker creates a new tag picker
func NewTagPicker(s *styles.Styles) TagPicker {
	ti := textinput.New()
	ti.Prompt = "🔍 "
	ti.Placeholder = "search tags..."
	ti.CharLimit = 100

	return TagPicker{
		styles:      s,
		visible:     false,
		cursor:      0,
		searchInput: ti,
	}
}

// SetTags sets the available tags and positions the cursor on the active tag.
// When activeTag is non-empty, a "Clear filter" option is prepended.
func (tp *TagPicker) SetTags(tags []string, activeTag string) {
	tp.activeTag = activeTag
	tp.cursor = 0
	tp.searchInput.SetValue("")

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
	tp.searchInput.Focus()
}

// Hide makes the tag picker invisible
func (tp *TagPicker) Hide() {
	tp.visible = false
	tp.searchInput.Blur()
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

// SearchQuery returns the current search query text (for testing and status display).
func (tp TagPicker) SearchQuery() string {
	return tp.searchInput.Value()
}

// visibleOptions returns the options filtered by the current search query.
// The "Clear filter" entry is always retained when present so users can reset
// the filter without clearing the search first.
func (tp TagPicker) visibleOptions() []tagOption {
	query := strings.ToLower(strings.TrimSpace(tp.searchInput.Value()))
	if query == "" {
		return tp.options
	}
	filtered := make([]tagOption, 0, len(tp.options))
	for _, opt := range tp.options {
		if opt.IsClear || strings.Contains(strings.ToLower(opt.Name), query) {
			filtered = append(filtered, opt)
		}
	}
	return filtered
}

// Update handles messages
func (tp TagPicker) Update(msg tea.Msg) (TagPicker, tea.Cmd) {
	if !tp.visible {
		return tp, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return tp, nil
	}

	switch {
	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("esc"))):
		tp.visible = false
		tp.searchInput.Blur()
		return tp, nil

	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up"))):
		if tp.cursor > 0 {
			tp.cursor--
		}
		return tp, nil

	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down"))):
		opts := tp.visibleOptions()
		if tp.cursor < len(opts)-1 {
			tp.cursor++
		}
		return tp, nil

	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
		opts := tp.visibleOptions()
		if len(opts) == 0 || tp.cursor >= len(opts) {
			return tp, nil
		}
		selected := opts[tp.cursor]
		tp.visible = false
		tp.searchInput.Blur()
		tag := selected.Name
		if selected.IsClear {
			tag = ""
		}
		return tp, func() tea.Msg {
			return TagSelectedMsg{Tag: tag}
		}
	}

	prev := tp.searchInput.Value()
	var cmd tea.Cmd
	tp.searchInput, cmd = tp.searchInput.Update(keyMsg)
	if tp.searchInput.Value() != prev {
		tp.cursor = 0
	}
	return tp, cmd
}

// View renders the tag picker
func (tp TagPicker) View() string {
	if !tp.visible {
		return ""
	}

	titleText := "Filter by Tag"
	helpTextStr := "type to search • ↑/↓: navigate • enter: select • esc: cancel"

	opts := tp.visibleOptions()
	searchView := tp.searchInput.View()

	maxWidth := minModalWidth
	if len(titleText) > maxWidth {
		maxWidth = len(titleText)
	}
	if len(helpTextStr) > maxWidth {
		maxWidth = len(helpTextStr)
	}
	if lipgloss.Width(searchView) > maxWidth {
		maxWidth = lipgloss.Width(searchView)
	}

	for _, opt := range opts {
		lineLen := len(fmt.Sprintf("> ● %s", opt.Name))
		if lineLen > maxWidth {
			maxWidth = lineLen
		}
	}

	var optionList string
	if len(opts) == 0 {
		optionList = lipgloss.NewStyle().
			Foreground(tp.styles.Theme.GetForegroundMuted()).
			Background(tp.styles.Theme.GetBackground()).
			Italic(true).
			Width(maxWidth).
			Render("  no matching tags") + "\n"
	}
	for i, opt := range opts {
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

	searchBar := lipgloss.NewStyle().
		Background(tp.styles.Theme.GetBackground()).
		Width(maxWidth).
		Render(searchView)

	helpText := lipgloss.NewStyle().
		Foreground(tp.styles.Theme.GetForegroundMuted()).
		Background(tp.styles.Theme.GetBackground()).
		Width(maxWidth).
		Render(helpTextStr)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		searchBar,
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
