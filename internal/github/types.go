package github

import (
	"encoding/json"
	"time"
)

// User represents a GitHub user in wire responses. It appears as the author,
// assignee, or reviewer in issue, PR, review, and comment payloads.
type User struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

// Label represents a GitHub issue/PR label in wire responses.
type Label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// Milestone represents a GitHub milestone wire type embedded in issue payloads.
// Number is the milestone number; Title is the human-readable name.
type Milestone struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
}

// Issue represents a GitHub REST issue wire type
// (GET /repos/{owner}/{repo}/issues/{number}).
// StateReason is null when the issue is open or when the API omits it (legacy
// issues); use a pointer so the mapper can distinguish null from empty string.
// Assignee is null when no one is assigned.
// ClosedAt is null while the issue is open.
// Milestone is null when no milestone is assigned.
//
// PullRequest is non-nil when this "issue" is actually a pull request. The
// GET /repos/{owner}/{repo}/issues endpoint returns both issues and PRs mixed
// together — every PR is also an "issue" in GitHub's data model. Callers that
// want only real issues MUST skip any entry where PullRequest is non-nil.
// The field is decoded as a raw JSON value so that the presence/absence of the
// "pull_request" key is detectable without importing the full PR wire type.
type Issue struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	StateReason *string    `json:"state_reason"`
	User        User       `json:"user"`
	Assignee    *User      `json:"assignee"`
	Labels      []Label    `json:"labels"`
	Milestone   *Milestone `json:"milestone"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	HTMLURL     string     `json:"html_url"`
	// PullRequest is non-nil when this object is a pull request masquerading as
	// an issue. Filter out any Issue where PullRequest != nil.
	PullRequest *json.RawMessage `json:"pull_request"`
}

// IssueComment represents a single comment on a GitHub issue
// (GET /repos/{owner}/{repo}/issues/{number}/comments).
// Body is the raw Markdown text. User is the comment author.
// UpdatedAt differs from CreatedAt only when the comment has been edited.
// HTMLURL is the permalink to the comment on github.com.
type IssueComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HTMLURL   string    `json:"html_url"`
}

// PullRequestBranch holds the branch reference within a pull request.
type PullRequestBranch struct {
	Ref string `json:"ref"`
}

// PullRequest represents a GitHub REST pull request wire type
// (GET /repos/{owner}/{repo}/pulls/{number}).
// ClosedAt and MergedAt are null while the PR is open.
// RequestedReviewers lists reviewers who have been requested but have not yet
// submitted a review; the reviews endpoint only returns those who already acted.
type PullRequest struct {
	Number             int               `json:"number"`
	Title              string            `json:"title"`
	Body               string            `json:"body"`
	State              string            `json:"state"`
	Draft              bool              `json:"draft"`
	User               User              `json:"user"`
	RequestedReviewers []User            `json:"requested_reviewers"`
	Head               PullRequestBranch `json:"head"`
	Base               PullRequestBranch `json:"base"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	ClosedAt           *time.Time        `json:"closed_at"`
	MergedAt           *time.Time        `json:"merged_at"`
	HTMLURL            string            `json:"html_url"`
}

// Review represents a GitHub REST pull request review wire type
// (GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews).
// State is one of: APPROVED, CHANGES_REQUESTED, COMMENTED, PENDING, DISMISSED.
type Review struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"`
	User        User      `json:"user"`
	Body        string    `json:"body"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// ReviewComment represents a GitHub REST pull request review comment wire type
// (GET /repos/{owner}/{repo}/pulls/{pull_number}/comments).
// InReplyToID is null for the first (root) comment in a thread.
// Line is null for some legacy comments not anchored to a specific line;
// OriginalLine carries the anchor position when Line is null (outdated diff).
// HTMLURL is the permalink to the comment on github.com.
type ReviewComment struct {
	ID           int64     `json:"id"`
	InReplyToID  *int64    `json:"in_reply_to_id"`
	Path         string    `json:"path"`
	Line         *int      `json:"line"`
	OriginalLine *int      `json:"original_line"`
	Body         string    `json:"body"`
	User         User      `json:"user"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	HTMLURL      string    `json:"html_url"`
}

// WorkflowRun represents a GitHub Actions workflow run wire type
// (GET /repos/{owner}/{repo}/actions/runs/{run_id}).
// Conclusion is null while the run has not yet completed.
// RunStartedAt is null until the run leaves the queue; prefer it over CreatedAt
// (queue time) as the pipeline start time.
// HeadSHA is the commit SHA that triggered the run.
type WorkflowRun struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	Conclusion   *string    `json:"conclusion"`
	RunNumber    int        `json:"run_number"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RunStartedAt *time.Time `json:"run_started_at"`
	HTMLURL      string     `json:"html_url"`
}

// Job represents a GitHub Actions workflow job wire type
// (GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs, inside JobsResponse).
// Conclusion is null while the job is running.
// StartedAt / CompletedAt are null for queued or not-yet-started jobs.
type Job struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Steps       []Step     `json:"steps"`
}

// JobsResponse is the envelope returned by
// GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs.
type JobsResponse struct {
	TotalCount int   `json:"total_count"`
	Jobs       []Job `json:"jobs"`
}

// Step represents a single step within a GitHub Actions job.
// Conclusion is null while the step is running.
// StartedAt / CompletedAt are null for steps that have not started yet.
type Step struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion"`
	Number      int        `json:"number"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// PRFile represents a changed file in a pull request
// (GET /repos/{owner}/{repo}/pulls/{pull_number}/files).
// PreviousFilename is non-empty only on renames.
type PRFile struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"` // "added", "removed", "modified", "renamed", "copied", "changed", "unchanged"
	PreviousFilename string `json:"previous_filename,omitempty"`
	Changes          int    `json:"changes"`
}
