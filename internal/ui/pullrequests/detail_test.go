package pullrequests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDetailModel_ViewportUsesFullAvailableHeight(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:            101,
		Title:         "Test PR",
		SourceRefName: "refs/heads/feature/test",
		TargetRefName: "refs/heads/main",
		Repository:    azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)

	height := 30
	model.SetSize(80, height)

	// Create enough files to fill viewport
	files := make([]azdevops.IterationChange, 30)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	model.SetChangedFiles(files)

	view := model.View()
	lines := strings.Split(view, "\n")

	if len(lines) != height {
		t.Errorf("PR detail view output has %d lines, want %d (height passed to SetSize)", len(lines), height)
	}
}

func TestDetailModel_HasStyles(t *testing.T) {
	pr := azdevops.PullRequest{ID: 123, Title: "Test"}
	m := NewDetailModel(nil, pr)

	if m.styles == nil {
		t.Error("Expected detail model to have styles initialized")
	}
}

func TestDetailModel_WithStyles(t *testing.T) {
	pr := azdevops.PullRequest{ID: 123, Title: "Test"}
	customStyles := styles.NewStyles(styles.GetThemeByNameWithFallback("nord"))
	m := NewDetailModelWithStyles(nil, pr, customStyles)

	if m.styles != customStyles {
		t.Error("Expected detail model to use provided custom styles")
	}
}

func TestDetailIconsWithStyles(t *testing.T) {
	themes := []string{"dark", "gruvbox", "nord", "dracula"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			if !strings.Contains(reviewerVoteIconWithStyles(10, s), "✓") {
				t.Error("reviewerVoteIconWithStyles(10) should contain ✓")
			}
			if !strings.Contains(reviewerVoteIconWithStyles(-10, s), "✗") {
				t.Error("reviewerVoteIconWithStyles(-10) should contain ✗")
			}

			if !strings.Contains(threadStatusIconWithStyles("active", s), "●") {
				t.Error("threadStatusIconWithStyles('active') should contain ●")
			}
			if !strings.Contains(threadStatusIconWithStyles("fixed", s), "✓") {
				t.Error("threadStatusIconWithStyles('fixed') should contain ✓")
			}
		})
	}
}

func TestNewDetailModel(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:            101,
		Title:         "Test PR",
		Description:   "Test description",
		Status:        "active",
		SourceRefName: "refs/heads/feature/test",
		TargetRefName: "refs/heads/main",
		CreatedBy:     azdevops.Identity{DisplayName: "John Doe"},
		Repository:    azdevops.Repository{ID: "repo-123", Name: "my-repo"},
	}

	model := NewDetailModel(nil, pr)

	if model.GetPR().ID != 101 {
		t.Errorf("Model PR ID = %d, want 101", model.GetPR().ID)
	}
}

func TestDetailModel_SetSize(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)

	model.SetSize(80, 24)

	if model.width != 80 {
		t.Errorf("Width = %d, want 80", model.width)
	}
	if model.height != 24 {
		t.Errorf("Height = %d, want 24", model.height)
	}
}

func TestDetailModel_SetChangedFiles(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/new.go"}, ChangeType: "add"},
	}

	model.SetChangedFiles(files)

	if len(model.changedFiles) != 2 {
		t.Errorf("Changed files length = %d, want 2", len(model.changedFiles))
	}
}

func TestDetailModel_SetChangedFiles_FiltersTreeEntries(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/", GitObjectType: "tree"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/main.go", GitObjectType: "blob"}, ChangeType: "edit"},
		{ChangeID: 3, Item: azdevops.ChangeItem{Path: "/src", GitObjectType: "tree"}, ChangeType: "edit"},
	}

	model.SetChangedFiles(files)

	if len(model.changedFiles) != 1 {
		t.Errorf("Changed files length = %d, want 1 (tree entries should be filtered)", len(model.changedFiles))
	}
}

