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
		ID:         101,
		Title:      "Test PR",
		Repository: azdevops.Repository{ID: "repo-123"},
	}
	model := NewDetailModel(nil, pr)
	model.SetSize(80, 24)

	threads := []azdevops.Thread{
		{ID: 1, Status: "active", Comments: []azdevops.Comment{{ID: 1, Content: "1"}}},
		{ID: 2, Status: "active", Comments: []azdevops.Comment{{ID: 2, Content: "2"}}},
		{ID: 3, Status: "active", Comments: []azdevops.Comment{{ID: 3, Content: "3"}}},
		{ID: 4, Status: "active", Comments: []azdevops.Comment{{ID: 4, Content: "4"}}},
	}
	model.SetThreads(threads)

	// At start, scroll percent should be 0
	percent := model.GetScrollPercent()
	if percent != 0 {
		t.Errorf("Initial scroll percent = %f, want 0", percent)
	}

	// Move to end
	model.MoveDown()
	model.MoveDown()
	model.MoveDown()

	percent = model.GetScrollPercent()
	if percent != 100 {
		t.Errorf("End scroll percent = %f, want 100", percent)
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
