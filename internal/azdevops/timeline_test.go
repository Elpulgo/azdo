package azdevops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTimelineRecordUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		want     TimelineRecord
		wantErr  bool
	}{
		{
			name: "stage record",
			jsonData: `{
				"id": "stage-1-id",
				"parentId": null,
				"type": "Stage",
				"name": "Build",
				"state": "completed",
				"result": "succeeded",
				"order": 1,
				"startTime": "2024-02-06T10:00:00Z",
				"finishTime": "2024-02-06T10:05:00Z",
				"log": {
					"id": 10,
					"type": "Container",
					"url": "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/10"
				}
			}`,
			want: TimelineRecord{
				ID:         "stage-1-id",
				ParentID:   nil,
				Type:       "Stage",
				Name:       "Build",
				State:      "completed",
				Result:     "succeeded",
				Order:      1,
				StartTime:  parseTimePtr(t, "2024-02-06T10:00:00Z"),
				FinishTime: parseTimePtr(t, "2024-02-06T10:05:00Z"),
				Log: &LogReference{
					ID:   10,
					Type: "Container",
					URL:  "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/10",
				},
			},
			wantErr: false,
		},
		{
			name: "job record with parent",
			jsonData: `{
				"id": "job-1-id",
				"parentId": "stage-1-id",
				"type": "Job",
				"name": "Build Job",
				"state": "completed",
				"result": "succeeded",
				"order": 1,
				"startTime": "2024-02-06T10:00:30Z",
				"finishTime": "2024-02-06T10:04:30Z",
				"log": null
			}`,
			want: TimelineRecord{
				ID:         "job-1-id",
				ParentID:   strPtr("stage-1-id"),
				Type:       "Job",
				Name:       "Build Job",
				State:      "completed",
				Result:     "succeeded",
				Order:      1,
				StartTime:  parseTimePtr(t, "2024-02-06T10:00:30Z"),
				FinishTime: parseTimePtr(t, "2024-02-06T10:04:30Z"),
				Log:        nil,
			},
			wantErr: false,
		},
		{
			name: "task record in progress",
			jsonData: `{
				"id": "task-1-id",
				"parentId": "job-1-id",
				"type": "Task",
				"name": "Run npm install",
				"state": "inProgress",
				"result": null,
				"order": 2,
				"startTime": "2024-02-06T10:01:00Z",
				"finishTime": null,
				"log": {
					"id": 15,
					"type": "Container",
					"url": "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/15"
				}
			}`,
			want: TimelineRecord{
				ID:         "task-1-id",
				ParentID:   strPtr("job-1-id"),
				Type:       "Task",
				Name:       "Run npm install",
				State:      "inProgress",
				Result:     "",
				Order:      2,
				StartTime:  parseTimePtr(t, "2024-02-06T10:01:00Z"),
				FinishTime: nil,
				Log: &LogReference{
					ID:   15,
					Type: "Container",
					URL:  "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/15",
				},
			},
			wantErr: false,
		},
		{
			name: "failed task record",
			jsonData: `{
				"id": "task-2-id",
				"parentId": "job-1-id",
				"type": "Task",
				"name": "Run tests",
				"state": "completed",
				"result": "failed",
				"order": 3,
				"startTime": "2024-02-06T10:02:00Z",
				"finishTime": "2024-02-06T10:03:00Z",
				"log": {
					"id": 16,
					"type": "Container",
					"url": "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/16"
				},
				"issues": [
					{
						"type": "error",
						"message": "Test failed: expected 1 but got 2"
					}
				]
			}`,
			want: TimelineRecord{
				ID:         "task-2-id",
				ParentID:   strPtr("job-1-id"),
				Type:       "Task",
				Name:       "Run tests",
				State:      "completed",
				Result:     "failed",
				Order:      3,
				StartTime:  parseTimePtr(t, "2024-02-06T10:02:00Z"),
				FinishTime: parseTimePtr(t, "2024-02-06T10:03:00Z"),
				Log: &LogReference{
					ID:   16,
					Type: "Container",
					URL:  "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/16",
				},
				Issues: []Issue{
					{
						Type:    "error",
						Message: "Test failed: expected 1 but got 2",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got TimelineRecord
			err := json.Unmarshal([]byte(tt.jsonData), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Compare fields
			if got.ID != tt.want.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.want.ID)
			}
			if !strPointersEqual(got.ParentID, tt.want.ParentID) {
				t.Errorf("ParentID = %v, want %v", formatStrPtr(got.ParentID), formatStrPtr(tt.want.ParentID))
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", got.Name, tt.want.Name)
			}
			if got.State != tt.want.State {
				t.Errorf("State = %v, want %v", got.State, tt.want.State)
			}
			if got.Result != tt.want.Result {
				t.Errorf("Result = %v, want %v", got.Result, tt.want.Result)
			}
			if got.Order != tt.want.Order {
				t.Errorf("Order = %v, want %v", got.Order, tt.want.Order)
			}
			if !timePointersEqual(got.StartTime, tt.want.StartTime) {
				t.Errorf("StartTime = %v, want %v", formatTimePtr(got.StartTime), formatTimePtr(tt.want.StartTime))
			}
			if !timePointersEqual(got.FinishTime, tt.want.FinishTime) {
				t.Errorf("FinishTime = %v, want %v", formatTimePtr(got.FinishTime), formatTimePtr(tt.want.FinishTime))
			}
			if !logReferencesEqual(got.Log, tt.want.Log) {
				t.Errorf("Log = %v, want %v", got.Log, tt.want.Log)
			}
			if len(got.Issues) != len(tt.want.Issues) {
				t.Errorf("len(Issues) = %v, want %v", len(got.Issues), len(tt.want.Issues))
			} else {
				for i, issue := range got.Issues {
					if issue.Type != tt.want.Issues[i].Type {
						t.Errorf("Issues[%d].Type = %v, want %v", i, issue.Type, tt.want.Issues[i].Type)
					}
					if issue.Message != tt.want.Issues[i].Message {
						t.Errorf("Issues[%d].Message = %v, want %v", i, issue.Message, tt.want.Issues[i].Message)
					}
				}
			}
		})
	}
}

