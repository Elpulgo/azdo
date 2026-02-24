package pullrequests

import (
	"fmt"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/diff"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestDiffModel() *DiffModel {
	pr := azdevops.PullRequest{
		ID:            101,
		Title:         "Test PR",
		SourceRefName: "refs/heads/feature/test",
		TargetRefName: "refs/heads/main",
		Repository:    azdevops.Repository{ID: "repo-123", Name: "test-repo"},
	}
	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Fix this", Author: azdevops.Identity{DisplayName: "Alice"}},
			},
		},
	}
	s := styles.DefaultStyles()
	return NewDiffModel(nil, pr, threads, s)
}

func TestNewDiffModel(t *testing.T) {
	m := newTestDiffModel()

	if m.viewMode != DiffFileList {
		t.Errorf("Initial viewMode = %d, want DiffFileList", m.viewMode)
	}
	if m.loading {
		t.Error("Should not be loading initially")
	}
	if m.inputMode != InputNone {
		t.Error("Input mode should be InputNone initially")
	}
}

func TestDiffModel_FileListNavigation(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)

	// Simulate receiving changed files
	m.changedFiles = []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/new.go"}, ChangeType: "add"},
		{ChangeID: 3, Item: azdevops.ChangeItem{Path: "/src/old.go"}, ChangeType: "delete"},
	}
	m.updateFileListViewport()

	if m.fileIndex != 0 {
		t.Errorf("Initial fileIndex = %d, want 0", m.fileIndex)
	}

	// Navigate down
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.fileIndex != 1 {
		t.Errorf("After j, fileIndex = %d, want 1", m.fileIndex)
	}

	// Navigate down again
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.fileIndex != 2 {
		t.Errorf("After j, fileIndex = %d, want 2", m.fileIndex)
	}

	// Navigate down at bottom (should stay)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.fileIndex != 2 {
		t.Errorf("After j at bottom, fileIndex = %d, want 2", m.fileIndex)
	}

	// Navigate up
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.fileIndex != 1 {
		t.Errorf("After k, fileIndex = %d, want 1", m.fileIndex)
	}
}

func TestDiffModel_FileListNavigation_UpDown(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)

	m.changedFiles = []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/a.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/b.go"}, ChangeType: "edit"},
	}
	m.updateFileListViewport()

	// Arrow down
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.fileIndex != 1 {
		t.Errorf("After down, fileIndex = %d, want 1", m.fileIndex)
	}

	// Arrow up
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.fileIndex != 0 {
		t.Errorf("After up, fileIndex = %d, want 0", m.fileIndex)
	}

	// Arrow up at top (should stay)
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.fileIndex != 0 {
		t.Errorf("After up at top, fileIndex = %d, want 0", m.fileIndex)
	}
}

func TestDiffModel_ChangeTypeDisplay(t *testing.T) {
	s := styles.DefaultStyles()

	tests := []struct {
		changeType string
		wantIcon   string
	}{
		{"add", "+"},
		{"edit", "~"},
		{"delete", "-"},
		{"rename", "→"},
		{"unknown", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.changeType, func(t *testing.T) {
			icon, _ := changeTypeDisplay(tt.changeType, s)
			if icon != tt.wantIcon {
				t.Errorf("changeTypeDisplay(%q) icon = %q, want %q", tt.changeType, icon, tt.wantIcon)
			}
		})
	}
}

func TestDiffModel_BuildDiffLines(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)

	m.currentDiff = &diff.FileDiff{
		Path:       "/src/main.go",
		ChangeType: "edit",
		Hunks: []diff.Hunk{
			{
				OldStart: 1, OldCount: 3,
				NewStart: 1, NewCount: 3,
				Lines: []diff.Line{
					{Type: diff.Context, Content: "line1", OldNum: 1, NewNum: 1},
					{Type: diff.Removed, Content: "old", OldNum: 2, NewNum: 0},
					{Type: diff.Added, Content: "new", OldNum: 0, NewNum: 2},
					{Type: diff.Context, Content: "line3", OldNum: 3, NewNum: 3},
				},
			},
		},
	}
	m.fileThreads = make(map[int][]azdevops.Thread)

	m.buildDiffLines()

	// Expect: hunk header + 4 diff lines = 5
	if len(m.diffLines) != 5 {
		t.Fatalf("Expected 5 diffLines, got %d", len(m.diffLines))
	}

	// First line should be hunk header
	if m.diffLines[0].Type != diffLineHunkHeader {
		t.Errorf("diffLines[0].Type = %d, want diffLineHunkHeader", m.diffLines[0].Type)
	}

	// Verify types
	expectedTypes := []diffLineType{diffLineHunkHeader, diffLineContext, diffLineRemoved, diffLineAdded, diffLineContext}
	for i, expected := range expectedTypes {
		if m.diffLines[i].Type != expected {
			t.Errorf("diffLines[%d].Type = %d, want %d", i, m.diffLines[i].Type, expected)
		}
	}
}

