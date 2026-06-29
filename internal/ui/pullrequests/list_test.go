package pullrequests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModelWithStyles(t *testing.T) {
	customStyles := styles.NewStyles(styles.GetThemeByNameWithFallback("gruvbox"))
	m := NewModelWithStyles(nil, customStyles)

	if m.styles != customStyles {
		t.Error("Expected model to use provided custom styles")
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		name         string
		statusCat    provider.StateCategory
		isDraft      bool
		wantContains string
	}{
		{
			name:         "active PR shows Active",
			statusCat:    provider.StateCategoryActive,
			isDraft:      false,
			wantContains: "● Active",
		},
		{
			name:         "draft PR shows Draft",
			statusCat:    provider.StateCategoryActive,
			isDraft:      true,
			wantContains: "Draft",
		},
		{
			name:         "completed PR shows Merged",
			statusCat:    provider.StateCategoryClosedDone,
			isDraft:      false,
			wantContains: "Merged",
		},
		{
			name:         "abandoned PR shows Closed",
			statusCat:    provider.StateCategoryRemoved,
			isDraft:      false,
			wantContains: "○ Closed",
		},
		{
			name:         "unknown status category shows glyph",
			statusCat:    provider.StateCategoryUnknown,
			isDraft:      false,
			wantContains: "○",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusIconWithStyles(tt.statusCat, tt.isDraft, styles.DefaultStyles())

			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("statusIconWithStyles(%v, %v) = %q, want to contain %q",
					tt.statusCat, tt.isDraft, got, tt.wantContains)
			}
		})
	}
}

func TestVoteIcon(t *testing.T) {
	tests := []struct {
		name         string
		reviewers    []provider.Reviewer
		wantContains string
	}{
		{
			name:         "no reviewers shows dash",
			reviewers:    []provider.Reviewer{},
			wantContains: "-",
		},
		{
			name: "approved vote shows check",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User", Vote: 10, Kind: provider.VoteKindApproved},
			},
			wantContains: "✓",
		},
		{
			name: "approved with suggestions shows tilde",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User", Vote: 5, Kind: provider.VoteKindApprovedWithSuggestions},
			},
			wantContains: "~",
		},
		{
			name: "rejected vote shows x",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User", Vote: -10, Kind: provider.VoteKindRejected},
			},
			wantContains: "✗",
		},
		{
			name: "waiting for author shows wait icon",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User", Vote: -5, Kind: provider.VoteKindWaitingForAuthor},
			},
			wantContains: "◐",
		},
		{
			name: "no vote shows pending",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User", Vote: 0, Kind: provider.VoteKindNoVote},
			},
			wantContains: "○",
		},
		{
			name: "mixed votes shows most significant (approved)",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User1", Vote: 10, Kind: provider.VoteKindApproved},
				{ID: "2", DisplayName: "User2", Vote: 0, Kind: provider.VoteKindNoVote},
			},
			wantContains: "✓",
		},
		{
			name: "mixed votes shows most significant (rejected)",
			reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "User1", Vote: 10, Kind: provider.VoteKindApproved},
				{ID: "2", DisplayName: "User2", Vote: -10, Kind: provider.VoteKindRejected},
			},
			wantContains: "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voteIconWithStyles(tt.reviewers, styles.DefaultStyles())

			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("voteIconWithStyles() = %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestNewModel(t *testing.T) {
	model := NewModel(nil)

	if model.GetViewMode() != ViewList {
		t.Errorf("Initial ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}

	if len(model.list.Items()) != 0 {
		t.Errorf("Initial prs length = %d, want 0", len(model.list.Items()))
	}
}

func TestUpdateWithSetPRsMsg(t *testing.T) {
	model := NewModel(nil)

	prs := []provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "101"},
			Title:         "Add feature",
			Status:        "active",
			SourceRefName: "refs/heads/feature/test",
			TargetRefName: "refs/heads/main",
			CreatedByName: "John Doe",
			RepositoryName: "my-repo",
		},
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "102"},
			Title:         "Fix bug",
			Status:        "active",
			IsDraft:       true,
			SourceRefName: "refs/heads/fix/bug",
			TargetRefName: "refs/heads/main",
			CreatedByName: "Jane Smith",
			RepositoryName: "my-repo",
		},
	}

	model, _ = model.Update(SetPRsMsg{PRs: prs})

	if len(model.list.Items()) != 2 {
		t.Errorf("After SetPRsMsg, prs length = %d, want 2", len(model.list.Items()))
	}

	if model.list.Items()[0].Identity.ID != "101" {
		t.Errorf("First PR ID = %q, want 101", model.list.Items()[0].Identity.ID)
	}
}

