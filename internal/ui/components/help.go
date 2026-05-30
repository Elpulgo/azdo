package components

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// minModalWidth is defined in listpicker.go and shared across components

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
	styles       *styles.Styles
	visible      bool
	width        int
	height       int
	sections     []HelpSection
	configPath   string
	versionInfo  string
	scrollOffset int
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
				Title: "Tabs",
				Bindings: []HelpBinding{
					{Key: "1/2/3", Description: "PR / Work Items / Pipelines"},
					{Key: "←/→", Description: "Previous / next tab"},
				},
			},
			{
				Title: "Actions",
				Bindings: []HelpBinding{
					{Key: "f", Description: "Search / filter"},
					{Key: "m", Description: "Toggle my items (PRs / work items)"},
					{Key: "A", Description: "Toggle as reviewer (PRs)"},
					{Key: "T", Description: "Filter by tag (work items)"},
					{Key: "s", Description: "Filter by state (work items)"},
					{Key: "S", Description: "Filter by status (pipelines)"},
					{Key: "r", Description: "Refresh data"},
					{Key: "v", Description: "Vote on PR (detail view)"},
					{Key: "w", Description: "Change work item state (detail view)"},
					{Key: "o", Description: "Open in browser (PR / work item detail)"},
					{Key: "t", Description: "Select theme"},
					{Key: "?", Description: "Toggle help"},
					{Key: "q", Description: "Quit application"},
				},
			},
			{
				Title: "Code Review (PR diff)",
				Bindings: []HelpBinding{
					{Key: "c", Description: "Create new comment"},
					{Key: "p", Description: "Reply to nearest thread"},
					{Key: "x", Description: "Resolve nearest thread"},
					{Key: "n", Description: "Jump to next comment"},
					{Key: "N", Description: "Jump to previous comment"},
				},
			},
			{
				Title: "Log Viewer (pipelines)",
				Bindings: []HelpBinding{
					{Key: "g", Description: "Go to top"},
					{Key: "G", Description: "Go to bottom"},
				},
			},
		},
	}
}

// Show makes the help modal visible.
func (h *HelpModal) Show() {
	h.visible = true
	h.scrollOffset = 0
}

// Hide hides the help modal.
func (h *HelpModal) Hide() {
	h.visible = false
	h.scrollOffset = 0
}

// Toggle toggles the help modal visibility.
func (h *HelpModal) Toggle() {
	h.visible = !h.visible
	if !h.visible {
		h.scrollOffset = 0
	}
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

// SetConfigPath sets the config file path to display in the help modal.
func (h *HelpModal) SetConfigPath(path string) {
	h.configPath = path
}

// SetVersionInfo sets the version info string to display in the help modal.
func (h *HelpModal) SetVersionInfo(info string) {
	h.versionInfo = info
}

// AddSection adds a custom section to the help modal.
func (h *HelpModal) AddSection(title string, bindings []HelpBinding) {
	h.sections = append(h.sections, HelpSection{
		Title:    title,
		Bindings: bindings,
	})
}

// RemoveSection removes a section by title.
func (h *HelpModal) RemoveSection(title string) {
	filtered := h.sections[:0]
	for _, s := range h.sections {
		if s.Title != title {
			filtered = append(filtered, s)
		}
	}
	h.sections = filtered
}

// RemoveBindingsByDescription removes specific bindings from the Actions section
// whose description contains the given substring.
func (h *HelpModal) RemoveBindingsByDescription(substr string) {
	for i, section := range h.sections {
		if section.Title == "Actions" {
			filtered := section.Bindings[:0]
			for _, b := range section.Bindings {
				if !containsSubstring(b.Description, substr) {
					filtered = append(filtered, b)
				}
			}
			h.sections[i].Bindings = filtered
		}
	}
}

func containsSubstring(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// UpdateTabsBinding replaces the tab keys and descriptions in the Tabs section.
func (h *HelpModal) UpdateTabsBinding(keys, description string) {
	for i, section := range h.sections {
		if section.Title == "Tabs" {
			for j, b := range section.Bindings {
				if strings.Contains(b.Key, "/") && strings.Contains(b.Description, "/") {
					h.sections[i].Bindings[j] = HelpBinding{Key: keys, Description: description}
					return
				}
			}
		}
	}
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
		case "down", "j":
			h.scrollBy(1)
			return h, nil
		case "up", "k":
			h.scrollBy(-1)
			return h, nil
		case "pgdown":
			h.scrollBy(h.pageStep())
			return h, nil
		case "pgup":
			h.scrollBy(-h.pageStep())
			return h, nil
		}
	}

	return h, nil
}

// scrollBy adjusts the scroll offset and clamps it to the valid range
// for the current content.
func (h *HelpModal) scrollBy(delta int) {
	h.scrollOffset += delta
	if max := h.maxScrollOffset(); h.scrollOffset > max {
		h.scrollOffset = max
	}
	if h.scrollOffset < 0 {
		h.scrollOffset = 0
	}
}

// pageStep returns the number of lines a page-up/page-down should move,
// based on the current body viewport height. Falls back to a sensible
// minimum so paging always makes progress.
func (h *HelpModal) pageStep() int {
	step := h.bodyHeight() - 1
	if step < 1 {
		step = 1
	}
	return step
}

// maxScrollOffset is the highest scroll offset that keeps the last line
// of the body visible.
func (h *HelpModal) maxScrollOffset() int {
	lines := h.bodyLines(h.contentWidth())
	max := len(lines) - h.bodyHeight()
	if max < 0 {
		return 0
	}
	return max
}

// modalChromeRows counts the non-body lines the rendered modal always
// consumes: top + bottom border (2), top + bottom padding (2), the title
// row plus its bottom margin (2), the blank spacer above the footer (1),
// and the footer line itself (1).
const modalChromeRows = 8