func TestDiffModel_BuildDiffLines_WithComments(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)

	m.currentDiff = &diff.FileDiff{
		Path:       "/src/main.go",
		ChangeType: "edit",
		Hunks: []diff.Hunk{
			{
				OldStart: 9, OldCount: 3,
				NewStart: 9, NewCount: 3,
				Lines: []diff.Line{
					{Type: diff.Context, Content: "line9", OldNum: 9, NewNum: 9},
					{Type: diff.Context, Content: "line10", OldNum: 10, NewNum: 10},
					{Type: diff.Context, Content: "line11", OldNum: 11, NewNum: 11},
				},
			},
		},
	}
	m.fileThreads = map[int][]azdevops.Thread{
		10: {
			{
				ID:     1,
				Status: "active",
				Comments: []azdevops.Comment{
					{ID: 1, Content: "Fix this", Author: azdevops.Identity{DisplayName: "Alice"}},
					{ID: 2, Content: "Will do", Author: azdevops.Identity{DisplayName: "Bob"}, ParentCommentID: 1},
				},
			},
		},
	}

	m.buildDiffLines()

	// Expect: hunk header + line9 + line10 + 2 comments + line11 = 6
	if len(m.diffLines) != 6 {
		t.Fatalf("Expected 6 diffLines, got %d", len(m.diffLines))
	}

	// Lines at index 3 and 4 should be comments
	if m.diffLines[3].Type != diffLineComment {
		t.Errorf("diffLines[3].Type = %d, want diffLineComment", m.diffLines[3].Type)
	}
	if m.diffLines[3].ThreadID != 1 {
		t.Errorf("diffLines[3].ThreadID = %d, want 1", m.diffLines[3].ThreadID)
	}
	if m.diffLines[4].Type != diffLineComment {
		t.Errorf("diffLines[4].Type = %d, want diffLineComment", m.diffLines[4].Type)
	}
	if m.diffLines[4].CommentIdx != 1 {
		t.Errorf("diffLines[4].CommentIdx = %d, want 1", m.diffLines[4].CommentIdx)
	}
}

func TestDiffModel_DiffViewNavigation(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.viewMode = DiffFileView

	m.diffLines = []diffLine{
		{Type: diffLineHunkHeader, Content: "@@ -1,3 +1,3 @@"},
		{Type: diffLineContext, Content: "line1", OldNum: 1, NewNum: 1},
		{Type: diffLineRemoved, Content: "old", OldNum: 2},
		{Type: diffLineAdded, Content: "new", NewNum: 2},
		{Type: diffLineContext, Content: "line3", OldNum: 3, NewNum: 3},
	}
	m.updateDiffViewport()

	if m.selectedLine != 0 {
		t.Errorf("Initial selectedLine = %d, want 0", m.selectedLine)
	}

	// Move down
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.selectedLine != 1 {
		t.Errorf("After j, selectedLine = %d, want 1", m.selectedLine)
	}

	// Move up
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.selectedLine != 0 {
		t.Errorf("After k, selectedLine = %d, want 0", m.selectedLine)
	}
}

func TestDiffModel_FindNearestThread(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.viewMode = DiffFileView

	m.diffLines = []diffLine{
		{Type: diffLineContext, Content: "line1"},
		{Type: diffLineContext, Content: "line2"},
		{Type: diffLineComment, Content: "Alice: Fix this", ThreadID: 5},
		{Type: diffLineContext, Content: "line3"},
	}

	// From line 3 (after comment), should find thread 5
	m.selectedLine = 3
	if got := m.findNearestThread(); got != 5 {
		t.Errorf("findNearestThread() from line 3 = %d, want 5", got)
	}

	// From line 0 (before comment), should still find thread 5
	m.selectedLine = 0
	if got := m.findNearestThread(); got != 5 {
		t.Errorf("findNearestThread() from line 0 = %d, want 5", got)
	}
}

