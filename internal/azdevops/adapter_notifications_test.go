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

// TestNewAdapterWithNotifications_LookbackDays_ClampedToMax pins task-11
// review finding 3(a): NewAdapterWithNotifications clamps lookbackDays to
// MaxNotificationLookbackDays itself, regardless of caller. This closes the
// gap internal/config.LoadFrom's own AzureLookbackDaysMax clamp leaves open
// for any other caller — a direct construction bypassing LoadFrom entirely
// (a test, a future caller, a bug) could otherwise still reach Azure with a
// window wide enough to outlive orphanTTL, reintroducing the
// resurfaces-as-unread gap the clamp exists to close.
func TestNewAdapterWithNotifications_LookbackDays_ClampedToMax(t *testing.T) {
	tests := []struct {
		name         string
		lookbackDays int
		want         int
	}{
		{name: "zero falls back to the default", lookbackDays: 0, want: DefaultNotificationLookbackDays},
		{name: "negative falls back to the default", lookbackDays: -1, want: DefaultNotificationLookbackDays},
		{name: "at the max is unaffected", lookbackDays: MaxNotificationLookbackDays, want: MaxNotificationLookbackDays},
		{name: "above the max is clamped down to it", lookbackDays: MaxNotificationLookbackDays + 1, want: MaxNotificationLookbackDays},
		{name: "far above the max is clamped down to it", lookbackDays: 90, want: MaxNotificationLookbackDays},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdapterWithNotifications(nil, nil, tt.lookbackDays, DefaultNotificationSourceToggles())
			if a.notifLookbackDays != tt.want {
				t.Errorf("notifLookbackDays = %d, want %d", a.notifLookbackDays, tt.want)
			}
		})
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