func TestDetailModel_FileNavigation(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/a.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/b.go"}, ChangeType: "add"},
		{ChangeID: 3, Item: azdevops.ChangeItem{Path: "/src/c.go"}, ChangeType: "delete"},
	}
	model.SetChangedFiles(files)

	// Initial selection should be 0
	if model.SelectedIndex() != 0 {
		t.Errorf("Initial SelectedIndex = %d, want 0", model.SelectedIndex())
	}

	// Move down
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if model.SelectedIndex() != 1 {
		t.Errorf("After down, SelectedIndex = %d, want 1", model.SelectedIndex())
	}

	// Move up
	model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if model.SelectedIndex() != 0 {
		t.Errorf("After up, SelectedIndex = %d, want 0", model.SelectedIndex())
	}

	// Can't go above 0
	model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if model.SelectedIndex() != 0 {
		t.Errorf("After up at top, SelectedIndex = %d, want 0", model.SelectedIndex())
	}
}

func TestDetailModel_FileNavigation_JK(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/a.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/b.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if model.SelectedIndex() != 1 {
		t.Errorf("After j, SelectedIndex = %d, want 1", model.SelectedIndex())
	}

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if model.SelectedIndex() != 0 {
		t.Errorf("After k, SelectedIndex = %d, want 0", model.SelectedIndex())
	}
}

func TestDetailModel_SelectedFile(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/new.go"}, ChangeType: "add"},
	}
	model.SetChangedFiles(files)

	selected := model.SelectedFile()
	if selected == nil {
		t.Fatal("SelectedFile should not be nil")
	}
	if selected.Item.Path != "/src/main.go" {
		t.Errorf("SelectedFile path = %q, want /src/main.go", selected.Item.Path)
	}

	model.MoveDown()
	selected = model.SelectedFile()
	if selected == nil {
		t.Fatal("SelectedFile should not be nil after move")
	}
	if selected.Item.Path != "/src/new.go" {
		t.Errorf("SelectedFile path = %q, want /src/new.go", selected.Item.Path)
	}
}

func TestDetailModel_EmptyFiles(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	model.SetChangedFiles([]azdevops.IterationChange{})

	selected := model.SelectedFile()
	if selected != nil {
		t.Error("SelectedFile should be nil when no files")
	}
}

func TestDetailModel_View_Loading(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.loading = true
	model.spinner.SetVisible(true)

	view := model.View()

	if !strings.Contains(view, "Loading") {
		t.Error("Loading view should contain 'Loading'")
	}
}

var errMockDetail = fmt.Errorf("mock error")

func TestDetailModel_View_Error(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.err = errMockDetail

	view := model.View()

	if !strings.Contains(view, "Error") {
		t.Error("Error view should contain 'Error'")
	}
}

func TestDetailModel_View_WithContent(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:            101,
		Title:         "Add new feature",
		Description:   "This is a test description",
		Status:        "active",
		SourceRefName: "refs/heads/feature/test",
		TargetRefName: "refs/heads/main",
		CreatedBy:     azdevops.Identity{DisplayName: "John Doe"},
		Repository:    azdevops.Repository{ID: "repo-123", Name: "my-repo"},
		Reviewers: []azdevops.Reviewer{
			{ID: "1", DisplayName: "Jane Smith", Vote: 10},
		},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	view := model.View()

	if !strings.Contains(view, "Add new feature") {
		t.Error("View should contain PR title")
	}
	if !strings.Contains(view, "This is a test description") {
		t.Error("View should contain PR description")
	}
	if !strings.Contains(view, "Jane Smith") {
		t.Error("View should contain reviewer name")
	}
	if !strings.Contains(view, "Approved") {
		t.Error("View should contain vote description 'Approved' for vote 10")
	}
}

func TestDetailModel_View_ShowsChangedFiles(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/new.go"}, ChangeType: "add"},
	}
	model.SetChangedFiles(files)

	view := model.View()

	if !strings.Contains(view, "Changed files (2)") {
		t.Error("View should contain 'Changed files (2)' header")
	}
	if !strings.Contains(view, "/src/main.go") {
		t.Error("View should contain file path /src/main.go")
	}
	if !strings.Contains(view, "/src/new.go") {
		t.Error("View should contain file path /src/new.go")
	}
}

