package github_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
)

// TestIssue_JSONRoundTrip verifies that the Issue wire type unmarshals key
// fields correctly, including the nullable assignee, state_reason, and milestone.
func TestIssue_JSONRoundTrip(t *testing.T) {
	const raw = `{
		"number": 42,
		"title": "Fix the regression",
		"body": "Details here.",
		"state": "closed",
		"state_reason": "completed",
		"user": {"login": "octocat", "id": 1},
		"assignee": null,
		"labels": [{"id": 10, "name": "bug", "color": "d73a4a", "description": "Something isn't working"}],
		"milestone": null,
		"created_at": "2026-01-01T10:00:00Z",
		"updated_at": "2026-01-02T11:00:00Z",
		"closed_at": "2026-01-02T12:00:00Z",
		"html_url": "https://github.com/owner/repo/issues/42"
	}`

	var issue github.Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if issue.Number != 42 {
		t.Errorf("Number = %d, want 42", issue.Number)
	}
	if issue.Title != "Fix the regression" {
		t.Errorf("Title = %q, want %q", issue.Title, "Fix the regression")
	}
	if issue.State != "closed" {
		t.Errorf("State = %q, want %q", issue.State, "closed")
	}
	if issue.StateReason == nil || *issue.StateReason != "completed" {
		t.Errorf("StateReason = %v, want *%q", issue.StateReason, "completed")
	}
	if issue.Assignee != nil {
		t.Errorf("Assignee = %v, want nil (JSON null)", issue.Assignee)
	}
	if len(issue.Labels) != 1 || issue.Labels[0].Name != "bug" {
		t.Errorf("Labels = %v, want one label named 'bug'", issue.Labels)
	}
	if issue.User.Login != "octocat" {
		t.Errorf("User.Login = %q, want %q", issue.User.Login, "octocat")
	}
	if issue.Milestone != nil {
		t.Errorf("Milestone = %v, want nil (JSON null)", issue.Milestone)
	}
	if issue.ClosedAt == nil {
		t.Errorf("ClosedAt = nil, want non-nil")
	}
	if issue.HTMLURL != "https://github.com/owner/repo/issues/42" {
		t.Errorf("HTMLURL = %q", issue.HTMLURL)
	}
}

// TestIssue_WithMilestone verifies that a non-null milestone decodes correctly.
func TestIssue_WithMilestone(t *testing.T) {
	const raw = `{
		"number": 5,
		"title": "Milestone issue",
		"state": "open",
		"state_reason": null,
		"user": {"login": "dev", "id": 2},
		"labels": [],
		"milestone": {"title": "v1.0", "number": 3},
		"created_at": "2026-06-01T00:00:00Z",
		"updated_at": "2026-06-01T00:00:00Z",
		"html_url": "https://github.com/owner/repo/issues/5"
	}`

	var issue github.Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if issue.Milestone == nil {
		t.Fatal("Milestone = nil, want non-nil")
	}
	if issue.Milestone.Title != "v1.0" {
		t.Errorf("Milestone.Title = %q, want %q", issue.Milestone.Title, "v1.0")
	}
	if issue.Milestone.Number != 3 {
		t.Errorf("Milestone.Number = %d, want 3", issue.Milestone.Number)
	}
}

// TestIssue_NullStateReason verifies that a null state_reason decodes to nil.
func TestIssue_NullStateReason(t *testing.T) {
	const raw = `{"number": 1, "title": "Open", "state": "open", "state_reason": null,
		"user": {"login": "u", "id": 2}, "labels": [],
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
		"html_url": "https://github.com/o/r/issues/1"}`

	var issue github.Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if issue.StateReason != nil {
		t.Errorf("StateReason = %v, want nil for JSON null", issue.StateReason)
	}
}

// TestPullRequest_JSONRoundTrip verifies nullable MergedAt and ClosedAt, and
// that RequestedReviewers decodes correctly.
func TestPullRequest_JSONRoundTrip(t *testing.T) {
	const raw = `{
		"number": 7,
		"title": "Add feature",
		"body": "Body text.",
		"state": "open",
		"draft": false,
		"user": {"login": "dev", "id": 5},
		"requested_reviewers": [{"login": "reviewer1", "id": 9}],
		"head": {"ref": "feature-branch"},
		"base": {"ref": "main"},
		"created_at": "2026-02-01T00:00:00Z",
		"updated_at": "2026-02-02T00:00:00Z",
		"closed_at": null,
		"merged_at": null,
		"html_url": "https://github.com/owner/repo/pull/7"
	}`

	var pr github.PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if pr.Number != 7 {
		t.Errorf("Number = %d, want 7", pr.Number)
	}
	if pr.Head.Ref != "feature-branch" {
		t.Errorf("Head.Ref = %q, want %q", pr.Head.Ref, "feature-branch")
	}
	if pr.Base.Ref != "main" {
		t.Errorf("Base.Ref = %q, want %q", pr.Base.Ref, "main")
	}
	if pr.MergedAt != nil {
		t.Errorf("MergedAt = %v, want nil", pr.MergedAt)
	}
	if pr.ClosedAt != nil {
		t.Errorf("ClosedAt = %v, want nil", pr.ClosedAt)
	}
	if pr.Draft {
		t.Error("Draft = true, want false")
	}
	if len(pr.RequestedReviewers) != 1 {
		t.Fatalf("len(RequestedReviewers) = %d, want 1", len(pr.RequestedReviewers))
	}
	if pr.RequestedReviewers[0].Login != "reviewer1" {
		t.Errorf("RequestedReviewers[0].Login = %q, want %q", pr.RequestedReviewers[0].Login, "reviewer1")
	}
	if pr.RequestedReviewers[0].ID != 9 {
		t.Errorf("RequestedReviewers[0].ID = %d, want 9", pr.RequestedReviewers[0].ID)
	}
}

