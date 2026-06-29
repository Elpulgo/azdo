package pipelines

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/display"
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
	list         listview.Model[provider.PipelineRun]
	client       provider.Provider
	logViewer    *LogViewerModel
	viewMode     ViewMode
	width        int
	height       int
	styles       *styles.Styles
	activeStatus string
	statusPicker components.ListPicker
	allRuns      []provider.PipelineRun
}

// NewModel creates a new pipeline list model with default styles
func NewModel(client provider.Provider) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pipeline list model with custom styles
func NewModelWithStyles(client provider.Provider, s *styles.Styles) Model {
	isMulti := client != nil && client.IsMultiProject()

	columns := []listview.ColumnSpec{
		{Title: "Status", WidthPct: 10, MinWidth: 10},
		{Title: "Pipeline", WidthPct: 12, MinWidth: 15},
		{Title: "Branch", WidthPct: 20, MinWidth: 10},
		{Title: "Build", WidthPct: 24, MinWidth: 8},
		{Title: "Timestamp", WidthPct: 15, MinWidth: 16},
		{Title: "Duration", WidthPct: 10, MinWidth: 8},
	}

	if isMulti {
		columns = append(
			[]listview.ColumnSpec{{Title: "Project", WidthPct: 12, MinWidth: 10}},
			columns...,
		)
	}

	listview.NormalizeWidths(columns)

	toRows := runsToRows
	if isMulti {
		toRows = runsToRowsMulti
	}

	filterFunc := filterPipelineRun
	if isMulti {
		filterFunc = filterPipelineRunMulti
	}

	cfg := listview.Config[provider.PipelineRun]{
		Columns:        columns,
		LoadingMessage: "Loading pipeline runs...",
		EntityName:     "pipeline runs",
		MinWidth:       50,
		ToRows:         toRows,
		Fetch: func() tea.Cmd {
			return fetchPipelineRuns(client)
		},
		EnterDetail: func(item provider.PipelineRun, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			d := NewDetailModelWithStyles(client, item, st)
			d.SetSize(w, h)
			return &detailAdapter{d}, d.Init()
		},
		HasContextBar: func(mode listview.ViewMode) bool {
			return mode == listview.ViewDetail
		},
		FilterFunc: filterFunc,
	}

	return Model{
		list:         listview.New(cfg, s),
		client:       client,
		viewMode:     ViewList,
		styles:       s,
		statusPicker: components.NewListPicker(s),
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
		criticalCmd := components.NewCriticalErrorCmd(msg.err)
		if criticalCmd != nil {
			// Critical errors are shown via the error modal; don't display inline
			m.list = m.list.HandleFetchResult(nil, nil)
			return m, criticalCmd
		}
		// For partial errors, treat data as valid (some projects succeeded)
		var partialErr *azdevops.PartialError
		if errors.As(msg.err, &partialErr) {
			m.allRuns = msg.runs
			m.list = m.list.HandleFetchResult(m.applyStatusFilter(msg.runs), nil)
			return m, nil
		}
		m.allRuns = msg.runs
		m.list = m.list.HandleFetchResult(m.applyStatusFilter(msg.runs), msg.err)
		return m, nil
	case SetRunsMsg:
		m.allRuns = msg.Runs
		m.list = m.list.SetItems(m.applyStatusFilter(msg.Runs))
		return m, nil

	case components.ListPickerSelectedMsg:
		m.activeStatus = msg.Value
		m.statusPicker.Hide()
		m.list = m.list.SetItems(m.applyStatusFilter(m.allRuns))
		return m, nil
	}

	// When status picker is visible, route all input to it
	if m.statusPicker.IsVisible() {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			m.statusPicker, cmd = m.statusPicker.Update(kmsg)
			return m, cmd
		}
		return m, nil
	}

	// Route by pipeline-specific view mode
	switch m.viewMode {
	case ViewLogs:
		return m.updateLogViewer(msg)
	case ViewDetail:
		return m.updateDetail(msg)
	default:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "S" && !m.list.IsSearching() && m.viewMode == ViewList {
			statuses := getPipelineStatuses()
			options := make([]components.ListPickerOption, len(statuses))
			for i, status := range statuses {
				options[i] = components.ListPickerOption{Name: status.Name, Icon: status.Icon}
			}
			m.statusPicker.SetConfig("Filter by Status", options, m.activeStatus, true)
			m.statusPicker.Show()
			return m, nil
		}
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
		// When the detail view is searching, let it handle all keys
		// except enter (which should still toggle expand / open logs)
		if adapter, ok := m.list.Detail().(*detailAdapter); ok && adapter.model.IsSearching() {
			if keyMsg.String() != "enter" {
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				return m, cmd
			}
		}

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
	if selected == nil || selected.Record.LogID == 0 {
		return m, nil
	}

	run := detail.GetRun()
	m.logViewer = NewLogViewerModelWithStyles(
		m.client,
		run.Identity.Scope,
		parseBuildID(run.Identity.ID),
		selected.Record.LogID,
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

// IsSearching returns true if the list or detail view is currently in search/filter mode.
func (m Model) IsSearching() bool {
	if m.list.IsSearching() {
		return true
	}
	if m.viewMode == ViewDetail {
		if adapter, ok := m.list.Detail().(*detailAdapter); ok {
			return adapter.model.IsSearching()
		}
	}
	return false
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

// runsToRows converts pipeline runs to table rows.
// When the items span more than one distinct provider Kind (detected via
// display.MixedKinds), a leading glyph cell is prepended to each row so the
// user can tell which backend each entry originates from.
func runsToRows(items []provider.PipelineRun, s *styles.Styles) []table.Row {
	kinds := make([]provider.Kind, len(items))
	for i, run := range items {
		kinds[i] = run.Identity.Kind
	}
	mixed := display.MixedKinds(kinds)

	rows := make([]table.Row, len(items))
	for i, run := range items {
		cells := table.Row{
			statusIconWithStyles(run.RunStatus, s),
			run.DefinitionName,
			branchShortName(run.SourceBranch),
			run.BuildNumber,
			runTimestamp(run.QueueTime),
			runDuration(run.StartTime, run.FinishTime),
		}
		if mixed {
			cells = append(table.Row{display.KindStyle(run.Identity.Kind, s).Render(display.KindGlyph(run.Identity.Kind))}, cells...)
		}
		rows[i] = cells
	}
	return rows
}

// statusIconWithStyles returns a colored status icon using the provided styles.
// It uses the neutral RunStatus enum and the display map so theming is
// centralised in internal/ui/display.
func statusIconWithStyles(runStatus provider.RunStatus, s *styles.Styles) string {
	glyph := display.RunStatusGlyph(runStatus)
	label := display.RunStatusLabel(runStatus)
	style := display.RunStatusStyle(runStatus, s)
	if label == "" {
		// RunStatusUnknown (and detail-only statuses) have no list label;
		// render just the glyph to avoid showing an empty string.
		return style.Render(glyph)
	}
	return style.Render(glyph + " " + label)
}

// runsToRowsMulti converts pipeline runs to table rows with a Project column.
// When the items span more than one distinct provider Kind (detected via
// display.MixedKinds), a leading glyph cell is prepended before the Project
// column so the layout is: [glyph?] [project] [status] [pipeline] …
func runsToRowsMulti(items []provider.PipelineRun, s *styles.Styles) []table.Row {
	kinds := make([]provider.Kind, len(items))
	for i, run := range items {
		kinds[i] = run.Identity.Kind
	}
	mixed := display.MixedKinds(kinds)

	rows := make([]table.Row, len(items))
	for i, run := range items {
		cells := table.Row{
			run.Identity.ScopeDisplay,
			statusIconWithStyles(run.RunStatus, s),
			run.DefinitionName,
			branchShortName(run.SourceBranch),
			run.BuildNumber,
			runTimestamp(run.QueueTime),
			runDuration(run.StartTime, run.FinishTime),
		}
		if mixed {
			cells = append(table.Row{display.KindStyle(run.Identity.Kind, s).Render(display.KindGlyph(run.Identity.Kind))}, cells...)
		}
		rows[i] = cells
	}
	return rows
}

// filterPipelineRun returns true if the pipeline run matches the search query.
func filterPipelineRun(run provider.PipelineRun, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(run.DefinitionName), q) ||
		strings.Contains(strings.ToLower(run.SourceBranch), q) ||
		strings.Contains(strings.ToLower(run.BuildNumber), q)
}

// filterPipelineRunMulti matches pipeline run fields including project name.
func filterPipelineRunMulti(run provider.PipelineRun, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(run.Identity.ScopeDisplay), q) ||
		strings.Contains(strings.ToLower(run.Identity.Scope), q) ||
		strings.Contains(strings.ToLower(run.DefinitionName), q) ||
		strings.Contains(strings.ToLower(run.SourceBranch), q) ||
		strings.Contains(strings.ToLower(run.BuildNumber), q)
}

type pipelineStatus struct {
	Name string
	Icon string
}

func getPipelineStatuses() []pipelineStatus {
	return []pipelineStatus{
		{Name: "Running", Icon: "●"},
		{Name: "Queued", Icon: "○"},
		{Name: "Success", Icon: "✓"},
		{Name: "Failed", Icon: "✗"},
		{Name: "Cancel", Icon: "⊘"},
		{Name: "Partial", Icon: "◐"},
	}
}

func getStatusKey(runStatus provider.RunStatus) string {
	return display.RunStatusLabel(runStatus)
}

func (m Model) applyStatusFilter(runs []provider.PipelineRun) []provider.PipelineRun {
	if m.activeStatus == "" {
		return runs
	}
	var filtered []provider.PipelineRun
	for _, run := range runs {
		if getStatusKey(run.RunStatus) == m.activeStatus {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func (m Model) IsStatusFilterActive() bool {
	return m.activeStatus != ""
}

func (m Model) ActiveStatus() string {
	return m.activeStatus
}

func (m Model) IsStatusPickerVisible() bool {
	return m.statusPicker.IsVisible()
}

func (m Model) StatusPickerView() string {
	return m.statusPicker.View()
}

func (m *Model) SetStatusPickerSize(width, height int) {
	m.statusPicker.SetSize(width, height)
}

// Messages

type pipelineRunsMsg struct {
	runs []provider.PipelineRun
	err  error
}

// SetRunsMsg is a message to directly set the pipeline runs (from polling)
type SetRunsMsg struct {
	Runs []provider.PipelineRun
}

// fetchPipelineRuns fetches pipeline runs via the provider.
func fetchPipelineRuns(client provider.Provider) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return pipelineRunsMsg{runs: nil, err: nil}
		}
		runs, err := client.ListPipelineRuns(30, provider.ListOpts{})
		return pipelineRunsMsg{runs: runs, err: err}
	}
}

// parseBuildID parses the numeric build ID from the Identity.ID string.
// Returns 0 if the string cannot be parsed.
func parseBuildID(id string) int {
	n, _ := strconv.Atoi(id)
	return n
}

// branchShortName strips the refs/heads/ or refs/tags/ prefix from a branch ref.
func branchShortName(ref string) string {
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		return strings.TrimPrefix(ref, "refs/tags/")
	}
	return ref
}

// runTimestamp formats a queue time for display in the pipeline table.
func runTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// runDuration returns a human-readable duration for a pipeline run.
func runDuration(startTime, finishTime *time.Time) string {
	if startTime == nil || finishTime == nil {
		return "-"
	}
	d := finishTime.Sub(*startTime)
	return formatDuration(d)
}
