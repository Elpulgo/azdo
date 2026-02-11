package pullrequests

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
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
}

// NewDetailModel creates a new PR detail model
func NewDetailModel(client *azdevops.Client, pr azdevops.PullRequest) *DetailModel {
	return &DetailModel{
		client:        client,
		pr:            pr,
		threads:       []azdevops.Thread{},
		selectedIndex: 0,
	}
}

// Init initializes the detail model
func (m *DetailModel) Init() tea.Cmd {
	return m.fetchThreads()
}

// Update handles messages for the detail view
func (m *DetailModel) Update(msg tea.Msg) (*DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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
		case "r":
			m.loading = true
			return m, m.fetchThreads()
		}

	case threadsMsg:
		m.loading = false
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
		return m, m.fetchThreads()
	}

	return m, nil
}

// View renders the detail view
func (m *DetailModel) View() string {
	// Helper to wrap content with proper height
	wrapContent := func(content string) string {
		availableHeight := m.height - 5
		if availableHeight < 1 {
			availableHeight = 10
		}
		contentStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(availableHeight)
		return contentStyle.Render(content)
	}

	if m.err != nil {
		return wrapContent(fmt.Sprintf("Error loading threads: %v\n\nPress r to retry, Esc to go back", m.err))
	}

	if m.loading {
		return wrapContent(fmt.Sprintf("Loading threads for PR #%d...", m.pr.ID))
	}

	var sb strings.Builder

	// Header with PR title
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	sb.WriteString(headerStyle.Render(fmt.Sprintf("PR #%d: %s", m.pr.ID, m.pr.Title)))
	sb.WriteString("\n")

	// Branch info
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	sb.WriteString(branchStyle.Render(fmt.Sprintf("%s → %s", m.pr.SourceBranchShortName(), m.pr.TargetBranchShortName())))
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

	// Fill available height
	availableHeight := m.height - 5 // Account for tab bar and status bar
	if availableHeight < 1 {
		availableHeight = 10
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(availableHeight)

	return contentStyle.Render(sb.String())
}

// renderThread renders a single thread with all its comments
func (m *DetailModel) renderThread(thread azdevops.Thread, selected bool) string {
	var sb strings.Builder

	icon := threadStatusIcon(thread.Status)
	statusStyle := lipgloss.NewStyle().Bold(true)

	// Header line with status icon and file location
	headerParts := []string{icon, statusStyle.Render(thread.StatusDescription())}

	// Add file path and line number for code comments
	if thread.IsCodeComment() {
		fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
		shortPath := shortenFilePath(thread.ThreadContext.FilePath)
		location := shortPath
		if thread.ThreadContext.RightFileStart != nil {
			location = fmt.Sprintf("%s:%d", shortPath, thread.ThreadContext.RightFileStart.Line)
		}
		headerParts = append(headerParts, fileStyle.Render(location))
	}

	// Build header line with selection indicator on this line only
	headerLine := "  " + strings.Join(headerParts, " ")
	if selected {
		selectedStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("57")).
			Foreground(lipgloss.Color("229"))
		headerLine = selectedStyle.Render(headerLine)
	}
	sb.WriteString(headerLine)
	sb.WriteString("\n")

	// Render all comments in the thread
	for i, comment := range thread.Comments {
		indent := "    "
		if comment.ParentCommentID != 0 {
			indent = "      └ " // Reply indicator
		}

		authorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
		contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

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
		authorLen := len(comment.Author.DisplayName) + 2 // +2 for ": "
		indentLen := len(indent)
		availableWidth := m.width - indentLen - authorLen - 4 // -4 for margin
		if availableWidth < 20 {
			availableWidth = 60 // fallback
		}

		// Truncate if longer than available width
		if len(content) > availableWidth {
			content = content[:availableWidth-3] + "..."
		}

		commentLine := fmt.Sprintf("%s%s: %s",
			indent,
			authorStyle.Render(comment.Author.DisplayName),
			contentStyle.Render(content))

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

	// Account for header (4 lines) and footer (2 lines)
	viewportHeight := height - 8
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if !m.ready {
		m.viewport = viewport.New(width, viewportHeight)
		m.viewport.HighPerformanceRendering = false
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
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		sb.WriteString(descStyle.Render(m.pr.Description))
		sb.WriteString("\n\n")
	}

	// Reviewers section
	if len(m.pr.Reviewers) > 0 {
		sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
		sb.WriteString(sectionStyle.Render("Reviewers"))
		sb.WriteString("\n")
		for _, reviewer := range m.pr.Reviewers {
			icon := reviewerVoteIcon(reviewer.Vote)
			sb.WriteString(fmt.Sprintf("  %s %s\n", icon, reviewer.DisplayName))
		}
		sb.WriteString("\n")
	}

	// Threads section
	if len(m.threads) > 0 {
		sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
		sb.WriteString(sectionStyle.Render(fmt.Sprintf("Comments (%d)", len(m.threads))))
		sb.WriteString("\n")

		for i, thread := range m.threads {
			line := m.renderThread(thread, i == m.selectedIndex)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	} else {
		grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		sb.WriteString(grayStyle.Render("No comments"))
		sb.WriteString("\n")
	}

	m.viewport.SetContent(sb.String())
}

// ensureSelectedVisible scrolls the viewport to keep the selected item visible
func (m *DetailModel) ensureSelectedVisible() {
	if !m.ready || len(m.threads) == 0 {
		return
	}

	// Calculate actual line position of selected thread
	selectedLineStart := m.getSelectedThreadLineOffset()
	threadHeight := m.getThreadLineCount(m.threads[m.selectedIndex])

	// Add a small margin so the selected item isn't right at the edge
	const margin = 2

	visibleStart := m.viewport.YOffset
	visibleEnd := visibleStart + m.viewport.Height - 1

	// If selected thread header is above the visible area (with margin), scroll up
	if selectedLineStart < visibleStart+margin {
		newOffset := selectedLineStart - margin
		if newOffset < 0 {
			newOffset = 0
		}
		m.viewport.SetYOffset(newOffset)
	} else if selectedLineStart+threadHeight > visibleEnd-margin {
		// If selected thread extends below the visible area (with margin), scroll down
		// Position so the header is visible with some margin from the bottom
		newOffset := selectedLineStart - m.viewport.Height + threadHeight + margin + 1
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
	if len(m.pr.Reviewers) > 0 {
		lineOffset += 1 + len(m.pr.Reviewers) + 1
	}
	// Comments header line
	lineOffset += 1
	return lineOffset
}

// getThreadLineCount returns the number of lines a thread takes to render
// Each thread has 1 header line + N comment lines
func (m *DetailModel) getThreadLineCount(thread azdevops.Thread) int {
	return 1 + len(thread.Comments)
}

// getSelectedThreadLineOffset returns the line number where the selected thread starts
func (m *DetailModel) getSelectedThreadLineOffset() int {
	offset := m.getThreadsLineOffset()
	for i := 0; i < m.selectedIndex && i < len(m.threads); i++ {
		offset += m.getThreadLineCount(m.threads[i]) + 1 // +1 for the newline after each thread
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
		{Key: "r", Description: "refresh"},
		{Key: "esc", Description: "back"},
	}
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

// reviewerVoteIcon returns an icon for the reviewer's vote
func reviewerVoteIcon(vote int) string {
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	orangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	switch vote {
	case 10:
		return greenStyle.Render("✓")
	case 5:
		return yellowStyle.Render("~")
	case 0:
		return grayStyle.Render("○")
	case -5:
		return orangeStyle.Render("◐")
	case -10:
		return redStyle.Render("✗")
	default:
		return grayStyle.Render("?")
	}
}

// threadStatusIcon returns an icon for the thread status
func threadStatusIcon(status string) string {
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	blueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	orangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	switch status {
	case "active":
		return blueStyle.Render("●")
	case "fixed":
		return greenStyle.Render("✓")
	case "wontFix", "closed":
		return grayStyle.Render("○")
	case "pending":
		return orangeStyle.Render("◐")
	default:
		return grayStyle.Render("○")
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
