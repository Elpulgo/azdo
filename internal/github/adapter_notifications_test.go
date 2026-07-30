package github_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// Decision 38: this compile-time assertion belongs in the default build, not
// behind //go:build adapter (nothing runs that tag — see Decision 38's
// rationale). Following provider/composite_test.go:13 and
// provider/notifications_test.go:25 for the untagged var _ pattern.
// ---------------------------------------------------------------------------

var _ provider.NotificationSource = (*github.Adapter)(nil)

// ---------------------------------------------------------------------------
// Decision 40: task 15's PollIntervalHinter does not exist yet. Declared
// locally here and asserted against *github.Adapter; task 15 defines the real
// interface where it is consumed and asserts again. Go's structural typing
// makes the two independent: if Adapter loses PollInterval(), this assertion
// breaks immediately.
// ---------------------------------------------------------------------------

type pollIntervalHinter interface {
	PollInterval() time.Duration
}

var _ pollIntervalHinter = (*github.Adapter)(nil)

// ---------------------------------------------------------------------------
// nil nc — List/MarkRead/MarkDone must error, never panic (Decision 39).
// ---------------------------------------------------------------------------

func TestAdapter_List_NilNotificationsClient_ReturnsDescriptiveError(t *testing.T) {
	mc, err := github.NewMultiClient([]string{"o/r"}, "tok", github.DefaultLabelConvention(), nil)
	if err != nil {
		t.Fatalf("NewMultiClient: %v", err)
	}
	a := github.NewAdapter(mc) // nc left nil

	got, err := a.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want a descriptive error for a nil notifications client")
	}
	if got != nil {
		t.Fatalf("List() result = %+v, want nil alongside the error", got)
	}
}

func TestAdapter_MarkRead_NilNotificationsClient_ReturnsDescriptiveError(t *testing.T) {
	a := github.NewAdapter(nil) // nc left nil

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"}
	if err := a.MarkRead(id); err == nil {
		t.Fatal("MarkRead() error = nil, want a descriptive error for a nil notifications client")
	}
}

func TestAdapter_MarkDone_NilNotificationsClient_ReturnsDescriptiveError(t *testing.T) {
	a := github.NewAdapter(nil) // nc left nil

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"}
	if err := a.MarkDone(id); err == nil {
		t.Fatal("MarkDone() error = nil, want a descriptive error for a nil notifications client")
	}
}

func TestAdapter_PollInterval_NilNotificationsClient_ReturnsZero(t *testing.T) {
	a := github.NewAdapter(nil) // nc left nil
	if got := a.PollInterval(); got != 0 {
		t.Errorf("PollInterval() = %v, want 0 for a nil notifications client", got)
	}
}

// ---------------------------------------------------------------------------
// Mapping: List maps wire threads to neutral notifications, resolving
// ScopeDisplay per-row via MultiClient.DisplayNameFor for configured repos and
// falling back to the mapper's own Scope default for unconfigured ones
// (Decision 35).
// ---------------------------------------------------------------------------

const twoThreadsBody = `[
  {"id":"1","unread":true,"reason":"review_requested","updated_at":"2026-07-01T10:00:00Z",
   "subject":{"title":"Fix bug","url":"https://api.github.com/repos/configured/repo/pulls/42","type":"PullRequest"},
   "repository":{"full_name":"configured/repo","html_url":"https://github.com/configured/repo"}},
  {"id":"2","unread":false,"reason":"mention","updated_at":"2026-07-02T10:00:00Z",
   "subject":{"title":"Some issue","url":"https://api.github.com/repos/other/repo/issues/7","type":"Issue"},
   "repository":{"full_name":"other/repo","html_url":"https://github.com/other/repo"}}
]`

