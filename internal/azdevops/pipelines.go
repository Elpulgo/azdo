package azdevops

import (
	"encoding/json"
	"fmt"
	"time"
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

// ListMyFailedPipelineRuns retrieves completed, failed pipeline runs
// requested for userID whose FinishTime falls within the last lookbackDays
// days, narrowed **server-side** via the List Builds 7.1 query parameters
// (`statusFilter`, `resultFilter`, `requestedFor`, `minTime`). This backs the
// "my failed pipeline runs" notification source (task 7 of the phase-2
// notifications spec, SourceCIFailed in notifications_source_cifailed.go).
//
// It is a deliberately separate method from ListPipelineRuns, not a new
// parameter on it: ListPipelineRuns backs the pipelines pane, which wants the
// N most recent runs across all pipelines, all users and all results, and
// must keep that signature and behaviour untouched for that caller.
// ListPipelineRuns's own $top window is spent before any "mine and failed"
// filtering happens, which is exactly the defect this method exists to avoid
// for the notification source: on a busy project, a client-side filter over
// ListPipelineRuns's window can age a still-unread failure out of the feed
// entirely.
//
// lookbackDays is clamped to 0 when negative, matching
// Client.ListRecentlyAssignedWorkItems's own guard against a malformed query
// value (convention 11's habit applied to a query parameter rather than an
// id): `minTime` values are formatted RFC3339 and a negative lookback would
// otherwise put minTime in the future, which would make the server return no
// rows rather than fail loudly.
//
// top: maximum number of runs to return, passed straight through to $top.
func (c *Client) ListMyFailedPipelineRuns(userID string, lookbackDays, top int) ([]PipelineRun, error) {
	if lookbackDays < 0 {
		lookbackDays = 0
	}
	minTime := time.Now().Add(-time.Duration(lookbackDays) * 24 * time.Hour).UTC().Format(time.RFC3339)

	path := fmt.Sprintf(
		"/build/builds?api-version=7.1&statusFilter=completed&resultFilter=failed"+
			"&requestedFor=%s&minTime=%s&$top=%d&queryOrder=finishTimeDescending",
		userID, minTime, top,
	)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list failed pipeline runs: %w", err)
	}

	var response PipelineRunsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for pipeline runs: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return response.Value, nil
}
