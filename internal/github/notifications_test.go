package github

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Decision 12: all=true always present
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_RequestsAllTrue(t *testing.T) {
	var capturedAll string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAll = r.URL.Query().Get("all")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if capturedAll != "true" {
		t.Errorf("all query param = %q, want %q", capturedAll, "true")
	}
}

// ---------------------------------------------------------------------------
// Review feedback item 7: nothing pinned the method or path — mutating the
// endpoint to "/notificationz" left the suite green.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_RequestsCorrectMethodPathAndPerPage(t *testing.T) {
	var capturedMethod, capturedPath, capturedPerPage string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedPerPage = r.URL.Query().Get("per_page")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if capturedMethod != http.MethodGet {
		t.Errorf("method = %q, want %q", capturedMethod, http.MethodGet)
	}
	if capturedPath != "/notifications" {
		t.Errorf("path = %q, want %q", capturedPath, "/notifications")
	}
	if capturedPerPage != "100" {
		t.Errorf("per_page = %q, want %q", capturedPerPage, "100")
	}
}

// ---------------------------------------------------------------------------
// Decision 27: List always hands out a copy, on the 304 path too — not just
// the 200 path. The previous version of this test only ever mutated a result
// that traced back to a 200 response (both here and in
// TestNotificationsClient_List_304_ReturnsCachedSliceUnchanged), so reverting
// *only* the 304 path's cloneThreads(c.cached) call back to a bare
// `return c.cached, nil` left the whole suite green — nothing exercised
// mutating a result that itself came from a 304. This three-call version
// closes that gap: 200 (caches), then 304 (mutate *that* result in place),
// then 304 again (assert the mutation left no trace) — which is exactly the
// corruption fold-in 2 measured: a caller in-place-filtering [1 2 3] down to
// [2] left the cache [2 2 3].
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_200_MutatingResultDoesNotAffectLaterCall(t *testing.T) {
	call := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","unread":true,"reason":"subscribed"},{"id":"2","unread":true,"reason":"mention"},{"id":"3","unread":true,"reason":"author"}]`))
			return
		}
		// Every call after the first hits the 304 cache-read path.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() (200) error = %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("first List() len = %d, want 3", len(first))
	}

	second, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("second List() (304) error = %v", err)
	}
	if len(second) != 3 {
		t.Fatalf("second List() len = %d, want 3", len(second))
	}

	// The standard in-place filter idiom, applied to the SECOND call's
	// (304) result — not the first. If the 304 path handed back an alias of
	// c.cached rather than a copy, this filter corrupts c.cached directly,
	// and the third call (also 304, from the same cache) would observe it.
	out := second[:0]
	for _, row := range second {
		if row.ID == "2" {
			out = append(out, row)
		}
	}
	if len(out) != 1 || out[0].ID != "2" {
		t.Fatalf("filter setup failed: out = %+v", out)
	}

	third, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("third List() (304) error = %v", err)
	}
	if len(third) != 3 {
		t.Fatalf("third List() len = %d, want 3 — mutating the second call's (304) result must not have rewritten the cache", len(third))
	}
	wantIDs := []string{"1", "2", "3"}
	for i, id := range wantIDs {
		if third[i].ID != id {
			t.Errorf("third[%d].ID = %q, want %q", i, third[i].ID, id)
		}
	}
}

