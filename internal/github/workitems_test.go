package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// ListWorkItems
// ---------------------------------------------------------------------------

func TestClient_ListWorkItems_FiltersPRs(t *testing.T) {
	// Fixture: one real issue and one PR-shaped object (has "pull_request" key).
	fixture := `[
		{
			"number": 1,
			"title": "Real issue",
			"body": "body",
			"state": "open",
			"user": {"login": "alice", "id": 1},
			"labels": [],
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-02T00:00:00Z",
			"html_url": "https://github.com/o/r/issues/1"
		},
		{
			"number": 2,
			"title": "A pull request",
			"body": "",
			"state": "open",
			"user": {"login": "bob", "id": 2},
			"labels": [],
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-02T00:00:00Z",
			"html_url": "https://github.com/o/r/pull/2",
			"pull_request": {"url": "https://api.github.com/repos/o/r/pulls/2"}
		}
	]`

	var capturedPath, capturedState string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedState = r.URL.Query().Get("state")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		States: []provider.StateCategory{
			provider.StateCategoryNew,
			provider.StateCategoryActive,
		},
	}
	issues, err := c.ListWorkItems(10, opts)
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	// Assert path
	if want := "/repos/o/r/issues"; capturedPath != want {
		t.Errorf("path = %q, want %q", capturedPath, want)
	}

	// All-open categories → state=open
	if capturedState != "open" {
		t.Errorf("state param = %q, want %q", capturedState, "open")
	}

	// PR must be filtered out
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Number != 1 {
		t.Errorf("issue Number = %d, want 1", issues[0].Number)
	}
	if issues[0].PullRequest != nil {
		t.Error("filtered issue should not have PullRequest set")
	}
}

func TestClient_ListWorkItems_PaginatesViaLinkHeader(t *testing.T) {
	// Two pages of issues; the first advertises a "next" Link relation.
	page1 := `[
		{"number": 1, "title": "one", "state": "open", "user": {"login": "a", "id": 1}},
		{"number": 2, "title": "two", "state": "open", "user": {"login": "a", "id": 1}}
	]`
	page2 := `[
		{"number": 3, "title": "three", "state": "open", "user": {"login": "a", "id": 1}}
	]`

	var srv *httptest.Server
	var perPageSeen string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pp := r.URL.Query().Get("per_page"); pp != "" {
			perPageSeen = pp
		}
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte(page2))
			return
		}
		w.Header().Set("Link", `<`+srv.URL+`/repos/o/r/issues?page=2>; rel="next"`)
		w.Write([]byte(page1))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	issues, err := c.ListWorkItems(100, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("len(issues) = %d, want 3 (both pages collected)", len(issues))
	}
	if issues[0].Number != 1 || issues[2].Number != 3 {
		t.Errorf("unexpected issue order: %d..%d", issues[0].Number, issues[2].Number)
	}
	// The first page must request the max page size, not the caller's top.
	if perPageSeen != "100" {
		t.Errorf("per_page = %q, want %q (issuePerPageCap)", perPageSeen, "100")
	}
}

func TestClient_ListWorkItems_AllClosedStates(t *testing.T) {
	var capturedState string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedState = r.URL.Query().Get("state")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		States: []provider.StateCategory{
			provider.StateCategoryClosedDone,
			provider.StateCategoryRemoved,
		},
	}
	_, err := c.ListWorkItems(5, opts)
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	if capturedState != "closed" {
		t.Errorf("state param = %q, want %q", capturedState, "closed")
	}
}

