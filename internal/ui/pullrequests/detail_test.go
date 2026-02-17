package pullrequests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDetailModel_ViewportUsesFullAvailableHeight(t *testing.T) {
	// The height passed to SetSize is already the content area (after app-level
	// borders and footer are subtracted). The PR detail view should only subtract
	// its own header lines (title + branch + separator = 3 lines).
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

	// Create enough threads to fill viewport
	threads := make([]azdevops.Thread, 30)
	for i := range threads {
		threads[i] = azdevops.Thread{
			ID: i + 1, Status: "active",
			Comments: []azdevops.Comment{
				{ID: i + 1, Content: fmt.Sprintf("Comment %d", i+1), Author: azdevops.Identity{DisplayName: "User"}},
			},
		}
	}
	model.SetThreads(threads)

	view := model.View()
	lines := strings.Split(view, "\n")

	// Total output lines should equal the height passed in.
	if len(lines) != height {
		t.Errorf("PR detail view output has %d lines, want %d (height passed to SetSize). "+
			"Viewport is not using full available height.", len(lines), height)
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

			// Test reviewer vote icons
			if !strings.Contains(reviewerVoteIconWithStyles(10, s), "✓") {
				t.Error("reviewerVoteIconWithStyles(10) should contain ✓")
			}
			if !strings.Contains(reviewerVoteIconWithStyles(-10, s), "✗") {
				t.Error("reviewerVoteIconWithStyles(-10) should contain ✗")
			}

			// Test thread status icons
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

	// Model should be ready after SetSize
	if model.width != 80 {
		t.Errorf("Width = %d, want 80", model.width)
	}
	if model.height != 24 {
		t.Errorf("Height = %d, want 24", model.height)
	}
}

func TestDetailModel_SetThreads(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 1, Content: "First comment"},
			},
		},
		{
			ID:     2,
			Status: "fixed",
			Comments: []azdevops.Comment{
				{ID: 2, Content: "Second comment"},
			},
		},
	}

	model.SetThreads(threads)

	if len(model.threads) != 2 {
		t.Errorf("Threads length = %d, want 2", len(model.threads))
	}
}

func TestDetailModel_SetThreads_FiltersSystemComments(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 1, Content: "This looks good!", CommentType: "text"},
			},
		},
		{
			ID:     2,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 2, Content: "Microsoft.VisualStudio.Services.CodeReview.PolicyViolation", CommentType: "system"},
			},
		},
		{
			ID:     3,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 3, Content: "Please fix this issue", CommentType: "text"},
			},
		},
	}

	model.SetThreads(threads)

	// Should filter out the Microsoft.VisualStudio system comment
	if len(model.threads) != 2 {
		t.Errorf("Threads length = %d, want 2 (system comments should be filtered)", len(model.threads))
	}

	// Verify the correct threads are kept
	if model.threads[0].ID != 1 {
		t.Errorf("threads[0].ID = %d, want 1", model.threads[0].ID)
	}
	if model.threads[1].ID != 3 {
		t.Errorf("threads[1].ID = %d, want 3", model.threads[1].ID)
	}
}

func TestDetailModel_Navigation(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
		Reviewers: []azdevops.Reviewer{
			{ID: "1", DisplayName: "User 1", Vote: 10},
			{ID: "2", DisplayName: "User 2", Vote: 0},
		},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{ID: 1, Status: "active", Comments: []azdevops.Comment{{ID: 1, Content: "Comment 1"}}},
		{ID: 2, Status: "fixed", Comments: []azdevops.Comment{{ID: 2, Content: "Comment 2"}}},
		{ID: 3, Status: "active", Comments: []azdevops.Comment{{ID: 3, Content: "Comment 3"}}},
	}
	model.SetThreads(threads)

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

	// Should contain PR title
	if !strings.Contains(view, "Add new feature") {
		t.Error("View should contain PR title")
	}

	// Should contain description
	if !strings.Contains(view, "This is a test description") {
		t.Error("View should contain PR description")
	}

	// Should contain reviewer name
	if !strings.Contains(view, "Jane Smith") {
		t.Error("View should contain reviewer name")
	}

	// Should contain vote description (not just the number)
	if !strings.Contains(view, "Approved") {
		t.Error("View should contain vote description 'Approved' for vote 10")
	}
}

