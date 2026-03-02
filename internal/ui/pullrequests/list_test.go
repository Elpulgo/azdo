package pullrequests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/components"
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
		status       string
		isDraft      bool
		wantContains string
	}{
		{
			name:         "active PR shows Active",
			status:       "active",
			isDraft:      false,
			wantContains: "Active",
		},
		{
			name:         "Active (capitalized) shows Active",
			status:       "Active",
			isDraft:      false,
			wantContains: "Active",
		},
		{
			name:         "draft PR shows Draft",
			status:       "active",
			isDraft:      true,
			wantContains: "Draft",
		},
		{
			name:         "completed PR shows Merged",
			status:       "completed",
			isDraft:      false,
			wantContains: "Merged",
		},
		{
			name:         "abandoned PR shows Closed",
			status:       "abandoned",
			isDraft:      false,
			wantContains: "Closed",
		},
		{
			name:         "unknown status shows the status",
			status:       "unknown",
			isDraft:      false,
			wantContains: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusIconWithStyles(tt.status, tt.isDraft, styles.DefaultStyles())

			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("statusIconWithStyles(%q, %v) = %q, want to contain %q",
					tt.status, tt.isDraft, got, tt.wantContains)
			}
		})
	}
}

func TestVoteIcon(t *testing.T) {
	tests := []struct {
		name         string
		reviewers    []azdevops.Reviewer
		wantContains string
	}{
		{
			name:         "no reviewers shows dash",
			reviewers:    []azdevops.Reviewer{},
			wantContains: "-",
		},
		{
			name: "approved vote shows check",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User", Vote: 10},
			},
			wantContains: "✓",
		},
		{
			name: "approved with suggestions shows tilde",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User", Vote: 5},
			},
			wantContains: "~",
		},
		{
			name: "rejected vote shows x",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User", Vote: -10},
			},
			wantContains: "✗",
		},
		{
			name: "waiting for author shows wait icon",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User", Vote: -5},
			},
			wantContains: "◐",
		},
		{
			name: "no vote shows pending",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User", Vote: 0},
			},
			wantContains: "○",
		},
		{
			name: "mixed votes shows most significant (approved)",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User1", Vote: 10},
				{ID: "2", DisplayName: "User2", Vote: 0},
			},
			wantContains: "✓",
		},
		{
			name: "mixed votes shows most significant (rejected)",
			reviewers: []azdevops.Reviewer{
				{ID: "1", DisplayName: "User1", Vote: 10},
				{ID: "2", DisplayName: "User2", Vote: -10},
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

	prs := []azdevops.PullRequest{
		{
			ID:            101,
			Title:         "Add feature",
			Status:        "active",
			SourceRefName: "refs/heads/feature/test",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "John Doe"},
			Repository:    azdevops.Repository{Name: "my-repo"},
		},
		{
			ID:            102,
			Title:         "Fix bug",
			Status:        "active",
			IsDraft:       true,
			SourceRefName: "refs/heads/fix/bug",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "Jane Smith"},
			Repository:    azdevops.Repository{Name: "my-repo"},
		},
	}

	model, _ = model.Update(SetPRsMsg{PRs: prs})

	if len(model.list.Items()) != 2 {
		t.Errorf("After SetPRsMsg, prs length = %d, want 2", len(model.list.Items()))
	}

	if model.list.Items()[0].ID != 101 {
		t.Errorf("First PR ID = %d, want 101", model.list.Items()[0].ID)
	}
}

func TestUpdateWithPullRequestsMsg(t *testing.T) {
	model := NewModel(nil)

	prs := []azdevops.PullRequest{
		{
			ID:     201,
			Title:  "Test PR",
			Status: "active",
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
	model.list = model.list.SetItems([]azdevops.PullRequest{
		{
			ID:            123,
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/feature/test",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "Test User"},
			Repository:    azdevops.Repository{Name: "test-repo"},
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
	model.list = model.list.SetItems([]azdevops.PullRequest{})
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := model.View()

	if !strings.Contains(view, "No pull requests") {
		t.Error("Empty view should contain 'No pull requests'")
	}
}

func TestPRsToRows(t *testing.T) {
	s := styles.DefaultStyles()
	now := time.Now()

	prs := []azdevops.PullRequest{
		{
			ID:            101,
			Title:         "Add new feature",
			Status:        "active",
			IsDraft:       false,
			SourceRefName: "refs/heads/feature/new",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "John Doe"},
			Repository:    azdevops.Repository{Name: "my-repo"},
			CreationDate:  now,
			Reviewers: []azdevops.Reviewer{
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
	model.list = model.list.SetItems([]azdevops.PullRequest{
		{
			ID:            123,
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "User"},
			Repository:    azdevops.Repository{Name: "repo"},
		},
	})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !model.HasContextBar() {
		t.Error("Detail view should have context bar")
	}
}

func TestFilterPR(t *testing.T) {
	pr := azdevops.PullRequest{
		Title:         "Add login feature",
		CreatedBy:     azdevops.Identity{DisplayName: "John Doe"},
		Repository:    azdevops.Repository{Name: "frontend-app"},
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
	prs := []azdevops.PullRequest{
		{
			ID:                 101,
			Title:              "Test PR",
			Status:             "active",
			SourceRefName:      "refs/heads/feature/x",
			TargetRefName:      "refs/heads/main",
			CreatedBy:          azdevops.Identity{DisplayName: "John"},
			Repository:         azdevops.Repository{Name: "repo"},
			ProjectName:        "alpha",
			ProjectDisplayName: "alpha",
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
	model.list = model.list.SetItems([]azdevops.PullRequest{
		{
			ID:            123,
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "User"},
			Repository:    azdevops.Repository{Name: "repo"},
		},
	})

	// Without diff view, IsSearching should be false
	if model.IsSearching() {
		t.Error("IsSearching() should be false without active diff view")
	}

	// Simulate having an active diff view with input mode
	s := styles.DefaultStyles()
	model.diffView = NewDiffModel(nil, azdevops.PullRequest{}, nil, s)
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
	model.list = model.list.SetItems([]azdevops.PullRequest{
		{
			ID:            123,
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "User"},
			Repository:    azdevops.Repository{Name: "repo"},
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

func TestFilterPRMulti_MatchesProjectName(t *testing.T) {
	pr := azdevops.PullRequest{
		Title:       "Test PR",
		ProjectName: "alpha",
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
	model.diffView = NewDiffModel(nil, azdevops.PullRequest{
		ID:            123,
		Title:         "Test PR",
		SourceRefName: "refs/heads/feature/test",
		TargetRefName: "refs/heads/main",
		Repository:    azdevops.Repository{ID: "repo-123"},
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
	model.list = model.list.SetItems([]azdevops.PullRequest{
		{
			ID:            123,
			Title:         "Test PR",
			Status:        "active",
			SourceRefName: "refs/heads/test",
			TargetRefName: "refs/heads/main",
			CreatedBy:     azdevops.Identity{DisplayName: "User"},
			Repository:    azdevops.Repository{ID: "repo-123", Name: "repo"},
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

	file := azdevops.IterationChange{
		ChangeID:   1,
		Item:       azdevops.ChangeItem{Path: "/src/main.go"},
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
