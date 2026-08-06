package azdevops

import (
	"encoding/json"
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

	rows, err := SourceReviewRequested(mc, 50, now)
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

	rows, err := SourceReviewRequested(mc, 50, time.Now())
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
	_, err := SourceReviewRequested(nil, 50, time.Now())
	if err == nil {
		t.Fatal("expected error for nil MultiClient")
	}
}

func TestSourceReviewRequested_PropagatesListError(t *testing.T) {
	errServer := newErrorServer(t)
	defer errServer.Close()

	// A second, succeeding project is required so len(errs) < len(mc.clients)
	// and ListPullRequestsAsReviewer returns rows alongside a *PartialError
	// rather than an all-projects-failed error — otherwise the errors.As
	// branch below is unreachable-by-construction (task 4 review feedback).
	okServer := newPRServer(t, []PullRequest{
		{
			ID:           1,
			Title:        "Surviving PR",
			Repository:   Repository{ID: "repo-1"},
			Reviewers:    []Reviewer{{ID: "user-1", Vote: 0}},
			CreationDate: time.Now(),
		},
	})
	defer okServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": errServer,
		"beta":  okServer,
	})
	setUserIDs(mc, "user-1")

	rows, err := SourceReviewRequested(mc, 50, time.Now())
	if err == nil {
		t.Fatal("expected error to propagate from ListPullRequestsAsReviewer")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a PartialError with one project failing and one succeeding, got: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the surviving project's row to be returned alongside the PartialError, got %d rows", len(rows))
	}
	if rows[0].Identity.Scope != "beta" {
		t.Errorf("Identity.Scope = %q, want the surviving project %q", rows[0].Identity.Scope, "beta")
	}
}

// --- Activity stamp (decision 2's resurrection requirement) ---

func TestPrActivityStamp_UsesLastMergeSourceCommitDate(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pushed := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)

	pr := PullRequest{
		CreationDate: created,
		LastMergeSourceCommit: &GitCommitRef{
			CommitID:  "abc123",
			Committer: GitUserDate{Date: pushed},
		},
	}

	got := prActivityStamp(pr, now)
	if !got.Equal(pushed) {
		t.Errorf("prActivityStamp() = %v, want the pushed commit date %v (not CreationDate %v)", got, pushed, created)
	}
}

func TestPrActivityStamp_FallsBackToCreationDate(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

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
			got := prActivityStamp(tt.pr, now)
			if !got.Equal(created) {
				t.Errorf("prActivityStamp() = %v, want CreationDate %v", got, created)
			}
		})
	}
}

// TestPrActivityStamp_ClampsFutureCommitterDate pins the fix for the task 4
// review's second 🟡: a user-settable or clock-skewed committer date
// (GIT_COMMITTER_DATE, rebase --committer-date-is-author-date, a build agent
// with a wrong clock) must never produce a stamp after now — Reconcile
// stores whatever this returns as LastActivity unconditionally, and an
// unclamped future stamp would raise the bar past anything a real push could
// ever clear, freezing the row's triage state permanently.
func TestPrActivityStamp_ClampsFutureCommitterDate(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(5 * 365 * 24 * time.Hour) // e.g. a commit dated 2031

	pr := PullRequest{
		CreationDate: now.Add(-time.Hour),
		LastMergeSourceCommit: &GitCommitRef{
			CommitID:  "abc123",
			Committer: GitUserDate{Date: future},
		},
	}

	got := prActivityStamp(pr, now)
	if !got.Equal(now) {
		t.Errorf("prActivityStamp() = %v, want it clamped to now (%v), not the future committer date %v", got, now, future)
	}
}

// TestPrActivityStamp_ClampsFutureCreationDate covers the CreationDate
// fallback path taking the same clamp — a malformed or clock-skewed
// creationDate must not escape either.
func TestPrActivityStamp_ClampsFutureCreationDate(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)

	pr := PullRequest{CreationDate: future}

	got := prActivityStamp(pr, now)
	if !got.Equal(now) {
		t.Errorf("prActivityStamp() = %v, want it clamped to now (%v), not the future CreationDate %v", got, now, future)
	}
}

