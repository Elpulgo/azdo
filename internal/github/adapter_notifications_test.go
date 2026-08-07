package github_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// This compile-time assertion belongs in the default build, not behind a
// //go:build adapter tag that nothing runs. Follows provider/composite_test.go:13
// and provider/notifications_test.go:25 for the untagged var _ pattern.
// ---------------------------------------------------------------------------

var _ provider.NotificationSource = (*github.Adapter)(nil)

// ---------------------------------------------------------------------------
// A local PollIntervalHinter, asserted against *github.Adapter. The real
// interface in internal/provider asserts again independently; Go's structural
// typing keeps the two separate, so if Adapter loses PollInterval() this
// assertion breaks immediately.
// ---------------------------------------------------------------------------

type pollIntervalHinter interface {
	PollInterval() time.Duration
}

var _ pollIntervalHinter = (*github.Adapter)(nil)

// ---------------------------------------------------------------------------
// The real interface now exists in internal/provider, colocated with
// NotificationSource. This assertion is additive, not a replacement for the
// local one above — the two are structurally independent, so either one
// breaking on its own pins a real regression.
// ---------------------------------------------------------------------------

var _ provider.PollIntervalHinter = (*github.Adapter)(nil)

// ---------------------------------------------------------------------------
// nil nc — List/MarkRead/MarkDone must error, never panic.
//
// "Descriptive" is only enforced if the message itself is asserted: an
// err != nil check passes just as happily on errors.New("x"), and this text is
// rendered in-view, so each of the three messages is pinned below.
// ---------------------------------------------------------------------------

