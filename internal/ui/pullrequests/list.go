package pullrequests

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

// ViewMode represents the current view in the pull requests UI
type ViewMode int

const (
	ViewList   ViewMode = iota // PR list view
	ViewDetail                 // PR detail view (description + threads)
	ViewDiff                   // Diff view (changed files + file diffs)
)

// Model represents the pull request list view with sub-views
type Model struct {
	list     listview.Model[azdevops.PullRequest]
	client   *azdevops.MultiClient
	diffView *DiffModel
	viewMode ViewMode
	width    int
	height   int
	styles   *styles.Styles
}

// NewModel creates a new pull request list model with default styles
func NewModel(client *azdevops.MultiClient) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pull request list model with custom styles
func NewModelWithStyles(client *azdevops.MultiClient, s *styles.Styles) Model {
	isMulti := client != nil && client.IsMultiProject()

	columns := []listview.ColumnSpec{
		{Title: "Status", WidthPct: 10, MinWidth: 8},
		{Title: "Title", WidthPct: 30, MinWidth: 15},
		{Title: "Branches", WidthPct: 20, MinWidth: 12},
		{Title: "Author", WidthPct: 15, MinWidth: 10},
		{Title: "Repo", WidthPct: 15, MinWidth: 10},
		{Title: "Reviews", WidthPct: 10, MinWidth: 6},
	}

	if isMulti {
		columns = append(
			[]listview.ColumnSpec{{Title: "Project", WidthPct: 10, MinWidth: 10}},
			columns...,
		)
	}

	listview.NormalizeWidths(columns)

	toRows := prsToRows
	if isMulti {
		toRows = prsToRowsMulti
	}

	filterFunc := filterPR
	if isMulti {
		filterFunc = filterPRMulti
	}

	cfg := listview.Config[azdevops.PullRequest]{
		Columns:        columns,
		LoadingMessage: "Loading pull requests...",
		EntityName:     "pull requests",
		MinWidth:       50,
		ToRows:         toRows,
		Fetch: func() tea.Cmd {
			return fetchPullRequestsMulti(client)
		},
		EnterDetail: func(item azdevops.PullRequest, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			var projectClient *azdevops.Client
			if client != nil {
				projectClient = client.ClientFor(item.ProjectName)
			}
			d := NewDetailModelWithStyles(projectClient, item, st)
			d.SetSize(w, h)
			return &detailAdapter{d}, d.Init()
		},
		HasContextBar: func(mode listview.ViewMode) bool {
			return mode == listview.ViewDetail
		},
		FilterFunc: filterFunc,
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
	// Track window size
	if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wmsg.Width
		m.height = wmsg.Height
	}

	// Handle domain-specific messages
	switch msg := msg.(type) {
	case pullRequestsMsg:
		m.list = m.list.HandleFetchResult(msg.prs, msg.err)
		return m, nil
	case SetPRsMsg:
		m.list = m.list.SetItems(msg.PRs)
		return m, nil
	}

	// Route by view mode
	switch m.viewMode {
	case ViewDiff:
		return m.updateDiffView(msg)
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

// updateDetail intercepts detail-mode messages for file selection to enter diff view
func (m Model) updateDetail(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case openGeneralCommentsMsg:
		// User pressed Enter on general comments in the detail view
		if adapter, ok := m.list.Detail().(*detailAdapter); ok {
			detail := adapter.model
			pr := detail.GetPR()
			threads := detail.GetThreads()
			var projectClient *azdevops.Client
			if m.client != nil {
				projectClient = m.client.ClientFor(pr.ProjectName)
			}
			m.diffView = NewDiffModel(projectClient, pr, threads, m.styles)
			m.diffView.SetSize(m.width, m.height)
			m.viewMode = ViewDiff
			// Open directly into general comments view
			return m, m.diffView.InitGeneralComments()
		}
		return m, nil

	case openFileDiffMsg:
		// User pressed Enter on a file in the detail view - open diff for that file
		if adapter, ok := m.list.Detail().(*detailAdapter); ok {
			detail := adapter.model
			pr := detail.GetPR()
			threads := detail.GetThreads()
			var projectClient *azdevops.Client
			if m.client != nil {
				projectClient = m.client.ClientFor(pr.ProjectName)
			}
			m.diffView = NewDiffModel(projectClient, pr, threads, m.styles)
			m.diffView.SetSize(m.width, m.height)
			m.viewMode = ViewDiff
			// Initialize and immediately open the selected file
			return m, m.diffView.InitWithFile(msg.file)
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "esc" {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			m.viewMode = ViewList
			return m, cmd
		}
	}

	// Delegate to generic model
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// updateDiffView handles messages when in diff view mode
func (m Model) updateDiffView(msg tea.Msg) (Model, tea.Cmd) {
	if m.diffView == nil {
		m.viewMode = ViewDetail
		return m, nil
	}

	switch msg := msg.(type) {
	case exitDiffViewMsg:
		m.viewMode = ViewDetail
		m.diffView = nil
		return m, nil
	case tea.WindowSizeMsg:
		m.diffView.SetSize(msg.Width, msg.Height)
	}

	var cmd tea.Cmd
	m.diffView, cmd = m.diffView.Update(msg)
	return m, cmd
}

// View renders the view
func (m Model) View() string {
	if m.viewMode == ViewDiff && m.diffView != nil {
		return m.diffView.View()
	}
	return m.list.View()
}

// GetViewMode returns the current view mode (for testing)
func (m Model) GetViewMode() ViewMode {
	return m.viewMode
}

// GetContextItems returns context bar items for the current view
func (m Model) GetContextItems() []components.ContextItem {
	if m.viewMode == ViewDiff && m.diffView != nil {
		return m.diffView.GetContextItems()
	}
	return m.list.GetContextItems()
}

// GetScrollPercent returns the scroll percentage for the current view
func (m Model) GetScrollPercent() float64 {
	if m.viewMode == ViewDiff && m.diffView != nil {
		return m.diffView.GetScrollPercent()
	}
	return m.list.GetScrollPercent()
}

// GetStatusMessage returns the status message for the current view
func (m Model) GetStatusMessage() string {
	if m.viewMode == ViewDiff && m.diffView != nil {
		return m.diffView.GetStatusMessage()
	}
	return m.list.GetStatusMessage()
}

// HasContextBar returns true if the current view should show a context bar
func (m Model) HasContextBar() bool {
	if m.viewMode == ViewDiff {
		return true
	}
	return m.list.HasContextBar()
}

// IsSearching returns true if the view has an active text input that should
// suppress global keyboard shortcuts. This includes search/filter mode and
// comment/reply input in the diff view.
func (m Model) IsSearching() bool {
	if m.list.IsSearching() {
		return true
	}
	if m.viewMode == ViewDiff && m.diffView != nil && m.diffView.IsInputActive() {
		return true
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

// prsToRows converts pull requests to table rows
func prsToRows(items []azdevops.PullRequest, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, pr := range items {
		branchInfo := fmt.Sprintf("%s → %s", pr.SourceBranchShortName(), pr.TargetBranchShortName())
		rows[i] = table.Row{
			statusIconWithStyles(pr.Status, pr.IsDraft, s),
			pr.Title,
			branchInfo,
			pr.CreatedBy.DisplayName,
			pr.Repository.Name,
			voteIconWithStyles(pr.Reviewers, s),
		}
	}
	return rows
}

// statusIconWithStyles returns a colored status icon using the provided styles
func statusIconWithStyles(status string, isDraft bool, s *styles.Styles) string {
	statusLower := strings.ToLower(status)

	if isDraft {
		return s.Warning.Render("◐ Draft")
	}

	switch statusLower {
	case "active":
		return s.Info.Render("● Active")
	case "completed":
		return s.Success.Render("✓ Merged")
	case "abandoned":
		return s.Muted.Render("○ Closed")
	default:
		return s.Muted.Render(status)
	}
}

// voteIconWithStyles returns a summary icon for reviewer votes using provided styles
func voteIconWithStyles(reviewers []azdevops.Reviewer, s *styles.Styles) string {
	if len(reviewers) == 0 {
		return s.Muted.Render("-")
	}

	hasRejected := false
	hasWaiting := false
	hasApprovedWithSuggestions := false
	hasApproved := false
	hasNoVote := false

	for _, r := range reviewers {
		switch r.Vote {
		case -10:
			hasRejected = true
		case -5:
			hasWaiting = true
		case 5:
			hasApprovedWithSuggestions = true
		case 10:
			hasApproved = true
		case 0:
			hasNoVote = true
		}
	}

	count := len(reviewers)

	switch {
	case hasRejected:
		return s.Error.Render(fmt.Sprintf("✗ %d", count))
	case hasWaiting:
		return s.Warning.Render(fmt.Sprintf("◐ %d", count))
	case hasApprovedWithSuggestions:
		return s.Warning.Render(fmt.Sprintf("~ %d", count))
	case hasApproved:
		return s.Success.Render(fmt.Sprintf("✓ %d", count))
	case hasNoVote:
		return s.Muted.Render(fmt.Sprintf("○ %d", count))
	default:
		return s.Muted.Render(fmt.Sprintf("- %d", count))
	}
}

// prsToRowsMulti converts pull requests to table rows with a Project column.
func prsToRowsMulti(items []azdevops.PullRequest, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, pr := range items {
		branchInfo := fmt.Sprintf("%s → %s", pr.SourceBranchShortName(), pr.TargetBranchShortName())
		rows[i] = table.Row{
			pr.ProjectName,
			statusIconWithStyles(pr.Status, pr.IsDraft, s),
			pr.Title,
			branchInfo,
			pr.CreatedBy.DisplayName,
			pr.Repository.Name,
			voteIconWithStyles(pr.Reviewers, s),
		}
	}
	return rows
}

// filterPR returns true if the pull request matches the search query.
func filterPR(pr azdevops.PullRequest, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(pr.Title), q) ||
		strings.Contains(strings.ToLower(pr.CreatedBy.DisplayName), q) ||
		strings.Contains(strings.ToLower(pr.Repository.Name), q) ||
		strings.Contains(strings.ToLower(pr.SourceRefName), q) ||
		strings.Contains(strings.ToLower(pr.TargetRefName), q)
}

// filterPRMulti matches PR fields including project name.
func filterPRMulti(pr azdevops.PullRequest, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(pr.ProjectName), q) ||
		strings.Contains(strings.ToLower(pr.Title), q) ||
		strings.Contains(strings.ToLower(pr.CreatedBy.DisplayName), q) ||
		strings.Contains(strings.ToLower(pr.Repository.Name), q) ||
		strings.Contains(strings.ToLower(pr.SourceRefName), q) ||
		strings.Contains(strings.ToLower(pr.TargetRefName), q)
}

// Messages

type pullRequestsMsg struct {
	prs []azdevops.PullRequest
	err error
}

// SetPRsMsg is a message to directly set the pull requests (from polling)
type SetPRsMsg struct {
	PRs []azdevops.PullRequest
}

// fetchPullRequestsMulti fetches pull requests from all projects via MultiClient.
func fetchPullRequestsMulti(client *azdevops.MultiClient) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return pullRequestsMsg{prs: nil, err: nil}
		}
		prs, err := client.ListPullRequests(25)
		return pullRequestsMsg{prs: prs, err: err}
	}
}
