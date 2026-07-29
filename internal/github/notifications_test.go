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
// Decision 27: List always hands out a copy, on the 200 path too — not just
// the 304 path. Mutating a 200 result must never leak into a later call's
// result, which is exactly the corruption fold-in 2 measured: a caller
// in-place-filtering [1 2 3] down to [2] left the cache [2 2 3].
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
		// Second call hits the 304 cache-read path, so this exercises the
		// exact corruption fold-in 2 measured: if the 200 path had stored a
		// slice that aliases what it returned, the in-place filter below
		// would corrupt c.cached and this 304 would hand back the corrupted
		// rows.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("first List() len = %d, want 3", len(first))
	}

	// The standard in-place filter idiom: keep only id "2".
	out := first[:0]
	for _, row := range first {
		if row.ID == "2" {
			out = append(out, row)
		}
	}
	if len(out) != 1 || out[0].ID != "2" {
		t.Fatalf("filter setup failed: out = %+v", out)
	}

	second, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second) != 3 {
		t.Fatalf("second List() len = %d, want 3 — the in-place filter of the first result must not have rewritten the cache", len(second))
	}
	wantIDs := []string{"1", "2", "3"}
	for i, id := range wantIDs {
		if second[i].ID != id {
			t.Errorf("second[%d].ID = %q, want %q", i, second[i].ID, id)
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
	if requests > maxNotificationPages+1 {
		t.Errorf("requests = %d, want bounded near maxNotificationPages (%d)", requests, maxNotificationPages)
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
// Review feedback item 4 / Decision 28: a 304 with nothing cached (or nothing
// cached for this request shape) is an error, never an empty feed.
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
// Review feedback nit 9: a 304 on page >= 2 fails the whole List rather than
// being special-cased — documented here since the pagination loop reads as
// though 304 is handled everywhere. Safe: the cache from the prior
// successful fetch is left untouched.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_304OnSecondPage_FailsWholeList(t *testing.T) {
	var srv *httptest.Server
	call := 0

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		switch call {
		case 1:
			w.Header().Set("Link", `<`+srv.URL+`/notifications?page=2>; rel="next"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","reason":"subscribed"}]`))
		default:
			// A misbehaving intermediary sends a 304 on the second page even
			// though no If-Modified-Since was sent for it.
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	_, err := c.List(NotificationListOpts{})
	if err == nil {
		t.Fatal("List() error = nil, want an error when page 2 returns an unsolicited 304")
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
