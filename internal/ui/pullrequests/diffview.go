package pullrequests

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/diff"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DiffViewMode represents the current sub-view within the diff viewer
type DiffViewMode int

const (
	DiffFileList DiffViewMode = iota // selectable list of changed files
	DiffFileView                     // scrollable diff for single file
)

// InputMode represents what kind of text input is active
type InputMode int

const (
	InputNone       InputMode = iota
	InputNewComment           // creating new code comment on a line
	InputReply                // replying to existing thread
)

// diffLineType represents the type of a flattened diff display line
type diffLineType int

const (
	diffLineContext    diffLineType = iota
	diffLineAdded
	diffLineRemoved
	diffLineHunkHeader
	diffLineComment
	diffLineFileHeader
)

// diffLine is a flattened rendering line in the diff view
type diffLine struct {
	Type       diffLineType
	Content    string
	OldNum     int
	NewNum     int
	ThreadID   int // non-zero if this is a comment line
	CommentIdx int
}

// DiffModel is the diff viewer component
type DiffModel struct {
	client  *azdevops.Client
	pr      azdevops.PullRequest
	threads []azdevops.Thread

	// File list state
	changedFiles []azdevops.IterationChange
	fileIndex    int

	// File diff state
	currentFile *azdevops.IterationChange
	currentDiff *diff.FileDiff
	fileThreads map[int][]azdevops.Thread // newLineNum -> threads

	// Flattened rendering
	diffLines    []diffLine
	selectedLine int

	// Input
	inputMode     InputMode
	textInput     textinput.Model
	replyThreadID int

	// Layout
	viewMode      DiffViewMode
	viewport      viewport.Model
	width, height int
	ready         bool
	loading       bool
	err           error
	statusMessage string
	spinner       *components.LoadingIndicator
	styles        *styles.Styles
}

// NewDiffModel creates a new diff viewer model
func NewDiffModel(client *azdevops.Client, pr azdevops.PullRequest, threads []azdevops.Thread, s *styles.Styles) *DiffModel {
	sp := components.NewLoadingIndicator(s)
	sp.SetMessage("Loading changed files...")

	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 500

	return &DiffModel{
		client:   client,
		pr:       pr,
		threads:  threads,
		viewMode: DiffFileList,
		spinner:  sp,
		styles:   s,
		textInput: ti,
	}
}

// Init initializes the diff model by fetching changed files
func (m *DiffModel) Init() tea.Cmd {
	m.loading = true
	m.spinner.SetVisible(true)
	return tea.Batch(m.fetchChangedFiles(), m.spinner.Init())
}

// InitWithFile initializes the diff model and immediately opens a specific file's diff
func (m *DiffModel) InitWithFile(file azdevops.IterationChange) tea.Cmd {
	m.currentFile = &file
	m.loading = true
	m.spinner.SetMessage("Loading diff...")
	m.spinner.SetVisible(true)
	return tea.Batch(m.fetchChangedFiles(), m.fetchFileDiff(file), m.spinner.Init())
}

// Update handles messages
func (m *DiffModel) Update(msg tea.Msg) (*DiffModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case changedFilesMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.changedFiles = filterFileChanges(msg.changes)
		m.fileIndex = 0
		if m.viewMode == DiffFileList {
			m.updateFileListViewport()
		}

	case fileDiffMsg:
		m.loading = false
		m.spinner.SetVisible(false)
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.currentDiff = msg.diff
		m.fileThreads = msg.fileThreads
		m.viewMode = DiffFileView
		m.selectedLine = 0
		m.buildDiffLines()
		m.updateDiffViewport()

	case commentResultMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.statusMessage = msg.message
			// Refresh threads
			return m, m.refreshThreads()
		}

	case threadsRefreshMsg:
		if msg.err == nil {
			m.threads = msg.threads
			if m.viewMode == DiffFileView && m.currentFile != nil {
				m.fileThreads = diff.MapThreadsToLines(m.threads, m.currentFile.Item.Path)
				m.buildDiffLines()
				m.updateDiffViewport()
			}
		}

	case tea.KeyMsg:
		if m.inputMode != InputNone {
			return m.updateInput(msg)
		}
		switch m.viewMode {
		case DiffFileList:
			return m.updateFileList(msg)
		case DiffFileView:
			return m.updateDiffView(msg)
		}
	}

	return m, nil
}

