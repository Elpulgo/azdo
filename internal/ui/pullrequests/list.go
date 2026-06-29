package pullrequests

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/display"
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
	list           listview.Model[provider.PullRequest]
	client         provider.Provider
	diffView       *DiffModel
	viewMode       ViewMode
	width          int
	height         int
	styles         *styles.Styles
	myPRsOnly      bool
	asReviewerOnly bool
	allPRs         []provider.PullRequest
	myPRs          []provider.PullRequest
	asReviewerPRs  []provider.PullRequest

	// pendingDetailID is the PR ID requested by startup state restore.
	// Cleared on the first populate (whether or not the lookup succeeded)
	// so subsequent polls cannot hijack the user back into detail view.
	pendingDetailID       int
	pendingRestoreHandled bool
}

// NewModel creates a new pull request list model with default styles
func NewModel(client provider.Provider) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new pull request list model with custom styles
func NewModelWithStyles(client provider.Provider, s *styles.Styles) Model {
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

	cfg := listview.Config[provider.PullRequest]{
		Columns:        columns,
		LoadingMessage: "Loading pull requests...",
		EntityName:     "pull requests",
		MinWidth:       50,
		ToRows:         toRows,
		Fetch: func() tea.Cmd {
			return fetchPullRequestsMulti(client)
		},
		EnterDetail: func(item provider.PullRequest, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
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
		criticalCmd := components.NewCriticalErrorCmd(msg.err)
		if criticalCmd != nil {
			m.list = m.list.HandleFetchResult(nil, nil)
			return m, criticalCmd
		}
		var partialErr *azdevops.PartialError
		if errors.As(msg.err, &partialErr) {
			m.allPRs = msg.prs
			if m.myPRsOnly {
				return m, fetchMyPullRequestsMulti(m.client)
			}
			if m.asReviewerOnly {
				return m, fetchPullRequestsAsReviewerMulti(m.client)
			}
			m.list = m.list.HandleFetchResult(msg.prs, nil)
			return m.withRestore(nil)
		}
		m.allPRs = msg.prs
		if m.myPRsOnly {
			return m, fetchMyPullRequestsMulti(m.client)
		}
		if m.asReviewerOnly {
			return m, fetchPullRequestsAsReviewerMulti(m.client)
		}
		m.list = m.list.HandleFetchResult(msg.prs, msg.err)
		return m.withRestore(nil)
	case myPullRequestsMsg:
		if msg.err != nil {
			var partialErr *azdevops.PartialError
			if errors.As(msg.err, &partialErr) {
				m.myPRs = msg.prs
				m.list = m.list.SetItems(msg.prs)
				return m.withRestore(nil)
			}
			// On error, fall back to showing all items
			m.myPRsOnly = false
			m.myPRs = nil
			m.list = m.list.SetItems(m.allPRs)
			return m.withRestore(nil)
		}
		m.myPRs = msg.prs
		m.list = m.list.SetItems(msg.prs)
		return m.withRestore(nil)
	case asReviewerPullRequestsMsg:
		if msg.err != nil {
			var partialErr *azdevops.PartialError
			if errors.As(msg.err, &partialErr) {
				m.asReviewerPRs = msg.prs
				m.list = m.list.SetItems(msg.prs)
				return m.withRestore(nil)
			}
			m.asReviewerOnly = false
			m.asReviewerPRs = nil
			m.list = m.list.SetItems(m.allPRs)
			return m.withRestore(nil)
		}
		m.asReviewerPRs = msg.prs
		m.list = m.list.SetItems(msg.prs)
		return m.withRestore(nil)
	case SetPRsMsg:
		m.allPRs = msg.PRs
		if !m.myPRsOnly && !m.asReviewerOnly {
			m.list = m.list.SetItems(msg.PRs)
			return m.withRestore(nil)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "m" && !m.list.IsSearching() && m.viewMode == ViewList {
			m.myPRsOnly = !m.myPRsOnly
			if m.myPRsOnly {
				// Mutually exclusive with as-reviewer
				m.asReviewerOnly = false
				m.asReviewerPRs = nil
				return m, fetchMyPullRequestsMulti(m.client)
			}
			m.myPRs = nil
			m.list = m.list.SetItems(m.allPRs)
			return m, nil
		}
		if msg.String() == "A" && !m.list.IsSearching() && m.viewMode == ViewList {
			m.asReviewerOnly = !m.asReviewerOnly
			if m.asReviewerOnly {
				m.myPRsOnly = false
				m.myPRs = nil
				return m, fetchPullRequestsAsReviewerMulti(m.client)
			}
			m.asReviewerPRs = nil
			m.list = m.list.SetItems(m.allPRs)
			return m, nil
		}
		// esc clears an active "my PRs" / "as-reviewer" filter, mirroring how
		// esc exits search. It only ever turns a filter OFF — never on — so the
		// full list is restored. When searching, esc is left to exit search first.
		if msg.String() == "esc" && !m.list.IsSearching() && m.viewMode == ViewList {
			if m.myPRsOnly {
				m.myPRsOnly = false
				m.myPRs = nil
				m.list = m.list.SetItems(m.allPRs)
				return m, nil
			}
			if m.asReviewerOnly {
				m.asReviewerOnly = false
				m.asReviewerPRs = nil
				m.list = m.list.SetItems(m.allPRs)
				return m, nil
			}
		}
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
			m.diffView = NewDiffModel(m.client, pr, threads, m.styles)
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
			m.diffView = NewDiffModel(m.client, pr, threads, m.styles)
			m.diffView.SetSize(m.width, m.height)
			m.viewMode = ViewDiff
			// Initialize and immediately open the selected file
			return m, m.diffView.InitWithFile(msg.file)
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "esc" {
			// If the detail view has a modal open (e.g. vote picker),
			// let it handle esc first instead of navigating back
			if adapter, ok := m.list.Detail().(*detailAdapter); ok {
				if adapter.model.votePicker.IsVisible() {
					var cmd tea.Cmd
					m.list, cmd = m.list.Update(msg)
					return m, cmd
				}
			}
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

// IsMyPRsActive returns true if the "my PRs" filter is active.
func (m Model) IsMyPRsActive() bool {
	return m.myPRsOnly
}

// DetailItemID returns the ID of the PR whose detail view is currently
// open, or 0 when the user is on the list. Used by the state store to
// persist the last-viewed PR across sessions.
func (m Model) DetailItemID() int {
	if m.viewMode != ViewDetail {
		return 0
	}
	adapter, ok := m.list.Detail().(*detailAdapter)
	if !ok || adapter == nil {
		return 0
	}
	return adapter.model.GetPRID()
}

// WithPendingDetailRestore queues a request to open the PR with this ID in
// detail view as soon as the list is populated. The pending intent is
// consumed by the first populate event — found or not — so polling
// refreshes cannot re-trigger it.
func (m Model) WithPendingDetailRestore(id int) Model {
	m.pendingDetailID = id
	m.pendingRestoreHandled = false
	return m
}

// tryRestoreDetail attempts to open detail for the pending ID, if any.
// Returns the (possibly updated) model and the detail's Init cmd. Always
// marks the intent as handled on the first call.
func (m Model) tryRestoreDetail() (Model, tea.Cmd) {
	if m.pendingRestoreHandled || m.pendingDetailID == 0 {
		return m, nil
	}
	target := m.pendingDetailID
	m.pendingDetailID = 0
	m.pendingRestoreHandled = true

	idx := m.list.FindIndex(func(pr provider.PullRequest) bool {
		return prNumericID(pr) == target
	})
	if idx < 0 {
		return m, nil
	}
	m.list.SetCursor(idx)
	list, cmd := m.list.OpenSelectedDetail()
	m.list = list
	m.viewMode = ViewDetail
	return m, cmd
}

// withRestore is a small adapter used at populate sites: it runs restore
// (if any) and returns the combined command alongside any caller cmd.
func (m Model) withRestore(prev tea.Cmd) (Model, tea.Cmd) {
	m, restoreCmd := m.tryRestoreDetail()
	switch {
	case prev == nil:
		return m, restoreCmd
	case restoreCmd == nil:
		return m, prev
	default:
		return m, tea.Batch(prev, restoreCmd)
	}
}

// IsAsReviewerActive returns true if the "as reviewer" filter is active.
func (m Model) IsAsReviewerActive() bool {
	return m.asReviewerOnly
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

// branchShortName strips the refs/heads/ prefix from a ref name.
func branchShortName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// prsToRows converts pull requests to table rows.
// When the items span more than one distinct provider Kind (detected via
// display.MixedKinds), a leading glyph cell is prepended to each row so the
// user can tell which backend each entry originates from.
func prsToRows(items []provider.PullRequest, s *styles.Styles) []table.Row {
	kinds := make([]provider.Kind, len(items))
	for i, pr := range items {
		kinds[i] = pr.Identity.Kind
	}
	mixed := display.MixedKinds(kinds)

	rows := make([]table.Row, len(items))
	for i, pr := range items {
		branchInfo := fmt.Sprintf("%s → %s", branchShortName(pr.SourceRefName), branchShortName(pr.TargetRefName))
		cells := table.Row{
			statusIconWithStyles(pr.StatusCategory, pr.IsDraft, s),
			pr.Title,
			branchInfo,
			pr.CreatedByName,
			pr.RepositoryName,
			voteIconWithStyles(pr.Reviewers, s),
		}
		if mixed {
			cells = append(table.Row{display.KindStyle(pr.Identity.Kind, s).Render(display.KindGlyph(pr.Identity.Kind))}, cells...)
		}
		rows[i] = cells
	}
	return rows
}

// statusIconWithStyles returns a colored status icon using the provided styles.
// isDraft takes precedence over StatusCategory. For the "completed" (merged) status,
// the label "Merged" is used. For unknown/unmapped statuses the raw status string is
// used as fallback via a separate path in the caller.
func statusIconWithStyles(statusCat provider.StateCategory, isDraft bool, s *styles.Styles) string {
	if isDraft {
		return s.Warning.Render("◐ Draft")
	}

	switch statusCat {
	case provider.StateCategoryActive:
		// PR active uses filled circle, not the work-item half-circle glyph.
		return s.Info.Render("● Active")
	case provider.StateCategoryClosedDone:
		// PR "completed" renders as "Merged", not "Closed".
		return display.StateStyle(statusCat, s).Render(display.StateGlyph(statusCat) + " Merged")
	case provider.StateCategoryRemoved:
		// PR "abandoned" renders as "○ Closed" with Muted style, not the work-item ✗/Error.
		return s.Muted.Render("○ Closed")
	default:
		// Unknown/custom: the raw status string is unavailable at this point
		// so fall back to the glyph-only rendering from the display map.
		return display.StateStyle(statusCat, s).Render(display.StateGlyph(statusCat))
	}
}

// voteIconWithStyles returns a summary icon for reviewer votes using provided styles.
// Priority order (most significant wins): Rejected > WaitingForAuthor >
// ApprovedWithSuggestions > Approved > NoVote.
func voteIconWithStyles(reviewers []provider.Reviewer, s *styles.Styles) string {
	if len(reviewers) == 0 {
		return s.Muted.Render("-")
	}

	hasRejected := false
	hasWaiting := false
	hasApprovedWithSuggestions := false
	hasApproved := false
	hasNoVote := false

	for _, r := range reviewers {
		switch r.Kind {
		case provider.VoteKindRejected:
			hasRejected = true
		case provider.VoteKindWaitingForAuthor:
			hasWaiting = true
		case provider.VoteKindApprovedWithSuggestions:
			hasApprovedWithSuggestions = true
		case provider.VoteKindApproved:
			hasApproved = true
		case provider.VoteKindNoVote:
			hasNoVote = true
		}
	}

	count := len(reviewers)

	switch {
	case hasRejected:
		return display.VoteStyle(provider.VoteKindRejected, s).Render(fmt.Sprintf("%s %d", display.VoteGlyph(provider.VoteKindRejected), count))
	case hasWaiting:
		return display.VoteStyle(provider.VoteKindWaitingForAuthor, s).Render(fmt.Sprintf("%s %d", display.VoteGlyph(provider.VoteKindWaitingForAuthor), count))
	case hasApprovedWithSuggestions:
		return display.VoteStyle(provider.VoteKindApprovedWithSuggestions, s).Render(fmt.Sprintf("%s %d", display.VoteGlyph(provider.VoteKindApprovedWithSuggestions), count))
	case hasApproved:
		return display.VoteStyle(provider.VoteKindApproved, s).Render(fmt.Sprintf("%s %d", display.VoteGlyph(provider.VoteKindApproved), count))
	case hasNoVote:
		return display.VoteStyle(provider.VoteKindNoVote, s).Render(fmt.Sprintf("%s %d", display.VoteGlyph(provider.VoteKindNoVote), count))
	default:
		return s.Muted.Render(fmt.Sprintf("- %d", count))
	}
}

// prsToRowsMulti converts pull requests to table rows with a Project column.
// When the items span more than one distinct provider Kind (detected via
// display.MixedKinds), a leading glyph cell is prepended before the Project
// column so the layout is: [glyph?] [project] [status] [title] …
func prsToRowsMulti(items []provider.PullRequest, s *styles.Styles) []table.Row {
	kinds := make([]provider.Kind, len(items))
	for i, pr := range items {
		kinds[i] = pr.Identity.Kind
	}
	mixed := display.MixedKinds(kinds)

	rows := make([]table.Row, len(items))
	for i, pr := range items {
		branchInfo := fmt.Sprintf("%s → %s", branchShortName(pr.SourceRefName), branchShortName(pr.TargetRefName))
		cells := table.Row{
			pr.Identity.ScopeDisplay,
			statusIconWithStyles(pr.StatusCategory, pr.IsDraft, s),
			pr.Title,
			branchInfo,
			pr.CreatedByName,
			pr.RepositoryName,
			voteIconWithStyles(pr.Reviewers, s),
		}
		if mixed {
			cells = append(table.Row{display.KindStyle(pr.Identity.Kind, s).Render(display.KindGlyph(pr.Identity.Kind))}, cells...)
		}
		rows[i] = cells
	}
	return rows
}

// filterPR returns true if the pull request matches the search query.
func filterPR(pr provider.PullRequest, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(pr.Title), q) ||
		strings.Contains(strings.ToLower(pr.CreatedByName), q) ||
		strings.Contains(strings.ToLower(pr.RepositoryName), q) ||
		strings.Contains(strings.ToLower(pr.SourceRefName), q) ||
		strings.Contains(strings.ToLower(pr.TargetRefName), q)
}

// filterPRMulti matches PR fields including project name.
func filterPRMulti(pr provider.PullRequest, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(pr.Identity.ScopeDisplay), q) ||
		strings.Contains(strings.ToLower(pr.Identity.Scope), q) ||
		strings.Contains(strings.ToLower(pr.Title), q) ||
		strings.Contains(strings.ToLower(pr.CreatedByName), q) ||
		strings.Contains(strings.ToLower(pr.RepositoryName), q) ||
		strings.Contains(strings.ToLower(pr.SourceRefName), q) ||
		strings.Contains(strings.ToLower(pr.TargetRefName), q)
}

// prNumericID parses the numeric PR ID from Identity.ID.
// Returns 0 if the ID cannot be parsed.
func prNumericID(pr provider.PullRequest) int {
	id := 0
	fmt.Sscanf(pr.Identity.ID, "%d", &id)
	return id
}

// Messages

type pullRequestsMsg struct {
	prs []provider.PullRequest
	err error
}

type myPullRequestsMsg struct {
	prs []provider.PullRequest
	err error
}

type asReviewerPullRequestsMsg struct {
	prs []provider.PullRequest
	err error
}

// SetPRsMsg is a message to directly set the pull requests (from polling)
type SetPRsMsg struct {
	PRs []provider.PullRequest
}

// fetchPullRequestsMulti fetches pull requests from all projects via the provider.
func fetchPullRequestsMulti(client provider.Provider) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return pullRequestsMsg{prs: nil, err: nil}
		}
		prs, err := client.ListPullRequests(25, provider.ListOpts{})
		return pullRequestsMsg{prs: prs, err: err}
	}
}

// fetchMyPullRequestsMulti fetches pull requests created by the authenticated user.
func fetchMyPullRequestsMulti(client provider.Provider) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return myPullRequestsMsg{prs: nil, err: nil}
		}
		prs, err := client.ListMyPullRequests(25, provider.ListOpts{Mine: true})
		return myPullRequestsMsg{prs: prs, err: err}
	}
}

// fetchPullRequestsAsReviewerMulti fetches pull requests where the authenticated user is a reviewer.
func fetchPullRequestsAsReviewerMulti(client provider.Provider) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return asReviewerPullRequestsMsg{prs: nil, err: nil}
		}
		prs, err := client.ListPullRequestsAsReviewer(25, provider.ListOpts{})
		return asReviewerPullRequestsMsg{prs: prs, err: err}
	}
}