// ---------------------------------------------------------------------------
// Link rel=next pagination, with multiple rels present
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_FollowsLinkHeader_MultipleRels(t *testing.T) {
	var srv *httptest.Server
	var requestedPages []string

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		requestedPages = append(requestedPages, page)

		switch page {
		case "1":
			// Multiple rels present so a naive substring match on `rel="next"`
			// would not be sufficient — the parser must pick the right one.
			w.Header().Set("Link",
				`<`+srv.URL+`/notifications?page=2>; rel="next", `+
					`<`+srv.URL+`/notifications?page=2>; rel="last", `+
					`<`+srv.URL+`/notifications?page=1>; rel="prev"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","reason":"subscribed"},{"id":"2","reason":"mention"}]`))
		default:
			// Last page: no "next" relation, only "prev" and "first".
			w.Header().Set("Link",
				`<`+srv.URL+`/notifications?page=1>; rel="prev", `+
					`<`+srv.URL+`/notifications?page=1>; rel="first"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"3","reason":"author"}]`))
		}
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	got, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	wantIDs := []string{"1", "2", "3"}
	if len(got) != len(wantIDs) {
		t.Fatalf("len(got) = %d, want %d (%+v)", len(got), len(wantIDs), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
	if len(requestedPages) != 2 {
		t.Errorf("requested %d pages, want 2: %v", len(requestedPages), requestedPages)
	}
}

// ---------------------------------------------------------------------------
// Review feedback item 5 / Decision 29: a self-referential Link rel="next"
// must not loop forever. Measured on the unfixed code: 501 requests, 500
// rows, err == nil.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_CyclicNextLink_ReturnsError(t *testing.T) {
	var srv *httptest.Server
	requests := 0

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		// Always points at itself: a genuinely cyclic Link header.
		w.Header().Set("Link", `<`+srv.URL+`/notifications?page=cycle>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	_, err := c.List(NotificationListOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want an error for a cyclic Link rel=\"next\"")
	}
	// Asserted against the literal 50, not maxNotificationPages: reading the
	// constant under test here would let a mutation that widens the constant
	// (e.g. to 500) pass silently, since "requests" would widen right along
	// with it. 50 = 1 initial fetch + 49 more before the loop's pages > 50
	// check trips (see the boundary arithmetic at the top of the pagination
	// loop in List) — an independently-computed value, not derived from
	// maxNotificationPages's current value.
	if requests != 50 {
		t.Errorf("requests = %d, want exactly 50", requests)
	}
}

// ---------------------------------------------------------------------------
// If-Modified-Since: absent on first call, present once a Last-Modified value
// has been cached from a prior 200 response.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_IfModifiedSince_OnlyAfterCache(t *testing.T) {
	const lastModified = "Wed, 21 Oct 2015 07:28:00 GMT"
	var capturedIfModifiedSince []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIfModifiedSince = append(capturedIfModifiedSince, r.Header.Get("If-Modified-Since"))
		w.Header().Set("Last-Modified", lastModified)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("second List() error = %v", err)
	}

	if len(capturedIfModifiedSince) != 2 {
		t.Fatalf("got %d requests, want 2", len(capturedIfModifiedSince))
	}
	if capturedIfModifiedSince[0] != "" {
		t.Errorf("first call If-Modified-Since = %q, want empty (no cached timestamp yet)", capturedIfModifiedSince[0])
	}
	if capturedIfModifiedSince[1] != lastModified {
		t.Errorf("second call If-Modified-Since = %q, want %q", capturedIfModifiedSince[1], lastModified)
	}
}

// ---------------------------------------------------------------------------
// Round-3 review item 9: List assigns c.lastModified unconditionally from
// the response header (List's final lines), not only when the header is
// non-empty as an earlier revision did. A 200 that omits Last-Modified
// therefore clears any previously cached validator, even though it still
// sets cached/cachedPath — making the NEXT call's request unconditional
// again. This is the correct trade (a validator must pair with the cache it
// belongs to; keeping a stale one would offer it as if it still described
// the current cache), but nothing pinned it before this test.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_200WithoutLastModified_ClearsValidator(t *testing.T) {
	const lastModified1 = "Wed, 21 Oct 2015 07:28:00 GMT"
	call := 0
	var capturedIfModifiedSince []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		capturedIfModifiedSince = append(capturedIfModifiedSince, r.Header.Get("If-Modified-Since"))
		if call == 1 {
			w.Header().Set("Last-Modified", lastModified1)
		}
		// Calls 2 and 3 send no Last-Modified header at all — an unusual but
		// real shape GitHub's API can return.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("third List() error = %v", err)
	}

	if len(capturedIfModifiedSince) != 3 {
		t.Fatalf("got %d requests, want 3", len(capturedIfModifiedSince))
	}
	if capturedIfModifiedSince[0] != "" {
		t.Errorf("first call If-Modified-Since = %q, want empty (nothing cached yet)", capturedIfModifiedSince[0])
	}
	if capturedIfModifiedSince[1] != lastModified1 {
		t.Errorf("second call If-Modified-Since = %q, want %q (cached from the first response)", capturedIfModifiedSince[1], lastModified1)
	}
	if capturedIfModifiedSince[2] != "" {
		t.Errorf("third call If-Modified-Since = %q, want empty — the second response's 200 with no Last-Modified header must clear the cached validator", capturedIfModifiedSince[2])
	}
}

// ---------------------------------------------------------------------------
// 304 Not Modified: cached slice returned unchanged, never cleared.
//
// Review feedback item 2: the previous version of this test compared
// second[i] against first[i] — two aliases of one backing array — which is a
// content-tautology that stays green even if the 304 path clobbers every
// cached row's fields, because both "first" and "second" would read the
// clobbered value back. Pinned against independently-declared expected
// values, and re-checked after mutating the first result, so the copy
// (Decision 27) is actually exercised.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_304_ReturnsCachedSliceUnchanged(t *testing.T) {
	call := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","unread":true,"reason":"subscribed"},{"id":"2","unread":false,"reason":"mention"}]`))
			return
		}
		// Second call: inbox unchanged, empty body per GitHub's documented
		// 304 contract.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	// Independently-declared expected values — never derived from the
	// production code's own output, so a mutation that corrupts the cache on
	// the 304 path cannot smuggle itself past this assertion.
	wantID := []string{"1", "2"}
	wantUnread := []bool{true, false}
	wantReason := []string{"subscribed", "mention"}

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first List() len = %d, want 2", len(first))
	}
	for i := range first {
		if first[i].ID != wantID[i] || first[i].Unread != wantUnread[i] || first[i].Reason != wantReason[i] {
			t.Fatalf("first[%d] = %+v, want id=%q unread=%v reason=%q", i, first[i], wantID[i], wantUnread[i], wantReason[i])
		}
	}

	// Mutate the first result in place before asking for the second. If the
	// 304 path returned an alias of the cache (rather than a copy), this
	// would corrupt what the next call hands back — the whole point of
	// Decision 27's copy requirement.
	first[0].Unread = false
	first[0].Reason = "CLOBBERED-BY-TEST"

	second, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("second List() len = %d, want 2 (unchanged)", len(second))
	}
	for i := range second {
		if second[i].ID != wantID[i] || second[i].Unread != wantUnread[i] || second[i].Reason != wantReason[i] {
			t.Errorf("second[%d] = %+v, want id=%q unread=%v reason=%q (must survive the first result's mutation)", i, second[i], wantID[i], wantUnread[i], wantReason[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Review feedback item 3 / Decision 27 fold-in: the cached validator must be
// scoped to the request shape that produced it. List({}) then
// List({Participating: true}) must not condition the second query's request
// on the first query's validator — otherwise a 304 for the participating-only
// query would hand back the whole-inbox cache as if it were the complete
// participating-only answer (a truncated feed presented as complete).
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_DifferentRequestShape_NeverSendsMismatchedValidator(t *testing.T) {
	var capturedIfModifiedSince []string
	var capturedParticipating []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIfModifiedSince = append(capturedIfModifiedSince, r.Header.Get("If-Modified-Since"))
		capturedParticipating = append(capturedParticipating, r.URL.Query().Get("participating"))
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if _, err := c.List(NotificationListOpts{Participating: true}); err != nil {
		t.Fatalf("second List() (different shape) error = %v", err)
	}

	if len(capturedIfModifiedSince) != 2 {
		t.Fatalf("got %d requests, want 2", len(capturedIfModifiedSince))
	}
	if capturedIfModifiedSince[0] != "" {
		t.Errorf("first call If-Modified-Since = %q, want empty", capturedIfModifiedSince[0])
	}
	if capturedIfModifiedSince[1] != "" {
		t.Errorf("second call (different request shape) If-Modified-Since = %q, want empty — the cache from a different query shape must not be offered as a validator", capturedIfModifiedSince[1])
	}
	if capturedParticipating[0] != "" {
		t.Errorf("first call participating = %q, want empty", capturedParticipating[0])
	}
	if capturedParticipating[1] != "true" {
		t.Errorf("second call participating = %q, want %q", capturedParticipating[1], "true")
	}
}

// ---------------------------------------------------------------------------
// Round-3 review item 4: cachedPath is buildPath(opts), which has no host
// component. Without SetBaseURL invalidating the cache, switching base URLs
// (e.g. github.com -> a GitHub Enterprise instance, or any demo-mode
// redirect) with the same NotificationListOpts would still match cachedPath,
// so a validator captured against one host would be offered to another —
// and a 304 would hand back the FIRST host's cached rows presented as the
// second host's answer. srv2 below deliberately synthesises exactly that
// 304-if-conditioned response so the test fails loudly (wrong rows) rather
// than subtly (an extra unwanted header) if the invalidation regresses.
// ---------------------------------------------------------------------------

func TestNotificationsClient_SetBaseURL_InvalidatesCache(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") != "" {
			// This is the failure mode under test: if srv1's cache were still
			// considered valid for srv2, an unrelated server conditioned on
			// srv1's validator would (plausibly) still say "not modified",
			// and List would hand back srv1's rows as srv2's answer.
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"99","reason":"mention"}]`))
	}))
	defer srv2.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv1.URL)

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() (srv1) error = %v", err)
	}
	if len(first) != 1 || first[0].ID != "1" {
		t.Fatalf("first List() = %+v, want single row id=1", first)
	}

	// Switching base URLs must invalidate the cache and validator.
	c.SetBaseURL(srv2.URL)

	second, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("second List() (srv2, after SetBaseURL) error = %v", err)
	}
	if len(second) != 1 || second[0].ID != "99" {
		t.Fatalf("second List() = %+v, want single row id=99 from the new base URL — got the previous base URL's cached rows instead", second)
	}
}

// ---------------------------------------------------------------------------
// Review feedback item 4 / Decision 28: a 304 with nothing cached (or nothing
// cached for this request shape) is an error, never an empty feed.
//
// Round-3 review item 6: the returned error must wrap the underlying
// *APIError (StatusCode == 304) with %w, not just format it into a plain
// string — otherwise a caller (tasks 13, 19) needing to branch on "was this
// an unsolicited 304" versus "a transport failure" has no route but string
// matching. Asserted here via errors.As.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_304WithNoCache_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A caching proxy/MITM appliance synthesising a 304 nobody asked for
		// (Decision 28's rationale) — the first-ever call has no cached
		// validator to condition on, so this is unsolicited.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	got, err := c.List(NotificationListOpts{})
	if err == nil {
		t.Fatalf("List() error = nil, got = %+v, want an error for a 304 with nothing cached", got)
	}
	if got != nil {
		t.Errorf("List() returned %+v on error, want nil", got)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *APIError from %v — the 304 must be wrapped with %%w, not just formatted into the message", err)
	}
	if apiErr.StatusCode != http.StatusNotModified {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotModified)
	}
}

// ---------------------------------------------------------------------------
// X-Poll-Interval parsing, including malformed/absent-header fallback.
// ---------------------------------------------------------------------------

func TestNotificationsClient_PollInterval_ParsesHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Poll-Interval", "30")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := c.PollInterval(), 30*time.Second; got != want {
		t.Errorf("PollInterval() = %v, want %v", got, want)
	}
}

func TestNotificationsClient_PollInterval_MalformedHeaderFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Poll-Interval", "not-a-number")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := c.PollInterval(), defaultPollInterval; got != want {
		t.Errorf("PollInterval() = %v, want fallback %v", got, want)
	}
}

func TestNotificationsClient_PollInterval_NegativeHeaderFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Poll-Interval", "-5")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := c.PollInterval(), defaultPollInterval; got != want {
		t.Errorf("PollInterval() = %v, want fallback %v", got, want)
	}
}

func TestNotificationsClient_PollInterval_AbsentHeaderFallsBackBeforeAnyCall(t *testing.T) {
	c := NewNotificationsClient("tok")
	if got, want := c.PollInterval(), defaultPollInterval; got != want {
		t.Errorf("PollInterval() before any call = %v, want fallback %v", got, want)
	}
}

func TestNotificationsClient_PollInterval_AbsentHeaderKeepsPreviousValue(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			w.Header().Set("X-Poll-Interval", "45")
		}
		// Second call: header absent entirely.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if got, want := c.PollInterval(), 45*time.Second; got != want {
		t.Errorf("PollInterval() after first call = %v, want %v", got, want)
	}

	if _, err := c.List(NotificationListOpts{}); err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if got, want := c.PollInterval(), 45*time.Second; got != want {
		t.Errorf("PollInterval() after second call (absent header) = %v, want unchanged %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Review feedback nit 8: fmt.Errorf("github: list notifications: %w", ...)
// double-prefixed, since APIError.Error() already starts with "github:" —
// user-visible in the error pane. List now returns the *APIError unwrapped.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_ErrorIsNotDoublePrefixed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	_, err := c.List(NotificationListOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want an error for a 500 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *APIError from %v", err)
	}
	if got := strings.Count(err.Error(), "github:"); got != 1 {
		t.Errorf("error message %q contains %d occurrences of %q, want 1 (double-prefixed)", err.Error(), got, "github:")
	}
}

// ---------------------------------------------------------------------------
// Round-3 review item 5: getPage (List's HTTP call site) is a SECOND,
// independent newAPIError call site from Client.get — TestClient_Get_403_
// MissingScope_RecoversScopeHeaders in client_test.go only exercises the
// other one. Changing getPage's newAPIError call to pass an empty
// http.Header{} instead of resp.Header leaves that suite green (Required/
// GrantedScopes come back empty and nothing at the List level notices).
// Pinned here at the List level, via errors.As, the same way task 19 will
// need to detect it.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_403_MissingScope_RecoversScopeHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accepted-OAuth-Scopes", "notifications")
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	_, err := c.List(NotificationListOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want an error for a missing-scope 403")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *APIError from %v", err)
	}
	if apiErr.RequiredScopes != "notifications" {
		t.Errorf("RequiredScopes = %q, want %q", apiErr.RequiredScopes, "notifications")
	}
	if apiErr.GrantedScopes != "repo, read:org" {
		t.Errorf("GrantedScopes = %q, want %q", apiErr.GrantedScopes, "repo, read:org")
	}
}

// ---------------------------------------------------------------------------
// Review feedback nit 9: a 304 on page >= 2 fails the whole List rather than
// being special-cased — documented here since the pagination loop reads as
// though 304 is handled everywhere.
//
// Round-3 review item 10: the comment above the mid-walk fetch in List
// claims "the cache from the prior successful fetch is left untouched", but
// nothing asserted it — this test only ever checked that the failing call
// itself returned an error. Extended to a three-call sequence: a first
// successful List() populates the cache and validator; a second List() fails
// mid-walk on an unsolicited page-2 304 (after a fresh, different page-1 200
// — so there's something-other-than-the-original-cache in flight for the
// assertion to distinguish from); a third List() proves the cache and
// validator on offer are still the FIRST call's, never the second (failed)
// call's partial page-1 fetch.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_304OnSecondPage_FailsWholeList(t *testing.T) {
	var srv *httptest.Server
	call := 0
	var capturedIfModifiedSince []string
	const lastModified1 = "Wed, 21 Oct 2015 07:28:00 GMT"

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		capturedIfModifiedSince = append(capturedIfModifiedSince, r.Header.Get("If-Modified-Since"))
		switch call {
		case 1:
			// First List() call: succeeds outright, single page, caches a
			// validator and rows.
			w.Header().Set("Last-Modified", lastModified1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
		case 2:
			// Second List() call, page 1: a fresh 200 with a DIFFERENT
			// Last-Modified and rows, plus a Link header pointing at page 2 —
			// the walk is mid-flight when it fails below, so if this partial
			// fetch leaked into the cache it would be distinguishable from
			// call 1's.
			w.Header().Set("Last-Modified", "Thu, 22 Oct 2015 07:28:00 GMT")
			w.Header().Set("Link", `<`+srv.URL+`/notifications?page=2>; rel="next"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"2","reason":"mention"}]`))
		case 3:
			// Second List() call, page 2: a misbehaving intermediary sends a
			// 304 even though no If-Modified-Since was sent for it. This
			// fails the whole List call.
			w.WriteHeader(http.StatusNotModified)
		default:
			// Third List() call: if the cache/validator is still call 1's,
			// this If-Modified-Since matches and a 304 is the correct
			// response — proven by the returned rows below.
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 1 || first[0].ID != "1" {
		t.Fatalf("first List() = %+v, want single row id=1", first)
	}

	if _, err := c.List(NotificationListOpts{}); err == nil {
		t.Fatal("second List() error = nil, want an error when page 2 returns an unsolicited 304")
	}

	third, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("third List() error = %v", err)
	}
	if len(third) != 1 || third[0].ID != "1" {
		t.Fatalf("third List() = %+v, want the untouched cache from the first successful fetch (single row id=1), not the second call's failed mid-walk fetch", third)
	}
	if len(capturedIfModifiedSince) != 4 {
		t.Fatalf("got %d requests, want 4", len(capturedIfModifiedSince))
	}
	if capturedIfModifiedSince[3] != lastModified1 {
		t.Errorf("third call If-Modified-Since = %q, want %q — the validator from the first successful fetch, untouched by the second call's mid-walk failure", capturedIfModifiedSince[3], lastModified1)
	}
}

// ---------------------------------------------------------------------------
// Optional query params (participating, since) — not part of the acceptance
// criteria's numbered list but documented in the API shapes section.
// ---------------------------------------------------------------------------

// Review feedback item 10: the original version of this test computed "want"
// with the same since.Format(time.RFC3339) call the production code makes,
// on an already-UTC fixture — so the ".UTC()" normalisation in buildPath was
// never actually pinned; a production bug that dropped the .UTC() call would
// still pass on a UTC-already input. Asserts the literal expected string and
// adds a non-UTC input row so the normalisation itself is exercised.
// ---------------------------------------------------------------------------
// Task 6: MarkRead / MarkDone — method and path pinned as literals, not
// derived from the code under test (a test that reads a constant it is meant
// to be pinning has already slipped through this run once).
// ---------------------------------------------------------------------------

func TestNotificationsClient_MarkRead_RequestsCorrectMethodAndPath(t *testing.T) {
	var capturedMethod, capturedPath string
	requests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusResetContent)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if err := c.MarkRead("42"); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if capturedMethod != "PATCH" {
		t.Errorf("method = %q, want %q", capturedMethod, "PATCH")
	}
	if capturedPath != "/notifications/threads/42" {
		t.Errorf("path = %q, want %q", capturedPath, "/notifications/threads/42")
	}
}

func TestNotificationsClient_MarkDone_RequestsCorrectMethodAndPath(t *testing.T) {
	var capturedMethod, capturedPath string
	requests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusResetContent)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	if err := c.MarkDone("42"); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if capturedMethod != "DELETE" {
		t.Errorf("method = %q, want %q", capturedMethod, "DELETE")
	}
	if capturedPath != "/notifications/threads/42" {
		t.Errorf("path = %q, want %q", capturedPath, "/notifications/threads/42")
	}
}

// ---------------------------------------------------------------------------
// Task 6: rejected thread ids must issue zero requests — the id guard runs
// before any HTTP request is built. Rows per task 6's acceptance criteria:
// "", "abc", "-5" (the convention-11 negative-input shape), "0", and "007"
// (isItemNumber's extra leading-zero strictness, reused from
// mapping_notifications.go — see MarkRead's doc comment).
// ---------------------------------------------------------------------------

func TestNotificationsClient_MarkRead_RejectedIds_IssueZeroRequests(t *testing.T) {
	ids := []string{"", "abc", "-5", "0", "007"}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			c := NewNotificationsClient("tok")
			c.SetBaseURL(srv.URL)

			if err := c.MarkRead(id); err == nil {
				t.Fatalf("MarkRead(%q) error = nil, want a rejection error", id)
			}
			if requests != 0 {
				t.Errorf("requests = %d, want 0 — a rejected id must never reach the network", requests)
			}
		})
	}
}

func TestNotificationsClient_MarkDone_RejectedIds_IssueZeroRequests(t *testing.T) {
	ids := []string{"", "abc", "-5", "0", "007"}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			c := NewNotificationsClient("tok")
			c.SetBaseURL(srv.URL)

			if err := c.MarkDone(id); err == nil {
				t.Fatalf("MarkDone(%q) error = nil, want a rejection error", id)
			}
			if requests != 0 {
				t.Errorf("requests = %d, want 0 — a rejected id must never reach the network", requests)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 6: a non-2xx response surfaces as an error carrying the status code.
// ---------------------------------------------------------------------------

func TestNotificationsClient_MarkRead_NonSuccessStatus_ReturnsErrorWithStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	err := c.MarkRead("42")
	if err == nil {
		t.Fatal("MarkRead() error = nil, want an error for a 404 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *APIError from %v", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
}

func TestNotificationsClient_MarkDone_NonSuccessStatus_ReturnsErrorWithStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	err := c.MarkDone("42")
	if err == nil {
		t.Fatal("MarkDone() error = nil, want an error for a 403 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not recover *APIError from %v", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}
}

// ---------------------------------------------------------------------------
// Decision 37, the important regression test: List (200, populates the
// cache) -> MarkRead -> List again against a server that would answer 304 to
// any conditional request. Without cacheGen invalidation, the second List
// would send the cached If-Modified-Since, get a 304, and
// cloneThreads(c.cached) would faithfully replay the row the user just
// dismissed with Unread: true. Asserted on observable behaviour (the returned
// row's Unread field), not on the private cacheGen/cachedGen counters.
// ---------------------------------------------------------------------------

func TestNotificationsClient_MarkRead_InvalidatesCache_SecondListNotStale(t *testing.T) {
	listCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/notifications/threads/1":
			w.WriteHeader(http.StatusResetContent)
		case r.Method == http.MethodGet:
			listCalls++
			if listCalls == 1 {
				w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[{"id":"1","unread":true,"reason":"subscribed"}]`))
				return
			}
			// Second List(): a server that would happily 304 any conditional
			// request — the failure mode under test is the client offering
			// If-Modified-Since here at all after MarkRead. A correctly
			// invalidated cache sends none, so this branch always answers
			// with a fresh 200 reflecting the now-read state.
			if r.Header.Get("If-Modified-Since") != "" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","unread":false,"reason":"subscribed"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 1 || !first[0].Unread {
		t.Fatalf("first List() = %+v, want single unread row", first)
	}

	if err := c.MarkRead("1"); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}

	second, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second List() len = %d, want 1", len(second))
	}
	if second[0].Unread {
		t.Fatal("second List()[0].Unread = true, want false — MarkRead must invalidate the cache so a 304 cannot resurrect the row the user just dismissed (Decision 37)")
	}
}

