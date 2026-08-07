package azdevops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// This compile-time assertion belongs in the default build, not behind a
// //go:build adapter tag that nothing runs (following
// internal/github/adapter_notifications_test.go's own precedent, which in
// turn follows provider/composite_test.go and provider/notifications_test.go
// for the untagged var _ pattern).
// ---------------------------------------------------------------------------

var _ provider.NotificationSource = (*Adapter)(nil)

// TestAdapter_DoesNotImplementPollIntervalHinter pins decision 10: Azure
// DevOps has no server-suggested polling cadence, so Adapter must not
// satisfy provider.PollIntervalHinter (unlike github.Adapter, which forwards
// GitHub's X-Poll-Interval header). Go has no compile-time "does not
// implement" construct, so this is a runtime type assertion, mirroring
// provider/composite_notifications_test.go's identical check on
// fakeNotifyBackend.
func TestAdapter_DoesNotImplementPollIntervalHinter(t *testing.T) {
	var src provider.NotificationSource = NewAdapterWithNotifications(nil, nil, 0, DefaultNotificationSourceToggles())
	if _, ok := src.(provider.PollIntervalHinter); ok {
		t.Fatal("Adapter must not implement PollIntervalHinter — Azure DevOps has no equivalent hint (decision 10)")
	}
}

// ---------------------------------------------------------------------------
// nil-configuration guards — List/MarkRead/MarkDone must error, never panic.
// ---------------------------------------------------------------------------