func TestUpdateWithPullRequestsMsg(t *testing.T) {
	model := NewModel(nil)

	prs := []provider.PullRequest{
		{
			Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "201"},
			Title:    "Test PR",
			Status:   "active",
		},
	}

	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	if len(model.list.Items()) != 1 {
		t.Errorf("After pullRequestsMsg, prs length = %d, want 1", len(model.list.Items()))
	}
}

func TestUpdateWithPullRequestsMsgError(t *testing.T) {
	model := NewModel(nil)

	model, _ = model.Update(pullRequestsMsg{prs: nil, err: errMock})

	// View should show error
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := model.View()
	if !strings.Contains(view, "Error") {
		t.Error("After pullRequestsMsg with error, view should show error")
	}
}

func TestViewModeNavigation(t *testing.T) {
	model := NewModel(nil)

	if model.GetViewMode() != ViewList {
		t.Errorf("Initial ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}

	// Simulate having some PRs loaded
	model.list = model.list.SetItems([]provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "123"},
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/feature/test",
			TargetRefName: "refs/heads/main",
			CreatedByName: "Test User",
			RepositoryName: "test-repo",
		},
	})

	// Enter should transition to detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("After Enter, ViewMode = %d, want ViewDetail (%d)", model.GetViewMode(), ViewDetail)
	}

	// Detail model should be set
	if model.list.Detail() == nil {
		t.Error("After Enter, detail model should not be nil")
	}

	// Esc should go back to list
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.GetViewMode() != ViewList {
		t.Errorf("After Esc, ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}
}

func TestViewError(t *testing.T) {
	model := NewModel(nil)
	model, _ = model.Update(pullRequestsMsg{prs: nil, err: errMock})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := model.View()

	if !strings.Contains(view, "Error") {
		t.Error("Error view should contain 'Error'")
	}
}

func TestViewEmpty(t *testing.T) {
	model := NewModel(nil)
	model.list = model.list.SetItems([]provider.PullRequest{})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := model.View()

	if !strings.Contains(view, "No pull requests") {
		t.Error("Empty view should contain 'No pull requests'")
	}
}

func TestPRsToRows(t *testing.T) {
	s := styles.DefaultStyles()
	now := time.Now()

	prs := []provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "101"},
			Title:         "Add new feature",
			Status:        "active",
			IsDraft:       false,
			SourceRefName: "refs/heads/feature/new",
			TargetRefName: "refs/heads/main",
			CreatedByName: "John Doe",
			RepositoryName: "my-repo",
			CreationDate:  now,
			Reviewers: []provider.Reviewer{
				{ID: "1", DisplayName: "Jane", Vote: 10},
			},
		},
	}

	rows := prsToRows(prs, s)

	if len(rows) != 1 {
		t.Fatalf("prsToRows() returned %d rows, want 1", len(rows))
	}

	row := rows[0]
	if len(row) != 6 {
		t.Errorf("Row has %d columns, want 6", len(row))
	}

	if row[1] != "Add new feature" {
		t.Errorf("Title column = %q, want 'Add new feature'", row[1])
	}

	if row[3] != "John Doe" {
		t.Errorf("Author column = %q, want 'John Doe'", row[3])
	}

	if row[4] != "my-repo" {
		t.Errorf("Repo column = %q, want 'my-repo'", row[4])
	}
}

func TestGetContextItems(t *testing.T) {
	model := NewModel(nil)

	items := model.GetContextItems()
	if items != nil {
		t.Error("List view should return nil context items")
	}
}

func TestHasContextBar(t *testing.T) {
	model := NewModel(nil)

	if model.HasContextBar() {
		t.Error("List view should not have context bar")
	}

	// PR detail view should have context bar (shows diff, navigate, etc.)
	model.list = model.list.SetItems([]provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "123"},
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedByName: "User",
			RepositoryName: "repo",
		},
	})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !model.HasContextBar() {
		t.Error("Detail view should have context bar")
	}
}

