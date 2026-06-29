package workitems

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Elpulgo/azdo/internal/browser"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openURL is a package-level seam so tests can intercept browser launches.
var openURL = browser.Open

// openURLResultMsg is sent when an attempt to open a URL in the browser completes.
type openURLResultMsg struct {
	err error
}

// stateUpdateResultMsg is sent when a state update completes
type stateUpdateResultMsg struct {
	newState string
	err      error
}

// statesLoadedMsg is sent when work item type states have been fetched
type statesLoadedMsg struct {
	states []provider.WorkItemTypeState
	err    error
}

// commentsLoadedMsg is sent when work item comments have been fetched
type commentsLoadedMsg struct {
	comments []provider.WorkItemComment
	err      error
}

// commentPostedMsg is sent when a new comment has been posted
type commentPostedMsg struct {
	comment *provider.WorkItemComment
	err     error
}

// DetailModel represents the work item detail view
type DetailModel struct {
	client        provider.Provider
	workItem      provider.WorkItem
	width         int
	height        int
	viewport      viewport.Model
	ready         bool
	styles        *styles.Styles
	statePicker   components.StatePicker
	loading       bool
	spinner       *components.LoadingIndicator
	statusMessage string

	comments        []provider.WorkItemComment
	commentsLoading bool
	commentsErr     error
	commentForm     components.CommentForm
	posting         bool   // a comment POST is in flight
	pendingComment  string // draft text retained across an in-flight post
}

// NewDetailModel creates a new work item detail model with default styles
func NewDetailModel(client provider.Provider, wi provider.WorkItem) *DetailModel {
	return NewDetailModelWithStyles(client, wi, styles.DefaultStyles())
}

// NewDetailModelWithStyles creates a new work item detail model with custom styles
func NewDetailModelWithStyles(client provider.Provider, wi provider.WorkItem, s *styles.Styles) *DetailModel {
	spinner := components.NewLoadingIndicator(s)
	return &DetailModel{
		client:      client,
		workItem:    wi,
		styles:      s,
		statePicker: components.NewStatePicker(s),
		spinner:     spinner,
		commentForm: components.NewCommentForm(s),
	}
}

// Init initializes the detail model, kicking off the comment fetch so the
// Discussion section is populated as soon as the detail view opens.
func (m *DetailModel) Init() tea.Cmd {
	m.commentsLoading = true
	if m.ready {
		m.updateViewportContent()
	}
	return m.fetchComments()
}