// updateFileList handles key events in file list mode
func (m *DiffModel) updateFileList(msg tea.KeyMsg) (*DiffModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.fileIndex > 0 {
			m.fileIndex--
			m.updateFileListViewport()
		}
	case "down", "j":
		if m.fileIndex < len(m.changedFiles)-1 {
			m.fileIndex++
			m.updateFileListViewport()
		}
	case "pgup":
		m.fileIndex -= m.viewport.Height
		if m.fileIndex < 0 {
			m.fileIndex = 0
		}
		m.updateFileListViewport()
	case "pgdown":
		m.fileIndex += m.viewport.Height
		if m.fileIndex >= len(m.changedFiles) {
			m.fileIndex = len(m.changedFiles) - 1
		}
		m.updateFileListViewport()
	case "enter":
		if len(m.changedFiles) > 0 && m.fileIndex < len(m.changedFiles) {
			change := m.changedFiles[m.fileIndex]
			m.currentFile = &change
			m.loading = true
			m.spinner.SetMessage("Loading diff...")
			m.spinner.SetVisible(true)
			return m, tea.Batch(m.fetchFileDiff(change), m.spinner.Tick())
		}
	case "r":
		m.loading = true
		m.spinner.SetMessage("Refreshing...")
		m.spinner.SetVisible(true)
		m.err = nil
		return m, tea.Batch(m.fetchChangedFiles(), m.spinner.Tick())
	case "esc":
		return m, func() tea.Msg { return exitDiffViewMsg{} }
	}
	return m, nil
}

// updateDiffView handles key events in file diff mode
func (m *DiffModel) updateDiffView(msg tea.KeyMsg) (*DiffModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedLine > 0 {
			m.selectedLine--
			m.updateDiffViewport()
			m.ensureDiffLineVisible()
		}
	case "down", "j":
		if m.selectedLine < len(m.diffLines)-1 {
			m.selectedLine++
			m.updateDiffViewport()
			m.ensureDiffLineVisible()
		}
	case "pgup":
		m.selectedLine -= m.viewport.Height
		if m.selectedLine < 0 {
			m.selectedLine = 0
		}
		m.updateDiffViewport()
		m.ensureDiffLineVisible()
	case "pgdown":
		m.selectedLine += m.viewport.Height
		if m.selectedLine >= len(m.diffLines) {
			m.selectedLine = len(m.diffLines) - 1
		}
		m.updateDiffViewport()
		m.ensureDiffLineVisible()
	case "c":
		// Create new comment on current line
		line := m.currentDiffLine()
		if line != nil && (line.Type == diffLineAdded || line.Type == diffLineContext || line.Type == diffLineRemoved) {
			m.inputMode = InputNewComment
			m.textInput.SetValue("")
			m.textInput.Focus()
			m.textInput.Placeholder = "New comment..."
			return m, m.textInput.Focus()
		}
	case "p":
		// Reply to nearest thread
		threadID := m.findNearestThread()
		if threadID > 0 {
			m.inputMode = InputReply
			m.replyThreadID = threadID
			m.textInput.SetValue("")
			m.textInput.Focus()
			m.textInput.Placeholder = "Reply..."
			return m, m.textInput.Focus()
		}
	case "x":
		// Resolve nearest thread
		threadID := m.findNearestThread()
		if threadID > 0 {
			return m, m.resolveThread(threadID)
		}
	case "n":
		// Jump to next comment
		m.jumpToNextComment(1)
		m.updateDiffViewport()
		m.ensureDiffLineVisible()
	case "N":
		// Jump to previous comment
		m.jumpToNextComment(-1)
		m.updateDiffViewport()
		m.ensureDiffLineVisible()
	case "esc":
		// Exit diff view entirely, back to detail
		return m, func() tea.Msg { return exitDiffViewMsg{} }
	}
	return m, nil
}

