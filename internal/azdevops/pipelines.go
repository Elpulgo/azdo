package azdevops

import (
	"encoding/json"
	"fmt"
)

// ListPipelineRuns retrieves the most recent pipeline runs (builds) for the project
// top: maximum number of runs to return (typically 25-100)
func (c *Client) ListPipelineRuns(top int) ([]PipelineRun, error) {
	path := fmt.Sprintf("/build/builds?api-version=7.1&$top=%d", top)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list pipeline runs: %w", err)
	}

	var response PipelineRunsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline runs response: %w", err)
	}

	return response.Value, nil
}
