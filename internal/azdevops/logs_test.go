package azdevops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildLogUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		want     BuildLog
		wantErr  bool
	}{
		{
			name: "complete log entry",
			jsonData: `{
				"id": 5,
				"type": "Container",
				"url": "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/5",
				"lineCount": 150,
				"createdOn": "2024-02-06T10:00:00Z",
				"lastChangedOn": "2024-02-06T10:05:00Z"
			}`,
			want: BuildLog{
				ID:            5,
				Type:          "Container",
				URL:           "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/5",
				LineCount:     150,
				CreatedOn:     parseTimePtr(t, "2024-02-06T10:00:00Z"),
				LastChangedOn: parseTimePtr(t, "2024-02-06T10:05:00Z"),
			},
			wantErr: false,
		},
		{
			name: "log entry without line count",
			jsonData: `{
				"id": 10,
				"type": "Container",
				"url": "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/10",
				"createdOn": "2024-02-06T11:00:00Z"
			}`,
			want: BuildLog{
				ID:        10,
				Type:      "Container",
				URL:       "https://dev.azure.com/org/proj/_apis/build/builds/123/logs/10",
				LineCount: 0,
				CreatedOn: parseTimePtr(t, "2024-02-06T11:00:00Z"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got BuildLog
			err := json.Unmarshal([]byte(tt.jsonData), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if got.ID != tt.want.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.want.ID)
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.URL != tt.want.URL {
				t.Errorf("URL = %v, want %v", got.URL, tt.want.URL)
			}
			if got.LineCount != tt.want.LineCount {
				t.Errorf("LineCount = %v, want %v", got.LineCount, tt.want.LineCount)
			}
		})
	}
}

func TestBuildLogsResponseUnmarshal(t *testing.T) {
	jsonData := `{
		"count": 3,
		"value": [
			{"id": 1, "type": "Container", "url": "url1", "lineCount": 10},
			{"id": 2, "type": "Container", "url": "url2", "lineCount": 20},
			{"id": 3, "type": "Container", "url": "url3", "lineCount": 30}
		]
	}`

	var response BuildLogsResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if response.Count != 3 {
		t.Errorf("Count = %v, want 3", response.Count)
	}

	if len(response.Value) != 3 {
		t.Fatalf("len(Value) = %v, want 3", len(response.Value))
	}

	if response.Value[0].ID != 1 {
		t.Errorf("Value[0].ID = %v, want 1", response.Value[0].ID)
	}
	if response.Value[2].LineCount != 30 {
		t.Errorf("Value[2].LineCount = %v, want 30", response.Value[2].LineCount)
	}
}

func TestListBuildLogs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		expectedPath := "/build/builds/12345/logs"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		query := r.URL.Query()
		if query.Get("api-version") != "7.1" {
			t.Errorf("Expected api-version=7.1, got %s", query.Get("api-version"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 2,
			"value": [
				{
					"id": 5,
					"type": "Container",
					"url": "https://dev.azure.com/org/proj/_apis/build/builds/12345/logs/5",
					"lineCount": 100
				},
				{
					"id": 6,
					"type": "Container",
					"url": "https://dev.azure.com/org/proj/_apis/build/builds/12345/logs/6",
					"lineCount": 250
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

	logs, err := client.ListBuildLogs(12345)
	if err != nil {
		t.Fatalf("ListBuildLogs() error = %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("len(logs) = %v, want 2", len(logs))
	}

	if logs[0].ID != 5 {
		t.Errorf("logs[0].ID = %v, want 5", logs[0].ID)
	}
	if logs[1].LineCount != 250 {
		t.Errorf("logs[1].LineCount = %v, want 250", logs[1].LineCount)
	}
}

func TestListBuildLogs_EmptyList(t *testing.T) {
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

	logs, err := client.ListBuildLogs(999)
	if err != nil {
		t.Fatalf("ListBuildLogs() error = %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("Expected 0 logs, got %d", len(logs))
	}
}

func TestListBuildLogs_HTTPError(t *testing.T) {
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

	_, err = client.ListBuildLogs(99999)
	if err == nil {
		t.Error("Expected error for 404 response, got nil")
	}
}

func TestGetBuildLogContent_Success(t *testing.T) {
	expectedContent := `2024-02-06T10:00:00.000Z Starting npm install...
2024-02-06T10:00:01.000Z added 1234 packages in 45s
2024-02-06T10:00:02.000Z npm install completed successfully`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		expectedPath := "/build/builds/12345/logs/5"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		query := r.URL.Query()
		if query.Get("api-version") != "7.1" {
			t.Errorf("Expected api-version=7.1, got %s", query.Get("api-version"))
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedContent))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	content, err := client.GetBuildLogContent(12345, 5)
	if err != nil {
		t.Fatalf("GetBuildLogContent() error = %v", err)
	}

	if content != expectedContent {
		t.Errorf("content = %q, want %q", content, expectedContent)
	}
}

func TestGetBuildLogContent_EmptyLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	content, err := client.GetBuildLogContent(12345, 5)
	if err != nil {
		t.Fatalf("GetBuildLogContent() error = %v", err)
	}

	if content != "" {
		t.Errorf("Expected empty string, got %q", content)
	}
}

func TestGetBuildLogContent_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Log not found"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	_, err = client.GetBuildLogContent(12345, 999)
	if err == nil {
		t.Error("Expected error for 404 response, got nil")
	}
}

func TestGetBuildLogContent_LargeLog(t *testing.T) {
	// Simulate a large log with many lines
	var builder strings.Builder
	for i := 0; i < 1000; i++ {
		builder.WriteString("Log line ")
		builder.WriteString(string(rune('0' + i%10)))
		builder.WriteString("\n")
	}
	largeContent := builder.String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeContent))
	}))
	defer server.Close()

	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = server.URL

	content, err := client.GetBuildLogContent(12345, 5)
	if err != nil {
		t.Fatalf("GetBuildLogContent() error = %v", err)
	}

	if content != largeContent {
		t.Errorf("content length = %d, want %d", len(content), len(largeContent))
	}
}

func TestGetBuildLogContent_NetworkError(t *testing.T) {
	client, err := NewClient("test-org", "test-project", "test-pat")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.baseURL = "http://invalid-host-that-does-not-exist.local"

	_, err = client.GetBuildLogContent(12345, 5)
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}
