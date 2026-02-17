package pipelines

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// ViewMode represents the current view in the pipelines UI
type ViewMode int

const (
	ViewList   ViewMode = iota // Pipeline list view
	ViewDetail                 // Pipeline detail/timeline view
	ViewLogs                   // Log viewer
)

// Model represents the pipeline list view with sub-views
type Model struct {
	list      listview.Model[azdevops.PipelineRun]
	client    *azdevops.Client
	logViewer *LogViewerModel
	viewMode  ViewMode
	width     int
	height    int
	styles    *styles.Styles
}

// NewModel creates a new pipeline list model with default styles
func NewModel(client *azdevops.Client) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pipeline list model with custom styles
func NewModelWithStyles(client *azdevops.Client, s *styles.Styles) Model {
	cfg := listview.Config[azdevops.PipelineRun]{
		Columns: []listview.ColumnSpec{
			{Title: "Status", WidthPct: 10, MinWidth: 10},
			{Title: "Pipeline", WidthPct: 25, MinWidth: 15},
			{Title: "Branch", WidthPct: 22, MinWidth: 10},
			{Title: "Build", WidthPct: 13, MinWidth: 8},
			{Title: "Timestamp", WidthPct: 15, MinWidth: 16},
			{Title: "Duration", WidthPct: 15, MinWidth: 8},
		},
		LoadingMessage: "Loading pipeline runs...",
		EntityName:     "pipeline runs",
		MinWidth:       80,
		ToRows:         runsToRows,
		Fetch: func() tea.Cmd {
			return fetchPipelineRuns(client)
		},
		EnterDetail: func(item azdevops.PipelineRun, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			d := NewDetailModelWithStyles(client, item, st)
			d.SetSize(w, h)
			return &detailAdapter{d}, d.Init()
		},
		HasContextBar: func(mode listview.ViewMode) bool {
			// Pipeline always shows context bar in detail mode
			// ViewLogs is handled separately by the wrapper
			return mode == listview.ViewDetail
		},
	}

	return Model{
		list:     listview.New(cfg, s),
		client:   client,
		viewMode: ViewList,
		styles:   s,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.list.Init()
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Track window size for log viewer
	if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wmsg.Width
		m.height = wmsg.Height
	}

	// Handle domain-specific messages
	switch msg := msg.(type) {
	case pipelineRunsMsg:
		m.list = m.list.HandleFetchResult(msg.runs, msg.err)
		return m, nil
	case SetRunsMsg:
		m.list = m.list.SetItems(msg.Runs)
		return m, nil
	}

	// Route by pipeline-specific view mode
	switch m.viewMode {
	case ViewLogs:
		return m.updateLogViewer(msg)
	case ViewDetail:
		return m.updateDetail(msg)
	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		// Sync viewMode from generic model
		if m.list.GetViewMode() == listview.ViewDetail {
			m.viewMode = ViewDetail
		} else {
			m.viewMode = ViewList
		}
		return m, cmd
	}
}

