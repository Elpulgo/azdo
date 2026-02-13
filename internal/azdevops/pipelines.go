package azdevops

import (
	"encoding/json"
	"fmt"
)

// ListPipelineRuns retrieves the most recent pipeline runs (builds) for the project
// top: maximum number of runs to return (typically 25-100)
// Results are ordered by queue time descending (most recent first)
func (c *Client) ListPipelineRuns(top int) ([]PipelineRun, error) {
	path := fmt.Sprintf("/build/builds?api-version=7.1&$top=%d&queryOrder=queueTimeDescending", top)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list pipeline runs: %w", err)
	}

	var response PipelineRunsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for pipeline runs: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return response.Value, nil
}