// TestWorkflowRun_NullConclusion verifies that conclusion and run_started_at
// decode to nil while a run is in progress.
func TestWorkflowRun_NullConclusion(t *testing.T) {
	const raw = `{
		"id": 12345,
		"name": "CI",
		"status": "in_progress",
		"conclusion": null,
		"run_number": 10,
		"head_branch": "main",
		"head_sha": "abc123def456",
		"created_at": "2026-03-01T00:00:00Z",
		"updated_at": "2026-03-01T00:01:00Z",
		"run_started_at": null,
		"html_url": "https://github.com/owner/repo/actions/runs/12345"
	}`

	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if run.ID != 12345 {
		t.Errorf("ID = %d, want 12345", run.ID)
	}
	if run.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", run.Status, "in_progress")
	}
	if run.Conclusion != nil {
		t.Errorf("Conclusion = %v, want nil for in-progress run", run.Conclusion)
	}
	if run.RunNumber != 10 {
		t.Errorf("RunNumber = %d, want 10", run.RunNumber)
	}
	if run.HeadBranch != "main" {
		t.Errorf("HeadBranch = %q, want %q", run.HeadBranch, "main")
	}
	if run.HeadSHA != "abc123def456" {
		t.Errorf("HeadSHA = %q, want %q", run.HeadSHA, "abc123def456")
	}
	if run.RunStartedAt != nil {
		t.Errorf("RunStartedAt = %v, want nil", run.RunStartedAt)
	}
}

// TestWorkflowRun_Completed verifies that a non-null conclusion, run_started_at,
// and head_sha parse correctly.
func TestWorkflowRun_Completed(t *testing.T) {
	const raw = `{
		"id": 9,
		"name": "CI",
		"status": "completed",
		"conclusion": "success",
		"run_number": 9,
		"head_branch": "main",
		"head_sha": "deadbeef1234",
		"created_at": "2026-03-02T00:00:00Z",
		"updated_at": "2026-03-02T00:05:00Z",
		"run_started_at": "2026-03-02T00:00:05Z",
		"html_url": "https://github.com/owner/repo/actions/runs/9"
	}`

	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if run.Conclusion == nil {
		t.Fatal("Conclusion = nil, want non-nil for completed run")
	}
	if *run.Conclusion != "success" {
		t.Errorf("*Conclusion = %q, want %q", *run.Conclusion, "success")
	}
	if run.HeadSHA != "deadbeef1234" {
		t.Errorf("HeadSHA = %q, want %q", run.HeadSHA, "deadbeef1234")
	}
	if run.RunStartedAt == nil {
		t.Fatal("RunStartedAt = nil, want non-nil for completed run")
	}
	wantStart := time.Date(2026, 3, 2, 0, 0, 5, 0, time.UTC)
	if !run.RunStartedAt.Equal(wantStart) {
		t.Errorf("RunStartedAt = %v, want %v", run.RunStartedAt, wantStart)
	}
}

// TestJob_WithSteps verifies Job and Step embedding including null conclusion.
func TestJob_WithSteps(t *testing.T) {
	const raw = `{
		"id": 100,
		"name": "build",
		"status": "completed",
		"conclusion": "failure",
		"started_at": "2026-03-01T00:00:00Z",
		"completed_at": "2026-03-01T00:02:00Z",
		"steps": [
			{
				"name": "Checkout",
				"status": "completed",
				"conclusion": "success",
				"number": 1,
				"started_at": "2026-03-01T00:00:00Z",
				"completed_at": "2026-03-01T00:00:30Z"
			},
			{
				"name": "Build",
				"status": "completed",
				"conclusion": "failure",
				"number": 2,
				"started_at": "2026-03-01T00:00:30Z",
				"completed_at": "2026-03-01T00:02:00Z"
			}
		]
	}`

	var job github.Job
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if job.Name != "build" {
		t.Errorf("Name = %q, want %q", job.Name, "build")
	}
	if job.Conclusion == nil || *job.Conclusion != "failure" {
		t.Errorf("Conclusion = %v, want *%q", job.Conclusion, "failure")
	}
	if job.StartedAt == nil {
		t.Error("StartedAt = nil, want non-nil")
	}
	if len(job.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(job.Steps))
	}
	if job.Steps[0].Name != "Checkout" {
		t.Errorf("Steps[0].Name = %q, want %q", job.Steps[0].Name, "Checkout")
	}
	if job.Steps[0].Conclusion == nil || *job.Steps[0].Conclusion != "success" {
		t.Errorf("Steps[0].Conclusion = %v, want *%q", job.Steps[0].Conclusion, "success")
	}
	if job.Steps[1].Number != 2 {
		t.Errorf("Steps[1].Number = %d, want 2", job.Steps[1].Number)
	}
}