func TestFilterPR(t *testing.T) {
	pr := provider.PullRequest{
		Title:         "Add login feature",
		CreatedByName: "John Doe",
		RepositoryName: "frontend-app",
		SourceRefName: "refs/heads/feature/login",
		TargetRefName: "refs/heads/main",
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"login", true},         // matches title
		{"LOGIN", true},         // case-insensitive
		{"john", true},          // matches author
		{"frontend", true},      // matches repo name
		{"feature/login", true}, // matches source branch
		{"main", true},          // matches target branch
		{"nonexistent", false},  // no match
		{"", true},              // empty query matches all
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := filterPR(pr, tt.query)
			if got != tt.want {
				t.Errorf("filterPR(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// errMock is a simple error for testing
var errMock = fmt.Errorf("mock error")

func TestSpinnerIntegration(t *testing.T) {
	model := NewModel(nil)
	// Trigger refresh which sets loading state on the returned model
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := model.View()

	if !strings.Contains(view, "Loading") || !strings.Contains(view, "pull requests") {
		t.Errorf("Loading view should contain loading message, got: %q", view)
	}
}

func TestPrsToRowsMulti_IncludesProjectColumn(t *testing.T) {
	s := styles.DefaultStyles()
	prs := []provider.PullRequest{
		{
			Identity: provider.Identity{
				Kind:         provider.KindAzure,
				Scope:        "alpha",
				ScopeDisplay: "alpha",
				ID:           "101",
			},
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/feature/x",
			TargetRefName: "refs/heads/main",
			CreatedByName: "John",
			RepositoryName: "repo",
		},
	}

	rows := prsToRowsMulti(prs, s)
	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if len(row) != 7 {
		t.Fatalf("Expected 7 columns (with Project), got %d", len(row))
	}
	if row[0] != "alpha" {
		t.Errorf("Project column = %q, want 'alpha'", row[0])
	}
}

func TestModel_IsSearching_WhenDiffViewInputActive(t *testing.T) {
	model := NewModel(nil)

	// Set up PR data and navigate to diff view
	model.list = model.list.SetItems([]provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "123"},
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedByName: "User",
			RepositoryName: "repo",
		},
	})

	// Without diff view, IsSearching should be false
	if model.IsSearching() {
		t.Error("IsSearching() should be false without active diff view")
	}

	// Simulate having an active diff view with input mode
	s := styles.DefaultStyles()
	model.diffView = NewDiffModel(nil, provider.PullRequest{}, nil, s)
	model.viewMode = ViewDiff

	// Without input active, IsSearching should still be false
	if model.IsSearching() {
		t.Error("IsSearching() should be false when diff view has no active input")
	}

	// With input active, IsSearching should be true
	model.diffView.inputMode = InputNewComment
	if !model.IsSearching() {
		t.Error("IsSearching() should be true when diff view has active input (InputNewComment)")
	}

	// With reply input active, IsSearching should also be true
	model.diffView.inputMode = InputReply
	if !model.IsSearching() {
		t.Error("IsSearching() should be true when diff view has active input (InputReply)")
	}
}

func TestModel_VoteFlowThroughDetailView(t *testing.T) {
	model := NewModel(nil)

	// Set up PR data and navigate to detail view
	model.list = model.list.SetItems([]provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "123"},
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedByName: "User",
			RepositoryName: "repo",
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Fatalf("Expected ViewDetail, got %d", model.GetViewMode())
	}

	// Press 'v' to open vote picker
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})

	// Verify vote picker is visible through the detail adapter
	if adapter, ok := model.list.Detail().(*detailAdapter); ok {
		if !adapter.model.votePicker.IsVisible() {
			t.Error("Vote picker should be visible after pressing 'v' in detail view")
		}
	} else {
		t.Fatal("Expected detailAdapter")
	}

	// Press Esc to close vote picker (not the detail view)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Should still be in detail view (Esc closed the picker, not the view)
	if model.GetViewMode() != ViewDetail {
		t.Error("Esc should close vote picker, not exit detail view")
	}

	if adapter, ok := model.list.Detail().(*detailAdapter); ok {
		if adapter.model.votePicker.IsVisible() {
			t.Error("Vote picker should be hidden after Esc")
		}
	}

	// Now pressing Esc again should exit detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.GetViewMode() != ViewList {
		t.Error("Second Esc should exit detail view back to list")
	}
}

func TestUpdate_PullRequestsMsg_CriticalErrorNotShownInline(t *testing.T) {
	model := NewModel(nil)
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	criticalErr := fmt.Errorf("all projects failed: [HTTP request failed with status 400]")
	model, cmd := model.Update(pullRequestsMsg{prs: nil, err: criticalErr})

	if cmd == nil {
		t.Fatal("Expected a command to be returned for critical error, got nil")
	}
	msg := cmd()
	if _, ok := msg.(components.CriticalErrorMsg); !ok {
		t.Errorf("Expected CriticalErrorMsg, got %T", msg)
	}

	// Critical error should NOT show inline
	view := model.View()
	if strings.Contains(view, "Error loading") {
		t.Error("Critical error should not be displayed inline in the list view")
	}
}

