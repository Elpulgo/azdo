package workitems

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
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
}

// NewDetailModel creates a new work item detail model
func NewDetailModel(client *azdevops.Client, wi azdevops.WorkItem) *DetailModel {
	return &DetailModel{
		client:   client,
		workItem: wi,
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

	// Fixed header with work item type and ID
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	sb.WriteString(headerStyle.Render(fmt.Sprintf("%s #%d: %s", wi.TypeIcon(), wi.ID, wi.Fields.Title)))
	sb.WriteString("\n")

	// State and priority
	stateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	sb.WriteString(stateStyle.Render(fmt.Sprintf("State: %s %s  |  Priority: P%d", wi.StateIcon(), wi.Fields.State, wi.Fields.Priority)))
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

	// Fill available height
	availableHeight := m.height - 5
	if availableHeight < 1 {
		availableHeight = 10
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(availableHeight)

	return contentStyle.Render(sb.String())
}

// updateViewportContent builds the scrollable content and sets it in the viewport
func (m *DetailModel) updateViewportContent() {
	var sb strings.Builder
	wi := m.workItem

	// Assigned To
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
	if wi.Fields.AssignedTo != nil {
		sb.WriteString(labelStyle.Render("Assigned To: "))
		sb.WriteString(wi.Fields.AssignedTo.DisplayName)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(labelStyle.Render("Assigned To: "))
		grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		sb.WriteString(grayStyle.Render("Unassigned"))
		sb.WriteString("\n\n")
	}

	// Iteration Path
	if wi.Fields.IterationPath != "" {
		sb.WriteString(labelStyle.Render("Iteration: "))
		sb.WriteString(shortenIterationPath(wi.Fields.IterationPath))
		sb.WriteString("\n\n")
	}

	// Description (with HTML stripped)
	if wi.Fields.Description != "" {
		sb.WriteString(labelStyle.Render("Description"))
		sb.WriteString("\n")
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		cleanDesc := stripHTMLTags(wi.Fields.Description)
		sb.WriteString(descStyle.Render(cleanDesc))
		sb.WriteString("\n")
	} else {
		grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		sb.WriteString(grayStyle.Render("No description"))
		sb.WriteString("\n")
	}

	// Link to work item
	if m.client != nil {
		sb.WriteString("\n")
		linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Underline(true)
		url := buildWorkItemURL(m.client.GetOrg(), m.client.GetProject(), wi.ID)
		if url != "" {
			sb.WriteString(hyperlink(linkStyle.Render("Open in browser"), url))
			sb.WriteString("\n")
		}
	}

	m.viewport.SetContent(sb.String())
}

// SetSize sets the size of the detail view
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	viewportHeight := height - 8
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if !m.ready {
		m.viewport = viewport.New(width, viewportHeight)
		m.viewport.HighPerformanceRendering = false
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
