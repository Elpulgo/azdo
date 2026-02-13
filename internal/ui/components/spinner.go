package components

import (
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LoadingIndicator is a component that displays a spinner with a message
// when visible. It wraps the bubbles spinner component.
type LoadingIndicator struct {
	styles  *styles.Styles
	spinner spinner.Model
	message string
	visible bool
}

// NewLoadingIndicator creates a new LoadingIndicator with default settings.
func NewLoadingIndicator(s *styles.Styles) *LoadingIndicator {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Theme.Spinner))

	return &LoadingIndicator{
		styles:  s,
		spinner: sp,
		message: "Loading...",
		visible: false,
	}
}

// SetMessage sets the loading message to display.
func (l *LoadingIndicator) SetMessage(msg string) {
	l.message = msg
}

// SetVisible sets whether the loading indicator is visible.
func (l *LoadingIndicator) SetVisible(visible bool) {
	l.visible = visible
}

// IsVisible returns whether the loading indicator is currently visible.
func (l *LoadingIndicator) IsVisible() bool {
	return l.visible
}

// Toggle toggles the visibility of the loading indicator.
func (l *LoadingIndicator) Toggle() {
	l.visible = !l.visible
}

// Init initializes the spinner and returns the tick command.
func (l *LoadingIndicator) Init() tea.Cmd {
	return l.spinner.Tick
}

// Update handles spinner tick messages.
func (l *LoadingIndicator) Update(msg tea.Msg) (*LoadingIndicator, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(msg)
		return l, cmd
	}
	return l, nil
}

// View renders the loading indicator.
// Returns an empty string if not visible.
func (l *LoadingIndicator) View() string {
	if !l.visible {
		return ""
	}
	return l.spinner.View() + " " + l.styles.Spinner.Render(l.message)
}

// Tick returns the spinner tick command.
// Use this to keep the spinner animating.
func (l *LoadingIndicator) Tick() tea.Cmd {
	return l.spinner.Tick
}
