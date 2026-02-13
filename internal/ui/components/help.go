package components

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HelpBinding represents a single keybinding entry.
type HelpBinding struct {
	Key         string
	Description string
}

// HelpSection represents a group of related keybindings.
type HelpSection struct {
	Title    string
	Bindings []HelpBinding
}

// HelpModal is an overlay that displays available keybindings.
type HelpModal struct {
	styles   *styles.Styles
	visible  bool
	width    int
	height   int
	sections []HelpSection
}

// NewHelpModal creates a new HelpModal with default keybindings.
func NewHelpModal(s *styles.Styles) *HelpModal {
	return &HelpModal{
		styles:  s,
		visible: false,
		sections: []HelpSection{
			{
				Title: "Navigation",
				Bindings: []HelpBinding{
					{Key: "↑/k", Description: "Move up"},
					{Key: "↓/j", Description: "Move down"},
					{Key: "pgup/pgdn", Description: "Page up / down"},
					{Key: "enter", Description: "View details / expand"},
					{Key: "esc", Description: "Go back"},
				},
			},
			{
				Title: "Actions",
				Bindings: []HelpBinding{
					{Key: "r", Description: "Refresh data"},
					{Key: "t", Description: "Select theme"},
					{Key: "?", Description: "Toggle help"},
					{Key: "q", Description: "Quit application"},
				},
			},
		},
	}
}

// Show makes the help modal visible.
func (h *HelpModal) Show() {
	h.visible = true
}

// Hide hides the help modal.
func (h *HelpModal) Hide() {
	h.visible = false
}

// Toggle toggles the help modal visibility.
func (h *HelpModal) Toggle() {
	h.visible = !h.visible
}

// IsVisible returns true if the modal is visible.
func (h *HelpModal) IsVisible() bool {
	return h.visible
}

// SetSize sets the available size for the modal.
func (h *HelpModal) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// AddSection adds a custom section to the help modal.
func (h *HelpModal) AddSection(title string, bindings []HelpBinding) {
	h.sections = append(h.sections, HelpSection{
		Title:    title,
		Bindings: bindings,
	})
}

// Update handles key events for the help modal.
func (h *HelpModal) Update(msg tea.Msg) (*HelpModal, tea.Cmd) {
	if !h.visible {
		return h, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "?":
			h.Hide()
			return h, nil
		}
	}

	return h, nil
}

// View renders the help modal overlay.
func (h *HelpModal) View() string {
	if !h.visible {
		return ""
	}

	// Build styles from theme
	helpModalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(h.styles.Theme.Accent)).
		Padding(1, 2).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt))

	helpTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Accent)).
		Bold(true).
		MarginBottom(1)

	helpSectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Secondary)).
		Bold(true).
		MarginTop(1)

	helpKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Accent)).
		Bold(true).
		Width(12)

	helpDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Foreground))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.ForegroundMuted))

	var content strings.Builder

	// Title
	content.WriteString(helpTitleStyle.Render("⌨ Keyboard Shortcuts"))
	content.WriteString("\n")

	// Sections
	for _, section := range h.sections {
		content.WriteString(helpSectionStyle.Render(section.Title))
		content.WriteString("\n")

		for _, binding := range section.Bindings {
			line := helpKeyStyle.Render(binding.Key) + helpDescStyle.Render(binding.Description)
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	// Footer hint
	content.WriteString("\n")
	content.WriteString(footerStyle.Render("Press esc, q, or ? to close"))

	// Render the modal box
	modal := helpModalStyle.Render(content.String())

	// Center the modal on screen
	if h.width > 0 && h.height > 0 {
		modalWidth := lipgloss.Width(modal)
		modalHeight := lipgloss.Height(modal)

		// Calculate padding to center
		leftPad := (h.width - modalWidth) / 2
		topPad := (h.height - modalHeight) / 2

		if leftPad < 0 {
			leftPad = 0
		}
		if topPad < 0 {
			topPad = 0
		}

		// Build centered output
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
