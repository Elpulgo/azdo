package pullrequests

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DetailModel represents the PR detail view showing description, reviewers, and threads
type DetailModel struct {
	client        *azdevops.Client
	pr            azdevops.PullRequest
	threads       []azdevops.Thread
	selectedIndex int
	loading       bool
	err           error
	width         int
	height        int
	viewport      viewport.Model
	ready         bool
	statusMessage string
	spinner       *components.LoadingIndicator
	styles        *styles.Styles
}

// NewDetailModel creates a new PR detail model with default styles
func NewDetailModel(client *azdevops.Client, pr azdevops.PullRequest) *DetailModel {
	return NewDetailModelWithStyles(client, pr, styles.DefaultStyles())
}

// NewDetailModelWithStyles creates a new PR detail model with custom styles
func NewDetailModelWithStyles(client *azdevops.Client, pr azdevops.PullRequest, s *styles.Styles) *DetailModel {
	spinner := components.NewLoadingIndicator(s)
	spinner.SetMessage(fmt.Sprintf("Loading threads for PR #%d...", pr.ID))

	return &DetailModel{
		client:        client,
		pr:            pr,
		threads:       []azdevops.Thread{},
		selectedIndex: 0,
		spinner:       spinner,
		styles:        s,
	}
}

// Init initializes the detail model
func (m *DetailModel) Init() tea.Cmd {
	m.loading = true
	m.spinner.SetVisible(true)
	return tea.Batch(m.fetchThreads(), m.spinner.Init())
}

// Update handles messages for the detail view
func (m *DetailModel) Update(msg tea.Msg) (*DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		// Forward spinner tick messages
		if m.loading {
			var spinnerCmd tea.Cmd
			m.spinner, spinnerCmd = m.spinner.Update(msg)
			return m, spinnerCmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.MoveUp()
		case "down", "j":
			m.MoveDown()
		case "pgup":
			m.PageUp()
		case "pgdown":
			m.PageDown()
		case "d":
			return m, func() tea.Msg { return enterDiffViewMsg{} }
		case "r":
			m.loading = true
			m.spinner.SetVisible(true)
			return m, tea.Batch(m.fetchThreads(), m.spinner.Tick())
		}

	case threadsMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.SetThreads(msg.threads)

	case voteResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.statusMessage = msg.message
		// Refresh threads after voting
		m.loading = true
		m.spinner.SetVisible(true)
		return m, tea.Batch(m.fetchThreads(), m.spinner.Tick())
	}

	return m, nil
}

// View renders the detail view
func (m *DetailModel) View() string {
	// Helper to wrap content with consistent width
	wrapContent := func(content string) string {
		contentStyle := lipgloss.NewStyle().
			Width(m.width)
		return contentStyle.Render(content)
	}

	if m.err != nil {
		return wrapContent(fmt.Sprintf("Error loading threads: %v\n\nPress r to retry, Esc to go back", m.err))
	}

	if m.loading {
		return wrapContent(m.spinner.View())
	}

	var sb strings.Builder

	// Header with PR title
	sb.WriteString(m.styles.Header.Render(fmt.Sprintf("PR #%d: %s", m.pr.ID, m.pr.Title)))
	sb.WriteString("\n")

	// Branch info
	sb.WriteString(m.styles.Muted.Render(fmt.Sprintf("%s → %s", m.pr.SourceBranchShortName(), m.pr.TargetBranchShortName())))
	sb.WriteString("\n")
	separatorWidth := min(m.width-2, 60)
	if separatorWidth < 1 {
		separatorWidth = 60
	}
	sb.WriteString(strings.Repeat("─", separatorWidth))
	sb.WriteString("\n")

	// Viewport with scrollable content
	if m.ready {
		sb.WriteString(m.viewport.View())
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width)

	return contentStyle.Render(sb.String())
}