func TestAdapter_List_MapsThreadsAndResolvesScopeDisplay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(twoThreadsBody))
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)

	mc, err := github.NewMultiClient([]string{"configured/repo"}, "tok", github.DefaultLabelConvention(),
		map[string]string{"configured/repo": "Configured Repo"})
	if err != nil {
		t.Fatalf("NewMultiClient: %v", err)
	}
	a := github.NewAdapterWithNotifications(mc, nc)

	got, err := a.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}

	configured := got[0]
	if configured.Identity.Kind != provider.KindGitHub {
		t.Errorf("row[0].Identity.Kind = %v, want KindGitHub", configured.Identity.Kind)
	}
	if configured.Identity.Scope != "configured/repo" {
		t.Errorf("row[0].Identity.Scope = %q, want %q", configured.Identity.Scope, "configured/repo")
	}
	if configured.Identity.ScopeDisplay != "Configured Repo" {
		t.Errorf("row[0].Identity.ScopeDisplay = %q, want %q (configured display name)", configured.Identity.ScopeDisplay, "Configured Repo")
	}
	if configured.Identity.ID != "1" {
		t.Errorf("row[0].Identity.ID = %q, want %q", configured.Identity.ID, "1")
	}
	if configured.Reason != provider.NotificationReasonReviewRequested {
		t.Errorf("row[0].Reason = %v, want ReviewRequested", configured.Reason)
	}
	if configured.Read {
		t.Error("row[0].Read = true, want false (wire unread=true)")
	}
	if want := "https://github.com/configured/repo/pull/42"; configured.WebURL != want {
		t.Errorf("row[0].WebURL = %q, want %q", configured.WebURL, want)
	}

	unconfigured := got[1]
	if unconfigured.Identity.Scope != "other/repo" {
		t.Errorf("row[1].Identity.Scope = %q, want %q", unconfigured.Identity.Scope, "other/repo")
	}
	// Decision 35: mc.DisplayNameFor cannot resolve a display name for an
	// unconfigured repo — the mapper's own fallback to Scope covers it.
	if unconfigured.Identity.ScopeDisplay != "other/repo" {
		t.Errorf("row[1].Identity.ScopeDisplay = %q, want %q (fallback to Scope for an unconfigured repo)", unconfigured.Identity.ScopeDisplay, "other/repo")
	}
	if !unconfigured.Read {
		t.Error("row[1].Read = false, want true (wire unread=false)")
	}
}

// TestAdapter_List_NilMultiClient_StillMapsWithoutPanic asserts Decision 39's
// reachable state: a GitHub config whose token never built a per-repo
// MultiClient (mc nil) must still be able to call List through nc alone,
// without panicking, falling back to the mapper's own ScopeDisplay default
// for every row.
func TestAdapter_List_NilMultiClient_StillMapsWithoutPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(twoThreadsBody))
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)

	a := github.NewAdapterWithNotifications(nil, nc)

	got, err := a.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	if got[0].Identity.ScopeDisplay != "configured/repo" {
		t.Errorf("row[0].Identity.ScopeDisplay = %q, want %q (mc nil, mapper's own fallback)", got[0].Identity.ScopeDisplay, "configured/repo")
	}
}

// ---------------------------------------------------------------------------
// Decision 31: NotifOpts.Max is honoured by truncating on return, never by
// stopping nc.List's walk early.
// ---------------------------------------------------------------------------

const fiveThreadsBodyTemplate = `[
  {"id":"1","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t1","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"2","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t2","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"3","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t3","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"4","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t4","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"5","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t5","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}}
]`

// TestAdapter_List_MaxSmallerThanOnePageTruncates is the first of the two
// required Decision 31 tests: a single-page response of 5 threads with
// Max: 2 must come back truncated to 2 rows.
func TestAdapter_List_MaxSmallerThanOnePageTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fiveThreadsBodyTemplate))
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	got, err := a.List(provider.NotifOpts{Max: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List(Max: 2) len = %d, want 2", len(got))
	}
	if got[0].Identity.ID != "1" || got[1].Identity.ID != "2" {
		t.Fatalf("List(Max: 2) ids = [%s %s], want [1 2]", got[0].Identity.ID, got[1].Identity.ID)
	}
}

// TestAdapter_List_LargerMaxLaterServedFromCache_IsNotStuckAtSmallerMax is the
// second, load-bearing Decision 31 test: this is the one that would catch
// Max being pushed down to stop nc.List's walk early. If Max stopped the
// walk, the client would cache only the truncated 2-item set under a
// cachedPath that does not encode Max, and this second call — same request
// shape, larger Max — would 304 against that truncated cache and still come
// back with 2 rows instead of 5.
func TestAdapter_List_LargerMaxLaterServedFromCache_IsNotStuckAtSmallerMax(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fiveThreadsBodyTemplate))
			return
		}
		// Second call: served from nc's cache via 304 — the request shape
		// (buildPath) is identical, since Max never reaches NotificationListOpts.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	small, err := a.List(provider.NotifOpts{Max: 2})
	if err != nil {
		t.Fatalf("first List(Max: 2) error = %v", err)
	}
	if len(small) != 2 {
		t.Fatalf("first List(Max: 2) len = %d, want 2", len(small))
	}

	large, err := a.List(provider.NotifOpts{Max: 5})
	if err != nil {
		t.Fatalf("second List(Max: 5) error = %v", err)
	}
	if call != 2 {
		t.Fatalf("server was called %d times, want exactly 2 (second call must hit the 304 path, proving it was served from cache)", call)
	}
	if len(large) != 5 {
		t.Fatalf("second List(Max: 5) len = %d, want 5 — a smaller Max must not have truncated what nc.List cached (Decision 31)", len(large))
	}
}

// ---------------------------------------------------------------------------
// Decision 28: an unsolicited 304 (no matching cache) surfaces as an error
// from nc.List — Adapter.List must propagate it, never translate it into an
// empty slice.
// ---------------------------------------------------------------------------

