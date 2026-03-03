package azdevops

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetBuildTimeline_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify the API endpoint
		expectedPath := "/build/builds/12345/timeline"
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
			"id": "timeline-12345",
			"changeId": 10,
			"records": [
				{
					"id": "stage-build",
					"parentId": null,
					"type": "Stage",
					"name": "Build",
					"state": "completed",
					"result": "succeeded",
					"order": 1,
					"startTime": "2024-02-06T10:00:00Z",
					"finishTime": "2024-02-06T10:10:00Z"
				},
				{
					"id": "job-compile",
					"parentId": "stage-build",
					"type": "Job",
					"name": "Compile",
					"state": "completed",
					"result": "succeeded",
					"order": 1,
					"startTime": "2024-02-06T10:00:30Z",
					"finishTime": "2024-02-06T10:09:30Z",
					"log": {
						"id": 5,
						"type": "Container",
						"url": "https://dev.azure.com/org/proj/_apis/build/builds/12345/logs/5"
					}
				},
				{
					"id": "task-npm",
					"parentId": "job-compile",
					"type": "Task",
					"name": "npm install",
					"state": "completed",
					"result": "succeeded",
					"order": 1,
					"startTime": "2024-02-06T10:01:00Z",
					"finishTime": "2024-02-06T10:03:00Z",
					"log": {
						"id": 6,
						"type": "Container",
						"url": "https://dev.azure.com/org/proj/_apis/build/builds/12345/logs/6"
					}
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

	timeline, err := client.GetBuildTimeline(12345)
	if err != nil {
		t.Fatalf("GetBuildTimeline() error = %v", err)
	}

	if timeline.ID != "timeline-12345" {
		t.Errorf("timeline.ID = %v, want timeline-12345", timeline.ID)
	}

	if len(timeline.Records) != 3 {
		t.Fatalf("len(timeline.Records) = %v, want 3", len(timeline.Records))
	}

	// Verify stage
	if timeline.Records[0].Type != "Stage" {
		t.Errorf("Records[0].Type = %v, want Stage", timeline.Records[0].Type)
	}
	if timeline.Records[0].Name != "Build" {
		t.Errorf("Records[0].Name = %v, want Build", timeline.Records[0].Name)
	}

	// Verify job has log
	if timeline.Records[1].Log == nil {
		t.Error("Records[1].Log should not be nil")
	} else if timeline.Records[1].Log.ID != 5 {
		t.Errorf("Records[1].Log.ID = %v, want 5", timeline.Records[1].Log.ID)
	}
}

func TestGetBuildTimeline_EmptyTimeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": "empty-timeline", "changeId": 0, "records": []}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	timeline, err := client.GetBuildTimeline(999)
	if err != nil {
		t.Fatalf("GetBuildTimeline() error = %v", err)
	}

	if len(timeline.Records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(timeline.Records))
	}
}

func TestGetBuildTimeline_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Build not found"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.GetBuildTimeline(99999)
	if err == nil {
		t.Error("Expected error for 404 response, got nil")
	}
}

func TestGetBuildTimeline_InvalidJSON(t *testing.T) {
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

	_, err = client.GetBuildTimeline(12345)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