// updateInput handles key events in text input mode
func (m *DiffModel) updateInput(msg tea.KeyMsg) (*DiffModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = InputNone
		m.textInput.Blur()
		return m, nil
	case "enter":
		content := strings.TrimSpace(m.textInput.Value())
		if content == "" {
			return m, nil
		}
		m.textInput.Blur()
		mode := m.inputMode
		m.inputMode = InputNone

		switch mode {
		case InputNewComment:
			line := m.currentDiffLine()
			if line != nil && m.currentFile != nil {
				lineNum := line.NewNum
				if lineNum == 0 {
					lineNum = line.OldNum
				}
				return m, m.createCodeComment(m.currentFile.Item.Path, lineNum, content)
			}
		case InputReply:
			if m.replyThreadID > 0 {
				return m, m.replyToThread(m.replyThreadID, content)
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the diff view
func (m *DiffModel) View() string {
	contentStyle := lipgloss.NewStyle().Width(m.width)

	if m.err != nil {
		return contentStyle.Render(fmt.Sprintf("Error: %v\n\nPress r to retry, Esc to go back", m.err))
	}
	if m.loading {
		return contentStyle.Render(m.spinner.View())
	}

	switch m.viewMode {
	case DiffFileList:
		return contentStyle.Render(m.viewFileList())
	case DiffFileView:
		return contentStyle.Render(m.viewFileDiff())
	}
	return ""
}

// viewFileList renders the list of changed files
func (m *DiffModel) viewFileList() string {
	if !m.ready {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(m.styles.Header.Render(fmt.Sprintf("Changed files (%d)", len(m.changedFiles))))
	sb.WriteString("\n")
	sb.WriteString(m.viewport.View())
	return sb.String()
}

// viewFileDiff renders the diff for a single file
func (m *DiffModel) viewFileDiff() string {
	if !m.ready {
		return ""
	}

	var sb strings.Builder

	// File header
	if m.currentFile != nil {
		sb.WriteString(m.styles.DiffHeader.Render(fmt.Sprintf(" %s ", m.currentFile.Item.Path)))
		sb.WriteString("\n")
	}

	sb.WriteString(m.viewport.View())

	// Input bar
	if m.inputMode != InputNone {
		sb.WriteString("\n")
		sb.WriteString(m.textInput.View())
	}

	return sb.String()
}

// SetSize sets the component size
func (m *DiffModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	headerLines := 1 // file list or diff header
	viewportHeight := height - headerLines
	if m.inputMode != InputNone {
		viewportHeight-- // input bar
	}
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

	if m.viewMode == DiffFileList {
		m.updateFileListViewport()
	} else {
		m.updateDiffViewport()
	}
}

// GetContextItems returns context items for the current view
func (m *DiffModel) GetContextItems() []components.ContextItem {
	if m.inputMode != InputNone {
		return []components.ContextItem{
			{Key: "enter", Description: "submit"},
			{Key: "esc", Description: "cancel"},
		}
	}

	switch m.viewMode {
	case DiffFileList:
		return []components.ContextItem{
			{Key: "↑↓", Description: "navigate"},
			{Key: "pgup/pgdn", Description: "page"},
			{Key: "enter", Description: "open"},
			{Key: "r", Description: "refresh"},
			{Key: "esc", Description: "back"},
		}
	case DiffFileView:
		return []components.ContextItem{
			{Key: "↑↓", Description: "scroll"},
			{Key: "c", Description: "comment"},
			{Key: "p", Description: "reply"},
			{Key: "x", Description: "resolve"},
			{Key: "n/N", Description: "next/prev comment"},
			{Key: "esc", Description: "back"},
		}
	}
	return nil
}

// GetScrollPercent returns the scroll percentage
func (m *DiffModel) GetScrollPercent() float64 {
	if !m.ready {
		return 0
	}
	return m.viewport.ScrollPercent() * 100
}

// GetStatusMessage returns the status message
func (m *DiffModel) GetStatusMessage() string {
	return m.statusMessage
}

// --- Rendering helpers ---

// updateFileListViewport rebuilds the file list viewport content
func (m *DiffModel) updateFileListViewport() {
	if !m.ready {
		return
	}

	var sb strings.Builder
	for i, change := range m.changedFiles {
		icon, style := changeTypeDisplay(change.ChangeType, m.styles)
		line := fmt.Sprintf("  %s %s", icon, change.Item.Path)
		if change.ChangeType == "rename" && change.OriginalPath != "" {
			line = fmt.Sprintf("  %s %s -> %s", icon, change.OriginalPath, change.Item.Path)
		}
		if i == m.fileIndex {
			sb.WriteString(m.styles.Selected.Render(line))
		} else {
			sb.WriteString(style.Render(line))
		}
		if i < len(m.changedFiles)-1 {
			sb.WriteString("\n")
		}
	}

	if len(m.changedFiles) == 0 {
		sb.WriteString(m.styles.Muted.Render("  No changed files"))
	}

	m.viewport.SetContent(sb.String())
	m.ensureFileIndexVisible()
}

// ensureFileIndexVisible scrolls the viewport to keep the selected file visible
func (m *DiffModel) ensureFileIndexVisible() {
	if !m.ready || len(m.changedFiles) == 0 {
		return
	}

	if m.fileIndex < m.viewport.YOffset {
		m.viewport.SetYOffset(m.fileIndex)
	} else if m.fileIndex >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.fileIndex - m.viewport.Height + 1)
	}
}

// buildDiffLines flattens hunks + inline comments into diffLines slice
func (m *DiffModel) buildDiffLines() {
	m.diffLines = nil
	if m.currentDiff == nil {
		return
	}

	for _, hunk := range m.currentDiff.Hunks {
		// Hunk header
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount)
		m.diffLines = append(m.diffLines, diffLine{
			Type:    diffLineHunkHeader,
			Content: header,
		})

		for _, line := range hunk.Lines {
			var dlt diffLineType
			switch line.Type {
			case diff.Added:
				dlt = diffLineAdded
			case diff.Removed:
				dlt = diffLineRemoved
			default:
				dlt = diffLineContext
			}

			m.diffLines = append(m.diffLines, diffLine{
				Type:    dlt,
				Content: line.Content,
				OldNum:  line.OldNum,
				NewNum:  line.NewNum,
			})

			// Insert inline comments after the relevant line
			lineNum := line.NewNum
			if lineNum == 0 {
				lineNum = line.OldNum
			}
			if threads, ok := m.fileThreads[lineNum]; ok && line.Type != diff.Removed {
				for _, thread := range threads {
					for ci, comment := range thread.Comments {
						m.diffLines = append(m.diffLines, diffLine{
							Type:       diffLineComment,
							Content:    fmt.Sprintf("%s: %s", comment.Author.DisplayName, comment.Content),
							ThreadID:   thread.ID,
							CommentIdx: ci,
						})
					}
				}
				// Remove from map to avoid duplicates if same line appears in multiple hunks
				delete(m.fileThreads, lineNum)
			}
		}
	}
}

// updateDiffViewport rebuilds the diff viewport content
func (m *DiffModel) updateDiffViewport() {
	if !m.ready {
		return
	}

	var sb strings.Builder
	for i, line := range m.diffLines {
		rendered := m.renderDiffLine(line, i == m.selectedLine)
		sb.WriteString(rendered)
		if i < len(m.diffLines)-1 {
			sb.WriteString("\n")
		}
	}

	if len(m.diffLines) == 0 {
		sb.WriteString(m.styles.Muted.Render("  No changes"))
	}

	m.viewport.SetContent(sb.String())
}

// renderDiffLine renders a single flattened diff line
func (m *DiffModel) renderDiffLine(line diffLine, selected bool) string {
	var result string

	switch line.Type {
	case diffLineHunkHeader:
		result = m.styles.DiffHunkHeader.Render(line.Content)

	case diffLineContext:
		oldNum := fmt.Sprintf("%4d", line.OldNum)
		newNum := fmt.Sprintf("%4d", line.NewNum)
		gutter := m.styles.DiffLineNum.Render(oldNum) + " " + m.styles.DiffLineNum.Render(newNum)
		result = gutter + "  " + m.styles.DiffContext.Render(line.Content)

	case diffLineAdded:
		oldNum := "    "
		newNum := fmt.Sprintf("%4d", line.NewNum)
		gutter := m.styles.DiffLineNum.Render(oldNum) + " " + m.styles.DiffLineNum.Render(newNum)
		result = gutter + m.styles.DiffAdded.Render(" +" + line.Content)

	case diffLineRemoved:
		oldNum := fmt.Sprintf("%4d", line.OldNum)
		newNum := "    "
		gutter := m.styles.DiffLineNum.Render(oldNum) + " " + m.styles.DiffLineNum.Render(newNum)
		result = gutter + m.styles.DiffRemoved.Render(" -" + line.Content)

	case diffLineComment:
		gutter := "           " // 11 spaces matching line number gutter width
		var firstIndent, contIndent string
		if line.CommentIdx > 0 {
			firstIndent = gutter + "  └ "
			contIndent = gutter + "    "
		} else {
			firstIndent = gutter + "  "
			contIndent = gutter + "  "
		}
		contentLines := strings.Split(line.Content, "\n")
		for i, l := range contentLines {
			if i == 0 {
				contentLines[i] = firstIndent + l
			} else {
				contentLines[i] = contIndent + l
			}
		}
		result = m.styles.Info.Render(strings.Join(contentLines, "\n"))

	case diffLineFileHeader:
		result = m.styles.DiffHeader.Render(line.Content)
	}

	if selected {
		result = m.styles.Selected.Render(result)
	}

	return result
}

// ensureDiffLineVisible scrolls the viewport to keep selected line visible
func (m *DiffModel) ensureDiffLineVisible() {
	if !m.ready || len(m.diffLines) == 0 {
		return
	}

	if m.selectedLine < m.viewport.YOffset {
		m.viewport.SetYOffset(m.selectedLine)
	} else if m.selectedLine >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.selectedLine - m.viewport.Height + 1)
	}
}

