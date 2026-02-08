package azdevops

import (
	"strings"
	"time"
)

// PipelineRun represents a build/pipeline run in Azure DevOps
type PipelineRun struct {
	ID            int                `json:"id"`
	BuildNumber   string             `json:"buildNumber"`
	Status        string             `json:"status"`        // "inProgress", "completed", "canceling", "postponed", "notStarted"
	Result        string             `json:"result"`        // "succeeded", "failed", "canceled", "partiallySucceeded", "none"
	SourceBranch  string             `json:"sourceBranch"`  // e.g., "refs/heads/main"
	SourceVersion string             `json:"sourceVersion"` // Git commit SHA
	QueueTime     time.Time          `json:"queueTime"`
	StartTime     *time.Time         `json:"startTime"`
	FinishTime    *time.Time         `json:"finishTime"`
	Definition    PipelineDefinition `json:"definition"`
	Project       Project            `json:"project"`
	Links         Links              `json:"_links"`
}

// PipelineDefinition represents a pipeline definition
type PipelineDefinition struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Project represents an Azure DevOps project
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Links contains HATEOAS links
type Links struct {
	Web Link `json:"web"`
}

// Link represents a single HATEOAS link
type Link struct {
	Href string `json:"href"`
}

// PipelineRunsResponse represents the API response for listing pipeline runs
type PipelineRunsResponse struct {
	Count int           `json:"count"`
	Value []PipelineRun `json:"value"`
}

// BranchShortName returns the short branch name without the refs/heads/ prefix
func (pr *PipelineRun) BranchShortName() string {
	if pr.SourceBranch == "" {
		return ""
	}

	// Remove refs/heads/ prefix
	if strings.HasPrefix(pr.SourceBranch, "refs/heads/") {
		return strings.TrimPrefix(pr.SourceBranch, "refs/heads/")
	}

	// Remove refs/tags/ prefix
	if strings.HasPrefix(pr.SourceBranch, "refs/tags/") {
		return strings.TrimPrefix(pr.SourceBranch, "refs/tags/")
	}

	return pr.SourceBranch
}

// Duration returns a human-readable duration string for the pipeline run
func (pr *PipelineRun) Duration() string {
	// If not started, return dash
	if pr.StartTime == nil {
		return "-"
	}

	// If in progress (no finish time), return dash for now
	// In a real UI, we might calculate elapsed time since start
	if pr.FinishTime == nil {
		return "-"
	}

	duration := pr.FinishTime.Sub(*pr.StartTime)
	return duration.String()
}
