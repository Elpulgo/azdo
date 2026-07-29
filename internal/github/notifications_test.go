package github

import (
	"net/http"
	"net/http/httptest"
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
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_304_ReturnsCachedSliceUnchanged(t *testing.T) {
	call := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"1","reason":"subscribed"},{"id":"2","reason":"mention"}]`))
			return
		}
		// Second call: inbox unchanged, empty body per GitHub's documented
		// 304 contract.
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	c := NewNotificationsClient("tok")
	c.SetBaseURL(srv.URL)

	first, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first List() len = %d, want 2", len(first))
	}

	second, err := c.List(NotificationListOpts{})
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}

	if len(second) != len(first) {
		t.Fatalf("second List() len = %d, want %d (unchanged)", len(second), len(first))
	}
	for i := range first {
		if second[i] != first[i] {
			t.Errorf("second[%d] = %+v, want %+v (unchanged)", i, second[i], first[i])
		}
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
// Optional query params (participating, since) — not part of the acceptance
// criteria's numbered list but documented in the API shapes section.
// ---------------------------------------------------------------------------

func TestNotificationsClient_List_ParticipatingAndSince(t *testing.T) {
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

	since := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if _, err := c.List(NotificationListOpts{Participating: true, Since: since}); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if capturedParticipating != "true" {
		t.Errorf("participating query param = %q, want %q", capturedParticipating, "true")
	}
	if want := since.Format(time.RFC3339); capturedSince != want {
		t.Errorf("since query param = %q, want %q", capturedSince, want)
	}
}