func TestDiffModel_FindNearestThread_NoThreads(t *testing.T) {
	m := newTestDiffModel()
	m.diffLines = []diffLine{
		{Type: diffLineContext, Content: "line1"},
	}
	m.selectedLine = 0
	if got := m.findNearestThread(); got != 0 {
		t.Errorf("findNearestThread() with no threads = %d, want 0", got)
	}
}

func TestDiffModel_JumpToNextComment(t *testing.T) {
	m := newTestDiffModel()
	m.diffLines = []diffLine{
		{Type: diffLineContext, Content: "line1"},
		{Type: diffLineComment, Content: "comment1", ThreadID: 1},
		{Type: diffLineContext, Content: "line2"},
		{Type: diffLineComment, Content: "comment2", ThreadID: 2},
		{Type: diffLineContext, Content: "line3"},
	}
	m.selectedLine = 0

	// Jump forward to first comment
	m.jumpToNextComment(1)
	if m.selectedLine != 1 {
		t.Errorf("After jumpToNextComment(1), selectedLine = %d, want 1", m.selectedLine)
	}

	// Jump forward to second comment
	m.jumpToNextComment(1)
	if m.selectedLine != 3 {
		t.Errorf("After jumpToNextComment(1), selectedLine = %d, want 3", m.selectedLine)
	}

	// Jump backward to first comment
	m.jumpToNextComment(-1)
	if m.selectedLine != 1 {
		t.Errorf("After jumpToNextComment(-1), selectedLine = %d, want 1", m.selectedLine)
	}
}

func TestDiffModel_GetContextItems_FileList(t *testing.T) {
	m := newTestDiffModel()
	m.viewMode = DiffFileList

	items := m.GetContextItems()
	if len(items) == 0 {
		t.Error("Expected context items for file list mode")
	}

	// Should have navigate, page, open, refresh, back
	hasNavigate := false
	hasPage := false
	hasBack := false
	for _, item := range items {
		if item.Description == "navigate" {
			hasNavigate = true
		}
		if item.Description == "page" {
			hasPage = true
		}
		if item.Description == "back" {
			hasBack = true
		}
	}
	if !hasNavigate {
		t.Error("Missing 'navigate' context item")
	}
	if !hasPage {
		t.Error("Missing 'page' context item")
	}
	if !hasBack {
		t.Error("Missing 'back' context item")
	}
}

func TestDiffModel_GetContextItems_DiffView(t *testing.T) {
	m := newTestDiffModel()
	m.viewMode = DiffFileView

	items := m.GetContextItems()
	if len(items) == 0 {
		t.Error("Expected context items for diff view mode")
	}

	// Should have comment, reply, resolve
	hasComment := false
	hasReply := false
	hasResolve := false
	for _, item := range items {
		if item.Description == "comment" {
			hasComment = true
		}
		if item.Description == "reply" {
			hasReply = true
		}
		if item.Description == "resolve" {
			hasResolve = true
		}
	}
	if !hasComment {
		t.Error("Missing 'comment' context item")
	}
	if !hasReply {
		t.Error("Missing 'reply' context item")
	}
	if !hasResolve {
		t.Error("Missing 'resolve' context item")
	}
}

func TestDiffModel_GetContextItems_InputMode(t *testing.T) {
	m := newTestDiffModel()
	m.inputMode = InputNewComment

	items := m.GetContextItems()
	if len(items) != 2 {
		t.Fatalf("Expected 2 context items in input mode, got %d", len(items))
	}
}

func TestDiffModel_EscFromFileList(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.viewMode = DiffFileList

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Expected esc to produce a command")
	}

	// Execute the command to get the message
	msg := cmd()
	if _, ok := msg.(exitDiffViewMsg); !ok {
		t.Errorf("Expected exitDiffViewMsg, got %T", msg)
	}
}

