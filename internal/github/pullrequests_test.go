package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// ListPullRequests
// ---------------------------------------------------------------------------

func TestClient_ListPullRequests_PathAndStateParam(t *testing.T) {
	fixture := `[
		{
			"number": 5,
			"title": "Add feature",
			"body": "body",
			"state": "open",
			"draft": false,
			"user": {"login": "alice", "id": 1},
			"requested_reviewers": [],
			"head": {"ref": "feature/x", "sha": "abc123"},
			"base": {"ref": "main", "sha": "def456"},
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-02T00:00:00Z",
			"html_url": "https://github.com/o/r/pull/5"
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
		States: []provider.StateCategory{provider.StateCategoryActive},
	}
	prs, err := c.ListPullRequests(10, opts)
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	if capturedPath != "/repos/o/r/pulls" {
		t.Errorf("path = %q, want /repos/o/r/pulls", capturedPath)
	}
	if capturedState != "open" {
		t.Errorf("state param = %q, want open", capturedState)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	if prs[0].Number != 5 {
		t.Errorf("prs[0].Number = %d, want 5", prs[0].Number)
	}
	if prs[0].Head.Ref != "feature/x" {
		t.Errorf("prs[0].Head.Ref = %q, want feature/x", prs[0].Head.Ref)
	}
	if prs[0].Head.SHA != "abc123" {
		t.Errorf("prs[0].Head.SHA = %q, want abc123", prs[0].Head.SHA)
	}
}

func TestClient_ListPullRequests_ClosedState(t *testing.T) {
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
		States: []provider.StateCategory{provider.StateCategoryClosedDone},
	}
	_, err := c.ListPullRequests(5, opts)
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if capturedState != "closed" {
		t.Errorf("state param = %q, want closed", capturedState)
	}
}

func TestClient_ListPullRequests_TopCappedAt100(t *testing.T) {
	var capturedPerPage string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPerPage = r.URL.Query().Get("per_page")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.ListPullRequests(9999, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if capturedPerPage != "100" {
		t.Errorf("per_page = %q, want 100", capturedPerPage)
	}
}

// ---------------------------------------------------------------------------
// ListMyPullRequests
// ---------------------------------------------------------------------------

func TestClient_ListMyPullRequests_SearchEndpointAndQuery(t *testing.T) {
	mergedAt := time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC)
	fixture := `{
		"total_count": 1,
		"items": [
			{
				"number": 11,
				"title": "My merged PR",
				"body": "desc",
				"state": "closed",
				"user": {"login": "alice", "id": 1},
				"created_at": "2024-06-01T00:00:00Z",
				"updated_at": "2024-06-15T00:00:00Z",
				"closed_at": "2024-07-01T12:00:00Z",
				"html_url": "https://github.com/o/r/pull/11",
				"pull_request": {"merged_at": "2024-07-01T12:00:00Z", "url": "https://api.github.com/repos/o/r/pulls/11"}
			}
		]
	}`

	var capturedPath, capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	prs, err := c.ListMyPullRequests(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListMyPullRequests() error = %v", err)
	}

	if capturedPath != "/search/issues" {
		t.Errorf("path = %q, want /search/issues", capturedPath)
	}
	if !strings.Contains(capturedQ, "is:pr") {
		t.Errorf("q = %q: missing is:pr", capturedQ)
	}
	if !strings.Contains(capturedQ, "author:@me") {
		t.Errorf("q = %q: missing author:@me", capturedQ)
	}
	if !strings.Contains(capturedQ, "repo:o/r") {
		t.Errorf("q = %q: missing repo:o/r", capturedQ)
	}

	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	if prs[0].Number != 11 {
		t.Errorf("prs[0].Number = %d, want 11", prs[0].Number)
	}
	// merged_at must be captured from the nested pull_request sub-object.
	if prs[0].MergedAt == nil {
		t.Fatal("prs[0].MergedAt = nil, want non-nil (from nested pull_request.merged_at)")
	}
	if !prs[0].MergedAt.Equal(mergedAt) {
		t.Errorf("prs[0].MergedAt = %v, want %v", prs[0].MergedAt, mergedAt)
	}
	// Fidelity: Head/Base are zero for search results.
	if prs[0].Head.Ref != "" {
		t.Errorf("prs[0].Head.Ref = %q, want empty (search results are issue-shaped)", prs[0].Head.Ref)
	}
}

func TestClient_ListMyPullRequests_OpenStateQualifier(t *testing.T) {
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{States: []provider.StateCategory{provider.StateCategoryActive}}
	_, err := c.ListMyPullRequests(10, opts)
	if err != nil {
		t.Fatalf("ListMyPullRequests() error = %v", err)
	}
	if !strings.Contains(capturedQ, "state:open") {
		t.Errorf("q = %q: expected state:open for all-open states", capturedQ)
	}
}

func TestClient_ListMyPullRequests_NullMergedAt(t *testing.T) {
	fixture := `{
		"total_count": 1,
		"items": [
			{
				"number": 3,
				"title": "Open PR",
				"state": "open",
				"user": {"login": "bob", "id": 2},
				"created_at": "2024-05-01T00:00:00Z",
				"updated_at": "2024-05-02T00:00:00Z",
				"html_url": "https://github.com/o/r/pull/3",
				"pull_request": {"merged_at": null, "url": "https://api.github.com/repos/o/r/pulls/3"}
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	prs, err := c.ListMyPullRequests(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListMyPullRequests() error = %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	if prs[0].MergedAt != nil {
		t.Errorf("prs[0].MergedAt = %v, want nil for open PR", prs[0].MergedAt)
	}
}

// ---------------------------------------------------------------------------
// ListPullRequestsAsReviewer
// ---------------------------------------------------------------------------

func TestClient_ListPullRequestsAsReviewer_QueryQualifier(t *testing.T) {
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.ListPullRequestsAsReviewer(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPullRequestsAsReviewer() error = %v", err)
	}
	if !strings.Contains(capturedQ, "is:pr") {
		t.Errorf("q = %q: missing is:pr", capturedQ)
	}
	if !strings.Contains(capturedQ, "review-requested:@me") {
		t.Errorf("q = %q: missing review-requested:@me", capturedQ)
	}
	if strings.Contains(capturedQ, "author:@me") {
		t.Errorf("q = %q: must not contain author:@me (wrong method)", capturedQ)
	}
}

func TestClient_ListPullRequestsAsReviewer_ClosedState(t *testing.T) {
	var capturedQ string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQ, _ = url.QueryUnescape(r.URL.Query().Get("q"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{States: []provider.StateCategory{provider.StateCategoryClosedDone}}
	_, err := c.ListPullRequestsAsReviewer(10, opts)
	if err != nil {
		t.Fatalf("ListPullRequestsAsReviewer() error = %v", err)
	}
	if !strings.Contains(capturedQ, "state:closed") {
		t.Errorf("q = %q: expected state:closed", capturedQ)
	}
}

// ---------------------------------------------------------------------------
// GetPRThreads
// ---------------------------------------------------------------------------

func TestClient_GetPRThreads_ReturnsFlatList(t *testing.T) {
	fixture := `[
		{
			"id": 100,
			"in_reply_to_id": null,
			"path": "internal/foo.go",
			"line": 10,
			"body": "Root comment",
			"user": {"login": "alice", "id": 1},
			"created_at": "2024-04-01T08:00:00Z",
			"updated_at": "2024-04-01T08:00:00Z",
			"html_url": "https://github.com/o/r/pull/7#discussion_r100"
		},
		{
			"id": 101,
			"in_reply_to_id": 100,
			"path": "internal/foo.go",
			"line": 10,
			"body": "Reply",
			"user": {"login": "bob", "id": 2},
			"created_at": "2024-04-01T09:00:00Z",
			"updated_at": "2024-04-01T09:00:00Z",
			"html_url": "https://github.com/o/r/pull/7#discussion_r101"
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

	comments, err := c.GetPRThreads(7)
	if err != nil {
		t.Fatalf("GetPRThreads() error = %v", err)
	}

	if capturedPath != "/repos/o/r/pulls/7/comments" {
		t.Errorf("path = %q, want /repos/o/r/pulls/7/comments", capturedPath)
	}
	// Method returns flat list — grouping is the adapter's job.
	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2 (flat, not grouped)", len(comments))
	}
	if comments[0].ID != 100 {
		t.Errorf("comments[0].ID = %d, want 100", comments[0].ID)
	}
	if comments[1].InReplyToID == nil || *comments[1].InReplyToID != 100 {
		t.Errorf("comments[1].InReplyToID = %v, want *100", comments[1].InReplyToID)
	}
}

// ---------------------------------------------------------------------------
// GetPRFiles
// ---------------------------------------------------------------------------

func TestClient_GetPRFiles_DecodesFixture(t *testing.T) {
	fixture := `[
		{
			"filename": "internal/foo.go",
			"status": "modified",
			"changes": 5
		},
		{
			"filename": "internal/bar.go",
			"status": "added",
			"changes": 20,
			"previous_filename": ""
		},
		{
			"filename": "old/path.go",
			"status": "renamed",
			"previous_filename": "old/old_path.go",
			"changes": 0
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

	files, err := c.GetPRFiles(3)
	if err != nil {
		t.Fatalf("GetPRFiles() error = %v", err)
	}

	if capturedPath != "/repos/o/r/pulls/3/files" {
		t.Errorf("path = %q, want /repos/o/r/pulls/3/files", capturedPath)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	if files[0].Filename != "internal/foo.go" {
		t.Errorf("files[0].Filename = %q, want internal/foo.go", files[0].Filename)
	}
	if files[0].Status != "modified" {
		t.Errorf("files[0].Status = %q, want modified", files[0].Status)
	}
	if files[2].PreviousFilename != "old/old_path.go" {
		t.Errorf("files[2].PreviousFilename = %q, want old/old_path.go", files[2].PreviousFilename)
	}
}

// ---------------------------------------------------------------------------
// VotePullRequest
// ---------------------------------------------------------------------------

func TestClient_VotePullRequest_Approve(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody submitReviewBody

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

	// vote = 10 (VoteApprove in Azure convention, maps to > 0)
	if err := c.VotePullRequest(7, 10); err != nil {
		t.Fatalf("VotePullRequest(10) error = %v", err)
	}
	if capturedMethod != "POST" {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedPath != "/repos/o/r/pulls/7/reviews" {
		t.Errorf("path = %q, want /repos/o/r/pulls/7/reviews", capturedPath)
	}
	if capturedBody.Event != "APPROVE" {
		t.Errorf("event = %q, want APPROVE for vote=10", capturedBody.Event)
	}
}

func TestClient_VotePullRequest_RequestChanges(t *testing.T) {
	var capturedBody submitReviewBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// vote = -10 (VoteReject in Azure convention, maps to < 0)
	if err := c.VotePullRequest(7, -10); err != nil {
		t.Fatalf("VotePullRequest(-10) error = %v", err)
	}
	if capturedBody.Event != "REQUEST_CHANGES" {
		t.Errorf("event = %q, want REQUEST_CHANGES for vote=-10", capturedBody.Event)
	}
}

func TestClient_VotePullRequest_Comment(t *testing.T) {
	var capturedBody submitReviewBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// vote = 0 (no-vote/reset, maps to == 0)
	if err := c.VotePullRequest(7, 0); err != nil {
		t.Fatalf("VotePullRequest(0) error = %v", err)
	}
	if capturedBody.Event != "COMMENT" {
		t.Errorf("event = %q, want COMMENT for vote=0", capturedBody.Event)
	}
}

func TestClient_VotePullRequest_IntermediatePositive(t *testing.T) {
	var capturedBody submitReviewBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// vote = 5 (ApproveWithSuggestions in Azure → > 0 → APPROVE on GitHub)
	if err := c.VotePullRequest(7, 5); err != nil {
		t.Fatalf("VotePullRequest(5) error = %v", err)
	}
	if capturedBody.Event != "APPROVE" {
		t.Errorf("event = %q, want APPROVE for vote=5 (>0)", capturedBody.Event)
	}
}

// ---------------------------------------------------------------------------
// GetFileContent
// ---------------------------------------------------------------------------

func TestClient_GetFileContent_Base64DecodeWithWrappedNewlines(t *testing.T) {
	// base64("hello world") = aGVsbG8gd29ybGQ= (16 chars).
	// We split it at position 8 with a newline to simulate GitHub's 60-char line
	// wrapping, proving the decoder correctly strips newlines before decoding.
	const wantContent = "hello world"

	var capturedPath, capturedRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		// Use a raw string so that \n is a JSON string escape (backslash+n), not a
		// Go newline literal. json.Unmarshal will decode \n → real newline in the
		// Content field, which our decoder must strip before calling base64.Decode.
		w.Write([]byte(`{"content":"aGVsbG8g\nd29ybGQ=\n","encoding":"base64"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	content, err := c.GetFileContent("internal/main.go", "feature/my-feature")
	if err != nil {
		t.Fatalf("GetFileContent() error = %v", err)
	}

	if capturedPath != "/repos/o/r/contents/internal/main.go" {
		t.Errorf("path = %q, want /repos/o/r/contents/internal/main.go", capturedPath)
	}
	// Verify the branch name "/" is percent-encoded in the raw URL so it doesn't
	// collide with the path separator.
	if !strings.Contains(capturedRawQuery, "ref=feature%2Fmy-feature") {
		t.Errorf("raw query = %q: branch name / must be percent-encoded as %%2F", capturedRawQuery)
	}
	if content != wantContent {
		t.Errorf("content = %q, want %q", content, wantContent)
	}
}

func TestClient_GetFileContent_RefQueryEscaped(t *testing.T) {
	var capturedRef string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRef = r.URL.Query().Get("ref")
		w.WriteHeader(http.StatusOK)
		// Return empty base64 (valid but empty file).
		w.Write([]byte(`{"content":"","encoding":"base64"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.GetFileContent("README.md", "main")
	if err != nil {
		t.Fatalf("GetFileContent() error = %v", err)
	}
	if capturedRef != "main" {
		t.Errorf("ref = %q, want main", capturedRef)
	}
}

func TestClient_GetFileContent_LeadingSlashStripped(t *testing.T) {
	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"content":"","encoding":"base64"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// Leading slash in filePath must not produce a double-slash in the URL.
	_, err := c.GetFileContent("/src/main.go", "main")
	if err != nil {
		t.Fatalf("GetFileContent() error = %v", err)
	}
	if capturedPath != "/repos/o/r/contents/src/main.go" {
		t.Errorf("path = %q, want /repos/o/r/contents/src/main.go", capturedPath)
	}
}

// ---------------------------------------------------------------------------
// AddPRCodeComment
// ---------------------------------------------------------------------------

func TestClient_AddPRCodeComment_FetchesHeadSHAThenPosts(t *testing.T) {
	const prFixture = `{
		"number": 9,
		"title": "PR title",
		"state": "open",
		"draft": false,
		"user": {"login": "dev", "id": 1},
		"requested_reviewers": [],
		"head": {"ref": "feature/x", "sha": "deadbeef"},
		"base": {"ref": "main", "sha": "cafebabe"},
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-01T00:00:00Z",
		"html_url": "https://github.com/o/r/pull/9"
	}`

	const commentFixture = `{
		"id": 500,
		"in_reply_to_id": null,
		"path": "cmd/main.go",
		"line": 42,
		"body": "This looks wrong",
		"user": {"login": "reviewer", "id": 7},
		"created_at": "2024-04-10T10:00:00Z",
		"updated_at": "2024-04-10T10:00:00Z",
		"html_url": "https://github.com/o/r/pull/9#discussion_r500"
	}`

	var callPaths []string
	var capturedCommentBody addCodeCommentBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callPaths = append(callPaths, r.Method+":"+r.URL.Path)
		switch r.URL.Path {
		case "/repos/o/r/pulls/9":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(prFixture))
		case "/repos/o/r/pulls/9/comments":
			_ = json.NewDecoder(r.Body).Decode(&capturedCommentBody)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(commentFixture))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	created, err := c.AddPRCodeComment(9, "cmd/main.go", 42, "This looks wrong")
	if err != nil {
		t.Fatalf("AddPRCodeComment() error = %v", err)
	}

	// Must make two requests: GET PR, then POST comment.
	if len(callPaths) != 2 {
		t.Fatalf("callPaths = %v, want 2 calls", callPaths)
	}
	if callPaths[0] != "GET:/repos/o/r/pulls/9" {
		t.Errorf("first call = %q, want GET:/repos/o/r/pulls/9", callPaths[0])
	}
	if callPaths[1] != "POST:/repos/o/r/pulls/9/comments" {
		t.Errorf("second call = %q, want POST:/repos/o/r/pulls/9/comments", callPaths[1])
	}

	// Assert the POST body carries the head SHA, path, line, and side.
	if capturedCommentBody.CommitID != "deadbeef" {
		t.Errorf("commit_id = %q, want deadbeef (head SHA from fetched PR)", capturedCommentBody.CommitID)
	}
	if capturedCommentBody.Path != "cmd/main.go" {
		t.Errorf("path = %q, want cmd/main.go", capturedCommentBody.Path)
	}
	if capturedCommentBody.Line != 42 {
		t.Errorf("line = %d, want 42", capturedCommentBody.Line)
	}
	if capturedCommentBody.Side != "RIGHT" {
		t.Errorf("side = %q, want RIGHT", capturedCommentBody.Side)
	}

	if created.ID != 500 {
		t.Errorf("created.ID = %d, want 500", created.ID)
	}
}

// ---------------------------------------------------------------------------
// AddPRComment
// ---------------------------------------------------------------------------

func TestClient_AddPRComment_PostsToIssueEndpoint(t *testing.T) {
	const responseFixture = `{
		"id": 888,
		"body": "LGTM!",
		"user": {"login": "alice", "id": 1},
		"created_at": "2024-05-01T10:00:00Z",
		"updated_at": "2024-05-01T10:00:00Z",
		"html_url": "https://github.com/o/r/issues/7#issuecomment-888"
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

	comment, err := c.AddPRComment(7, "LGTM!")
	if err != nil {
		t.Fatalf("AddPRComment() error = %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	// General PR comments go to the issue-comment endpoint.
	if capturedPath != "/repos/o/r/issues/7/comments" {
		t.Errorf("path = %q, want /repos/o/r/issues/7/comments", capturedPath)
	}
	if capturedBody.Body != "LGTM!" {
		t.Errorf("body.body = %q, want LGTM!", capturedBody.Body)
	}
	if comment.ID != 888 {
		t.Errorf("comment.ID = %d, want 888", comment.ID)
	}
	if comment.User.Login != "alice" {
		t.Errorf("comment.User.Login = %q, want alice", comment.User.Login)
	}
}

// ---------------------------------------------------------------------------
// ReplyToThread
// ---------------------------------------------------------------------------

func TestClient_ReplyToThread_PostsWithInReplyTo(t *testing.T) {
	const responseFixture = `{
		"id": 999,
		"in_reply_to_id": 100,
		"path": "internal/foo.go",
		"line": 10,
		"body": "Done, fixed!",
		"user": {"login": "dev", "id": 5},
		"created_at": "2024-06-01T12:00:00Z",
		"updated_at": "2024-06-01T12:00:00Z",
		"html_url": "https://github.com/o/r/pull/7#discussion_r999"
	}`

	var capturedMethod, capturedPath string
	var capturedBody replyToThreadBody

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

	created, err := c.ReplyToThread(7, 100, "Done, fixed!")
	if err != nil {
		t.Fatalf("ReplyToThread() error = %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedPath != "/repos/o/r/pulls/7/comments" {
		t.Errorf("path = %q, want /repos/o/r/pulls/7/comments", capturedPath)
	}
	if capturedBody.Body != "Done, fixed!" {
		t.Errorf("body.body = %q, want Done, fixed!", capturedBody.Body)
	}
	if capturedBody.InReplyTo != 100 {
		t.Errorf("body.in_reply_to = %d, want 100", capturedBody.InReplyTo)
	}
	if created.ID != 999 {
		t.Errorf("created.ID = %d, want 999", created.ID)
	}
	if created.InReplyToID == nil || *created.InReplyToID != 100 {
		t.Errorf("created.InReplyToID = %v, want *100", created.InReplyToID)
	}
}

// ---------------------------------------------------------------------------
// UpdateThreadStatus — GraphQL path
// ---------------------------------------------------------------------------

// threadsQueryFixture is a canned GraphQL response for the reviewThreads query,
// containing one thread whose first comment has databaseId 42.
const threadsQueryFixture = `{
	"data": {
		"repository": {
			"pullRequest": {
				"reviewThreads": {
					"nodes": [
						{
							"id": "RT_kwDOABC123",
							"isResolved": false,
							"comments": {
								"nodes": [{"databaseId": 42}]
							}
						}
					]
				}
			}
		}
	}
}`

// resolveMutFixture is a canned successful GraphQL mutation response.
const resolveMutFixture = `{"data":{"resolveReviewThread":{"thread":{"id":"RT_kwDOABC123"}}}}`

// unresolveMutFixture is a canned successful GraphQL unresolve response.
const unresolveMutFixture = `{"data":{"unresolveReviewThread":{"thread":{"id":"RT_kwDOABC123"}}}}`

func TestClient_UpdateThreadStatus_Resolve(t *testing.T) {
	callCount := 0
	var capturedMutBody graphqlRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		switch callCount {
		case 1:
			// First call: reviewThreads query.
			w.Write([]byte(threadsQueryFixture))
		case 2:
			// Second call: resolveReviewThread mutation.
			_ = json.NewDecoder(r.Body).Decode(&capturedMutBody)
			w.Write([]byte(resolveMutFixture))
		default:
			t.Errorf("unexpected call #%d to /graphql", callCount)
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.UpdateThreadStatus(7, 42, "fixed"); err != nil {
		t.Fatalf("UpdateThreadStatus(fixed) error = %v", err)
	}

	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (query + mutation)", callCount)
	}
	// Mutation must use resolveReviewThread.
	if !strings.Contains(capturedMutBody.Query, "resolveReviewThread") {
		t.Errorf("mutation query = %q: expected resolveReviewThread", capturedMutBody.Query)
	}
	// Mutation must carry the matched thread node ID.
	idVar, _ := capturedMutBody.Variables["id"].(string)
	if idVar != "RT_kwDOABC123" {
		t.Errorf("mutation variables[id] = %q, want RT_kwDOABC123", idVar)
	}
}

func TestClient_UpdateThreadStatus_Resolved_CaseInsensitive(t *testing.T) {
	callCount := 0
	var capturedMutBody graphqlRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		switch callCount {
		case 1:
			w.Write([]byte(threadsQueryFixture))
		case 2:
			_ = json.NewDecoder(r.Body).Decode(&capturedMutBody)
			w.Write([]byte(resolveMutFixture))
		}
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// "CLOSED" (uppercase) should still map to resolve.
	if err := c.UpdateThreadStatus(7, 42, "CLOSED"); err != nil {
		t.Fatalf("UpdateThreadStatus(CLOSED) error = %v", err)
	}
	if !strings.Contains(capturedMutBody.Query, "resolveReviewThread") {
		t.Errorf("mutation query = %q: expected resolveReviewThread for CLOSED", capturedMutBody.Query)
	}
}

func TestClient_UpdateThreadStatus_Unresolve(t *testing.T) {
	callCount := 0
	var capturedMutBody graphqlRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		switch callCount {
		case 1:
			w.Write([]byte(threadsQueryFixture))
		case 2:
			_ = json.NewDecoder(r.Body).Decode(&capturedMutBody)
			w.Write([]byte(unresolveMutFixture))
		}
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.UpdateThreadStatus(7, 42, "active"); err != nil {
		t.Fatalf("UpdateThreadStatus(active) error = %v", err)
	}
	if !strings.Contains(capturedMutBody.Query, "unresolveReviewThread") {
		t.Errorf("mutation query = %q: expected unresolveReviewThread", capturedMutBody.Query)
	}
	idVar, _ := capturedMutBody.Variables["id"].(string)
	if idVar != "RT_kwDOABC123" {
		t.Errorf("mutation variables[id] = %q, want RT_kwDOABC123", idVar)
	}
}

func TestClient_UpdateThreadStatus_NoMatch_ReturnsError(t *testing.T) {
	// Return a threads response with databaseId 99 — does NOT match rootCommentID 42.
	const noMatchFixture = `{
		"data": {
			"repository": {
				"pullRequest": {
					"reviewThreads": {
						"nodes": [
							{
								"id": "RT_kwOther",
								"isResolved": false,
								"comments": {"nodes": [{"databaseId": 99}]}
							}
						]
					}
				}
			}
		}
	}`

	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(noMatchFixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	err := c.UpdateThreadStatus(7, 42, "fixed")
	if err == nil {
		t.Fatal("expected error for unmatched thread, got nil")
	}
	// Only the query should fire; no mutation.
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (query only, no mutation)", callCount)
	}
	// Error must mention the rootCommentID so the caller knows what failed.
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error = %q: expected rootCommentID 42 in message", err.Error())
	}
}

func TestClient_UpdateThreadStatus_UnrecognizedStatus_ReturnsError(t *testing.T) {
	// The status classification happens only AFTER the thread is found, so we
	// need the query to return a matching thread first.
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		// Return a matching thread so we reach the status-check branch.
		w.Write([]byte(threadsQueryFixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	err := c.UpdateThreadStatus(7, 42, "unknown_status")
	if err == nil {
		t.Fatal("expected error for unrecognized status, got nil")
	}
	if !strings.Contains(err.Error(), "unrecognized status") {
		t.Errorf("error = %q: expected 'unrecognized status'", err.Error())
	}
	if !strings.Contains(err.Error(), "unknown_status") {
		t.Errorf("error = %q: expected the bad status in the message", err.Error())
	}
	// Only the query fires; no mutation (error returned before mutation).
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (query only)", callCount)
	}
}

func TestClient_UpdateThreadStatus_QueryVariablesSetCorrectly(t *testing.T) {
	var capturedQueryBody graphqlRequest
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		switch callCount {
		case 1:
			_ = json.NewDecoder(r.Body).Decode(&capturedQueryBody)
			w.Write([]byte(threadsQueryFixture))
		case 2:
			w.Write([]byte(resolveMutFixture))
		}
	}))
	defer srv.Close()

	c := NewClient("owner", "reponame", "tok")
	c.SetBaseURL(srv.URL)

	_ = c.UpdateThreadStatus(13, 42, "resolved")

	// Assert the query variables use the client's owner/repo.
	if capturedQueryBody.Variables["owner"] != "owner" {
		t.Errorf("variables[owner] = %v, want owner", capturedQueryBody.Variables["owner"])
	}
	if capturedQueryBody.Variables["repo"] != "reponame" {
		t.Errorf("variables[repo] = %v, want reponame", capturedQueryBody.Variables["repo"])
	}
	// number is JSON-decoded as float64 from map[string]any.
	number, _ := capturedQueryBody.Variables["number"].(float64)
	if number != 13 {
		t.Errorf("variables[number] = %v, want 13", capturedQueryBody.Variables["number"])
	}
}
