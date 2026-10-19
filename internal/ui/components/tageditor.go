package components

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TagAddMsg is emitted when the user presses Enter with text in the input.
// Tag is the raw input; the parent validates and normalises it.
type TagAddMsg struct {
	Tag string
}

// TagRemoveMsg is emitted when the user presses Delete/Ctrl+D on a
// highlighted existing tag.
type TagRemoveMsg struct {
	Tag string
}

// TagEditorClosedMsg is emitted when the user closes the editor (Esc).
type TagEditorClosedMsg struct{}

// TagEditor is an inline editor listing an item's current tags above a
// single-line input. Enter adds the typed tag. While the input is empty,
// up/down highlight an existing tag and Delete/Ctrl+D removes it. Esc closes.
type TagEditor struct {
	styles  *styles.Styles
	input   textinput.Model
	tags    []string
	cursor  int
	errMsg  string
	visible bool
}

// NewTagEditor creates a new, hidden tag editor.
func NewTagEditor(s *styles.Styles) TagEditor {
	ti := textinput.New()
	ti.Prompt = "+ "
	ti.Placeholder = "new tag..."
	ti.CharLimit = 100

	return TagEditor{
		styles: s,
		input:  ti,
	}
}

// Show makes the editor visible with the given current tags, clearing any
// previous input and error.
func (e *TagEditor) Show(tags []string) {
	e.visible = true
	e.input.Reset()
	e.errMsg = ""
	e.SetTags(tags)
}

// Hide makes the editor invisible and blurs the input.
func (e *TagEditor) Hide() {
	e.visible = false
	e.input.Blur()
}

// IsVisible returns whether the editor is visible.
func (e TagEditor) IsVisible() bool {
	return e.visible
}

// Focus focuses the input so it captures keystrokes.
func (e *TagEditor) Focus() tea.Cmd {
	return e.input.Focus()
}

// SetTags replaces the listed tags, keeping the highlight in range.
func (e *TagEditor) SetTags(tags []string) {
	e.tags = tags
	if e.cursor >= len(e.tags) {
		e.cursor = max(len(e.tags)-1, 0)
	}
}

// Tags returns the listed tags.
func (e TagEditor) Tags() []string {
	return e.tags
}

// SetError sets the inline error shown below the input. Empty clears it.
func (e *TagEditor) SetError(msg string) {
	e.errMsg = msg
}

// Error returns the inline error currently shown.
func (e TagEditor) Error() string {
	return e.errMsg
}

// Value returns the current input text.
func (e TagEditor) Value() string {
	return e.input.Value()
}

// ClearInput empties the input (used after a successful add).
func (e *TagEditor) ClearInput() {
	e.input.Reset()
}

// SetWidth sizes the input to the available width.
func (e *TagEditor) SetWidth(width int) {
	w := width - 8
	if w < 10 {
		w = 10
	}
	e.input.Width = w
}

// Height returns the number of terminal rows the editor occupies when visible:
// the tag list (at least one row for the empty hint), the input, the error
// line when set, the top/bottom border and the help line.
func (e TagEditor) Height() int {
	rows := max(len(e.tags), 1) + 1
	if e.errMsg != "" {
		rows++
	}
	return rows + 3
}

// Update handles key input while the editor is visible.
func (e TagEditor) Update(msg tea.Msg) (TagEditor, tea.Cmd) {
	if !e.visible {
		return e, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			// Hide synchronously so the dispatched TagEditorClosedMsg is
			// handled by the parent rather than re-captured by this editor.
			e.Hide()
			return e, func() tea.Msg { return TagEditorClosedMsg{} }
		case "enter":
			raw := e.input.Value()
			if raw == "" {
				return e, nil
			}
			return e, func() tea.Msg { return TagAddMsg{Tag: raw} }
		case "up":
			if e.input.Value() == "" {
				if e.cursor > 0 {
					e.cursor--
				}
				return e, nil
			}
		case "down":
			if e.input.Value() == "" {
				if e.cursor < len(e.tags)-1 {
					e.cursor++
				}
				return e, nil
			}
		case "delete", "ctrl+d":
			if e.input.Value() == "" && len(e.tags) > 0 {
				tag := e.tags[e.cursor]
				return e, func() tea.Msg { return TagRemoveMsg{Tag: tag} }
			}
		}
	}

	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return e, cmd
}

// View renders the inline editor.
func (e TagEditor) View() string {
	if !e.visible {
		return ""
	}

	muted := lipgloss.NewStyle().Foreground(e.styles.Theme.GetForegroundMuted())

	var body string
	if len(e.tags) == 0 {
		body = muted.Italic(true).Render("  no tags") + "\n"
	}
	for i, tag := range e.tags {
		line := fmt.Sprintf("  %s", tag)
		if i == e.cursor && e.input.Value() == "" {
			line = lipgloss.NewStyle().
				Foreground(e.styles.Theme.GetForeground()).
				Bold(true).
				Render(fmt.Sprintf("> %s", tag))
		}
		body += line + "\n"
	}
	body += e.input.View()
	if e.errMsg != "" {
		body += "\n" + e.styles.Error.Render(e.errMsg)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(e.styles.Theme.GetBorder()).
		Render(body)

	helpText := muted.Render("Enter: add • ↑/↓: select • Del/Ctrl+D: remove • Esc: close")

	return lipgloss.JoinVertical(lipgloss.Left, box, helpText)
}
