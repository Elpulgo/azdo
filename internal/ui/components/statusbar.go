// Package components provides reusable UI components for the TUI.
package components

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/polling"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar is a component that displays keybindings, org/project info,
// and connection state at the bottom of the screen like lazygit.
type StatusBar struct {
	organization string
	project      string
	state        polling.ConnectionState
	keybindings  string
	width        int
}

// Styles for the status bar
var (
	// Main bar style - full width background
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))

	// Keybinding styles
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color("236")).
			Bold(true)

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236"))

	sepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Background(lipgloss.Color("236"))

	// Org/project style
	orgProjectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("236")).
			Bold(true)

	// Connection state styles
	connectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Background(lipgloss.Color("236"))

	connectingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Background(lipgloss.Color("236"))

	disconnectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Background(lipgloss.Color("236"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Background(lipgloss.Color("236"))
)

// NewStatusBar creates a new StatusBar with default values.
func NewStatusBar() *StatusBar {
	return &StatusBar{
		state:       polling.StateConnecting,
		keybindings: "",
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

// SetKeybindings sets the keybindings to display.
func (s *StatusBar) SetKeybindings(bindings string) {
	s.keybindings = bindings
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

// View renders the status bar as a full-width footer.
func (s *StatusBar) View() string {
	// Build the left section: keybindings
	left := s.renderKeybindings()

	// Build the center section: org/project
	center := s.renderOrgProject()

	// Build the right section: connection state
	right := s.renderConnectionState()

	// Calculate widths
	leftLen := lipgloss.Width(left)
	centerLen := lipgloss.Width(center)
	rightLen := lipgloss.Width(right)

	// Use terminal width or default
	width := s.width
	if width < 40 {
		width = 80
	}

	// Calculate spacing to distribute content
	totalContent := leftLen + centerLen + rightLen
	remainingSpace := width - totalContent - 2 // -2 for edge padding

	if remainingSpace < 2 {
		remainingSpace = 2
	}

	// Distribute space: more on right side to push center toward middle
	leftSpace := remainingSpace / 3
	rightSpace := remainingSpace - leftSpace

	// Build the bar content
	content := " " + left +
		strings.Repeat(" ", leftSpace) +
		center +
		strings.Repeat(" ", rightSpace) +
		right + " "

	// Pad to full width
	contentLen := lipgloss.Width(content)
	if contentLen < width {
		content = content + strings.Repeat(" ", width-contentLen)
	}

	return statusBarStyle.Render(content)
}

// renderKeybindings renders the keybindings section.
func (s *StatusBar) renderKeybindings() string {
	if s.keybindings != "" {
		return s.keybindings
	}

	// Default keybindings with styled keys
	sep := sepStyle.Render(" • ")
	return keyStyle.Render("r") + descStyle.Render(" refresh") + sep +
		keyStyle.Render("↑↓") + descStyle.Render(" navigate") + sep +
		keyStyle.Render("enter") + descStyle.Render(" details") + sep +
		keyStyle.Render("?") + descStyle.Render(" help") + sep +
		keyStyle.Render("q") + descStyle.Render(" quit")
}

// renderOrgProject renders the organization and project section.
func (s *StatusBar) renderOrgProject() string {
	if s.organization == "" && s.project == "" {
		return ""
	}

	sep := sepStyle.Render("/")

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
		return connectingStyle.Render("◐ connecting")
	case polling.StateDisconnected:
		return disconnectedStyle.Render("○ disconnected")
	case polling.StateError:
		return errorStyle.Render("✗ error")
	default:
		return disconnectedStyle.Render(fmt.Sprintf("? %s", s.state))
	}
}