func TestDetailModel_View_WithThreads(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{
			ID:            1,
			Status:        "active",
			PublishedDate: time.Now(),
			Comments: []azdevops.Comment{
				{ID: 1, Content: "This looks good!", Author: azdevops.Identity{DisplayName: "Reviewer"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should contain comment content
	if !strings.Contains(view, "This looks good!") {
		t.Error("View should contain comment content")
	}
}

func TestDetailModel_GetContextItems(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)

	items := model.GetContextItems()

	if len(items) == 0 {
		t.Error("Detail view should have context items")
	}
}

func TestDetailModel_GetScrollPercent(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:          101,
		Title:       "Test PR",
		Description: "A description that takes some space",
		Repository:  azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	// Use small viewport to force scrolling
	model.SetSize(80, 10)

	// Create many threads to overflow viewport
	threads := make([]azdevops.Thread, 30)
	for i := 0; i < 30; i++ {
		threads[i] = azdevops.Thread{
			ID:     i + 1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: i + 1, Content: fmt.Sprintf("Comment %d", i+1)},
			},
		}
	}
	model.SetThreads(threads)

	// At start, scroll percent should be 0 (at top of viewport)
	percent := model.GetScrollPercent()
	if percent != 0 {
		t.Errorf("Initial scroll percent = %f, want 0", percent)
	}

	// Page down multiple times to reach bottom
	for i := 0; i < 10; i++ {
		model.PageDown()
	}

	// Scroll percent should be higher after scrolling down
	percent = model.GetScrollPercent()
	if percent <= 0 {
		t.Errorf("After scrolling down, percent = %f, want > 0", percent)
	}
}

func TestDetailModel_SelectedThread(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{ID: 1, Status: "active", Comments: []azdevops.Comment{{ID: 1, Content: "First"}}},
		{ID: 2, Status: "fixed", Comments: []azdevops.Comment{{ID: 2, Content: "Second"}}},
	}
	model.SetThreads(threads)

	// Select first thread
	selected := model.SelectedThread()
	if selected == nil {
		t.Fatal("SelectedThread should not be nil")
	}
	if selected.ID != 1 {
		t.Errorf("SelectedThread ID = %d, want 1", selected.ID)
	}

	// Move to second thread
	model.MoveDown()
	selected = model.SelectedThread()
	if selected == nil {
		t.Fatal("SelectedThread should not be nil after move")
	}
	if selected.ID != 2 {
		t.Errorf("SelectedThread ID = %d, want 2", selected.ID)
	}
}

func TestDetailModel_EmptyThreads(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	// No threads
	model.SetThreads([]azdevops.Thread{})

	selected := model.SelectedThread()
	if selected != nil {
		t.Error("SelectedThread should be nil when no threads")
	}
}

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

var errMockDetail = fmt.Errorf("mock error")

func TestDetailModel_ViewportScrolling(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	// Set a small viewport height to force scrolling
	model.SetSize(80, 15) // Small height

	// Create many threads to overflow the viewport
	threads := make([]azdevops.Thread, 20)
	for i := 0; i < 20; i++ {
		threads[i] = azdevops.Thread{
			ID:     i + 1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: i + 1, Content: fmt.Sprintf("Comment number %d", i+1), Author: azdevops.Identity{DisplayName: "User"}},
			},
		}
	}
	model.SetThreads(threads)

	// Verify model is ready for viewport operations
	if !model.ready {
		t.Fatal("Model should be ready after SetSize")
	}

	// Initial state
	if model.SelectedIndex() != 0 {
		t.Errorf("Initial SelectedIndex = %d, want 0", model.SelectedIndex())
	}

	// Page down should scroll or move selection forward
	// With card-style threads (border + header + comments + border), each thread takes 4+ lines
	// HalfViewDown moves the viewport; selection updates based on what's visible
	initialYOffset := model.viewport.YOffset
	model.PageDown()
	// Either the viewport should scroll or selection should change (or we're at the end)
	if model.viewport.YOffset == initialYOffset && model.SelectedIndex() == 0 {
		// This is acceptable if viewport can show all content
		t.Log("PageDown didn't scroll - viewport may show all content")
	}

	// Move to near the end
	for i := 0; i < 15; i++ {
		model.MoveDown()
	}

	// Should be able to scroll to items at the end
	if model.SelectedIndex() < 10 {
		t.Errorf("After multiple MoveDown, SelectedIndex = %d, want >= 10", model.SelectedIndex())
	}

	// View should still render without error and contain content
	view := model.View()
	if view == "" {
		t.Error("View should not be empty after scrolling")
	}

	// The view should use viewport (contain the selected item's content)
	// Note: This tests that the viewport is rendering properly
	selected := model.SelectedThread()
	if selected == nil {
		t.Fatal("Should have a selected thread")
	}
}