func TestDetailModel_View_ShowsCommentCounts(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/new.go"}, ChangeType: "add"},
	}
	model.SetChangedFiles(files)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Fix this"},
				{ID: 2, Content: "Will do", ParentCommentID: 1},
			},
		},
		{
			ID:     2,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 25},
			},
			Comments: []azdevops.Comment{
				{ID: 3, Content: "Also fix this"},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// /src/main.go should show (3) - 2 comments from thread 1 + 1 from thread 2
	if !strings.Contains(view, "(3)") {
		t.Error("View should contain comment count (3) for /src/main.go")
	}
}

func TestDetailModel_View_NoCommentCountForZero(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/clean.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	view := model.View()

	// Should not show a comment count indicator when there are no comments
	// The file line should be just the icon and path, no "(0)"
	if strings.Contains(view, "(0)") {
		t.Error("View should NOT show (0) comment count for files with no comments")
	}
}

func TestDetailModel_EnterEmitsOpenFileDiffMsg(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/new.go"}, ChangeType: "add"},
	}
	model.SetChangedFiles(files)

	// Select second file
	model.MoveDown()

	// Press enter
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Expected enter to produce a command")
	}

	msg := cmd()
	openMsg, ok := msg.(openFileDiffMsg)
	if !ok {
		t.Fatalf("Expected openFileDiffMsg, got %T", msg)
	}
	if openMsg.file.Item.Path != "/src/new.go" {
		t.Errorf("openFileDiffMsg file path = %q, want /src/new.go", openMsg.file.Item.Path)
	}
}

func TestDetailModel_EnterDoesNothingWithNoFiles(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)
	model.SetChangedFiles([]azdevops.IterationChange{})

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("Enter with no files should not produce a command")
	}
}

func TestDetailModel_View_ShowsGoToPRLink(t *testing.T) {
	client, _ := azdevops.NewClient("myorg", "myproject", "test-pat")

	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
		Reviewers: []azdevops.Reviewer{
			{ID: "1", DisplayName: "Reviewer 1", Vote: 10},
		},
	}
	model := NewDetailModel(client, pr)
	model.SetSize(100, 40)

	view := model.View()

	if !strings.Contains(view, "Go to PR") {
		t.Error("View should contain 'Go to PR' link text")
	}
}

func TestDetailModel_View_ShowsNoChangedFilesMessage(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	view := model.View()

	if !strings.Contains(view, "No changed files") {
		t.Error("View should contain 'No changed files' when file list is empty")
	}
}

func TestDetailModel_GetScrollPercent(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:          101,
		Title:       "Test PR",
		Description: "A description",
		Repository:  azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 10) // small viewport

	files := make([]azdevops.IterationChange, 30)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	model.SetChangedFiles(files)

	percent := model.GetScrollPercent()
	if percent != 0 {
		t.Errorf("Initial scroll percent = %f, want 0", percent)
	}

	for i := 0; i < 10; i++ {
		model.PageDown()
	}

	percent = model.GetScrollPercent()
	if percent <= 0 {
		t.Errorf("After scrolling down, percent = %f, want > 0", percent)
	}
}

func TestDetailModel_FileListScrolling(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 15) // small viewport

	files := make([]azdevops.IterationChange, 20)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	model.SetChangedFiles(files)

	if !model.ready {
		t.Fatal("Model should be ready after SetSize")
	}

	// Navigate to end
	for i := 0; i < 19; i++ {
		model.MoveDown()
	}

	if model.SelectedIndex() != 19 {
		t.Errorf("After scrolling to end, SelectedIndex = %d, want 19", model.SelectedIndex())
	}

	// Navigate back to start
	for i := 0; i < 19; i++ {
		model.MoveUp()
	}

	if model.SelectedIndex() != 0 {
		t.Errorf("After scrolling back, SelectedIndex = %d, want 0", model.SelectedIndex())
	}
}

