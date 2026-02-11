package azdevops

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListPullRequests_Success(t *testing.T) {
	// Create a test server that returns mock pull requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify the API endpoint
		expectedPath := "/git/pullrequests"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Verify query parameters
		query := r.URL.Query()
		if query.Get("api-version") != "7.1" {
			t.Errorf("Expected api-version=7.1, got %s", query.Get("api-version"))
		}
		if query.Get("$top") != "25" {
			t.Errorf("Expected $top=25, got %s", query.Get("$top"))
		}
		if query.Get("searchCriteria.status") != "active" {
			t.Errorf("Expected searchCriteria.status=active, got %s", query.Get("searchCriteria.status"))
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 2,
			"value": [
				{
					"pullRequestId": 101,
					"title": "Add new feature",
					"description": "This PR adds an awesome new feature",
					"status": "active",
					"creationDate": "2024-02-06T10:00:00Z",
					"sourceRefName": "refs/heads/feature/new-feature",
					"targetRefName": "refs/heads/main",
					"isDraft": false,
					"createdBy": {
						"id": "user-123",
						"displayName": "John Doe",
						"uniqueName": "john.doe@example.com"
					},
					"repository": {
						"id": "repo-456",
						"name": "my-repo"
					},
					"reviewers": [
						{
							"id": "reviewer-1",
							"displayName": "Jane Smith",
							"vote": 0
						},
						{
							"id": "reviewer-2",
							"displayName": "Bob Wilson",
							"vote": 10
						}
					]
				},
				{
					"pullRequestId": 102,
					"title": "Fix critical bug",
					"description": "Fixes issue #42",
					"status": "active",
					"creationDate": "2024-02-05T14:30:00Z",
					"sourceRefName": "refs/heads/fix/bug-42",
					"targetRefName": "refs/heads/main",
					"isDraft": true,
					"createdBy": {
						"id": "user-789",
						"displayName": "Alice Johnson",
						"uniqueName": "alice.j@example.com"
					},
					"repository": {
						"id": "repo-456",
						"name": "my-repo"
					},
					"reviewers": []
				}
			]
		}`))
	}))
	defer server.Close()

	// Create a client with the test server URL
	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Override the base URL to use the test server
	client.baseURL = server.URL

	// Call ListPullRequests
	prs, err := client.ListPullRequests(25)
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	// Verify we got 2 PRs
	if len(prs) != 2 {
		t.Fatalf("Expected 2 PRs, got %d", len(prs))
	}

	// Verify first PR
	pr1 := prs[0]
	if pr1.ID != 101 {
		t.Errorf("prs[0].ID = %d, want 101", pr1.ID)
	}
	if pr1.Title != "Add new feature" {
		t.Errorf("prs[0].Title = %s, want 'Add new feature'", pr1.Title)
	}
	if pr1.Description != "This PR adds an awesome new feature" {
		t.Errorf("prs[0].Description = %s, want 'This PR adds an awesome new feature'", pr1.Description)
	}
	if pr1.Status != "active" {
		t.Errorf("prs[0].Status = %s, want active", pr1.Status)
	}
	if pr1.SourceRefName != "refs/heads/feature/new-feature" {
		t.Errorf("prs[0].SourceRefName = %s, want refs/heads/feature/new-feature", pr1.SourceRefName)
	}
	if pr1.TargetRefName != "refs/heads/main" {
		t.Errorf("prs[0].TargetRefName = %s, want refs/heads/main", pr1.TargetRefName)
	}
	if pr1.IsDraft {
		t.Error("prs[0].IsDraft should be false")
	}
	if pr1.CreatedBy.DisplayName != "John Doe" {
		t.Errorf("prs[0].CreatedBy.DisplayName = %s, want 'John Doe'", pr1.CreatedBy.DisplayName)
	}
	if pr1.Repository.Name != "my-repo" {
		t.Errorf("prs[0].Repository.Name = %s, want 'my-repo'", pr1.Repository.Name)
	}
	if len(pr1.Reviewers) != 2 {
		t.Errorf("prs[0].Reviewers length = %d, want 2", len(pr1.Reviewers))
	}

	// Verify second PR
	pr2 := prs[1]
	if pr2.ID != 102 {
		t.Errorf("prs[1].ID = %d, want 102", pr2.ID)
	}
	if !pr2.IsDraft {
		t.Error("prs[1].IsDraft should be true")
	}
	if len(pr2.Reviewers) != 0 {
		t.Errorf("prs[1].Reviewers length = %d, want 0", len(pr2.Reviewers))
	}
}

func TestListPullRequests_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count": 0, "value": []}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	prs, err := client.ListPullRequests(25)
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	if len(prs) != 0 {
		t.Errorf("Expected 0 PRs, got %d", len(prs))
	}
}

func TestListPullRequests_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Unauthorized"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.ListPullRequests(25)
	if err == nil {
		t.Error("Expected error for 401 response, got nil")
	}
}

func TestListPullRequests_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.ListPullRequests(25)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestListPullRequests_CustomTop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("$top") != "50" {
			t.Errorf("Expected $top=50, got %s", query.Get("$top"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count": 0, "value": []}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.ListPullRequests(50)
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
}

func TestListPullRequests_NetworkError(t *testing.T) {
	// Create a client pointing to an invalid URL
	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = "http://invalid-host-that-does-not-exist.local"

	_, err = client.ListPullRequests(25)
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

func TestPullRequest_SourceBranchShortName(t *testing.T) {
	tests := []struct {
		name          string
		sourceRefName string
		want          string
	}{
		{
			name:          "standard branch",
			sourceRefName: "refs/heads/feature/my-feature",
			want:          "feature/my-feature",
		},
		{
			name:          "main branch",
			sourceRefName: "refs/heads/main",
			want:          "main",
		},
		{
			name:          "empty string",
			sourceRefName: "",
			want:          "",
		},
		{
			name:          "no prefix",
			sourceRefName: "some-branch",
			want:          "some-branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{SourceRefName: tt.sourceRefName}
			got := pr.SourceBranchShortName()
			if got != tt.want {
				t.Errorf("SourceBranchShortName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPullRequest_TargetBranchShortName(t *testing.T) {
	tests := []struct {
		name          string
		targetRefName string
		want          string
	}{
		{
			name:          "main branch",
			targetRefName: "refs/heads/main",
			want:          "main",
		},
		{
			name:          "develop branch",
			targetRefName: "refs/heads/develop",
			want:          "develop",
		},
		{
			name:          "empty string",
			targetRefName: "",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{TargetRefName: tt.targetRefName}
			got := pr.TargetBranchShortName()
			if got != tt.want {
				t.Errorf("TargetBranchShortName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReviewer_VoteDescription(t *testing.T) {
	tests := []struct {
		name string
		vote int
		want string
	}{
		{name: "approved", vote: 10, want: "Approved"},
		{name: "approved with suggestions", vote: 5, want: "Approved with suggestions"},
		{name: "no vote", vote: 0, want: "No vote"},
		{name: "waiting", vote: -5, want: "Waiting for author"},
		{name: "rejected", vote: -10, want: "Rejected"},
		{name: "unknown positive", vote: 99, want: "Unknown"},
		{name: "unknown negative", vote: -99, want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reviewer{Vote: tt.vote}
			got := r.VoteDescription()
			if got != tt.want {
				t.Errorf("VoteDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetPRThreads_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify the API endpoint
		expectedPath := "/git/repositories/repo-123/pullRequests/101/threads"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Verify query parameters
		query := r.URL.Query()
		if query.Get("api-version") != "7.1" {
			t.Errorf("Expected api-version=7.1, got %s", query.Get("api-version"))
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 2,
			"value": [
				{
					"id": 1,
					"publishedDate": "2024-02-06T10:00:00Z",
					"lastUpdatedDate": "2024-02-06T12:00:00Z",
					"status": "active",
					"threadContext": {
						"filePath": "/src/main.go",
						"rightFileStart": {"line": 10, "offset": 1},
						"rightFileEnd": {"line": 10, "offset": 20}
					},
					"comments": [
						{
							"id": 1,
							"parentCommentId": 0,
							"content": "This looks good!",
							"publishedDate": "2024-02-06T10:00:00Z",
							"lastUpdatedDate": "2024-02-06T10:00:00Z",
							"commentType": "text",
							"author": {
								"id": "user-123",
								"displayName": "John Doe",
								"uniqueName": "john.doe@example.com"
							}
						},
						{
							"id": 2,
							"parentCommentId": 1,
							"content": "Thanks for the review!",
							"publishedDate": "2024-02-06T12:00:00Z",
							"lastUpdatedDate": "2024-02-06T12:00:00Z",
							"commentType": "text",
							"author": {
								"id": "user-456",
								"displayName": "Jane Smith",
								"uniqueName": "jane.s@example.com"
							}
						}
					],
					"isDeleted": false
				},
				{
					"id": 2,
					"publishedDate": "2024-02-06T11:00:00Z",
					"lastUpdatedDate": "2024-02-06T11:00:00Z",
					"status": "fixed",
					"comments": [
						{
							"id": 3,
							"parentCommentId": 0,
							"content": "Please add error handling here",
							"publishedDate": "2024-02-06T11:00:00Z",
							"lastUpdatedDate": "2024-02-06T11:00:00Z",
							"commentType": "text",
							"author": {
								"id": "user-789",
								"displayName": "Bob Wilson",
								"uniqueName": "bob.w@example.com"
							}
						}
					],
					"isDeleted": false
				}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	threads, err := client.GetPRThreads("repo-123", 101)
	if err != nil {
		t.Fatalf("GetPRThreads() error = %v", err)
	}

	// Verify we got 2 threads
	if len(threads) != 2 {
		t.Fatalf("Expected 2 threads, got %d", len(threads))
	}

	// Verify first thread
	thread1 := threads[0]
	if thread1.ID != 1 {
		t.Errorf("threads[0].ID = %d, want 1", thread1.ID)
	}
	if thread1.Status != "active" {
		t.Errorf("threads[0].Status = %s, want 'active'", thread1.Status)
	}
	if thread1.ThreadContext == nil {
		t.Error("threads[0].ThreadContext should not be nil")
	} else {
		if thread1.ThreadContext.FilePath != "/src/main.go" {
			t.Errorf("threads[0].ThreadContext.FilePath = %s, want '/src/main.go'", thread1.ThreadContext.FilePath)
		}
	}
	if len(thread1.Comments) != 2 {
		t.Errorf("threads[0].Comments length = %d, want 2", len(thread1.Comments))
	}

	// Verify first comment
	comment1 := thread1.Comments[0]
	if comment1.ID != 1 {
		t.Errorf("comment.ID = %d, want 1", comment1.ID)
	}
	if comment1.Content != "This looks good!" {
		t.Errorf("comment.Content = %s, want 'This looks good!'", comment1.Content)
	}
	if comment1.Author.DisplayName != "John Doe" {
		t.Errorf("comment.Author.DisplayName = %s, want 'John Doe'", comment1.Author.DisplayName)
	}

	// Verify second thread
	thread2 := threads[1]
	if thread2.Status != "fixed" {
		t.Errorf("threads[1].Status = %s, want 'fixed'", thread2.Status)
	}
}

func TestGetPRThreads_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count": 0, "value": []}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	threads, err := client.GetPRThreads("repo-123", 101)
	if err != nil {
		t.Fatalf("GetPRThreads() error = %v", err)
	}

	if len(threads) != 0 {
		t.Errorf("Expected 0 threads, got %d", len(threads))
	}
}

func TestGetPRThreads_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Pull request not found"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.GetPRThreads("repo-123", 101)
	if err == nil {
		t.Error("Expected error for 404 response, got nil")
	}
}

func TestGetPRThreads_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.GetPRThreads("repo-123", 101)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestThread_IsCodeComment(t *testing.T) {
	tests := []struct {
		name          string
		threadContext *ThreadContext
		want          bool
	}{
		{
			name:          "nil context is not code comment",
			threadContext: nil,
			want:          false,
		},
		{
			name:          "with file path is code comment",
			threadContext: &ThreadContext{FilePath: "/src/main.go"},
			want:          true,
		},
		{
			name:          "empty file path is not code comment",
			threadContext: &ThreadContext{FilePath: ""},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := &Thread{ThreadContext: tt.threadContext}
			got := thread.IsCodeComment()
			if got != tt.want {
				t.Errorf("IsCodeComment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThread_StatusDescription(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "active", status: "active", want: "Active"},
		{name: "fixed", status: "fixed", want: "Resolved"},
		{name: "wontFix", status: "wontFix", want: "Won't fix"},
		{name: "closed", status: "closed", want: "Closed"},
		{name: "pending", status: "pending", want: "Pending"},
		{name: "unknown", status: "unknown", want: "Unknown"},
		{name: "empty", status: "", want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := &Thread{Status: tt.status}
			got := thread.StatusDescription()
			if got != tt.want {
				t.Errorf("StatusDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVotePullRequest_Approve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "PUT" {
			t.Errorf("Expected PUT request, got %s", r.Method)
		}

		// Verify the API endpoint
		expectedPath := "/git/repositories/repo-123/pullRequests/101/reviewers/me"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Verify query parameters
		query := r.URL.Query()
		if query.Get("api-version") != "7.1" {
			t.Errorf("Expected api-version=7.1, got %s", query.Get("api-version"))
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"vote": 10}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	err = client.VotePullRequest("repo-123", 101, VoteApprove)
	if err != nil {
		t.Fatalf("VotePullRequest() error = %v", err)
	}
}

func TestVotePullRequest_Reject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("Expected PUT request, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"vote": -10}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	err = client.VotePullRequest("repo-123", 101, VoteReject)
	if err != nil {
		t.Fatalf("VotePullRequest() error = %v", err)
	}
}

func TestVotePullRequest_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Access denied"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	err = client.VotePullRequest("repo-123", 101, VoteApprove)
	if err == nil {
		t.Error("Expected error for 403 response, got nil")
	}
}

func TestAddPRComment_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		expectedPath := "/git/repositories/repo-123/pullRequests/101/threads"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
			"id": 5,
			"status": "active",
			"comments": [
				{
					"id": 1,
					"content": "LGTM!",
					"author": {"displayName": "Test User"}
				}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	thread, err := client.AddPRComment("repo-123", 101, "LGTM!")
	if err != nil {
		t.Fatalf("AddPRComment() error = %v", err)
	}

	if thread.ID != 5 {
		t.Errorf("Thread ID = %d, want 5", thread.ID)
	}
	if len(thread.Comments) != 1 {
		t.Errorf("Comments length = %d, want 1", len(thread.Comments))
	}
}

func TestAddPRComment_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message": "Bad request"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.AddPRComment("repo-123", 101, "Test comment")
	if err == nil {
		t.Error("Expected error for 400 response, got nil")
	}
}

func TestVoteValue(t *testing.T) {
	if VoteApprove != 10 {
		t.Errorf("VoteApprove = %d, want 10", VoteApprove)
	}
	if VoteApproveWithSuggestions != 5 {
		t.Errorf("VoteApproveWithSuggestions = %d, want 5", VoteApproveWithSuggestions)
	}
	if VoteNoVote != 0 {
		t.Errorf("VoteNoVote = %d, want 0", VoteNoVote)
	}
	if VoteWaitForAuthor != -5 {
		t.Errorf("VoteWaitForAuthor = %d, want -5", VoteWaitForAuthor)
	}
	if VoteReject != -10 {
		t.Errorf("VoteReject = %d, want -10", VoteReject)
	}
}

func TestFilterSystemThreads(t *testing.T) {
	tests := []struct {
		name     string
		threads  []Thread
		wantLen  int
		wantIDs  []int
	}{
		{
			name: "filters out Microsoft.VisualStudio comments",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "This looks good!", CommentType: "text"},
					},
				},
				{
					ID:     2,
					Status: "active",
					Comments: []Comment{
						{ID: 2, Content: "Microsoft.VisualStudio.Services.CodeReview.PolicyViolation", CommentType: "system"},
					},
				},
				{
					ID:     3,
					Status: "active",
					Comments: []Comment{
						{ID: 3, Content: "Please fix this", CommentType: "text"},
					},
				},
			},
			wantLen: 2,
			wantIDs: []int{1, 3},
		},
		{
			name: "filters threads where first comment starts with Microsoft.VisualStudio",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "Microsoft.VisualStudio.Discussion.Something", CommentType: "text"},
					},
				},
				{
					ID:     2,
					Status: "active",
					Comments: []Comment{
						{ID: 2, Content: "Good work!", CommentType: "text"},
					},
				},
			},
			wantLen: 1,
			wantIDs: []int{2},
		},
		{
			name:    "returns empty slice for empty input",
			threads: []Thread{},
			wantLen: 0,
			wantIDs: []int{},
		},
		{
			name: "keeps threads with no comments",
			threads: []Thread{
				{
					ID:       1,
					Status:   "active",
					Comments: []Comment{},
				},
			},
			wantLen: 1,
			wantIDs: []int{1},
		},
		{
			name: "filters all system threads",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "Microsoft.VisualStudio.Services.Something", CommentType: "system"},
					},
				},
				{
					ID:     2,
					Status: "active",
					Comments: []Comment{
						{ID: 2, Content: "Microsoft.VisualStudio.Another.Thing", CommentType: "system"},
					},
				},
			},
			wantLen: 0,
			wantIDs: []int{},
		},
		{
			name: "filters TFS reference update comments",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "Microsoft.VisualStudio.Services.TFS: The reference refs/heads/feature/test was updated.", CommentType: "system"},
					},
				},
				{
					ID:     2,
					Status: "active",
					Comments: []Comment{
						{ID: 2, Content: "Real comment here", CommentType: "text"},
					},
				},
			},
			wantLen: 1,
			wantIDs: []int{2},
		},
		{
			name: "filters comments with leading whitespace",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "  Microsoft.VisualStudio.Services.TFS: Something", CommentType: "system"},
					},
				},
			},
			wantLen: 0,
			wantIDs: []int{},
		},
		{
			name: "filters thread if ANY comment is system comment",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "", CommentType: "text"},
						{ID: 2, Content: "Microsoft.VisualStudio.Services.TFS: Updated reference", CommentType: "system"},
					},
				},
				{
					ID:     2,
					Status: "active",
					Comments: []Comment{
						{ID: 3, Content: "Real review comment", CommentType: "text"},
					},
				},
			},
			wantLen: 1,
			wantIDs: []int{2},
		},
		{
			name: "filters thread by system author name",
			threads: []Thread{
				{
					ID:     1,
					Status: "active",
					Comments: []Comment{
						{ID: 1, Content: "The reference refs/heads/feature/test was updated.", Author: Identity{DisplayName: "Microsoft.VisualStudio.Services.TFS"}},
					},
				},
				{
					ID:     2,
					Status: "active",
					Comments: []Comment{
						{ID: 2, Content: "Looks good!", Author: Identity{DisplayName: "John Doe"}},
					},
				},
			},
			wantLen: 1,
			wantIDs: []int{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterSystemThreads(tt.threads)
			if len(got) != tt.wantLen {
				t.Errorf("FilterSystemThreads() returned %d threads, want %d", len(got), tt.wantLen)
			}
			for i, wantID := range tt.wantIDs {
				if i >= len(got) {
					break
				}
				if got[i].ID != wantID {
					t.Errorf("FilterSystemThreads()[%d].ID = %d, want %d", i, got[i].ID, wantID)
				}
			}
		})
	}
}
