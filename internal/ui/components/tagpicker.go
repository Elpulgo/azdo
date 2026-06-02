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

// TagSelectedMsg is sent when a single tag is selected (single-select mode).
// An empty Tag means "clear filter".
type TagSelectedMsg struct {
	Tag string
}

// TagsSelectedMsg is sent when a multi-select picker is confirmed with enter.
// An empty slice means "clear selection".
type TagsSelectedMsg struct {
	Tags []string
}

// tagOption represents a single tag choice in the picker
type tagOption struct {
	Name     string
	IsClear  bool // true for the "Clear filter" option
	Selected bool // multi-select mode: whether this tag is currently chosen
}

// TagPicker is a modal component for selecting tag(s) to filter by. Defaults
// to single-select; call SetTagsMulti to switch to multi-select mode.
type TagPicker struct {
	styles      *styles.Styles
	visible     bool
	width       int
	height      int
	options     []tagOption
	cursor      int
	activeTag   string
	multiSelect bool
	title       string // overridden in multi-select mode
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
// Single-select mode. When activeTag is non-empty, a "Clear filter" option is
// prepended.
func (tp *TagPicker) SetTags(tags []string, activeTag string) {
	tp.multiSelect = false
	tp.title = "Filter by Tag"
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
			offset := 0
			if activeTag != "" {
				offset = 1
			}
			tp.cursor = i + offset
		}
	}
}

// SetTagsMulti switches the picker into multi-select mode. `selected` is the
// initial set of chosen tags. Confirm with enter, which emits TagsSelectedMsg.
func (tp *TagPicker) SetTagsMulti(tags []string, selected []string) {
	tp.multiSelect = true
	tp.title = "Pick sprints"
	tp.activeTag = ""
	tp.cursor = 0
	tp.searchInput.SetValue("")

	sel := make(map[string]struct{}, len(selected))
	for _, s := range selected {
		sel[s] = struct{}{}
	}

	tp.options = make([]tagOption, 0, len(tags))
	for _, tag := range tags {
		_, isSelected := sel[tag]
		tp.options = append(tp.options, tagOption{Name: tag, Selected: isSelected})
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

	case key.Matches(keyMsg, key.NewBinding(key.WithKeys(" "))):
		// Space toggles selection in multi-select mode; ignored in single-select.
		if !tp.multiSelect {
			break
		}
		opts := tp.visibleOptions()
		if len(opts) == 0 || tp.cursor >= len(opts) {
			return tp, nil
		}
		name := opts[tp.cursor].Name
		for i := range tp.options {
			if tp.options[i].Name == name {
				tp.options[i].Selected = !tp.options[i].Selected
				break
			}
		}
		return tp, nil

	case key.Matches(keyMsg, key.NewBinding(key.WithKeys("enter"))):
		opts := tp.visibleOptions()
		if tp.multiSelect {
			// Multi-select confirm: emit the full chosen set.
			tp.visible = false
			tp.searchInput.Blur()
			var chosen []string
			for _, o := range tp.options {
				if o.Selected {
					chosen = append(chosen, o.Name)
				}
			}
			return tp, func() tea.Msg {
				return TagsSelectedMsg{Tags: chosen}
			}
		}
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

	titleText := tp.title
	if titleText == "" {
		titleText = "Filter by Tag"
	}
	helpTextStr := "type to search • ↑/↓: navigate • enter: select • esc: cancel"
	if tp.multiSelect {
		helpTextStr = "type to search • ↑/↓: navigate • space: toggle • enter: confirm • esc: cancel"
	}

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
		if tp.multiSelect {
			if opt.Selected {
				icon = "☑"
			} else {
				icon = "☐"
			}
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