func TestDetailModel_LargeFileList_Scrolling(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:          101,
		Title:       "Test PR with many files",
		Description: "A test description",
		Repository:  azdevops.Repository{ID: "repo-123"},
		Reviewers: []azdevops.Reviewer{
			{ID: "1", DisplayName: "Reviewer 1", Vote: 10},
		},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 30)

	files := make([]azdevops.IterationChange, 100)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: []string{"edit", "add", "delete"}[i%3],
		}
	}
	model.SetChangedFiles(files)

	// Scroll down through all files
	for i := 0; i < 99; i++ {
		prevIndex := model.SelectedIndex()
		model.MoveDown()
		if model.SelectedIndex() != prevIndex+1 {
			t.Errorf("MoveDown at index %d: got %d, want %d", prevIndex, model.SelectedIndex(), prevIndex+1)
		}
	}

	if model.SelectedIndex() != 99 {
		t.Errorf("After scrolling to end, SelectedIndex = %d, want 99", model.SelectedIndex())
	}

	// Scroll back up
	for i := 0; i < 99; i++ {
		model.MoveUp()
	}

	if model.SelectedIndex() != 0 {
		t.Errorf("After scrolling to start, SelectedIndex = %d, want 0", model.SelectedIndex())
	}

	// View should always render without panic
	view := model.View()
	if view == "" {
		t.Error("View should not be empty")
	}
}

func TestDetailModel_PageUpDown(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 12)

	files := make([]azdevops.IterationChange, 30)
	for i := range files {
		files[i] = azdevops.IterationChange{
			ChangeID:   i + 1,
			Item:       azdevops.ChangeItem{Path: fmt.Sprintf("/src/file%d.go", i)},
			ChangeType: "edit",
		}
	}
	model.SetChangedFiles(files)

	initialYOffset := model.viewport.YOffset
	model.PageDown()
	afterPageDownYOffset := model.viewport.YOffset

	if afterPageDownYOffset <= initialYOffset {
		t.Errorf("PageDown should scroll viewport down, YOffset: %d -> %d", initialYOffset, afterPageDownYOffset)
	}

	model.PageUp()
	afterPageUpYOffset := model.viewport.YOffset

	if afterPageUpYOffset >= afterPageDownYOffset {
		t.Errorf("PageUp should scroll viewport up, YOffset: %d -> %d", afterPageDownYOffset, afterPageUpYOffset)
	}

	// Page up at top should stay at top
	model.PageUp()
	model.PageUp()
	model.PageUp()
	if model.viewport.YOffset != 0 {
		t.Errorf("Multiple PageUp at top should result in YOffset 0, got %d", model.viewport.YOffset)
	}

	if model.SelectedIndex() < 0 || model.SelectedIndex() >= len(files) {
		t.Errorf("SelectedIndex %d should be valid", model.SelectedIndex())
	}
}

func TestDetailModel_View_ShowsChangeTypeIcons(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/added.go"}, ChangeType: "add"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/src/edited.go"}, ChangeType: "edit"},
		{ChangeID: 3, Item: azdevops.ChangeItem{Path: "/src/deleted.go"}, ChangeType: "delete"},
		{ChangeID: 4, Item: azdevops.ChangeItem{Path: "/src/renamed.go"}, ChangeType: "rename", OriginalPath: "/src/old_name.go"},
	}
	model.SetChangedFiles(files)

	view := model.View()

	if !strings.Contains(view, "+") {
		t.Error("View should contain '+' icon for added files")
	}
	if !strings.Contains(view, "~") {
		t.Error("View should contain '~' icon for edited files")
	}
	if !strings.Contains(view, "-") {
		t.Error("View should contain '-' icon for deleted files")
	}
	if !strings.Contains(view, "→") {
		t.Error("View should contain '→' icon for renamed files")
	}
}

func TestDetailModel_View_RenamedShowsBothPaths(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/new_name.go"}, ChangeType: "rename", OriginalPath: "/src/old_name.go"},
	}
	model.SetChangedFiles(files)

	view := model.View()

	if !strings.Contains(view, "/src/old_name.go") {
		t.Error("View should contain original path for renamed files")
	}
	if !strings.Contains(view, "/src/new_name.go") {
		t.Error("View should contain new path for renamed files")
	}
}

func TestDetailModel_GetThreads(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{ID: 1, Status: "active", Comments: []azdevops.Comment{{ID: 1, Content: "Comment"}}},
	}
	model.SetThreads(threads)

	got := model.GetThreads()
	if len(got) != 1 {
		t.Errorf("GetThreads() length = %d, want 1", len(got))
	}
}

func TestDetailModel_GetChangedFiles(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/a.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	got := model.GetChangedFiles()
	if len(got) != 1 {
		t.Errorf("GetChangedFiles() length = %d, want 1", len(got))
	}
}

