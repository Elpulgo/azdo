package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// notifPerPageCap is the maximum page size accepted by GET /notifications.
// Mirrors issuePerPageCap: request the largest page GitHub allows and follow
// the Link header (see nextPageURL / client.go) to collect every page.
const notifPerPageCap = 100

// maxNotificationPages bounds the Link rel="next" walk in List. This is cycle
// protection against a malformed or malicious "next" relation, not the
// user-configured page budget — Decision 29 splits the two: this constant
// stops a misbehaving server (a self-referential "next" produced 501
// requests and 500 rows with err == nil before this existed), while
// NotifOpts.Max (task 7) honours the user's configured item cap at the
// adapter boundary. Both are needed; neither subsumes the other.
const maxNotificationPages = 50

// defaultPollInterval is the fallback cadence used when GitHub's
// X-Poll-Interval response header is absent or malformed. 60 seconds is
// GitHub's own documented default poll interval for GET /notifications, so it
// is used here rather than an arbitrary value — but this is the documented
// default, not something observed live (no token in this environment).
const defaultPollInterval = 60 * time.Second

// NotificationListOpts carries fetch intent for NotificationsClient.List.
// There is deliberately no "unread only" option: Decision 12 requires every
// fetch to request all=true regardless of caller intent, so unread-only
// filtering has exactly one landing site — the client-side filter in task 10.
type NotificationListOpts struct {
	// Participating restricts results to notifications where the
	// authenticated user is directly participating, mapping to GitHub's
	// participating=true query parameter (Decision 10). False requests the
	// whole inbox.
	Participating bool

	// Since restricts results to notifications updated at or after this
	// time, mapping to GitHub's since query parameter. Zero value means no
	// lower bound and the parameter is omitted.
	Since time.Time
}

// NotificationsClient is a user-scoped GitHub REST API client for
// GET /notifications. Unlike Client, it is NOT scoped to an owner/repo: the
// notifications endpoint is user-level (see the spec's Constraints section
// and learned convention 2, which explicitly does not apply here). It
// therefore cannot join the per-repo scope-routed fan-out (MultiClient) and
// is constructed and driven independently.
//
// NotificationsClient caches enough state across calls to honour GitHub's
// conditional-request contract for this endpoint:
//   - the Last-Modified header from the last 200 response is echoed back as
//     If-Modified-Since on the next call, but only when the request shape
//     (buildPath(opts)) matches the one that produced it — otherwise a
//     cached validator from one query (e.g. the whole inbox) would be sent
//     for a differently-shaped query (e.g. participating-only) and a 304
//     would hand back the wrong cached rows presented as a fresh answer
//     (Decision 27, fold-in 3);
//   - a 304 response (an unmodified inbox) returns a copy of the previously
//     cached slice, unchanged, rather than an empty one — an emptied
//     attention feed is indistinguishable from "you're clear" (Decision 20's
//     rationale), so a 304 must never be read as "the inbox is now empty".
//     A 304 with no cache that matches the current request shape is an
//     error, never an empty feed (Decision 28);
//   - the X-Poll-Interval header is parsed and exposed via PollInterval, the
//     cadence floor GitHub asks callers to respect (Decision 8). This is
//     deliberately a plain accessor, not a fourth method on
//     provider.NotificationSource: task 15 discovers it through a separate
//     PollIntervalHinter optional interface (Decision 23).
//
// This type is mutable across calls (cache, validator, poll interval), unlike
// Client/MultiClient which are immutable after construction — so, unlike
// those types, it needs its own lock. mu is held across the entire List body
// (not just the field reads/writes) because Bubble Tea runs every tea.Cmd in
// its own goroutine: a poll tick and a user-triggered refresh key genuinely
// run List concurrently, and holding the lock for the whole fetch also
// serialises the conditional-request pairing (the If-Modified-Since sent
// must correspond to the lastModified/cachedPath this same call reads),
// which is the point, not a missed opportunity to narrow the critical
// section.
//
// pollInterval deliberately does NOT share mu (Decision 32): app.go calls
// poller.StartPolling()/OnTick() — which read PollInterval() — on the Bubble
// Tea main goroutine, so if the accessor blocked on mu, a tick landing during
// a multi-page List refresh would freeze the main goroutine for as long as
// that refresh takes (worst case maxNotificationPages HTTP round trips),
// dropping keypresses and rendering. pollInterval is therefore an
// atomic.Int64 of nanoseconds, written by applyPollInterval and read by
// PollInterval, with no lock at all.
type NotificationsClient struct {
	mu sync.Mutex

	baseURL    string
	token      string
	httpClient *http.Client

	// lastModified and cached are only valid for the request shape recorded
	// in cachedPath (see buildPath) — an empty cachedPath means nothing is
	// cached yet.
	lastModified string
	cached       []NotificationThread
	cachedPath   string

	// pollInterval holds the cadence GitHub last asked for, in nanoseconds,
	// with 0 meaning "not yet set". Guarded independently of mu — see the
	// type-level doc comment (Decision 32) — via atomic loads/stores rather
	// than a second mutex, since it is a single word.
	pollInterval atomic.Int64
}

