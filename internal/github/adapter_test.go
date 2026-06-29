package github

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// Kind / IsMultiProject
// ---------------------------------------------------------------------------

func TestAdapter_Kind_ReturnsKindGitHub(t *testing.T) {
	mc, _ := NewMultiClient([]string{"o/r"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)
	if a.Kind() != provider.KindGitHub {
		t.Errorf("Kind() = %v, want KindGitHub", a.Kind())
	}
}

func TestAdapter_Kind_NilMultiClient(t *testing.T) {
	a := NewAdapter(nil)
	if a.Kind() != provider.KindGitHub {
		t.Errorf("Kind() = %v, want KindGitHub even for nil mc", a.Kind())
	}
}

func TestAdapter_IsMultiProject(t *testing.T) {
	single, _ := NewMultiClient([]string{"o/r"}, "tok", DefaultLabelConvention(), nil)
	multi, _ := NewMultiClient([]string{"o/r1", "o/r2"}, "tok", DefaultLabelConvention(), nil)

	if NewAdapter(single).IsMultiProject() {
		t.Error("IsMultiProject() = true for single repo, want false")
	}
	if !NewAdapter(multi).IsMultiProject() {
		t.Error("IsMultiProject() = false for two repos, want true")
	}
	if NewAdapter(nil).IsMultiProject() {
		t.Error("IsMultiProject() = true for nil mc, want false")
	}
}

// ---------------------------------------------------------------------------
// Nil MultiClient returns errors
// ---------------------------------------------------------------------------

func TestAdapter_NilMultiClient_ListErrors(t *testing.T) {
	a := NewAdapter(nil)

	if _, err := a.ListWorkItems(10, provider.ListOpts{}); err == nil {
		t.Error("ListWorkItems with nil mc should error")
	}
	if _, err := a.ListMyWorkItems(10, provider.ListOpts{}); err == nil {
		t.Error("ListMyWorkItems with nil mc should error")
	}
	if _, err := a.ListPullRequests(10, provider.ListOpts{}); err == nil {
		t.Error("ListPullRequests with nil mc should error")
	}
	if _, err := a.ListMyPullRequests(10, provider.ListOpts{}); err == nil {
		t.Error("ListMyPullRequests with nil mc should error")
	}
	if _, err := a.ListPullRequestsAsReviewer(10, provider.ListOpts{}); err == nil {
		t.Error("ListPullRequestsAsReviewer with nil mc should error")
	}
	if _, err := a.ListPipelineRuns(10, provider.ListOpts{}); err == nil {
		t.Error("ListPipelineRuns with nil mc should error")
	}
}

// ---------------------------------------------------------------------------
// Unknown scope → "no client for scope" error
// ---------------------------------------------------------------------------

func TestAdapter_UnknownScope_ReturnsNoClientError(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	_, err := a.GetPRThreads("unknown/scope", "", 1)
	if err == nil {
		t.Fatal("expected error for unknown scope, got nil")
	}
	if !strings.Contains(err.Error(), "no client for scope") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no client for scope")
	}
}

func TestAdapter_UnknownScope_WorkItem(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	_, err := a.GetWorkItemComments("unknown/scope", 1)
	if err == nil {
		t.Fatal("expected error for unknown scope, got nil")
	}
	if !strings.Contains(err.Error(), "no client for scope") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no client for scope")
	}
}

// ---------------------------------------------------------------------------
// GetPRIterations — synthetic single iteration
// ---------------------------------------------------------------------------

func TestAdapter_GetPRIterations_ReturnsSyntheticIteration(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	iters, err := a.GetPRIterations("owner/repo", "", 42)
	if err != nil {
		t.Fatalf("GetPRIterations: %v", err)
	}
	if len(iters) != 1 {
		t.Fatalf("want exactly 1 iteration, got %d", len(iters))
	}
	if iters[0].ID != 1 {
		t.Errorf("synthetic iteration ID = %d, want 1", iters[0].ID)
	}
	if iters[0].Description == "" {
		t.Error("synthetic iteration Description must not be empty")
	}
}

