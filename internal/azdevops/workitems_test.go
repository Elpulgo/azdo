package azdevops

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestWorkItemFields_ReproStepsDeserialization(t *testing.T) {
	// Bug work items in Azure DevOps use Microsoft.VSTS.TCM.ReproSteps
	// instead of System.Description for their description content
	jsonData := `{
		"System.Title": "A bug",
		"System.WorkItemType": "Bug",
		"Microsoft.VSTS.TCM.ReproSteps": "<div>Steps to reproduce</div>"
	}`

	var fields WorkItemFields
	if err := json.Unmarshal([]byte(jsonData), &fields); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if fields.ReproSteps != "<div>Steps to reproduce</div>" {
		t.Errorf("ReproSteps = %q, want %q", fields.ReproSteps, "<div>Steps to reproduce</div>")
	}
}

func TestWorkItem_EffectiveDescription(t *testing.T) {
	tests := []struct {
		name         string
		workItemType string
		description  string
		reproSteps   string
		want         string
	}{
		{
			name:         "Bug with ReproSteps uses ReproSteps",
			workItemType: "Bug",
			description:  "",
			reproSteps:   "Steps to reproduce the bug",
			want:         "Steps to reproduce the bug",
		},
		{
			name:         "Bug with both fields uses ReproSteps",
			workItemType: "Bug",
			description:  "Some description",
			reproSteps:   "Steps to reproduce",
			want:         "Steps to reproduce",
		},
		{
			name:         "Bug with only Description falls back to Description",
			workItemType: "Bug",
			description:  "Bug description",
			reproSteps:   "",
			want:         "Bug description",
		},
		{
			name:         "Task uses Description",
			workItemType: "Task",
			description:  "Task description",
			reproSteps:   "",
			want:         "Task description",
		},
		{
			name:         "User Story uses Description",
			workItemType: "User Story",
			description:  "Story description",
			reproSteps:   "",
			want:         "Story description",
		},
		{
			name:         "Bug with neither field returns empty",
			workItemType: "Bug",
			description:  "",
			reproSteps:   "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wi := WorkItem{
				Fields: WorkItemFields{
					WorkItemType: tt.workItemType,
					Description:  tt.description,
					ReproSteps:   tt.reproSteps,
				},
			}
			got := wi.EffectiveDescription()
			if got != tt.want {
				t.Errorf("EffectiveDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_GetWorkItems_RequestsReproStepsField(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		response := WorkItemsResponse{
			Count: 1,
			Value: []WorkItem{
				{ID: 1, Fields: WorkItemFields{Title: "Bug", WorkItemType: "Bug"}},
			},
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

	_, err := client.GetWorkItems([]int{1})
	if err != nil {
		t.Fatalf("GetWorkItems() error = %v", err)
	}

	if !strings.Contains(capturedPath, "Microsoft.VSTS.TCM.ReproSteps") {
		t.Errorf("GetWorkItems request must include Microsoft.VSTS.TCM.ReproSteps field.\nGot path: %s", capturedPath)
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

func TestClient_ListWorkItems_QueryScopedToProject(t *testing.T) {
	// Verify the WIQL query includes project scoping to prevent duplicates
	// when multiple project clients query simultaneously
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			bodyBytes, _ := io.ReadAll(r.Body)
			capturedBody = string(bodyBytes)
			response := WIQLResponse{
				WorkItems: []WorkItemReference{
					{ID: 100, URL: "http://example.com/100"},
				},
			}
			json.NewEncoder(w).Encode(response)
		} else {
			response := WorkItemsResponse{
				Count: 1,
				Value: []WorkItem{
					{ID: 100, Fields: WorkItemFields{Title: "Item 1", State: "Active"}},
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

	_, err := client.ListWorkItems(50)
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	if !strings.Contains(capturedBody, "@project") {
		t.Errorf("WIQL query must scope to @project to prevent duplicates in multi-project mode.\nGot query body: %s", capturedBody)
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

func TestClient_ListMyWorkItems_QueryContainsAtMe(t *testing.T) {
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			bodyBytes, _ := io.ReadAll(r.Body)
			capturedBody = string(bodyBytes)
			response := WIQLResponse{WorkItems: []WorkItemReference{}}
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

	_, err := client.ListMyWorkItems(50)
	if err != nil {
		t.Fatalf("ListMyWorkItems() error = %v", err)
	}

	if !strings.Contains(capturedBody, "@Me") {
		t.Errorf("WIQL query must contain @Me macro.\nGot query body: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "@project") {
		t.Errorf("WIQL query must scope to @project.\nGot query body: %s", capturedBody)
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

func TestClient_GetWorkItemTypeStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/wit/workitemtypes/Bug/states") {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		response := WorkItemTypeStatesResponse{
			Count: 4,
			Value: []WorkItemTypeState{
				{Name: "New", Color: "b2b2b2", Category: "Proposed"},
				{Name: "Active", Color: "007acc", Category: "InProgress"},
				{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
				{Name: "Closed", Color: "339933", Category: "Completed"},
			},
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

	states, err := client.GetWorkItemTypeStates("Bug")
	if err != nil {
		t.Fatalf("GetWorkItemTypeStates() error = %v", err)
	}

	if len(states) != 4 {
		t.Fatalf("Expected 4 states, got %d", len(states))
	}
	if states[0].Name != "New" {
		t.Errorf("Expected first state 'New', got %q", states[0].Name)
	}
	if states[0].Category != "Proposed" {
		t.Errorf("Expected category 'Proposed', got %q", states[0].Category)
	}
}

func TestClient_GetWorkItemTypeStates_ExcludesRemovedCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := WorkItemTypeStatesResponse{
			Count: 5,
			Value: []WorkItemTypeState{
				{Name: "New", Color: "b2b2b2", Category: "Proposed"},
				{Name: "Active", Color: "007acc", Category: "InProgress"},
				{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
				{Name: "Closed", Color: "339933", Category: "Completed"},
				{Name: "Removed", Color: "ffffff", Category: "Removed"},
			},
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

	states, err := client.GetWorkItemTypeStates("Bug")
	if err != nil {
		t.Fatalf("GetWorkItemTypeStates() error = %v", err)
	}

	for _, s := range states {
		if s.Category == "Removed" {
			t.Errorf("GetWorkItemTypeStates should exclude 'Removed' category, but found state %q", s.Name)
		}
	}
}

func TestClient_UpdateWorkItemState(t *testing.T) {
	var capturedBody string
	var capturedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("Expected PATCH, got %s", r.Method)
		}
		capturedContentType = r.Header.Get("Content-Type")
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		response := WorkItem{
			ID:  123,
			Rev: 2,
			Fields: WorkItemFields{
				Title: "Fix bug",
				State: "Resolved",
			},
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

	err := client.UpdateWorkItemState(123, "Resolved")
	if err != nil {
		t.Fatalf("UpdateWorkItemState() error = %v", err)
	}

	if capturedContentType != "application/json-patch+json" {
		t.Errorf("Expected Content-Type 'application/json-patch+json', got %q", capturedContentType)
	}

	if !strings.Contains(capturedBody, "/fields/System.State") {
		t.Errorf("Expected body to contain '/fields/System.State', got %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "Resolved") {
		t.Errorf("Expected body to contain 'Resolved', got %s", capturedBody)
	}
}

func TestClient_UpdateWorkItemState_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	err := client.UpdateWorkItemState(123, "InvalidState")
	if err == nil {
		t.Error("Expected error for bad request, got nil")
	}
}