func TestDetailModel_ViewportEnsuresSelectedVisible(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 12) // Very small viewport

	// Create threads
	threads := make([]azdevops.Thread, 15)
	for i := 0; i < 15; i++ {
		threads[i] = azdevops.Thread{
			ID:     i + 1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: i + 1, Content: fmt.Sprintf("Thread %d", i+1), Author: azdevops.Identity{DisplayName: "User"}},
			},
		}
	}
	model.SetThreads(threads)

	// Move to an item that's beyond the initial viewport
	for i := 0; i < 10; i++ {
		model.MoveDown()
	}

	// After scrolling, viewport should have adjusted
	if model.viewport.YOffset == 0 && model.SelectedIndex() > 5 {
		// If selected item is beyond viewport but YOffset is still 0,
		// the scrolling isn't working
		t.Error("Viewport YOffset should have changed to keep selected item visible")
	}
}

func TestDetailModel_View_ShowsFilePathForCodeComments(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 42, Offset: 1},
			},
			Comments: []azdevops.Comment{
				{ID: 1, Content: "This looks good!", Author: azdevops.Identity{DisplayName: "Reviewer"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should show shortened file path (last 2 segments)
	if !strings.Contains(view, "../src/main.go") {
		t.Error("View should contain shortened file path for code comments")
	}
	// Should show line number
	if !strings.Contains(view, "42") {
		t.Error("View should contain line number for code comments")
	}
}

func TestDetailModel_View_ShowsAllCommentsInThread(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 1, ParentCommentID: 0, Content: "Please fix this", Author: azdevops.Identity{DisplayName: "Reviewer"}},
				{ID: 2, ParentCommentID: 1, Content: "Done, fixed it", Author: azdevops.Identity{DisplayName: "Author"}},
				{ID: 3, ParentCommentID: 1, Content: "Thanks!", Author: azdevops.Identity{DisplayName: "Reviewer"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should show all comments, not just the first one
	if !strings.Contains(view, "Please fix this") {
		t.Error("View should contain first comment")
	}
	if !strings.Contains(view, "Done, fixed it") {
		t.Error("View should contain reply comment")
	}
	if !strings.Contains(view, "Thanks!") {
		t.Error("View should contain second reply comment")
	}
}

func TestDetailModel_View_ShowsThreadStatus(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "fixed",
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Issue resolved", Author: azdevops.Identity{DisplayName: "Reviewer"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should show resolved status
	if !strings.Contains(view, "✓") {
		t.Error("View should contain resolved status icon for fixed threads")
	}
}

func TestDetailModel_View_ShowsStatusTextForAllStatuses(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}

	tests := []struct {
		status   string
		wantText string
	}{
		{"active", "Active"},
		{"fixed", "Resolved"},
		{"closed", "Closed"},
		{"wontFix", "Won't fix"},
		{"pending", "Pending"},
		{"", "Unknown"},
		{"unknown_status", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			model := NewDetailModel(nil, pr)
			model.SetSize(100, 40)

			threads := []azdevops.Thread{
				{
					ID:     1,
					Status: tt.status,
					Comments: []azdevops.Comment{
						{ID: 1, Content: "Test comment", Author: azdevops.Identity{DisplayName: "User"}},
					},
				},
			}
			model.SetThreads(threads)

			view := model.View()
			if !strings.Contains(view, tt.wantText) {
				t.Errorf("View should contain status text %q for status %q", tt.wantText, tt.status)
			}
		})
	}
}

func TestDetailModel_View_ShowsGoToPRLink(t *testing.T) {
	// Create a mock client to provide org/project info
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

	// Should contain "Go to PR" text
	if !strings.Contains(view, "Go to PR") {
		t.Error("View should contain 'Go to PR' link text")
	}
}

func TestDetailModel_View_HandlesLineBreaksInComments(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Line one\nLine two\nLine three", Author: azdevops.Identity{DisplayName: "User"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Line breaks should be replaced with spaces
	if strings.Contains(view, "Line one\n") {
		t.Error("View should not contain raw line breaks in comment content")
	}
	// Content should still be present
	if !strings.Contains(view, "Line one") || !strings.Contains(view, "Line two") {
		t.Error("View should contain comment content")
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
		{
			name:     "returns empty when org is missing",
			org:      "",
			project:  "myproject",
			repoID:   "repo-guid",
			prID:     123,
			expected: "",
		},
		{
			name:     "returns empty when project is missing",
			org:      "myorg",
			project:  "",
			repoID:   "repo-guid",
			prID:     123,
			expected: "",
		},
		{
			name:     "returns empty when repoID is missing",
			org:      "myorg",
			project:  "myproject",
			repoID:   "",
			prID:     123,
			expected: "",
		},
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
		{
			name:     "returns empty when org is missing",
			org:      "",
			project:  "myproject",
			repoID:   "repo-guid",
			prID:     123,
			threadID: 456,
			expected: "",
		},
		{
			name:     "returns empty when project is missing",
			org:      "myorg",
			project:  "",
			repoID:   "repo-guid",
			prID:     123,
			threadID: 456,
			expected: "",
		},
		{
			name:     "returns empty when repoID is missing",
			org:      "myorg",
			project:  "myproject",
			repoID:   "",
			prID:     123,
			threadID: 456,
			expected: "",
		},
		{
			name:     "returns empty when threadID is zero",
			org:      "myorg",
			project:  "myproject",
			repoID:   "repo-guid",
			prID:     123,
			threadID: 0,
			expected: "",
		},
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
		{
			name:     "ASCII string within limit",
			input:    "hello",
			maxRunes: 10,
			expected: "hello",
		},
		{
			name:     "ASCII string exceeds limit",
			input:    "hello world",
			maxRunes: 5,
			expected: "hello",
		},
		{
			name:     "Unicode string within limit",
			input:    "héllo wörld",
			maxRunes: 15,
			expected: "héllo wörld",
		},
		{
			name:     "Unicode string exceeds limit",
			input:    "héllo wörld",
			maxRunes: 5,
			expected: "héllo",
		},
		{
			name:     "Swedish characters",
			input:    "uppdateras här",
			maxRunes: 10,
			expected: "uppdateras",
		},
		{
			name:     "empty string",
			input:    "",
			maxRunes: 5,
			expected: "",
		},
		{
			name:     "zero max runes",
			input:    "hello",
			maxRunes: 0,
			expected: "",
		},
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
		{
			name:     "long path gets shortened",
			input:    "/Services/UnitService/Extensions/UnitService.cs",
			expected: "../Extensions/UnitService.cs",
		},
		{
			name:     "path with 3 segments",
			input:    "/src/main/App.go",
			expected: "../main/App.go",
		},
		{
			name:     "path with 2 segments",
			input:    "/src/main.go",
			expected: "../src/main.go",
		},
		{
			name:     "path with 1 segment (just filename at root)",
			input:    "/main.go",
			expected: "main.go",
		},
		{
			name:     "simple filename",
			input:    "main.go",
			expected: "main.go",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "path with many segments",
			input:    "/a/b/c/d/e/f/g.txt",
			expected: "../f/g.txt",
		},
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

func TestDetailModel_GetContextItems_NoApproveReject(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)

	items := model.GetContextItems()

	// Should have context items for navigation and back
	if len(items) == 0 {
		t.Fatal("Detail view should have context items")
	}

	// Should NOT have approve or reject actions
	for _, item := range items {
		if item.Description == "approve" {
			t.Error("Context items should not include 'approve' - view should be read-only")
		}
		if item.Description == "reject" {
			t.Error("Context items should not include 'reject' - view should be read-only")
		}
		if item.Key == "a" {
			t.Error("Context items should not include 'a' key for approve")
		}
		if item.Key == "x" {
			t.Error("Context items should not include 'x' key for reject")
		}
	}
}

func TestDetailModel_View_ShowsShortenedFilePath(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 40)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/Services/UnitService/Extensions/UnitService.cs",
				RightFileStart: &azdevops.FilePosition{Line: 91, Offset: 1},
			},
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Please review", Author: azdevops.Identity{DisplayName: "Reviewer"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should show shortened path (last 2 segments)
	if !strings.Contains(view, "../Extensions/UnitService.cs:91") {
		t.Error("View should contain shortened file path '../Extensions/UnitService.cs:91'")
	}

	// Should NOT show full path
	if strings.Contains(view, "/Services/UnitService/Extensions/UnitService.cs") {
		t.Error("View should NOT contain full file path")
	}
}

