package workitems

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode re-exports listview.ViewMode for backward compatibility.
type ViewMode = listview.ViewMode

const (
	ViewList   = listview.ViewList
	ViewDetail = listview.ViewDetail
)

// Model represents the work items list view with sub-views
type Model struct {
	list   listview.Model[azdevops.WorkItem]
	client *azdevops.Client
	styles *styles.Styles
}

// NewModel creates a new work items list model with default styles
func NewModel(client *azdevops.Client) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new work items list model with custom styles
func NewModelWithStyles(client *azdevops.Client, s *styles.Styles) Model {
	cfg := listview.Config[azdevops.WorkItem]{
		Columns: []listview.ColumnSpec{
			{Title: "Type", WidthPct: 10, MinWidth: 8},
			{Title: "ID", WidthPct: 8, MinWidth: 6},
			{Title: "Title", WidthPct: 32, MinWidth: 15},
			{Title: "State", WidthPct: 18, MinWidth: 16},
			{Title: "Prio", WidthPct: 6, MinWidth: 4},
			{Title: "Assigned", WidthPct: 26, MinWidth: 10},
		},
		LoadingMessage: "Loading work items...",
		EntityName:     "work items",
		MinWidth:       70,
		ToRows:         workItemsToRows,
		Fetch: func() tea.Cmd {
			return fetchWorkItems(client)
		},
		EnterDetail: func(item azdevops.WorkItem, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			d := NewDetailModelWithStyles(client, item, st)
			d.SetSize(w, h)
			return &detailAdapter{d}, nil
		},
	}

	return Model{
		list:   listview.New(cfg, s),
		client: client,
		styles: s,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.list.Init()
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case workItemsMsg:
		m.list = m.list.HandleFetchResult(msg.workItems, msg.err)
		return m, nil
	case SetWorkItemsMsg:
		m.list = m.list.SetItems(msg.WorkItems)
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the view
func (m Model) View() string {
	return m.list.View()
}

// GetViewMode returns the current view mode (for testing)
func (m Model) GetViewMode() ViewMode {
	return m.list.GetViewMode()
}

// GetContextItems returns context bar items for the current view
func (m Model) GetContextItems() []components.ContextItem {
	return m.list.GetContextItems()
}

// GetScrollPercent returns the scroll percentage for the current view
func (m Model) GetScrollPercent() float64 {
	return m.list.GetScrollPercent()
}

// GetStatusMessage returns the status message for the current view
func (m Model) GetStatusMessage() string {
	return m.list.GetStatusMessage()
}

// HasContextBar returns true if the current view should show a context bar
func (m Model) HasContextBar() bool {
	return m.list.HasContextBar()
}

// detailAdapter wraps *DetailModel to satisfy listview.DetailView
type detailAdapter struct {
	model *DetailModel
}

func (a *detailAdapter) Update(msg tea.Msg) (listview.DetailView, tea.Cmd) {
	var cmd tea.Cmd
	a.model, cmd = a.model.Update(msg)
	return a, cmd
}

func (a *detailAdapter) View() string {
	return a.model.View()
}

func (a *detailAdapter) SetSize(width, height int) {
	a.model.SetSize(width, height)
}

func (a *detailAdapter) GetContextItems() []components.ContextItem {
	return a.model.GetContextItems()
}

func (a *detailAdapter) GetScrollPercent() float64 {
	return a.model.GetScrollPercent()
}

func (a *detailAdapter) GetStatusMessage() string {
	return a.model.GetStatusMessage()
}

// workItemsToRows converts work items to table rows
func workItemsToRows(items []azdevops.WorkItem, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, wi := range items {
		rows[i] = table.Row{
			typeIconWithStyles(wi.Fields.WorkItemType, s),
			strconv.Itoa(wi.ID),
			wi.Fields.Title,
			stateTextWithStyles(wi.Fields.State, s),
			priorityTextWithStyles(wi.Fields.Priority, s),
			wi.AssignedToName(),
		}
	}
	return rows
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

// Icon/text formatting functions (unchanged)

// typeIconWithStyles returns a styled text label for the work item type using provided styles
func typeIconWithStyles(workItemType string, s *styles.Styles) string {
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

// stateTextWithStyles returns styled text for the work item state using provided styles
func stateTextWithStyles(state string, s *styles.Styles) string {
	stateLower := strings.ToLower(state)
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