// --- Helper function tests (unchanged) ---

func TestReviewerVoteIcon(t *testing.T) {
	tests := []struct {
		name         string
		vote         int
		wantContains string
	}{
		{name: "approved", vote: 10, wantContains: "✓"},
		{name: "approved with suggestions", vote: 5, wantContains: "~"},
		{name: "no vote", vote: 0, wantContains: "○"},
		{name: "waiting", vote: -5, wantContains: "◐"},
		{name: "rejected", vote: -10, wantContains: "✗"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reviewerVoteIconWithStyles(tt.vote, styles.DefaultStyles())
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("reviewerVoteIconWithStyles(%d) = %q, want to contain %q", tt.vote, got, tt.wantContains)
			}
		})
	}
}

func TestReviewerVoteDescription(t *testing.T) {
	tests := []struct {
		name     string
		vote     int
		expected string
	}{
		{name: "approved", vote: 10, expected: "Approved"},
		{name: "approved with suggestions", vote: 5, expected: "Approved with suggestions"},
		{name: "no vote", vote: 0, expected: "No vote"},
		{name: "waiting for author", vote: -5, expected: "Waiting for author"},
		{name: "rejected", vote: -10, expected: "Rejected"},
		{name: "unknown vote", vote: 99, expected: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reviewerVoteDescription(tt.vote)
			if got != tt.expected {
				t.Errorf("reviewerVoteDescription(%d) = %q, want %q", tt.vote, got, tt.expected)
			}
		})
	}
}

func TestThreadStatusIcon(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		wantContains string
	}{
		{name: "active", status: "active", wantContains: "●"},
		{name: "fixed", status: "fixed", wantContains: "✓"},
		{name: "wontFix", status: "wontFix", wantContains: "○"},
		{name: "closed", status: "closed", wantContains: "○"},
		{name: "pending", status: "pending", wantContains: "◐"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := threadStatusIconWithStyles(tt.status, styles.DefaultStyles())
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("threadStatusIconWithStyles(%q) = %q, want to contain %q", tt.status, got, tt.wantContains)
			}
		})
	}
}

func TestHyperlink(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		url      string
		expected string
	}{
		{
			name:     "creates OSC 8 hyperlink",
			text:     "Click me",
			url:      "https://example.com",
			expected: "\x1b]8;;https://example.com\x07Click me\x1b]8;;\x07",
		},
		{
			name:     "falls back to plain text when URL is empty",
			text:     "Plain text",
			url:      "",
			expected: "Plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hyperlink(tt.text, tt.url)
			if got != tt.expected {
				t.Errorf("hyperlink(%q, %q) = %q, want %q", tt.text, tt.url, got, tt.expected)
			}
		})
	}
}