func TestDetailModel_View_HasVisualSeparationBetweenThreads(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 50)

	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: 1, Content: "First thread comment", Author: azdevops.Identity{DisplayName: "User1"}},
			},
		},
		{
			ID:     2,
			Status: "fixed",
			Comments: []azdevops.Comment{
				{ID: 2, Content: "Second thread comment", Author: azdevops.Identity{DisplayName: "User2"}},
			},
		},
	}
	model.SetThreads(threads)

	view := model.View()

	// Should have visual separator between threads (horizontal line)
	if !strings.Contains(view, "─") {
		t.Error("View should contain horizontal separator lines between threads")
	}
}

func TestDetailModel_GetThreadLineCount(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)

	tests := []struct {
		name          string
		thread        azdevops.Thread
		expectedLines int
	}{
		{
			name: "thread with 1 comment",
			thread: azdevops.Thread{
				ID:     1,
				Status: "active",
				Comments: []azdevops.Comment{
					{ID: 1, Content: "Single comment"},
				},
			},
			expectedLines: 3, // header + 1 comment + blank line
		},
		{
			name: "thread with 3 comments",
			thread: azdevops.Thread{
				ID:     2,
				Status: "fixed",
				Comments: []azdevops.Comment{
					{ID: 1, Content: "Comment 1"},
					{ID: 2, Content: "Comment 2"},
					{ID: 3, Content: "Comment 3"},
				},
			},
			expectedLines: 5, // header + 3 comments + blank line
		},
		{
			name: "thread with no comments",
			thread: azdevops.Thread{
				ID:       3,
				Status:   "active",
				Comments: []azdevops.Comment{},
			},
			expectedLines: 2, // header + blank line
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.getThreadLineCount(tt.thread)
			if got != tt.expectedLines {
				t.Errorf("getThreadLineCount() = %d, want %d", got, tt.expectedLines)
			}
		})
	}
}