// NewNotificationsClient creates a user-scoped GitHub notifications client.
// token is a GitHub personal access token or GitHub App installation token
// that must carry the "notifications" scope (classic PAT) or
// "Notifications: read+write" (fine-grained) — a missing scope surfaces as a
// 403 *APIError; see APIError.RequiredScopes/GrantedScopes.
func NewNotificationsClient(token string) *NotificationsClient {
	return &NotificationsClient{
		baseURL: defaultBaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetBaseURL overrides the API base URL. Used in tests to point the client
// at an httptest.Server, and in production to point at a GitHub Enterprise
// instance instead of github.com.
//
// It unconditionally clears the cache, cached validator and cached request
// shape. buildPath(opts) — the cache key — has no host component, so without
// this a cache populated against one base URL would still match cachedPath
// after switching base URLs: a later List with the same opts would offer the
// previous host's Last-Modified as a validator, and a 304 would hand back the
// previous host's rows presented as the new host's answer.
//
// SetBaseURL is deliberately left unlocked, matching Client.SetBaseURL and
// azdevops.Client.SetBaseURL: it is a construction-time-only override (tests,
// demo mode, switching to a GHE instance) and its contract is that it must
// not be called concurrently with List — callers needing to redirect a
// live client must not do so while a fetch may be in flight.
func (c *NotificationsClient) SetBaseURL(url string) {
	c.baseURL = url
	c.cachedPath = ""
	c.lastModified = ""
	c.cached = nil
}

// PollInterval returns the cadence floor GitHub most recently asked for via
// X-Poll-Interval. Before any successful call, or whenever the header was
// absent or malformed on every call so far, this falls back to
// defaultPollInterval (60s) — a garbage or missing header must never panic
// or produce a nonsense (e.g. zero or negative) interval.
//
// Deliberately does not take mu (Decision 32) — see the type-level doc
// comment — so a poll tick reading the cadence on Bubble Tea's main goroutine
// never blocks behind an in-flight, possibly multi-page List call.
func (c *NotificationsClient) PollInterval() time.Duration {
	if ns := c.pollInterval.Load(); ns > 0 {
		return time.Duration(ns)
	}
	return defaultPollInterval
}

// List fetches the caller's notification inbox. Per Decision 12 it always
// requests all=true — the default (unread-only) response would otherwise
// make a row vanish from the very next poll the moment it is marked read.
// opts shapes the request further (participating, since) but never disables
// all=true.
//
// Pagination follows the Link header (rel="next") across every page, bounded
// by maxNotificationPages as cycle protection against a malformed or
// self-referential "next" relation (Decision 29); wire→neutral mapping is NOT
// performed here (task 5) — this returns the raw wire shape.
//
// A 304 (Not Modified) response — returned when the previously cached
// Last-Modified value is still current for this exact request shape — returns
// a copy of the cached slice from the last successful fetch for that shape,
// unchanged. It is never interpreted as an empty inbox (Decision 20). A 304
// that does not match a cached request shape (nothing cached yet, or the
// cache belongs to a differently-shaped request) is reported as an error
// rather than silently returned as an empty or stale feed (Decision 28).
//
// The returned slice is always a copy (Decision 27, fold-in 2): List hands
// out a fresh copy on both the 200 and 304 paths, so a caller's in-place
// filter/sort of the returned slice can never rewrite the cache that a later
// 304 hands back.
func (c *NotificationsClient) List(opts NotificationListOpts) ([]NotificationThread, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := c.buildPath(opts)

	// Only offer the cached validator when it was produced by this exact
	// request shape — otherwise a 200 for a different query could be
	// conditioned against a validator that has nothing to do with it.
	ifModifiedSince := ""
	if path == c.cachedPath {
		ifModifiedSince = c.lastModified
	}

	body, header, err := c.getPage(path, ifModifiedSince)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotModified {
			c.applyPollInterval(header)
			if path != c.cachedPath {
				// Decision 28: a 304 that does not match a cache produced by
				// this exact request shape is never treated as "the inbox is
				// empty" — it is reported as an error instead. Wrapped with
				// %w (not just formatted in) so a caller can errors.As this
				// back to the underlying *APIError (StatusCode == 304) and
				// branch on that rather than string-matching the message —
				// tasks 13 and 19 both need to tell an unsolicited 304 apart
				// from a transport failure.
				return nil, fmt.Errorf("github: list notifications: received 304 Not Modified with no matching cached response: %w", apiErr)
			}
			return cloneThreads(c.cached), nil
		}
		return nil, err
	}
	c.applyPollInterval(header)

	var page []NotificationThread
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("github: decode response: %w", err)
	}
	all := append([]NotificationThread(nil), page...)

	next := nextPageURL(header)
	pages := 1
	for next != "" {
		pages++
		if pages > maxNotificationPages {
			// Cycle protection (Decision 29): a self-referential "next"
			// relation would otherwise loop forever inside a tea.Cmd, which
			// never returns and leaves the pane stuck on "loading" with no
			// error and no way to cancel.
			return nil, fmt.Errorf("github: list notifications: exceeded %d pages following Link rel=\"next\"; possible cyclic pagination", maxNotificationPages)
		}

		// GET /notifications never sends If-Modified-Since for subsequent
		// pages (ifModifiedSince is deliberately empty below), so a 304 here
		// can only come from a misbehaving intermediary (Decision 28's
		// caching-proxy scenario). getPage surfaces it as a plain *APIError,
		// which is returned unwrapped below like any other page fetch
		// failure: the whole List call fails, but the cache and validator
		// from the previous successful fetch are left untouched, so the
		// caller does not lose its last-known-good inbox over a mid-walk
		// failure.
		pageBody, pageHeader, err := c.getPage(next, "")
		if err != nil {
			return nil, err
		}
		var more []NotificationThread
		if err := json.Unmarshal(pageBody, &more); err != nil {
			return nil, fmt.Errorf("github: decode response: %w", err)
		}
		all = append(all, more...)
		next = nextPageURL(pageHeader)
	}

	c.lastModified = header.Get("Last-Modified")
	c.cached = all
	c.cachedPath = path
	return cloneThreads(all), nil
}

