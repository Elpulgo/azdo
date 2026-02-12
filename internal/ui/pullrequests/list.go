package pullrequests

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents the current view in the pull requests UI
type ViewMode int

const (
	ViewList   ViewMode = iota // PR list view
	ViewDetail                 // PR detail view with threads
)

// baseStyle is used for consistent styling (no border - table handles its own)
var baseStyle = lipgloss.NewStyle()

// Model represents the pull request list view with sub-views
type Model struct {
	table    table.Model
	client   *azdevops.Client
	prs      []azdevops.PullRequest
	loading  bool
	err      error
	width    int
	height   int
	viewMode ViewMode
	detail   *DetailModel
	spinner  *components.LoadingIndicator
	styles   *styles.Styles
}

// Column width ratios (percentages of available width)
const (
	statusWidthPct  = 10 // Status column percentage
	titleWidthPct   = 30 // Title column percentage
	branchWidthPct  = 20 // Branch column percentage
	authorWidthPct  = 15 // Author column percentage
	repoWidthPct    = 15 // Repository column percentage
	reviewsWidthPct = 10 // Reviews column percentage
)

// Minimum column widths
const (
	minStatusWidth  = 8
	minTitleWidth   = 15
	minBranchWidth  = 12
	minAuthorWidth  = 10
	minRepoWidth    = 10
	minReviewsWidth = 6
)

// NewModel creates a new pull request list model with default styles
func NewModel(client *azdevops.Client) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pull request list model with custom styles
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
	spinner.SetMessage("Loading pull requests...")

	return Model{
		table:    t,
		client:   client,
		prs:      []azdevops.PullRequest{},
		viewMode: ViewList,
		spinner:  spinner,
		styles:   s,
	}
}

// makeColumns creates table columns sized for the given width
func makeColumns(width int) []table.Column {
	// Account for table structure:
	// - 6 chars for column separators (between 6 columns)
	// - Some padding for cell content
	// Total overhead: ~10 chars
	available := width - 10
	if available < 70 {
		available = 70 // Minimum usable width
	}

	// Calculate widths based on percentages
	statusW := max(minStatusWidth, available*statusWidthPct/100)
	titleW := max(minTitleWidth, available*titleWidthPct/100)
	branchW := max(minBranchWidth, available*branchWidthPct/100)
	authorW := max(minAuthorWidth, available*authorWidthPct/100)
	repoW := max(minRepoWidth, available*repoWidthPct/100)
	reviewsW := max(minReviewsWidth, available*reviewsWidthPct/100)

	return []table.Column{
		{Title: "Status", Width: statusW},
		{Title: "Title", Width: titleW},
		{Title: "Branches", Width: branchW},
		{Title: "Author", Width: authorW},
		{Title: "Repo", Width: repoW},
		{Title: "Reviews", Width: reviewsW},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	m.spinner.SetVisible(true)
	return tea.Batch(fetchPullRequests(m.client), m.spinner.Init())
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
			return m, tea.Batch(fetchPullRequests(m.client), m.spinner.Tick())
		case "enter":
			// Navigate to detail view
			return m.enterDetailView()
		}

	case pullRequestsMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.prs = msg.prs
		m.table.SetRows(m.prsToRows())
		return m, nil

	case SetPRsMsg:
		// Direct update from polling - clear loading and error states
		m.loading = false
		m.spinner.SetVisible(false)
		m.err = nil
		m.prs = msg.PRs
		m.table.SetRows(m.prsToRows())
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

// enterDetailView navigates to the detail view for the selected PR
func (m Model) enterDetailView() (Model, tea.Cmd) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.prs) {
		return m, nil
	}

	selectedPR := m.prs[idx]
	m.detail = NewDetailModelWithStyles(m.client, selectedPR, m.styles)
	m.detail.SetSize(m.width, m.height)
	m.viewMode = ViewDetail

	return m, m.detail.Init()
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

// viewList renders the pull request list view
func (m Model) viewList() string {
	var content string

	if m.err != nil {
		content = fmt.Sprintf("Error loading pull requests: %v\n\nPress r to retry, q to quit", m.err)
	} else if m.loading {
		content = m.spinner.View() + "\n\nPress q to quit"
	} else if len(m.prs) == 0 {
		content = "No pull requests found.\n\nPress r to refresh, q to quit"
	} else {
		return baseStyle.Render(m.table.View())
	}

	// For non-table content, fill available height
	availableHeight := m.height - 5 // Account for tab bar and status bar
	if availableHeight < 1 {
		availableHeight = 10
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(availableHeight)

	return contentStyle.Render(content)
}

// prsToRows converts pull requests to table rows
func (m Model) prsToRows() []table.Row {
	rows := make([]table.Row, len(m.prs))
	for i, pr := range m.prs {
		branchInfo := fmt.Sprintf("%s → %s", pr.SourceBranchShortName(), pr.TargetBranchShortName())
		rows[i] = table.Row{
			statusIconWithStyles(pr.Status, pr.IsDraft, m.styles),
			pr.Title,
			branchInfo,
			pr.CreatedBy.DisplayName,
			pr.Repository.Name,
			voteIconWithStyles(pr.Reviewers, m.styles),
		}
	}
	return rows
}

// statusIcon returns a colored status icon for the pull request using default styles
func statusIcon(status string, isDraft bool) string {
	return statusIconWithStyles(status, isDraft, styles.DefaultStyles())
}

// statusIconWithStyles returns a colored status icon using the provided styles
func statusIconWithStyles(status string, isDraft bool, s *styles.Styles) string {
	statusLower := strings.ToLower(status)

	// Draft takes precedence
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

// voteIcon returns a summary icon for reviewer votes using default styles
func voteIcon(reviewers []azdevops.Reviewer) string {
	return voteIconWithStyles(reviewers, styles.DefaultStyles())
}

// voteIconWithStyles returns a summary icon for reviewer votes using provided styles
func voteIconWithStyles(reviewers []azdevops.Reviewer, s *styles.Styles) string {
	if len(reviewers) == 0 {
		return s.Muted.Render("-")
	}

	// Find the most significant vote (rejected > waiting > approved with suggestions > approved > no vote)
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
// PR detail view no longer shows context bar - scroll % is shown in status bar instead
func (m Model) HasContextBar() bool {
	return false
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
