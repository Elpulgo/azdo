// Package components provides reusable UI components for the TUI.
package components

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar is a component that displays keybindings, org/project info,
// and connection state at the bottom of the screen like lazygit.
type StatusBar struct {
	styles         *styles.Styles
	organization   string
	project        string
	state          polling.ConnectionState
	keybindings    string
	scrollPercent  float64
	showScroll     bool
	width          int
	errorMessage   string
	filterLabel    string
	updateMessage  string
	warningMessage string
	contextItems   []ContextItem
	contextStatus  string
}

// NewStatusBar creates a new StatusBar with default values.
func NewStatusBar(s *styles.Styles) *StatusBar {
	return &StatusBar{
		styles:      s,
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

// GetState returns the current connection state.
func (s *StatusBar) GetState() polling.ConnectionState {
	return s.state
}

// SetState sets the connection state.
func (s *StatusBar) SetState(state polling.ConnectionState) {
	s.state = state
}

// GetWarningMessage returns the current warning message.
func (s *StatusBar) GetWarningMessage() string {
	return s.warningMessage
}

// SetKeybindings sets the keybindings to display.
func (s *StatusBar) SetKeybindings(bindings string) {
	s.keybindings = bindings
}

// SetContextItems sets context-specific keybindings that replace the default
// keybindings in the footer. Base shortcuts (esc back, ? help, q quit) are
// automatically appended with deduplication.
func (s *StatusBar) SetContextItems(items []ContextItem) {
	s.contextItems = items
}

// ClearContextItems removes context-specific keybindings, restoring defaults.
func (s *StatusBar) ClearContextItems() {
	s.contextItems = nil
	s.contextStatus = ""
}

// SetContextStatus sets an italic status message displayed alongside context items.
func (s *StatusBar) SetContextStatus(status string) {
	s.contextStatus = status
}

// SetWidth sets the width of the status bar.
func (s *StatusBar) SetWidth(width int) {
	s.width = width
}

// SetScrollPercent sets the scroll percentage (0-100).
func (s *StatusBar) SetScrollPercent(percent float64) {
	s.scrollPercent = percent
}

// ShowScrollPercent enables or disables showing the scroll percentage.
func (s *StatusBar) ShowScrollPercent(show bool) {
	s.showScroll = show
}

// SetErrorMessage sets the error message to display.
func (s *StatusBar) SetErrorMessage(message string) {
	s.errorMessage = message
}

// ClearErrorMessage clears the error message.
func (s *StatusBar) ClearErrorMessage() {
	s.errorMessage = ""
}

// SetFilterLabel sets a filter indicator label to display in the status bar.
func (s *StatusBar) SetFilterLabel(label string) {
	s.filterLabel = label
}

// ClearFilterLabel removes the filter indicator label.
func (s *StatusBar) ClearFilterLabel() {
	s.filterLabel = ""
}

// SetUpdateMessage sets the update notification message.
func (s *StatusBar) SetUpdateMessage(message string) {
	s.updateMessage = message
}

// SetWarningMessage sets a persistent warning message that displays regardless of connection state.
func (s *StatusBar) SetWarningMessage(message string) {
	s.warningMessage = message
}

// ClearWarningMessage clears the persistent warning message.
func (s *StatusBar) ClearWarningMessage() {
	s.warningMessage = ""
}

// Init implements tea.Model (no initialization needed).
func (s *StatusBar) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model (status bar doesn't handle messages).
func (s *StatusBar) Update(msg tea.Msg) (*StatusBar, tea.Cmd) {
	return s, nil
}

// View renders the status bar as a full-width footer with box border.
func (s *StatusBar) View() string {
	// Use terminal width or default
	width := s.width
	if width < 40 {
		width = 80
	}

	// Build separator style
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.styles.Theme.Border)).
		Background(lipgloss.Color(s.styles.Theme.Background))

	sep := sepStyle.Render(" │ ")

	parts := []string{}

	// If there's an error message and state is error, show it prominently
	if s.errorMessage != "" && s.state == polling.StateError {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(s.styles.Theme.Error)).
			Background(lipgloss.Color(s.styles.Theme.Background)).
			Bold(true)
		parts = append(parts, errorStyle.Render(s.errorMessage))
	} else {
		parts = append(parts, s.renderKeybindings())
	}

	if s.contextStatus != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(s.styles.Theme.ForegroundMuted)).
			Background(lipgloss.Color(s.styles.Theme.Background)).
			Italic(true)
		parts = append(parts, statusStyle.Render(s.contextStatus))
	}

	if s.warningMessage != "" {
		warningStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(s.styles.Theme.Warning)).
			Background(lipgloss.Color(s.styles.Theme.Background)).
			Bold(true)
		parts = append(parts, warningStyle.Render("⚠ "+s.warningMessage))
	}

	if s.filterLabel != "" {
		filterStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(s.styles.Theme.Background)).
			Background(lipgloss.Color(s.styles.Theme.Accent)).
			Bold(true).
			Padding(0, 1)
		parts = append(parts, filterStyle.Render(s.filterLabel))
	}

	if s.updateMessage != "" {
		updateStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(s.styles.Theme.Warning)).
			Background(lipgloss.Color(s.styles.Theme.Background)).
			Bold(true)
		parts = append(parts, updateStyle.Render(s.updateMessage))
	}

	if orgProj := s.renderOrgProject(); orgProj != "" {
		parts = append(parts, orgProj)
	}

	if scrollPercent := s.renderScrollPercent(); scrollPercent != "" {
		parts = append(parts, scrollPercent)
	}

	parts = append(parts, s.renderConnectionState())

	// Join with separators, left-aligned
	content := strings.Join(parts, sep)

	// Calculate box inner width (subtract 2 for border sides)
	boxInnerWidth := width - 2
	if boxInnerWidth < 20 {
		boxInnerWidth = 20
	}

	return s.styles.BoxRounded.Width(boxInnerWidth).Render(content)
}

