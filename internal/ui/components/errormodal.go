package components

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// minErrorModalWidth is the minimum width for the error modal content.
const minErrorModalWidth = 50

// ErrorInfo holds classified error information for display in the error modal.
type ErrorInfo struct {
	Title   string
	Message string
	Hint    string
}

// ErrorModal is an overlay that displays critical error messages.
type ErrorModal struct {
	styles  *styles.Styles
	visible bool
	width   int
	height  int
	title   string
	message string
	hint    string
}

// NewErrorModal creates a new ErrorModal.
func NewErrorModal(s *styles.Styles) *ErrorModal {
	return &ErrorModal{
		styles:  s,
		visible: false,
	}
}

// Show makes the error modal visible with the given content.
func (m *ErrorModal) Show(title, message, hint string) {
	m.title = title
	m.message = message
	m.hint = hint
	m.visible = true
}

// Hide hides the error modal.
func (m *ErrorModal) Hide() {
	m.visible = false
}

// IsVisible returns true if the modal is visible.
func (m *ErrorModal) IsVisible() bool {
	return m.visible
}

// SetSize sets the available size for the modal.
func (m *ErrorModal) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles key events for the error modal.
func (m *ErrorModal) Update(msg tea.Msg) (*ErrorModal, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.Hide()
			return m, nil
		}
	}

	return m, nil
}

// View renders the error modal overlay.
func (m *ErrorModal) View() string {
	if !m.visible {
		return ""
	}

	contentWidth := minErrorModalWidth
	if len(m.title) > contentWidth {
		contentWidth = len(m.title)
	}
	if len(m.message) > contentWidth {
		contentWidth = len(m.message)
	}

	// Build styles from theme using error color for border
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(string(m.styles.Theme.Error))).
		Padding(1, 2).
		Background(lipgloss.Color(string(m.styles.Theme.BackgroundAlt)))

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(string(m.styles.Theme.Error))).
		Bold(true).
		MarginBottom(1).
		Width(contentWidth).
		Background(lipgloss.Color(string(m.styles.Theme.BackgroundAlt)))

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(string(m.styles.Theme.Foreground))).
		Width(contentWidth).
		Background(lipgloss.Color(string(m.styles.Theme.BackgroundAlt)))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(string(m.styles.Theme.Accent))).
		Bold(true).
		MarginTop(1).
		Width(contentWidth).
		Background(lipgloss.Color(string(m.styles.Theme.BackgroundAlt)))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(string(m.styles.Theme.ForegroundMuted))).
		Width(contentWidth).
		Background(lipgloss.Color(string(m.styles.Theme.BackgroundAlt)))

	var content strings.Builder

	content.WriteString(titleStyle.Render(m.title))
	content.WriteString("\n")
	content.WriteString(messageStyle.Render(m.message))
	content.WriteString("\n")

	if m.hint != "" {
		content.WriteString(hintStyle.Render(m.hint))
		content.WriteString("\n")
	}

	content.WriteString("\n")
	content.WriteString(footerStyle.Render("Press esc to dismiss"))

	modal := modalStyle.Render(content.String())

	// Center the modal on screen
	if m.width > 0 && m.height > 0 {
		modalWidth := lipgloss.Width(modal)
		modalHeight := lipgloss.Height(modal)

		leftPad := (m.width - modalWidth) / 2
		topPad := (m.height - modalHeight) / 2

		if leftPad < 0 {
			leftPad = 0
		}
		if topPad < 0 {
			topPad = 0
		}

		var centered strings.Builder
		for i := 0; i < topPad; i++ {
			centered.WriteString("\n")
		}

		lines := strings.Split(modal, "\n")
		for _, line := range lines {
			centered.WriteString(strings.Repeat(" ", leftPad))
			centered.WriteString(line)
			centered.WriteString("\n")
		}

		return centered.String()
	}

	return modal
}

// CriticalErrorMsg is a tea.Msg that any view can return as a command
// to signal a critical API error to the root app model, which will show
// the error modal overlay.
type CriticalErrorMsg struct {
	Title   string
	Message string
	Hint    string
}

// NewCriticalErrorCmd checks if err is a critical error and, if so, returns
// a tea.Cmd that emits a CriticalErrorMsg. Returns nil if the error is not critical.
func NewCriticalErrorCmd(err error) tea.Cmd {
	info := ClassifyError(err)
	if info == nil {
		return nil
	}
	return func() tea.Msg {
		return CriticalErrorMsg{
			Title:   info.Title,
			Message: info.Message,
			Hint:    info.Hint,
		}
	}
}

// ClassifyError examines an error and returns an ErrorInfo if it represents
// a critical/known error that should be shown in the error modal.
// Returns nil for transient or unknown errors.
func ClassifyError(err error) *ErrorInfo {
	if err == nil {
		return nil
	}

	msg := err.Error()

	if strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "resource not found") {
		return &ErrorInfo{
			Title:   "Configuration Error",
			Message: "The API returned 'not found'. Your organization or project name in the configuration may be incorrect.",
			Hint:    "Check your config file and verify the organization and project names match your Azure DevOps setup.",
		}
	}

	if strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "authentication failed") {
		return &ErrorInfo{
			Title:   "Authentication Error",
			Message: "Your Personal Access Token (PAT) may be expired or invalid.",
			Hint:    "Run 'azdo auth' to update your PAT.",
		}
	}

	if strings.Contains(msg, "HTTP 403") || strings.Contains(msg, "access denied") {
		return &ErrorInfo{
			Title:   "Authentication Error",
			Message: "Your PAT does not have sufficient permissions for this operation.",
			Hint:    "Run 'azdo auth' to update your PAT with the required scopes.",
		}
	}

	return nil
}
