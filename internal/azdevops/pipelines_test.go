package azdevops

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestListPipelineRuns_CustomTop(t *testing.T) {
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

	_, err = client.ListPipelineRuns(50)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}
}

func TestListPipelineRuns_NetworkError(t *testing.T) {
	// Create a client pointing to an invalid URL
	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = "http://invalid-host-that-does-not-exist.local"

	_, err = client.ListPipelineRuns(25)
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}