func TestModel_MyPRsToggle(t *testing.T) {
	model := NewModel(nil)

	prs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "My PR", CreatedByName: "Me"},
		{Identity: provider.Identity{ID: "2"}, Title: "Other PR", CreatedByName: "Other"},
	}

	// Load PRs
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	if model.IsMyPRsActive() {
		t.Error("my PRs filter should be off initially")
	}

	// Press 'm' to toggle on
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if !model.IsMyPRsActive() {
		t.Error("my PRs filter should be on after pressing 'm'")
	}
	// With nil client, fetch returns immediately with nil data
	if cmd == nil {
		t.Error("expected a fetch command when toggling on")
	}

	// Press 'm' again to toggle off — should restore all items
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if model.IsMyPRsActive() {
		t.Error("my PRs filter should be off after second 'm' press")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("after toggle off, expected 2 PRs, got %d", len(model.list.Items()))
	}
}

func TestModel_MyPRsToggle_NotInSearchMode(t *testing.T) {
	model := NewModel(nil)

	prs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR", CreatedByName: "user-1"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Enter search mode
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !model.IsSearching() {
		t.Fatal("should be in search mode")
	}

	// Press 'm' while searching — should NOT toggle
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if model.IsMyPRsActive() {
		t.Error("'m' should not toggle my PRs filter while in search mode")
	}
}

func TestModel_MyPRsToggle_NotInDetailView(t *testing.T) {
	model := NewModel(nil)

	prs := []provider.PullRequest{
		{
			Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "1"},
			Title: "PR", Status: "active",
			SourceRefName: "refs/heads/test", TargetRefName: "refs/heads/main",
			CreatedByName: "User", RepositoryName: "repo",
		},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Fatal("should be in detail view")
	}

	// Press 'm' while in detail — should NOT toggle
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if model.IsMyPRsActive() {
		t.Error("'m' should not toggle my PRs filter while in detail view")
	}
}

func TestModel_MyPRsMsg_SetsItems(t *testing.T) {
	model := NewModel(nil)

	allPRs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "My PR", CreatedByName: "user-1"},
		{Identity: provider.Identity{ID: "2"}, Title: "Other PR", CreatedByName: "user-2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: allPRs, err: nil})

	// Toggle on
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Simulate server returning filtered results
	myPRs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "My PR", CreatedByName: "user-1"},
	}
	model, _ = model.Update(myPullRequestsMsg{prs: myPRs, err: nil})

	if len(model.list.Items()) != 1 {
		t.Errorf("expected 1 PR after my PRs fetch, got %d", len(model.list.Items()))
	}
	if model.list.Items()[0].Identity.ID != "1" {
		t.Errorf("expected PR ID '1', got %q", model.list.Items()[0].Identity.ID)
	}
}

func TestModel_MyPRsMsg_ErrorFallsBack(t *testing.T) {
	model := NewModel(nil)

	allPRs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR 1"},
		{Identity: provider.Identity{ID: "2"}, Title: "PR 2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: allPRs, err: nil})

	// Toggle on
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Simulate error
	model, _ = model.Update(myPullRequestsMsg{prs: nil, err: fmt.Errorf("fetch failed")})

	// Should fall back to all items
	if model.IsMyPRsActive() {
		t.Error("should have toggled off on error")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("expected 2 PRs (fallback to all), got %d", len(model.list.Items()))
	}
}

func TestModel_AsReviewerToggle(t *testing.T) {
	model := NewModel(nil)

	prs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR 1"},
		{Identity: provider.Identity{ID: "2"}, Title: "PR 2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	if model.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be off initially")
	}

	// Press 'A' to toggle on
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if !model.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be on after pressing 'A'")
	}
	if cmd == nil {
		t.Error("expected fetch command when toggling on")
	}

	// Press 'A' again to toggle off — should restore all items
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if model.IsAsReviewerActive() {
		t.Error("as-reviewer filter should be off after second 'A'")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("after toggle off, expected 2 PRs, got %d", len(model.list.Items()))
	}
}

func TestModel_AsReviewerToggle_DisablesMyPRs(t *testing.T) {
	model := NewModel(nil)
	model, _ = model.Update(pullRequestsMsg{prs: []provider.PullRequest{{Identity: provider.Identity{ID: "1"}}}, err: nil})

	// Turn on my PRs
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !model.IsMyPRsActive() {
		t.Fatal("my PRs should be on")
	}

	// Toggle 'A' — should deactivate my PRs (mutually exclusive)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if model.IsMyPRsActive() {
		t.Error("'A' should turn off my PRs filter")
	}
	if !model.IsAsReviewerActive() {
		t.Error("'A' should turn on as-reviewer filter")
	}
}

