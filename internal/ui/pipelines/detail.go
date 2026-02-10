package pipelines

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TimelineNode represents a node in the timeline tree with its children
type TimelineNode struct {
	Record   azdevops.TimelineRecord
	Children []*TimelineNode
	Depth    int
}

// DetailModel represents the pipeline detail view showing timeline
type DetailModel struct {
	client        *azdevops.Client
	run           azdevops.PipelineRun
	timeline      *azdevops.Timeline
	tree          []*TimelineNode
	flatItems     []*TimelineNode
	selectedIndex int
	loading       bool
	err           error
	width         int
	height        int
	viewport      viewport.Model
	ready         bool
}

// NewDetailModel creates a new detail model for a pipeline run
func NewDetailModel(client *azdevops.Client, run azdevops.PipelineRun) *DetailModel {
	return &DetailModel{
		client:        client,
		run:           run,
		selectedIndex: 0,
	}
}

// Init initializes the model and fetches timeline
func (m *DetailModel) Init() tea.Cmd {
	return m.fetchTimeline()
}

// SetTimeline sets the timeline data (useful for testing)
func (m *DetailModel) SetTimeline(timeline *azdevops.Timeline) {
	m.timeline = timeline
	m.tree = buildTimelineTree(timeline)
	m.flatItems = flattenTree(m.tree)
	m.selectedIndex = 0
	if m.ready {
		m.updateViewportContent()
	}
}

// updateViewportContent updates the viewport content based on current items and selection
func (m *DetailModel) updateViewportContent() {
	if len(m.flatItems) == 0 {
		return
	}

	var sb strings.Builder
	for i, node := range m.flatItems {
		line := m.renderRecord(node, i == m.selectedIndex)
		sb.WriteString(line)
		if i < len(m.flatItems)-1 {
			sb.WriteString("\n")
		}
	}
	m.viewport.SetContent(sb.String())
}

// SetSize sets the view dimensions
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Account for header (2 lines) and footer (2 lines)
	viewportHeight := height - 6
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

	// Update viewport content if we have items
	if len(m.flatItems) > 0 {
		m.updateViewportContent()
	}
}

// Update handles messages
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
			return m, m.fetchTimeline()
		}

	case timelineMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.SetTimeline(msg.timeline)
	}

	return m, nil
}

// View renders the detail view
func (m *DetailModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error loading timeline: %v\n\nPress r to retry, Esc to go back", m.err)
	}

	if m.loading {
		return fmt.Sprintf("Loading timeline for %s #%s...", m.run.Definition.Name, m.run.BuildNumber)
	}

	if m.timeline == nil || len(m.flatItems) == 0 {
		return "No timeline data available.\n\nPress r to refresh, Esc to go back"
	}

	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	sb.WriteString(headerStyle.Render(fmt.Sprintf("%s #%s", m.run.Definition.Name, m.run.BuildNumber)))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(m.width-2, 60)))
	sb.WriteString("\n")

	// Viewport with timeline records
	if m.ready {
		sb.WriteString(m.viewport.View())
	}

	return sb.String()
}

// renderRecord renders a single timeline record
func (m *DetailModel) renderRecord(node *TimelineNode, selected bool) string {
	indent := indentForType(node.Record.Type)
	icon := recordIcon(node.Record.State, node.Record.Result)
	duration := formatRecordDuration(node.Record.StartTime, node.Record.FinishTime)

	// Format: [indent][icon] [name] [duration]
	line := fmt.Sprintf("%s%s %s", indent, icon, node.Record.Name)

	if duration != "-" {
		line = fmt.Sprintf("%s (%s)", line, duration)
	}

	// Add log indicator if available
	if node.Record.Log != nil {
		line = fmt.Sprintf("%s 📄", line)
	}

	if selected {
		selectedStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("57")).
			Foreground(lipgloss.Color("229"))
		return selectedStyle.Render(line)
	}

	return line
}

// SelectedIndex returns the current selection index
func (m *DetailModel) SelectedIndex() int {
	return m.selectedIndex
}

// SelectedItem returns the currently selected timeline node
func (m *DetailModel) SelectedItem() *TimelineNode {
	if len(m.flatItems) == 0 || m.selectedIndex >= len(m.flatItems) {
		return nil
	}
	return m.flatItems[m.selectedIndex]
}

// MoveUp moves selection up
func (m *DetailModel) MoveUp() {
	if m.selectedIndex > 0 {
		m.selectedIndex--
		m.updateViewportContent()
		m.ensureSelectedVisible()
	}
}

// MoveDown moves selection down
func (m *DetailModel) MoveDown() {
	if m.selectedIndex < len(m.flatItems)-1 {
		m.selectedIndex++
		m.updateViewportContent()
		m.ensureSelectedVisible()
	}
}

// PageUp moves selection up by one page (viewport height)
func (m *DetailModel) PageUp() {
	if !m.ready || len(m.flatItems) == 0 {
		return
	}
	pageSize := m.viewport.Height
	if pageSize < 1 {
		pageSize = 1
	}
	m.selectedIndex -= pageSize
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
	m.updateViewportContent()
	m.ensureSelectedVisible()
}

// PageDown moves selection down by one page (viewport height)
func (m *DetailModel) PageDown() {
	if !m.ready || len(m.flatItems) == 0 {
		return
	}
	pageSize := m.viewport.Height
	if pageSize < 1 {
		pageSize = 1
	}
	m.selectedIndex += pageSize
	if m.selectedIndex >= len(m.flatItems) {
		m.selectedIndex = len(m.flatItems) - 1
	}
	m.updateViewportContent()
	m.ensureSelectedVisible()
}