func TestAdapter_GetPRIterations_UnknownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	_, err := a.GetPRIterations("other/repo", "", 1)
	if err == nil {
		t.Fatal("expected error for unknown scope")
	}
}

// ---------------------------------------------------------------------------
// GetPRIterationChanges — maps PR files
// ---------------------------------------------------------------------------

const prFilesFixture = `[
	{"filename": "main.go", "status": "modified", "changes": 5},
	{"filename": "README.md", "status": "added", "changes": 10}
]`

func TestAdapter_GetPRIterationChanges_MapsPRFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(prFilesFixture))
	}))
	defer srv.Close()

	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner/repo").SetBaseURL(srv.URL)
	a := NewAdapter(mc)

	// iterationID=1 (the only synthetic iteration) — should be ignored.
	changes, err := a.GetPRIterationChanges("owner/repo", "", 7, 1)
	if err != nil {
		t.Fatalf("GetPRIterationChanges: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %d", len(changes))
	}
	// ChangeIDs are 1-based sequential.
	if changes[0].ChangeID != 1 {
		t.Errorf("changes[0].ChangeID = %d, want 1", changes[0].ChangeID)
	}
	if changes[1].ChangeID != 2 {
		t.Errorf("changes[1].ChangeID = %d, want 2", changes[1].ChangeID)
	}
	if changes[0].Path != "main.go" {
		t.Errorf("changes[0].Path = %q, want %q", changes[0].Path, "main.go")
	}
	if changes[0].ChangeType != "edit" {
		t.Errorf("changes[0].ChangeType = %q, want %q", changes[0].ChangeType, "edit")
	}
	if changes[1].ChangeType != "add" {
		t.Errorf("changes[1].ChangeType = %q, want %q", changes[1].ChangeType, "add")
	}
}

// ---------------------------------------------------------------------------
// GetPRThreads — flat comments grouped into threads
// ---------------------------------------------------------------------------

const reviewCommentsFixture = `[
	{
		"id": 100,
		"path": "main.go",
		"line": 42,
		"body": "root comment",
		"user": {"login": "alice", "id": 1},
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-01T00:00:00Z",
		"html_url": "https://github.com/owner/repo/pull/5#discussion_r100"
	},
	{
		"id": 101,
		"in_reply_to_id": 100,
		"path": "main.go",
		"line": 42,
		"body": "a reply",
		"user": {"login": "bob", "id": 2},
		"created_at": "2024-01-01T01:00:00Z",
		"updated_at": "2024-01-01T01:00:00Z",
		"html_url": "https://github.com/owner/repo/pull/5#discussion_r101"
	}
]`

func TestAdapter_GetPRThreads_GroupsCommentsIntoThreads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(reviewCommentsFixture))
	}))
	defer srv.Close()

	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner/repo").SetBaseURL(srv.URL)
	a := NewAdapter(mc)

	threads, err := a.GetPRThreads("owner/repo", "", 5)
	if err != nil {
		t.Fatalf("GetPRThreads: %v", err)
	}
	// Two comments (one root + one reply) should produce exactly one thread.
	if len(threads) != 1 {
		t.Fatalf("want 1 thread, got %d", len(threads))
	}
	if len(threads[0].Comments) != 2 {
		t.Errorf("want 2 comments in thread, got %d", len(threads[0].Comments))
	}
	if threads[0].Comments[0].Content != "root comment" {
		t.Errorf("thread root = %q, want %q", threads[0].Comments[0].Content, "root comment")
	}
	if threads[0].Comments[1].Content != "a reply" {
		t.Errorf("thread reply = %q, want %q", threads[0].Comments[1].Content, "a reply")
	}
	// Thread identity should be stamped with scope and KindGitHub.
	if threads[0].Identity.Scope != "owner/repo" {
		t.Errorf("thread scope = %q, want %q", threads[0].Identity.Scope, "owner/repo")
	}
	if threads[0].Identity.Kind != provider.KindGitHub {
		t.Errorf("thread kind = %v, want KindGitHub", threads[0].Identity.Kind)
	}
}

