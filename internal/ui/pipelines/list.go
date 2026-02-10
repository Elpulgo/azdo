package pipelines

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// baseStyle is used for consistent styling (no border - table handles its own)
var baseStyle = lipgloss.NewStyle()

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

// Column width ratios (percentages of available width)
const (
	statusWidthPct   = 12 // Status column percentage
	pipelineWidthPct = 30 // Pipeline column percentage
	branchWidthPct   = 28 // Branch column percentage
	buildWidthPct    = 15 // Build column percentage
	durationWidthPct = 15 // Duration column percentage
)

// Minimum column widths
const (
	minStatusWidth   = 10
	minPipelineWidth = 15
	minBranchWidth   = 10
	minBuildWidth    = 8
	minDurationWidth = 8
)

// NewModel creates a new pipeline list model
func NewModel(client *azdevops.Client) Model {
	// Start with minimum widths, will be resized on first WindowSizeMsg
	columns := makeColumns(80)

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

// makeColumns creates table columns sized for the given width
func makeColumns(width int) []table.Column {
	// Account for table structure:
	// - 4 chars for column separators (between 5 columns)
	// - Some padding for cell content
	// Total overhead: ~8 chars
	available := width - 8
	if available < 60 {
		available = 60 // Minimum usable width
	}

	// Calculate widths based on percentages
	statusW := max(minStatusWidth, available*statusWidthPct/100)
	pipelineW := max(minPipelineWidth, available*pipelineWidthPct/100)
	branchW := max(minBranchWidth, available*branchWidthPct/100)
	buildW := max(minBuildWidth, available*buildWidthPct/100)
	durationW := max(minDurationWidth, available*durationWidthPct/100)

	return []table.Column{
		{Title: "Status", Width: statusW},
		{Title: "Pipeline", Width: pipelineW},
		{Title: "Branch", Width: branchW},
		{Title: "Build", Width: buildW},
		{Title: "Duration", Width: durationW},
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
		m.table.SetColumns(makeColumns(msg.Width))

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

	// Define styles
	blueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	orangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	switch {
	case statusLower == "inprogress":
		return blueStyle.Render("● Running")
	case statusLower == "notstarted":
		return blueStyle.Render("○ Queued")
	case statusLower == "canceling":
		return orangeStyle.Render("⊘ Cancel")
	case resultLower == "succeeded":
		return greenStyle.Render("✓ Success")
	case resultLower == "failed":
		return redStyle.Render("✗ Failed")
	case resultLower == "canceled":
		return grayStyle.Render("○ Canceled")
	case resultLower == "partiallysucceeded":
		return yellowStyle.Render("⚠ Partial")
	default:
		// Debug: show what we received
		return grayStyle.Render(fmt.Sprintf("%s/%s", status, result))
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
