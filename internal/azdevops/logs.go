package azdevops

import (
	"encoding/json"
	"fmt"
)

// ListBuildLogs retrieves all logs for a specific build
func (c *Client) ListBuildLogs(buildID int) ([]BuildLog, error) {
	path := fmt.Sprintf("/build/builds/%d/logs?api-version=7.1", buildID)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list build logs: %w", err)
	}

	var response BuildLogsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal build logs response: %w", err)
	}

	return response.Value, nil
}

// GetBuildLogContent retrieves the content of a specific log
func (c *Client) GetBuildLogContent(buildID, logID int) (string, error) {
	path := fmt.Sprintf("/build/builds/%d/logs/%d?api-version=7.1", buildID, logID)

	body, err := c.get(path)
	if err != nil {
		return "", fmt.Errorf("failed to get build log content: %w", err)
	}

	return string(body), nil
}