// renderThread renders a single thread with all its comments
func (m *DetailModel) renderThread(thread azdevops.Thread, selected bool) string {
	var sb strings.Builder

	icon := threadStatusIconWithStyles(thread.Status, m.styles)
	statusStyle := lipgloss.NewStyle().Bold(true)

	// Header line with status icon and file location
	headerParts := []string{icon, statusStyle.Render(thread.StatusDescription())}

	// Add file path and line number for code comments
	if thread.IsCodeComment() {
		shortPath := shortenFilePath(thread.ThreadContext.FilePath)
		location := shortPath
		if thread.ThreadContext.RightFileStart != nil {
			location = fmt.Sprintf("%s:%d", shortPath, thread.ThreadContext.RightFileStart.Line)
		}

		// Build hyperlink URL to the thread/comment if we have client info
		var threadURL string
		if m.client != nil {
			threadURL = buildPRThreadURL(
				m.client.GetOrg(),
				m.client.GetProject(),
				m.pr.Repository.ID,
				m.pr.ID,
				thread.ID,
			)
		}

		// Render as hyperlink (falls back to plain text if no URL)
		styledLocation := m.styles.Link.Render(location)
		headerParts = append(headerParts, hyperlink(styledLocation, threadURL))
	}

	// Build header line with selection indicator on this line only
	headerLine := "  " + strings.Join(headerParts, " ")
	if selected {
		headerLine = m.styles.Selected.Render(headerLine)
	}
	sb.WriteString(headerLine)
	sb.WriteString("\n")

	// Render all comments in the thread
	for i, comment := range thread.Comments {
		indent := "    "
		if comment.ParentCommentID != 0 {
			indent = "      └ " // Reply indicator
		}

		// Replace line breaks with spaces for cleaner display
		content := strings.ReplaceAll(comment.Content, "\r\n", " ")
		content = strings.ReplaceAll(content, "\n", " ")
		content = strings.ReplaceAll(content, "\r", " ")
		// Collapse multiple spaces
		for strings.Contains(content, "  ") {
			content = strings.ReplaceAll(content, "  ", " ")
		}
		content = strings.TrimSpace(content)

		// Calculate available width for content (account for indent and author)
		authorLen := utf8.RuneCountInString(comment.Author.DisplayName) + 2 // +2 for ": "
		indentLen := utf8.RuneCountInString(indent)
		availableWidth := m.width - indentLen - authorLen - 4 // -4 for margin
		if availableWidth < 20 {
			availableWidth = 60 // fallback
		}

		// Truncate if longer than available width (use rune count for proper Unicode handling)
		if utf8.RuneCountInString(content) > availableWidth {
			content = truncateString(content, availableWidth-3) + "..."
		}

		commentLine := fmt.Sprintf("%s%s: %s",
			indent,
			m.styles.Header.Render(comment.Author.DisplayName),
			m.styles.Value.Render(content))

		sb.WriteString(commentLine)
		if i < len(thread.Comments)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// SetSize sets the size of the detail view
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Account for header lines rendered in View(): title (1) + branch (1) + separator (1) = 3
	headerLines := 3
	viewportHeight := height - headerLines
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if !m.ready {
		m.viewport = viewport.New(width, viewportHeight)
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = viewportHeight
	}

	// Update viewport content
	m.updateViewportContent()
}

// updateViewportContent builds the content and sets it in the viewport
func (m *DetailModel) updateViewportContent() {
	var sb strings.Builder

	// Description
	if m.pr.Description != "" {
		sb.WriteString(m.styles.Value.Render(m.pr.Description))
		sb.WriteString("\n\n")
	}

	// "Go to PR" link
	if m.client != nil {
		prURL := buildPROverviewURL(
			m.client.GetOrg(),
			m.client.GetProject(),
			m.pr.Repository.ID,
			m.pr.ID,
		)
		if prURL != "" {
			sb.WriteString(hyperlink(m.styles.Link.Render("Go to PR"), prURL))
			sb.WriteString("\n\n")
		}
	}

	// Reviewers section
	if len(m.pr.Reviewers) > 0 {
		sb.WriteString(m.styles.Label.Render("Reviewers"))
		sb.WriteString("\n")
		for _, reviewer := range m.pr.Reviewers {
			icon := reviewerVoteIconWithStyles(reviewer.Vote, m.styles)
			voteDesc := reviewerVoteDescription(reviewer.Vote)
			sb.WriteString(fmt.Sprintf("  %s %s (%s)\n", icon, reviewer.DisplayName, m.styles.Muted.Render(voteDesc)))
		}
		sb.WriteString("\n")
	}

	// Threads section
	if len(m.threads) > 0 {
		sb.WriteString(m.styles.Label.Render(fmt.Sprintf("Comments (%d)", len(m.threads))))
		sb.WriteString("\n")

		for i, thread := range m.threads {
			line := m.renderThread(thread, i == m.selectedIndex)
			sb.WriteString(line)
			sb.WriteString("\n\n") // Extra blank line between threads for spacing
		}
	} else {
		sb.WriteString(m.styles.Muted.Render("No comments"))
		sb.WriteString("\n")
	}

	m.viewport.SetContent(sb.String())
}

// ensureSelectedVisible scrolls the viewport to keep the selected item visible
// This mirrors the pipeline detail view behavior - only scroll when selection
// is actually outside the visible area
func (m *DetailModel) ensureSelectedVisible() {
	if !m.ready || len(m.threads) == 0 {
		return
	}

	// Calculate actual line position of selected thread
	selectedLineStart := m.getSelectedThreadLineOffset()
	threadHeight := m.getThreadLineCount(m.threads[m.selectedIndex])
	selectedLineEnd := selectedLineStart + threadHeight - 1

	visibleStart := m.viewport.YOffset
	visibleEnd := visibleStart + m.viewport.Height - 1

	// Only scroll if selection is actually outside visible area
	if selectedLineStart < visibleStart {
		// Thread header is above visible area - scroll up to show it at top
		m.viewport.SetYOffset(selectedLineStart)
	} else if selectedLineEnd > visibleEnd {
		// Thread end is below visible area - scroll down minimally
		// Position so thread end is at the bottom of viewport
		newOffset := selectedLineEnd - m.viewport.Height + 1
		if newOffset < 0 {
			newOffset = 0
		}
		m.viewport.SetYOffset(newOffset)
	}
}

// SetThreads sets the threads (useful for testing)
// Filters out system-generated threads (e.g., Microsoft.VisualStudio comments)
func (m *DetailModel) SetThreads(threads []azdevops.Thread) {
	m.threads = azdevops.FilterSystemThreads(threads)
	m.selectedIndex = 0
	if m.ready {
		m.updateViewportContent()
	}
}

// MoveUp moves selection up or scrolls viewport if at top
func (m *DetailModel) MoveUp() {
	if !m.ready {
		return
	}
	if m.selectedIndex > 0 {
		m.selectedIndex--
		// Save viewport position before content update
		savedOffset := m.viewport.YOffset
		m.updateViewportContent()
		// Restore position, then adjust if needed
		m.viewport.SetYOffset(savedOffset)
		m.ensureSelectedVisible()
	} else {
		// At first thread, scroll viewport up to show description/reviewers
		m.viewport.LineUp(1)
	}
}

// MoveDown moves selection down or scrolls viewport if at bottom
func (m *DetailModel) MoveDown() {
	if !m.ready {
		return
	}
	if len(m.threads) > 0 && m.selectedIndex < len(m.threads)-1 {
		m.selectedIndex++
		// Save viewport position before content update
		savedOffset := m.viewport.YOffset
		m.updateViewportContent()
		// Restore position, then adjust if needed
		m.viewport.SetYOffset(savedOffset)
		m.ensureSelectedVisible()
	} else {
		// At last thread, scroll viewport down to show more content
		m.viewport.LineDown(1)
	}
}

// PageUp scrolls the viewport up by one page
func (m *DetailModel) PageUp() {
	if !m.ready {
		return
	}
	// Scroll viewport directly
	m.viewport.HalfViewUp()
	// Update thread selection based on what's visible
	m.updateSelectionFromViewport()
}

// PageDown scrolls the viewport down by one page
func (m *DetailModel) PageDown() {
	if !m.ready {
		return
	}
	// Scroll viewport directly
	m.viewport.HalfViewDown()
	// Update thread selection based on what's visible
	m.updateSelectionFromViewport()
}

// updateSelectionFromViewport updates the selected thread based on viewport position
func (m *DetailModel) updateSelectionFromViewport() {
	if len(m.threads) == 0 {
		return
	}

	// Calculate line offset where threads start
	lineOffset := m.getThreadsLineOffset()

	// Find which thread is visible at the top of the viewport (with small margin)
	targetLine := m.viewport.YOffset + 2 // Small margin from top

	// Find which thread contains this line
	currentLine := lineOffset
	for i, thread := range m.threads {
		threadLines := m.getThreadLineCount(thread) + 1 // +1 for newline after thread
		threadEnd := currentLine + threadLines
		if targetLine < threadEnd {
			m.selectedIndex = i
			// Save position before content update
			savedOffset := m.viewport.YOffset
			m.updateViewportContent()
			// Restore position - don't let content update change it
			m.viewport.SetYOffset(savedOffset)
			return
		}
		currentLine = threadEnd
	}

	// If we're past all threads, select the last one
	m.selectedIndex = len(m.threads) - 1
	savedOffset := m.viewport.YOffset
	m.updateViewportContent()
	m.viewport.SetYOffset(savedOffset)
}

// getThreadsLineOffset returns the line number where threads section starts
func (m *DetailModel) getThreadsLineOffset() int {
	lineOffset := 0
	if m.pr.Description != "" {
		lineOffset += strings.Count(m.pr.Description, "\n") + 2
	}
	// "Go to PR" link takes 2 lines (link + empty line)
	if m.client != nil && m.pr.Repository.ID != "" {
		lineOffset += 2
	}
	if len(m.pr.Reviewers) > 0 {
		lineOffset += 1 + len(m.pr.Reviewers) + 1
	}
	// Comments header line
	lineOffset += 1
	return lineOffset
}

// getThreadLineCount returns the number of lines a thread takes to render
// Each thread has: header line + N comment lines + blank line separator
func (m *DetailModel) getThreadLineCount(thread azdevops.Thread) int {
	return 2 + len(thread.Comments) // header + comments + blank line
}

// getSelectedThreadLineOffset returns the line number where the selected thread starts
func (m *DetailModel) getSelectedThreadLineOffset() int {
	offset := m.getThreadsLineOffset()
	for i := 0; i < m.selectedIndex && i < len(m.threads); i++ {
		offset += m.getThreadLineCount(m.threads[i]) // includes blank line separator
	}
	return offset
}

// SelectedIndex returns the current selection index
func (m *DetailModel) SelectedIndex() int {
	return m.selectedIndex
}

// SelectedThread returns the currently selected thread
func (m *DetailModel) SelectedThread() *azdevops.Thread {
	if len(m.threads) == 0 || m.selectedIndex >= len(m.threads) {
		return nil
	}
	return &m.threads[m.selectedIndex]
}

// GetContextItems returns context items for the detail view
func (m *DetailModel) GetContextItems() []components.ContextItem {
	return []components.ContextItem{
		{Key: "↑↓", Description: "navigate"},
		{Key: "d", Description: "diff"},
		{Key: "r", Description: "refresh"},
		{Key: "esc", Description: "back"},
	}
}

// GetThreads returns the current threads (for passing to DiffModel)
func (m *DetailModel) GetThreads() []azdevops.Thread {
	return m.threads
}

// GetScrollPercent returns the scroll percentage based on viewport position
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

// GetPR returns the pull request
func (m *DetailModel) GetPR() azdevops.PullRequest {
	return m.pr
}

// Helper functions

// hyperlink creates an OSC 8 terminal hyperlink
// Format: \x1b]8;;URL\x07TEXT\x1b]8;;\x07
// Falls back to just text if URL is empty
func hyperlink(text, url string) string {
	if url == "" {
		return text
	}
	return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, text)
}

