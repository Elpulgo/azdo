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
		case "a":
			// Approve PR
			return m, m.votePR(azdevops.VoteApprove)
		case "x":
			// Reject PR
			return m, m.votePR(azdevops.VoteReject)
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
		location := thread.ThreadContext.FilePath
		if thread.ThreadContext.RightFileStart != nil {
			location = fmt.Sprintf("%s:%d", thread.ThreadContext.FilePath, thread.ThreadContext.RightFileStart.Line)
		}
		headerParts = append(headerParts, fileStyle.Render(location))
	}

	sb.WriteString("  " + strings.Join(headerParts, " "))
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

	result := sb.String()

	if selected {
		selectedStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("57")).
			Foreground(lipgloss.Color("229"))
		return selectedStyle.Render(result)
	}

	return result
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

	lineOffset := m.getThreadsLineOffset()
	selectedLine := lineOffset + m.selectedIndex

	visibleStart := m.viewport.YOffset
	visibleEnd := visibleStart + m.viewport.Height - 1

	if selectedLine < visibleStart {
		m.viewport.SetYOffset(selectedLine)
	} else if selectedLine > visibleEnd {
		m.viewport.SetYOffset(selectedLine - m.viewport.Height + 1)
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
		m.updateViewportContent()
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
		m.updateViewportContent()
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

	// Find which thread is most visible in the center of the viewport
	viewportCenter := m.viewport.YOffset + m.viewport.Height/2

	// Calculate which thread index corresponds to viewport center
	threadLine := viewportCenter - lineOffset
	if threadLine < 0 {
		m.selectedIndex = 0
	} else if threadLine >= len(m.threads) {
		m.selectedIndex = len(m.threads) - 1
	} else {
		m.selectedIndex = threadLine
	}

	m.updateViewportContent()
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
		{Key: "a", Description: "approve"},
		{Key: "x", Description: "reject"},
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
