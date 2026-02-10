package azdevops

import (
	"strings"
	"time"
)

// Timeline represents a build timeline containing stages, jobs, and tasks
type Timeline struct {
	ID           string           `json:"id"`
	ChangeID     int              `json:"changeId"`
	LastChangedBy string          `json:"lastChangedBy"`
	LastChangedOn *time.Time      `json:"lastChangedOn"`
	Records      []TimelineRecord `json:"records"`
}

// TimelineRecord represents a single record in the timeline (stage, job, or task)
type TimelineRecord struct {
	ID         string        `json:"id"`
	ParentID   *string       `json:"parentId"`
	Type       string        `json:"type"`   // "Stage", "Job", "Task", "Phase", "Checkpoint"
	Name       string        `json:"name"`
	State      string        `json:"state"`  // "pending", "inProgress", "completed"
	Result     string        `json:"result"` // "succeeded", "succeededWithIssues", "failed", "canceled", "skipped", "abandoned"
	Order      int           `json:"order"`
	StartTime  *time.Time    `json:"startTime"`
	FinishTime *time.Time    `json:"finishTime"`
	Log        *LogReference `json:"log"`
	Issues     []Issue       `json:"issues"`
}

// LogReference contains metadata about a build log
type LogReference struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Issue represents an error or warning in a timeline record
type Issue struct {
	Type    string `json:"type"`    // "error", "warning"
	Message string `json:"message"`
}

// BuildLog represents metadata about a build log
type BuildLog struct {
	ID            int        `json:"id"`
	Type          string     `json:"type"`
	URL           string     `json:"url"`
	LineCount     int        `json:"lineCount"`
	CreatedOn     *time.Time `json:"createdOn"`
	LastChangedOn *time.Time `json:"lastChangedOn"`
}

// BuildLogsResponse represents the API response for listing build logs
type BuildLogsResponse struct {
	Count int        `json:"count"`
	Value []BuildLog `json:"value"`
}

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