func TestModel_MyPRsToggle_DisablesAsReviewer(t *testing.T) {
	model := NewModel(nil)
	model, _ = model.Update(pullRequestsMsg{prs: []provider.PullRequest{{Identity: provider.Identity{ID: "1"}}}, err: nil})

	// Turn on as-reviewer
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if !model.IsAsReviewerActive() {
		t.Fatal("as-reviewer should be on")
	}

	// Toggle 'm' — should deactivate as-reviewer
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if model.IsAsReviewerActive() {
		t.Error("'m' should turn off as-reviewer filter")
	}
	if !model.IsMyPRsActive() {
		t.Error("'m' should turn on my PRs filter")
	}
}

func TestModel_AsReviewerToggle_NotInSearchMode(t *testing.T) {
	model := NewModel(nil)
	model, _ = model.Update(pullRequestsMsg{prs: []provider.PullRequest{{Identity: provider.Identity{ID: "1"}}}, err: nil})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !model.IsSearching() {
		t.Fatal("should be in search mode")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if model.IsAsReviewerActive() {
		t.Error("'A' should not toggle while searching")
	}
}

func TestModel_AsReviewerToggle_NotInDetailView(t *testing.T) {
	model := NewModel(nil)
	prs := []provider.PullRequest{
		{
			Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "1"},
			Title: "PR", Status: "active",
			SourceRefName: "refs/heads/x", TargetRefName: "refs/heads/main",
			CreatedByName: "User", RepositoryName: "r",
		},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Fatal("should be in detail view")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if model.IsAsReviewerActive() {
		t.Error("'A' should not toggle in detail view")
	}
}

func TestModel_MyPRs_EscTogglesOff(t *testing.T) {
	model := NewModel(nil)
	prs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR 1"},
		{Identity: provider.Identity{ID: "2"}, Title: "PR 2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	// Toggle my PRs on
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !model.IsMyPRsActive() {
		t.Fatal("my PRs filter should be on after pressing 'm'")
	}

	// esc should toggle it back off and restore all items
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.IsMyPRsActive() {
		t.Error("esc should turn off my PRs filter")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("after esc, expected 2 PRs restored, got %d", len(model.list.Items()))
	}
}

func TestModel_AsReviewer_EscTogglesOff(t *testing.T) {
	model := NewModel(nil)
	prs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR 1"},
		{Identity: provider.Identity{ID: "2"}, Title: "PR 2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	// Toggle as-reviewer on
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if !model.IsAsReviewerActive() {
		t.Fatal("as-reviewer filter should be on after pressing 'A'")
	}

	// esc should toggle it back off and restore all items
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.IsAsReviewerActive() {
		t.Error("esc should turn off as-reviewer filter")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("after esc, expected 2 PRs restored, got %d", len(model.list.Items()))
	}
}

func TestModel_Esc_DoesNotTurnOnMyPRs(t *testing.T) {
	model := NewModel(nil)
	prs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR 1"},
		{Identity: provider.Identity{ID: "2"}, Title: "PR 2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})

	// With no filter active, esc must NOT switch from all PRs to my PRs
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.IsMyPRsActive() {
		t.Error("esc should not turn on my PRs filter")
	}
	if model.IsAsReviewerActive() {
		t.Error("esc should not turn on as-reviewer filter")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("expected 2 PRs to remain, got %d", len(model.list.Items()))
	}
}

func TestModel_Esc_InSearchMode_ExitsSearchNotFilter(t *testing.T) {
	model := NewModel(nil)
	prs := []provider.PullRequest{{Identity: provider.Identity{ID: "1"}, Title: "PR 1"}}
	model, _ = model.Update(pullRequestsMsg{prs: prs, err: nil})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Turn on my PRs, then enter search
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !model.IsSearching() {
		t.Fatal("should be in search mode")
	}

	// First esc exits search but must leave the my PRs filter intact
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.IsSearching() {
		t.Error("esc should exit search mode")
	}
	if !model.IsMyPRsActive() {
		t.Error("esc that exits search must not also turn off my PRs filter")
	}
}

func TestModel_AsReviewerMsg_SetsItems(t *testing.T) {
	model := NewModel(nil)

	allPRs := []provider.PullRequest{
		{Identity: provider.Identity{ID: "1"}, Title: "PR 1"},
		{Identity: provider.Identity{ID: "2"}, Title: "PR 2"},
	}
	model, _ = model.Update(pullRequestsMsg{prs: allPRs, err: nil})

	// Toggle on
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})

	// Simulate server returning filtered results
	reviewerPRs := []provider.PullRequest{{Identity: provider.Identity{ID: "2"}, Title: "PR 2"}}
	model, _ = model.Update(asReviewerPullRequestsMsg{prs: reviewerPRs, err: nil})

	if len(model.list.Items()) != 1 {
		t.Errorf("expected 1 PR, got %d", len(model.list.Items()))
	}
	if model.list.Items()[0].Identity.ID != "2" {
		t.Errorf("expected PR ID '2', got %q", model.list.Items()[0].Identity.ID)
	}
}

func TestModel_AsReviewerMsg_ErrorFallsBack(t *testing.T) {
	model := NewModel(nil)

	allPRs := []provider.PullRequest{{Identity: provider.Identity{ID: "1"}}, {Identity: provider.Identity{ID: "2"}}}
	model, _ = model.Update(pullRequestsMsg{prs: allPRs, err: nil})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})

	model, _ = model.Update(asReviewerPullRequestsMsg{prs: nil, err: fmt.Errorf("fetch failed")})

	if model.IsAsReviewerActive() {
		t.Error("should have toggled off on error")
	}
	if len(model.list.Items()) != 2 {
		t.Errorf("expected 2 PRs (fallback to all), got %d", len(model.list.Items()))
	}
}