func TestTimelineUnmarshal(t *testing.T) {
	jsonData := `{
		"id": "timeline-123",
		"changeId": 5,
		"lastChangedBy": "user-id",
		"lastChangedOn": "2024-02-06T10:05:00Z",
		"records": [
			{
				"id": "stage-1",
				"parentId": null,
				"type": "Stage",
				"name": "Build",
				"state": "completed",
				"result": "succeeded",
				"order": 1,
				"startTime": "2024-02-06T10:00:00Z",
				"finishTime": "2024-02-06T10:05:00Z"
			},
			{
				"id": "job-1",
				"parentId": "stage-1",
				"type": "Job",
				"name": "Build Job",
				"state": "completed",
				"result": "succeeded",
				"order": 1,
				"startTime": "2024-02-06T10:00:30Z",
				"finishTime": "2024-02-06T10:04:30Z"
			}
		]
	}`

	var timeline Timeline
	err := json.Unmarshal([]byte(jsonData), &timeline)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if timeline.ID != "timeline-123" {
		t.Errorf("ID = %v, want timeline-123", timeline.ID)
	}
	if timeline.ChangeID != 5 {
		t.Errorf("ChangeID = %v, want 5", timeline.ChangeID)
	}
	if len(timeline.Records) != 2 {
		t.Fatalf("len(Records) = %v, want 2", len(timeline.Records))
	}

	// Check stage record
	stage := timeline.Records[0]
	if stage.Type != "Stage" {
		t.Errorf("Records[0].Type = %v, want Stage", stage.Type)
	}
	if stage.Name != "Build" {
		t.Errorf("Records[0].Name = %v, want Build", stage.Name)
	}

	// Check job record
	job := timeline.Records[1]
	if job.Type != "Job" {
		t.Errorf("Records[1].Type = %v, want Job", job.Type)
	}
	if *job.ParentID != "stage-1" {
		t.Errorf("Records[1].ParentID = %v, want stage-1", *job.ParentID)
	}
}

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

func TestGetBuildTimeline_NetworkError(t *testing.T) {
	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = "http://invalid-host-that-does-not-exist.local"

	_, err = client.GetBuildTimeline(12345)
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func strPointersEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func formatStrPtr(s *string) string {
	if s == nil {
		return "nil"
	}
	return *s
}

func logReferencesEqual(a, b *LogReference) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ID == b.ID && a.Type == b.Type && a.URL == b.URL
}