// cloneThreads returns a defensive copy of in. List hands this out on both
// the 200 and 304 paths (Decision 27, fold-in 2) so a caller's in-place
// filter of the returned slice (e.g. out := s[:0]; append(out, ...)) can
// never rewrite c.cached, and so mutating one call's result can never affect
// a later call's result. A nil input returns nil rather than an empty
// non-nil slice, matching the zero-notifications wire shape.
func cloneThreads(in []NotificationThread) []NotificationThread {
	if in == nil {
		return nil
	}
	out := make([]NotificationThread, len(in))
	copy(out, in)
	return out
}

// buildPath constructs the GET /notifications path and query string for
// opts. all=true is always set (Decision 12) regardless of opts.
func (c *NotificationsClient) buildPath(opts NotificationListOpts) string {
	params := url.Values{}
	params.Set("all", "true")
	if opts.Participating {
		params.Set("participating", "true")
	}
	if !opts.Since.IsZero() {
		params.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	params.Set("per_page", strconv.Itoa(notifPerPageCap))
	return "/notifications?" + params.Encode()
}

// applyPollInterval parses the X-Poll-Interval header, if present, updating
// c.pollInterval. An absent header leaves the previous value untouched; a
// malformed (non-integer or <= 0) value is ignored rather than applied —
// neither case may panic or leave a nonsense interval in place. Use
// PollInterval to read the value with its fallback applied.
//
// Precondition: none — c.pollInterval is an atomic.Int64 (Decision 32), so
// this may be called with or without mu held. In practice every call site is
// inside List, which already holds mu for unrelated reasons (the fetch
// itself); that is incidental, not a requirement of this function. Future
// callers (e.g. task 6's mark read/done, if it ever needs to observe
// X-Poll-Interval) do not need to take mu first.
func (c *NotificationsClient) applyPollInterval(header http.Header) {
	raw := header.Get("X-Poll-Interval")
	if raw == "" {
		return
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		return
	}
	c.pollInterval.Store(int64(time.Duration(secs) * time.Second))
}

// getPage performs an authenticated GET against rawURL (a leading-slash path
// joined to baseURL, or an absolute URL as returned by a Link "next"
// relation), setting If-Modified-Since when ifModifiedSince is non-empty.
//
// Unlike Client.do, a non-2xx status is still returned alongside the
// response headers (not just the error) — List needs the headers on a 304
// response to read X-Poll-Interval, and errors.As lets it detect the 304
// itself via the returned *APIError.
func (c *NotificationsClient) getPage(rawURL, ifModifiedSince string) ([]byte, http.Header, error) {
	fullURL := rawURL
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		fullURL = c.baseURL + rawURL
	}

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if ifModifiedSince != "" {
		req.Header.Set("If-Modified-Since", ifModifiedSince)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("github: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("github: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Header, newAPIError(resp.StatusCode, resp.Header, body)
	}
	return body, resp.Header, nil
}
