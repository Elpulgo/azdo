package azdevops

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