// currentDiffLine returns the currently selected diff line
func (m *DiffModel) currentDiffLine() *diffLine {
	if m.selectedLine < 0 || m.selectedLine >= len(m.diffLines) {
		return nil
	}
	return &m.diffLines[m.selectedLine]
}

// findNearestThread finds the nearest thread ID to the current selection
func (m *DiffModel) findNearestThread() int {
	if len(m.diffLines) == 0 {
		return 0
	}

	// Search from current position upward for a comment line
	for i := m.selectedLine; i >= 0; i-- {
		if m.diffLines[i].Type == diffLineComment && m.diffLines[i].ThreadID > 0 {
			return m.diffLines[i].ThreadID
		}
	}
	// Search downward
	for i := m.selectedLine; i < len(m.diffLines); i++ {
		if m.diffLines[i].Type == diffLineComment && m.diffLines[i].ThreadID > 0 {
			return m.diffLines[i].ThreadID
		}
	}
	return 0
}

// jumpToNextComment moves the selection to the next/previous comment
func (m *DiffModel) jumpToNextComment(direction int) {
	if len(m.diffLines) == 0 {
		return
	}

	start := m.selectedLine + direction
	for i := start; i >= 0 && i < len(m.diffLines); i += direction {
		if m.diffLines[i].Type == diffLineComment {
			m.selectedLine = i
			return
		}
	}
}


