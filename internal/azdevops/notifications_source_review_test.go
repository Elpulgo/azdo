package azdevops

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// setUserIDs stamps every project's underlying *Client with a cached user ID,
// bypassing the real connectionData network call ListPullRequestsAsReviewer
// makes internally to resolve the reviewer id (see Client.SetUserID's doc
// comment — "used by demo mode", reused here for the same reason: no network
// in tests).
func setUserIDs(mc *MultiClient, userID string) {
	for _, p := range mc.Projects() {
		mc.ClientFor(p).SetUserID(userID)
	}
}

func TestSourceReviewRequested_MapsPRToNotification(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	server := newPRServer(t, []PullRequest{
		{
			ID:           42,
			Title:        "Add widget",
			Repository:   Repository{ID: "repo-1", Name: "myrepo"},
			Reviewers:    []Reviewer{{ID: "user-1", Vote: 0}},
			CreationDate: now,
		},
	})
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	rows, err := SourceReviewRequested(mc, 50)
	if err != nil {
		t.Fatalf("SourceReviewRequested failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row.Identity.Kind != provider.KindAzure {
		t.Errorf("Identity.Kind = %v, want KindAzure", row.Identity.Kind)
	}
	if row.Identity.Scope != "alpha" {
		t.Errorf("Identity.Scope = %q, want %q", row.Identity.Scope, "alpha")
	}
	if row.Identity.ID != "review/pr/42" {
		t.Errorf("Identity.ID = %q, want %q", row.Identity.ID, "review/pr/42")
	}
	if row.Title != "Add widget" {
		t.Errorf("Title = %q, want %q", row.Title, "Add widget")
	}
	if row.Reason != provider.NotificationReasonReviewRequested {
		t.Errorf("Reason = %v, want NotificationReasonReviewRequested", row.Reason)
	}
}

func TestSourceReviewRequested_NegativeID_ProducesEmptyIdentityID(t *testing.T) {
	// NotifKey guards <= 0 ids (convention 11): a malformed negative id must
	// not silently produce a non-empty, malformed key.
	server := newPRServer(t, []PullRequest{
		{ID: -1, Title: "Broken", Repository: Repository{ID: "repo-1"}},
	})
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	rows, err := SourceReviewRequested(mc, 50)
	if err != nil {
		t.Fatalf("SourceReviewRequested failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Identity.ID != "" {
		t.Errorf("Identity.ID = %q, want empty for a non-positive PR id", rows[0].Identity.ID)
	}
}

func TestSourceReviewRequested_NilMultiClient_ReturnsError(t *testing.T) {
	_, err := SourceReviewRequested(nil, 50)
	if err == nil {
		t.Fatal("expected error for nil MultiClient")
	}
}

func TestSourceReviewRequested_PropagatesListError(t *testing.T) {
	errServer := newErrorServer(t)
	defer errServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": errServer})
	setUserIDs(mc, "user-1")

	_, err := SourceReviewRequested(mc, 50)
	if err == nil {
		t.Fatal("expected error to propagate from ListPullRequestsAsReviewer")
	}
	var partialErr *PartialError
	if errors.As(err, &partialErr) {
		t.Fatalf("expected an all-projects-failed error, got a PartialError: %v", err)
	}
}

// --- Activity stamp (decision 2's resurrection requirement) ---

func TestPrActivityStamp_UsesLastMergeSourceCommitDate(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pushed := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	pr := PullRequest{
		CreationDate: created,
		LastMergeSourceCommit: &GitCommitRef{
			CommitID:  "abc123",
			Committer: GitUserDate{Date: pushed},
		},
	}

	got := prActivityStamp(pr)
	if !got.Equal(pushed) {
		t.Errorf("prActivityStamp() = %v, want the pushed commit date %v (not CreationDate %v)", got, pushed, created)
	}
}

func TestPrActivityStamp_FallsBackToCreationDate(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		pr   PullRequest
	}{
		{"nil LastMergeSourceCommit", PullRequest{CreationDate: created}},
		{"zero Committer.Date", PullRequest{
			CreationDate:          created,
			LastMergeSourceCommit: &GitCommitRef{CommitID: "abc123"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prActivityStamp(tt.pr)
			if !got.Equal(created) {
				t.Errorf("prActivityStamp() = %v, want CreationDate %v", got, created)
			}
		})
	}
}

// TestSourceReviewRequested_NewPushResurrectsDismissedRow pins the exact
// spec requirement: "the activity stamp ... must be the PR's last-update
// timestamp, not the creation time — a new push has to resurrect a row the
// user already dismissed." It drives SourceReviewRequested's output through
// Reconcile end to end, not just the stamp in isolation.
func TestSourceReviewRequested_NewPushResurrectsDismissedRow(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstPush := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	pr := PullRequest{
		ID:           7,
		Title:        "Add widget",
		CreationDate: created,
		Repository:   Repository{ID: "repo-1"},
		LastMergeSourceCommit: &GitCommitRef{
			CommitID:  "commit-1",
			Committer: GitUserDate{Date: firstPush},
		},
	}

	server := newPRServer(t, []PullRequest{pr})
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	rows, err := SourceReviewRequested(mc, 50)
	if err != nil {
		t.Fatalf("SourceReviewRequested failed: %v", err)
	}

	now := firstPush.Add(time.Hour)
	rows, state := Reconcile(rows, TriageState{}, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first reconcile, got %d", len(rows))
	}
	// User dismisses (marks done) the row.
	entry := state[rows[0].Identity.ID]
	entry.Done = true
	state[rows[0].Identity.ID] = entry

	// A re-poll with no new activity: the row must stay dropped (done).
	rows2, state2 := Reconcile(rows, state, now.Add(time.Minute))
	if len(rows2) != 0 {
		t.Fatalf("expected the done row to stay dropped on an unchanged poll, got %d rows", len(rows2))
	}

	// Simulate a new push: LastMergeSourceCommit advances.
	newPush := firstPush.Add(24 * time.Hour)
	pushedPR := pr
	pushedPR.LastMergeSourceCommit = &GitCommitRef{
		CommitID:  "commit-2",
		Committer: GitUserDate{Date: newPush},
	}
	pushedRow := mapReviewRequested(mc, pushedPR)

	rows3, _ := Reconcile([]provider.Notification{pushedRow}, state2, newPush.Add(time.Minute))
	if len(rows3) != 1 {
		t.Fatalf("expected the pushed PR to resurrect as 1 row, got %d", len(rows3))
	}
	if rows3[0].Done {
		t.Error("resurrected row must not still be Done")
	}
	if rows3[0].Read {
		t.Error("resurrected row must not still be Read")
	}
}

// --- WebURL degradation ladder ---

func TestReviewRequestedWebURL_DegradationLadder(t *testing.T) {
	server := newPRServer(t, nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	tests := []struct {
		name string
		pr   PullRequest
		want string
	}{
		{
			name: "repo and id resolvable: deep link",
			pr:   PullRequest{ID: 42, ProjectName: "alpha", Repository: Repository{ID: "repo-1"}},
			want: "https://dev.azure.com/testorg/alpha/_git/repo-1/pullrequest/42",
		},
		{
			name: "empty repository id: falls back to project page",
			pr:   PullRequest{ID: 42, ProjectName: "alpha", Repository: Repository{ID: ""}},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "non-positive PR id: falls back to project page",
			pr:   PullRequest{ID: 0, ProjectName: "alpha", Repository: Repository{ID: "repo-1"}},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "negative PR id: falls back to project page",
			pr:   PullRequest{ID: -5, ProjectName: "alpha", Repository: Repository{ID: "repo-1"}},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "unresolvable scope: empty (no project page buildable)",
			pr:   PullRequest{ID: 42, ProjectName: "unknown-project", Repository: Repository{ID: "repo-1"}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reviewRequestedWebURL(mc, tt.pr)
			if got != tt.want {
				t.Errorf("reviewRequestedWebURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReviewRequestedWebURL_NilMultiClient(t *testing.T) {
	got := reviewRequestedWebURL(nil, PullRequest{ID: 42, Repository: Repository{ID: "repo-1"}})
	if got != "" {
		t.Errorf("reviewRequestedWebURL(nil, ...) = %q, want empty", got)
	}
}

func TestMapReviewRequested_WebURLIsLegalEmptyString(t *testing.T) {
	server := newPRServer(t, nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	pr := PullRequest{ID: 42, ProjectName: "does-not-exist", Repository: Repository{ID: "repo-1"}}
	row := mapReviewRequested(mc, pr)
	if row.WebURL != "" {
		t.Errorf("WebURL = %q, want empty string (legal degraded result)", row.WebURL)
	}
}
