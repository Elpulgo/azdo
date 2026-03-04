package demo

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

func TestServerPullRequests(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/git/pullrequests?api-version=7.1&$top=25&searchCriteria.status=active")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.PullRequestsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if result.Count == 0 {
		t.Error("expected non-zero count")
	}
	if len(result.Value) == 0 {
		t.Error("expected non-empty value")
	}
}

func TestServerWIQL(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/wit/wiql?api-version=7.1&$top=50", "application/json",
		strings.NewReader(`{"query":"SELECT [System.Id] FROM WorkItems"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.WIQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.WorkItems) == 0 {
		t.Error("expected non-empty work items")
	}
}

func TestServerWorkItems(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/wit/workitems?ids=5001,5002&fields=System.Title&api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.WorkItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) == 0 {
		t.Error("expected non-empty work items")
	}
}

func TestServerPipelineRuns(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/build/builds?api-version=7.1&$top=25&queryOrder=queueTimeDescending")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.PipelineRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) == 0 {
		t.Error("expected non-empty pipeline runs")
	}
}

func TestServerPRThreads(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/git/repositories/repo-001/pullRequests/1042/threads?api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.ThreadsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) == 0 {
		t.Error("expected non-empty threads")
	}
}

func TestServerPRIterations(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/git/repositories/repo-001/pullRequests/1042/iterations?api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.IterationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) == 0 {
		t.Error("expected non-empty iterations")
	}
}

func TestServerIterationChanges(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/git/repositories/repo-001/pullRequests/1042/iterations/1/changes?api-version=7.1&$compareTo=0")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.IterationChangesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.ChangeEntries) == 0 {
		t.Error("expected non-empty change entries")
	}
}

func TestServerBuildTimeline(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/build/builds/8001/timeline?api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.Timeline
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Records) == 0 {
		t.Error("expected non-empty timeline records")
	}
}

func TestServerBuildLogs(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/build/builds/8001/logs?api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.BuildLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) == 0 {
		t.Error("expected non-empty build logs")
	}
}

func TestServerBuildLogContent(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/build/builds/8001/logs/1?api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("expected non-empty log content")
	}
}

func TestServerWorkItemTypeStates(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/wit/workitemtypes/Bug/states?api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.WorkItemTypeStatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) == 0 {
		t.Error("expected non-empty states")
	}
}

func TestServerConnectionData(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/_apis/connectionData")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		AuthenticatedUser struct {
			ID string `json:"id"`
		} `json:"authenticatedUser"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if result.AuthenticatedUser.ID == "" {
		t.Error("expected non-empty user ID")
	}
}

func TestServerVotePullRequest(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut,
		srv.URL+"/git/repositories/repo-001/pullRequests/1042/reviewers/user-001?api-version=7.1",
		strings.NewReader(`{"vote": 10}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServerReplyToThread(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/git/repositories/repo-001/pullRequests/1042/threads/1/comments?api-version=7.1",
		"application/json",
		strings.NewReader(`{"content": "test reply"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result azdevops.Comment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if result.ID == 0 {
		t.Error("expected non-zero comment ID")
	}
}

func TestServerResolveThread(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/git/repositories/repo-001/pullRequests/1042/threads/1?api-version=7.1",
		strings.NewReader(`{"status": "fixed"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServerUpdateWorkItemState(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/wit/workitems/5001?api-version=7.1",
		strings.NewReader(`[{"op":"replace","path":"/fields/System.State","value":"Resolved"}]`))
	req.Header.Set("Content-Type", "application/json-patch+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServerFileContent(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/git/repositories/repo-001/items?path=/src/auth.go&versionType=branch&version=main&api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("expected non-empty file content")
	}
}