func TestFilterPRMulti_MatchesProjectName(t *testing.T) {
	pr := provider.PullRequest{
		Identity: provider.Identity{Scope: "alpha", ScopeDisplay: "alpha"},
		Title:    "Test PR",
	}

	if !filterPRMulti(pr, "alpha") {
		t.Error("filterPRMulti should match project name 'alpha'")
	}
	if filterPRMulti(pr, "beta") {
		t.Error("filterPRMulti should not match 'beta'")
	}
}

// --- updateDiffView routing tests ---

func newModelInDiffView() Model {
	s := styles.DefaultStyles()
	model := NewModel(nil)
	model.viewMode = ViewDiff
	model.diffView = NewDiffModel(nil, provider.PullRequest{
		Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "123"},
		Title:         "Test PR",
		SourceRefName: "refs/heads/feature/test",
		TargetRefName: "refs/heads/main",
		RepositoryID:  "repo-123",
	}, nil, s)
	model.diffView.SetSize(80, 24)
	return model
}

func TestUpdateDiffView_ExitDiffViewMsg_ReturnsToDetail(t *testing.T) {
	model := newModelInDiffView()

	model, _ = model.Update(exitDiffViewMsg{})

	if model.GetViewMode() != ViewDetail {
		t.Errorf("After exitDiffViewMsg, viewMode = %d, want ViewDetail", model.GetViewMode())
	}
	if model.diffView != nil {
		t.Error("After exitDiffViewMsg, diffView should be nil")
	}
}

func TestUpdateDiffView_WindowSizeMsg_PropagatedToDiffView(t *testing.T) {
	model := newModelInDiffView()

	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if model.diffView.width != 120 || model.diffView.height != 40 {
		t.Errorf("DiffView size = %dx%d, want 120x40", model.diffView.width, model.diffView.height)
	}
}

func TestUpdateDiffView_NilDiffView_FallsBackToDetail(t *testing.T) {
	model := NewModel(nil)
	model.viewMode = ViewDiff
	model.diffView = nil

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	if model.GetViewMode() != ViewDetail {
		t.Errorf("With nil diffView, viewMode should fall back to ViewDetail, got %d", model.GetViewMode())
	}
}

// --- openFileDiffMsg / openGeneralCommentsMsg tests ---

func newModelInDetailView() Model {
	model := NewModel(nil)
	model.list = model.list.SetItems([]provider.PullRequest{
		{
			Identity:      provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "123"},
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedByName: "User",
			RepositoryID:  "repo-123",
			RepositoryName: "repo",
		},
	})
	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return model
}

func TestUpdateDetail_OpenFileDiffMsg_TransitionsToDiffView(t *testing.T) {
	model := newModelInDetailView()
	if model.GetViewMode() != ViewDetail {
		t.Fatalf("Expected ViewDetail, got %d", model.GetViewMode())
	}

	file := provider.IterationChange{
		ChangeID:   1,
		Path:       "/src/main.go",
		ChangeType: "edit",
	}

	model, cmd := model.Update(openFileDiffMsg{file: file})

	if model.GetViewMode() != ViewDiff {
		t.Errorf("After openFileDiffMsg, viewMode = %d, want ViewDiff", model.GetViewMode())
	}
	if model.diffView == nil {
		t.Fatal("After openFileDiffMsg, diffView should not be nil")
	}
	if cmd == nil {
		t.Error("After openFileDiffMsg, expected a command to fetch diff")
	}
}

