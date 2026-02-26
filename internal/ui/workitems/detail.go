package workitems

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateUpdateResultMsg is sent when a state update completes
type stateUpdateResultMsg struct {
	newState string
	err      error
}

// statesLoadedMsg is sent when work item type states have been fetched
type statesLoadedMsg struct {
	states []azdevops.WorkItemTypeState
	err    error
}

// DetailModel represents the work item detail view
type DetailModel struct {
	client        *azdevops.Client
	workItem      azdevops.WorkItem
	width         int
	height        int
	viewport      viewport.Model
	ready         bool
	styles        *styles.Styles
	statePicker   components.StatePicker
	loading       bool
	spinner       *components.LoadingIndicator
	statusMessage string
}

// NewDetailModel creates a new work item detail model with default styles
func NewDetailModel(client *azdevops.Client, wi azdevops.WorkItem) *DetailModel {
	return NewDetailModelWithStyles(client, wi, styles.DefaultStyles())
}

// NewDetailModelWithStyles creates a new work item detail model with custom styles
func NewDetailModelWithStyles(client *azdevops.Client, wi azdevops.WorkItem, s *styles.Styles) *DetailModel {
	spinner := components.NewLoadingIndicator(s)
	return &DetailModel{
		client:      client,
		workItem:    wi,
		styles:      s,
		statePicker: components.NewStatePicker(s),
		spinner:     spinner,
	}
}

// Init initializes the detail model
func (m *DetailModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the detail view
func (m *DetailModel) Update(msg tea.Msg) (*DetailModel, tea.Cmd) {
	// Route input to state picker when visible
	if m.statePicker.IsVisible() {
		var cmd tea.Cmd
		m.statePicker, cmd = m.statePicker.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case components.StateSelectedMsg:
		m.loading = true
		m.spinner.SetVisible(true)
		m.spinner.SetMessage("Updating state...")
		return m, tea.Batch(m.updateState(msg.State), m.spinner.Tick())

	case stateUpdateResultMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.workItem.Fields.State = msg.newState
		m.statusMessage = fmt.Sprintf("State changed to %s", msg.newState)
		m.updateViewportContent()
		// Signal the list to refresh so the new state is visible
		return m, func() tea.Msg { return WorkItemStateChangedMsg{} }

	case statesLoadedMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.statePicker.SetStates(msg.states, m.workItem.Fields.State)
		m.statePicker.SetSize(m.width, m.height)
		m.statePicker.Show()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var spinnerCmd tea.Cmd
			m.spinner, spinnerCmd = m.spinner.Update(msg)
			return m, spinnerCmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			m.loading = true
			m.spinner.SetVisible(true)
			m.spinner.SetMessage("Loading states...")
			return m, tea.Batch(m.fetchStates(), m.spinner.Tick())
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

// fetchStates fetches available states for the work item type
func (m *DetailModel) fetchStates() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return statesLoadedMsg{err: fmt.Errorf("no client available")}
		}
		states, err := m.client.GetWorkItemTypeStates(m.workItem.Fields.WorkItemType)
		return statesLoadedMsg{states: states, err: err}
	}
}

// updateState sends the state update to the API
func (m *DetailModel) updateState(state string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return stateUpdateResultMsg{err: fmt.Errorf("no client available")}
		}
		err := m.client.UpdateWorkItemState(m.workItem.ID, state)
		if err != nil {
			return stateUpdateResultMsg{err: err}
		}
		return stateUpdateResultMsg{newState: state}
	}
}

// View renders the detail view
func (m *DetailModel) View() string {
	// State picker overlay takes precedence
	if m.statePicker.IsVisible() {
		return m.statePicker.View()
	}

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

	// Link to work item (shown before description for quick access)
	if m.client != nil {
		url := buildWorkItemURL(m.client.GetOrg(), m.client.GetProject(), wi.ID)
		if url != "" {
			sb.WriteString(hyperlink(m.styles.Link.Render("Open in browser"), url))
			sb.WriteString("\n\n")
		}
	}

	// Description (with HTML stripped)
	// Bugs use ReproSteps field; other types use Description
	effectiveDesc := wi.EffectiveDescription()
	if effectiveDesc != "" {
		sb.WriteString(m.styles.Label.Render("Description"))
		sb.WriteString("\n")
		cleanDesc := stripHTMLTags(effectiveDesc)
		sb.WriteString(m.styles.Value.Render(cleanDesc))
		sb.WriteString("\n")
	} else {
		sb.WriteString(m.styles.Muted.Render("No description"))
		sb.WriteString("\n")
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
		{Key: "s", Description: "Change state"},
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
	return m.statusMessage
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
