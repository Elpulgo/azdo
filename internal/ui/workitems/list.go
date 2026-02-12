package workitems

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents the current view in the work items UI
type ViewMode int

const (
	ViewList   ViewMode = iota // Work items list view
	ViewDetail                 // Work item detail view
)

// baseStyle is used for consistent styling
var baseStyle = lipgloss.NewStyle()

// Model represents the work items list view with sub-views
type Model struct {
	table     table.Model
	client    *azdevops.Client
	workItems []azdevops.WorkItem
	loading   bool
	err       error
	width     int
	height    int
	viewMode  ViewMode
	detail    *DetailModel
	spinner   *components.LoadingIndicator
	styles    *styles.Styles
}

// Column width ratios (percentages of available width) - must total 100%
const (
	typeWidthPct     = 10 // Type column percentage (matches PR status column)
	idWidthPct       = 8  // ID column percentage
	titleWidthPct    = 32 // Title column percentage
	stateWidthPct    = 18 // State column percentage (needs space for "Ready for Test")
	priorityWidthPct = 6  // Priority column percentage
	assignedWidthPct = 26 // Assigned column percentage (10+8+32+18+6+26=100)
)

// Minimum column widths
const (
	minTypeWidth     = 8 // "Feature" (7 chars) + padding
	minIDWidth       = 6
	minTitleWidth    = 15
	minStateWidth    = 16
	minPriorityWidth = 4
	minAssignedWidth = 10
)

// NewModel creates a new work items list model with default styles
func NewModel(client *azdevops.Client) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new work items list model with custom styles
func NewModelWithStyles(client *azdevops.Client, s *styles.Styles) Model {
	// Start with minimum widths, will be resized on first WindowSizeMsg
	columns := makeColumns(80)

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	ts := table.DefaultStyles()
	ts.Header = s.TableHeader
	ts.Cell = s.TableCell
	ts.Selected = s.TableSelected
	t.SetStyles(ts)

	spinner := components.NewLoadingIndicator(s)
	spinner.SetMessage("Loading work items...")

	return Model{
		table:     t,
		client:    client,
		workItems: []azdevops.WorkItem{},
		viewMode:  ViewList,
		spinner:   spinner,
		styles:    s,
	}
}

// makeColumns creates table columns sized for the given width
func makeColumns(width int) []table.Column {
	// Account for table structure:
	// - 5 chars for column separators (between 6 columns)
	// - Some padding for cell content
	// Total overhead: ~10 chars
	available := width - 10
	if available < 70 {
		available = 70 // Minimum usable width
	}

	// Calculate widths based on percentages
	typeW := max(minTypeWidth, available*typeWidthPct/100)
	idW := max(minIDWidth, available*idWidthPct/100)
	titleW := max(minTitleWidth, available*titleWidthPct/100)
	stateW := max(minStateWidth, available*stateWidthPct/100)
	priorityW := max(minPriorityWidth, available*priorityWidthPct/100)
	assignedW := max(minAssignedWidth, available*assignedWidthPct/100)

	return []table.Column{
		{Title: "Type", Width: typeW},
		{Title: "ID", Width: idW},
		{Title: "Title", Width: titleW},
		{Title: "State", Width: stateW},
		{Title: "Pri", Width: priorityW},
		{Title: "Assigned", Width: assignedW},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	m.spinner.SetVisible(true)
	return tea.Batch(fetchWorkItems(m.client), m.spinner.Init())
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle window resize for all views
	if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wmsg.Width
		m.height = wmsg.Height
	}

	// Route to the appropriate view
	switch m.viewMode {
	case ViewDetail:
		return m.updateDetail(msg)
	default:
		return m.updateList(msg)
	}
}