func TestUpdateDetail_OpenGeneralCommentsMsg_TransitionsToDiffView(t *testing.T) {
	model := newModelInDetailView()
	if model.GetViewMode() != ViewDetail {
		t.Fatalf("Expected ViewDetail, got %d", model.GetViewMode())
	}

	model, cmd := model.Update(openGeneralCommentsMsg{})

	if model.GetViewMode() != ViewDiff {
		t.Errorf("After openGeneralCommentsMsg, viewMode = %d, want ViewDiff", model.GetViewMode())
	}
	if model.diffView == nil {
		t.Fatal("After openGeneralCommentsMsg, diffView should not be nil")
	}
	if !model.diffView.viewingGeneralComments {
		t.Error("After openGeneralCommentsMsg, diffView.viewingGeneralComments should be true")
	}
	if cmd == nil {
		t.Error("After openGeneralCommentsMsg, expected a command")
	}
}

// --- Delegation tests ---

func TestGetScrollPercent_DelegatedToDiffView(t *testing.T) {
	model := newModelInDiffView()

	// Populate enough content to be scrollable
	model.diffView.diffLines = make([]diffLine, 50)
	for i := range model.diffView.diffLines {
		model.diffView.diffLines[i] = diffLine{
			Type:    diffLineContext,
			Content: fmt.Sprintf("line%d", i),
			OldNum:  i + 1,
			NewNum:  i + 1,
		}
	}
	model.diffView.updateDiffViewport()

	// At top, scroll percent should be 0
	pct := model.GetScrollPercent()
	if pct != 0 {
		t.Errorf("At top, GetScrollPercent() = %f, want 0", pct)
	}

	// Scroll to the bottom
	for i := 0; i < 49; i++ {
		model.diffView.selectedLine = i + 1
		model.diffView.updateDiffViewport()
		model.diffView.ensureDiffLineVisible()
	}

	pct = model.GetScrollPercent()
	if pct < 90 {
		t.Errorf("After scrolling to bottom, GetScrollPercent() = %f, want ~100", pct)
	}
}

func TestGetStatusMessage_DelegatedToDiffView(t *testing.T) {
	model := newModelInDiffView()
	model.diffView.statusMessage = "Comment added"

	msg := model.GetStatusMessage()
	if msg != "Comment added" {
		t.Errorf("GetStatusMessage() = %q, want %q", msg, "Comment added")
	}
}

// ─── Mixed-kind glyph column tests ───────────────────────────────────────────
// These tests use a synthetic provider.Kind(2) to simulate a second backend
// (e.g. a future GitHub provider) without adding any KindGitHub constant.
// In production, all items are KindAzure so the glyph column never appears.

// TestPrsToRows_MixedKinds_GlyphColumnFirst verifies that when items span more
// than one distinct Kind, prsToRows prepends a glyph cell at position 0.
// Convention #6: assert the full glyph token AND the KindStyle foreground.
func TestPrsToRows_MixedKinds_GlyphColumnFirst(t *testing.T) {
	s := styles.DefaultStyles()

	prs := []provider.PullRequest{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "1"},
			Title:          "PR One",
			SourceRefName:  "refs/heads/feature/one",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Alice",
			RepositoryName: "repo",
		},
		{
			Identity:       provider.Identity{Kind: provider.Kind(2), Scope: "proj", ID: "2"},
			Title:          "PR Two",
			SourceRefName:  "refs/heads/feature/two",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Bob",
			RepositoryName: "repo",
		},
	}

	rows := prsToRows(prs, s)

	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}
	// Mixed kinds: 6 normal + 1 leading glyph = 7 cells per row.
	const wantCells = 7
	if len(rows[0]) != wantCells {
		t.Fatalf("Mixed-kind row[0] has %d cells, want %d (glyph + 6 normal)", len(rows[0]), wantCells)
	}
	if len(rows[1]) != wantCells {
		t.Fatalf("Mixed-kind row[1] has %d cells, want %d", len(rows[1]), wantCells)
	}

	// Row 0 (KindAzure): glyph cell must contain the Azure glyph "⬡".
	wantGlyph0 := display.KindGlyph(provider.KindAzure)
	if !strings.Contains(rows[0][0], wantGlyph0) {
		t.Errorf("Row 0 glyph cell = %q, want to contain %q", rows[0][0], wantGlyph0)
	}

	// Row 1 (Kind(2)): glyph cell must contain the unknown-kind fallback "?".
	wantGlyph1 := display.KindGlyph(provider.Kind(2))
	if !strings.Contains(rows[1][0], wantGlyph1) {
		t.Errorf("Row 1 glyph cell = %q, want to contain %q", rows[1][0], wantGlyph1)
	}

	// Style assertion: KindStyle(KindAzure) must use the Muted foreground.
	wantFg := s.Theme.ForegroundMuted
	gotFg := display.KindStyle(provider.KindAzure, s).GetForeground()
	if gotFg != wantFg {
		t.Errorf("KindStyle(KindAzure) foreground = %v, want %v (Muted)", gotFg, wantFg)
	}
}