func TestDiffModel_EscFromDiffView(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.viewMode = DiffFileView
	m.currentFile = &azdevops.IterationChange{Item: azdevops.ChangeItem{Path: "/test.go"}}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Expected esc from diff view to produce a command")
	}

	// Should emit exitDiffViewMsg (back to detail view)
	msg := cmd()
	if _, ok := msg.(exitDiffViewMsg); !ok {
		t.Errorf("Expected exitDiffViewMsg, got %T", msg)
	}
}

func TestDiffModel_View_Loading(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.loading = true
	m.spinner.SetVisible(true)

	view := m.View()
	// Should show spinner, not error
	if view == "" {
		t.Error("View should not be empty when loading")
	}
}

func TestDiffModel_View_Error(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.err = fmt.Errorf("test error")

	view := m.View()
	if view == "" {
		t.Error("View should not be empty when error")
	}
}

func TestDiffModel_View_EmptyFileList(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.changedFiles = nil
	m.updateFileListViewport()

	view := m.View()
	if view == "" {
		t.Error("View should not be empty for empty file list")
	}
}

func TestDiffModel_ChangedFilesMsg(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.loading = true

	changes := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/a.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/b.go"}, ChangeType: "add"},
	}

	m.Update(changedFilesMsg{changes: changes})

	if m.loading {
		t.Error("Should not be loading after changedFilesMsg")
	}
	if len(m.changedFiles) != 2 {
		t.Errorf("Expected 2 changed files, got %d", len(m.changedFiles))
	}
	if m.fileIndex != 0 {
		t.Errorf("fileIndex = %d, want 0", m.fileIndex)
	}
}

func TestDiffModel_ChangedFilesMsg_Error(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.loading = true

	m.Update(changedFilesMsg{err: fmt.Errorf("API error")})

	if m.loading {
		t.Error("Should not be loading after error")
	}
	if m.err == nil {
		t.Error("Expected error to be set")
	}
}

func TestDiffModel_FileListScrollsToKeepSelectionVisible(t *testing.T) {
	m := newTestDiffModel()
	// Small viewport: only 5 lines visible
	m.SetSize(80, 6) // 6 - 1 header = 5 viewport lines

	// Create 10 files, more than fit in the viewport
	files := make([]azdevops.IterationChange, 10)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 0
	m.updateFileListViewport()

	// Navigate down past the viewport boundary
	for i := 0; i < 7; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}

	if m.fileIndex != 7 {
		t.Fatalf("fileIndex = %d, want 7", m.fileIndex)
	}

	// The viewport should have scrolled so that fileIndex 7 is visible
	yOffset := m.viewport.YOffset
	viewBottom := yOffset + m.viewport.Height - 1
	if m.fileIndex < yOffset || m.fileIndex > viewBottom {
		t.Errorf("fileIndex %d not visible in viewport (yOffset=%d, height=%d, viewBottom=%d)",
			m.fileIndex, yOffset, m.viewport.Height, viewBottom)
	}
}

func TestDiffModel_FileListScrollsBackUp(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 6) // 5 viewport lines

	files := make([]azdevops.IterationChange, 10)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 0
	m.updateFileListViewport()

	// Navigate to bottom
	for i := 0; i < 9; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	// Navigate back to top
	for i := 0; i < 9; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	}

	if m.fileIndex != 0 {
		t.Fatalf("fileIndex = %d, want 0", m.fileIndex)
	}

	// Viewport should have scrolled back so file 0 is visible
	if m.viewport.YOffset > 0 {
		t.Errorf("viewport.YOffset = %d, want 0 (should scroll back to top)", m.viewport.YOffset)
	}
}

func TestDiffModel_FiltersFolderEntries(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.loading = true

	// Simulate API response with folder/tree entries mixed in
	changes := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/", GitObjectType: "tree"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/main.go", GitObjectType: "blob"}, ChangeType: "edit"},
		{ChangeID: 3, Item: azdevops.ChangeItem{Path: "/src", GitObjectType: "tree"}, ChangeType: "edit"},
		{ChangeID: 4, Item: azdevops.ChangeItem{Path: "/src/utils.go", GitObjectType: "blob"}, ChangeType: "add"},
	}

	m.Update(changedFilesMsg{changes: changes})

	// Should only have the blob entries (actual files)
	if len(m.changedFiles) != 2 {
		t.Errorf("Expected 2 file entries after filtering, got %d", len(m.changedFiles))
		for i, f := range m.changedFiles {
			t.Logf("  [%d] path=%q type=%q", i, f.Item.Path, f.Item.GitObjectType)
		}
	}
}

