package azdevops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNewMultiClient(t *testing.T) {
	t.Run("creates clients for each project", func(t *testing.T) {
		mc, err := NewMultiClient("myorg", []string{"alpha", "beta"}, "pat123")
		if err != nil {
			t.Fatalf("NewMultiClient failed: %v", err)
		}

		if mc.GetOrg() != "myorg" {
			t.Errorf("GetOrg() = %q, want %q", mc.GetOrg(), "myorg")
		}

		projects := mc.Projects()
		sort.Strings(projects)
		if len(projects) != 2 || projects[0] != "alpha" || projects[1] != "beta" {
			t.Errorf("Projects() = %v, want [alpha beta]", projects)
		}
	})

	t.Run("returns error for empty project name", func(t *testing.T) {
		_, err := NewMultiClient("myorg", []string{"alpha", ""}, "pat123")
		if err == nil {
			t.Fatal("expected error for empty project name")
		}
	})

	t.Run("returns error for empty projects list", func(t *testing.T) {
		_, err := NewMultiClient("myorg", []string{}, "pat123")
		if err == nil {
			t.Fatal("expected error for empty projects list")
		}
	})
}

func TestMultiClient_IsMultiProject(t *testing.T) {
	tests := []struct {
		projects []string
		want     bool
	}{
		{[]string{"alpha"}, false},
		{[]string{"alpha", "beta"}, true},
	}
	for _, tt := range tests {
		mc, err := NewMultiClient("org", tt.projects, "pat")
		if err != nil {
			t.Fatal(err)
		}
		if got := mc.IsMultiProject(); got != tt.want {
			t.Errorf("IsMultiProject() with %d projects = %v, want %v", len(tt.projects), got, tt.want)
		}
	}
}

func TestMultiClient_ClientFor(t *testing.T) {
	mc, err := NewMultiClient("org", []string{"alpha", "beta"}, "pat")
	if err != nil {
		t.Fatal(err)
	}

	c := mc.ClientFor("alpha")
	if c == nil {
		t.Fatal("ClientFor(alpha) returned nil")
	}
	if c.GetProject() != "alpha" {
		t.Errorf("ClientFor(alpha).GetProject() = %q", c.GetProject())
	}

	if mc.ClientFor("nonexistent") != nil {
		t.Error("ClientFor(nonexistent) should return nil")
	}
}

// Helper to create a test server that responds with pipeline runs for a given project
func newPipelineRunServer(t *testing.T, projectName string, runs []PipelineRun) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineRunsResponse{Value: runs}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func newPRServer(t *testing.T, prs []PullRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Value []PullRequest `json:"value"`
		}{Value: prs}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func newWorkItemServer(t *testing.T, items []WorkItem) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "wiql") {
			// Return work item IDs
			refs := make([]WorkItemReference, len(items))
			for i, item := range items {
				refs[i] = WorkItemReference{ID: item.ID}
			}
			resp := struct {
				WorkItems []WorkItemReference `json:"workItems"`
			}{WorkItems: refs}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Return full work items
		resp := struct {
			Value []WorkItem `json:"value"`
		}{Value: items}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func newErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
	}))
}

// newMultiClientWithServers creates a MultiClient with test servers overriding baseURLs.
func newMultiClientWithServers(t *testing.T, servers map[string]*httptest.Server) *MultiClient {
	t.Helper()
	clients := make(map[string]*Client, len(servers))
	for project, server := range servers {
		c, err := NewClient("testorg", project, "testpat")
		if err != nil {
			t.Fatalf("failed to create client for %q: %v", project, err)
		}
		c.baseURL = server.URL
		clients[project] = c
	}
	return &MultiClient{
		org:     "testorg",
		pat:     "testpat",
		clients: clients,
	}
}

func TestMultiClient_ListPipelineRuns_MergedAndSorted(t *testing.T) {
	now := time.Now()
	alphaRuns := []PipelineRun{
		{ID: 1, QueueTime: now.Add(-1 * time.Minute), Project: Project{Name: "alpha"}},
		{ID: 3, QueueTime: now.Add(-3 * time.Minute), Project: Project{Name: "alpha"}},
	}
	betaRuns := []PipelineRun{
		{ID: 2, QueueTime: now.Add(-2 * time.Minute), Project: Project{Name: "beta"}},
	}

	alphaServer := newPipelineRunServer(t, "alpha", alphaRuns)
	defer alphaServer.Close()
	betaServer := newPipelineRunServer(t, "beta", betaRuns)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	runs, err := mc.ListPipelineRuns(10)
	if err != nil {
		t.Fatalf("ListPipelineRuns failed: %v", err)
	}

	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}

	// Should be sorted by QueueTime descending
	if runs[0].ID != 1 || runs[1].ID != 2 || runs[2].ID != 3 {
		t.Errorf("runs not sorted correctly: IDs = [%d, %d, %d]", runs[0].ID, runs[1].ID, runs[2].ID)
	}
}