func TestClient_ListWorkItems_EmptyStates_ReturnsAll(t *testing.T) {
	var capturedState string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedState = r.URL.Query().Get("state")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.ListWorkItems(5, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	if capturedState != "all" {
		t.Errorf("state param = %q, want %q", capturedState, "all")
	}
}

func TestClient_ListWorkItems_TopCappedAt100(t *testing.T) {
	var capturedPerPage string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPerPage = r.URL.Query().Get("per_page")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.ListWorkItems(9999, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	if capturedPerPage != "100" {
		t.Errorf("per_page = %q, want %q", capturedPerPage, "100")
	}
}

// ---------------------------------------------------------------------------
// ListMyWorkItems
// ---------------------------------------------------------------------------

func TestClient_ListMyWorkItems_SearchEndpointAndQuery(t *testing.T) {
	fixture := `{
		"total_count": 1,
		"items": [
			{
				"number": 42,
				"title": "My issue",
				"body": "description",
				"state": "open",
				"user": {"login": "carol", "id": 3},
				"labels": [],
				"created_at": "2024-03-01T00:00:00Z",
				"updated_at": "2024-03-02T00:00:00Z",
				"html_url": "https://github.com/o/r/issues/42"
			}
		]
	}`

	var capturedPath string
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	issues, err := c.ListMyWorkItems(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListMyWorkItems() error = %v", err)
	}

	// Must hit /search/issues
	if capturedPath != "/search/issues" {
		t.Errorf("path = %q, want /search/issues", capturedPath)
	}

	// q must contain is:issue and assignee:@me
	if !strings.Contains(capturedQ, "is:issue") {
		t.Errorf("q = %q: missing is:issue", capturedQ)
	}
	if !strings.Contains(capturedQ, "assignee:@me") {
		t.Errorf("q = %q: missing assignee:@me", capturedQ)
	}
	if !strings.Contains(capturedQ, "repo:o/r") {
		t.Errorf("q = %q: missing repo:o/r", capturedQ)
	}

	// Items must be unwrapped from envelope
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Number != 42 {
		t.Errorf("issue Number = %d, want 42", issues[0].Number)
	}
}

func TestClient_ListMyWorkItems_OpenStateQualifier(t *testing.T) {
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		States: []provider.StateCategory{provider.StateCategoryActive},
	}
	_, err := c.ListMyWorkItems(10, opts)
	if err != nil {
		t.Fatalf("ListMyWorkItems() error = %v", err)
	}

	if !strings.Contains(capturedQ, "state:open") {
		t.Errorf("q = %q: expected state:open qualifier for all-open states", capturedQ)
	}
}

func TestClient_ListMyWorkItems_ClosedStateQualifier(t *testing.T) {
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		States: []provider.StateCategory{provider.StateCategoryClosedDone},
	}
	_, err := c.ListMyWorkItems(10, opts)
	if err != nil {
		t.Fatalf("ListMyWorkItems() error = %v", err)
	}

	if !strings.Contains(capturedQ, "state:closed") {
		t.Errorf("q = %q: expected state:closed qualifier for all-closed states", capturedQ)
	}
}

func TestClient_ListMyWorkItems_MixedStates_NoStateQualifier(t *testing.T) {
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		States: []provider.StateCategory{
			provider.StateCategoryActive,
			provider.StateCategoryClosedDone,
		},
	}
	_, err := c.ListMyWorkItems(10, opts)
	if err != nil {
		t.Fatalf("ListMyWorkItems() error = %v", err)
	}

	if strings.Contains(capturedQ, "state:") {
		t.Errorf("q = %q: mixed states should produce no state: qualifier", capturedQ)
	}
}

// ---------------------------------------------------------------------------
// GetWorkItemTypeStates
// ---------------------------------------------------------------------------

func TestClient_GetWorkItemTypeStates_StaticOpenClosed(t *testing.T) {
	// No HTTP server needed — this is a static method.
	c := NewClient("o", "r", "tok")

	states, err := c.GetWorkItemTypeStates("issue")
	if err != nil {
		t.Fatalf("GetWorkItemTypeStates() error = %v", err)
	}

	if len(states) != 2 {
		t.Fatalf("len(states) = %d, want 2", len(states))
	}

	// First state: open / InProgress
	if states[0].Name != "open" {
		t.Errorf("states[0].Name = %q, want %q", states[0].Name, "open")
	}
	if states[0].Category != "InProgress" {
		t.Errorf("states[0].Category = %q, want %q", states[0].Category, "InProgress")
	}

	// Second state: closed / Completed
	if states[1].Name != "closed" {
		t.Errorf("states[1].Name = %q, want %q", states[1].Name, "closed")
	}
	if states[1].Category != "Completed" {
		t.Errorf("states[1].Category = %q, want %q", states[1].Category, "Completed")
	}
}