func TestAdapter_List_NilClient_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, nil, 0, DefaultNotificationSourceToggles())
	got, err := a.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want a descriptive error for a nil client")
	}
	if got != nil {
		t.Fatalf("List() result = %+v, want nil alongside the error", got)
	}
	if want := "azdevops: notifications: no client configured"; err.Error() != want {
		t.Errorf("List() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkRead_NilStore_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, nil, 0, DefaultNotificationSourceToggles())
	err := a.MarkRead(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/1"})
	if err == nil {
		t.Fatal("MarkRead() error = nil, want a descriptive error for a nil store")
	}
	if want := "azdevops: mark read: no notifications store configured"; err.Error() != want {
		t.Errorf("MarkRead() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkDone_NilStore_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, nil, 0, DefaultNotificationSourceToggles())
	err := a.MarkDone(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/1"})
	if err == nil {
		t.Fatal("MarkDone() error = nil, want a descriptive error for a nil store")
	}
	if want := "azdevops: mark done: no notifications store configured"; err.Error() != want {
		t.Errorf("MarkDone() error = %q, want %q", err.Error(), want)
	}
}

func newTestTriageStore(t *testing.T) *TriageStore {
	t.Helper()
	store, err := NewTriageStore(t.TempDir() + "/notifications.yaml")
	if err != nil {
		t.Fatalf("NewTriageStore: %v", err)
	}
	// No debounced write should ever land mid-test: assertions read State()
	// directly rather than the on-disk file.
	store.SetDebounce(time.Hour)
	return store
}

func TestAdapter_MarkRead_WrongKind_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, newTestTriageStore(t), 0, DefaultNotificationSourceToggles())
	err := a.MarkRead(provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"})
	if err == nil {
		t.Fatal("MarkRead() error = nil, want a descriptive error for a mismatched Kind")
	}
	if want := `azdevops: mark read: identity kind "github" is not "azure"`; err.Error() != want {
		t.Errorf("MarkRead() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkDone_WrongKind_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, newTestTriageStore(t), 0, DefaultNotificationSourceToggles())
	err := a.MarkDone(provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"})
	if err == nil {
		t.Fatal("MarkDone() error = nil, want a descriptive error for a mismatched Kind")
	}
	if want := `azdevops: mark done: identity kind "github" is not "azure"`; err.Error() != want {
		t.Errorf("MarkDone() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkRead_UpdatesLocalTriageState(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())

	id := provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}
	if err := a.MarkRead(id); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	entry, ok := store.State()["review/pr/42"]
	if !ok {
		t.Fatal("MarkRead did not create a triage entry")
	}
	if !entry.Read {
		t.Error("entry.Read = false, want true after MarkRead")
	}
	if entry.LastActivity.IsZero() || entry.LastSeen.IsZero() {
		t.Errorf("entry = %+v, want a non-zero LastActivity and LastSeen", entry)
	}
}

func TestAdapter_MarkDone_UpdatesLocalTriageState(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())

	id := provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}
	if err := a.MarkDone(id); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	entry, ok := store.State()["review/pr/42"]
	if !ok {
		t.Fatal("MarkDone did not create a triage entry")
	}
	if !entry.Done {
		t.Error("entry.Done = false, want true after MarkDone")
	}
}

// ---------------------------------------------------------------------------
// Combined-fixture composition tests: a single project's httptest server
// backs all four sources at once, routed by real HTTP path/method the same
// way each source's own client method actually calls it — genuinely failing
// at the transport level rather than a hand-built error value, per this
// package's own established trap list.
// ---------------------------------------------------------------------------

// composerFixture backs every request the four sources can issue against a
// single project:
//   - /git/pullrequests -> prs (SourceReviewRequested)
//   - /wit/wiql (POST)  -> a single WorkItemReference for workItem.ID, shared
//     by SourceMentioned's stage 1 and SourceAssigned's WIQL query alike —
//     decision 2 expects the same underlying work item to surface as two
//     independent rows ("mention/wi/N" and "assigned/wi/N") when it matches
//     both sources, so the fixture does not need to discriminate on query
//     text to produce that.
//   - /wit/workitems (GET, lowercase) -> the bulk work item batch, always
//     workItem
//   - /workItems/{id}/comments (GET, capital I) -> mentionComments, for
//     SourceMentioned's stage 2
//   - /build/builds -> runs (SourceCIFailed)
//
// failBuilds, when true, makes the /build/builds route return a 500 instead
// of runs, modelling a genuine transport-level failure of exactly one
// source without touching any of the others' routes.
type composerFixture struct {
	prs             []PullRequest
	workItem        WorkItem
	mentionComments []WorkItemComment
	runs            []PipelineRun
	failBuilds      bool
}

func newComposerServer(t *testing.T, f *composerFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if m := commentsPathRe.FindStringSubmatch(path); m != nil {
			w.Header().Set("Content-Type", "application/json")
			resp := struct {
				TotalCount int               `json:"totalCount"`
				Count      int               `json:"count"`
				Comments   []WorkItemComment `json:"comments"`
			}{Comments: f.mentionComments, Count: len(f.mentionComments), TotalCount: len(f.mentionComments)}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if strings.Contains(path, "/wit/wiql") {
			w.Header().Set("Content-Type", "application/json")
			resp := struct {
				WorkItems []WorkItemReference `json:"workItems"`
			}{WorkItems: []WorkItemReference{{ID: f.workItem.ID}}}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if strings.Contains(path, "/wit/workitems") {
			w.Header().Set("Content-Type", "application/json")
			resp := struct {
				Value []WorkItem `json:"value"`
			}{Value: []WorkItem{f.workItem}}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if strings.Contains(path, "/git/pullrequests") {
			w.Header().Set("Content-Type", "application/json")
			resp := struct {
				Value []PullRequest `json:"value"`
			}{Value: f.prs}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if strings.Contains(path, "/build/builds") {
			if f.failBuilds {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"message":"boom"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			resp := PipelineRunsResponse{Value: f.runs}
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

const composerTestUserID = "user-1"

// newComposerFixture builds a fixture with one distinguishable row per
// source, staggered so UpdatedAt order is unambiguous:
// mention (-30m, newest) > review (-1h) > assigned (-2h) > cifail (-3h,
// oldest). now must be truncated to whole seconds by the caller (RFC3339 /
// JSON round-tripping through the fixture server drops sub-second
// precision), matching every other test in this package that round-trips
// timestamps through an httptest fixture.
func newComposerFixture(now time.Time) *composerFixture {
	finish := now.Add(-3 * time.Hour)
	return &composerFixture{
		prs: []PullRequest{
			{
				ID:           42,
				Title:        "Add widget",
				Repository:   Repository{ID: "repo-1", Name: "myrepo"},
				CreationDate: now.Add(-time.Hour),
			},
		},
		workItem: WorkItem{
			ID: 100,
			Fields: WorkItemFields{
				Title:       "Fix the bug",
				ChangedDate: now.Add(-2 * time.Hour),
			},
		},
		mentionComments: []WorkItemComment{
			{
				ID:          1,
				CreatedDate: now.Add(-30 * time.Minute),
				Mentions:    []CommentMention{{TargetID: composerTestUserID}},
			},
		},
		runs: []PipelineRun{
			{
				ID:           7,
				BuildNumber:  "1",
				Status:       "completed",
				Result:       "failed",
				Definition:   PipelineDefinition{Name: "CI"},
				QueueTime:    now.Add(-4 * time.Hour),
				FinishTime:   &finish,
				RequestedFor: Identity{ID: composerTestUserID},
			},
		},
	}
}

func newComposerAdapter(t *testing.T, f *composerFixture) (*Adapter, *httptest.Server) {
	t.Helper()
	server := newComposerServer(t, f)
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles())
	return a, server
}

// TestAdapter_List_MergesAllFourSourcesDeterministically pins the core
// composition contract: all four sources' rows land in one feed, sorted
// newest-first by UpdatedAt, regardless of goroutine completion order.
func TestAdapter_List_MergesAllFourSourcesDeterministically(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	a, server := newComposerAdapter(t, f)
	defer server.Close()

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4: %+v", len(rows), rows)
	}

	wantIDs := []string{"mention/wi/100", "review/pr/42", "assigned/wi/100", "cifail/run/7"}
	for i, want := range wantIDs {
		if rows[i].Identity.ID != want {
			t.Errorf("rows[%d].Identity.ID = %q, want %q (full order: %v)", i, rows[i].Identity.ID, want, identityIDs(rows))
		}
		if rows[i].Identity.Kind != provider.KindAzure {
			t.Errorf("rows[%d].Identity.Kind = %v, want KindAzure", i, rows[i].Identity.Kind)
		}
	}

	// Determinism: a second call against the same fixture must produce the
	// exact same order, even though goroutine completion order is not
	// controlled by the test — a test that only passed because of timing on
	// one run would not be a test.
	rows2, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("second list() error = %v", err)
	}
	if got, want := identityIDs(rows2), identityIDs(rows); !equalStrings(got, want) {
		t.Errorf("second list() order = %v, want %v (order must not depend on goroutine timing)", got, want)
	}
}

func identityIDs(rows []provider.Notification) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Identity.ID
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAdapter_List_OneSourceFails_Degrades pins task 8's resolved
// absorb-on-error contract: SourceCIFailed's transport genuinely fails (a
// 500 from /build/builds, not a hand-built error), but the other three
// sources' rows still come back with a nil error rather than the whole feed
// emptying.
func TestAdapter_List_OneSourceFails_Degrades(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	f.failBuilds = true
	a, server := newComposerAdapter(t, f)
	defer server.Close()

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v, want nil (a single failing source must degrade, not propagate)", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (review, mentioned, assigned survive; cifail failed): %+v", len(rows), rows)
	}
	for _, row := range rows {
		if row.Identity.ID == "cifail/run/7" {
			t.Errorf("cifail/run/7 present in rows, want it absent since its source failed: %+v", rows)
		}
	}
}

// TestAdapter_List_AllSourcesFail_ReturnsError pins the other half of the
// absorb-or-propagate contract: when every source fails, the caller must see
// an error, never an empty slice masquerading as "you're all caught up".
func TestAdapter_List_AllSourcesFail_ReturnsError(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	}))
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	a := NewAdapterWithNotifications(mc, newTestTriageStore(t), 14, DefaultNotificationSourceToggles())

	rows, err := a.list(provider.NotifOpts{}, now)
	if err == nil {
		t.Fatal("list() error = nil, want an error when every source fails")
	}
	if rows != nil {
		t.Fatalf("list() rows = %+v, want nil alongside the error", rows)
	}
}

// TestAdapter_List_FoldsLocalReadStateAcrossPolls pins the phase-1
// unread-semantics constraint at this adapter's boundary: a subject marked
// done via the local TriageStore must not reappear in the very next List
// call whose underlying data is unchanged, and nothing above List needs to
// know that read/done state is tracked locally rather than by Azure's
// (nonexistent) server-side inbox.
func TestAdapter_List_FoldsLocalReadStateAcrossPolls(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	a, server := newComposerAdapter(t, f)
	defer server.Close()

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	if err := a.MarkDone(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	rows2, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("second list() error = %v", err)
	}
	if len(rows2) != 3 {
		t.Fatalf("len(rows2) = %d, want 3 (review/pr/42 marked done): %+v", len(rows2), rows2)
	}
	for _, row := range rows2 {
		if row.Identity.ID == "review/pr/42" {
			t.Errorf("review/pr/42 present after MarkDone, want it dropped: %+v", rows2)
		}
	}
}

// TestAdapter_List_HonoursMax pins opts.Max truncation, mirroring
// github.Adapter.List's own behaviour (see List's doc comment on task 12).
func TestAdapter_List_HonoursMax(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	a, server := newComposerAdapter(t, f)
	defer server.Close()

	rows, err := a.list(provider.NotifOpts{Max: 2}, now)
	if err != nil {
		t.Fatalf("list() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	wantIDs := []string{"mention/wi/100", "review/pr/42"}
	if got := identityIDs(rows); !equalStrings(got, wantIDs) {
		t.Errorf("rows = %v, want the 2 newest %v", got, wantIDs)
	}
}
