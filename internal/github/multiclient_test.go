package github

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewMultiClient_RequiresAtLeastOneRepo(t *testing.T) {
	_, err := NewMultiClient(nil, "tok", DefaultLabelConvention(), nil)
	if err == nil {
		t.Fatal("expected error for empty repos, got nil")
	}
}

func TestNewMultiClient_MalformedRepo(t *testing.T) {
	cases := []string{"noslash", "", "/noslash", "owner/"}
	for _, r := range cases {
		_, err := NewMultiClient([]string{r}, "tok", DefaultLabelConvention(), nil)
		if err == nil {
			t.Errorf("expected error for repo %q, got nil", r)
		}
	}
}

func TestNewMultiClient_ValidRepos(t *testing.T) {
	mc, err := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	if err != nil {
		t.Fatalf("NewMultiClient() error = %v", err)
	}
	if mc.ClientFor("owner/repo") == nil {
		t.Error("ClientFor(owner/repo) = nil, want non-nil")
	}
}

func TestNewMultiClient_ZeroConvDefaultsToLabelConvention(t *testing.T) {
	// A zero-value LabelConvention should be defaulted to DefaultLabelConvention().
	mc, err := NewMultiClient([]string{"owner/repo"}, "tok", LabelConvention{}, nil)
	if err != nil {
		t.Fatalf("NewMultiClient() error = %v", err)
	}
	def := DefaultLabelConvention()
	if mc.conv.TypePrefix != def.TypePrefix || mc.conv.PriorityPrefix != def.PriorityPrefix {
		t.Errorf("conv = %+v, want %+v", mc.conv, def)
	}
}

// ---------------------------------------------------------------------------
// ClientFor / DisplayNameFor / IsMultiProject / Scopes
// ---------------------------------------------------------------------------

func TestMultiClient_ClientFor_ReturnsNilForUnknownScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	if mc.ClientFor("other/repo") != nil {
		t.Error("ClientFor(unknown) should return nil")
	}
}

func TestMultiClient_DisplayNameFor_FallbackToScope(t *testing.T) {
	mc, _ := NewMultiClient([]string{"owner/repo"}, "tok", DefaultLabelConvention(), nil)
	if mc.DisplayNameFor("owner/repo") != "owner/repo" {
		t.Errorf("DisplayNameFor without names = %q, want %q", mc.DisplayNameFor("owner/repo"), "owner/repo")
	}
}

func TestMultiClient_DisplayNameFor_UsesConfiguredName(t *testing.T) {
	mc, _ := NewMultiClient(
		[]string{"owner/repo"},
		"tok",
		DefaultLabelConvention(),
		map[string]string{"owner/repo": "My Repo"},
	)
	if mc.DisplayNameFor("owner/repo") != "My Repo" {
		t.Errorf("DisplayNameFor = %q, want %q", mc.DisplayNameFor("owner/repo"), "My Repo")
	}
}

func TestMultiClient_IsMultiProject(t *testing.T) {
	single, _ := NewMultiClient([]string{"o/r1"}, "tok", DefaultLabelConvention(), nil)
	multi, _ := NewMultiClient([]string{"o/r1", "o/r2"}, "tok", DefaultLabelConvention(), nil)

	if single.IsMultiProject() {
		t.Error("IsMultiProject() = true for single repo, want false")
	}
	if !multi.IsMultiProject() {
		t.Error("IsMultiProject() = false for two repos, want true")
	}
}

func TestMultiClient_Scopes_Sorted(t *testing.T) {
	mc, _ := NewMultiClient([]string{"b/repo", "a/repo"}, "tok", DefaultLabelConvention(), nil)
	scopes := mc.Scopes()
	if len(scopes) != 2 {
		t.Fatalf("Scopes() len = %d, want 2", len(scopes))
	}
	if scopes[0] != "a/repo" || scopes[1] != "b/repo" {
		t.Errorf("Scopes() = %v, want [a/repo, b/repo]", scopes)
	}
}

// ---------------------------------------------------------------------------
// ListWorkItems — merge, sort, identity, partial failure, all-fail
// ---------------------------------------------------------------------------

