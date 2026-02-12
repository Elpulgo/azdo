package pipelines

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents the current view in the pipelines UI
type ViewMode int

const (
	ViewList   ViewMode = iota // Pipeline list view
	ViewDetail                 // Pipeline detail/timeline view
	ViewLogs                   // Log viewer
)

// baseStyle is used for consistent styling (no border - table handles its own)
var baseStyle = lipgloss.NewStyle()

// Model represents the pipeline list view with sub-views
type Model struct {
	table     table.Model
	client    *azdevops.Client
	runs      []azdevops.PipelineRun
	loading   bool
	err       error
	width     int
	height    int
	viewMode  ViewMode
	detail    *DetailModel
	logViewer *LogViewerModel
	spinner   *components.LoadingIndicator
	styles    *styles.Styles
}

// Column width ratios (percentages of available width)
const (
	statusWidthPct    = 10 // Status column percentage
	pipelineWidthPct  = 25 // Pipeline column percentage
	branchWidthPct    = 22 // Branch column percentage
	buildWidthPct     = 13 // Build column percentage
	timestampWidthPct = 15 // Timestamp column percentage
	durationWidthPct  = 15 // Duration column percentage
)

// Minimum column widths
const (
	minStatusWidth    = 10
	minPipelineWidth  = 15
	minBranchWidth    = 10
	minBuildWidth     = 8
	minTimestampWidth = 16
	minDurationWidth  = 8
)

// NewModel creates a new pipeline list model with default styles
func NewModel(client *azdevops.Client) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pipeline list model with custom styles
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
	ts.Selected = s.TableSelected
	t.SetStyles(ts)

	spinner := components.NewLoadingIndicator(s)
	spinner.SetMessage("Loading pipeline runs...")

	return Model{
		table:    t,
		client:   client,
		runs:     []azdevops.PipelineRun{},
		viewMode: ViewList,
		spinner:  spinner,
		styles:   s,
	}
}

// makeColumns creates table columns sized for the given width
func makeColumns(width int) []table.Column {
	// Account for table structure:
	// - 5 chars for column separators (between 6 columns)
	// - Some padding for cell content
	// Total overhead: ~10 chars
	available := width - 10
	if available < 80 {
		available = 80 // Minimum usable width
	}

	// Calculate widths based on percentages
	statusW := max(minStatusWidth, available*statusWidthPct/100)
	pipelineW := max(minPipelineWidth, available*pipelineWidthPct/100)
	branchW := max(minBranchWidth, available*branchWidthPct/100)
	buildW := max(minBuildWidth, available*buildWidthPct/100)
	timestampW := max(minTimestampWidth, available*timestampWidthPct/100)
	durationW := max(minDurationWidth, available*durationWidthPct/100)

	return []table.Column{
		{Title: "Status", Width: statusW},
		{Title: "Pipeline", Width: pipelineW},
		{Title: "Branch", Width: branchW},
		{Title: "Build", Width: buildW},
		{Title: "Timestamp", Width: timestampW},
		{Title: "Duration", Width: durationW},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	m.spinner.SetVisible(true)
	return tea.Batch(fetchPipelineRuns(m.client), m.spinner.Init())
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
	case ViewLogs:
		return m.updateLogViewer(msg)
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
			return m, tea.Batch(fetchPipelineRuns(m.client), m.spinner.Tick())
		case "enter":
			// Navigate to detail view
			return m.enterDetailView()
		}

	case pipelineRunsMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.runs = msg.runs
		m.table.SetRows(m.runsToRows())
		return m, nil

	case SetRunsMsg:
		// Direct update from polling - clear loading and error states
		m.loading = false
		m.spinner.SetVisible(false)
		m.err = nil
		m.runs = msg.Runs
		m.table.SetRows(m.runsToRows())
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
		case "enter":
			// Navigate to log viewer if selected item has a log
			return m.enterLogView()
		}
	}

	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
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
			// Go back to detail
			m.viewMode = ViewDetail
			m.logViewer = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.logViewer, cmd = m.logViewer.Update(msg)
	return m, cmd
}

// enterDetailView navigates to the detail view for the selected pipeline
func (m Model) enterDetailView() (Model, tea.Cmd) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.runs) {
		return m, nil
	}

	selectedRun := m.runs[idx]
	m.detail = NewDetailModel(m.client, selectedRun)
	m.detail.SetSize(m.width, m.height)
	m.viewMode = ViewDetail

	return m, m.detail.Init()
}

// enterLogView navigates to the log viewer for the selected timeline item
func (m Model) enterLogView() (Model, tea.Cmd) {
	if m.detail == nil {
		return m, nil
	}

	selected := m.detail.SelectedItem()
	if selected == nil || selected.Record.Log == nil {
		// No log available for this item
		return m, nil
	}

	run := m.detail.GetRun()
	m.logViewer = NewLogViewerModel(
		m.client,
		run.ID,
		selected.Record.Log.ID,
		selected.Record.Name,
	)
	m.logViewer.SetSize(m.width, m.height)
	m.viewMode = ViewLogs

	return m, m.logViewer.Init()
}

// View renders the view
func (m Model) View() string {
	switch m.viewMode {
	case ViewDetail:
		if m.detail != nil {
			return m.detail.View()
		}
	case ViewLogs:
		if m.logViewer != nil {
			return m.logViewer.View()
		}
	}

	// Default: list view
	return m.viewList()
}

// viewList renders the pipeline list view
func (m Model) viewList() string {
	if m.err != nil {
		return fmt.Sprintf("Error loading pipeline runs: %v\n\nPress r to retry, q to quit", m.err)
	}

	if m.loading {
		return m.spinner.View() + "\n\nPress q to quit"
	}

	if len(m.runs) == 0 {
		return "No pipeline runs found.\n\nPress r to refresh, q to quit"
	}

	return baseStyle.Render(m.table.View())
}

// runsToRows converts pipeline runs to table rows
func (m Model) runsToRows() []table.Row {
	rows := make([]table.Row, len(m.runs))
	for i, run := range m.runs {
		rows[i] = table.Row{
			statusIconWithStyles(run.Status, run.Result, m.styles),
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
	// Use case-insensitive comparison for status values
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
		return s.Warning.Render("⚠ Partial")
	default:
		// Debug: show what we received
		return s.Muted.Render(fmt.Sprintf("%s/%s", status, result))
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
	case ViewLogs:
		if m.logViewer != nil {
			return m.logViewer.GetContextItems()
		}
	}
	// List view has no specific context items (uses main footer)
	return nil
}

// GetScrollPercent returns the scroll percentage for the current view
func (m Model) GetScrollPercent() float64 {
	switch m.viewMode {
	case ViewDetail:
		if m.detail != nil {
			return m.detail.GetScrollPercent()
		}
	case ViewLogs:
		if m.logViewer != nil {
			return m.logViewer.GetScrollPercent()
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
	return m.viewMode == ViewDetail || m.viewMode == ViewLogs
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