// ---------------------------------------------------------------------------
// URL helpers
// ---------------------------------------------------------------------------

func TestAdapter_WorkItemURL_KnownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"octocat/hello-world"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	want := "https://github.com/octocat/hello-world/issues/42"
	got := a.WorkItemURL("octocat/hello-world", 42)
	if got != want {
		t.Errorf("WorkItemURL = %q, want %q", got, want)
	}
}

func TestAdapter_WorkItemURL_UnknownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	if got := a.WorkItemURL("other/repo", 42); got != "" {
		t.Errorf("WorkItemURL(unknown scope) = %q, want %q", got, "")
	}
}

func TestAdapter_WorkItemURL_NilMultiClient(t *testing.T) {
	a := NewAdapter(nil)
	if got := a.WorkItemURL("owner/repo", 1); got != "" {
		t.Errorf("WorkItemURL(nil mc) = %q, want %q", got, "")
	}
}

func TestAdapter_PRURL_KnownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"octocat/hello-world"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	want := "https://github.com/octocat/hello-world/pull/7"
	got := a.PRURL("octocat/hello-world", "ignored-repo-id", 7)
	if got != want {
		t.Errorf("PRURL = %q, want %q", got, want)
	}
}

func TestAdapter_PRURL_UnknownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	if got := a.PRURL("other/repo", "", 7); got != "" {
		t.Errorf("PRURL(unknown scope) = %q, want %q", got, "")
	}
}

func TestAdapter_PRThreadWebURL_KnownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"octocat/hello-world"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	want := "https://github.com/octocat/hello-world/pull/5#discussion_r100"
	got := a.PRThreadWebURL("octocat/hello-world", "", 5, 100)
	if got != want {
		t.Errorf("PRThreadWebURL = %q, want %q", got, want)
	}
}

func TestAdapter_PRThreadWebURL_ZeroThreadID(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	if got := a.PRThreadWebURL("owner/repo", "", 5, 0); got != "" {
		t.Errorf("PRThreadWebURL(threadID=0) = %q, want %q", got, "")
	}
}

func TestAdapter_PipelineURL_KnownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"octocat/hello-world"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	want := "https://github.com/octocat/hello-world/actions/runs/999"
	got := a.PipelineURL("octocat/hello-world", 999)
	if got != want {
		t.Errorf("PipelineURL = %q, want %q", got, want)
	}
}

func TestAdapter_PipelineURL_UnknownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	a := NewAdapter(mc)

	if got := a.PipelineURL("other/repo", 1); got != "" {
		t.Errorf("PipelineURL(unknown scope) = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// GetWorkItemComments — maps IssueComments to neutral types
// ---------------------------------------------------------------------------

const issueCommentsFixture = `[
	{
		"id": 500,
		"body": "first comment",
		"user": {"login": "alice", "id": 1},
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-01T00:00:00Z",
		"html_url": "https://github.com/owner/repo/issues/3#issuecomment-500"
	}
]`

func TestAdapter_GetWorkItemComments_MapsComments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(issueCommentsFixture))
	}))
	defer srv.Close()

	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner/repo").SetBaseURL(srv.URL)
	a := NewAdapter(mc)

	comments, err := a.GetWorkItemComments("owner/repo", 3)
	if err != nil {
		t.Fatalf("GetWorkItemComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("want 1 comment, got %d", len(comments))
	}
	if comments[0].Text != "first comment" {
		t.Errorf("comment Text = %q, want %q", comments[0].Text, "first comment")
	}
	if comments[0].Identity.Scope != "owner/repo" {
		t.Errorf("comment scope = %q, want %q", comments[0].Identity.Scope, "owner/repo")
	}
	if comments[0].Identity.Kind != provider.KindGitHub {
		t.Errorf("comment kind = %v, want KindGitHub", comments[0].Identity.Kind)
	}
}

// ---------------------------------------------------------------------------
// GetBuildTimeline — maps run + jobs to Timeline
// ---------------------------------------------------------------------------

