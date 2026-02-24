package pullrequests

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/diff"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DetailModel represents the PR detail view showing description, reviewers, and changed files
type DetailModel struct {
	client        *azdevops.Client
	pr            azdevops.PullRequest
	threads       []azdevops.Thread
	changedFiles  []azdevops.IterationChange
	commentCounts map[string]int // filePath -> comment count
	fileIndex     int
	loading       bool
	threadsLoaded bool
	filesLoaded   bool
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
	spinner.SetMessage(fmt.Sprintf("Loading PR #%d...", pr.ID))

	return &DetailModel{
		client:        client,
		pr:            pr,
		threads:       []azdevops.Thread{},
		commentCounts: make(map[string]int),
		fileIndex:     0,
		spinner:       spinner,
		styles:        s,
	}
}

// Init initializes the detail model
func (m *DetailModel) Init() tea.Cmd {
	m.loading = true
	m.threadsLoaded = false
	m.filesLoaded = false
	m.spinner.SetVisible(true)
	return tea.Batch(m.fetchThreads(), m.fetchChangedFiles(), m.spinner.Init())
}

// Update handles messages for the detail view
func (m *DetailModel) Update(msg tea.Msg) (*DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
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
		case "enter":
			if len(m.changedFiles) > 0 && m.fileIndex < len(m.changedFiles) {
				return m, func() tea.Msg {
					return openFileDiffMsg{
						file: m.changedFiles[m.fileIndex],
					}
				}
			}
		case "r":
			m.loading = true
			m.threadsLoaded = false
			m.filesLoaded = false
			m.spinner.SetVisible(true)
			return m, tea.Batch(m.fetchThreads(), m.fetchChangedFiles(), m.spinner.Tick())
		}

	case threadsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.threads = azdevops.FilterSystemThreads(msg.threads)
		m.threadsLoaded = true
		m.finishLoading()

	case changedFilesMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.changedFiles = filterFileChanges(msg.changes)
		m.fileIndex = 0
		m.filesLoaded = true
		m.finishLoading()

	case voteResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.statusMessage = msg.message
		m.loading = true
		m.spinner.SetVisible(true)
		return m, tea.Batch(m.fetchThreads(), m.spinner.Tick())
	}

	return m, nil
}

// finishLoading clears the loading state when both threads and files have arrived
func (m *DetailModel) finishLoading() {
	if !m.threadsLoaded || !m.filesLoaded {
		return
	}
	m.loading = false
	m.spinner.SetVisible(false)
	m.commentCounts = diff.CountCommentsPerFile(m.threads)
	if m.ready {
		m.updateViewportContent()
	}
}