const wantNoClientMsg = "no notifications client configured"

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
	if want := "github: notifications: " + wantNoClientMsg; err.Error() != want {
		t.Errorf("List() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkRead_NilNotificationsClient_ReturnsDescriptiveError(t *testing.T) {
	a := github.NewAdapter(nil) // nc left nil

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"}
	err := a.MarkRead(id)
	if err == nil {
		t.Fatal("MarkRead() error = nil, want a descriptive error for a nil notifications client")
	}
	if want := "github: mark read: " + wantNoClientMsg; err.Error() != want {
		t.Errorf("MarkRead() error = %q, want %q", err.Error(), want)
	}
}

func TestAdapter_MarkDone_NilNotificationsClient_ReturnsDescriptiveError(t *testing.T) {
	a := github.NewAdapter(nil) // nc left nil

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"}
	err := a.MarkDone(id)
	if err == nil {
		t.Fatal("MarkDone() error = nil, want a descriptive error for a nil notifications client")
	}
	if want := "github: mark done: " + wantNoClientMsg; err.Error() != want {
		t.Errorf("MarkDone() error = %q, want %q", err.Error(), want)
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
// falling back to the mapper's own Scope default for unconfigured ones.
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
	// mc.DisplayNameFor cannot resolve a display name for an unconfigured repo —
	// the mapper's own fallback to Scope covers it.
	if unconfigured.Identity.ScopeDisplay != "other/repo" {
		t.Errorf("row[1].Identity.ScopeDisplay = %q, want %q (fallback to Scope for an unconfigured repo)", unconfigured.Identity.ScopeDisplay, "other/repo")
	}
	if !unconfigured.Read {
		t.Error("row[1].Read = false, want true (wire unread=false)")
	}
}

// TestAdapter_List_NilMultiClient_StillMapsWithoutPanic asserts a reachable
// state: a GitHub config whose token never built a per-repo
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

// TestAdapter_List_ForwardsParticipatingOnlyAndSince pins the adapter→client
// wiring, which is the only route from config to the server query.
// TestNotificationsClient_List_ParticipatingAndSince pins buildPath, but it
// constructs NotificationListOpts itself, so it stays green if Adapter.List
// stops populating either field — and both drops fail open and silently:
// dropping Participating disables the server-side narrowing (the
// whole inbox gets fetched), dropping Since turns since_days into a no-op
// pulling unbounded history. Neither shows up as an error anywhere.
//
// The since value is a fixed literal (never time.Now()) so the expected query
// string is a constant.
func TestAdapter_List_ForwardsParticipatingOnlyAndSince(t *testing.T) {
	var capturedParticipating, capturedSince string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedParticipating = r.URL.Query().Get("participating")
		capturedSince = r.URL.Query().Get("since")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	since := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if _, err := a.List(provider.NotifOpts{ParticipatingOnly: true, Since: since}); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if capturedParticipating != "true" {
		t.Errorf("participating query param = %q, want %q — NotifOpts.ParticipatingOnly must reach NotificationListOpts.Participating", capturedParticipating, "true")
	}
	if want := "2026-07-01T12:00:00Z"; capturedSince != want {
		t.Errorf("since query param = %q, want %q — NotifOpts.Since must reach NotificationListOpts.Since", capturedSince, want)
	}
}

// ---------------------------------------------------------------------------
// NotifOpts.Max — since decision 15 / task 12 of the phase-2 notifications
// spec, Max is honoured exclusively by provider.CompositeProvider.List, after
// it merges every capable backend's rows and sorts them newest-first.
// Adapter.List no longer truncates on it at all (phase 1's original
// per-adapter truncation, which these tests used to pin, is removed — see
// Adapter.List's own doc comment for why leaving it in place would
// double-apply the cap once a second capable backend exists).
// ---------------------------------------------------------------------------

const fiveThreadsBodyTemplate = `[
  {"id":"1","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t1","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"2","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t2","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"3","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t3","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"4","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t4","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}},
  {"id":"5","unread":true,"reason":"subscribed","updated_at":"2026-07-01T10:00:00Z","subject":{"title":"t5","url":"","type":"Discussion"},"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"}}
]`

// TestAdapter_List_MaxDoesNotTruncateAtThisLayer pins the new contract: a
// single-page response of 5 threads comes back as all 5 regardless of Max,
// including a Max smaller than the result — the exact case that used to
// truncate before task 12 moved the cap to CompositeProvider.List.
func TestAdapter_List_MaxDoesNotTruncateAtThisLayer(t *testing.T) {
	tests := []struct {
		name string
		max  int
	}{
		{name: "Max smaller than result", max: 2},
		{name: "negative Max", max: -1},
		{name: "zero Max", max: 0},
		{name: "Max equal to result length", max: 5},
		{name: "Max larger than result length", max: 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fiveThreadsBodyTemplate))
			}))
			defer srv.Close()

			nc := github.NewNotificationsClient("tok")
			nc.SetBaseURL(srv.URL)
			a := github.NewAdapterWithNotifications(nil, nc)

			got, err := a.List(provider.NotifOpts{Max: tt.max})
			if err != nil {
				t.Fatalf("List(Max: %d) error = %v", tt.max, err)
			}
			if len(got) != 5 {
				t.Fatalf("List(Max: %d) len = %d, want 5 — Adapter.List must not truncate on Max", tt.max, len(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// An unsolicited 304 (no matching cache) surfaces as an error from nc.List —
// Adapter.List must propagate it, never translate it into an empty slice.
//
// Both tests below also pin the *shape* of the propagation, not just that an
// error came back: Adapter.List returns nc.List's error verbatim, so errors.As
// still recovers the underlying *APIError with its StatusCode and its 403 scope
// headers intact. A later well-meaning wrap here —
// fmt.Errorf("github: notifications: %w", err), which double-prefixes the
// user-visible message, or %v, which breaks errors.As outright — would
// otherwise leave every adapter test green. Mirrors
// TestNotificationsClient_List_304WithNoCache_ReturnsError and
// TestNotificationsClient_List_403_MissingScope_RecoversScopeHeaders at the
// client layer.
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
		t.Fatal("List() error = nil, want an error for an unsolicited 304 — an emptied feed reads as \"you're clear\"")
	}
	if got != nil {
		t.Fatalf("List() result = %+v, want nil alongside the error, not an empty-but-non-nil slice", got)
	}

	var apiErr *github.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *github.APIError from %v — Adapter.List must return nc.List's error unchanged so the caller can branch on the 304", err)
	}
	if apiErr.StatusCode != http.StatusNotModified {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotModified)
	}

	// No extra prefix from the adapter. Asserted against the client's own error
	// for the same response rather than a hardcoded string, so this pins "the
	// adapter adds nothing" without also pinning nc.List's wording (which
	// deliberately wraps with %w around an already-"github:"-prefixed *APIError).
	direct := github.NewNotificationsClient("tok")
	direct.SetBaseURL(srv.URL)
	_, clientErr := direct.List(github.NotificationListOpts{})
	if clientErr == nil {
		t.Fatal("nc.List() error = nil for an unsolicited 304, want an error")
	}
	if err.Error() != clientErr.Error() {
		t.Errorf("Adapter.List() error = %q, want nc.List()'s verbatim %q — an added prefix double-prefixes the user-visible message", err.Error(), clientErr.Error())
	}
}

func TestAdapter_List_403_MissingScope_RecoversScopeHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accepted-OAuth-Scopes", "notifications")
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer srv.Close()

	nc := github.NewNotificationsClient("tok")
	nc.SetBaseURL(srv.URL)
	a := github.NewAdapterWithNotifications(nil, nc)

	got, err := a.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want an error for a missing-scope 403")
	}
	if got != nil {
		t.Errorf("List() result = %+v, want nil alongside the error", got)
	}

	var apiErr *github.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *github.APIError from %v", err)
	}
	if apiErr.RequiredScopes != "notifications" {
		t.Errorf("RequiredScopes = %q, want %q — the missing scope is named in this field", apiErr.RequiredScopes, "notifications")
	}
	if apiErr.GrantedScopes != "repo, read:org" {
		t.Errorf("GrantedScopes = %q, want %q", apiErr.GrantedScopes, "repo, read:org")
	}
	// This path returns the bare *APIError, whose Error() already starts with
	// "github:" — so exactly one occurrence, no adapter-added second prefix.
	if got := strings.Count(err.Error(), "github:"); got != 1 {
		t.Errorf("error message %q contains %d occurrences of %q, want 1 (double-prefixed)", err.Error(), got, "github:")
	}
}