func TestClient_GetWorkItemTypeStates_IgnoresWorkItemType(t *testing.T) {
	c := NewClient("o", "r", "tok")

	for _, wt := range []string{"issue", "bug", "task", "", "anything"} {
		states, err := c.GetWorkItemTypeStates(wt)
		if err != nil {
			t.Errorf("GetWorkItemTypeStates(%q) error = %v", wt, err)
			continue
		}
		if len(states) != 2 {
			t.Errorf("GetWorkItemTypeStates(%q) len = %d, want 2", wt, len(states))
		}
	}
}

// ---------------------------------------------------------------------------
// UpdateWorkItemState
// ---------------------------------------------------------------------------

func TestClient_UpdateWorkItemState_Open(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody updateIssueStateBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.UpdateWorkItemState(7, "open"); err != nil {
		t.Fatalf("UpdateWorkItemState(open) error = %v", err)
	}

	if capturedMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", capturedMethod)
	}
	if capturedPath != "/repos/o/r/issues/7" {
		t.Errorf("path = %q, want /repos/o/r/issues/7", capturedPath)
	}
	if capturedBody.State != "open" {
		t.Errorf("body.state = %q, want open", capturedBody.State)
	}
	if capturedBody.StateReason != "reopened" {
		t.Errorf("body.state_reason = %q, want reopened", capturedBody.StateReason)
	}
}

func TestClient_UpdateWorkItemState_Closed(t *testing.T) {
	var capturedBody updateIssueStateBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.UpdateWorkItemState(7, "closed"); err != nil {
		t.Fatalf("UpdateWorkItemState(closed) error = %v", err)
	}

	if capturedBody.State != "closed" {
		t.Errorf("body.state = %q, want closed", capturedBody.State)
	}
	if capturedBody.StateReason != "completed" {
		t.Errorf("body.state_reason = %q, want completed", capturedBody.StateReason)
	}
}

func TestClient_UpdateWorkItemState_CaseInsensitive(t *testing.T) {
	var capturedBody updateIssueStateBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.UpdateWorkItemState(1, "CLOSED"); err != nil {
		t.Fatalf("UpdateWorkItemState(CLOSED) error = %v", err)
	}
	if capturedBody.State != "closed" {
		t.Errorf("body.state = %q, want closed", capturedBody.State)
	}
}

func TestClient_UpdateWorkItemState_UnrecognizedState_ReturnsError(t *testing.T) {
	// No server needed — the error is returned before any HTTP call.
	c := NewClient("o", "r", "tok")

	err := c.UpdateWorkItemState(1, "merged")
	if err == nil {
		t.Fatal("expected error for unrecognized state, got nil")
	}
	if !strings.Contains(err.Error(), "unrecognized state") {
		t.Errorf("error = %q: expected 'unrecognized state'", err.Error())
	}
	if !strings.Contains(err.Error(), "merged") {
		t.Errorf("error = %q: expected the bad state name in the message", err.Error())
	}
}

// ---------------------------------------------------------------------------
// GetWorkItemComments
// ---------------------------------------------------------------------------

func TestClient_GetWorkItemComments_DecodesFixture(t *testing.T) {
	fixture := `[
		{
			"id": 100,
			"body": "First comment",
			"user": {"login": "alice", "id": 1},
			"created_at": "2024-04-01T10:00:00Z",
			"updated_at": "2024-04-01T10:00:00Z",
			"html_url": "https://github.com/o/r/issues/5#issuecomment-100"
		},
		{
			"id": 101,
			"body": "Second comment",
			"user": {"login": "bob", "id": 2},
			"created_at": "2024-04-02T11:00:00Z",
			"updated_at": "2024-04-02T11:00:00Z",
			"html_url": "https://github.com/o/r/issues/5#issuecomment-101"
		}
	]`

	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	comments, err := c.GetWorkItemComments(5)
	if err != nil {
		t.Fatalf("GetWorkItemComments() error = %v", err)
	}

	if capturedPath != "/repos/o/r/issues/5/comments" {
		t.Errorf("path = %q, want /repos/o/r/issues/5/comments", capturedPath)
	}

	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}
	if comments[0].ID != 100 {
		t.Errorf("comments[0].ID = %d, want 100", comments[0].ID)
	}
	if comments[0].Body != "First comment" {
		t.Errorf("comments[0].Body = %q, want %q", comments[0].Body, "First comment")
	}
	if comments[0].User.Login != "alice" {
		t.Errorf("comments[0].User.Login = %q, want alice", comments[0].User.Login)
	}
	if comments[1].ID != 101 {
		t.Errorf("comments[1].ID = %d, want 101", comments[1].ID)
	}
}