// renderKeybindings renders the keybindings section.
func (s *StatusBar) renderKeybindings() string {
	if len(s.contextItems) > 0 {
		return s.renderContextKeybindings()
	}

	if s.keybindings != "" {
		return s.keybindings
	}

	// Build styles from theme
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.styles.Theme.Border)).
		Background(lipgloss.Color(s.styles.Theme.Background))

	// Default keybindings with styled keys
	sep := sepStyle.Render(" • ")
	return s.styles.Key.Render("r") + s.styles.Description.Render(" refresh") + sep +
		s.styles.Key.Render("↑↓") + s.styles.Description.Render(" navigate") + sep +
		s.styles.Key.Render("enter") + s.styles.Description.Render(" details") + sep +
		s.styles.Key.Render("esc") + s.styles.Description.Render(" back") + sep +
		s.styles.Key.Render("?") + s.styles.Description.Render(" help") + sep +
		s.styles.Key.Render("q") + s.styles.Description.Render(" quit")
}

// renderContextKeybindings renders context-specific keybindings with base
// shortcuts (esc back, ? help, q quit) appended, deduplicating any that
// already exist in the context items.
func (s *StatusBar) renderContextKeybindings() string {
	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.styles.Theme.Border)).
		Background(lipgloss.Color(s.styles.Theme.Background))
	sep := sepStyle.Render(" • ")

	// Track which base shortcut keys are already in context items
	hasKey := make(map[string]bool)
	for _, item := range s.contextItems {
		hasKey[item.Key] = true
	}

	// Render context items
	var parts []string
	for _, item := range s.contextItems {
		parts = append(parts, s.styles.Key.Render(item.Key)+" "+s.styles.Description.Render(item.Description))
	}

	// Append base shortcuts with deduplication
	baseShortcuts := []ContextItem{
		{Key: "esc", Description: "back"},
		{Key: "?", Description: "help"},
		{Key: "q", Description: "quit"},
	}
	for _, base := range baseShortcuts {
		if !hasKey[base.Key] {
			parts = append(parts, s.styles.Key.Render(base.Key)+" "+s.styles.Description.Render(base.Description))
		}
	}

	return strings.Join(parts, sep)
}

// renderOrgProject renders the organization and project section.
func (s *StatusBar) renderOrgProject() string {
	if s.organization == "" && s.project == "" {
		return ""
	}

	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.styles.Theme.Border)).
		Background(lipgloss.Color(s.styles.Theme.Background))

	orgProjectStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(s.styles.Theme.Secondary)).
		Background(lipgloss.Color(s.styles.Theme.Background)).
		Bold(true)

	sep := sepStyle.Render("/")

	if s.organization != "" && s.project != "" {
		return orgProjectStyle.Render(s.organization) + sep + orgProjectStyle.Render(s.project)
	}

	if s.organization != "" {
		return orgProjectStyle.Render(s.organization)
	}

	return orgProjectStyle.Render(s.project)
}

// renderScrollPercent renders the scroll percentage indicator.
func (s *StatusBar) renderScrollPercent() string {
	if !s.showScroll {
		return ""
	}
	return s.styles.ScrollInfo.Render(fmt.Sprintf("%.0f%%", s.scrollPercent))
}

// renderConnectionState renders the connection state indicator.
func (s *StatusBar) renderConnectionState() string {
	switch s.state {
	case polling.StateConnected:
		return s.styles.Connected.Render("●")
	case polling.StateConnecting:
		return s.styles.Connecting.Render("◐ connecting")
	case polling.StateDisconnected:
		return s.styles.Disconnected.Render("○ disconnected")
	case polling.StateError:
		return s.styles.ConnError.Render("✗ error")
	default:
		return s.styles.Disconnected.Render(fmt.Sprintf("? %s", s.state))
	}
}