func TestMultiClient_ListPipelineRuns_PartialFailure(t *testing.T) {
	now := time.Now()
	alphaRuns := []PipelineRun{
		{ID: 1, QueueTime: now, Project: Project{Name: "alpha"}},
	}

	alphaServer := newPipelineRunServer(t, "alpha", alphaRuns)
	defer alphaServer.Close()
	errorServer := newErrorServer(t)
	defer errorServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  errorServer,
	})

	runs, err := mc.ListPipelineRuns(10)
	// Partial failure: should return results from successful project
	if err != nil {
		t.Fatalf("expected no error for partial failure, got: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run from partial result, got %d", len(runs))
	}
}

func TestMultiClient_ListPipelineRuns_AllFail(t *testing.T) {
	errorServer1 := newErrorServer(t)
	defer errorServer1.Close()
	errorServer2 := newErrorServer(t)
	defer errorServer2.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": errorServer1,
		"beta":  errorServer2,
	})

	_, err := mc.ListPipelineRuns(10)
	if err == nil {
		t.Fatal("expected error when all projects fail")
	}
}

func TestMultiClient_ListPullRequests_MergedSortedAndTagged(t *testing.T) {
	now := time.Now()
	alphaPRs := []PullRequest{
		{ID: 10, Title: "Alpha PR", CreationDate: now.Add(-1 * time.Hour)},
	}
	betaPRs := []PullRequest{
		{ID: 20, Title: "Beta PR", CreationDate: now},
	}

	alphaServer := newPRServer(t, alphaPRs)
	defer alphaServer.Close()
	betaServer := newPRServer(t, betaPRs)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	prs, err := mc.ListPullRequests(25)
	if err != nil {
		t.Fatalf("ListPullRequests failed: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}

	// Sorted by CreationDate descending: beta PR (now) before alpha PR (now-1h)
	if prs[0].ID != 20 {
		t.Errorf("expected first PR ID=20 (newest), got %d", prs[0].ID)
	}

	// Project tagging
	if prs[0].ProjectName != "beta" {
		t.Errorf("expected prs[0].ProjectName = 'beta', got %q", prs[0].ProjectName)
	}
	if prs[1].ProjectName != "alpha" {
		t.Errorf("expected prs[1].ProjectName = 'alpha', got %q", prs[1].ProjectName)
	}
}

func TestMultiClient_ListWorkItems_MergedSortedAndTagged(t *testing.T) {
	now := time.Now()
	alphaItems := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Alpha WI", ChangedDate: now.Add(-2 * time.Hour)}},
	}
	betaItems := []WorkItem{
		{ID: 200, Fields: WorkItemFields{Title: "Beta WI", ChangedDate: now}},
	}

	alphaServer := newWorkItemServer(t, alphaItems)
	defer alphaServer.Close()
	betaServer := newWorkItemServer(t, betaItems)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	items, err := mc.ListWorkItems(50)
	if err != nil {
		t.Fatalf("ListWorkItems failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Sorted by ChangedDate descending
	if items[0].ID != 200 {
		t.Errorf("expected first item ID=200 (newest), got %d", items[0].ID)
	}

	// Project tagging
	if items[0].ProjectName != "beta" {
		t.Errorf("expected items[0].ProjectName = 'beta', got %q", items[0].ProjectName)
	}
	if items[1].ProjectName != "alpha" {
		t.Errorf("expected items[1].ProjectName = 'alpha', got %q", items[1].ProjectName)
	}
}

func TestMultiClient_SingleProject_BehavesLikeSingleClient(t *testing.T) {
	now := time.Now()
	runs := []PipelineRun{
		{ID: 1, QueueTime: now, Project: Project{Name: "only"}},
	}

	server := newPipelineRunServer(t, "only", runs)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"only": server,
	})

	result, err := mc.ListPipelineRuns(10)
	if err != nil {
		t.Fatalf("ListPipelineRuns failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 run, got %d", len(result))
	}
	if result[0].ID != 1 {
		t.Errorf("expected run ID=1, got %d", result[0].ID)
	}
}

// Ensure PipelineRunsResponse is accessible for test helpers
func TestPipelineRunsResponse_Exists(t *testing.T) {
	_ = PipelineRunsResponse{}
	_ = fmt.Sprintf("%v", PipelineRunsResponse{})
}
