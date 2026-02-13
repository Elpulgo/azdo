package azdevops

import (
	"encoding/json"
	"fmt"
)

// GetBuildTimeline retrieves the timeline for a specific build
// The timeline contains all stages, jobs, and tasks with their status and logs
func (c *Client) GetBuildTimeline(buildID int) (*Timeline, error) {
	path := fmt.Sprintf("/build/builds/%d/timeline?api-version=7.1", buildID)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get build timeline: %w", err)
	}

	var timeline Timeline
	err = json.Unmarshal(body, &timeline)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for build timeline: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return &timeline, nil
}
