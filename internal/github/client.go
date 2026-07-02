// Package github implements a per-repository GitHub REST API client.
// It mirrors the internal/azdevops layering: a per-repo Client handles HTTP
// auth + JSON decode; a MultiClient (task 12) fans out across repos using
// provider.PartialError; an Adapter (task 12) satisfies provider.Provider.
//
// Phase 3 constraint: this package is unwired. Nothing in cmd/, internal/app,
// or main.go references it until Phase 4.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	// apiVersion is the GitHub REST API version header value sent with every
	// request. Pinning this ensures stable response shapes even as GitHub
	// evolves the API.
	apiVersion = "2022-11-28"
)

// Client is a per-repository GitHub REST API client. It carries the
// repository identity, base URL (overridable for tests), auth token, and an
// *http.Client. All HTTP helpers on Client set the three standard GitHub
// request headers:
//
//	Authorization: Bearer <token>
//	Accept: application/vnd.github+json
//	X-GitHub-Api-Version: 2022-11-28
type Client struct {
	owner      string
	repo       string
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a GitHub REST API client scoped to owner/repo.
// token is a GitHub personal access token or GitHub App installation token.
// Call SetBaseURL to redirect to an httptest.Server in tests.
func NewClient(owner, repo, token string) *Client {
	return &Client{
		owner:   owner,
		repo:    repo,
		baseURL: defaultBaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetBaseURL overrides the API base URL. Used in tests to point the client
// at an httptest.Server, and in demo mode to point at a local mock server.
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

// Owner returns the repository owner login.
func (c *Client) Owner() string { return c.owner }

// Repo returns the repository name.
func (c *Client) Repo() string { return c.repo }

// Scope returns the canonical "owner/repo" identifier used as the
// provider.Identity.Scope value at the mapping boundary (tasks 5–12).
func (c *Client) Scope() string { return c.owner + "/" + c.repo }

// newRequest builds an authenticated HTTP request targeting baseURL+path
// with the three mandatory GitHub REST headers pre-set.
func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	return req, nil
}

// do executes req, reads the response body, and returns the raw bytes.
// A non-2xx status code is converted to a descriptive *APIError; the response
// body is intentionally not surfaced in the error string to avoid leaking
// server-side details (mirrors azdevops.formatHTTPError).
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newAPIError(resp.StatusCode, resp.Header, body)
	}
	return body, nil
}

// get performs an authenticated GET request and returns the raw response body.
func (c *Client) get(path string) ([]byte, error) {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

// getJSON performs an authenticated GET request and JSON-decodes the response
// body into dst. dst must be a non-nil pointer.
func (c *Client) getJSON(path string, dst any) error {
	body, err := c.get(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("github: decode response: %w", err)
	}
	return nil
}

// getPage performs an authenticated GET and returns both the raw response body
// and the response headers. The headers are required to read the RFC 5988 Link
// header that drives GitHub's pagination (see nextPageURL and getAllPages).
//
// rawURL may be a leading-slash path (joined to baseURL) or an absolute URL.
// The absolute form is used when following a Link "next" relation, which GitHub
// returns as a fully-qualified URL.
func (c *Client) getPage(rawURL string) ([]byte, http.Header, error) {
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
		return nil, nil, newAPIError(resp.StatusCode, resp.Header, body)
	}
	return body, resp.Header, nil
}

// nextPageURL extracts the URL of the rel="next" relation from an RFC 5988 Link
// header, as emitted by GitHub's paginated endpoints. It returns "" when the
// header is absent or carries no "next" relation (i.e. the last page). Example:
//
//	<https://api.github.com/...?page=2>; rel="next", <...?page=9>; rel="last"
func nextPageURL(header http.Header) string {
	link := header.Get("Link")
	if link == "" {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		segments := strings.Split(part, ";")
		if len(segments) < 2 {
			continue
		}
		urlPart := strings.TrimSpace(segments[0])
		if !strings.HasPrefix(urlPart, "<") || !strings.HasSuffix(urlPart, ">") {
			continue
		}
		for _, seg := range segments[1:] {
			if strings.TrimSpace(seg) == `rel="next"` {
				return urlPart[1 : len(urlPart)-1]
			}
		}
	}
	return ""
}

