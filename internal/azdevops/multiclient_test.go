package azdevops

import (
	"encoding/json"
	"errors"
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
		mc, err := NewMultiClient("myorg", []string{"alpha", "beta"}, "pat123", nil)
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
		_, err := NewMultiClient("myorg", []string{"alpha", ""}, "pat123", nil)
		if err == nil {
			t.Fatal("expected error for empty project name")
		}
	})

	t.Run("returns error for empty projects list", func(t *testing.T) {
		_, err := NewMultiClient("myorg", []string{}, "pat123", nil)
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
		mc, err := NewMultiClient("org", tt.projects, "pat", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := mc.IsMultiProject(); got != tt.want {
			t.Errorf("IsMultiProject() with %d projects = %v, want %v", len(tt.projects), got, tt.want)
		}
	}
}

func TestMultiClient_ClientFor(t *testing.T) {
	mc, err := NewMultiClient("org", []string{"alpha", "beta"}, "pat", nil)
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

// newPipelineRunServerWithStatusFilter responds differently depending on the
// statusFilter query parameter, so a test can simulate a build that only
// shows up in the active (notStarted/inProgress) query and has aged out of
// the plain recent-runs response.
func newPipelineRunServerWithStatusFilter(t *testing.T, recent, queued []PipelineRun) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var resp PipelineRunsResponse
		switch r.URL.Query().Get("statusFilter") {
		case "notStarted":
			resp = PipelineRunsResponse{Value: queued}
		case "inProgress":
			resp = PipelineRunsResponse{Value: nil}
		default:
			resp = PipelineRunsResponse{Value: recent}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// TestMultiClient_ListPipelineRuns_SurfacesStaleQueuedRun pins the fix for a
// build that has been sitting queued long enough that its QueueTime falls
// outside the plain recent-runs response (top's queueTimeDescending window),
// even though Azure DevOps still reports it as queued right now.
// ListPipelineRuns must merge in Client.ListActivePipelineRuns's results so
// it isn't hidden.
func TestMultiClient_ListPipelineRuns_SurfacesStaleQueuedRun(t *testing.T) {
	now := time.Now()
	recent := []PipelineRun{
		{ID: 1, QueueTime: now, Status: "completed", Result: "succeeded", Project: Project{Name: "alpha"}},
	}
	staleQueued := []PipelineRun{
		{ID: 99, QueueTime: now.Add(-2 * time.Hour), Status: "notStarted", Project: Project{Name: "alpha"}},
	}

	server := newPipelineRunServerWithStatusFilter(t, recent, staleQueued)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	runs, err := mc.ListPipelineRuns(10)
	if err != nil {
		t.Fatalf("ListPipelineRuns failed: %v", err)
	}

	var found bool
	for _, r := range runs {
		if r.ID == 99 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected stale queued run (ID=99) to be merged in via ListActivePipelineRuns, got runs=%v", runs)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs (1 recent + 1 merged active), got %d", len(runs))
	}
}

// TestMergeActiveRuns_DedupesByID pins that a run present in both the
// recent-runs and active-runs responses (e.g. an inProgress build that's
// recent enough to appear in both) is only kept once.
func TestMergeActiveRuns_DedupesByID(t *testing.T) {
	recent := []PipelineRun{{ID: 1}, {ID: 2}}
	active := []PipelineRun{{ID: 2}, {ID: 3}}

	merged := mergeActiveRuns(recent, active)

	if len(merged) != 3 {
		t.Fatalf("expected 3 runs after dedup, got %d: %+v", len(merged), merged)
	}
	ids := map[int]bool{}
	for _, r := range merged {
		ids[r.ID] = true
	}
	if !ids[1] || !ids[2] || !ids[3] {
		t.Errorf("expected IDs {1,2,3}, got %+v", merged)
	}
}

// TestMergeActiveRuns_EmptyActive pins that an empty (or nil) active slice
// leaves recent untouched, e.g. when a project has no queued/running builds.
func TestMergeActiveRuns_EmptyActive(t *testing.T) {
	recent := []PipelineRun{{ID: 1}}

	merged := mergeActiveRuns(recent, nil)

	if len(merged) != 1 || merged[0].ID != 1 {
		t.Errorf("expected recent unchanged, got %+v", merged)
	}
}

// newPipelineRunServerFailingActiveQueries succeeds for the plain
// recent-runs query but fails any request carrying a statusFilter, so a test
// can simulate Client.ListActivePipelineRuns erroring without affecting
// Client.ListPipelineRuns.
func newPipelineRunServerFailingActiveQueries(t *testing.T, recent []PipelineRun) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("statusFilter") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message":"internal error"}`))
			return
		}
		resp := PipelineRunsResponse{Value: recent}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// TestMultiClient_ListPipelineRuns_ActiveFetchFailureIsBestEffort pins that a
// failure in Client.ListActivePipelineRuns (e.g. a transient 500) must not
// invalidate an otherwise-successful ListPipelineRuns call: the recent-runs
// fetch already succeeded and its results must still be returned as-is,
// without surfacing the active-fetch error.
func TestMultiClient_ListPipelineRuns_ActiveFetchFailureIsBestEffort(t *testing.T) {
	now := time.Now()
	recent := []PipelineRun{
		{ID: 1, QueueTime: now, Project: Project{Name: "alpha"}},
	}

	server := newPipelineRunServerFailingActiveQueries(t, recent)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	runs, err := mc.ListPipelineRuns(10)
	if err != nil {
		t.Fatalf("ListPipelineRuns failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != 1 {
		t.Fatalf("expected the recent run to survive an active-fetch failure, got %+v", runs)
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
	// Partial failure: should return results AND a PartialError
	if len(runs) != 1 {
		t.Fatalf("expected 1 run from partial result, got %d", len(runs))
	}

	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected PartialError, got: %v", err)
	}
	if partialErr.Total != 2 {
		t.Errorf("expected Total=2, got %d", partialErr.Total)
	}
	if partialErr.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", partialErr.Failed)
	}
}

func TestPartialError_Error(t *testing.T) {
	pe := &PartialError{
		Failed: 1,
		Total:  3,
		Errors: []error{fmt.Errorf("project beta failed")},
	}

	msg := pe.Error()
	if !strings.Contains(msg, "1") || !strings.Contains(msg, "3") {
		t.Errorf("expected error message to contain failed/total counts, got: %s", msg)
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

func TestMultiClient_ListMyFailedPipelineRuns_MergedSortedAndTagged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	alphaFinish := now.Add(-2 * time.Hour)
	betaFinish := now.Add(-1 * time.Hour)
	alphaRuns := []PipelineRun{
		{ID: 1, Status: "completed", Result: "failed", FinishTime: &alphaFinish, RequestedFor: Identity{ID: "user-1"}},
	}
	betaRuns := []PipelineRun{
		{ID: 2, Status: "completed", Result: "failed", FinishTime: &betaFinish, RequestedFor: Identity{ID: "user-1"}},
	}

	alphaServer := newPipelineRunServer(t, "alpha", alphaRuns)
	defer alphaServer.Close()
	betaServer := newPipelineRunServer(t, "beta", betaRuns)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	runs, err := mc.ListMyFailedPipelineRuns("user-1", 14, 10)
	if err != nil {
		t.Fatalf("ListMyFailedPipelineRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	// Sorted by FinishTime descending: beta run (now-1h) before alpha run (now-2h)
	if runs[0].ID != 2 {
		t.Errorf("expected first run ID=2 (newest FinishTime), got %d", runs[0].ID)
	}
	if runs[0].ProjectName != "beta" {
		t.Errorf("expected runs[0].ProjectName = 'beta', got %q", runs[0].ProjectName)
	}
	if runs[1].ProjectName != "alpha" {
		t.Errorf("expected runs[1].ProjectName = 'alpha', got %q", runs[1].ProjectName)
	}
}

func TestMultiClient_ListMyFailedPipelineRuns_PartialFailure(t *testing.T) {
	finish := time.Now().UTC()
	alphaRuns := []PipelineRun{
		{ID: 1, Status: "completed", Result: "failed", FinishTime: &finish, RequestedFor: Identity{ID: "user-1"}},
	}

	alphaServer := newPipelineRunServer(t, "alpha", alphaRuns)
	defer alphaServer.Close()
	errorServer := newErrorServer(t)
	defer errorServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  errorServer,
	})

	runs, err := mc.ListMyFailedPipelineRuns("user-1", 14, 10)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run from partial result, got %d", len(runs))
	}

	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected PartialError, got: %v", err)
	}
	// Failed/Total are project counts, not run counts (finding 1 of task 7's
	// review: MultiClient's PartialError contract must match every other
	// fan-out method's).
	if partialErr.Total != 2 {
		t.Errorf("expected Total=2 (project count), got %d", partialErr.Total)
	}
	if partialErr.Failed != 1 {
		t.Errorf("expected Failed=1 (project count), got %d", partialErr.Failed)
	}
}