// ---------------------------------------------------------------------------
// MarkRead / MarkDone: forward id.ID straight through; reject a mismatched
// Kind as a caller bug.
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

	// An Azure identity handed to the GitHub adapter — a caller bug to catch,
	// not a wrong-backend request to silently route.
	id := provider.Identity{Kind: provider.KindAzure, Scope: "o/r", ID: "42"}
	err := a.MarkRead(id)
	if err == nil {
		t.Fatal("MarkRead() error = nil, want an error for a mismatched Identity.Kind")
	}
	if want := `github: mark read: identity kind "azure" is not "github"`; err.Error() != want {
		t.Errorf("MarkRead() error = %q, want %q", err.Error(), want)
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
	err := a.MarkDone(id)
	if err == nil {
		t.Fatal("MarkDone() error = nil, want an error for a mismatched Identity.Kind")
	}
	if want := `github: mark done: identity kind "azure" is not "github"`; err.Error() != want {
		t.Errorf("MarkDone() error = %q, want %q", err.Error(), want)
	}
	if requests != 0 {
		t.Errorf("server received %d requests, want 0 — a mismatched Kind must be rejected before any network call", requests)
	}
}

// TestAdapter_Mark_ZeroKind_ErrorNamesTheEmptyKind covers the zero Identity.Kind
// explicitly. Rejecting it is correct — the composite routes by
// Identity.Kind, so only KindGitHub can legitimately arrive — but the message
// had a hole in it: Kind.String() returns "" for the zero value and %v uses the
// Stringer, so the rendered text was "identity kind  is not github", two spaces
// where the kind belongs. %q makes the empty kind visible as "" instead.
func TestAdapter_Mark_ZeroKind_ErrorNamesTheEmptyKind(t *testing.T) {
	tests := []struct {
		name string
		call func(*github.Adapter, provider.Identity) error
		want string
	}{
		{
			name: "MarkRead",
			call: func(a *github.Adapter, id provider.Identity) error { return a.MarkRead(id) },
			want: `github: mark read: identity kind "" is not "github"`,
		},
		{
			name: "MarkDone",
			call: func(a *github.Adapter, id provider.Identity) error { return a.MarkDone(id) },
			want: `github: mark done: identity kind "" is not "github"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusResetContent)
			}))
			defer srv.Close()

			nc := github.NewNotificationsClient("tok")
			nc.SetBaseURL(srv.URL)
			a := github.NewAdapterWithNotifications(nil, nc)

			// Kind deliberately left at its zero value.
			err := tt.call(a, provider.Identity{ID: "42"})
			if err == nil {
				t.Fatalf("%s() error = nil, want an error for a zero Identity.Kind", tt.name)
			}
			if err.Error() != tt.want {
				t.Errorf("%s() error = %q, want %q", tt.name, err.Error(), tt.want)
			}
			if strings.Contains(err.Error(), "kind  is") {
				t.Errorf("%s() error = %q — the zero Kind rendered as a blank hole; format it with %%q, not %%v", tt.name, err.Error())
			}
			if requests != 0 {
				t.Errorf("server received %d requests, want 0 — a zero Kind must be rejected before any network call", requests)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MarkRead must not block behind an in-flight List. -race is unavailable in
// this environment, so this is asserted structurally rather than by timing:
// List's HTTP request is parked in the handler on a channel, and MarkRead
// (hitting the same server, a different method/path) is proven to complete
// before the test releases List's handler. If anything on the mark path took
// the fetch mutex it would deadlock behind List until the release, which the
// select below with a bounded timeout catches.
//
// This is the sole test for that contract at BOTH layers. Adapter.MarkRead/
// MarkDone forward straight through with no lock of their own, so driving the
// adapter guards the adapter-layer regression (reinstating a shared lock)
// AND, because the calls land in NotificationsClient.List/MarkRead unchanged,
// the client-layer one (markThread reaching for c.mu — verified: adding
// c.mu.Lock()/defer c.mu.Unlock() to markThread fails this test at the 2s
// timeout). A client-level duplicate of this choreography in
// notifications_test.go killed a strict subset and was removed.
//
// Releasing the parked handler must be guaranteed on every exit path, not just
// the happy one. Every branch of the select below can t.Fatal, which
// runtime.Goexits into the deferred srv.Close() — and srv.Close blocks until
// in-flight handlers return, so a GET handler still parked on <-release turned
// this test's own failure into a package-wide test timeout and goroutine dump
// instead of the one-line failure it is worded to produce (CI's default timeout
// is 10 minutes).
// The release is therefore wrapped in a sync.Once and deferred BEFORE
// srv.Close() is deferred: defers run LIFO, so the handler is always freed
// first and srv.Close() never blocks. Deliberately not t.Cleanup — cleanups run
// only after the test function has fully unwound, which is too late to unblock
// a deferred srv.Close() inside it.
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
		t.Fatal("MarkRead() did not complete while a List() call was in flight — structural evidence Adapter reinstated a shared lock")
	}

	releaseHandler()
	if err := <-listDone; err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// PollInterval forwards nc's cadence hint.
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
