package pipelines

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

// Model represents the pipeline list view
type Model struct {
	table   table.Model
	client  *azdevops.Client
	runs    []azdevops.PipelineRun
	loading bool
	err     error
	width   int
	height  int
}

// NewModel creates a new pipeline list model
func NewModel(client *azdevops.Client) Model {
	columns := []table.Column{
		{Title: "Status", Width: 8},
		{Title: "Pipeline", Width: 30},
		{Title: "Branch", Width: 25},
		{Title: "Build", Width: 15},
		{Title: "Duration", Width: 10},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return Model{
		table:  t,
		client: client,
		runs:   []azdevops.PipelineRun{},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return fetchPipelineRuns(m.client)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetHeight(msg.Height - 5)

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			// Manual refresh
			m.loading = true
			return m, fetchPipelineRuns(m.client)
		}

	case pipelineRunsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.runs = msg.runs
		m.table.SetRows(m.runsToRows())
		return m, nil
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the view
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error loading pipeline runs: %v\n\nPress r to retry, q to quit", m.err)
	}

	if m.loading {
		return "Loading pipeline runs...\n\nPress q to quit"
	}

	if len(m.runs) == 0 {
		return "No pipeline runs found.\n\nPress r to refresh, q to quit"
	}

	help := "\n  r: refresh • ↑↓: navigate • q: quit"
	return baseStyle.Render(m.table.View()) + help
}

// runsToRows converts pipeline runs to table rows
func (m Model) runsToRows() []table.Row {
	rows := make([]table.Row, len(m.runs))
	for i, run := range m.runs {
		rows[i] = table.Row{
			statusIcon(run.Status, run.Result),
			run.Definition.Name,
			run.BranchShortName(),
			run.BuildNumber,
			run.Duration(),
		}
	}
	return rows
}

// statusIcon returns a colored status icon for the pipeline run
func statusIcon(status, result string) string {
	// Use case-insensitive comparison for status values
	statusLower := strings.ToLower(status)
	resultLower := strings.ToLower(result)

	switch {
	case statusLower == "inprogress":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("⟳ Running")
	case statusLower == "notstarted":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("◷ Queued")
	case statusLower == "canceling":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("⊘ Canceling")
	case resultLower == "succeeded":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓ Success")
	case resultLower == "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗ Failed")
	case resultLower == "canceled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("○ Canceled")
	case resultLower == "partiallysucceeded":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("⚠ Partial")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("○ Unknown")
	}
}

// Messages

type pipelineRunsMsg struct {
	runs []azdevops.PipelineRun
	err  error
}

// fetchPipelineRuns fetches pipeline runs from Azure DevOps
func fetchPipelineRuns(client *azdevops.Client) tea.Cmd {
	return func() tea.Msg {
		runs, err := client.ListPipelineRuns(25)
		return pipelineRunsMsg{runs: runs, err: err}
	}
}