// ensureSelectedVisible scrolls the viewport to keep the selected item visible
func (m *DetailModel) ensureSelectedVisible() {
	if !m.ready || len(m.flatItems) == 0 {
		return
	}

	// Each item is one line, so line number = selectedIndex
	visibleStart := m.viewport.YOffset
	visibleEnd := visibleStart + m.viewport.Height - 1

	if m.selectedIndex < visibleStart {
		m.viewport.SetYOffset(m.selectedIndex)
	} else if m.selectedIndex > visibleEnd {
		m.viewport.SetYOffset(m.selectedIndex - m.viewport.Height + 1)
	}
}

// CanViewLogs returns true if the selected item has logs that can be viewed
func (m *DetailModel) CanViewLogs() bool {
	selected := m.SelectedItem()
	return selected != nil && selected.Record.Log != nil
}

// GetStatusMessage returns a status message based on the selected item
func (m *DetailModel) GetStatusMessage() string {
	selected := m.SelectedItem()
	if selected == nil {
		return ""
	}

	if selected.Record.Log == nil {
		return fmt.Sprintf("%s has no logs", selected.Record.Type)
	}
	return ""
}

// GetRun returns the pipeline run
func (m *DetailModel) GetRun() azdevops.PipelineRun {
	return m.run
}

// GetContextItems returns context bar items for this view
func (m *DetailModel) GetContextItems() []components.ContextItem {
	return []components.ContextItem{
		{Key: "↑↓/pgup/pgdn", Description: "navigate"},
		{Key: "enter", Description: "view logs"},
	}
}

// GetScrollPercent returns the current scroll percentage (0-100)
// Based on selection position relative to total items
func (m *DetailModel) GetScrollPercent() float64 {
	if !m.ready || len(m.flatItems) <= 1 {
		return 0
	}
	return float64(m.selectedIndex) / float64(len(m.flatItems)-1) * 100
}

// Messages

type timelineMsg struct {
	timeline *azdevops.Timeline
	err      error
}

func (m *DetailModel) fetchTimeline() tea.Cmd {
	return func() tea.Msg {
		timeline, err := m.client.GetBuildTimeline(m.run.ID)
		return timelineMsg{timeline: timeline, err: err}
	}
}

// Helper functions

// recordIcon returns an icon based on state and result
func recordIcon(state, result string) string {
	stateLower := strings.ToLower(state)
	resultLower := strings.ToLower(result)

	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	blueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	yellowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	switch {
	case stateLower == "inprogress":
		return blueStyle.Render("●")
	case stateLower == "pending":
		return grayStyle.Render("○")
	case resultLower == "succeeded":
		return greenStyle.Render("✓")
	case resultLower == "succeededwithissues":
		return yellowStyle.Render("⚠")
	case resultLower == "failed":
		return redStyle.Render("✗")
	case resultLower == "canceled", resultLower == "skipped", resultLower == "abandoned":
		return grayStyle.Render("○")
	default:
		return grayStyle.Render("○")
	}
}

// indentForType returns the indentation string for a record type
func indentForType(recordType string) string {
	switch recordType {
	case "Stage":
		return ""
	case "Job", "Phase":
		return "  "
	case "Task":
		return "    "
	default:
		return "    "
	}
}

// formatRecordDuration formats the duration of a timeline record
func formatRecordDuration(startTime, finishTime *time.Time) string {
	if startTime == nil {
		return "-"
	}
	if finishTime == nil {
		return "-"
	}

	duration := finishTime.Sub(*startTime)
	return formatDuration(duration)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%dm%ds", hours, mins, secs)
}

// buildTimelineTree builds a tree structure from flat timeline records
func buildTimelineTree(timeline *azdevops.Timeline) []*TimelineNode {
	if timeline == nil || len(timeline.Records) == 0 {
		return nil
	}

	// Create a map of all nodes by ID
	nodeMap := make(map[string]*TimelineNode)
	for i := range timeline.Records {
		record := timeline.Records[i]
		nodeMap[record.ID] = &TimelineNode{
			Record:   record,
			Children: []*TimelineNode{},
		}
	}

	// Build the tree by linking parents and children
	var roots []*TimelineNode
	for _, node := range nodeMap {
		if node.Record.ParentID == nil {
			roots = append(roots, node)
		} else {
			parentNode, ok := nodeMap[*node.Record.ParentID]
			if ok {
				parentNode.Children = append(parentNode.Children, node)
			} else {
				// Orphan node, treat as root
				roots = append(roots, node)
			}
		}
	}

	// Sort roots and children by Order
	sortNodes(roots)
	for _, root := range roots {
		sortNodesRecursive(root)
	}

	// Set depth for all nodes
	setDepth(roots, 0)

	return roots
}

// sortNodes sorts a slice of nodes by Order
func sortNodes(nodes []*TimelineNode) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Record.Order < nodes[j].Record.Order
	})
}

// sortNodesRecursive sorts children of a node recursively
func sortNodesRecursive(node *TimelineNode) {
	sortNodes(node.Children)
	for _, child := range node.Children {
		sortNodesRecursive(child)
	}
}

// setDepth sets the depth for all nodes in the tree
func setDepth(nodes []*TimelineNode, depth int) {
	for _, node := range nodes {
		node.Depth = depth
		setDepth(node.Children, depth+1)
	}
}

// flattenTree converts a tree to a flat list (depth-first)
func flattenTree(roots []*TimelineNode) []*TimelineNode {
	var result []*TimelineNode
	for _, root := range roots {
		result = append(result, flattenNode(root)...)
	}
	return result
}

// flattenNode flattens a single node and its children
func flattenNode(node *TimelineNode) []*TimelineNode {
	result := []*TimelineNode{node}
	for _, child := range node.Children {
		result = append(result, flattenNode(child)...)
	}
	return result
}
