package github

import (
	"encoding/json"
	"fmt"

	"github.com/Elpulgo/azdo/internal/provider"
)

// pipelinePerPageCap is the maximum page size accepted by
// GET /repos/.../actions/runs and GET /repos/.../actions/runs/{id}/jobs.
// The GitHub REST API hard-caps per_page at 100. ListPipelineRuns requests this
// page size and follows the Link header (see getAllPages) to collect up to the
// caller's requested top across multiple pages.
const pipelinePerPageCap = 100

// workflowRunsResponse is the envelope returned by
// GET /repos/{owner}/{repo}/actions/runs.
// The actual run slice lives under "workflow_runs"; TotalCount is informational.
type workflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

// mapRunStatusParam translates a single neutral provider.RunStatus into the
// GitHub Actions ?status= query parameter value accepted by the runs endpoint.
//
// GitHub's runs endpoint accepts a single status value per request. The mapping
// below covers cases with a clean 1:1 correspondence between the neutral enum
// and a GitHub status string. Values with no GitHub equivalent return "".
//
// Mapping:
//
//	RunStatusRunning   → "in_progress"
//	RunStatusQueued    → "queued"
//	RunStatusPending   → "waiting"  (deployment-protection gate)
//	RunStatusSucceeded → "success"
//	RunStatusFailed    → "failure"
//	RunStatusCanceled  → "cancelled"
//
// RunStatusCanceling, RunStatusPartiallySucceeded, RunStatusSucceededWithIssues,
// and RunStatusUnknown have no clean GitHub equivalent and return "" (omit param).
func mapRunStatusParam(s provider.RunStatus) string {
	switch s {
	case provider.RunStatusRunning:
		return "in_progress"
	case provider.RunStatusQueued:
		return "queued"
	case provider.RunStatusPending:
		return "waiting"
	case provider.RunStatusSucceeded:
		return "success"
	case provider.RunStatusFailed:
		return "failure"
	case provider.RunStatusCanceled:
		return "cancelled"
	default:
		// RunStatusCanceling, RunStatusPartiallySucceeded,
		// RunStatusSucceededWithIssues, RunStatusUnknown — no clean 1:1 mapping.
		return ""
	}
}

// ListPipelineRuns returns up to top Actions workflow runs for the repository,
// ordered by most recently created (GitHub default).
//
// The runs endpoint is paginated via the Link header: pages of pipelinePerPageCap
// (100) are followed until top runs are collected or the pages run out. A
// top <= 0 defaults to a single page (pipelinePerPageCap).
//
// opts.Statuses is mapped to the GitHub ?status= query parameter. Because the
// runs endpoint only accepts a single status value, the param is added only when
// opts.Statuses contains exactly one RunStatus with a clean 1:1 mapping (see
// mapRunStatusParam). Empty, multi-element, or non-mapping sets omit the param
// and return unfiltered results.
func (c *Client) ListPipelineRuns(top int, opts provider.ListOpts) ([]WorkflowRun, error) {
	if top <= 0 {
		top = pipelinePerPageCap
	}

	path := fmt.Sprintf("/repos/%s/%s/actions/runs?per_page=%d", c.owner, c.repo, pipelinePerPageCap)

	// Add ?status= only when a single RunStatus with a clean mapping is requested.
	// Multiple statuses cannot be expressed in one status= param, so they are
	// intentionally left unfiltered (the adapter post-filters if needed).
	if len(opts.Statuses) == 1 {
		if param := mapRunStatusParam(opts.Statuses[0]); param != "" {
			path += "&status=" + param
		}
	}

	runs, err := getAllPages(c, path, top, func(body []byte) ([]WorkflowRun, error) {
		var envelope workflowRunsResponse
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("github: decode response: %w", err)
		}
		return envelope.WorkflowRuns, nil
	})
	if err != nil {
		return nil, fmt.Errorf("github: list pipeline runs: %w", err)
	}
	return runs, nil
}

// GetBuildTimeline fetches both the workflow run and its jobs from GitHub
// Actions and returns them as wire types for the adapter to map.
//
// The adapter calls MapTimeline(run, jobs, scope, scopeDisplay) (see
// internal/github/mapping_pipeline.go) to produce a provider.Timeline.
// Returning the wire pair rather than building the provider.Timeline here keeps
// the mapping boundary in one place (the adapter) and mirrors the azdevops
// layering convention.
//
// Two sequential GETs are issued:
//
//  1. GET /repos/{owner}/{repo}/actions/runs/{runID}            → WorkflowRun
//  2. GET /repos/{owner}/{repo}/actions/runs/{runID}/jobs       → JobsResponse → Jobs
//
// If either fetch fails the error is wrapped with %w and zero values are
// returned (zero WorkflowRun, nil Jobs).
func (c *Client) GetBuildTimeline(runID int) (WorkflowRun, []Job, error) {
	runPath := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", c.owner, c.repo, runID)
	var run WorkflowRun
	if err := c.getJSON(runPath, &run); err != nil {
		return WorkflowRun{}, nil, fmt.Errorf("github: get build timeline (run): %w", err)
	}

	jobsPath := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=%d",
		c.owner, c.repo, runID, pipelinePerPageCap)
	var jobsResp JobsResponse
	if err := c.getJSON(jobsPath, &jobsResp); err != nil {
		return WorkflowRun{}, nil, fmt.Errorf("github: get build timeline (jobs): %w", err)
	}

	return run, jobsResp.Jobs, nil
}

// GetBuildLogContent fetches the plaintext log for a specific job.
//
// In the task-7 timeline mapping (MapTimeline in mapping_pipeline.go), a Job
// timeline record carries LogID == int(job.ID) and Step records carry LogID == 0.
// Therefore logID IS the GitHub job ID.
//
// Note: runID is accepted for signature parity with the azdevops equivalent
// GetBuildLogContent(buildID, logID) but is NOT sent to the endpoint; the
// GitHub job-log endpoint identifies the log by job ID (logID) alone:
//
//	GET /repos/{owner}/{repo}/actions/jobs/{logID}/logs
//
// This endpoint responds with a 302 redirect to a short-lived plaintext blob
// URL. Go's http.Client follows the redirect automatically. On a cross-host
// redirect the Authorization header is stripped by the standard library (as
// required by RFC 9110 §15.4.4), which is the expected behavior — the blob
// store does not accept the GitHub Bearer token. The existing get() helper
// handles this transparently.
//
// If logID == 0 the method returns an error immediately without making any
// HTTP request. LogID == 0 indicates a Step timeline record; steps share their
// parent job's log. Callers should use the parent Job's LogID instead.
func (c *Client) GetBuildLogContent(runID int, logID int) (string, error) {
	if logID == 0 {
		return "", fmt.Errorf("github: get build log content: logID == 0 indicates a Step record " +
			"(steps share their job's log); use the parent Job LogID instead")
	}

	// runID is accepted for signature parity but unused; the job-log endpoint
	// identifies the log by job ID (logID) alone.
	_ = runID

	path := fmt.Sprintf("/repos/%s/%s/actions/jobs/%d/logs", c.owner, c.repo, logID)
	body, err := c.get(path)
	if err != nil {
		return "", fmt.Errorf("github: get build log content: %w", err)
	}
	return string(body), nil
}
