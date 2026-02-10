package pipelines

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// timestampRegex matches Azure DevOps log timestamps like "2024-02-06T10:00:00.000Z "
var timestampRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s*`)

// LogViewerModel represents a scrollable log viewer
type LogViewerModel struct {
	client   *azdevops.Client
	buildID  int
	logID    int
	title    string
	content  string
	viewport viewport.Model
	loading  bool
	err      error
	width    int
	height   int
	ready    bool
}

// NewLogViewerModel creates a new log viewer model
func NewLogViewerModel(client *azdevops.Client, buildID, logID int, title string) *LogViewerModel {
	return &LogViewerModel{
		client:  client,
		buildID: buildID,
		logID:   logID,
		title:   title,
		loading: true,
	}
}

// Init initializes the model and fetches log content
func (m *LogViewerModel) Init() tea.Cmd {
	return m.fetchLogContent()
}

// SetSize sets the viewport size
func (m *LogViewerModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Account for header and footer
	viewportHeight := height - 4
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

	// Re-set content if we have it
	if m.content != "" {
		m.viewport.SetContent(m.formatContent())
	}
}

// SetContent sets the log content
func (m *LogViewerModel) SetContent(content string) {
	m.content = content
	m.loading = false
	if m.ready {
		m.viewport.SetContent(m.formatContent())
		m.viewport.GotoTop()
	}
}

// SetError sets an error state
func (m *LogViewerModel) SetError(message string) {
	m.err = errors.New(message)
	m.loading = false
}

// GetContent returns the raw log content
func (m *LogViewerModel) GetContent() string {
	return m.content
}

// GetTitle returns the log title
func (m *LogViewerModel) GetTitle() string {
	return m.title
}

// GetBuildID returns the build ID
func (m *LogViewerModel) GetBuildID() int {
	return m.buildID
}

// GetLogID returns the log ID
func (m *LogViewerModel) GetLogID() int {
	return m.logID
}

// IsLoading returns true if content is loading
func (m *LogViewerModel) IsLoading() bool {
	return m.loading
}

// GetError returns any error
func (m *LogViewerModel) GetError() error {
	return m.err
}

// Update handles messages
func (m *LogViewerModel) Update(msg tea.Msg) (*LogViewerModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.loading = true
			m.err = nil
			return m, m.fetchLogContent()
		case "g":
			m.viewport.GotoTop()
		case "G":
			m.viewport.GotoBottom()
		}

	case logContentMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.SetContent(msg.content)
	}

	// Update viewport for scrolling
	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
	}

	return m, cmd
}

// View renders the log viewer
func (m *LogViewerModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error loading log: %v\n\nPress r to retry, Esc to go back", m.err)
	}

	if m.loading {
		return fmt.Sprintf("Loading log for %s...", m.title)
	}

	if m.content == "" {
		return fmt.Sprintf("Log: %s\n\nNo log content available.\n\nPress Esc to go back", m.title)
	}

	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Log: %s", m.title)))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width-2, 60)))
	sb.WriteString("\n")

	// Viewport content
	if m.ready {
		sb.WriteString(m.viewport.View())
	}

	return sb.String()
}

// formatContent formats the log content for display
func (m *LogViewerModel) formatContent() string {
	if m.content == "" {
		return ""
	}

	lines := formatLogLines(m.content)
	return strings.Join(lines, "\n")
}

// GetContextItems returns context bar items for this view
func (m *LogViewerModel) GetContextItems() []components.ContextItem {
	return []components.ContextItem{
		{Key: "↑↓/pgup/pgdn", Description: "scroll"},
		{Key: "g/G", Description: "top/bottom"},
		{Key: "esc", Description: "back"},
		{Key: "r", Description: "refresh"},
	}
}

// GetScrollPercent returns the current scroll percentage (0-100)
func (m *LogViewerModel) GetScrollPercent() float64 {
	if !m.ready {
		return 0
	}
	return m.viewport.ScrollPercent() * 100
}

// Messages

type logContentMsg struct {
	content string
	err     error
}

func (m *LogViewerModel) fetchLogContent() tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.GetBuildLogContent(m.buildID, m.logID)
		return logContentMsg{content: content, err: err}
	}
}

// Helper functions

// formatLogLines splits content into lines and optionally strips timestamps
func formatLogLines(content string) []string {
	if content == "" {
		return []string{}
	}

	// Split by newlines
	rawLines := strings.Split(content, "\n")

	// Filter out empty trailing lines
	var lines []string
	for _, line := range rawLines {
		// Strip Azure DevOps timestamps for cleaner display
		cleanLine := stripTimestamp(line)
		lines = append(lines, cleanLine)
	}

	// Remove trailing empty line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// stripTimestamp removes Azure DevOps timestamp prefix from a log line
func stripTimestamp(line string) string {
	return timestampRegex.ReplaceAllString(line, "")
}
