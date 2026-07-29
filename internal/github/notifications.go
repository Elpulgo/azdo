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
	"time"
)

// notifPerPageCap is the maximum page size accepted by GET /notifications.
// Mirrors issuePerPageCap: request the largest page GitHub allows and follow
// the Link header (see nextPageURL / client.go) to collect every page.
const notifPerPageCap = 100

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
//     If-Modified-Since on the next call;
//   - a 304 response (an unmodified inbox) returns the previously cached
//     slice unchanged rather than an empty one — an emptied attention feed
//     is indistinguishable from "you're clear" (Decision 20's rationale), so
//     a 304 must never be read as "the inbox is now empty";
//   - the X-Poll-Interval header is parsed and exposed via PollInterval, the
//     cadence floor GitHub asks callers to respect (Decision 8). This is
//     deliberately a plain accessor, not a fourth method on
//     provider.NotificationSource: task 15 discovers it through a separate
//     PollIntervalHinter optional interface (Decision 23).
type NotificationsClient struct {
	baseURL    string
	token      string
	httpClient *http.Client

	lastModified string
	cached       []NotificationThread
	pollInterval time.Duration
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
// at an httptest.Server.
func (c *NotificationsClient) SetBaseURL(url string) {
	c.baseURL = url
}

// PollInterval returns the cadence floor GitHub most recently asked for via
// X-Poll-Interval. Before any successful call, or whenever the header was
// absent or malformed on every call so far, this falls back to
// defaultPollInterval (60s) — a garbage or missing header must never panic
// or produce a nonsense (e.g. zero or negative) interval.
func (c *NotificationsClient) PollInterval() time.Duration {
	if c.pollInterval <= 0 {
		return defaultPollInterval
	}
	return c.pollInterval
}

// List fetches the caller's notification inbox. Per Decision 12 it always
// requests all=true — the default (unread-only) response would otherwise
// make a row vanish from the very next poll the moment it is marked read.
// opts shapes the request further (participating, since) but never disables
// all=true.
//
// Pagination follows the Link header (rel="next") across every page; wire→
// neutral mapping is NOT performed here (task 5) — this returns the raw wire
// shape.
//
// A 304 (Not Modified) response — returned when the previously cached
// Last-Modified value is still current — returns the cached slice from the
// last successful fetch, unchanged. It is never interpreted as an empty
// inbox.
func (c *NotificationsClient) List(opts NotificationListOpts) ([]NotificationThread, error) {
	path := c.buildPath(opts)

	body, header, err := c.getPage(path, c.lastModified)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotModified {
			c.applyPollInterval(header)
			return c.cached, nil
		}
		return nil, fmt.Errorf("github: list notifications: %w", err)
	}
	c.applyPollInterval(header)

	var page []NotificationThread
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("github: decode response: %w", err)
	}
	all := append([]NotificationThread(nil), page...)

	next := nextPageURL(header)
	for next != "" {
		pageBody, pageHeader, err := c.getPage(next, "")
		if err != nil {
			return nil, fmt.Errorf("github: list notifications: %w", err)
		}
		var more []NotificationThread
		if err := json.Unmarshal(pageBody, &more); err != nil {
			return nil, fmt.Errorf("github: decode response: %w", err)
		}
		all = append(all, more...)
		next = nextPageURL(pageHeader)
	}

	if lm := header.Get("Last-Modified"); lm != "" {
		c.lastModified = lm
	}
	c.cached = all
	return all, nil
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
func (c *NotificationsClient) applyPollInterval(header http.Header) {
	raw := header.Get("X-Poll-Interval")
	if raw == "" {
		return
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		return
	}
	c.pollInterval = time.Duration(secs) * time.Second
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