func TestFilterFileChanges(t *testing.T) {
	changes := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/", GitObjectType: "tree"}},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/main.go", GitObjectType: "blob"}, ChangeType: "edit"},
		{ChangeID: 3, Item: azdevops.ChangeItem{Path: "/src", GitObjectType: "tree"}},
		{ChangeID: 4, Item: azdevops.ChangeItem{Path: "/src/utils.go"}, ChangeType: "add"},
		{ChangeID: 5, Item: azdevops.ChangeItem{Path: ""}},
	}

	filtered := filterFileChanges(changes)

	if len(filtered) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(filtered))
	}
	if filtered[0].Item.Path != "/src/main.go" {
		t.Errorf("filtered[0].Path = %q, want /src/main.go", filtered[0].Item.Path)
	}
	if filtered[1].Item.Path != "/src/utils.go" {
		t.Errorf("filtered[1].Path = %q, want /src/utils.go", filtered[1].Item.Path)
	}
}

func TestDiffModel_FileListPageDown(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 6) // 5 viewport lines

	files := make([]azdevops.IterationChange, 20)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 0
	m.updateFileListViewport()

	// Page down should jump by viewport height
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.fileIndex != m.viewport.Height {
		t.Errorf("After pgdown, fileIndex = %d, want %d", m.fileIndex, m.viewport.Height)
	}

	// Should be visible
	yOffset := m.viewport.YOffset
	viewBottom := yOffset + m.viewport.Height - 1
	if m.fileIndex < yOffset || m.fileIndex > viewBottom {
		t.Errorf("fileIndex %d not visible after pgdown (yOffset=%d, viewBottom=%d)",
			m.fileIndex, yOffset, viewBottom)
	}
}

func TestDiffModel_FileListPageUp(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 6) // 5 viewport lines

	files := make([]azdevops.IterationChange, 20)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 15
	m.updateFileListViewport()

	// Page up should jump back by viewport height
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	expected := 15 - m.viewport.Height
	if m.fileIndex != expected {
		t.Errorf("After pgup from 15, fileIndex = %d, want %d", m.fileIndex, expected)
	}
}

func TestDiffModel_FileListPageDown_ClampsAtEnd(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 6)

	files := make([]azdevops.IterationChange, 8)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 6
	m.updateFileListViewport()

	// Page down near the end should clamp to last item
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.fileIndex != 7 {
		t.Errorf("After pgdown near end, fileIndex = %d, want 7", m.fileIndex)
	}
}

func TestDiffModel_FileListPageUp_ClampsAtStart(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 6)

	files := make([]azdevops.IterationChange, 10)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 2
	m.updateFileListViewport()

	// Page up near the top should clamp to 0
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.fileIndex != 0 {
		t.Errorf("After pgup near start, fileIndex = %d, want 0", m.fileIndex)
	}
}

func TestDiffModel_FileListScrollPercent(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 6) // 5 viewport lines

	files := make([]azdevops.IterationChange, 20)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	m.changedFiles = files
	m.fileIndex = 0
	m.updateFileListViewport()

	// At top, scroll percent should be 0
	pct := m.GetScrollPercent()
	if pct != 0 {
		t.Errorf("At top, scroll percent = %f, want 0", pct)
	}

	// Navigate to bottom
	for i := 0; i < 19; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}

	// At bottom, scroll percent should be 100
	pct = m.GetScrollPercent()
	if pct != 100 {
		t.Errorf("At bottom, scroll percent = %f, want 100", pct)
	}
}

func TestDiffModel_FiltersEmptyPathEntries(t *testing.T) {
	m := newTestDiffModel()
	m.SetSize(80, 24)
	m.loading = true

	changes := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: ""}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}

	m.Update(changedFilesMsg{changes: changes})

	if len(m.changedFiles) != 1 {
		t.Errorf("Expected 1 file entry after filtering empty paths, got %d", len(m.changedFiles))
	}
	if len(m.changedFiles) > 0 && m.changedFiles[0].Item.Path != "/src/main.go" {
		t.Errorf("Expected /src/main.go, got %q", m.changedFiles[0].Item.Path)
	}
}
