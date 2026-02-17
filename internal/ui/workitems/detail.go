package workitems

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DetailModel represents the work item detail view
type DetailModel struct {
	client   *azdevops.Client
	workItem azdevops.WorkItem
	width    int
	height   int
	viewport viewport.Model
	ready    bool
	styles   *styles.Styles
}

// NewDetailModel creates a new work item detail model with default styles
func NewDetailModel(client *azdevops.Client, wi azdevops.WorkItem) *DetailModel {
	return NewDetailModelWithStyles(client, wi, styles.DefaultStyles())
}

// NewDetailModelWithStyles creates a new work item detail model with custom styles
func NewDetailModelWithStyles(client *azdevops.Client, wi azdevops.WorkItem, s *styles.Styles) *DetailModel {
	return &DetailModel{
		client:   client,
		workItem: wi,
		styles:   s,
	}
}

// Init initializes the detail model
func (m *DetailModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the detail view
func (m *DetailModel) Update(msg tea.Msg) (*DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.viewport.LineUp(1)
		case "down", "j":
			m.viewport.LineDown(1)
		case "pgup":
			m.viewport.HalfViewUp()
		case "pgdown":
			m.viewport.HalfViewDown()
		}
	}

	return m, nil
}

// View renders the detail view
func (m *DetailModel) View() string {
	var sb strings.Builder

	wi := m.workItem

	// Fixed header with ID and title (no type icon)
	sb.WriteString(m.styles.Header.Render(fmt.Sprintf("#%d: %s", wi.ID, wi.Fields.Title)))
	sb.WriteString("\n")

	// Type, state and priority
	sb.WriteString(m.styles.Muted.Render(fmt.Sprintf("%s  |  %s %s  |  P%d", wi.Fields.WorkItemType, wi.StateIcon(), wi.Fields.State, wi.Fields.Priority)))
	sb.WriteString("\n")

	// Separator
	separatorWidth := min(m.width-2, 60)
	if separatorWidth < 1 {
		separatorWidth = 60
	}
	sb.WriteString(strings.Repeat("─", separatorWidth))
	sb.WriteString("\n")

	// Scrollable viewport content
	if m.ready {
		sb.WriteString(m.viewport.View())
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width)

	return contentStyle.Render(sb.String())
}

// updateViewportContent builds the scrollable content and sets it in the viewport
func (m *DetailModel) updateViewportContent() {
	var sb strings.Builder
	wi := m.workItem

	// Assigned To
	if wi.Fields.AssignedTo != nil {
		sb.WriteString(m.styles.Label.Render("Assigned To: "))
		sb.WriteString(wi.Fields.AssignedTo.DisplayName)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(m.styles.Label.Render("Assigned To: "))
		sb.WriteString(m.styles.Muted.Render("Unassigned"))
		sb.WriteString("\n\n")
	}

	// Iteration Path
	if wi.Fields.IterationPath != "" {
		sb.WriteString(m.styles.Label.Render("Iteration: "))
		sb.WriteString(shortenIterationPath(wi.Fields.IterationPath))
		sb.WriteString("\n\n")
	}

	// Description (with HTML stripped)
	if wi.Fields.Description != "" {
		sb.WriteString(m.styles.Label.Render("Description"))
		sb.WriteString("\n")
		cleanDesc := stripHTMLTags(wi.Fields.Description)
		sb.WriteString(m.styles.Value.Render(cleanDesc))
		sb.WriteString("\n")
	} else {
		sb.WriteString(m.styles.Muted.Render("No description"))
		sb.WriteString("\n")
	}

	// Link to work item
	if m.client != nil {
		sb.WriteString("\n")
		url := buildWorkItemURL(m.client.GetOrg(), m.client.GetProject(), wi.ID)
		if url != "" {
			sb.WriteString(hyperlink(m.styles.Link.Render("Open in browser"), url))
			sb.WriteString("\n")
		}
	}

	m.viewport.SetContent(sb.String())
}

// SetSize sets the size of the detail view
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Account for header lines rendered in View(): title (1) + type/state (1) + separator (1) = 3
	headerLines := 3
	viewportHeight := height - headerLines
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if !m.ready {
		m.viewport = viewport.New(width, viewportHeight)
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = viewportHeight
	}

	// Update viewport content
	m.updateViewportContent()
}

// GetContextItems returns context items for the detail view
func (m *DetailModel) GetContextItems() []components.ContextItem {
	return []components.ContextItem{
		{Key: "↑↓", Description: "scroll"},
		{Key: "esc", Description: "back"},
	}
}

// GetScrollPercent returns the scroll percentage
func (m *DetailModel) GetScrollPercent() float64 {
	if !m.ready {
		return 0
	}
	return m.viewport.ScrollPercent() * 100
}

// GetStatusMessage returns the status message
func (m *DetailModel) GetStatusMessage() string {
	return ""
}

// GetWorkItem returns the work item
func (m *DetailModel) GetWorkItem() azdevops.WorkItem {
	return m.workItem
}

// Helper functions

// stripHTMLTags removes HTML tags from a string and converts to plain text
func stripHTMLTags(s string) string {
	// Convert block elements to newlines before stripping
	blockTags := regexp.MustCompile(`(?i)</(p|div|br|li|tr)>`)
	s = blockTags.ReplaceAllString(s, "\n")

	// Convert <br> and <br/> to newlines
	brTags := regexp.MustCompile(`(?i)<br\s*/?>`)
	s = brTags.ReplaceAllString(s, "\n")

	// Remove remaining HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")

	// Decode common HTML entities
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")

	// Clean up excessive blank lines (more than 2 newlines -> 2)
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}

	// Clean up spaces on each line
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	s = strings.Join(lines, "\n")

	return strings.TrimSpace(s)
}

// shortenIterationPath shortens a long iteration path
// e.g., "Project\\Sprint 1\\Week 1" -> "Sprint 1\\Week 1"
func shortenIterationPath(path string) string {
	parts := strings.Split(path, "\\")
	if len(parts) <= 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "\\")
}

// hyperlink creates an OSC 8 terminal hyperlink
func hyperlink(text, url string) string {
	if url == "" {
		return text
	}
	return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, text)
}

// buildWorkItemURL constructs the Azure DevOps URL to view a work item
func buildWorkItemURL(org, project string, id int) string {
	if org == "" || project == "" {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", org, project, id)
}
