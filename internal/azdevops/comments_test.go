package azdevops

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(serverURL string) *Client {
	return &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    serverURL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}
}

func TestClient_GetWorkItemComments(t *testing.T) {
	var capturedMethod, capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedQuery = r.URL.RawQuery
		w.Write([]byte(`{
			"totalCount": 2,
			"count": 2,
			"comments": [
				{
					"id": 45,
					"text": "Newest comment",
					"createdBy": { "displayName": "Jane Doe", "uniqueName": "jane@x.com", "id": "id-1" },
					"createdDate": "2019-01-21T20:12:14.683Z"
				},
				{
					"id": 44,
					"text": "Older comment",
					"createdBy": { "displayName": "John Roe", "uniqueName": "john@x.com", "id": "id-2" },
					"createdDate": "2019-01-20T23:26:33.383Z"
				}
			]
		}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	comments, err := client.GetWorkItemComments(299)
	if err != nil {
		t.Fatalf("GetWorkItemComments() error = %v", err)
	}

	if capturedMethod != "GET" {
		t.Errorf("Expected GET, got %s", capturedMethod)
	}
	for _, want := range []string{"api-version=7.1-preview.4", "order=desc", "top=200"} {
		if !strings.Contains(capturedQuery, want) {
			t.Errorf("Expected query to contain %q, got %q", want, capturedQuery)
		}
	}

	if len(comments) != 2 {
		t.Fatalf("Expected 2 comments, got %d", len(comments))
	}
	if comments[0].Text != "Newest comment" {
		t.Errorf("comments[0].Text = %q, want %q", comments[0].Text, "Newest comment")
	}
	if comments[0].CreatedBy.DisplayName != "Jane Doe" {
		t.Errorf("comments[0].CreatedBy.DisplayName = %q, want %q", comments[0].CreatedBy.DisplayName, "Jane Doe")
	}
	if comments[0].CreatedDate.IsZero() {
		t.Errorf("comments[0].CreatedDate should be parsed, got zero")
	}
	if comments[0].ID != 45 {
		t.Errorf("comments[0].ID = %d, want 45", comments[0].ID)
	}
}

func TestClient_GetWorkItemComments_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	if _, err := client.GetWorkItemComments(1); err == nil {
		t.Fatal("Expected error for 500 response, got nil")
	}
}

func TestClient_AddWorkItemComment(t *testing.T) {
	var capturedMethod, capturedBody, capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedQuery = r.URL.RawQuery
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		w.Write([]byte(`{
			"id": 100,
			"text": "Hello \"world\"",
			"createdBy": { "displayName": "Jane Doe" },
			"createdDate": "2019-01-21T20:12:14.683Z"
		}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	comment, err := client.AddWorkItemComment(299, `Hello "world"`)
	if err != nil {
		t.Fatalf("AddWorkItemComment() error = %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("Expected POST, got %s", capturedMethod)
	}
	if !strings.Contains(capturedQuery, "api-version=7.1-preview.4") {
		t.Errorf("Expected query to contain api-version=7.1-preview.4, got %q", capturedQuery)
	}
	// Body must be valid JSON with a "text" field, and the quotes must be escaped.
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(capturedBody), &payload); err != nil {
		t.Fatalf("request body is not valid JSON: %v (body=%s)", err, capturedBody)
	}
	if payload.Text != `Hello "world"` {
		t.Errorf("payload.Text = %q, want %q", payload.Text, `Hello "world"`)
	}

	if comment == nil || comment.ID != 100 {
		t.Fatalf("Expected returned comment with ID 100, got %+v", comment)
	}
	if comment.Text != `Hello "world"` {
		t.Errorf("comment.Text = %q, want %q", comment.Text, `Hello "world"`)
	}
}

func TestClient_AddWorkItemComment_RejectsEmpty(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	for _, text := range []string{"", "   ", "\n\t  "} {
		if _, err := client.AddWorkItemComment(1, text); err == nil {
			t.Errorf("AddWorkItemComment(%q) expected error, got nil", text)
		}
	}
	if called {
		t.Error("Expected no HTTP call for empty/whitespace text")
	}
}