func TestBuildPROverviewURL(t *testing.T) {
	tests := []struct {
		name     string
		org      string
		project  string
		repoID   string
		prID     int
		expected string
	}{
		{
			name:     "builds complete PR overview URL",
			org:      "myorg",
			project:  "myproject",
			repoID:   "repo-guid-123",
			prID:     123,
			expected: "https://dev.azure.com/myorg/myproject/_git/repo-guid-123/pullrequest/123",
		},
		{name: "returns empty when org is missing", org: "", project: "myproject", repoID: "repo-guid", prID: 123, expected: ""},
		{name: "returns empty when project is missing", org: "myorg", project: "", repoID: "repo-guid", prID: 123, expected: ""},
		{name: "returns empty when repoID is missing", org: "myorg", project: "myproject", repoID: "", prID: 123, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPROverviewURL(tt.org, tt.project, tt.repoID, tt.prID)
			if got != tt.expected {
				t.Errorf("buildPROverviewURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildPRThreadURL(t *testing.T) {
	tests := []struct {
		name     string
		org      string
		project  string
		repoID   string
		prID     int
		threadID int
		expected string
	}{
		{
			name:     "builds complete URL with discussionId",
			org:      "myorg",
			project:  "myproject",
			repoID:   "repo-guid-123",
			prID:     123,
			threadID: 456,
			expected: "https://dev.azure.com/myorg/myproject/_git/repo-guid-123/pullrequest/123?discussionId=456",
		},
		{name: "returns empty when org is missing", org: "", project: "myproject", repoID: "repo-guid", prID: 123, threadID: 456, expected: ""},
		{name: "returns empty when project is missing", org: "myorg", project: "", repoID: "repo-guid", prID: 123, threadID: 456, expected: ""},
		{name: "returns empty when repoID is missing", org: "myorg", project: "myproject", repoID: "", prID: 123, threadID: 456, expected: ""},
		{name: "returns empty when threadID is zero", org: "myorg", project: "myproject", repoID: "repo-guid", prID: 123, threadID: 0, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPRThreadURL(tt.org, tt.project, tt.repoID, tt.prID, tt.threadID)
			if got != tt.expected {
				t.Errorf("buildPRThreadURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		expected string
	}{
		{name: "ASCII string within limit", input: "hello", maxRunes: 10, expected: "hello"},
		{name: "ASCII string exceeds limit", input: "hello world", maxRunes: 5, expected: "hello"},
		{name: "Unicode string within limit", input: "héllo wörld", maxRunes: 15, expected: "héllo wörld"},
		{name: "Unicode string exceeds limit", input: "héllo wörld", maxRunes: 5, expected: "héllo"},
		{name: "Swedish characters", input: "uppdateras här", maxRunes: 10, expected: "uppdateras"},
		{name: "empty string", input: "", maxRunes: 5, expected: ""},
		{name: "zero max runes", input: "hello", maxRunes: 0, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxRunes)
			if got != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.expected)
			}
		})
	}
}

func TestShortenFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "long path gets shortened", input: "/Services/UnitService/Extensions/UnitService.cs", expected: "../Extensions/UnitService.cs"},
		{name: "path with 3 segments", input: "/src/main/App.go", expected: "../main/App.go"},
		{name: "path with 2 segments", input: "/src/main.go", expected: "../src/main.go"},
		{name: "path with 1 segment", input: "/main.go", expected: "main.go"},
		{name: "simple filename", input: "main.go", expected: "main.go"},
		{name: "empty path", input: "", expected: ""},
		{name: "path with many segments", input: "/a/b/c/d/e/f/g.txt", expected: "../f/g.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenFilePath(tt.input)
			if got != tt.expected {
				t.Errorf("shortenFilePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetailModel_View_ShowsGeneralComments(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	threads := []azdevops.Thread{
		{
			ID:            1,
			Status:        "active",
			ThreadContext: nil, // general comment
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Looks good overall", Author: azdevops.Identity{DisplayName: "Bob"}},
			},
		},
		{
			ID:     2,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
			Comments: []azdevops.Comment{
				{ID: 3, Content: "Fix this"},
			},
		},
		{
			ID:            3,
			Status:        "fixed",
			ThreadContext: nil, // resolved general comment
			Comments: []azdevops.Comment{
				{ID: 4, Content: "Add docs?", Author: azdevops.Identity{DisplayName: "Charlie"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should show general comments entry with count
	if !strings.Contains(view, "General comments (2)") {
		t.Error("View should contain 'General comments (2)' selectable entry")
	}
}

func TestDetailModel_View_NoGeneralCommentsSection(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	// Only code comments, no general comments
	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Fix this"},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should NOT show general comments section when there are none
	if strings.Contains(view, "General comments") {
		t.Error("View should NOT contain 'General comments' section when there are no general comments")
	}
}

func TestDetailModel_EnterOnGeneralCommentsEmitsMsg(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	threads := []azdevops.Thread{
		{
			ID:            1,
			Status:        "active",
			ThreadContext: nil,
			Comments:      []azdevops.Comment{{ID: 1, Content: "General comment"}},
		},
	}
	model.SetThreads(threads)

	// fileIndex should be 0 (general comments entry)
	if model.fileIndex != 0 {
		t.Fatalf("Initial fileIndex = %d, want 0", model.fileIndex)
	}

	// Press Enter — should emit openGeneralCommentsMsg
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Expected Enter to produce a command")
	}

	msg := cmd()
	if _, ok := msg.(openGeneralCommentsMsg); !ok {
		t.Errorf("Expected openGeneralCommentsMsg, got %T", msg)
	}
}

func TestDetailModel_NavigationWithGeneralComments(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/a.go"}, ChangeType: "edit"},
		{ChangeID: 2, Item: azdevops.ChangeItem{Path: "/b.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	threads := []azdevops.Thread{
		{ID: 1, ThreadContext: nil, Comments: []azdevops.Comment{{ID: 1, Content: "General"}}},
	}
	model.SetThreads(threads)

	// Index 0 = general comments, 1 = /a.go, 2 = /b.go
	if !model.isGeneralCommentsSelected() {
		t.Error("Initial selection should be general comments")
	}

	// Move down to first file
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if model.isGeneralCommentsSelected() {
		t.Error("After j, should not be on general comments")
	}
	if model.fileIndex != 1 {
		t.Errorf("After j, fileIndex = %d, want 1", model.fileIndex)
	}

	// Enter on a file should emit openFileDiffMsg
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Expected Enter to produce a command")
	}
	msg := cmd()
	if _, ok := msg.(openFileDiffMsg); !ok {
		t.Errorf("Expected openFileDiffMsg, got %T", msg)
	}
}

func TestDetailModel_VKeyOpensVotePicker(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	// Vote picker should be hidden initially
	if model.votePicker.IsVisible() {
		t.Error("Vote picker should be hidden initially")
	}

	// Press 'v' to open vote picker
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if cmd != nil {
		t.Error("Opening vote picker should not produce a command")
	}

	if !model.votePicker.IsVisible() {
		t.Error("Vote picker should be visible after pressing 'v'")
	}
}

func TestDetailModel_VotePickerRoutesInput(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	// Open vote picker
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})

	// While vote picker is visible, key input should route to it
	// Move cursor down in picker
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

	// File index should not change (input went to vote picker)
	if model.fileIndex != 0 {
		t.Errorf("fileIndex = %d, want 0 (input should route to vote picker)", model.fileIndex)
	}
}

func TestDetailModel_VotePickerEscCloses(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	// Open vote picker
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !model.votePicker.IsVisible() {
		t.Fatal("Vote picker should be visible")
	}

	// Press Esc to close
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if model.votePicker.IsVisible() {
		t.Error("Vote picker should be hidden after Esc")
	}
}

func TestDetailModel_VoteSelectedMsgTriggersVote(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	// Send VoteSelectedMsg directly
	model, cmd := model.Update(components.VoteSelectedMsg{Vote: azdevops.VoteApprove})

	if cmd == nil {
		t.Error("VoteSelectedMsg should produce a command")
	}

	if !model.loading {
		t.Error("Model should be in loading state after vote")
	}
}

func TestDetailModel_VotePRAllVoteTypes(t *testing.T) {
	tests := []struct {
		vote        int
		wantMessage string
	}{
		{azdevops.VoteApprove, "PR approved"},
		{azdevops.VoteApproveWithSuggestions, "PR approved with suggestions"},
		{azdevops.VoteWaitForAuthor, "Waiting for author"},
		{azdevops.VoteReject, "PR rejected"},
		{azdevops.VoteNoVote, "Vote reset"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("vote_%d", tt.vote), func(t *testing.T) {
			// voteResultDescription is tested separately since votePR
			// with nil client short-circuits before generating the message.
			got := voteResultDescription(tt.vote)
			if got != tt.wantMessage {
				t.Errorf("voteResultDescription(%d) = %q, want %q", tt.vote, got, tt.wantMessage)
			}
		})
	}
}

func TestDetailModel_ViewShowsVotePicker(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	files := []azdevops.IterationChange{
		{ChangeID: 1, Item: azdevops.ChangeItem{Path: "/src/main.go"}, ChangeType: "edit"},
	}
	model.SetChangedFiles(files)

	// Normal view should not show vote picker content
	normalView := model.View()

	// Open vote picker
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	voteView := model.View()

	// Vote picker view should be different from normal view
	if voteView == normalView {
		t.Error("View with vote picker should differ from normal view")
	}

	// Vote picker view should contain vote option text
	if !strings.Contains(voteView, "Approve") {
		t.Error("Vote picker view should contain 'Approve'")
	}
}

func TestChangeTypeDisplay(t *testing.T) {
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