// Update handles messages for the detail view
func (m *DetailModel) Update(msg tea.Msg) (*DetailModel, tea.Cmd) {
	// Route input to state picker when visible
	if m.statePicker.IsVisible() {
		var cmd tea.Cmd
		m.statePicker, cmd = m.statePicker.Update(msg)
		return m, cmd
	}

	// Route input to the comment form while it is open. The form hides itself
	// synchronously on submit/cancel, so the resulting CommentSubmittedMsg /
	// CommentFormCancelledMsg fall through to the handlers below instead of
	// being re-captured here.
	if m.commentForm.IsVisible() {
		var cmd tea.Cmd
		m.commentForm, cmd = m.commentForm.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {

	case components.CommentSubmittedMsg:
		m.pendingComment = msg.Text
		m.posting = true
		m.spinner.SetVisible(true)
		m.spinner.SetMessage("Posting comment...")
		// The form hid itself on submit; reclaim the viewport space.
		m.resizeViewport()
		return m, tea.Batch(m.postComment(msg.Text), m.spinner.Tick())

	case components.CommentFormCancelledMsg:
		m.pendingComment = ""
		m.resizeViewport()
		return m, nil

	case commentsLoadedMsg:
		m.commentsLoading = false
		m.commentsErr = msg.err
		m.comments = msg.comments
		m.updateViewportContent()
		return m, nil

	case commentPostedMsg:
		m.posting = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error posting comment: %v", msg.err)
			// Restore the draft so the user doesn't lose their text.
			m.commentForm.Reset()
			m.commentForm.SetValue(m.pendingComment)
			m.commentForm.SetWidth(m.width)
			m.commentForm.Show()
			m.resizeViewport()
			return m, m.commentForm.Focus()
		}
		m.pendingComment = ""
		m.statusMessage = "Comment added"
		// Re-fetch so the new comment appears in the correct (newest-first) position.
		m.commentsLoading = true
		m.updateViewportContent()
		return m, m.fetchComments()
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
		m.workItem.State = msg.newState
		m.statusMessage = fmt.Sprintf("State changed to %s", msg.newState)
		m.updateViewportContent()
		// Signal the list to refresh so the new state is visible
		return m, func() tea.Msg { return WorkItemStateChangedMsg{} }

	case openURLResultMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to open browser: %v", msg.err)
		} else {
			m.statusMessage = "Opened in browser"
		}
		return m, nil

	case statesLoadedMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.statePicker.SetStates(msg.states, m.workItem.State)
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
		case "w":
			m.loading = true
			m.spinner.SetVisible(true)
			m.spinner.SetMessage("Loading states...")
			return m, tea.Batch(m.fetchStates(), m.spinner.Tick())
		case "o":
			return m, m.openInBrowser()
		case "c":
			// Don't allow opening a new form while a post is in flight.
			if m.posting {
				return m, nil
			}
			m.commentForm.Reset()
			m.commentForm.SetWidth(m.width)
			m.commentForm.Show()
			m.resizeViewport()
			return m, m.commentForm.Focus()
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

// openInBrowser returns a command that opens the work item URL in the
// user's default browser. If no URL can be built (e.g. the client is nil),
// it sets a status message and returns nil.
func (m *DetailModel) openInBrowser() tea.Cmd {
	if m.client == nil {
		m.statusMessage = "Cannot open: no Azure DevOps client"
		return nil
	}
	wi := m.workItem
	url := m.client.WorkItemURL(wi.Identity.Scope, workItemNumericID(wi))
	if url == "" {
		m.statusMessage = "Cannot open: missing organization or project"
		return nil
	}
	return func() tea.Msg {
		return openURLResultMsg{err: openURL(url)}
	}
}

// fetchStates fetches available states for the work item type
func (m *DetailModel) fetchStates() tea.Cmd {
	client := m.client
	scope := m.workItem.Identity.Scope
	workItemType := m.workItem.WorkItemType
	return func() tea.Msg {
		if client == nil {
			return statesLoadedMsg{err: fmt.Errorf("no client available")}
		}
		states, err := client.GetWorkItemTypeStates(scope, workItemType)
		return statesLoadedMsg{states: states, err: err}
	}
}

// fetchComments fetches the work item's discussion comments (newest first).
func (m *DetailModel) fetchComments() tea.Cmd {
	client := m.client
	wi := m.workItem
	return func() tea.Msg {
		if client == nil {
			return commentsLoadedMsg{err: fmt.Errorf("no client available")}
		}
		comments, err := client.GetWorkItemComments(wi.Identity.Scope, workItemNumericID(wi))
		return commentsLoadedMsg{comments: comments, err: err}
	}
}

// postComment sends a new comment to the API.
func (m *DetailModel) postComment(text string) tea.Cmd {
	client := m.client
	wi := m.workItem
	return func() tea.Msg {
		if client == nil {
			return commentPostedMsg{err: fmt.Errorf("no client available")}
		}
		comment, err := client.AddWorkItemComment(wi.Identity.Scope, workItemNumericID(wi), text)
		return commentPostedMsg{comment: comment, err: err}
	}
}

// updateState sends the state update to the API
func (m *DetailModel) updateState(state string) tea.Cmd {
	client := m.client
	wi := m.workItem
	return func() tea.Msg {
		if client == nil {
			return stateUpdateResultMsg{err: fmt.Errorf("no client available")}
		}
		err := client.UpdateWorkItemState(wi.Identity.Scope, workItemNumericID(wi), state)
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
	id := workItemNumericID(wi)
	sb.WriteString(m.styles.Header.Render(fmt.Sprintf("#%d: %s", id, wi.Title)))
	sb.WriteString("\n")

	// Type, state and priority
	metadataStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.styles.Theme.Secondary))
	prioLabel := fmt.Sprintf("P%d", wi.Priority)
	if wi.Priority == 0 {
		prioLabel = "-"
	}
	sb.WriteString(metadataStyle.Render(fmt.Sprintf("%s  |  %s %s  |  %s", wi.WorkItemType, display.StateGlyph(wi.StateCategory), wi.State, prioLabel)))
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

	// Inline comment form, rendered below the viewport when open
	if m.commentForm.IsVisible() {
		sb.WriteString("\n")
		sb.WriteString(m.commentForm.View())
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
	if wi.AssignedToName != "" {
		sb.WriteString(m.styles.Label.Render("Assigned To: "))
		sb.WriteString(wi.AssignedToName)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(m.styles.Label.Render("Assigned To: "))
		sb.WriteString(m.styles.Muted.Render("Unassigned"))
		sb.WriteString("\n\n")
	}

	// Iteration Path
	if wi.IterationPath != "" {
		sb.WriteString(m.styles.Label.Render("Iteration: "))
		sb.WriteString(shortenIterationPath(wi.IterationPath))
		sb.WriteString("\n\n")
	}

	// Last changed timestamp
	if !wi.ChangedDate.IsZero() {
		sb.WriteString(m.styles.Label.Render("Last changed: "))
		sb.WriteString(wi.ChangedDate.Format("2006-01-02 15:04"))
		sb.WriteString("\n\n")
	}

	// Tags
	if tags := tagList(wi.Tags); len(tags) > 0 {
		sb.WriteString(m.styles.Label.Render("Tags: "))
		sb.WriteString(strings.Join(tags, ", "))
		sb.WriteString("\n\n")
	}

	// Link to work item (shown before description for quick access)
	if m.client != nil {
		url := m.client.WorkItemURL(wi.Identity.Scope, workItemNumericID(wi))
		if url != "" {
			sb.WriteString(hyperlink(m.styles.Link.Render("Open in browser"), url))
			sb.WriteString("\n\n")
		}
	}

	// Description (with HTML stripped)
	// Bugs use ReproSteps field; other types use Description
	effectiveDesc := wiEffectiveDescription(wi)
	if effectiveDesc != "" {
		sb.WriteString(m.styles.Label.Render("Description"))
		sb.WriteString("\n")
		cleanDesc := stripHTMLTags(effectiveDesc)
		descStyle := m.styles.Value.Width(m.width)
		sb.WriteString(descStyle.Render(cleanDesc))
		sb.WriteString("\n")
	} else {
		sb.WriteString(m.styles.Muted.Render("No description"))
		sb.WriteString("\n")
	}

	// Discussion (comments), newest first
	m.writeDiscussion(&sb)

	m.viewport.SetContent(sb.String())
}

// writeDiscussion appends the Discussion section (comments) to the viewport content.
func (m *DetailModel) writeDiscussion(sb *strings.Builder) {
	sb.WriteString("\n")
	sb.WriteString(m.styles.Label.Render(fmt.Sprintf("Discussion (%d)", len(m.comments))))
	sb.WriteString("\n\n")

	switch {
	case m.commentsLoading:
		sb.WriteString(m.styles.Muted.Render("Loading comments..."))
		sb.WriteString("\n")
		return
	case m.commentsErr != nil:
		sb.WriteString(m.styles.Muted.Render(fmt.Sprintf("Could not load comments: %v", m.commentsErr)))
		sb.WriteString("\n")
		return
	case len(m.comments) == 0:
		sb.WriteString(m.styles.Muted.Render("No comments yet. Press c to add one."))
		sb.WriteString("\n")
		return
	}

	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.styles.Theme.Secondary))
	bodyStyle := m.styles.Value.Width(m.width)
	for _, c := range m.comments {
		author := c.AuthorName
		if author == "" {
			author = "Unknown"
		}
		header := author
		if !c.CreatedDate.IsZero() {
			header = fmt.Sprintf("%s  ·  %s", author, c.CreatedDate.Format("2006-01-02 15:04"))
		}
		sb.WriteString(metaStyle.Render(header))
		sb.WriteString("\n")
		sb.WriteString(bodyStyle.Render(stripHTMLTags(c.Text)))
		sb.WriteString("\n\n")
	}
}

