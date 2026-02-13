package azdevops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkItem_TypeIcon(t *testing.T) {
	tests := []struct {
		workItemType string
		want         string
	}{
		{"Bug", "🐛"},
		{"Task", "📋"},
		{"User Story", "📖"},
		{"Feature", "⭐"},
		{"Epic", "🎯"},
		{"Issue", "❗"},
		{"Unknown", "📄"},
		{"", "📄"},
	}

	for _, tt := range tests {
		t.Run(tt.workItemType, func(t *testing.T) {
			wi := WorkItem{Fields: WorkItemFields{WorkItemType: tt.workItemType}}
			got := wi.TypeIcon()
			if got != tt.want {
				t.Errorf("TypeIcon() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkItem_StateIcon(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"New", "○"},
		{"new", "○"},
		{"Active", "◐"},
		{"active", "◐"},
		{"Resolved", "●"},
		{"resolved", "●"},
		{"Ready for Test", "●"},
		{"Ready For Test", "●"},
		{"ready for test", "●"},
		{"Closed", "✓"},
		{"closed", "✓"},
		{"Removed", "✗"},
		{"removed", "✗"},
		{"Unknown", "○"},
		{"", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			wi := WorkItem{Fields: WorkItemFields{State: tt.state}}
			got := wi.StateIcon()
			if got != tt.want {
				t.Errorf("StateIcon() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkItem_AssignedToName(t *testing.T) {
	tests := []struct {
		name       string
		assignedTo *Identity
		want       string
	}{
		{
			name:       "nil assignedTo",
			assignedTo: nil,
			want:       "-",
		},
		{
			name:       "with assignedTo",
			assignedTo: &Identity{DisplayName: "John Doe"},
			want:       "John Doe",
		},
		{
			name:       "empty displayName",
			assignedTo: &Identity{DisplayName: ""},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wi := WorkItem{Fields: WorkItemFields{AssignedTo: tt.assignedTo}}
			got := wi.AssignedToName()
			if got != tt.want {
				t.Errorf("AssignedToName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_QueryWorkItemIDs(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/test-org/test-project/_apis/wit/wiql" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		// Return mock response
		response := WIQLResponse{
			WorkItems: []WorkItemReference{
				{ID: 123, URL: "http://example.com/123"},
				{ID: 456, URL: "http://example.com/456"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with mock server
	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	ids, err := client.QueryWorkItemIDs("SELECT [System.Id] FROM WorkItems", 50)
	if err != nil {
		t.Fatalf("QueryWorkItemIDs() error = %v", err)
	}

	if len(ids) != 2 {
		t.Errorf("Expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != 123 || ids[1] != 456 {
		t.Errorf("Expected [123, 456], got %v", ids)
	}
}

func TestClient_GetWorkItems(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}

		// Return mock response
		response := WorkItemsResponse{
			Count: 2,
			Value: []WorkItem{
				{
					ID:  123,
					Rev: 1,
					Fields: WorkItemFields{
						Title:        "Fix bug",
						State:        "Active",
						WorkItemType: "Bug",
						Priority:     1,
					},
				},
				{
					ID:  456,
					Rev: 2,
					Fields: WorkItemFields{
						Title:        "Add feature",
						State:        "New",
						WorkItemType: "Task",
						Priority:     2,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with mock server
	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	workItems, err := client.GetWorkItems([]int{123, 456})
	if err != nil {
		t.Fatalf("GetWorkItems() error = %v", err)
	}

	if len(workItems) != 2 {
		t.Errorf("Expected 2 work items, got %d", len(workItems))
	}
	if workItems[0].ID != 123 || workItems[0].Fields.Title != "Fix bug" {
		t.Errorf("Work item 0 mismatch: %+v", workItems[0])
	}
}

func TestClient_GetWorkItems_EmptyIDs(t *testing.T) {
	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    "http://example.com",
		httpClient: http.DefaultClient,
	}

	workItems, err := client.GetWorkItems([]int{})
	if err != nil {
		t.Fatalf("GetWorkItems() error = %v", err)
	}

	if len(workItems) != 0 {
		t.Errorf("Expected empty slice, got %d items", len(workItems))
	}
}

func TestClient_ListWorkItems(t *testing.T) {
	callCount := 0

	// Create mock server that handles both WIQL and workitems endpoints
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if r.Method == "POST" {
			// WIQL endpoint
			response := WIQLResponse{
				WorkItems: []WorkItemReference{
					{ID: 100, URL: "http://example.com/100"},
					{ID: 200, URL: "http://example.com/200"},
				},
			}
			json.NewEncoder(w).Encode(response)
		} else {
			// GetWorkItems endpoint
			response := WorkItemsResponse{
				Count: 2,
				Value: []WorkItem{
					{ID: 100, Fields: WorkItemFields{Title: "Item 1", State: "Active"}},
					{ID: 200, Fields: WorkItemFields{Title: "Item 2", State: "New"}},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	workItems, err := client.ListWorkItems(50)
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	if len(workItems) != 2 {
		t.Errorf("Expected 2 work items, got %d", len(workItems))
	}
	if callCount != 2 {
		t.Errorf("Expected 2 API calls (WIQL + GetWorkItems), got %d", callCount)
	}
}

func TestClient_ListWorkItems_NoResults(t *testing.T) {
	// Create mock server that returns empty WIQL results
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := WIQLResponse{
			WorkItems: []WorkItemReference{},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	workItems, err := client.ListWorkItems(50)
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	if len(workItems) != 0 {
		t.Errorf("Expected 0 work items, got %d", len(workItems))
	}
}