const issueFixture1 = `[{
	"number": 1,
	"title": "Newer issue",
	"body": "",
	"state": "open",
	"user": {"login": "alice", "id": 1},
	"labels": [],
	"created_at": "2024-01-01T00:00:00Z",
	"updated_at": "2024-02-01T00:00:00Z",
	"html_url": "https://github.com/owner1/repo1/issues/1"
}]`

const issueFixture2 = `[{
	"number": 2,
	"title": "Older issue",
	"body": "",
	"state": "open",
	"user": {"login": "bob", "id": 2},
	"labels": [],
	"created_at": "2024-01-01T00:00:00Z",
	"updated_at": "2024-01-01T00:00:00Z",
	"html_url": "https://github.com/owner2/repo2/issues/2"
}]`

func TestMultiClient_ListWorkItems_MergesAndSortsByDate(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(issueFixture1))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(issueFixture2))
	}))
	defer srv2.Close()

	mc, err := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	if err != nil {
		t.Fatalf("NewMultiClient: %v", err)
	}
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	items, err := mc.ListWorkItems(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	// After sorting by ChangedDate desc, the newer issue should come first.
	if items[0].Title != "Newer issue" {
		t.Errorf("items[0].Title = %q, want %q", items[0].Title, "Newer issue")
	}
	if items[1].Title != "Older issue" {
		t.Errorf("items[1].Title = %q, want %q", items[1].Title, "Older issue")
	}
}

func TestMultiClient_ListWorkItems_IdentityScope(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(issueFixture1))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(issueFixture2))
	}))
	defer srv2.Close()

	mc, _ := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	items, err := mc.ListWorkItems(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems: %v", err)
	}

	found1, found2 := false, false
	for _, item := range items {
		switch item.Identity.Scope {
		case "owner1/repo1":
			found1 = true
			if item.Identity.Kind != provider.KindGitHub {
				t.Errorf("owner1/repo1 item Kind = %v, want KindGitHub", item.Identity.Kind)
			}
		case "owner2/repo2":
			found2 = true
		default:
			t.Errorf("unexpected scope %q in returned items", item.Identity.Scope)
		}
	}
	if !found1 {
		t.Error("no item from owner1/repo1")
	}
	if !found2 {
		t.Error("no item from owner2/repo2")
	}
}

func TestMultiClient_ListWorkItems_PartialFailure(t *testing.T) {
	// Server 1 returns 500; server 2 returns one healthy issue.
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(issueFixture2))
	}))
	defer srv2.Close()

	mc, _ := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	items, err := mc.ListWorkItems(10, provider.ListOpts{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var partialErr *provider.PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected *provider.PartialError, got: %T %v", err, err)
	}
	if partialErr.Failed != 1 {
		t.Errorf("Failed = %d, want 1", partialErr.Failed)
	}
	if partialErr.Total != 2 {
		t.Errorf("Total = %d, want 2", partialErr.Total)
	}
	if len(items) != 1 {
		t.Errorf("want 1 healthy item, got %d", len(items))
	}
}