// TestPrsToRows_AzureOnly_NoGlyphColumn verifies that Azure-only items produce
// the standard 6-cell layout without a leading glyph cell (unchanged behavior).
func TestPrsToRows_AzureOnly_NoGlyphColumn(t *testing.T) {
	s := styles.DefaultStyles()

	prs := []provider.PullRequest{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "1"},
			Title:          "PR One",
			SourceRefName:  "refs/heads/feature/one",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Alice",
			RepositoryName: "repo",
		},
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "2"},
			Title:          "PR Two",
			SourceRefName:  "refs/heads/feature/two",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Bob",
			RepositoryName: "repo",
		},
	}

	rows := prsToRows(prs, s)

	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}
	// Azure-only: must produce exactly 6 cells (no glyph column prepended).
	const wantCells = 6
	if len(rows[0]) != wantCells {
		t.Errorf("Azure-only row[0] has %d cells, want %d (no glyph column)", len(rows[0]), wantCells)
	}
}

// TestPrsToRowsMulti_MixedKinds_GlyphColumnFirst verifies the multi-project
// variant: [glyph] [project] [status] [title] … (8 cells total) when mixed.
func TestPrsToRowsMulti_MixedKinds_GlyphColumnFirst(t *testing.T) {
	s := styles.DefaultStyles()

	prs := []provider.PullRequest{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ScopeDisplay: "Alpha", ID: "1"},
			Title:          "PR One",
			SourceRefName:  "refs/heads/feature/one",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Alice",
			RepositoryName: "repo",
		},
		{
			Identity:       provider.Identity{Kind: provider.Kind(2), Scope: "beta", ScopeDisplay: "Beta", ID: "2"},
			Title:          "PR Two",
			SourceRefName:  "refs/heads/feature/two",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Bob",
			RepositoryName: "repo",
		},
	}

	rows := prsToRowsMulti(prs, s)

	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}
	// Mixed + multi: 7 normal (project + 6) + 1 glyph = 8 cells.
	const wantCells = 8
	if len(rows[0]) != wantCells {
		t.Fatalf("Mixed-kind multi row[0] has %d cells, want %d", len(rows[0]), wantCells)
	}

	// Glyph is at position 0; project is at position 1.
	wantGlyph := display.KindGlyph(provider.KindAzure)
	if !strings.Contains(rows[0][0], wantGlyph) {
		t.Errorf("Row 0 glyph cell = %q, want to contain %q", rows[0][0], wantGlyph)
	}
	if rows[0][1] != "Alpha" {
		t.Errorf("Row 0 project cell = %q, want %q", rows[0][1], "Alpha")
	}
}

// TestPrsToRowsMulti_AzureOnly_NoGlyphColumn verifies the multi-project
// variant produces the standard 7-cell layout when all items are KindAzure.
func TestPrsToRowsMulti_AzureOnly_NoGlyphColumn(t *testing.T) {
	s := styles.DefaultStyles()

	prs := []provider.PullRequest{
		{
			Identity:       provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ScopeDisplay: "Alpha", ID: "1"},
			Title:          "PR One",
			SourceRefName:  "refs/heads/feature/one",
			TargetRefName:  "refs/heads/main",
			CreatedByName:  "Alice",
			RepositoryName: "repo",
		},
	}

	rows := prsToRowsMulti(prs, s)

	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}
	// Azure-only multi: must be exactly 7 cells (project + 6 normal, no glyph).
	const wantCells = 7
	if len(rows[0]) != wantCells {
		t.Errorf("Azure-only multi row has %d cells, want %d (no glyph column)", len(rows[0]), wantCells)
	}
}