// updateList handles updates for the list view
func (m Model) updateList(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetHeight(msg.Height - 5)
		m.table.SetColumns(makeColumns(msg.Width))

	case spinner.TickMsg:
		// Forward spinner tick messages
		if m.loading {
			var spinnerCmd tea.Cmd
			m.spinner, spinnerCmd = m.spinner.Update(msg)
			return m, spinnerCmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			// Manual refresh
			m.loading = true
			m.spinner.SetVisible(true)
			return m, tea.Batch(fetchWorkItems(m.client), m.spinner.Tick())
		case "enter":
			// Navigate to detail view
			return m.enterDetailView()
		}

	case workItemsMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.workItems = msg.workItems
		m.table.SetRows(m.workItemsToRows())
		return m, nil

	case SetWorkItemsMsg:
		// Direct update from polling - clear loading and error states
		m.loading = false
		m.spinner.SetVisible(false)
		m.err = nil
		m.workItems = msg.WorkItems
		m.table.SetRows(m.workItemsToRows())
		return m, nil
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// updateDetail handles updates for the detail view
func (m Model) updateDetail(msg tea.Msg) (Model, tea.Cmd) {
	if m.detail == nil {
		m.viewMode = ViewList
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Detail model handles its own sizing

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Go back to list
			m.viewMode = ViewList
			m.detail = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

// enterDetailView navigates to the detail view for the selected work item
func (m Model) enterDetailView() (Model, tea.Cmd) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.workItems) {
		return m, nil
	}

	selectedItem := m.workItems[idx]
	m.detail = NewDetailModelWithStyles(m.client, selectedItem, m.styles)
	m.detail.SetSize(m.width, m.height)
	m.viewMode = ViewDetail

	return m, nil
}

// View renders the view
func (m Model) View() string {
	switch m.viewMode {
	case ViewDetail:
		if m.detail != nil {
			return m.detail.View()
		}
	}

	// Default: list view
	return m.viewList()
}

// viewList renders the work items list view
func (m Model) viewList() string {
	var content string

	if m.err != nil {
		content = fmt.Sprintf("Error loading work items: %v\n\nPress r to retry, q to quit", m.err)
	} else if m.loading {
		content = m.spinner.View() + "\n\nPress q to quit"
	} else if len(m.workItems) == 0 {
		content = "No work items found.\n\nPress r to refresh, q to quit"
	} else {
		return baseStyle.Render(m.table.View())
	}

	// For non-table content, fill available height
	availableHeight := m.height - 5
	if availableHeight < 1 {
		availableHeight = 10
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(availableHeight)

	return contentStyle.Render(content)
}

// workItemsToRows converts work items to table rows
func (m Model) workItemsToRows() []table.Row {
	rows := make([]table.Row, len(m.workItems))
	for i, wi := range m.workItems {
		rows[i] = table.Row{
			typeIconWithStyles(wi.Fields.WorkItemType, m.styles),
			strconv.Itoa(wi.ID),
			wi.Fields.Title,
			stateTextWithStyles(wi.Fields.State, m.styles),
			priorityTextWithStyles(wi.Fields.Priority, m.styles),
			wi.AssignedToName(),
		}
	}
	return rows
}

// typeIcon returns a styled text label for the work item type using default styles
func typeIcon(workItemType string) string {
	return typeIconWithStyles(workItemType, styles.DefaultStyles())
}

// typeIconWithStyles returns a styled text label for the work item type using provided styles
func typeIconWithStyles(workItemType string, s *styles.Styles) string {
	// Create accent style from theme for Feature/Epic
	accentStyle := lipgloss.NewStyle().Foreground(s.Theme.Accent)

	switch workItemType {
	case "Bug":
		return s.Error.Render("Bug")
	case "Task":
		return s.Info.Render("Task")
	case "User Story":
		return s.Success.Render("Story")
	case "Feature":
		return accentStyle.Render("Feature")
	case "Epic":
		return s.Warning.Render("Epic")
	case "Issue":
		return s.Error.Render("Issue")
	default:
		return s.Muted.Render("Item")
	}
}

// stateText returns styled text for the work item state using default styles
// Workflow: New → Active → Resolved/Ready for Test → Closed
func stateText(state string) string {
	return stateTextWithStyles(state, styles.DefaultStyles())
}

// stateTextWithStyles returns styled text for the work item state using provided styles
func stateTextWithStyles(state string, s *styles.Styles) string {
	// Normalize for comparison
	stateLower := strings.ToLower(state)

	// Create secondary style for "Ready" states
	secondaryStyle := lipgloss.NewStyle().Foreground(s.Theme.Secondary)

	switch {
	case stateLower == "new":
		return s.Muted.Render("New")
	case stateLower == "active":
		return s.Info.Render("Active")
	case stateLower == "resolved":
		return s.Warning.Render("Resolved")
	case strings.Contains(stateLower, "ready"):
		return secondaryStyle.Render(state)
	case stateLower == "closed":
		return s.Success.Render("Closed")
	case stateLower == "removed":
		return s.Error.Render("Removed")
	default:
		return s.Muted.Render(state)
	}
}

// priorityText returns styled text for priority using default styles
func priorityText(priority int) string {
	return priorityTextWithStyles(priority, styles.DefaultStyles())
}

// priorityTextWithStyles returns styled text for priority using provided styles
func priorityTextWithStyles(priority int, s *styles.Styles) string {
	switch priority {
	case 1:
		return s.Error.Render("P1")
	case 2:
		return s.Warning.Render("P2")
	case 3:
		return s.Warning.Render("P3")
	case 4:
		return s.Muted.Render("P4")
	default:
		return s.Muted.Render(fmt.Sprintf("P%d", priority))
	}
}

// GetViewMode returns the current view mode (for testing)
func (m Model) GetViewMode() ViewMode {
	return m.viewMode
}

// GetContextItems returns context bar items for the current view
func (m Model) GetContextItems() []components.ContextItem {
	switch m.viewMode {
	case ViewDetail:
		if m.detail != nil {
			return m.detail.GetContextItems()
		}
	}
	// List view has no specific context items
	return nil
}

// GetScrollPercent returns the scroll percentage for the current view
func (m Model) GetScrollPercent() float64 {
	switch m.viewMode {
	case ViewDetail:
		if m.detail != nil {
			return m.detail.GetScrollPercent()
		}
	}
	return 0
}

// GetStatusMessage returns the status message for the current view
func (m Model) GetStatusMessage() string {
	switch m.viewMode {
	case ViewDetail:
		if m.detail != nil {
			return m.detail.GetStatusMessage()
		}
	}
	return ""
}

// HasContextBar returns true if the current view should show a context bar
func (m Model) HasContextBar() bool {
	return false
}

// Messages

type workItemsMsg struct {
	workItems []azdevops.WorkItem
	err       error
}

// SetWorkItemsMsg is a message to directly set the work items (from polling)
type SetWorkItemsMsg struct {
	WorkItems []azdevops.WorkItem
}

// fetchWorkItems fetches work items from Azure DevOps
func fetchWorkItems(client *azdevops.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return workItemsMsg{workItems: nil, err: nil}
		}
		workItems, err := client.ListWorkItems(50)
		return workItemsMsg{workItems: workItems, err: err}
	}
}
