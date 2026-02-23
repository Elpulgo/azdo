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
	m := newTestDiffModel()

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
			icon, _ := m.changeTypeDisplay(tt.changeType)
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

	// Should have navigate, open, refresh, back
	hasNavigate := false
	hasBack := false
	for _, item := range items {
		if item.Description == "navigate" {
			hasNavigate = true
		}
		if item.Description == "back" {
			hasBack = true
		}
	}
	if !hasNavigate {
		t.Error("Missing 'navigate' context item")
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

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.viewMode != DiffFileList {
		t.Errorf("After esc from diff view, viewMode = %d, want DiffFileList", m.viewMode)
	}
	if m.currentFile != nil {
		t.Error("After esc from diff view, currentFile should be nil")
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