// SetSize sets the size of the detail view
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.commentForm.SetWidth(width)

	if !m.ready {
		m.viewport = viewport.New(width, 1)
		m.ready = true
	}

	m.resizeViewport()
	m.updateViewportContent()
}

// reservedLines returns the number of non-viewport rows the detail view renders:
// the fixed header (title + type/state + separator = 3), plus the inline comment
// form (a blank spacer + the form itself) when it is open.
func (m *DetailModel) reservedLines() int {
	lines := 3
	if m.commentForm.IsVisible() {
		lines += 1 + m.commentForm.Height()
	}
	return lines
}

// resizeViewport recomputes the viewport dimensions from the current size and
// reserved lines. Call this whenever the comment form is shown or hidden so the
// scrollable area shrinks/grows to make room for the form.
func (m *DetailModel) resizeViewport() {
	if !m.ready {
		return
	}
	h := m.height - m.reservedLines()
	if h < 1 {
		h = 1
	}
	m.viewport.Width = m.width
	m.viewport.Height = h
}

// GetContextItems returns context items for the detail view
func (m *DetailModel) GetContextItems() []components.ContextItem {
	return []components.ContextItem{
		{Key: "w", Description: "Change state"},
		{Key: "c", Description: "comment"},
		{Key: "o", Description: "open in browser"},
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

// GetWorkItemID returns the numeric ID of the work item.
func (m *DetailModel) GetWorkItemID() int {
	return workItemNumericID(m.workItem)
}

// Helper functions

// workItemNumericID converts the string Identity.ID to an int.
// Returns 0 for unset or non-numeric IDs.
func workItemNumericID(wi provider.WorkItem) int {
	if wi.Identity.ID == "" {
		return 0
	}
	n := 0
	for _, ch := range wi.Identity.ID {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// wiEffectiveDescription returns the appropriate description text for the work item.
// Bugs use ReproSteps when available; all other types use Description.
func wiEffectiveDescription(wi provider.WorkItem) string {
	if wi.WorkItemType == "Bug" && wi.ReproSteps != "" {
		return wi.ReproSteps
	}
	return wi.Description
}

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