// TestPrActivityStamp_WireFieldNames pins the wire field names by
// unmarshaling a raw JSON string literal shaped like Azure's real
// GetPullRequests response, rather than round-tripping the same Go struct
// the client unmarshals (which would round-trip symmetrically even under a
// misspelled tag). The committer date (09:00) is deliberately older than
// creationDate (10:00) — the realistic case, since you push the branch
// before opening the PR — so this also proves CreationDate is not read as a
// lower bound when a genuine, older commit date exists. A typo in any of the
// three json tags (pullRequestId, creationDate, lastMergeSourceCommit,
// committer, date) leaves LastMergeSourceCommit nil or its Committer.Date
// zero, which would make this test fail by falling back to CreationDate.
func TestPrActivityStamp_WireFieldNames(t *testing.T) {
	raw := `{
		"pullRequestId": 42,
		"title": "Add widget",
		"creationDate": "2026-01-01T10:00:00Z",
		"lastMergeSourceCommit": {
			"commitId": "abc123",
			"committer": {
				"date": "2026-01-01T09:00:00Z"
			}
		}
	}`

	var pr PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	if pr.LastMergeSourceCommit == nil {
		t.Fatal("LastMergeSourceCommit is nil: \"lastMergeSourceCommit\" or \"commitId\" json tag is misspelled")
	}
	if pr.LastMergeSourceCommit.Committer.Date.IsZero() {
		t.Fatal("Committer.Date is zero: \"committer\" or \"date\" json tag is misspelled")
	}

	wantCommitterDate := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	wantCreationDate := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	if !pr.CreationDate.Equal(wantCreationDate) {
		t.Fatalf("CreationDate = %v, want %v: \"creationDate\" json tag is misspelled", pr.CreationDate, wantCreationDate)
	}

	now := wantCreationDate.Add(time.Hour)
	got := prActivityStamp(pr, now)
	if !got.Equal(wantCommitterDate) {
		t.Errorf("prActivityStamp() = %v, want the older committer date %v (not the newer CreationDate %v) — a misspelled json tag would fall back to CreationDate", got, wantCommitterDate, wantCreationDate)
	}
}

// TestSourceReviewRequested_NewPushResurrectsDismissedRow pins the exact
// spec requirement: "the activity stamp ... must be the PR's last-update
// timestamp, not the creation time — a new push has to resurrect a row the
// user already dismissed." It drives SourceReviewRequested's output through
// Reconcile end to end, not just the stamp in isolation, and serves both
// polls from a real server round-trip (rather than synthesising the second
// with mapReviewRequested) so lastMergeSourceCommit genuinely travels over
// the wire for the push. ProjectName is set explicitly on the fixture so
// both polls' rows agree on Identity.Scope, since a bare struct literal for
// the second poll would otherwise carry a zero-value ProjectName that only
// MultiClient's fan-out normally populates.
func TestSourceReviewRequested_NewPushResurrectsDismissedRow(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstPush := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	pr := PullRequest{
		ID:           7,
		Title:        "Add widget",
		CreationDate: created,
		ProjectName:  "alpha",
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

	now := firstPush.Add(time.Hour)
	rows, err := SourceReviewRequested(mc, 50, now)
	if err != nil {
		t.Fatalf("SourceReviewRequested failed: %v", err)
	}

	rows, state := Reconcile(rows, TriageState{}, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first reconcile, got %d", len(rows))
	}
	firstScope := rows[0].Identity.Scope
	// User dismisses (marks done) the row.
	entry := state[rows[0].Identity.ID]
	entry.Done = true
	state[rows[0].Identity.ID] = entry

	// A re-poll with no new activity: the row must stay dropped (done).
	rows2, state2 := Reconcile(rows, state, now.Add(time.Minute))
	if len(rows2) != 0 {
		t.Fatalf("expected the done row to stay dropped on an unchanged poll, got %d rows", len(rows2))
	}

	// Simulate a new push via a second, real server round-trip:
	// lastMergeSourceCommit advances, exactly as Azure would report it after
	// a push to the source branch.
	newPush := firstPush.Add(24 * time.Hour)
	pushedPR := pr
	pushedPR.LastMergeSourceCommit = &GitCommitRef{
		CommitID:  "commit-2",
		Committer: GitUserDate{Date: newPush},
	}
	pushedServer := newPRServer(t, []PullRequest{pushedPR})
	defer pushedServer.Close()
	pushedMC := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": pushedServer})
	setUserIDs(pushedMC, "user-1")

	pushedNow := newPush.Add(time.Minute)
	pushedRows, err := SourceReviewRequested(pushedMC, 50, pushedNow)
	if err != nil {
		t.Fatalf("SourceReviewRequested (second poll) failed: %v", err)
	}
	if len(pushedRows) != 1 {
		t.Fatalf("expected 1 row from the second poll, got %d", len(pushedRows))
	}
	if pushedRows[0].Identity.Scope != firstScope {
		t.Fatalf("second poll's Identity.Scope = %q, want %q to match the first poll", pushedRows[0].Identity.Scope, firstScope)
	}

	rows3, _ := Reconcile(pushedRows, state2, pushedNow)
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
	row := mapReviewRequested(mc, pr, time.Now())
	if row.WebURL != "" {
		t.Errorf("WebURL = %q, want empty string (legal degraded result)", row.WebURL)
	}
}
