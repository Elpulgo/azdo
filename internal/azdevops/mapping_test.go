package azdevops_test

import (
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
)

const (
	testScope        = "MyProject"
	testScopeDisplay = "My Project"
)

// assertIdentityInvariant checks that all three required fields on an Identity
// are non-zero: Kind, Scope, and ID.
func assertIdentityInvariant(t *testing.T, name string, id provider.Identity) {
	t.Helper()
	if id.Kind == 0 {
		t.Errorf("%s: Identity.Kind is zero", name)
	}
	if id.Scope == "" {
		t.Errorf("%s: Identity.Scope is empty", name)
	}
	if id.ID == "" {
		t.Errorf("%s: Identity.ID is empty", name)
	}
}

// --- WorkItem ---

func TestMapWorkItem(t *testing.T) {
	now := time.Now()
	wire := azdevops.WorkItem{
		ID:  42,
		Rev: 1,
		Fields: azdevops.WorkItemFields{
			Title:         "Fix the bug",
			State:         "Active",
			WorkItemType:  "Bug",
			Priority:      2,
			ChangedDate:   now,
			CreatedDate:   now,
			IterationPath: "Iteration\\1",
			Description:   "desc",
			Tags:          "tag1; tag2",
			StoryPoints:   3.0,
		},
		URL:                "https://dev.azure.com/org/project/_workitems/edit/42",
		ProjectName:        testScope,
		ProjectDisplayName: testScopeDisplay,
	}

	got := azdevops.MapWorkItem(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "WorkItem", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.Scope != testScope {
		t.Errorf("expected Scope %q, got %q", testScope, got.Identity.Scope)
	}
	if got.Identity.ScopeDisplay != testScopeDisplay {
		t.Errorf("expected ScopeDisplay %q, got %q", testScopeDisplay, got.Identity.ScopeDisplay)
	}
	if got.Identity.ID != "42" {
		t.Errorf("expected ID %q, got %q", "42", got.Identity.ID)
	}
	if got.Title != wire.Fields.Title {
		t.Errorf("expected Title %q, got %q", wire.Fields.Title, got.Title)
	}
	if got.State != wire.Fields.State {
		t.Errorf("expected State %q, got %q", wire.Fields.State, got.State)
	}
	if got.WorkItemType != wire.Fields.WorkItemType {
		t.Errorf("expected WorkItemType %q, got %q", wire.Fields.WorkItemType, got.WorkItemType)
	}
}

// --- PullRequest ---

func TestMapPullRequest(t *testing.T) {
	now := time.Now()
	wire := azdevops.PullRequest{
		ID:            7,
		Title:         "My PR",
		Description:   "PR description",
		Status:        "active",
		CreationDate:  now,
		SourceRefName: "refs/heads/feature/x",
		TargetRefName: "refs/heads/main",
		IsDraft:       false,
		CreatedBy: azdevops.Identity{
			ID:          "user-uuid",
			DisplayName: "Alice",
		},
		Repository: azdevops.Repository{
			ID:   "repo-uuid",
			Name: "my-repo",
		},
		Reviewers: []azdevops.Reviewer{
			{ID: "rev-uuid", DisplayName: "Bob", Vote: 10},
		},
		ProjectName:        testScope,
		ProjectDisplayName: testScopeDisplay,
	}

	got := azdevops.MapPullRequest(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "PullRequest", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "7" {
		t.Errorf("expected ID %q, got %q", "7", got.Identity.ID)
	}
	if got.Title != wire.Title {
		t.Errorf("expected Title %q, got %q", wire.Title, got.Title)
	}
	if got.RepositoryID != wire.Repository.ID {
		t.Errorf("expected RepositoryID %q, got %q", wire.Repository.ID, got.RepositoryID)
	}
	if len(got.Reviewers) != 1 {
		t.Fatalf("expected 1 reviewer, got %d", len(got.Reviewers))
	}
	if got.Reviewers[0].Vote != 10 {
		t.Errorf("expected reviewer vote 10, got %d", got.Reviewers[0].Vote)
	}
}

// --- PipelineRun ---

func TestMapPipelineRun(t *testing.T) {
	now := time.Now()
	wire := azdevops.PipelineRun{
		ID:           99,
		BuildNumber:  "20260628.1",
		Status:       "completed",
		Result:       "succeeded",
		SourceBranch: "refs/heads/main",
		QueueTime:    now,
		Definition: azdevops.PipelineDefinition{
			ID:   5,
			Name: "CI Pipeline",
		},
		Links: azdevops.Links{
			Web: azdevops.Link{Href: "https://dev.azure.com/org/proj/_build/results?buildId=99"},
		},
		ProjectName:        testScope,
		ProjectDisplayName: testScopeDisplay,
	}

	got := azdevops.MapPipelineRun(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "PipelineRun", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "99" {
		t.Errorf("expected ID %q, got %q", "99", got.Identity.ID)
	}
	if got.BuildNumber != wire.BuildNumber {
		t.Errorf("expected BuildNumber %q, got %q", wire.BuildNumber, got.BuildNumber)
	}
	if got.DefinitionName != wire.Definition.Name {
		t.Errorf("expected DefinitionName %q, got %q", wire.Definition.Name, got.DefinitionName)
	}
	if got.WebURL != wire.Links.Web.Href {
		t.Errorf("expected WebURL %q, got %q", wire.Links.Web.Href, got.WebURL)
	}
}

// --- Thread ---

func TestMapThread(t *testing.T) {
	now := time.Now()
	wire := azdevops.Thread{
		ID:              13,
		PublishedDate:   now,
		LastUpdatedDate: now,
		Status:          "active",
		ThreadContext: &azdevops.ThreadContext{
			FilePath: "/src/main.go",
			RightFileStart: &azdevops.FilePosition{
				Line:   42,
				Offset: 1,
			},
		},
		Comments: []azdevops.Comment{
			{
				ID:              1,
				ParentCommentID: 0,
				Content:         "This needs fixing",
				PublishedDate:   now,
				LastUpdatedDate: now,
				CommentType:     "text",
				Author:          azdevops.Identity{ID: "author-uuid", DisplayName: "Alice"},
			},
		},
		IsDeleted: false,
	}

	got := azdevops.MapThread(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "Thread", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "13" {
		t.Errorf("expected ID %q, got %q", "13", got.Identity.ID)
	}
	// Critical: Line must map from RightFileStart.Line
	if got.Line != 42 {
		t.Errorf("expected Line 42 from RightFileStart.Line, got %d", got.Line)
	}
	if got.FilePath != "/src/main.go" {
		t.Errorf("expected FilePath %q, got %q", "/src/main.go", got.FilePath)
	}
	if len(got.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(got.Comments))
	}
}

func TestMapThread_NoThreadContext_ZeroLine(t *testing.T) {
	now := time.Now()
	wire := azdevops.Thread{
		ID:              7,
		PublishedDate:   now,
		LastUpdatedDate: now,
		Status:          "active",
		ThreadContext:   nil,
		Comments:        []azdevops.Comment{},
	}

	got := azdevops.MapThread(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "Thread(no context)", got.Identity)

	if got.Line != 0 {
		t.Errorf("expected Line 0 for general comment, got %d", got.Line)
	}
	if got.FilePath != "" {
		t.Errorf("expected empty FilePath for general comment, got %q", got.FilePath)
	}
}

// --- Comment ---

func TestMapComment(t *testing.T) {
	now := time.Now()
	wire := azdevops.Comment{
		ID:              3,
		ParentCommentID: 1,
		Content:         "LGTM",
		PublishedDate:   now,
		LastUpdatedDate: now,
		CommentType:     "text",
		Author:          azdevops.Identity{ID: "author-uuid", DisplayName: "Bob"},
	}

	got := azdevops.MapComment(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "Comment", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "3" {
		t.Errorf("expected ID %q, got %q", "3", got.Identity.ID)
	}
	if got.Content != wire.Content {
		t.Errorf("expected Content %q, got %q", wire.Content, got.Content)
	}
	if got.AuthorName != wire.Author.DisplayName {
		t.Errorf("expected AuthorName %q, got %q", wire.Author.DisplayName, got.AuthorName)
	}
}

// --- Timeline ---

func TestMapTimeline(t *testing.T) {
	now := time.Now()
	logID := 5
	wire := azdevops.Timeline{
		ID:       "timeline-uuid",
		ChangeID: 1,
		Records: []azdevops.TimelineRecord{
			{
				ID:    "record-uuid",
				Type:  "Stage",
				Name:  "Build",
				State: "completed",
				Log:   &azdevops.LogReference{ID: logID},
				Issues: []azdevops.Issue{
					{Type: "error", Message: "something failed"},
				},
				StartTime:  &now,
				FinishTime: &now,
			},
		},
	}

	got := azdevops.MapTimeline(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "Timeline", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "timeline-uuid" {
		t.Errorf("expected ID %q, got %q", "timeline-uuid", got.Identity.ID)
	}
	if len(got.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got.Records))
	}
	rec := got.Records[0]
	if rec.LogID != logID {
		t.Errorf("expected LogID %d, got %d", logID, rec.LogID)
	}
	if len(rec.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(rec.Issues))
	}
	if rec.Issues[0].Message != "something failed" {
		t.Errorf("expected issue message %q, got %q", "something failed", rec.Issues[0].Message)
	}
}

// --- BuildLog ---

func TestMapBuildLog(t *testing.T) {
	now := time.Now()
	wire := azdevops.BuildLog{
		ID:            11,
		Type:          "Container",
		URL:           "https://dev.azure.com/org/proj/_apis/build/builds/99/logs/11",
		LineCount:     500,
		CreatedOn:     &now,
		LastChangedOn: &now,
	}

	got := azdevops.MapBuildLog(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "BuildLog", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "11" {
		t.Errorf("expected ID %q, got %q", "11", got.Identity.ID)
	}
	if got.LogID != wire.ID {
		t.Errorf("expected LogID %d, got %d", wire.ID, got.LogID)
	}
	if got.LineCount != wire.LineCount {
		t.Errorf("expected LineCount %d, got %d", wire.LineCount, got.LineCount)
	}
}

// --- WorkItemComment ---

func TestMapWorkItemComment(t *testing.T) {
	now := time.Now()
	wire := azdevops.WorkItemComment{
		ID:          55,
		Text:        "Looks good",
		CreatedBy:   azdevops.Identity{ID: "user-uuid", DisplayName: "Charlie"},
		CreatedDate: now,
	}

	got := azdevops.MapWorkItemComment(wire, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "WorkItemComment", got.Identity)

	if got.Identity.Kind != provider.KindAzure {
		t.Errorf("expected Kind KindAzure, got %v", got.Identity.Kind)
	}
	if got.Identity.ID != "55" {
		t.Errorf("expected ID %q, got %q", "55", got.Identity.ID)
	}
	if got.Text != wire.Text {
		t.Errorf("expected Text %q, got %q", wire.Text, got.Text)
	}
	if got.AuthorName != wire.CreatedBy.DisplayName {
		t.Errorf("expected AuthorName %q, got %q", wire.CreatedBy.DisplayName, got.AuthorName)
	}
}

// --- TestIdentityInvariant (table-driven) ---

// mappedEntity is a helper that extracts an Identity from any mapped neutral type.
type mappedEntity struct {
	name     string
	identity provider.Identity
}

func TestIdentityInvariant(t *testing.T) {
	scope := testScope
	scopeDisplay := testScopeDisplay

	workItem := azdevops.MapWorkItem(azdevops.WorkItem{
		ID: 1,
		Fields: azdevops.WorkItemFields{
			Title: "t", State: "Active", WorkItemType: "Task",
		},
		ProjectName:        scope,
		ProjectDisplayName: scopeDisplay,
	}, scope, scopeDisplay)

	pr := azdevops.MapPullRequest(azdevops.PullRequest{
		ID:    2,
		Title: "PR",
		Repository: azdevops.Repository{
			ID:   "repo-uuid",
			Name: "repo",
		},
		ProjectName:        scope,
		ProjectDisplayName: scopeDisplay,
	}, scope, scopeDisplay)

	pipeline := azdevops.MapPipelineRun(azdevops.PipelineRun{
		ID:                 3,
		BuildNumber:        "1",
		ProjectName:        scope,
		ProjectDisplayName: scopeDisplay,
	}, scope, scopeDisplay)

	thread := azdevops.MapThread(azdevops.Thread{
		ID:     4,
		Status: "active",
	}, scope, scopeDisplay)

	comment := azdevops.MapComment(azdevops.Comment{
		ID:      5,
		Content: "x",
	}, scope, scopeDisplay)

	timeline := azdevops.MapTimeline(azdevops.Timeline{
		ID:      "tl-uuid",
		Records: []azdevops.TimelineRecord{},
	}, scope, scopeDisplay)

	buildLog := azdevops.MapBuildLog(azdevops.BuildLog{
		ID: 6,
	}, scope, scopeDisplay)

	wiComment := azdevops.MapWorkItemComment(azdevops.WorkItemComment{
		ID:   7,
		Text: "hi",
	}, scope, scopeDisplay)

	entities := []mappedEntity{
		{"WorkItem", workItem.Identity},
		{"PullRequest", pr.Identity},
		{"PipelineRun", pipeline.Identity},
		{"Thread", thread.Identity},
		{"Comment", comment.Identity},
		{"Timeline", timeline.Identity},
		{"BuildLog", buildLog.Identity},
		{"WorkItemComment", wiComment.Identity},
	}

	for _, e := range entities {
		t.Run(e.name, func(t *testing.T) {
			if e.identity.Kind == 0 {
				t.Errorf("Kind is zero")
			}
			if e.identity.Scope == "" {
				t.Errorf("Scope is empty")
			}
			if e.identity.ID == "" {
				t.Errorf("ID is empty")
			}
			// Validate Kind is a known value
			if e.identity.Kind != provider.KindAzure {
				t.Errorf("expected KindAzure (%d), got %d", provider.KindAzure, e.identity.Kind)
			}
		})
	}

}