// updateDetail intercepts detail-mode messages for enter (expand/collapse + log nav)
func (m Model) updateDetail(msg tea.Msg) (Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			// Get the detail adapter to access the underlying DetailModel
			if adapter, ok := m.list.Detail().(*detailAdapter); ok {
				detail := adapter.model
				if selected := detail.SelectedItem(); selected != nil && selected.HasChildren() {
					detail.ToggleExpand()
					return m, nil
				}
				return m.enterLogView(adapter)
			}
			return m, nil
		case "esc":
			// Delegate to generic model which handles esc -> list
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			m.viewMode = ViewList
			return m, cmd
		}
	}

	// Delegate other messages to the generic model
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// updateLogViewer handles updates for the log viewer
func (m Model) updateLogViewer(msg tea.Msg) (Model, tea.Cmd) {
	if m.logViewer == nil {
		m.viewMode = ViewDetail
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.viewMode = ViewDetail
			m.logViewer = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.logViewer, cmd = m.logViewer.Update(msg)
	return m, cmd
}

// enterLogView navigates to the log viewer for the selected timeline item
func (m Model) enterLogView(adapter *detailAdapter) (Model, tea.Cmd) {
	detail := adapter.model
	selected := detail.SelectedItem()
	if selected == nil || selected.Record.Log == nil {
		return m, nil
	}

	run := detail.GetRun()
	m.logViewer = NewLogViewerModelWithStyles(
		m.client,
		run.ID,
		selected.Record.Log.ID,
		selected.Record.Name,
		m.styles,
	)
	m.logViewer.SetSize(m.width, m.height)
	m.viewMode = ViewLogs

	return m, m.logViewer.Init()
}

// View renders the view
func (m Model) View() string {
	if m.viewMode == ViewLogs && m.logViewer != nil {
		return m.logViewer.View()
	}
	return m.list.View()
}

// GetViewMode returns the current view mode (for testing)
func (m Model) GetViewMode() ViewMode {
	return m.viewMode
}

// GetContextItems returns context bar items for the current view
func (m Model) GetContextItems() []components.ContextItem {
	if m.viewMode == ViewLogs && m.logViewer != nil {
		return m.logViewer.GetContextItems()
	}
	return m.list.GetContextItems()
}

// GetScrollPercent returns the scroll percentage for the current view
func (m Model) GetScrollPercent() float64 {
	if m.viewMode == ViewLogs && m.logViewer != nil {
		return m.logViewer.GetScrollPercent()
	}
	return m.list.GetScrollPercent()
}

// GetStatusMessage returns the status message for the current view
func (m Model) GetStatusMessage() string {
	return m.list.GetStatusMessage()
}

// HasContextBar returns true if the current view should show a context bar
func (m Model) HasContextBar() bool {
	if m.viewMode == ViewLogs {
		return true
	}
	return m.list.HasContextBar()
}

// TableHeight returns the table viewport height (for debugging).
func (m Model) TableHeight() int {
	return m.list.Table().Height()
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

// runsToRows converts pipeline runs to table rows
func runsToRows(items []azdevops.PipelineRun, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, run := range items {
		rows[i] = table.Row{
			statusIconWithStyles(run.Status, run.Result, s),
			run.Definition.Name,
			run.BranchShortName(),
			run.BuildNumber,
			run.Timestamp(),
			run.Duration(),
		}
	}
	return rows
}

// statusIcon returns a colored status icon for the pipeline run using default styles
func statusIcon(status, result string) string {
	return statusIconWithStyles(status, result, styles.DefaultStyles())
}

// statusIconWithStyles returns a colored status icon using the provided styles
func statusIconWithStyles(status, result string, s *styles.Styles) string {
	statusLower := strings.ToLower(status)
	resultLower := strings.ToLower(result)

	switch {
	case statusLower == "inprogress":
		return s.Info.Render("● Running")
	case statusLower == "notstarted":
		return s.Info.Render("○ Queued")
	case statusLower == "canceling":
		return s.Warning.Render("⊘ Cancel")
	case resultLower == "succeeded":
		return s.Success.Render("✓ Success")
	case resultLower == "failed":
		return s.Error.Render("✗ Failed")
	case resultLower == "canceled":
		return s.Muted.Render("○ Cancel")
	case resultLower == "partiallysucceeded":
		return s.Warning.Render("◐ Partial")
	default:
		return s.Muted.Render(fmt.Sprintf("%s/%s", status, result))
	}
}

// Messages

type pipelineRunsMsg struct {
	runs []azdevops.PipelineRun
	err  error
}

// SetRunsMsg is a message to directly set the pipeline runs (from polling)
type SetRunsMsg struct {
	Runs []azdevops.PipelineRun
}

// fetchPipelineRuns fetches pipeline runs from Azure DevOps
func fetchPipelineRuns(client *azdevops.Client) tea.Cmd {
	return func() tea.Msg {
		runs, err := client.ListPipelineRuns(30)
		return pipelineRunsMsg{runs: runs, err: err}
	}
}