// ---------------------------------------------------------------------------
// Decision 37 / concurrency: MarkRead must not block while a List is in
// flight. -race is unavailable in this environment, so this is asserted
// structurally rather than by timing: List's HTTP request is parked in the
// handler on a channel, and MarkRead (hitting the same server, a different
// method/path) is proven to complete before the test releases List's
// handler. If MarkRead took the fetch mutex, it would deadlock behind List
// until the release, which the select below with a bounded timeout catches.
// ---------------------------------------------------------------------------

func TestNotificationsClient_MarkRead_DoesNotBlockOnInFlightList(t *testing.T) {
	release := make(chan struct{})
	listStarted := make(chan struct{})

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

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	listDone := make(chan error, 1)
	go func() {
		_, err := c.List(NotificationListOpts{})
		listDone <- err
	}()

	<-listStarted // List's request has reached the handler and is now parked

	markDone := make(chan error, 1)
	go func() {
		markDone <- c.MarkRead("42")
	}()

	select {
	case err := <-markDone:
		if err != nil {
			t.Fatalf("MarkRead() error = %v, want nil", err)
		}
	case err := <-listDone:
		t.Fatalf("List() returned before MarkRead even though its handler is still parked on release: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("MarkRead() did not complete while a List() call was in flight — structural evidence it took the fetch mutex (Decision 37)")
	}

	close(release)
	if err := <-listDone; err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestNotificationsClient_List_ParticipatingAndSince(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60)

	tests := []struct {
		name      string
		since     time.Time
		wantSince string
	}{
		{
			name:      "already UTC",
			since:     time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
			wantSince: "2026-07-01T12:00:00Z",
		},
		{
			name:      "non-UTC offset normalised to UTC",
			since:     time.Date(2026, 7, 1, 14, 0, 0, 0, cest),
			wantSince: "2026-07-01T12:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParticipating, capturedSince string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedParticipating = r.URL.Query().Get("participating")
				capturedSince = r.URL.Query().Get("since")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[]`))
			}))
			defer srv.Close()

			c := NewNotificationsClient("tok")
			c.SetBaseURL(srv.URL)

			if _, err := c.List(NotificationListOpts{Participating: true, Since: tt.since}); err != nil {
				t.Fatalf("List() error = %v", err)
			}

			if capturedParticipating != "true" {
				t.Errorf("participating query param = %q, want %q", capturedParticipating, "true")
			}
			if capturedSince != tt.wantSince {
				t.Errorf("since query param = %q, want %q", capturedSince, tt.wantSince)
			}
		})
	}
}
