package azdevops

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestListPipelineRuns_Success(t *testing.T) {
	// Create a test server that returns mock pipeline runs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify the API endpoint (path should be /build/builds since baseURL includes /_apis)
		expectedPath := "/build/builds"
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

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 2,
			"value": [
				{
					"id": 12345,
					"buildNumber": "20240206.1",
					"status": "completed",
					"result": "succeeded",
					"sourceBranch": "refs/heads/main",
					"sourceVersion": "abc123def456",
					"queueTime": "2024-02-06T10:00:00Z",
					"startTime": "2024-02-06T10:01:00Z",
					"finishTime": "2024-02-06T10:15:00Z",
					"definition": {
						"id": 42,
						"name": "CI-Pipeline"
					},
					"project": {
						"id": "proj-123",
						"name": "MyProject"
					},
					"_links": {
						"web": {
							"href": "https://dev.azure.com/org/proj/_build/results?buildId=12345"
						}
					}
				},
				{
					"id": 12346,
					"buildNumber": "20240206.2",
					"status": "inProgress",
					"result": null,
					"sourceBranch": "refs/heads/feature/test",
					"sourceVersion": "def456abc123",
					"queueTime": "2024-02-06T11:00:00Z",
					"startTime": "2024-02-06T11:01:00Z",
					"definition": {
						"id": 42,
						"name": "CI-Pipeline"
					},
					"project": {
						"id": "proj-123",
						"name": "MyProject"
					},
					"_links": {
						"web": {
							"href": "https://dev.azure.com/org/proj/_build/results?buildId=12346"
						}
					}
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

	// Call ListPipelineRuns
	runs, err := client.ListPipelineRuns(25)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	// Verify we got 2 runs
	if len(runs) != 2 {
		t.Fatalf("Expected 2 runs, got %d", len(runs))
	}

	// Verify first run
	run1 := runs[0]
	if run1.ID != 12345 {
		t.Errorf("runs[0].ID = %d, want 12345", run1.ID)
	}
	if run1.BuildNumber != "20240206.1" {
		t.Errorf("runs[0].BuildNumber = %s, want 20240206.1", run1.BuildNumber)
	}
	if run1.Status != "completed" {
		t.Errorf("runs[0].Status = %s, want completed", run1.Status)
	}
	if run1.Result != "succeeded" {
		t.Errorf("runs[0].Result = %s, want succeeded", run1.Result)
	}
	if run1.SourceBranch != "refs/heads/main" {
		t.Errorf("runs[0].SourceBranch = %s, want refs/heads/main", run1.SourceBranch)
	}
	if run1.Definition.Name != "CI-Pipeline" {
		t.Errorf("runs[0].Definition.Name = %s, want CI-Pipeline", run1.Definition.Name)
	}

	// Verify second run
	run2 := runs[1]
	if run2.ID != 12346 {
		t.Errorf("runs[1].ID = %d, want 12346", run2.ID)
	}
	if run2.Status != "inProgress" {
		t.Errorf("runs[1].Status = %s, want inProgress", run2.Status)
	}
	if run2.FinishTime != nil {
		t.Errorf("runs[1].FinishTime should be nil for in-progress run")
	}
}

func TestListPipelineRuns_EmptyList(t *testing.T) {
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

	runs, err := client.ListPipelineRuns(25)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if len(runs) != 0 {
		t.Errorf("Expected 0 runs, got %d", len(runs))
	}
}

func TestListPipelineRuns_HTTPError(t *testing.T) {
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

	_, err = client.ListPipelineRuns(25)
	if err == nil {
		t.Error("Expected error for 401 response, got nil")
	}
}

// TestListActivePipelineRuns_QueriesBothStatusesAndConcatenates pins that
// ListActivePipelineRuns issues two separate requests (statusFilter accepts
// only a single value) — one for "notStarted", one for "inProgress" — and
// concatenates their results, since this is what lets a build that's been
// queued long enough to fall outside ListPipelineRuns's $top window still
// surface via the merge in MultiClient.ListPipelineRuns.
func TestListActivePipelineRuns_QueriesBothStatusesAndConcatenates(t *testing.T) {
	var gotStatusFilters []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusFilter := r.URL.Query().Get("statusFilter")
		gotStatusFilters = append(gotStatusFilters, statusFilter)

		expectedPath := "/build/builds"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		var body string
		switch statusFilter {
		case "notStarted":
			body = `{"count": 1, "value": [{"id": 1, "status": "notStarted"}]}`
		case "inProgress":
			body = `{"count": 1, "value": [{"id": 2, "status": "inProgress"}]}`
		default:
			t.Errorf("unexpected statusFilter %q", statusFilter)
			body = `{"count": 0, "value": []}`
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	runs, err := client.ListActivePipelineRuns()
	if err != nil {
		t.Fatalf("ListActivePipelineRuns() error = %v", err)
	}

	if len(gotStatusFilters) != 2 {
		t.Fatalf("expected 2 requests, got %d (%v)", len(gotStatusFilters), gotStatusFilters)
	}

	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	ids := map[int]bool{runs[0].ID: true, runs[1].ID: true}
	if !ids[1] || !ids[2] {
		t.Errorf("expected runs with IDs 1 and 2, got %+v", runs)
	}
}

func TestListActivePipelineRuns_HTTPError(t *testing.T) {
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

	_, err = client.ListActivePipelineRuns()
	if err == nil {
		t.Error("Expected error for 401 response, got nil")
	}
}

// TestListMyFailedPipelineRuns_QueryParameters pins that
// ListMyFailedPipelineRuns narrows server-side via the List Builds 7.1
// query parameters this method exists to add (task 7 of the phase-2
// notifications spec, finding 1 of its review): statusFilter, resultFilter,
// requestedFor, minTime and queryOrder, none of which ListPipelineRuns
// sends.
func TestListMyFailedPipelineRuns_QueryParameters(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		expectedPath := "/build/builds"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
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

	_, err = client.ListMyFailedPipelineRuns("user-1", 14, 25)
	if err != nil {
		t.Fatalf("ListMyFailedPipelineRuns() error = %v", err)
	}

	if got := gotQuery.Get("api-version"); got != "7.1" {
		t.Errorf("api-version = %q, want 7.1", got)
	}
	if got := gotQuery.Get("statusFilter"); got != "completed" {
		t.Errorf("statusFilter = %q, want completed", got)
	}
	if got := gotQuery.Get("resultFilter"); got != "failed" {
		t.Errorf("resultFilter = %q, want failed", got)
	}
	if got := gotQuery.Get("requestedFor"); got != "user-1" {
		t.Errorf("requestedFor = %q, want user-1", got)
	}
	if got := gotQuery.Get("$top"); got != "25" {
		t.Errorf("$top = %q, want 25", got)
	}
	if got := gotQuery.Get("queryOrder"); got != "finishTimeDescending" {
		t.Errorf("queryOrder = %q, want finishTimeDescending", got)
	}
	minTime := gotQuery.Get("minTime")
	if minTime == "" {
		t.Fatal("minTime is empty, want an RFC3339 timestamp roughly 14 days in the past")
	}
	parsed, err := time.Parse(time.RFC3339, minTime)
	if err != nil {
		t.Fatalf("minTime = %q is not a valid RFC3339 timestamp: %v", minTime, err)
	}
	wantAround := time.Now().Add(-14 * 24 * time.Hour)
	if diff := parsed.Sub(wantAround); diff < -time.Minute || diff > time.Minute {
		t.Errorf("minTime = %v, want within a minute of now-14d (%v)", parsed, wantAround)
	}
}

// TestListMyFailedPipelineRuns_NegativeLookbackDaysClampedToZero pins that a
// negative lookbackDays is clamped to 0, matching
// Client.ListRecentlyAssignedWorkItems's own guard: an un-clamped negative
// would put minTime in the future and silently return zero rows rather than
// failing loudly.
func TestListMyFailedPipelineRuns_NegativeLookbackDaysClampedToZero(t *testing.T) {
	var gotMinTime string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMinTime = r.URL.Query().Get("minTime")
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

	_, err = client.ListMyFailedPipelineRuns("user-1", -5, 25)
	if err != nil {
		t.Fatalf("ListMyFailedPipelineRuns() error = %v", err)
	}

	parsed, err := time.Parse(time.RFC3339, gotMinTime)
	if err != nil {
		t.Fatalf("minTime = %q is not a valid RFC3339 timestamp: %v", gotMinTime, err)
	}
	if diff := time.Since(parsed); diff < -time.Minute || diff > time.Minute {
		t.Errorf("minTime = %v, want within a minute of now (lookbackDays clamped to 0), got a diff of %v", parsed, diff)
	}
}

func TestListMyFailedPipelineRuns_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 1,
			"value": [
				{
					"id": 12345,
					"buildNumber": "20240206.1",
					"status": "completed",
					"result": "failed",
					"queueTime": "2024-02-06T10:00:00Z",
					"finishTime": "2024-02-06T10:15:00Z",
					"definition": {"id": 42, "name": "CI-Pipeline"},
					"requestedFor": {"id": "user-1", "displayName": "Me"}
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

	runs, err := client.ListMyFailedPipelineRuns("user-1", 14, 25)
	if err != nil {
		t.Fatalf("ListMyFailedPipelineRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].RequestedFor.ID != "user-1" {
		t.Errorf("RequestedFor.ID = %q, want user-1", runs[0].RequestedFor.ID)
	}
}

func TestListMyFailedPipelineRuns_HTTPError(t *testing.T) {
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

	_, err = client.ListMyFailedPipelineRuns("user-1", 14, 25)
	if err == nil {
		t.Error("Expected error for 401 response, got nil")
	}
}

func TestListPipelineRuns_InvalidJSON(t *testing.T) {
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

	_, err = client.ListPipelineRuns(25)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
