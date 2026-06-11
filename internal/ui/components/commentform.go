package components

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommentSubmittedMsg is emitted when the user submits a comment (Ctrl+S).
type CommentSubmittedMsg struct {
	Text string
}

// CommentFormCancelledMsg is emitted when the user cancels the form (Esc).
type CommentFormCancelledMsg struct{}

// commentFormHeight is the number of textarea rows shown in the form.
const commentFormHeight = 5

// CommentForm is an inline multi-line text form for composing a work item comment.
// Enter inserts a newline; Ctrl+S submits; Esc cancels.
type CommentForm struct {
	styles   *styles.Styles
	textarea textarea.Model
	visible  bool
}

// NewCommentForm creates a new inline comment form.
func NewCommentForm(s *styles.Styles) CommentForm {
	ta := textarea.New()
	ta.Placeholder = "Write a comment..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // no limit
	ta.SetHeight(commentFormHeight)

	return CommentForm{
		styles:   s,
		textarea: ta,
		visible:  false,
	}
}

// Show makes the form visible.
func (f *CommentForm) Show() {
	f.visible = true
}

// Hide makes the form invisible and blurs the textarea.
func (f *CommentForm) Hide() {
	f.visible = false
	f.textarea.Blur()
}

// IsVisible returns whether the form is visible.
func (f CommentForm) IsVisible() bool {
	return f.visible
}

// Focus focuses the textarea so it captures keystrokes.
func (f *CommentForm) Focus() tea.Cmd {
	return f.textarea.Focus()
}

// Reset clears the textarea content.
func (f *CommentForm) Reset() {
	f.textarea.Reset()
}

// SetValue replaces the textarea content (used to restore a draft after a failed send).
func (f *CommentForm) SetValue(s string) {
	f.textarea.SetValue(s)
}

// Value returns the current textarea content.
func (f CommentForm) Value() string {
	return f.textarea.Value()
}

// SetWidth sizes the textarea to the available width.
func (f *CommentForm) SetWidth(width int) {
	// Leave a little room for the border/padding.
	w := width - 4
	if w < 10 {
		w = 10
	}
	f.textarea.SetWidth(w)
}

// Height returns the number of terminal rows the form occupies when visible
// (textarea rows + border + help line).
func (f CommentForm) Height() int {
	// textarea height + top/bottom border (2) + help line (1)
	return commentFormHeight + 3
}

// Update handles messages for the form. Ctrl+S submits, Esc cancels, everything
// else (including Enter) is delegated to the textarea.
func (f CommentForm) Update(msg tea.Msg) (CommentForm, tea.Cmd) {
	if !f.visible {
		return f, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEsc:
			// Hide synchronously so the dispatched CommentFormCancelledMsg is
			// handled by the parent rather than re-captured by this form.
			f.visible = false
			f.textarea.Blur()
			return f, func() tea.Msg { return CommentFormCancelledMsg{} }
		case tea.KeyCtrlS:
			text := f.textarea.Value()
			if strings.TrimSpace(text) == "" {
				return f, nil
			}
			f.visible = false
			f.textarea.Blur()
			return f, func() tea.Msg { return CommentSubmittedMsg{Text: text} }
		}
	}

	var cmd tea.Cmd
	f.textarea, cmd = f.textarea.Update(msg)
	return f, cmd
}

// View renders the inline form.
func (f CommentForm) View() string {
	if !f.visible {
		return ""
	}

	helpText := lipgloss.NewStyle().
		Foreground(f.styles.Theme.GetForegroundMuted()).
		Render("Ctrl+S: send • Esc: cancel")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(f.styles.Theme.GetBorder()).
		Render(f.textarea.View())

	return lipgloss.JoinVertical(lipgloss.Left, box, helpText)
}