func TestMultiClient_ListMyFailedPipelineRuns_AllFail(t *testing.T) {
	errorServer1 := newErrorServer(t)
	defer errorServer1.Close()
	errorServer2 := newErrorServer(t)
	defer errorServer2.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": errorServer1,
		"beta":  errorServer2,
	})

	_, err := mc.ListMyFailedPipelineRuns("user-1", 14, 10)
	if err == nil {
		t.Fatal("expected error when all projects fail")
	}
}

// TestMultiClient_ListMyFailedPipelineRuns_NilFinishTimeDoesNotPanic pins
// that the merge sort survives a run with a nil FinishTime without
// dereferencing it. A run isMyFailedRun keeps should always carry one, but
// this method's own contract is "narrow server-side, re-check in Go" — the
// merge sort runs before that Go-side re-check gets a chance to drop
// anything, so a server that ignores every query parameter and returns an
// in-progress run alongside failed ones must not crash the fan-out.
func TestMultiClient_ListMyFailedPipelineRuns_NilFinishTimeDoesNotPanic(t *testing.T) {
	finish := time.Now().UTC()
	runs := []PipelineRun{
		{ID: 1, Status: "completed", Result: "failed", FinishTime: &finish, RequestedFor: Identity{ID: "user-1"}},
		{ID: 2, Status: "inProgress", Result: "none", FinishTime: nil, RequestedFor: Identity{ID: "user-1"}},
	}

	server := newPipelineRunServer(t, "alpha", runs)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	got, err := mc.ListMyFailedPipelineRuns("user-1", 14, 10)
	if err != nil {
		t.Fatalf("ListMyFailedPipelineRuns failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected both runs to pass through the fan-out (Go-side filtering is SourceCIFailed's job, not this method's), got %d", len(got))
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

func TestNewMultiClient_WithDisplayNames(t *testing.T) {
	mc, err := NewMultiClient("myorg", []string{"ugly-api"}, "pat123", map[string]string{"ugly-api": "Friendly"})
	if err != nil {
		t.Fatalf("NewMultiClient failed: %v", err)
	}

	if got := mc.DisplayNameFor("ugly-api"); got != "Friendly" {
		t.Errorf("DisplayNameFor(ugly-api) = %q, want %q", got, "Friendly")
	}

	// Unknown project returns API name
	if got := mc.DisplayNameFor("other"); got != "other" {
		t.Errorf("DisplayNameFor(other) = %q, want %q", got, "other")
	}
}

func TestNewMultiClient_NilDisplayNames(t *testing.T) {
	mc, err := NewMultiClient("myorg", []string{"proj"}, "pat123", nil)
	if err != nil {
		t.Fatalf("NewMultiClient failed: %v", err)
	}

	// Should return API name when no display names configured
	if got := mc.DisplayNameFor("proj"); got != "proj" {
		t.Errorf("DisplayNameFor(proj) = %q, want %q", got, "proj")
	}
}

func TestMultiClient_ListPipelineRuns_TagsDisplayName(t *testing.T) {
	now := time.Now()
	runs := []PipelineRun{
		{ID: 1, QueueTime: now, Project: Project{Name: "ugly-api"}},
	}

	server := newPipelineRunServer(t, "ugly-api", runs)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"ugly-api": server,
	})
	mc.displayNames = map[string]string{"ugly-api": "Friendly"}

	result, err := mc.ListPipelineRuns(10)
	if err != nil {
		t.Fatalf("ListPipelineRuns failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 run, got %d", len(result))
	}
	if result[0].ProjectName != "ugly-api" {
		t.Errorf("ProjectName = %q, want %q", result[0].ProjectName, "ugly-api")
	}
	if result[0].ProjectDisplayName != "Friendly" {
		t.Errorf("ProjectDisplayName = %q, want %q", result[0].ProjectDisplayName, "Friendly")
	}
}