// filterFileChanges removes folder/tree entries and entries with empty paths
func filterFileChanges(changes []azdevops.IterationChange) []azdevops.IterationChange {
	filtered := make([]azdevops.IterationChange, 0, len(changes))
	for _, c := range changes {
		if c.Item.Path == "" || c.Item.Path == "/" {
			continue
		}
		if c.Item.GitObjectType == "tree" {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// --- Messages ---

type changedFilesMsg struct {
	changes []azdevops.IterationChange
	err     error
}

type fileDiffMsg struct {
	diff        *diff.FileDiff
	fileThreads map[int][]azdevops.Thread
	err         error
}

type commentResultMsg struct {
	message string
	err     error
}

type threadsRefreshMsg struct {
	threads []azdevops.Thread
	err     error
}

// exitDiffViewMsg signals that the user wants to leave the diff view
type exitDiffViewMsg struct{}

// --- Commands ---

// fetchChangedFiles loads iterations and then changed files
func (m *DiffModel) fetchChangedFiles() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return changedFilesMsg{err: fmt.Errorf("no client available")}
		}

		// Get iterations
		iterations, err := m.client.GetPRIterations(m.pr.Repository.ID, m.pr.ID)
		if err != nil {
			return changedFilesMsg{err: err}
		}
		if len(iterations) == 0 {
			return changedFilesMsg{changes: nil, err: nil}
		}

		// Get changes from the latest iteration compared to base
		latestID := iterations[len(iterations)-1].ID
		changes, err := m.client.GetPRIterationChanges(m.pr.Repository.ID, m.pr.ID, latestID)
		if err != nil {
			return changedFilesMsg{err: err}
		}

		return changedFilesMsg{changes: changes}
	}
}