// ---------------------------------------------------------------------------
// AddWorkItemComment
// ---------------------------------------------------------------------------

func TestClient_AddWorkItemComment_PostsAndReturns(t *testing.T) {
	const responseFixture = `{
		"id": 999,
		"body": "Hello world",
		"user": {"login": "carol", "id": 5},
		"created_at": "2024-05-01T09:00:00Z",
		"updated_at": "2024-05-01T09:00:00Z",
		"html_url": "https://github.com/o/r/issues/3#issuecomment-999"
	}`

	var capturedMethod, capturedPath string
	var capturedBody addCommentBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(responseFixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	comment, err := c.AddWorkItemComment(3, "Hello world")
	if err != nil {
		t.Fatalf("AddWorkItemComment() error = %v", err)
	}

	// Assert method and path
	if capturedMethod != "POST" {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedPath != "/repos/o/r/issues/3/comments" {
		t.Errorf("path = %q, want /repos/o/r/issues/3/comments", capturedPath)
	}

	// Assert request body contains the text as "body"
	if capturedBody.Body != "Hello world" {
		t.Errorf("request body.body = %q, want %q", capturedBody.Body, "Hello world")
	}

	// Assert the echoed comment is returned
	if comment.ID != 999 {
		t.Errorf("comment.ID = %d, want 999", comment.ID)
	}
	if comment.Body != "Hello world" {
		t.Errorf("comment.Body = %q, want %q", comment.Body, "Hello world")
	}
	if comment.User.Login != "carol" {
		t.Errorf("comment.User.Login = %q, want carol", comment.User.Login)
	}
}

func TestClient_AddWorkItemComment_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.AddWorkItemComment(3, "text")
	if err == nil {
		t.Fatal("expected error for 422 response, got nil")
	}
	if !strings.Contains(err.Error(), "add work item comment") {
		t.Errorf("error = %q: expected wrapping message", err.Error())
	}
}

// ---------------------------------------------------------------------------
// mapStateParam (unit-tested directly as an unexported function inside the
// same package)
// ---------------------------------------------------------------------------

func TestMapStateParam(t *testing.T) {
	cases := []struct {
		name   string
		states []provider.StateCategory
		want   string
	}{
		{
			name:   "nil slice → all",
			states: nil,
			want:   "all",
		},
		{
			name:   "empty slice → all",
			states: []provider.StateCategory{},
			want:   "all",
		},
		{
			name:   "only New → open",
			states: []provider.StateCategory{provider.StateCategoryNew},
			want:   "open",
		},
		{
			name:   "Active + ReadyForTest → open",
			states: []provider.StateCategory{provider.StateCategoryActive, provider.StateCategoryReadyForTest},
			want:   "open",
		},
		{
			name:   "only ClosedDone → closed",
			states: []provider.StateCategory{provider.StateCategoryClosedDone},
			want:   "closed",
		},
		{
			name:   "ClosedDone + Removed → closed",
			states: []provider.StateCategory{provider.StateCategoryClosedDone, provider.StateCategoryRemoved},
			want:   "closed",
		},
		{
			name:   "Active + ClosedDone → all",
			states: []provider.StateCategory{provider.StateCategoryActive, provider.StateCategoryClosedDone},
			want:   "all",
		},
		{
			name:   "Unknown → open (open-bucket)",
			states: []provider.StateCategory{provider.StateCategoryUnknown},
			want:   "open",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapStateParam(tc.states)
			if got != tc.want {
				t.Errorf("mapStateParam(%v) = %q, want %q", tc.states, got, tc.want)
			}
		})
	}
}