// TestReview_JSONRoundTrip verifies the Review wire type.
func TestReview_JSONRoundTrip(t *testing.T) {
	const raw = `{
		"id": 55,
		"state": "APPROVED",
		"user": {"login": "reviewer", "id": 3},
		"body": "",
		"submitted_at": "2026-04-01T09:00:00Z"
	}`

	var review github.Review
	if err := json.Unmarshal([]byte(raw), &review); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if review.ID != 55 {
		t.Errorf("ID = %d, want 55", review.ID)
	}
	if review.State != "APPROVED" {
		t.Errorf("State = %q, want %q", review.State, "APPROVED")
	}
	if review.User.Login != "reviewer" {
		t.Errorf("User.Login = %q, want %q", review.User.Login, "reviewer")
	}
	wantTime := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	if !review.SubmittedAt.Equal(wantTime) {
		t.Errorf("SubmittedAt = %v, want %v", review.SubmittedAt, wantTime)
	}
}

// TestReviewComment_JSONRoundTrip verifies nullable InReplyToID and Line, plus
// the new OriginalLine (null here) and HTMLURL fields.
func TestReviewComment_JSONRoundTrip(t *testing.T) {
	const raw = `{
		"id": 200,
		"in_reply_to_id": null,
		"path": "internal/foo/bar.go",
		"line": 42,
		"original_line": null,
		"body": "Nit: rename this.",
		"user": {"login": "nit-picker", "id": 7},
		"created_at": "2026-05-01T08:00:00Z",
		"updated_at": "2026-05-01T08:30:00Z",
		"html_url": "https://github.com/owner/repo/pull/7#discussion_r200"
	}`

	var rc github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if rc.ID != 200 {
		t.Errorf("ID = %d, want 200", rc.ID)
	}
	if rc.InReplyToID != nil {
		t.Errorf("InReplyToID = %v, want nil", rc.InReplyToID)
	}
	if rc.Path != "internal/foo/bar.go" {
		t.Errorf("Path = %q", rc.Path)
	}
	if rc.Line == nil || *rc.Line != 42 {
		t.Errorf("Line = %v, want *42", rc.Line)
	}
	if rc.OriginalLine != nil {
		t.Errorf("OriginalLine = %v, want nil (JSON null)", rc.OriginalLine)
	}
	if rc.Body != "Nit: rename this." {
		t.Errorf("Body = %q", rc.Body)
	}
	if rc.HTMLURL != "https://github.com/owner/repo/pull/7#discussion_r200" {
		t.Errorf("HTMLURL = %q", rc.HTMLURL)
	}
}

// TestReviewComment_OriginalLine verifies that a legacy comment with null line
// but non-null original_line decodes the anchor correctly.
func TestReviewComment_OriginalLine(t *testing.T) {
	const raw = `{
		"id": 202,
		"in_reply_to_id": null,
		"path": "internal/foo/bar.go",
		"line": null,
		"original_line": 17,
		"body": "Outdated comment.",
		"user": {"login": "reviewer", "id": 3},
		"created_at": "2026-05-02T10:00:00Z",
		"updated_at": "2026-05-02T10:00:00Z",
		"html_url": "https://github.com/owner/repo/pull/7#discussion_r202"
	}`

	var rc github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if rc.Line != nil {
		t.Errorf("Line = %v, want nil for legacy comment", rc.Line)
	}
	if rc.OriginalLine == nil {
		t.Fatal("OriginalLine = nil, want non-nil")
	}
	if *rc.OriginalLine != 17 {
		t.Errorf("*OriginalLine = %d, want 17", *rc.OriginalLine)
	}
	if rc.HTMLURL != "https://github.com/owner/repo/pull/7#discussion_r202" {
		t.Errorf("HTMLURL = %q", rc.HTMLURL)
	}
}

// TestReviewComment_WithReplyID verifies that a non-null InReplyToID parses.
func TestReviewComment_WithReplyID(t *testing.T) {
	const raw = `{
		"id": 201,
		"in_reply_to_id": 200,
		"path": "internal/foo/bar.go",
		"line": 42,
		"body": "Agreed.",
		"user": {"login": "author", "id": 1},
		"created_at": "2026-05-01T09:00:00Z",
		"updated_at": "2026-05-01T09:00:00Z",
		"html_url": "https://github.com/owner/repo/pull/7#discussion_r201"
	}`

	var rc github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if rc.InReplyToID == nil {
		t.Fatal("InReplyToID = nil, want non-nil")
	}
	if *rc.InReplyToID != 200 {
		t.Errorf("*InReplyToID = %d, want 200", *rc.InReplyToID)
	}
}
