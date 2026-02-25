package azdevops

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents an Azure DevOps API client
type Client struct {
	org        string
	project    string
	pat        string
	baseURL    string
	httpClient *http.Client
	userID     string // cached authenticated user ID
}

// GetOrg returns the organization name
func (c *Client) GetOrg() string {
	return c.org
}

// GetProject returns the project name
func (c *Client) GetProject() string {
	return c.project
}

// NewClient creates a new Azure DevOps API client
func NewClient(org, project, pat string) (*Client, error) {
	if org == "" {
		return nil, fmt.Errorf("organization cannot be empty")
	}

	if project == "" {
		return nil, fmt.Errorf("project cannot be empty")
	}

	if pat == "" {
		return nil, fmt.Errorf("PAT cannot be empty")
	}

	baseURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis", org, project)

	return &Client{
		org:     org,
		project: project,
		pat:     pat,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// get performs a GET request to the Azure DevOps API
func (c *Client) get(path string) ([]byte, error) {
	// Construct full URL
	url := c.baseURL + path

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, formatHTTPError(resp.StatusCode, body)
	}

	return body, nil
}

// setAuthHeader sets the Authorization header with Basic auth using PAT
// Azure DevOps uses the format ":{PAT}" for basic auth
func (c *Client) setAuthHeader(req *http.Request) {
	auth := ":" + c.pat
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", "Basic "+encodedAuth)
}

// put performs a PUT request to the Azure DevOps API
func (c *Client) put(path string, body io.Reader) ([]byte, error) {
	return c.doRequest("PUT", path, body)
}

// patch performs a PATCH request to the Azure DevOps API
func (c *Client) patch(path string, body io.Reader) ([]byte, error) {
	return c.doRequest("PATCH", path, body)
}

// post performs a POST request to the Azure DevOps API
func (c *Client) post(path string, body io.Reader) ([]byte, error) {
	return c.doRequest("POST", path, body)
}

// doRequest performs an HTTP request with the given method
func (c *Client) doRequest(method, path string, body io.Reader) ([]byte, error) {
	url := c.baseURL + path

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, formatHTTPError(resp.StatusCode, respBody)
	}

	return respBody, nil
}

// connectionDataResponse holds the response from the connection data API
type connectionDataResponse struct {
	AuthenticatedUser struct {
		ID string `json:"id"`
	} `json:"authenticatedUser"`
}

// GetCurrentUserID returns the authenticated user's ID, fetching and caching it on first call
func (c *Client) GetCurrentUserID() (string, error) {
	if c.userID != "" {
		return c.userID, nil
	}

	// Connection data is at org level, not project-scoped
	url := fmt.Sprintf("https://dev.azure.com/%s/_apis/connectionData", c.org)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch connection data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", formatHTTPError(resp.StatusCode, body)
	}

	var data connectionDataResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("failed to parse connection data: %w", err)
	}

	if data.AuthenticatedUser.ID == "" {
		return "", fmt.Errorf("connection data did not contain a user ID")
	}

	c.userID = data.AuthenticatedUser.ID
	return c.userID, nil
}

// formatHTTPError creates a user-friendly error message based on the HTTP status code
func formatHTTPError(statusCode int, _ []byte) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("authentication failed (HTTP 401): your PAT may be expired or invalid. " +
			"Please generate a new PAT in Azure DevOps and update your configuration")
	case http.StatusForbidden:
		return fmt.Errorf("access denied (HTTP 403): your PAT does not have sufficient permissions. " +
			"Required scopes: Code (Read), Build (Read), Work Items (Read)")
	case http.StatusNotFound:
		return fmt.Errorf("resource not found (HTTP 404): the requested resource does not exist. " +
			"Please verify your organization and project names are correct in your configuration")
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limit exceeded (HTTP 429): too many requests to Azure DevOps. " +
			"Please wait a few minutes before retrying")
	case http.StatusInternalServerError:
		return fmt.Errorf("server error (HTTP 500): Azure DevOps encountered an internal error. " +
			"This is usually temporary - please try again in a few moments")
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable (HTTP 503): Azure DevOps is temporarily unavailable. " +
			"This is usually a temporary issue - please try again later")
	default:
		return fmt.Errorf("HTTP request failed with status %d", statusCode)
	}
}