// TestAdapter_List_NilStore_ReturnsDescriptiveError pins task 8 review's 🔴
// finding 1: the zero-value *Adapter from a bare NewAdapter(mc) still
// satisfies provider.NotificationSource by method set alone (Go's
// structural typing does not care that notifStore was never wired up), so
// list's own nil-notifStore guard — not a panic, not a silently-forever-
// empty feed with no error — is what a caller relying on that structural
// typing (the composite provider's Notifications tab capability check)
// actually gets back. mc is deliberately non-nil here (a real, reachable
// client) so this exercises the notifStore guard specifically, distinct
// from TestAdapter_List_NilClient_ReturnsDescriptiveError above.
func TestAdapter_List_NilStore_ReturnsDescriptiveError(t *testing.T) {
	server := newComposerServer(t, newComposerFixture(time.Now().UTC().Truncate(time.Second)))
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)

	a := NewAdapterWithNotifications(mc, nil, 0, DefaultNotificationSourceToggles())
	got, err := a.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want a descriptive error for a nil store")
	}
	if got != nil {
		t.Fatalf("List() result = %+v, want nil alongside the error", got)
	}
	if want := "azdevops: notifications: not configured"; err.Error() != want {
		t.Errorf("List() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkRead_NilStore_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, nil, 0, DefaultNotificationSourceToggles())
	err := a.MarkRead(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/1"})
	if err == nil {
		t.Fatal("MarkRead() error = nil, want a descriptive error for a nil store")
	}
	if want := "azdevops: mark read: notifications not configured"; err.Error() != want {
		t.Errorf("MarkRead() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkDone_NilStore_ReturnsDescriptiveError(t *testing.T) {
	a := NewAdapterWithNotifications(nil, nil, 0, DefaultNotificationSourceToggles())
	err := a.MarkDone(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/1"})
	if err == nil {
		t.Fatal("MarkDone() error = nil, want a descriptive error for a nil store")
	}
	if want := "azdevops: mark done: notifications not configured"; err.Error() != want {
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
	// Mirrors TestAdapter_MarkRead_UpdatesLocalTriageState's stamp
	// assertions: review's finding 2 found MarkDone's LastActivity/LastSeen
	// backfill (adapter_notifications.go) unpinned — deleting either
	// `if entry.LastActivity.IsZero() { entry.LastActivity = now }` or the
	// LastSeen equivalent left the whole suite green, because this test
	// previously asserted only entry.Done.
	if entry.LastActivity.IsZero() || entry.LastSeen.IsZero() {
		t.Errorf("entry = %+v, want a non-zero LastActivity and LastSeen", entry)
	}
}

// TestAdapter_MarkRead_EmptyID_ReturnsErrorAndCreatesNoEntry pins the
// empty-key guard: an empty Identity.ID must never reach the store as a map
// key, since it would be shared by every malformed row Reconcile already
// refuses to track (NotifKey's and Reconcile's own doc comments) — a
// permanently un-addressable entry that would otherwise sit in
// notifications.yaml until orphanTTL happened to prune it.
func TestAdapter_MarkRead_EmptyID_ReturnsErrorAndCreatesNoEntry(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())

	err := a.MarkRead(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: ""})
	if err == nil {
		t.Fatal("MarkRead() error = nil, want a descriptive error for an empty identity id")
	}
	if want := "azdevops: mark read: empty identity id"; err.Error() != want {
		t.Errorf("MarkRead() error = %q, want %q", err.Error(), want)
	}
	if got := store.State(); len(got) != 0 {
		t.Errorf("store.State() = %+v, want no entry written for an empty id", got)
	}
}

// TestAdapter_MarkDone_EmptyID_ReturnsErrorAndCreatesNoEntry mirrors
// TestAdapter_MarkRead_EmptyID_ReturnsErrorAndCreatesNoEntry for MarkDone.
func TestAdapter_MarkDone_EmptyID_ReturnsErrorAndCreatesNoEntry(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())

	err := a.MarkDone(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: ""})
	if err == nil {
		t.Fatal("MarkDone() error = nil, want a descriptive error for an empty identity id")
	}
	if want := "azdevops: mark done: empty identity id"; err.Error() != want {
		t.Errorf("MarkDone() error = %q, want %q", err.Error(), want)
	}
	if got := store.State(); len(got) != 0 {
		t.Errorf("store.State() = %+v, want no entry written for an empty id", got)
	}
}

// TestAdapter_MarkRead_AlreadyRead_IsNoOp pins that marking an already-read
// id does not churn LastActivity/LastSeen and does not re-dirty the
// debounced store (TriageStore.ApplyIfChanged). The debounce is set far
// longer than the test's own runtime, so a second write scheduling a new
// timer would be observable as store.dirty flipping back to true after a
// Flush already cleared it.
func TestAdapter_MarkRead_AlreadyRead_IsNoOp(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())
	id := provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}

	if err := a.MarkRead(id); err != nil {
		t.Fatalf("first MarkRead: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	first := store.State()["review/pr/42"]

	store.mu.Lock()
	store.dirty = false
	store.mu.Unlock()

	if err := a.MarkRead(id); err != nil {
		t.Fatalf("second MarkRead: %v", err)
	}

	store.mu.Lock()
	dirty := store.dirty
	store.mu.Unlock()
	if dirty {
		t.Error("store.dirty = true after re-marking an already-read id, want false (no-op)")
	}

	second := store.State()["review/pr/42"]
	if !second.LastActivity.Equal(first.LastActivity) {
		t.Errorf("LastActivity changed on a no-op MarkRead: first = %v, second = %v", first.LastActivity, second.LastActivity)
	}
	if !second.LastSeen.Equal(first.LastSeen) {
		t.Errorf("LastSeen changed on a no-op MarkRead: first = %v, second = %v", first.LastSeen, second.LastSeen)
	}
}

// TestAdapter_MarkDone_AlreadyDone_IsNoOp mirrors
// TestAdapter_MarkRead_AlreadyRead_IsNoOp for MarkDone.
//
// The first.LastActivity/LastSeen.IsZero() checks below are not redundant
// with TestAdapter_MarkDone_UpdatesLocalTriageState's own backfill
// assertions: review's finding 2 found that without them, this test alone is
// a tautology against the "delete MarkDone's LastActivity/LastSeen backfill"
// mutant — with the backfill gone, first and second are both the zero time,
// and Equal(zero, zero) still passes, so the no-op assertions below would
// keep passing even though the mutated code no longer stamps a fresh entry
// at all.
func TestAdapter_MarkDone_AlreadyDone_IsNoOp(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())
	id := provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}

	if err := a.MarkDone(id); err != nil {
		t.Fatalf("first MarkDone: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	first := store.State()["review/pr/42"]
	if first.LastActivity.IsZero() || first.LastSeen.IsZero() {
		t.Fatalf("first = %+v, want a non-zero LastActivity and LastSeen after the first MarkDone (otherwise the no-op checks below would tautologically compare two zero times)", first)
	}

	store.mu.Lock()
	store.dirty = false
	store.mu.Unlock()

	if err := a.MarkDone(id); err != nil {
		t.Fatalf("second MarkDone: %v", err)
	}

	store.mu.Lock()
	dirty := store.dirty
	store.mu.Unlock()
	if dirty {
		t.Error("store.dirty = true after re-marking an already-done id, want false (no-op)")
	}

	second := store.State()["review/pr/42"]
	if !second.LastActivity.Equal(first.LastActivity) {
		t.Errorf("LastActivity changed on a no-op MarkDone: first = %v, second = %v", first.LastActivity, second.LastActivity)
	}
	if !second.LastSeen.Equal(first.LastSeen) {
		t.Errorf("LastSeen changed on a no-op MarkDone: first = %v, second = %v", first.LastSeen, second.LastSeen)
	}
}

// TestAdapter_MarkDone_UnseenID_CreatesEntryWithSaneLastActivity mirrors
// TestAdapter_MarkRead_UnseenID_CreatesEntryWithSaneLastActivity for
// MarkDone: an entry created by a mark on a subject Reconcile has never seen
// must carry a LastActivity at least as new as any real row's UpdatedAt for
// the same subject, or the very next poll's Reconcile would read the mark as
// stale activity and clear Done — resurrecting a row the user just
// dismissed. Because Reconcile drops done rows from its returned slice
// entirely (rather than returning them with Done=true), the assertion here
// is the row's absence from the next poll's result, not a field on it the
// way MarkRead's twin asserts Read=true.
func TestAdapter_MarkDone_UnseenID_CreatesEntryWithSaneLastActivity(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())
	id := provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/99"}

	before := time.Now()
	if err := a.MarkDone(id); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	after := time.Now()

	entry, ok := store.State()["review/pr/99"]
	if !ok {
		t.Fatal("MarkDone did not create a triage entry for an unseen id")
	}
	if entry.LastActivity.Before(before) || entry.LastActivity.After(after) {
		t.Fatalf("entry.LastActivity = %v, want it within [%v, %v]", entry.LastActivity, before, after)
	}

	// A poll landing after the mark, whose row for the same subject is
	// stamped from well before the mark, must not resurrect the row.
	row := provider.Notification{
		Identity:  id,
		UpdatedAt: before.Add(-time.Hour),
	}
	rows, _ := Reconcile([]provider.Notification{row}, store.State(), after)
	if len(rows) != 0 {
		t.Fatalf("Reconcile returned %d rows, want 0 — a done mark must survive the very next poll, not be resurrected by it: %+v", len(rows), rows)
	}
}

// TestAdapter_MarkRead_UnseenID_CreatesEntryWithSaneLastActivity pins the
// task-9 invariant Reconcile's own doc comment records: an entry written by
// a mark rather than by Reconcile must carry a LastActivity at least as new
// as any real row's UpdatedAt for the same subject, or the very next poll's
// Reconcile would read the mark as stale activity and immediately clear it
// (Reconcile's "strictly newer" branch). Feeding the freshly created entry
// straight back through Reconcile with a row stamped well in the past proves
// the mark survives that poll rather than being resurrected by it.
func TestAdapter_MarkRead_UnseenID_CreatesEntryWithSaneLastActivity(t *testing.T) {
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(nil, store, 0, DefaultNotificationSourceToggles())
	id := provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/99"}

	before := time.Now()
	if err := a.MarkRead(id); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	after := time.Now()

	entry, ok := store.State()["review/pr/99"]
	if !ok {
		t.Fatal("MarkRead did not create a triage entry for an unseen id")
	}
	if entry.LastActivity.Before(before) || entry.LastActivity.After(after) {
		t.Fatalf("entry.LastActivity = %v, want it within [%v, %v]", entry.LastActivity, before, after)
	}

	// A poll landing after the mark, whose row for the same subject is
	// stamped from well before the mark, must not resurrect the row as
	// unread.
	row := provider.Notification{
		Identity:  id,
		UpdatedAt: before.Add(-time.Hour),
	}
	rows, _ := Reconcile([]provider.Notification{row}, store.State(), after)
	if len(rows) != 1 {
		t.Fatalf("Reconcile returned %d rows, want 1", len(rows))
	}
	if !rows[0].Read {
		t.Errorf("rows[0].Read = false after Reconcile, want true — the mark must survive the very next poll")
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
// source without touching any of the others' routes. failPRs is the same
// idea for /git/pullrequests, used to model one project of a multi-project
// MultiClient failing SourceReviewRequested's query while every other
// route on that same project still answers normally (task 8 review, 🟡
// finding 3's two-project fixture).
type composerFixture struct {
	prs             []PullRequest
	workItem        WorkItem
	mentionComments []WorkItemComment
	runs            []PipelineRun
	failBuilds      bool
	failPRs         bool
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
			if f.failPRs {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"message":"boom"}`))
				return
			}
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

// TestAdapter_List_PartialProjectFailure_KeepsSurvivingProjectRows pins task
// 8 review's 🟡 finding 3: runSourcesConcurrently's
// `rows = append(rows, r.rows...)` must run unconditionally, even for a
// source whose result carries a non-nil error alongside real rows (Azure's
// own rows-plus-*PartialError contract — see
// SourceReviewRequested/MultiClient.ListPullRequestsAsReviewerForUser's doc
// comments). newComposerAdapter's single-project fixture can never exercise
// this: with one project, a query either fully succeeds or fully fails,
// there is no "some projects gave rows, one didn't" case to absorb. This
// test is the two-project fixture that closes that gap: "alpha" is healthy,
// "beta" 500s specifically on /git/pullrequests (SourceReviewRequested)
// while answering every other route (wiql, workitems, comments, builds)
// with valid empty results, so only SourceReviewRequested's beta half
// fails — the other three sources see beta as a legitimate zero-row
// project, not an error.
func TestAdapter_List_PartialProjectFailure_KeepsSurvivingProjectRows(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	alphaServer := newComposerServer(t, newComposerFixture(now))
	defer alphaServer.Close()

	betaServer := newComposerServer(t, &composerFixture{failPRs: true})
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})
	setUserIDs(mc, composerTestUserID)
	a := NewAdapterWithNotifications(mc, newTestTriageStore(t), 14, DefaultNotificationSourceToggles())

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v, want nil (beta's SourceReviewRequested failure alone must not propagate — alpha and the other three sources are healthy)", err)
	}

	found := false
	for _, row := range rows {
		if row.Identity.ID == "review/pr/42" && row.Identity.Scope == "alpha" {
			found = true
		}
	}
	if !found {
		t.Errorf("rows = %v, want alpha's review/pr/42 present despite beta's ListPullRequestsAsReviewerForUser 500 (a rows-plus-error result must still contribute its rows)", identityIDs(rows))
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
	if want := "azdevops: notifications: all 4 sources failed:"; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("list() error = %q, want it to start with %q (provider-attributed, matching every other error in this package)", err.Error(), want)
	}
}

// TestAdapter_List_AllRowsTriagedAway_ReturnsEmptyFeedNotError pins task 8
// review's 🟡 finding 4: the absorb-or-propagate decision must be keyed on
// how many *sources* succeeded, not on how many rows the poll happened to
// return. Here every one of the four sources answers successfully, but
// every subject they surface has already been marked done in a prior poll
// — a legitimate "you're all caught up" empty feed, not an outage, and
// list must return (empty, nil), never an error, purely because
// len(reconciled) == 0.
func TestAdapter_List_AllRowsTriagedAway_ReturnsEmptyFeedNotError(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	a, server := newComposerAdapter(t, f)
	defer server.Close()

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("first list() error = %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("first list() returned no rows to triage away — fixture is not producing the expected subjects")
	}
	for _, row := range rows {
		if err := a.MarkDone(row.Identity); err != nil {
			t.Fatalf("MarkDone(%+v): %v", row.Identity, err)
		}
	}

	rows2, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("second list() error = %v, want nil (every source succeeded; zero rows is a legitimate empty feed, not a failure)", err)
	}
	if len(rows2) != 0 {
		t.Fatalf("second list() rows = %+v, want none — every subject was marked done", rows2)
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

// ---------------------------------------------------------------------------
// NotificationSourceToggles field-false coverage. Every test above this
// point uses DefaultNotificationSourceToggles() (all four true); nothing in
// this package otherwise exercises a single toggle set to false, let alone
// all four, leaving runSourcesConcurrently's per-toggle `if` branches (and
// its userID-resolve skip when none of the three id-dependent sources are
// enabled) unpinned.
// ---------------------------------------------------------------------------

// TestAdapter_List_CIFailedToggleOff_ExcludesSourceFromFeed pins that a
// single disabled source is excluded from the merged feed even though its
// fixture data would otherwise produce a row, and that the other three
// sources are unaffected.
func TestAdapter_List_CIFailedToggleOff_ExcludesSourceFromFeed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	toggles := DefaultNotificationSourceToggles()
	toggles.CIFailed = false
	a := NewAdapterWithNotifications(mc, newTestTriageStore(t), 14, toggles)

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (cifail excluded by its toggle): %+v", len(rows), identityIDs(rows))
	}
	for _, row := range rows {
		if row.Identity.ID == "cifail/run/7" {
			t.Errorf("cifail/run/7 present with CIFailed toggled off: %+v", identityIDs(rows))
		}
	}
}

// TestAdapter_List_AllTogglesOff_ReturnsEmptyFeedNotError pins list's
// jobCount == 0 absorb branch (task 8 review, 🟡 finding 4): with every
// source disabled, runSourcesConcurrently runs zero jobs and skips the
// userID resolve entirely, and list must treat that as "every enabled
// source succeeded" (there were none to fail) rather than propagating an
// error.
func TestAdapter_List_AllTogglesOff_ReturnsEmptyFeedNotError(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	// No userID stamped on the client at all: if runSourcesConcurrently's
	// toggle-off resolve skip regressed, this would fail closed with a
	// resolveAuthenticatedUserID error over the network instead of quietly
	// producing an empty feed, catching the regression outright.
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	a := NewAdapterWithNotifications(mc, newTestTriageStore(t), 14, NotificationSourceToggles{})

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v, want nil (zero enabled sources is a legal empty feed, not a failure)", err)
	}
	if len(rows) != 0 {
		t.Fatalf("list() rows = %+v, want none — every source is disabled", rows)
	}
}

// TestSortNotificationsDeterministically_TieBreaksOnScopeThenID pins the
// two tie-break branches sortNotificationsDeterministically's own doc
// comment claims but nothing previously exercised: every other test's
// fixture rows carry distinct UpdatedAt stamps, so ties never reach the
// Scope/ID comparisons at all.
func TestSortNotificationsDeterministically_TieBreaksOnScopeThenID(t *testing.T) {
	same := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	rows := []provider.Notification{
		{Identity: provider.Identity{Kind: provider.KindAzure, Scope: "beta", ID: "review/pr/1"}, UpdatedAt: same},
		{Identity: provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/2"}, UpdatedAt: same},
		{Identity: provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/1"}, UpdatedAt: same},
	}

	sortNotificationsDeterministically(rows)

	want := []string{"review/pr/1", "review/pr/2", "review/pr/1"}
	wantScopes := []string{"alpha", "alpha", "beta"}
	for i, row := range rows {
		if row.Identity.Scope != wantScopes[i] {
			t.Fatalf("rows[%d].Identity.Scope = %q, want %q (order: %v)", i, row.Identity.Scope, wantScopes[i], identityIDs(rows))
		}
		if row.Identity.ID != want[i] {
			t.Fatalf("rows[%d].Identity.ID = %q, want %q (order: %v)", i, row.Identity.ID, want[i], identityIDs(rows))
		}
	}
}
