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

// ViewMode re-exports listview.ViewMode for backward compatibility.
type ViewMode = listview.ViewMode

const (
	ViewList   = listview.ViewList
	ViewDetail = listview.ViewDetail
)

// Model represents the pull request list view with sub-views
type Model struct {
	list   listview.Model[azdevops.PullRequest]
	client *azdevops.Client
	styles *styles.Styles
}

// NewModel creates a new pull request list model with default styles
func NewModel(client *azdevops.Client) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pull request list model with custom styles
func NewModelWithStyles(client *azdevops.Client, s *styles.Styles) Model {
	cfg := listview.Config[azdevops.PullRequest]{
		Columns: []listview.ColumnSpec{
			{Title: "Status", WidthPct: 10, MinWidth: 8},
			{Title: "Title", WidthPct: 30, MinWidth: 15},
			{Title: "Branches", WidthPct: 20, MinWidth: 12},
			{Title: "Author", WidthPct: 15, MinWidth: 10},
			{Title: "Repo", WidthPct: 15, MinWidth: 10},
			{Title: "Reviews", WidthPct: 10, MinWidth: 6},
		},
		LoadingMessage: "Loading pull requests...",
		EntityName:     "pull requests",
		MinWidth:       70,
		ToRows:         prsToRows,
		Fetch: func() tea.Cmd {
			return fetchPullRequests(client)
		},
		EnterDetail: func(item azdevops.PullRequest, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			d := NewDetailModelWithStyles(client, item, st)
			d.SetSize(w, h)
			return &detailAdapter{d}, d.Init()
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
	case pullRequestsMsg:
		m.list = m.list.HandleFetchResult(msg.prs, msg.err)
		return m, nil
	case SetPRsMsg:
		m.list = m.list.SetItems(msg.PRs)
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

// Messages

type pullRequestsMsg struct {
	prs []azdevops.PullRequest
	err error
}

// SetPRsMsg is a message to directly set the pull requests (from polling)
type SetPRsMsg struct {
	PRs []azdevops.PullRequest
}

// fetchPullRequests fetches pull requests from Azure DevOps
func fetchPullRequests(client *azdevops.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return pullRequestsMsg{prs: nil, err: nil}
		}
		prs, err := client.ListPullRequests(25)
		return pullRequestsMsg{prs: prs, err: err}
	}
}