// contentWidth returns the rendered modal's inner content width. It is
// computed against the longest possible footer (with scroll hint) so the
// width stays stable regardless of whether the modal is currently
// scrollable.
func (h *HelpModal) contentWidth() int {
	titleText := "⌨ Keyboard Shortcuts"
	// Width must not depend on isScrollable() — otherwise contentWidth ↔
	// isScrollable ↔ footerText form a cycle.
	footerText := footerHintBase + footerHintScrollSuffix

	contentWidth := minModalWidth
	if len(titleText) > contentWidth {
		contentWidth = len(titleText)
	}
	if len(footerText) > contentWidth {
		contentWidth = len(footerText)
	}

	for _, section := range h.sections {
		if len(section.Title) > contentWidth {
			contentWidth = len(section.Title)
		}
		for _, binding := range section.Bindings {
			lineLen := 12 + len(binding.Description)
			if lineLen > contentWidth {
				contentWidth = lineLen
			}
		}
	}
	return contentWidth
}

const (
	footerHintBase         = "Press esc, q, or ? to close"
	footerHintScrollSuffix = " • ↑↓ scroll"
)

// bodyHeight returns the number of rows available for the scrollable
// body inside the modal. When the terminal height is unknown (h.height
// == 0), the full body is shown.
func (h *HelpModal) bodyHeight() int {
	if h.height <= 0 {
		return len(h.bodyLines(h.contentWidth()))
	}
	avail := h.height - modalChromeRows
	if avail < 1 {
		avail = 1
	}
	full := len(h.bodyLines(h.contentWidth()))
	if avail > full {
		return full
	}
	return avail
}

// bodyLines builds the styled, line-by-line representation of the
// section list and Info block. The returned slice is the unit the
// scroll offset indexes into.
func (h *HelpModal) bodyLines(contentWidth int) []string {
	helpSectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Secondary)).
		Bold(true).
		Width(contentWidth).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt))

	helpKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Accent)).
		Bold(true).
		Width(12).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt))

	helpDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Foreground)).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt)).
		Width(contentWidth - 12)

	blankLine := lipgloss.NewStyle().
		Width(contentWidth).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt)).
		Render("")

	var lines []string
	for i, section := range h.sections {
		if i > 0 {
			lines = append(lines, blankLine)
		}
		lines = append(lines, helpSectionStyle.Render(section.Title))
		for _, binding := range section.Bindings {
			lines = append(lines, helpKeyStyle.Render(binding.Key)+helpDescStyle.Render(binding.Description))
		}
	}

	if h.versionInfo != "" || h.configPath != "" {
		infoValueStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(h.styles.Theme.ForegroundMuted)).
			Background(lipgloss.Color(h.styles.Theme.BackgroundAlt)).
			Width(contentWidth)

		lines = append(lines, blankLine)
		lines = append(lines, helpSectionStyle.Render("Info"))
		if h.versionInfo != "" {
			lines = append(lines, infoValueStyle.Render("Version: "+h.versionInfo))
		}
		if h.configPath != "" {
			lines = append(lines, infoValueStyle.Render("Config: "+h.configPath))
		}
	}
	return lines
}

// footerText returns the footer hint, with scroll keys appended when
// the body is scrollable.
func (h *HelpModal) footerText() string {
	if h.isScrollable() {
		return footerHintBase + footerHintScrollSuffix
	}
	return footerHintBase
}

// isScrollable returns true when the body has more lines than the
// available body height.
func (h *HelpModal) isScrollable() bool {
	if h.height <= 0 {
		return false
	}
	avail := h.height - modalChromeRows
	return avail >= 1 && len(h.bodyLines(h.contentWidth())) > avail
}

// View renders the help modal overlay.
func (h *HelpModal) View() string {
	if !h.visible {
		return ""
	}

	contentWidth := h.contentWidth()

	helpModalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(h.styles.Theme.Accent)).
		Padding(1, 2).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt))

	helpTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.Accent)).
		Bold(true).
		MarginBottom(1).
		Width(contentWidth).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(h.styles.Theme.ForegroundMuted)).
		Width(contentWidth).
		Background(lipgloss.Color(h.styles.Theme.BackgroundAlt))

	bodyAll := h.bodyLines(contentWidth)
	bodyH := h.bodyHeight()
	if h.scrollOffset > len(bodyAll)-bodyH {
		h.scrollOffset = len(bodyAll) - bodyH
	}
	if h.scrollOffset < 0 {
		h.scrollOffset = 0
	}
	end := h.scrollOffset + bodyH
	if end > len(bodyAll) {
		end = len(bodyAll)
	}
	bodyVisible := bodyAll[h.scrollOffset:end]

	var content strings.Builder
	content.WriteString(helpTitleStyle.Render("⌨ Keyboard Shortcuts"))
	content.WriteString("\n")
	content.WriteString(strings.Join(bodyVisible, "\n"))
	content.WriteString("\n\n")
	content.WriteString(footerStyle.Render(h.footerText()))

	modal := helpModalStyle.Render(content.String())

	// Center the modal on screen.
	if h.width > 0 && h.height > 0 {
		modalWidth := lipgloss.Width(modal)
		modalHeight := lipgloss.Height(modal)

		leftPad := (h.width - modalWidth) / 2
		topPad := (h.height - modalHeight) / 2

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
		for i, line := range lines {
			centered.WriteString(strings.Repeat(" ", leftPad))
			centered.WriteString(line)
			if i < len(lines)-1 {
				centered.WriteString("\n")
			}
		}

		return centered.String()
	}

	return modal
}