// buildPRThreadURL constructs the Azure DevOps URL to view a specific comment thread in a PR
func buildPRThreadURL(org, project, repoID string, prID int, threadID int) string {
	if org == "" || project == "" || repoID == "" || threadID == 0 {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d?discussionId=%d",
		org, project, repoID, prID, threadID)
}

// buildPROverviewURL constructs the Azure DevOps URL to view the PR overview page
func buildPROverviewURL(org, project, repoID string, prID int) string {
	if org == "" || project == "" || repoID == "" {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d",
		org, project, repoID, prID)
}

// truncateString truncates a string to maxRunes runes (not bytes)
func truncateString(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// shortenFilePath shortens a file path to show only the last 2 segments
// e.g., /Services/UnitService/Extensions/UnitService.cs -> ../Extensions/UnitService.cs
func shortenFilePath(path string) string {
	if path == "" {
		return ""
	}

	// Split by forward slash (Azure DevOps paths use forward slashes)
	parts := strings.Split(path, "/")

	// Remove empty parts (from leading slash)
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}

	if len(nonEmpty) == 0 {
		return path
	}

	if len(nonEmpty) == 1 {
		// Just filename, return as-is
		return nonEmpty[0]
	}

	// Return last 2 parts with ../ prefix
	if len(nonEmpty) >= 2 {
		return "../" + nonEmpty[len(nonEmpty)-2] + "/" + nonEmpty[len(nonEmpty)-1]
	}

	return path
}

