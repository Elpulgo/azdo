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
	if m.err != nil {
		return fmt.Sprintf("Error loading threads: %v\n\nPress r to retry, Esc to go back", m.err)
	}

	if m.loading {
		return fmt.Sprintf("Loading threads for PR #%d...", m.pr.ID)
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
	sb.WriteString(strings.Repeat("─", min(m.width-2, 60)))
	sb.WriteString("\n\n")

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

	return sb.String()
}

// renderThread renders a single thread
func (m *DetailModel) renderThread(thread azdevops.Thread, selected bool) string {
	icon := threadStatusIcon(thread.Status)

	// Get first comment content (summary)
	content := ""
	author := ""
	if len(thread.Comments) > 0 {
		content = thread.Comments[0].Content
		author = thread.Comments[0].Author.DisplayName
		// Truncate long content
		if len(content) > 50 {
			content = content[:47] + "..."
		}
	}

	// Format: [icon] [status] - [author]: [content]
	line := fmt.Sprintf("  %s %s: %s", icon, author, content)

	// Add file path if it's a code comment
	if thread.IsCodeComment() {
		fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		line = fmt.Sprintf("%s %s", line, fileStyle.Render(fmt.Sprintf("(%s)", thread.ThreadContext.FilePath)))
	}

	if selected {
		selectedStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("57")).
			Foreground(lipgloss.Color("229"))
		return selectedStyle.Render(line)
	}

	return line
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
}

// SetThreads sets the threads (useful for testing)
func (m *DetailModel) SetThreads(threads []azdevops.Thread) {
	m.threads = threads
	m.selectedIndex = 0
}

// MoveUp moves selection up
func (m *DetailModel) MoveUp() {
	if m.selectedIndex > 0 {
		m.selectedIndex--
	}
}

// MoveDown moves selection down
func (m *DetailModel) MoveDown() {
	if len(m.threads) > 0 && m.selectedIndex < len(m.threads)-1 {
		m.selectedIndex++
	}
}

// PageUp moves selection up by one page
func (m *DetailModel) PageUp() {
	pageSize := 5
	m.selectedIndex -= pageSize
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

// PageDown moves selection down by one page
func (m *DetailModel) PageDown() {
	pageSize := 5
	m.selectedIndex += pageSize
	if len(m.threads) > 0 && m.selectedIndex >= len(m.threads) {
		m.selectedIndex = len(m.threads) - 1
	}
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

// GetScrollPercent returns the scroll percentage
func (m *DetailModel) GetScrollPercent() float64 {
	if len(m.threads) <= 1 {
		return 0
	}
	return float64(m.selectedIndex) / float64(len(m.threads)-1) * 100
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