func TestDetailModel_GetSelectedThreadLineOffset(t *testing.T) {
	pr := azdevops.PullRequest{ID: 101, Title: "Test PR"}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	// Create threads with varying comment counts
	// Each thread has: header + comments + blank line
	threads := []azdevops.Thread{
		{ID: 1, Status: "active", Comments: []azdevops.Comment{{ID: 1, Content: "Comment 1"}}},                // 3 lines (header + 1 comment + blank)
		{ID: 2, Status: "fixed", Comments: []azdevops.Comment{{ID: 2, Content: "Comment 2"}}},                 // 3 lines
		{ID: 3, Status: "active", Comments: []azdevops.Comment{{ID: 3, Content: "A"}, {ID: 4, Content: "B"}}}, // 4 lines (header + 2 comments + blank)
	}
	model.SetThreads(threads)

	// Thread section starts at line 1 (just the "Comments (3)" header since no description/reviewers)
	// Thread 0: lines 1-3 (header + comment + blank)
	// Thread 1: lines 4-6 (header + comment + blank)
	// Thread 2: lines 7-10 (header + 2 comments + blank)

	tests := []struct {
		selectedIndex  int
		expectedOffset int
	}{
		{0, 1}, // Thread 0 starts at line 1
		{1, 4}, // Thread 1 starts at line 4 (after thread 0: 1 + 3 = 4)
		{2, 7}, // Thread 2 starts at line 7 (after thread 1: 4 + 3 = 7)
	}

	for _, tt := range tests {
		model.selectedIndex = tt.selectedIndex
		got := model.getSelectedThreadLineOffset()
		if got != tt.expectedOffset {
			t.Errorf("getSelectedThreadLineOffset() for index %d = %d, want %d", tt.selectedIndex, got, tt.expectedOffset)
		}
	}
}