// reviewerVoteIconWithStyles returns an icon for the reviewer's vote using provided styles
func reviewerVoteIconWithStyles(vote int, s *styles.Styles) string {
	switch vote {
	case 10:
		return s.Success.Render("✓")
	case 5:
		return s.Warning.Render("~")
	case 0:
		return s.Muted.Render("○")
	case -5:
		return s.Warning.Render("◐")
	case -10:
		return s.Error.Render("✗")
	default:
		return s.Muted.Render("?")
	}
}

// reviewerVoteDescription returns a human-readable description of the vote
func reviewerVoteDescription(vote int) string {
	switch vote {
	case 10:
		return "Approved"
	case 5:
		return "Approved with suggestions"
	case 0:
		return "No vote"
	case -5:
		return "Waiting for author"
	case -10:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// threadStatusIconWithStyles returns an icon for the thread status using provided styles
func threadStatusIconWithStyles(status string, s *styles.Styles) string {
	switch status {
	case "active":
		return s.Info.Render("●")
	case "fixed":
		return s.Success.Render("✓")
	case "wontFix", "closed":
		return s.Muted.Render("○")
	case "pending":
		return s.Warning.Render("◐")
	default:
		return s.Muted.Render("○")
	}
}

// Messages

type threadsMsg struct {
	threads []azdevops.Thread
	err     error
}

type voteResultMsg struct {
	message string
	err     error
}

// fetchThreads fetches PR threads from Azure DevOps
func (m *DetailModel) fetchThreads() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return threadsMsg{threads: nil, err: nil}
		}
		threads, err := m.client.GetPRThreads(m.pr.Repository.ID, m.pr.ID)
		return threadsMsg{threads: threads, err: err}
	}
}

// votePR submits a vote on the PR
func (m *DetailModel) votePR(vote int) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return voteResultMsg{message: "", err: nil}
		}
		err := m.client.VotePullRequest(m.pr.Repository.ID, m.pr.ID, vote)
		if err != nil {
			return voteResultMsg{message: "", err: err}
		}

		var message string
		switch vote {
		case azdevops.VoteApprove:
			message = "PR approved"
		case azdevops.VoteReject:
			message = "PR rejected"
		default:
			message = "Vote submitted"
		}
		return voteResultMsg{message: message, err: nil}
	}
}