// View renders the detail view
func (m *DetailModel) View() string {
	wrapContent := func(content string) string {
		contentStyle := lipgloss.NewStyle().
			Width(m.width)
		return contentStyle.Render(content)
	}

	if m.err != nil {
		return wrapContent(fmt.Sprintf("Error: %v\n\nPress r to retry, Esc to go back", m.err))
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

	// Changed files section
	sb.WriteString(m.styles.Label.Render(fmt.Sprintf("Changed files (%d)", len(m.changedFiles))))
	sb.WriteString("\n")

	if len(m.changedFiles) > 0 {
		for i, change := range m.changedFiles {
			line := m.renderFileEntry(change, i == m.fileIndex)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString(m.styles.Muted.Render("  No changed files"))
		sb.WriteString("\n")
	}

	m.viewport.SetContent(sb.String())
}

// renderFileEntry renders a single file in the changed files list
func (m *DetailModel) renderFileEntry(change azdevops.IterationChange, selected bool) string {
	icon, style := changeTypeDisplay(change.ChangeType, m.styles)

	path := change.Item.Path
	if change.ChangeType == "rename" && change.OriginalPath != "" {
		path = fmt.Sprintf("%s -> %s", change.OriginalPath, change.Item.Path)
	}

	line := fmt.Sprintf("  %s %s", icon, path)

	// Add comment count if there are comments for this file
	count := m.commentCounts[change.Item.Path]
	if count > 0 {
		line += " " + m.styles.Info.Render(fmt.Sprintf("(%d)", count))
	}

	if selected {
		return m.styles.Selected.Render(line)
	}
	return style.Render(line)
}

// changeTypeDisplay returns an icon and style for a change type
func changeTypeDisplay(changeType string, s *styles.Styles) (string, lipgloss.Style) {
	switch changeType {
	case "add":
		return "+", s.Success
	case "edit":
		return "~", s.Info
	case "delete":
		return "-", s.Error
	case "rename":
		return "→", s.Warning
	default:
		return "?", s.Muted
	}
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

	m.updateViewportContent()
}

// ensureSelectedVisible scrolls the viewport to keep the selected file visible
func (m *DetailModel) ensureSelectedVisible() {
	if !m.ready || len(m.changedFiles) == 0 {
		return
	}

	selectedLine := m.getFileListLineOffset() + m.fileIndex
	if selectedLine < m.viewport.YOffset {
		m.viewport.SetYOffset(selectedLine)
	} else if selectedLine >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(selectedLine - m.viewport.Height + 1)
	}
}

// SetThreads sets the threads (useful for testing)
// Filters out system-generated threads
func (m *DetailModel) SetThreads(threads []azdevops.Thread) {
	m.threads = azdevops.FilterSystemThreads(threads)
	m.threadsLoaded = true
	m.commentCounts = diff.CountCommentsPerFile(m.threads)
	if m.ready {
		m.updateViewportContent()
	}
}

// SetChangedFiles sets the changed files (useful for testing)
func (m *DetailModel) SetChangedFiles(files []azdevops.IterationChange) {
	m.changedFiles = filterFileChanges(files)
	m.fileIndex = 0
	m.filesLoaded = true
	if m.ready {
		m.updateViewportContent()
	}
}

// MoveUp moves file selection up or scrolls viewport if at top
func (m *DetailModel) MoveUp() {
	if !m.ready {
		return
	}
	if m.fileIndex > 0 {
		m.fileIndex--
		savedOffset := m.viewport.YOffset
		m.updateViewportContent()
		m.viewport.SetYOffset(savedOffset)
		m.ensureSelectedVisible()
	} else {
		m.viewport.LineUp(1)
	}
}

// MoveDown moves file selection down or scrolls viewport if at bottom
func (m *DetailModel) MoveDown() {
	if !m.ready {
		return
	}
	if len(m.changedFiles) > 0 && m.fileIndex < len(m.changedFiles)-1 {
		m.fileIndex++
		savedOffset := m.viewport.YOffset
		m.updateViewportContent()
		m.viewport.SetYOffset(savedOffset)
		m.ensureSelectedVisible()
	} else {
		m.viewport.LineDown(1)
	}
}

// PageUp scrolls the viewport up by one page
func (m *DetailModel) PageUp() {
	if !m.ready {
		return
	}
	m.viewport.HalfViewUp()
	m.updateSelectionFromViewport()
}

// PageDown scrolls the viewport down by one page
func (m *DetailModel) PageDown() {
	if !m.ready {
		return
	}
	m.viewport.HalfViewDown()
	m.updateSelectionFromViewport()
}

// updateSelectionFromViewport updates the selected file based on viewport position
func (m *DetailModel) updateSelectionFromViewport() {
	if len(m.changedFiles) == 0 {
		return
	}

	fileListStart := m.getFileListLineOffset()
	targetLine := m.viewport.YOffset + 2 // small margin from top

	if targetLine < fileListStart {
		m.fileIndex = 0
	} else {
		idx := targetLine - fileListStart
		if idx >= len(m.changedFiles) {
			idx = len(m.changedFiles) - 1
		}
		m.fileIndex = idx
	}

	savedOffset := m.viewport.YOffset
	m.updateViewportContent()
	m.viewport.SetYOffset(savedOffset)
}

// getFileListLineOffset returns the line number where the file list items start
// (after the "Changed files (N)" header)
func (m *DetailModel) getFileListLineOffset() int {
	lineOffset := 0
	if m.pr.Description != "" {
		lineOffset += strings.Count(m.pr.Description, "\n") + 2
	}
	if m.client != nil && m.pr.Repository.ID != "" {
		lineOffset += 2
	}
	if len(m.pr.Reviewers) > 0 {
		lineOffset += 1 + len(m.pr.Reviewers) + 1
	}
	// "Changed files (N)" header line
	lineOffset += 1
	return lineOffset
}

// SelectedIndex returns the current file selection index
func (m *DetailModel) SelectedIndex() int {
	return m.fileIndex
}

// SelectedFile returns the currently selected changed file
func (m *DetailModel) SelectedFile() *azdevops.IterationChange {
	if len(m.changedFiles) == 0 || m.fileIndex >= len(m.changedFiles) {
		return nil
	}
	return &m.changedFiles[m.fileIndex]
}

// GetContextItems returns context items for the detail view
func (m *DetailModel) GetContextItems() []components.ContextItem {
	return []components.ContextItem{
		{Key: "↑↓", Description: "navigate"},
		{Key: "enter", Description: "open diff"},
		{Key: "r", Description: "refresh"},
		{Key: "esc", Description: "back"},
	}
}

// GetThreads returns the current threads (for passing to DiffModel)
func (m *DetailModel) GetThreads() []azdevops.Thread {
	return m.threads
}

// GetChangedFiles returns the changed files
func (m *DetailModel) GetChangedFiles() []azdevops.IterationChange {
	return m.changedFiles
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
func shortenFilePath(path string) string {
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
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
		return nonEmpty[0]
	}

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

// openFileDiffMsg signals that the user wants to open the diff for a specific file
type openFileDiffMsg struct {
	file azdevops.IterationChange
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

// fetchChangedFiles loads iterations and changed files
func (m *DetailModel) fetchChangedFiles() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return changedFilesMsg{changes: nil, err: nil}
		}

		iterations, err := m.client.GetPRIterations(m.pr.Repository.ID, m.pr.ID)
		if err != nil {
			return changedFilesMsg{err: err}
		}
		if len(iterations) == 0 {
			return changedFilesMsg{changes: nil, err: nil}
		}

		latestID := iterations[len(iterations)-1].ID
		changes, err := m.client.GetPRIterationChanges(m.pr.Repository.ID, m.pr.ID, latestID)
		if err != nil {
			return changedFilesMsg{err: err}
		}

		return changedFilesMsg{changes: changes}
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
