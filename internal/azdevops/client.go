package azdevops

import (
	"encoding/base64"
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
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(body))
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
