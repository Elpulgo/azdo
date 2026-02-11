package pullrequests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

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

	// Should contain reviewer
	if !strings.Contains(view, "Jane Smith") {
		t.Error("View should contain reviewer name")
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
			got := reviewerVoteIcon(tt.vote)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("reviewerVoteIcon(%d) = %q, want to contain %q", tt.vote, got, tt.wantContains)
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
			got := threadStatusIcon(tt.status)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("threadStatusIcon(%q) = %q, want to contain %q", tt.status, got, tt.wantContains)
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

	// Page down should move more than 1 item
	model.PageDown()
	if model.SelectedIndex() <= 1 {
		t.Errorf("After PageDown, SelectedIndex = %d, want > 1", model.SelectedIndex())
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