const buildRunFixture = `{
	"id": 1001,
	"name": "CI",
	"status": "completed",
	"conclusion": "success",
	"run_number": 5,
	"head_branch": "main",
	"head_sha": "abc123",
	"created_at": "2024-01-01T10:00:00Z",
	"updated_at": "2024-01-01T10:10:00Z",
	"html_url": "https://github.com/owner/repo/actions/runs/1001"
}`

const buildJobsFixture = `{"total_count":1,"jobs":[{
	"id": 2001,
	"name": "build",
	"status": "completed",
	"conclusion": "success",
	"started_at": "2024-01-01T10:01:00Z",
	"completed_at": "2024-01-01T10:09:00Z",
	"steps": [
		{
			"name": "Checkout",
			"status": "completed",
			"conclusion": "success",
			"number": 1,
			"started_at": "2024-01-01T10:01:00Z",
			"completed_at": "2024-01-01T10:02:00Z"
		}
	]
}]}`

func TestAdapter_GetBuildTimeline_MapsToTimeline(t *testing.T) {
	var reqCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "/jobs") {
			w.Write([]byte(buildJobsFixture))
		} else {
			w.Write([]byte(buildRunFixture))
		}
	}))
	defer srv.Close()

	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner/repo").SetBaseURL(srv.URL)
	a := NewAdapter(mc)

	tl, err := a.GetBuildTimeline("owner/repo", 1001)
	if err != nil {
		t.Fatalf("GetBuildTimeline: %v", err)
	}
	if tl == nil {
		t.Fatal("GetBuildTimeline returned nil timeline")
	}
	if tl.Identity.Scope != "owner/repo" {
		t.Errorf("timeline scope = %q, want %q", tl.Identity.Scope, "owner/repo")
	}
	if tl.Identity.Kind != provider.KindGitHub {
		t.Errorf("timeline kind = %v, want KindGitHub", tl.Identity.Kind)
	}
	// Expect one job record + one step record.
	if len(tl.Records) != 2 {
		t.Errorf("want 2 timeline records (1 job + 1 step), got %d", len(tl.Records))
	}
	if tl.Records[0].Type != "Job" {
		t.Errorf("records[0].Type = %q, want %q", tl.Records[0].Type, "Job")
	}
	if tl.Records[1].Type != "Task" {
		t.Errorf("records[1].Type = %q, want %q", tl.Records[1].Type, "Task")
	}
	// Two GET requests must have been made (run + jobs).
	if reqCount != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", reqCount)
	}
}

// ---------------------------------------------------------------------------
// AddPRComment — synthesizes a Thread from IssueComment
// ---------------------------------------------------------------------------

const addPRCommentResponse = `{
	"id": 700,
	"body": "general comment",
	"user": {"login": "alice", "id": 1},
	"created_at": "2024-01-01T00:00:00Z",
	"updated_at": "2024-01-01T00:00:00Z",
	"html_url": "https://github.com/owner/repo/issues/9#issuecomment-700"
}`

func TestAdapter_AddPRComment_SynthesizesThread(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(addPRCommentResponse))
	}))
	defer srv.Close()

	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner/repo").SetBaseURL(srv.URL)
	a := NewAdapter(mc)

	thread, err := a.AddPRComment("owner/repo", "", 9, "general comment")
	if err != nil {
		t.Fatalf("AddPRComment: %v", err)
	}
	if thread == nil {
		t.Fatal("AddPRComment returned nil thread")
	}
	if thread.FilePath != "" {
		t.Errorf("thread.FilePath = %q, want empty (general comment)", thread.FilePath)
	}
	if thread.Line != 0 {
		t.Errorf("thread.Line = %d, want 0 (general comment)", thread.Line)
	}
	if len(thread.Comments) != 1 {
		t.Fatalf("want 1 comment in synthesized thread, got %d", len(thread.Comments))
	}
	if thread.Comments[0].Content != "general comment" {
		t.Errorf("comment content = %q, want %q", thread.Comments[0].Content, "general comment")
	}
	if thread.Identity.Kind != provider.KindGitHub {
		t.Errorf("thread kind = %v, want KindGitHub", thread.Identity.Kind)
	}
}