func TestAdapter_List_PropagatesUnsolicited304Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every call returns 304, including the very first — nothing has ever
		// been cached, so this is unsolicited.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	got, err := a.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want an error for an unsolicited 304 (Decision 28) — an emptied feed reads as \"you're clear\"")
	}
	if got != nil {
		t.Fatalf("List() result = %+v, want nil alongside the error, not an empty-but-non-nil slice", got)
	}
}

// ---------------------------------------------------------------------------
// MarkRead / MarkDone: forward id.ID straight through; reject a mismatched
// Kind as a caller bug (Decision 14).
// ---------------------------------------------------------------------------

func TestAdapter_MarkRead_ForwardsID(t *testing.T) {
	var capturedMethod, capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusResetContent)
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "42"}
	if err := a.MarkRead(id); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if capturedMethod != http.MethodPatch {
		t.Errorf("method = %q, want %q", capturedMethod, http.MethodPatch)
	}
	if capturedPath != "/notifications/threads/42" {
		t.Errorf("path = %q, want %q", capturedPath, "/notifications/threads/42")
	}
}

func TestAdapter_MarkDone_ForwardsID(t *testing.T) {
	var capturedMethod, capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "42"}
	if err := a.MarkDone(id); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}
	if capturedMethod != http.MethodDelete {
		t.Errorf("method = %q, want %q", capturedMethod, http.MethodDelete)
	}
	if capturedPath != "/notifications/threads/42" {
		t.Errorf("path = %q, want %q", capturedPath, "/notifications/threads/42")
	}
}

func TestAdapter_MarkRead_MismatchedKind_ReturnsErrorAndIssuesNoRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusResetContent)
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	// An Azure identity handed to the GitHub adapter — a caller bug Decision
	// 14 exists to catch, not a wrong-backend request to silently route.
	id := provider.Identity{Kind: provider.KindAzure, Scope: "o/r", ID: "42"}
	if err := a.MarkRead(id); err == nil {
		t.Fatal("MarkRead() error = nil, want an error for a mismatched Identity.Kind")
	}
	if requests != 0 {
		t.Errorf("server received %d requests, want 0 — a mismatched Kind must be rejected before any network call", requests)
	}
}

func TestAdapter_MarkDone_MismatchedKind_ReturnsErrorAndIssuesNoRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	id := provider.Identity{Kind: provider.KindAzure, Scope: "o/r", ID: "42"}
	if err := a.MarkDone(id); err == nil {
		t.Fatal("MarkDone() error = nil, want an error for a mismatched Identity.Kind")
	}
	if requests != 0 {
		t.Errorf("server received %d requests, want 0 — a mismatched Kind must be rejected before any network call", requests)
	}
}

// ---------------------------------------------------------------------------
// Decision 37 at the adapter boundary: MarkRead must not block behind an
// in-flight List. Adapter.MarkRead/MarkDone forward straight through with no
// lock of their own, so this is structurally guaranteed as long as Adapter
// adds no mutex to this path — this test guards a regression at the adapter
// layer specifically, mirroring
// TestNotificationsClient_MarkRead_DoesNotBlockOnInFlightList at the client
// layer.
// ---------------------------------------------------------------------------

func TestAdapter_MarkRead_DoesNotBlockOnInFlightList(t *testing.T) {
	release := make(chan struct{})
	listStarted := make(chan struct{})

	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			close(listStarted)
			<-release
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`))
		case http.MethodPatch:
			w.WriteHeader(http.StatusResetContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	defer releaseHandler() // LIFO: runs before srv.Close() on every exit path

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	listDone := make(chan error, 1)
	go func() {
		_, err := a.List(provider.NotifOpts{})
		listDone <- err
	}()

	select {
	case <-listStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("List()'s handler was never reached")
	}

	markDone := make(chan error, 1)
	go func() {
		id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "42"}
		markDone <- a.MarkRead(id)
	}()

	select {
	case err := <-markDone:
		if err != nil {
			t.Fatalf("MarkRead() error = %v, want nil", err)
		}
	case err := <-listDone:
		t.Fatalf("List() returned before MarkRead even though its handler is still parked on release: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("MarkRead() did not complete while a List() call was in flight — structural evidence Adapter reinstated a shared lock (Decision 37)")
	}

	releaseHandler()
	if err := <-listDone; err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Decision 30: PollInterval forwards nc's cadence hint.
// ---------------------------------------------------------------------------

func TestAdapter_PollInterval_ForwardsClientValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Poll-Interval", "120")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	if _, err := a.List(provider.NotifOpts{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if want := 120 * time.Second; a.PollInterval() != want {
		t.Errorf("PollInterval() = %v, want %v", a.PollInterval(), want)
	}
}