// fetchFileDiff loads file content at both branches and computes the diff
func (m *DiffModel) fetchFileDiff(change azdevops.IterationChange) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return fileDiffMsg{err: fmt.Errorf("no client available")}
		}

		targetBranch := m.pr.TargetBranchShortName()
		sourceBranch := m.pr.SourceBranchShortName()

		var oldContent, newContent string
		var err error

		switch change.ChangeType {
		case "add":
			// New file: no old content
			newContent, err = m.client.GetFileContent(m.pr.Repository.ID, change.Item.Path, sourceBranch)
			if err != nil {
				return fileDiffMsg{err: err}
			}
		case "delete":
			// Deleted file: no new content
			oldContent, err = m.client.GetFileContent(m.pr.Repository.ID, change.Item.Path, targetBranch)
			if err != nil {
				return fileDiffMsg{err: err}
			}
		case "rename":
			// Renamed: old path on target, new path on source
			oldPath := change.OriginalPath
			if oldPath == "" {
				oldPath = change.Item.Path
			}
			oldContent, err = m.client.GetFileContent(m.pr.Repository.ID, oldPath, targetBranch)
			if err != nil {
				return fileDiffMsg{err: err}
			}
			newContent, err = m.client.GetFileContent(m.pr.Repository.ID, change.Item.Path, sourceBranch)
			if err != nil {
				return fileDiffMsg{err: err}
			}
		default: // "edit"
			oldContent, err = m.client.GetFileContent(m.pr.Repository.ID, change.Item.Path, targetBranch)
			if err != nil {
				return fileDiffMsg{err: err}
			}
			newContent, err = m.client.GetFileContent(m.pr.Repository.ID, change.Item.Path, sourceBranch)
			if err != nil {
				return fileDiffMsg{err: err}
			}
		}

		hunks := diff.ComputeDiff(oldContent, newContent, 5)
		fileDiff := &diff.FileDiff{
			Path:       change.Item.Path,
			ChangeType: change.ChangeType,
			OldPath:    change.OriginalPath,
			Hunks:      hunks,
		}

		fileThreads := diff.MapThreadsToLines(m.threads, change.Item.Path)

		return fileDiffMsg{diff: fileDiff, fileThreads: fileThreads}
	}
}

// createCodeComment creates a new code comment
func (m *DiffModel) createCodeComment(filePath string, line int, content string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return commentResultMsg{err: fmt.Errorf("no client available")}
		}
		_, err := m.client.AddPRCodeComment(m.pr.Repository.ID, m.pr.ID, filePath, line, content)
		if err != nil {
			return commentResultMsg{err: err}
		}
		return commentResultMsg{message: "Comment added"}
	}
}

// replyToThread replies to an existing thread
func (m *DiffModel) replyToThread(threadID int, content string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return commentResultMsg{err: fmt.Errorf("no client available")}
		}
		_, err := m.client.ReplyToThread(m.pr.Repository.ID, m.pr.ID, threadID, content)
		if err != nil {
			return commentResultMsg{err: err}
		}
		return commentResultMsg{message: "Reply added"}
	}
}

// resolveThread resolves a thread
func (m *DiffModel) resolveThread(threadID int) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return commentResultMsg{err: fmt.Errorf("no client available")}
		}
		err := m.client.UpdateThreadStatus(m.pr.Repository.ID, m.pr.ID, threadID, "fixed")
		if err != nil {
			return commentResultMsg{err: err}
		}
		return commentResultMsg{message: "Thread resolved"}
	}
}

// refreshThreads re-fetches threads from the API
func (m *DiffModel) refreshThreads() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return threadsRefreshMsg{err: fmt.Errorf("no client available")}
		}
		threads, err := m.client.GetPRThreads(m.pr.Repository.ID, m.pr.ID)
		if err != nil {
			return threadsRefreshMsg{err: err}
		}
		return threadsRefreshMsg{threads: azdevops.FilterSystemThreads(threads)}
	}
}
