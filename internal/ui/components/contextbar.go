package components

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

// ContextItem represents a keybinding or action in the context bar.
type ContextItem struct {
	Key         string
	Description string
}

// ContextBar is a reusable component that displays view-specific information
// such as keybindings, status messages, and scroll position. It appears above
// the main footer bar and can be customized per view.
type ContextBar struct {
	styles        *styles.Styles
	items         []ContextItem
	status        string
	scrollPercent float64
	showScroll    bool
	width         int
}

// NewContextBar creates a new ContextBar with default values.
func NewContextBar(s *styles.Styles) *ContextBar {
	return &ContextBar{
		styles:     s,
		items:      []ContextItem{},
		showScroll: false,
	}
}

// SetWidth sets the width of the context bar.
func (c *ContextBar) SetWidth(width int) {
	c.width = width
}

// SetItems sets all context items at once.
func (c *ContextBar) SetItems(items []ContextItem) {
	c.items = items
}

// AddItem adds a single context item.
func (c *ContextBar) AddItem(key, description string) {
	c.items = append(c.items, ContextItem{Key: key, Description: description})
}

// SetStatus sets a status message to display.
func (c *ContextBar) SetStatus(status string) {
	c.status = status
}

// SetScrollPercent sets the scroll percentage (0-100).
func (c *ContextBar) SetScrollPercent(percent float64) {
	c.scrollPercent = percent
}

// ShowScrollPercent enables or disables showing the scroll percentage.
func (c *ContextBar) ShowScrollPercent(show bool) {
	c.showScroll = show
}

// Clear resets all context bar content.
func (c *ContextBar) Clear() {
	c.items = []ContextItem{}
	c.status = ""
	c.scrollPercent = 0
	c.showScroll = false
}

// View renders the context bar.
func (c *ContextBar) View() string {
	width := c.width
	if width < 40 {
		width = 80
	}

	// Build styles from theme
	contextBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderBottom(false).
		BorderForeground(lipgloss.Color(c.styles.Theme.Border)).
		Foreground(lipgloss.Color(c.styles.Theme.Foreground))

	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(c.styles.Theme.Border))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(c.styles.Theme.ForegroundMuted)).
		Italic(true)

	var parts []string

	// Render keybinding items
	if len(c.items) > 0 {
		var itemStrings []string
		for _, item := range c.items {
			itemStr := c.styles.Key.Render(item.Key) + " " + c.styles.Description.Render(item.Description)
			itemStrings = append(itemStrings, itemStr)
		}
		sep := sepStyle.Render(" • ")
		parts = append(parts, strings.Join(itemStrings, sep))
	}

	// Add status message if present
	if c.status != "" {
		parts = append(parts, statusStyle.Render(c.status))
	}

	// Add scroll percentage if enabled
	if c.showScroll {
		scrollStr := c.styles.ScrollInfo.Render(fmt.Sprintf("%.0f%%", c.scrollPercent))
		parts = append(parts, scrollStr)
	}

	// Join all parts with separator
	content := strings.Join(parts, sepStyle.Render(" │ "))

	// Calculate box inner width
	boxInnerWidth := width - 2
	if boxInnerWidth < 20 {
		boxInnerWidth = 20
	}

	return contextBoxStyle.Width(boxInnerWidth).Render(content)
}
