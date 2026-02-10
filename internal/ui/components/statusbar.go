// Package components provides reusable UI components for the TUI.
package components

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar is a component that displays connection state, org/project info,
// and help hints at the bottom of the screen.
type StatusBar struct {
	organization string
	project      string
	state        polling.ConnectionState
	helpText     string
	width        int
}

// Styles for the status bar
var (
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	orgProjectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true)

	connectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	connectingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("226"))

	disconnectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// NewStatusBar creates a new StatusBar with default values.
func NewStatusBar() *StatusBar {
	return &StatusBar{
		state:    polling.StateConnecting,
		helpText: "? help",
	}
}

// SetOrganization sets the organization name to display.
func (s *StatusBar) SetOrganization(org string) {
	s.organization = org
}

// SetProject sets the project name to display.
func (s *StatusBar) SetProject(project string) {
	s.project = project
}

// SetState sets the connection state.
func (s *StatusBar) SetState(state polling.ConnectionState) {
	s.state = state
}

// SetHelpText sets custom help text to display.
func (s *StatusBar) SetHelpText(text string) {
	s.helpText = text
}

// SetWidth sets the width of the status bar.
func (s *StatusBar) SetWidth(width int) {
	s.width = width
}

// Init implements tea.Model (no initialization needed).
func (s *StatusBar) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model (status bar doesn't handle messages).
func (s *StatusBar) Update(msg tea.Msg) (*StatusBar, tea.Cmd) {
	return s, nil
}

// View renders the status bar.
func (s *StatusBar) View() string {
	// Build the left section: org/project
	left := s.renderOrgProject()

	// Build the center section: connection state
	center := s.renderConnectionState()

	// Build the right section: help text
	right := s.renderHelp()

	// Calculate available space
	leftLen := lipgloss.Width(left)
	centerLen := lipgloss.Width(center)
	rightLen := lipgloss.Width(right)

	// Minimum viable width
	minWidth := leftLen + centerLen + rightLen + 4
	width := s.width
	if width < minWidth {
		width = minWidth
	}

	// Create spacing between sections
	totalContentLen := leftLen + centerLen + rightLen
	remainingSpace := width - totalContentLen
	if remainingSpace < 2 {
		remainingSpace = 2
	}

	leftPadding := remainingSpace / 2
	rightPadding := remainingSpace - leftPadding

	// Build the full bar without line wrapping
	// Use inline style to avoid Width causing wrapping
	bar := left + strings.Repeat(" ", leftPadding) + center + strings.Repeat(" ", rightPadding) + right

	return statusBarStyle.Inline(true).Render(bar)
}

// renderOrgProject renders the organization and project section.
func (s *StatusBar) renderOrgProject() string {
	if s.organization == "" && s.project == "" {
		return ""
	}

	sep := separatorStyle.Render("/")

	if s.organization != "" && s.project != "" {
		return orgProjectStyle.Render(s.organization) + sep + orgProjectStyle.Render(s.project)
	}

	if s.organization != "" {
		return orgProjectStyle.Render(s.organization)
	}

	return orgProjectStyle.Render(s.project)
}

// renderConnectionState renders the connection state indicator.
func (s *StatusBar) renderConnectionState() string {
	switch s.state {
	case polling.StateConnected:
		return connectedStyle.Render("● connected")
	case polling.StateConnecting:
		return connectingStyle.Render("○ connecting")
	case polling.StateDisconnected:
		return disconnectedStyle.Render("○ disconnected")
	case polling.StateError:
		return errorStyle.Render("✗ error")
	default:
		return disconnectedStyle.Render(fmt.Sprintf("? %s", s.state))
	}
}

// renderHelp renders the help hint section.
func (s *StatusBar) renderHelp() string {
	if s.helpText == "" {
		return helpStyle.Render("? help")
	}
	return helpStyle.Render(s.helpText)
}
