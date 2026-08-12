package demo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// newMockHandler creates an http.Handler that serves fake Azure DevOps API
// responses. Every data route carries a leading {project} segment (demo.Run
// gives each project client a project-scoped base URL) so responses can be
// filtered to the requesting project — the multi-project fan-outs in
// MultiClient tag and merge per project and would otherwise duplicate every
// row across both demo projects.
func newMockHandler() http.Handler {
	mux := http.NewServeMux()

	// Connection data (org-level auth endpoint, no project segment)
	mux.HandleFunc("/_apis/connectionData", handleConnectionData)

	// Pull requests list
	mux.HandleFunc("/{project}/git/pullrequests", handlePullRequests)

	// PR detail endpoints: threads, iterations, changes, file content
	// These all start with /git/repositories/
	mux.HandleFunc("/{project}/git/repositories/", handleGitRepositories)

	// WIQL query (POST)
	mux.HandleFunc("/{project}/wit/wiql", handleWIQL)

	// Work item comments (list and add). The path segment is spelled
	// "workItems" — capital I — matching Client.GetWorkItemComments and
	// Client.AddWorkItemComment; ServeMux patterns are case-sensitive, so
	// this does not collide with the lowercase routes below.
	mux.HandleFunc("/{project}/wit/workItems/{id}/comments", handleWorkItemComments)

	// Work items by IDs and state updates (PATCH to /wit/workitems/{id})
	mux.HandleFunc("/{project}/wit/workitems", handleWorkItems)
	mux.HandleFunc("/{project}/wit/workitems/", handleWorkItems)

	// Work item type states
	mux.HandleFunc("/{project}/wit/workitemtypes/", handleWorkItemTypeStates)

	// Pipeline runs, timeline, logs
	mux.HandleFunc("/{project}/build/builds", handleBuilds)
	mux.HandleFunc("/{project}/build/builds/", handleBuildDetail)

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

func handlePullRequests(w http.ResponseWriter, r *http.Request) {
	prs := mockPullRequestsFor(r.PathValue("project"))

	// The review-requested notification source (and the pane's "as reviewer"
	// toggle) narrow server-side via searchCriteria.reviewerId; mirror that
	// narrowing here so the mock behaves like the real API.
	if reviewerID := r.URL.Query().Get("searchCriteria.reviewerId"); reviewerID != "" {
		var filtered []azdevops.PullRequest
		for _, pr := range prs {
			for _, reviewer := range pr.Reviewers {
				if reviewer.ID == reviewerID {
					filtered = append(filtered, pr)
					break
				}
			}
		}
		prs = filtered
	}

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

func handleWIQL(w http.ResponseWriter, r *http.Request) {
	items := mockWorkItemsFor(r.PathValue("project"))

	// Branch on the WIQL text the same way the real server resolves macros:
	// the assigned queries (ListMyWorkItems, ListRecentlyAssignedWorkItems)
	// filter on @Me and exclude closed states; the mentioned candidate query
	// filters on @RecentMentions. Everything else (panes, metrics) gets the
	// project's full set.
	var req struct {
		Query string `json:"query"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	switch {
	case strings.Contains(req.Query, "[System.AssignedTo] = @Me"):
		items = filterAssignedToDemoUser(items)
	case strings.Contains(req.Query, "@RecentMentions"):
		items = filterMentionCandidates(items)
	}

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

	items := mockWorkItemsFor(r.PathValue("project"))

	// The batch GET is always id-driven (WIQL first, then fetch by ids);
	// honour the ids parameter so a narrowed WIQL result stays narrowed.
	if idsParam := r.URL.Query().Get("ids"); idsParam != "" {
		wanted := make(map[int]bool)
		for _, s := range strings.Split(idsParam, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				wanted[id] = true
			}
		}
		var filtered []azdevops.WorkItem
		for _, item := range items {
			if wanted[item.ID] {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	writeJSON(w, azdevops.WorkItemsResponse{Count: len(items), Value: items})
}

func handleWorkItemComments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// AddWorkItemComment — echo the created comment
		var req struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, azdevops.WorkItemComment{
			ID:          999,
			Text:        req.Text,
			CreatedBy:   team[0],
			CreatedDate: now,
		})
		return
	}

	id, _ := strconv.Atoi(r.PathValue("id"))
	comments := mockWorkItemComments(id)
	writeJSON(w, map[string]any{
		"totalCount": len(comments),
		"count":      len(comments),
		"comments":   comments,
	})
}

func handleWorkItemTypeStates(w http.ResponseWriter, r *http.Request) {
	// Extract work item type from path: /wit/workitemtypes/{type}/states
	path := r.URL.Path
	idx := strings.Index(path, "/wit/workitemtypes/")
	path = path[idx+len("/wit/workitemtypes/"):]
	parts := strings.SplitN(path, "/", 2)
	wiType := parts[0]

	// URL decode spaces (%20 → space)
	wiType = strings.ReplaceAll(wiType, "%20", " ")

	states := mockWorkItemTypeStates(wiType)
	writeJSON(w, azdevops.WorkItemTypeStatesResponse{Count: len(states), Value: states})
}

func handleBuilds(w http.ResponseWriter, r *http.Request) {
	runs := mockPipelineRunsFor(r.PathValue("project"))
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