// getAllPages follows GitHub's Link-header pagination starting at firstPath,
// accumulating items until limit have been collected or the pages are exhausted.
// Each page body is handed to extract, which knows how to pull the item slice
// out of that endpoint's response shape — a bare array (/issues, /pulls) or an
// envelope (/search/issues, /actions/runs). firstPath should already request the
// maximum page size (per_page=100). limit trims the accumulated slice; a limit
// <= 0 is treated as no limit and follows every page.
//
// It is a free function rather than a method because Go methods cannot declare
// their own type parameters.
func getAllPages[T any](c *Client, firstPath string, limit int, extract func([]byte) ([]T, error)) ([]T, error) {
	var out []T
	next := firstPath
	for next != "" {
		body, header, err := c.getPage(next)
		if err != nil {
			return nil, err
		}
		items, err := extract(body)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if limit > 0 && len(out) >= limit {
			return out[:limit], nil
		}
		next = nextPageURL(header)
	}
	return out, nil
}

// extractArray is the getAllPages extractor for endpoints that return a bare
// JSON array page body (e.g. /issues, /pulls).
func extractArray[T any](body []byte) ([]T, error) {
	var page []T
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("github: decode response: %w", err)
	}
	return page, nil
}

// APIError is the typed error returned for every non-2xx GitHub response.
// Callers recover it with errors.As(err, &apiErr) — mirroring how the codebase
// already inspects provider.PartialError — to branch on the status code rather
// than string-matching the message. Later tasks rely on this: UpdateThreadStatus
// no-ops on 404/422, and rate-limit handling keys off RateLimited/RetryAfter.
//
// Error() never includes Message or any response body, so server-side details
// are not leaked through the error string; Message is retained on the struct
// for callers that explicitly need it.
type APIError struct {
	StatusCode  int    // HTTP status code of the failed response
	Message     string // GitHub's JSON {"message": "..."}, when present
	RateLimited bool   // true when the response indicates rate limiting
	RetryAfter  string // raw Retry-After header value, when present
}

// Error renders the friendly, status-specific message. A rate-limited response
// (429, or a 403 flagged by the response headers) always reports as a rate
// limit; a plain 403 keeps the missing-scopes wording.
func (e *APIError) Error() string {
	if e.RateLimited {
		return fmt.Sprintf("github: rate limit exceeded (HTTP %d): please wait before retrying", e.StatusCode)
	}
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return "github: authentication failed (HTTP 401): token may be expired or invalid"
	case http.StatusForbidden:
		return "github: access denied (HTTP 403): token lacks the required scopes"
	case http.StatusNotFound:
		return "github: resource not found (HTTP 404): check the repository name and token scopes"
	case http.StatusTooManyRequests:
		return "github: rate limit exceeded (HTTP 429): please wait before retrying"
	case http.StatusInternalServerError:
		return "github: server error (HTTP 500): GitHub encountered an internal error"
	case http.StatusServiceUnavailable:
		return "github: service unavailable (HTTP 503): GitHub is temporarily unavailable"
	default:
		return fmt.Sprintf("github: request failed with status %d", e.StatusCode)
	}
}

// newAPIError builds an *APIError from a non-2xx response. It parses GitHub's
// JSON {"message": "..."} body into Message and inspects the headers to tell a
// throttled 403 (X-RateLimit-Remaining: 0 or a Retry-After header) apart from a
// plain insufficient-scopes 403. A 429 is always treated as rate limited.
func newAPIError(statusCode int, header http.Header, body []byte) *APIError {
	e := &APIError{StatusCode: statusCode}

	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		e.Message = parsed.Message
	}

	e.RetryAfter = header.Get("Retry-After")

	switch statusCode {
	case http.StatusTooManyRequests:
		e.RateLimited = true
	case http.StatusForbidden:
		if header.Get("X-RateLimit-Remaining") == "0" || e.RetryAfter != "" {
			e.RateLimited = true
		}
	}

	return e
}

// apiError maps a non-2xx HTTP status code to a descriptive *APIError without
// response headers or body. Retained for status-only construction and tests;
// the do path uses newAPIError so it can detect rate limiting from headers.
func apiError(statusCode int) *APIError {
	return newAPIError(statusCode, http.Header{}, nil)
}

// doJSON sends a method+path request with an optional JSON-marshalled body and
// decodes the JSON response into dst. Pass dst=nil to discard the response body.
//
// payload is marshalled with encoding/json and sent as Content-Type:
// application/json. Authentication headers are identical to those set by
// newRequest (Bearer token, Accept, X-GitHub-Api-Version). A non-2xx response
// is converted to *APIError via the same newAPIError path as do(). Use this
// for POST and PATCH; keep get/getJSON for read-only requests.
func (c *Client) doJSON(method, path string, payload any, dst any) error {
	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("github: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := c.newRequest(method, path, bodyReader)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	raw, err := c.do(req)
	if err != nil {
		return err
	}

	if dst != nil {
		if err := json.Unmarshal(raw, dst); err != nil {
			return fmt.Errorf("github: decode response: %w", err)
		}
	}
	return nil
}