func TestDetailModel_LargeThreadCount_Scrolling(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:          101,
		Title:       "Test PR with many comments",
		Description: "A test description",
		Repository:  azdevops.Repository{ID: "repo-123"},
		Reviewers: []azdevops.Reviewer{
			{ID: "1", DisplayName: "Reviewer 1", Vote: 10},
			{ID: "2", DisplayName: "Reviewer 2", Vote: 0},
		},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(100, 30) // Reasonable viewport

	// Create 130 threads with varying comment counts (simulating real PR)
	threads := make([]azdevops.Thread, 130)
	for i := 0; i < 130; i++ {
		commentCount := (i % 3) + 1 // 1-3 comments per thread
		comments := make([]azdevops.Comment, commentCount)
		for j := 0; j < commentCount; j++ {
			comments[j] = azdevops.Comment{
				ID:      i*10 + j + 1,
				Content: fmt.Sprintf("Comment %d in thread %d", j+1, i+1),
				Author:  azdevops.Identity{DisplayName: "User"},
			}
		}
		threads[i] = azdevops.Thread{
			ID:       i + 1,
			Status:   []string{"active", "fixed", "pending"}[i%3],
			Comments: comments,
		}
	}
	model.SetThreads(threads)

	// Test 1: Scroll down through all threads
	for i := 0; i < 129; i++ {
		prevIndex := model.SelectedIndex()
		model.MoveDown()
		newIndex := model.SelectedIndex()

		// Selection should always increase by 1
		if newIndex != prevIndex+1 {
			t.Errorf("MoveDown at index %d: got %d, want %d", prevIndex, newIndex, prevIndex+1)
		}

		// Selection should always be valid
		if newIndex < 0 || newIndex >= 130 {
			t.Errorf("Invalid selection index after MoveDown: %d", newIndex)
		}
	}

	// Should be at the last thread now
	if model.SelectedIndex() != 129 {
		t.Errorf("After scrolling to end, SelectedIndex = %d, want 129", model.SelectedIndex())
	}

	// Test 2: Scroll back up through all threads
	for i := 0; i < 129; i++ {
		prevIndex := model.SelectedIndex()
		model.MoveUp()
		newIndex := model.SelectedIndex()

		// Selection should always decrease by 1
		if newIndex != prevIndex-1 {
			t.Errorf("MoveUp at index %d: got %d, want %d", prevIndex, newIndex, prevIndex-1)
		}
	}

	// Should be at the first thread now
	if model.SelectedIndex() != 0 {
		t.Errorf("After scrolling to start, SelectedIndex = %d, want 0", model.SelectedIndex())
	}

	// Test 3: PageDown multiple times
	for i := 0; i < 10; i++ {
		prevIndex := model.SelectedIndex()
		model.PageDown()
		// PageDown should move selection forward (or stay if at end)
		if model.SelectedIndex() < prevIndex && prevIndex < 129 {
			t.Errorf("PageDown decreased selection from %d to %d", prevIndex, model.SelectedIndex())
		}
	}

	// Test 4: PageUp multiple times
	for i := 0; i < 10; i++ {
		prevIndex := model.SelectedIndex()
		model.PageUp()
		// PageUp should move selection backward (or stay if at start)
		if model.SelectedIndex() > prevIndex && prevIndex > 0 {
			t.Errorf("PageUp increased selection from %d to %d", prevIndex, model.SelectedIndex())
		}
	}

	// Test 5: View should always render without panic
	view := model.View()
	if view == "" {
		t.Error("View should not be empty")
	}
}

func TestDetailModel_ScrollPreservesViewportPosition(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 20)

	// Create 50 threads
	threads := make([]azdevops.Thread, 50)
	for i := 0; i < 50; i++ {
		threads[i] = azdevops.Thread{
			ID:     i + 1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: i + 1, Content: fmt.Sprintf("Comment %d", i+1), Author: azdevops.Identity{DisplayName: "User"}},
			},
		}
	}
	model.SetThreads(threads)

	// Move to middle of list
	for i := 0; i < 25; i++ {
		model.MoveDown()
	}

	// Get current viewport position
	initialOffset := model.viewport.YOffset

	// Move down one more - should only scroll if necessary
	model.MoveDown()
	newOffset := model.viewport.YOffset

	// Viewport should not jump dramatically (at most by thread height)
	maxJump := 10 // reasonable max for a thread with comments
	if newOffset-initialOffset > maxJump {
		t.Errorf("Viewport jumped too much: from %d to %d (diff %d)", initialOffset, newOffset, newOffset-initialOffset)
	}
}

func TestDetailModel_PageUpDown(t *testing.T) {
	pr := azdevops.PullRequest{
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 12) // Small viewport to test scrolling

	// Create threads
	threads := make([]azdevops.Thread, 30)
	for i := 0; i < 30; i++ {
		threads[i] = azdevops.Thread{
			ID:     i + 1,
			Status: "active",
			Comments: []azdevops.Comment{
				{ID: i + 1, Content: fmt.Sprintf("Thread %d", i+1)},
			},
		}
	}
	model.SetThreads(threads)

	// Page down should scroll the viewport
	initialYOffset := model.viewport.YOffset
	model.PageDown()
	afterPageDownYOffset := model.viewport.YOffset

	if afterPageDownYOffset <= initialYOffset {
		t.Errorf("PageDown should scroll viewport down, YOffset: %d -> %d", initialYOffset, afterPageDownYOffset)
	}

	// Page up should scroll back
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

	// Selection should still work
	if model.SelectedIndex() < 0 || model.SelectedIndex() >= len(threads) {
		t.Errorf("SelectedIndex %d should be valid", model.SelectedIndex())
	}
}