func TestMultiClient_ListPullRequests_TagsDisplayName(t *testing.T) {
	now := time.Now()
	prs := []PullRequest{
		{ID: 10, Title: "Test PR", CreationDate: now},
	}

	server := newPRServer(t, prs)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"ugly-api": server,
	})
	mc.displayNames = map[string]string{"ugly-api": "Friendly"}

	result, err := mc.ListPullRequests(25)
	if err != nil {
		t.Fatalf("ListPullRequests failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(result))
	}
	if result[0].ProjectDisplayName != "Friendly" {
		t.Errorf("ProjectDisplayName = %q, want %q", result[0].ProjectDisplayName, "Friendly")
	}
}

func TestMultiClient_ListWorkItems_TagsDisplayName(t *testing.T) {
	now := time.Now()
	items := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Test WI", ChangedDate: now}},
	}

	server := newWorkItemServer(t, items)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"ugly-api": server,
	})
	mc.displayNames = map[string]string{"ugly-api": "Friendly"}

	result, err := mc.ListWorkItems(50)
	if err != nil {
		t.Fatalf("ListWorkItems failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].ProjectDisplayName != "Friendly" {
		t.Errorf("ProjectDisplayName = %q, want %q", result[0].ProjectDisplayName, "Friendly")
	}
}