func TestMultiClient_ListWorkItems_AllFail(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv2.Close()

	mc, _ := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	items, err := mc.ListWorkItems(10, provider.ListOpts{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// All-fail must NOT be a *provider.PartialError; it must be a plain error.
	var partialErr *provider.PartialError
	if errors.As(err, &partialErr) {
		t.Errorf("all-fail should not return *provider.PartialError, got: %v", err)
	}
	if items != nil {
		t.Errorf("all-fail should return nil items, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// ListPipelineRuns — basic merge + sort
// ---------------------------------------------------------------------------

const runFixture1 = `{"total_count":1,"workflow_runs":[{
	"id": 1001,
	"name": "CI",
	"status": "completed",
	"conclusion": "success",
	"run_number": 5,
	"head_branch": "main",
	"head_sha": "abc",
	"created_at": "2024-02-01T10:00:00Z",
	"updated_at": "2024-02-01T10:10:00Z",
	"html_url": "https://github.com/owner1/repo1/actions/runs/1001"
}]}`

const runFixture2 = `{"total_count":1,"workflow_runs":[{
	"id": 2001,
	"name": "CI",
	"status": "completed",
	"conclusion": "failure",
	"run_number": 3,
	"head_branch": "main",
	"head_sha": "def",
	"created_at": "2024-01-01T10:00:00Z",
	"updated_at": "2024-01-01T10:10:00Z",
	"html_url": "https://github.com/owner2/repo2/actions/runs/2001"
}]}`

func TestMultiClient_ListPipelineRuns_MergesAndSortsByQueueTime(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(runFixture1))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(runFixture2))
	}))
	defer srv2.Close()

	mc, _ := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	runs, err := mc.ListPipelineRuns(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(runs))
	}
	// Newer QueueTime (2024-02-01) comes first.
	if runs[0].Identity.Scope != "owner1/repo1" {
		t.Errorf("runs[0].Identity.Scope = %q, want %q", runs[0].Identity.Scope, "owner1/repo1")
	}
	if runs[1].Identity.Scope != "owner2/repo2" {
		t.Errorf("runs[1].Identity.Scope = %q, want %q", runs[1].Identity.Scope, "owner2/repo2")
	}
}

func TestMultiClient_ListPipelineRuns_PartialFailure(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(runFixture2))
	}))
	defer srv2.Close()

	mc, _ := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	runs, err := mc.ListPipelineRuns(10, provider.ListOpts{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var partialErr *provider.PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected *provider.PartialError, got: %T %v", err, err)
	}
	if partialErr.Failed != 1 || partialErr.Total != 2 {
		t.Errorf("PartialError{Failed:%d, Total:%d}, want {1,2}", partialErr.Failed, partialErr.Total)
	}
	if len(runs) != 1 {
		t.Errorf("want 1 healthy run, got %d", len(runs))
	}
}

// ---------------------------------------------------------------------------
// ListPullRequests — basic merge + sort
// ---------------------------------------------------------------------------

const prFixture1 = `[{
	"number": 10,
	"title": "Newer PR",
	"body": "",
	"state": "open",
	"draft": false,
	"user": {"login": "alice", "id": 1},
	"requested_reviewers": [],
	"head": {"ref": "feat/x", "sha": "abc"},
	"base": {"ref": "main", "sha": "def"},
	"created_at": "2024-02-01T00:00:00Z",
	"updated_at": "2024-02-01T00:00:00Z",
	"html_url": "https://github.com/owner1/repo1/pull/10"
}]`

const prFixture2 = `[{
	"number": 20,
	"title": "Older PR",
	"body": "",
	"state": "open",
	"draft": false,
	"user": {"login": "bob", "id": 2},
	"requested_reviewers": [],
	"head": {"ref": "feat/y", "sha": "ghi"},
	"base": {"ref": "main", "sha": "jkl"},
	"created_at": "2024-01-01T00:00:00Z",
	"updated_at": "2024-01-01T00:00:00Z",
	"html_url": "https://github.com/owner2/repo2/pull/20"
}]`

func TestMultiClient_ListPullRequests_MergesAndSortsByCreationDate(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(prFixture1))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(prFixture2))
	}))
	defer srv2.Close()

	mc, _ := NewMultiClient([]string{"owner1/repo1", "owner2/repo2"}, "tok", DefaultLabelConvention(), nil)
	mc.ClientFor("owner1/repo1").SetBaseURL(srv1.URL)
	mc.ClientFor("owner2/repo2").SetBaseURL(srv2.URL)

	prs, err := mc.ListPullRequests(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("want 2 PRs, got %d", len(prs))
	}
	// Newer CreationDate (2024-02-01) comes first.
	if prs[0].Title != "Newer PR" {
		t.Errorf("prs[0].Title = %q, want %q", prs[0].Title, "Newer PR")
	}
	if prs[1].Title != "Older PR" {
		t.Errorf("prs[1].Title = %q, want %q", prs[1].Title, "Older PR")
	}
}
