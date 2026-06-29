// Package github implements a per-repository GitHub REST API client.
// It mirrors the internal/azdevops layering: a per-repo Client handles HTTP
// auth + JSON decode; a MultiClient (task 12) fans out across repos using
// provider.PartialError; an Adapter (task 12) satisfies provider.Provider.
//
// Phase 3 constraint: this package is unwired. Nothing in cmd/, internal/app,
// or main.go references it until Phase 4.
package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// A non-2xx status code is converted to a descriptive apiError; the response
// body is intentionally not included in the error to avoid leaking server-side
// details (mirrors azdevops.formatHTTPError).
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
		return nil, apiError(resp.StatusCode)
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

// apiError maps a non-2xx HTTP status code to a descriptive, user-facing
// error. The caller should not include the response body to avoid leaking
// server-internal details.
func apiError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("github: authentication failed (HTTP 401): token may be expired or invalid")
	case http.StatusForbidden:
		return fmt.Errorf("github: access denied (HTTP 403): token lacks the required scopes")
	case http.StatusNotFound:
		return fmt.Errorf("github: resource not found (HTTP 404): check the repository name and token scopes")
	case http.StatusTooManyRequests:
		return fmt.Errorf("github: rate limit exceeded (HTTP 429): please wait before retrying")
	case http.StatusInternalServerError:
		return fmt.Errorf("github: server error (HTTP 500): GitHub encountered an internal error")
	case http.StatusServiceUnavailable:
		return fmt.Errorf("github: service unavailable (HTTP 503): GitHub is temporarily unavailable")
	default:
		return fmt.Errorf("github: request failed with status %d", statusCode)
	}
}