func TestMultiClient_ListMyWorkItems_MergedSortedAndTagged(t *testing.T) {
	now := time.Now()
	alphaItems := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Alpha My WI", ChangedDate: now.Add(-2 * time.Hour)}},
	}
	betaItems := []WorkItem{
		{ID: 200, Fields: WorkItemFields{Title: "Beta My WI", ChangedDate: now}},
	}

	alphaServer := newWorkItemServer(t, alphaItems)
	defer alphaServer.Close()
	betaServer := newWorkItemServer(t, betaItems)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	items, err := mc.ListMyWorkItems(50)
	if err != nil {
		t.Fatalf("ListMyWorkItems failed: %v", err)
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

func TestMultiClient_ListRecentlyAssignedWorkItems_MergedSortedAndTagged(t *testing.T) {
	now := time.Now()
	alphaItems := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Alpha Assigned WI", ChangedDate: now.Add(-2 * time.Hour)}},
	}
	betaItems := []WorkItem{
		{ID: 200, Fields: WorkItemFields{Title: "Beta Assigned WI", ChangedDate: now}},
	}

	alphaServer := newWorkItemServer(t, alphaItems)
	defer alphaServer.Close()
	betaServer := newWorkItemServer(t, betaItems)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	items, err := mc.ListRecentlyAssignedWorkItems(14, 50)
	if err != nil {
		t.Fatalf("ListRecentlyAssignedWorkItems failed: %v", err)
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

func TestMultiClient_ListRecentlyAssignedWorkItems_PartialFailure(t *testing.T) {
	now := time.Now()
	alphaItems := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Alpha Assigned WI", ChangedDate: now}},
	}

	alphaServer := newWorkItemServer(t, alphaItems)
	defer alphaServer.Close()
	errorServer := newErrorServer(t)
	defer errorServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  errorServer,
	})

	items, err := mc.ListRecentlyAssignedWorkItems(14, 50)
	if len(items) != 1 {
		t.Fatalf("expected 1 item from partial result, got %d", len(items))
	}

	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected PartialError, got: %v", err)
	}
	if partialErr.Total != 2 {
		t.Errorf("expected Total=2, got %d", partialErr.Total)
	}
	if partialErr.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", partialErr.Failed)
	}
}
