package demo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// newMockHandler creates an http.Handler that serves fake Azure DevOps API responses.
func newMockHandler() http.Handler {
	mux := http.NewServeMux()

	// Connection data (org-level auth endpoint)
	mux.HandleFunc("/_apis/connectionData", handleConnectionData)

	// Pull requests list
	mux.HandleFunc("/git/pullrequests", handlePullRequests)

	// PR detail endpoints: threads, iterations, changes, file content
	// These all start with /git/repositories/
	mux.HandleFunc("/git/repositories/", handleGitRepositories)

	// WIQL query (POST)
	mux.HandleFunc("/wit/wiql", handleWIQL)

	// Work items by IDs and state updates (PATCH to /wit/workitems/{id})
	mux.HandleFunc("/wit/workitems", handleWorkItems)
	mux.HandleFunc("/wit/workitems/", handleWorkItems)

	// Work item type states
	mux.HandleFunc("/wit/workitemtypes/", handleWorkItemTypeStates)

	// Pipeline runs, timeline, logs
	mux.HandleFunc("/build/builds", handleBuilds)
	mux.HandleFunc("/build/builds/", handleBuildDetail)

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func handleConnectionData(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"authenticatedUser": map[string]any{
			"id": demoUserID,
		},
	})
}

func handlePullRequests(w http.ResponseWriter, _ *http.Request) {
	prs := mockPullRequests()
	writeJSON(w, azdevops.PullRequestsResponse{Count: len(prs), Value: prs})
}

func handleGitRepositories(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.Contains(path, "/reviewers/"):
		// VotePullRequest — PUT, just acknowledge
		writeJSON(w, map[string]any{"id": demoUserID, "vote": 10})
	case strings.Contains(path, "/comments"):
		// ReplyToThread — POST, return a Comment
		handleReplyToThread(w, r)
	case strings.Contains(path, "/threads"):
		handlePRThreads(w, r)
	case strings.Contains(path, "/iterations") && strings.Contains(path, "/changes"):
		handleIterationChanges(w, r)
	case strings.Contains(path, "/iterations"):
		handlePRIterations(w, r)
	case strings.Contains(path, "/items"):
		handleFileContent(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handlePRThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// AddPRComment or AddPRCodeComment — return a simple thread
		writeJSON(w, azdevops.Thread{ID: 100})
		return
	}
	if r.Method == http.MethodPatch {
		// UpdateThreadStatus — acknowledge
		writeJSON(w, map[string]any{"id": 1, "status": "fixed"})
		return
	}
	threads := mockThreads()
	writeJSON(w, azdevops.ThreadsResponse{Count: len(threads), Value: threads})
}

func handleReplyToThread(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, azdevops.Comment{
		ID:          99,
		Content:     "Reply acknowledged",
		CommentType: "text",
		Author:      team[0],
	})
}

func handlePRIterations(w http.ResponseWriter, _ *http.Request) {
	iterations := mockPRIterations()
	writeJSON(w, azdevops.IterationsResponse{Count: len(iterations), Value: iterations})
}

func handleIterationChanges(w http.ResponseWriter, _ *http.Request) {
	changes := mockIterationChanges()
	writeJSON(w, azdevops.IterationChangesResponse{ChangeEntries: changes})
}

func handleFileContent(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("version")
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, mockFileContent(filePath, branch))
}

func handleWIQL(w http.ResponseWriter, _ *http.Request) {
	items := mockWorkItems()
	refs := make([]azdevops.WorkItemReference, len(items))
	for i, item := range items {
		refs[i] = azdevops.WorkItemReference{ID: item.ID}
	}
	writeJSON(w, azdevops.WIQLResponse{WorkItems: refs})
}

func handleWorkItems(w http.ResponseWriter, r *http.Request) {
	// PATCH for state update — just return success
	if r.Method == http.MethodPatch {
		writeJSON(w, map[string]any{"id": 1, "rev": 2})
		return
	}

	items := mockWorkItems()
	writeJSON(w, azdevops.WorkItemsResponse{Count: len(items), Value: items})
}

func handleWorkItemTypeStates(w http.ResponseWriter, r *http.Request) {
	// Extract work item type from path: /wit/workitemtypes/{type}/states
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/wit/workitemtypes/")
	parts := strings.SplitN(path, "/", 2)
	wiType := parts[0]

	// URL decode spaces (%20 → space)
	wiType = strings.ReplaceAll(wiType, "%20", " ")

	states := mockWorkItemTypeStates(wiType)
	writeJSON(w, azdevops.WorkItemTypeStatesResponse{Count: len(states), Value: states})
}

func handleBuilds(w http.ResponseWriter, _ *http.Request) {
	runs := mockPipelineRuns()
	writeJSON(w, azdevops.PipelineRunsResponse{Count: len(runs), Value: runs})
}

func handleBuildDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.Contains(path, "/timeline"):
		timeline := mockTimeline()
		writeJSON(w, timeline)
	case strings.HasSuffix(path, "/logs"):
		// List logs
		logs := mockBuildLogs()
		writeJSON(w, azdevops.BuildLogsResponse{Count: len(logs), Value: logs})
	case strings.Contains(path, "/logs/"):
		// Specific log content — extract log ID from path
		parts := strings.Split(path, "/logs/")
		if len(parts) == 2 {
			var logID int
			fmt.Sscanf(parts[1], "%d", &logID)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, mockBuildLogContent(logID))
		} else {
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}
