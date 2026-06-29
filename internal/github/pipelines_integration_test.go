//go:build integration

package github

import (
	"os"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Integration tests for the pipeline Client methods. These tests hit the real
// GitHub API and require a GITHUB_TOKEN environment variable. They are excluded
// from the default test run by the //go:build integration build tag.
//
// Run manually with:
//
//	CGO_ENABLED=0 GOCACHE=$PWD/.gocache TMPDIR=$PWD/.gotmp \
//	  GITHUB_TOKEN=<token> \
//	  go test -tags integration ./internal/github/... -v -run TestIntegrationPipeline
//
// NOTE: GetBuildLogContent is integration-only because the log endpoint
// (GET /actions/jobs/{id}/logs) issues a 302 redirect to a short-lived blob URL
// on a third-party host. The redirect is followed automatically by http.Client
// but requires a real GitHub token and live network access — not possible in the
// sandbox.

func integrationPipelineClient(t *testing.T) *Client {
	t.Helper()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set — skipping integration test")
	}
	// cli/cli is a known public repository with GitHub Actions runs.
	return NewClient("cli", "cli", token)
}

func TestIntegrationPipeline_ListPipelineRuns(t *testing.T) {
	c := integrationPipelineClient(t)

	runs, err := c.ListPipelineRuns(5, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	t.Logf("ListPipelineRuns: got %d runs", len(runs))
	for _, run := range runs {
		if run.ID == 0 {
			t.Error("WorkflowRun has zero ID")
		}
		if run.Name == "" {
			t.Error("WorkflowRun has empty Name")
		}
	}
}

func TestIntegrationPipeline_ListPipelineRuns_StatusFilter(t *testing.T) {
	c := integrationPipelineClient(t)

	// Filter for completed runs with a success conclusion (status=success).
	opts := provider.ListOpts{
		Statuses: []provider.RunStatus{provider.RunStatusSucceeded},
	}
	runs, err := c.ListPipelineRuns(5, opts)
	if err != nil {
		t.Fatalf("ListPipelineRuns(status=success) error = %v", err)
	}

	t.Logf("ListPipelineRuns(status=success): got %d runs", len(runs))
	// All returned runs should be completed with a success conclusion.
	for _, run := range runs {
		if run.Status != "completed" {
			t.Errorf("run %d: status = %q, want completed (status=success filter)", run.ID, run.Status)
		}
		if run.Conclusion == nil || *run.Conclusion != "success" {
			t.Errorf("run %d: conclusion = %v, want success", run.ID, run.Conclusion)
		}
	}
}

func TestIntegrationPipeline_GetBuildTimeline(t *testing.T) {
	c := integrationPipelineClient(t)

	// First, list a few runs to obtain a valid run ID.
	runs, err := c.ListPipelineRuns(3, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}
	if len(runs) == 0 {
		t.Skip("no runs found — skipping GetBuildTimeline integration test")
	}

	runID := int(runs[0].ID)
	t.Logf("GetBuildTimeline: using run ID %d", runID)

	run, jobs, err := c.GetBuildTimeline(runID)
	if err != nil {
		t.Fatalf("GetBuildTimeline(%d) error = %v", runID, err)
	}

	if run.ID != int64(runID) {
		t.Errorf("run.ID = %d, want %d", run.ID, runID)
	}
	t.Logf("GetBuildTimeline: run %d has %d job(s)", runID, len(jobs))

	for _, job := range jobs {
		if job.ID == 0 {
			t.Error("Job has zero ID")
		}
		if job.Name == "" {
			t.Error("Job has empty Name")
		}
		t.Logf("  job %d (%q): %d step(s)", job.ID, job.Name, len(job.Steps))
	}
}

func TestIntegrationPipeline_GetBuildLogContent(t *testing.T) {
	c := integrationPipelineClient(t)

	// List runs and pick the first completed one, then get its first job's log.
	opts := provider.ListOpts{
		Statuses: []provider.RunStatus{provider.RunStatusSucceeded},
	}
	runs, err := c.ListPipelineRuns(3, opts)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}
	if len(runs) == 0 {
		t.Skip("no completed runs found — skipping GetBuildLogContent integration test")
	}

	runID := int(runs[0].ID)
	_, jobs, err := c.GetBuildTimeline(runID)
	if err != nil {
		t.Fatalf("GetBuildTimeline(%d) error = %v", runID, err)
	}
	if len(jobs) == 0 {
		t.Skip("run has no jobs — skipping GetBuildLogContent integration test")
	}

	logID := int(jobs[0].ID)
	t.Logf("GetBuildLogContent: run %d, job %d", runID, logID)

	content, err := c.GetBuildLogContent(runID, logID)
	if err != nil {
		t.Fatalf("GetBuildLogContent(%d, %d) error = %v", runID, logID, err)
	}
	if content == "" {
		t.Error("GetBuildLogContent returned empty string — expected log output")
	}
	t.Logf("GetBuildLogContent: %d bytes returned", len(content))
}
