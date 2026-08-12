package demo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
)

// newDemoMultiClient wires a MultiClient against the mock server exactly the
// way demo.Run does: per-project base URLs and a pre-seeded user id.
func newDemoMultiClient(t *testing.T, srvURL string) *azdevops.MultiClient {
	t.Helper()
	projects := []string{projectNexus, projectHorizon}
	client, err := azdevops.NewMultiClient(demoOrg, projects, "demo-pat", map[string]string{
		projectNexus:   displayNexus,
		projectHorizon: displayHorizon,
	})
	if err != nil {
		t.Fatalf("failed to create multi-client: %v", err)
	}
	for _, project := range projects {
		c := client.ClientFor(project)
		c.SetBaseURL(srvURL + "/" + project)
		c.SetUserID(demoUserID)
	}
	return client
}

func TestServerProjectScoping(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	client := newDemoMultiClient(t, srv.URL)

	prs, err := client.ListPullRequests(25)
	if err != nil {
		t.Fatalf("ListPullRequests failed: %v", err)
	}
	if len(prs) != len(mockPullRequests()) {
		t.Errorf("expected %d PRs across both projects (no duplication), got %d", len(mockPullRequests()), len(prs))
	}

	items, err := client.ListWorkItems(50)
	if err != nil {
		t.Fatalf("ListWorkItems failed: %v", err)
	}
	if len(items) != len(mockWorkItems()) {
		t.Errorf("expected %d work items across both projects (no duplication), got %d", len(mockWorkItems()), len(items))
	}

	runs, err := client.ListPipelineRuns(25)
	if err != nil {
		t.Fatalf("ListPipelineRuns failed: %v", err)
	}
	if len(runs) != len(mockPipelineRuns()) {
		t.Errorf("expected %d pipeline runs across both projects (no duplication), got %d", len(mockPipelineRuns()), len(runs))
	}
}

func TestServerPullRequestsReviewerFilter(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/" + projectNexus +
		"/git/pullrequests?api-version=7.1&$top=50&searchCriteria.status=active&searchCriteria.reviewerId=" + demoUserID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result azdevops.PullRequestsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Nexus PRs where Alex Chen is a reviewer: 1039 and 1037.
	if len(result.Value) != 2 {
		t.Fatalf("expected 2 nexus PRs with demo user as reviewer, got %d", len(result.Value))
	}
	for _, pr := range result.Value {
		found := false
		for _, reviewer := range pr.Reviewers {
			if reviewer.ID == demoUserID {
				found = true
			}
		}
		if !found {
			t.Errorf("PR %d returned without demo user among reviewers", pr.ID)
		}
	}
}

func TestServerWIQLAssignedToMe(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	query := `{"query":"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.AssignedTo] = @Me AND [System.State] <> 'Closed'"}`
	resp, err := http.Post(srv.URL+"/"+projectNexus+"/wit/wiql?api-version=7.1&$top=50",
		"application/json", strings.NewReader(query))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result azdevops.WIQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Nexus items assigned to Alex Chen and not Closed: only 5001 (5008 is Closed).
	if len(result.WorkItems) != 1 || result.WorkItems[0].ID != 5001 {
		t.Errorf("expected assigned @Me query to return exactly work item 5001, got %v", result.WorkItems)
	}
}

func TestServerWIQLRecentMentions(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	query := `{"query":"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.Id] IN (@RecentMentions)"}`
	resp, err := http.Post(srv.URL+"/"+projectHorizon+"/wit/wiql?api-version=7.1&$top=50",
		"application/json", strings.NewReader(query))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result azdevops.WIQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.WorkItems) != 1 || result.WorkItems[0].ID != 6004 {
		t.Errorf("expected @RecentMentions query to return exactly work item 6004, got %v", result.WorkItems)
	}
}

func TestServerWorkItemsIDsFilter(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/" + projectNexus + "/wit/workitems?ids=5001,5003&fields=System.Title&api-version=7.1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result azdevops.WorkItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result.Value) != 2 {
		t.Fatalf("expected exactly the 2 requested work items, got %d", len(result.Value))
	}
	for _, item := range result.Value {
		if item.ID != 5001 && item.ID != 5003 {
			t.Errorf("unexpected work item %d in ids-filtered response", item.ID)
		}
	}
}

func TestServerWorkItemComments(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	client := newDemoMultiClient(t, srv.URL)

	comments, err := client.ClientFor(projectNexus).GetWorkItemComments(5002)
	if err != nil {
		t.Fatalf("GetWorkItemComments failed: %v", err)
	}
	if len(comments) == 0 {
		t.Fatal("expected comments for work item 5002")
	}

	found := false
	for _, comment := range comments {
		for _, m := range comment.Mentions {
			if m.TargetID == demoUserID {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected a comment on work item 5002 mentioning the demo user")
	}

	// Add-comment path (POST) must round-trip too.
	created, err := client.ClientFor(projectNexus).AddWorkItemComment(5002, "demo comment")
	if err != nil {
		t.Fatalf("AddWorkItemComment failed: %v", err)
	}
	if created.Text != "demo comment" {
		t.Errorf("expected echoed comment text, got %q", created.Text)
	}
}

// TestDemoNotificationsFeed drives the notifications adapter against the mock
// server wired exactly as demo.Run wires it, and asserts every source
// contributes the expected rows — this is the feed the demo's Notifications
// tab renders on first poll.
func TestDemoNotificationsFeed(t *testing.T) {
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	client := newDemoMultiClient(t, srv.URL)

	store, err := azdevops.NewTriageStore(filepath.Join(t.TempDir(), "notifications.json"))
	if err != nil {
		t.Fatalf("failed to create triage store: %v", err)
	}

	adapter := azdevops.NewAdapterWithNotifications(
		client, store, azdevops.DefaultNotificationLookbackDays,
		azdevops.DefaultNotificationSourceToggles(), 0)

	rows, err := adapter.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	byReason := map[provider.NotificationReason]int{}
	seen := map[string]bool{}
	for _, row := range rows {
		byReason[row.Reason]++
		key := row.Identity.Scope + "/" + row.Identity.ID
		if seen[key] {
			t.Errorf("duplicate notification %q", key)
		}
		seen[key] = true
		if row.Identity.Kind != provider.KindAzure {
			t.Errorf("notification %q has kind %v, want KindAzure", key, row.Identity.Kind)
		}
	}

	want := map[provider.NotificationReason]int{
		provider.NotificationReasonReviewRequested: 3, // PRs 1039, 1037 (nexus), 1040 (horizon)
		provider.NotificationReasonAssigned:        2, // work items 5001 (nexus), 6002 (horizon)
		provider.NotificationReasonMentioned:       2, // work items 5002 (nexus), 6004 (horizon)
		provider.NotificationReasonCIActivity:      2, // runs 8002 (nexus), 8008 (horizon)
	}
	for reason, count := range want {
		if byReason[reason] != count {
			t.Errorf("expected %d %v notifications, got %d", count, reason, byReason[reason])
		}
	}
	if len(rows) != 9 {
		t.Errorf("expected 9 notifications in total, got %d", len(rows))
	}
}
