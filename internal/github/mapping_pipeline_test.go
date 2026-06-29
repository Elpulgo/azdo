package github_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// ── MapPipelineRun ────────────────────────────────────────────────────────────

func TestMapPipelineRun_Completed_Success(t *testing.T) {
	const raw = `{
		"id": 123456789,
		"name": "CI Pipeline",
		"status": "completed",
		"conclusion": "success",
		"run_number": 42,
		"head_branch": "main",
		"head_sha": "abc1234def5678",
		"created_at": "2026-06-01T10:00:00Z",
		"updated_at": "2026-06-01T10:15:00Z",
		"run_started_at": "2026-06-01T10:01:00Z",
		"html_url": "https://github.com/octo/repo/actions/runs/123456789"
	}`

	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPipelineRun(run, testScope, testScopeDisplay)

	// Identity invariant
	assertIdentityInvariant(t, "PipelineRun(completed/success)", got.Identity)
	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Identity.ID != "123456789" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "123456789")
	}

	// Core fields
	if got.BuildNumber != "42" {
		t.Errorf("BuildNumber = %q, want %q", got.BuildNumber, "42")
	}
	if got.DefinitionName != "CI Pipeline" {
		t.Errorf("DefinitionName = %q, want %q", got.DefinitionName, "CI Pipeline")
	}
	if got.DefinitionID != 0 {
		t.Errorf("DefinitionID = %d, want 0 (workflow_id not on wire type)", got.DefinitionID)
	}
	if got.SourceBranch != "main" {
		t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, "main")
	}
	if got.SourceVersion != "abc1234def5678" {
		t.Errorf("SourceVersion = %q, want %q", got.SourceVersion, "abc1234def5678")
	}
	if got.WebURL != "https://github.com/octo/repo/actions/runs/123456789" {
		t.Errorf("WebURL = %q", got.WebURL)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
	if got.Result != "success" {
		t.Errorf("Result = %q, want %q", got.Result, "success")
	}

	// RunStatus enum
	if got.RunStatus != provider.RunStatusSucceeded {
		t.Errorf("RunStatus = %v, want RunStatusSucceeded", got.RunStatus)
	}

	// QueueTime from created_at
	wantQueue := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	if !got.QueueTime.Equal(wantQueue) {
		t.Errorf("QueueTime = %v, want %v", got.QueueTime, wantQueue)
	}

	// StartTime from run_started_at
	if got.StartTime == nil {
		t.Fatal("StartTime is nil, want non-nil")
	}
	wantStart := time.Date(2026, 6, 1, 10, 1, 0, 0, time.UTC)
	if !got.StartTime.Equal(wantStart) {
		t.Errorf("StartTime = %v, want %v", *got.StartTime, wantStart)
	}

	// FinishTime approximated from updated_at when completed
	if got.FinishTime == nil {
		t.Fatal("FinishTime is nil, want non-nil for completed run")
	}
	wantFinish := time.Date(2026, 6, 1, 10, 15, 0, 0, time.UTC)
	if !got.FinishTime.Equal(wantFinish) {
		t.Errorf("FinishTime = %v, want %v", *got.FinishTime, wantFinish)
	}
}

func TestMapPipelineRun_InProgress(t *testing.T) {
	const raw = `{
		"id": 999000001,
		"name": "Deploy to Staging",
		"status": "in_progress",
		"conclusion": null,
		"run_number": 7,
		"head_branch": "feature/new-ui",
		"head_sha": "deadbeef",
		"created_at": "2026-06-02T08:00:00Z",
		"updated_at": "2026-06-02T08:02:00Z",
		"run_started_at": "2026-06-02T08:01:00Z",
		"html_url": "https://github.com/octo/repo/actions/runs/999000001"
	}`

	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPipelineRun(run, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "PipelineRun(in_progress)", got.Identity)
	if got.Identity.ID != "999000001" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "999000001")
	}
	if got.BuildNumber != "7" {
		t.Errorf("BuildNumber = %q, want %q", got.BuildNumber, "7")
	}

	// Conclusion is null → Result is ""
	if got.Result != "" {
		t.Errorf("Result = %q, want empty string (null conclusion)", got.Result)
	}
	if got.RunStatus != provider.RunStatusRunning {
		t.Errorf("RunStatus = %v, want RunStatusRunning", got.RunStatus)
	}

	// StartTime from run_started_at
	if got.StartTime == nil {
		t.Fatal("StartTime is nil, want non-nil")
	}
	wantStart := time.Date(2026, 6, 2, 8, 1, 0, 0, time.UTC)
	if !got.StartTime.Equal(wantStart) {
		t.Errorf("StartTime = %v, want %v", *got.StartTime, wantStart)
	}

	// FinishTime must be nil for non-completed runs
	if got.FinishTime != nil {
		t.Errorf("FinishTime = %v, want nil for in-progress run", *got.FinishTime)
	}
}

func TestMapPipelineRun_Queued(t *testing.T) {
	const raw = `{
		"id": 777000002,
		"name": "Nightly Build",
		"status": "queued",
		"conclusion": null,
		"run_number": 100,
		"head_branch": "main",
		"head_sha": "cafebabe",
		"created_at": "2026-06-03T00:00:00Z",
		"updated_at": "2026-06-03T00:00:05Z",
		"run_started_at": null,
		"html_url": "https://github.com/octo/repo/actions/runs/777000002"
	}`

	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPipelineRun(run, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "PipelineRun(queued)", got.Identity)
	if got.RunStatus != provider.RunStatusQueued {
		t.Errorf("RunStatus = %v, want RunStatusQueued", got.RunStatus)
	}

	// run_started_at is null → StartTime nil
	if got.StartTime != nil {
		t.Errorf("StartTime = %v, want nil for queued run", *got.StartTime)
	}
	// FinishTime must be nil for non-completed runs
	if got.FinishTime != nil {
		t.Errorf("FinishTime = %v, want nil for queued run", *got.FinishTime)
	}

	if got.SourceBranch != "main" {
		t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, "main")
	}
	if got.BuildNumber != "100" {
		t.Errorf("BuildNumber = %q, want %q", got.BuildNumber, "100")
	}
}

// ── MapTimeline ───────────────────────────────────────────────────────────────

// timelineFixtureRun is the WorkflowRun embedded in the MapTimeline fixture.
const timelineFixtureRunJSON = `{
	"id": 500000001,
	"name": "Test Pipeline",
	"status": "completed",
	"conclusion": "failure",
	"run_number": 5,
	"head_branch": "fix/bug",
	"head_sha": "f00df00d",
	"created_at": "2026-06-10T09:00:00Z",
	"updated_at": "2026-06-10T09:30:00Z",
	"run_started_at": "2026-06-10T09:01:00Z",
	"html_url": "https://github.com/octo/repo/actions/runs/500000001"
}`

// Two jobs, each with steps (including a failed step in job 1 and a skipped step in job 2).
const timelineFixtureJobsJSON = `[
	{
		"id": 100000001,
		"name": "Build",
		"status": "completed",
		"conclusion": "failure",
		"started_at": "2026-06-10T09:02:00Z",
		"completed_at": "2026-06-10T09:15:00Z",
		"steps": [
			{
				"name": "Checkout",
				"status": "completed",
				"conclusion": "success",
				"number": 1,
				"started_at": "2026-06-10T09:02:05Z",
				"completed_at": "2026-06-10T09:02:20Z"
			},
			{
				"name": "Build binary",
				"status": "completed",
				"conclusion": "failure",
				"number": 2,
				"started_at": "2026-06-10T09:02:25Z",
				"completed_at": "2026-06-10T09:15:00Z"
			},
			{
				"name": "Upload artifact",
				"status": "completed",
				"conclusion": "skipped",
				"number": 3,
				"started_at": null,
				"completed_at": null
			}
		]
	},
	{
		"id": 100000002,
		"name": "Deploy",
		"status": "completed",
		"conclusion": "skipped",
		"started_at": null,
		"completed_at": null,
		"steps": [
			{
				"name": "Deploy to production",
				"status": "completed",
				"conclusion": "skipped",
				"number": 1,
				"started_at": null,
				"completed_at": null
			},
			{
				"name": "Notify Slack",
				"status": "completed",
				"conclusion": "skipped",
				"number": 2,
				"started_at": null,
				"completed_at": null
			}
		]
	}
]`

func TestMapTimeline_TwoJobsWithSteps(t *testing.T) {
	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(timelineFixtureRunJSON), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}

	var jobs []github.Job
	if err := json.Unmarshal([]byte(timelineFixtureJobsJSON), &jobs); err != nil {
		t.Fatalf("unmarshal jobs: %v", err)
	}

	got := github.MapTimeline(run, jobs, testScope, testScopeDisplay)

	// Identity
	assertIdentityInvariant(t, "Timeline", got.Identity)
	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Identity.ID != "500000001" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "500000001")
	}

	// Flat record count: 2 jobs + (3 steps + 2 steps) = 7 records
	if len(got.Records) != 7 {
		t.Fatalf("len(Records) = %d, want 7", len(got.Records))
	}

	// Collect records by type for easier assertions.
	var jobRecs, taskRecs []provider.TimelineRecord
	for _, r := range got.Records {
		switch r.Type {
		case "Job":
			jobRecs = append(jobRecs, r)
		case "Task":
			taskRecs = append(taskRecs, r)
		default:
			t.Errorf("unexpected record Type %q", r.Type)
		}
	}

	if len(jobRecs) != 2 {
		t.Fatalf("job record count = %d, want 2", len(jobRecs))
	}
	if len(taskRecs) != 5 {
		t.Fatalf("task record count = %d, want 5", len(taskRecs))
	}

	// ── Job record invariants ─────────────────────────────────────────────────

	for _, jr := range jobRecs {
		if jr.ParentID != "" {
			t.Errorf("job %q: ParentID = %q, want empty", jr.ID, jr.ParentID)
		}
		if jr.Type != "Job" {
			t.Errorf("job %q: Type = %q, want %q", jr.ID, jr.Type, "Job")
		}
		if jr.LogID == 0 {
			t.Errorf("job %q: LogID = 0, want int(job.ID)", jr.ID)
		}
		if jr.Order == 0 {
			t.Errorf("job %q: Order = 0, want > 0", jr.ID)
		}
	}

	// First job: Build (failed)
	buildRec := jobRecs[0]
	if buildRec.ID != "100000001" {
		t.Errorf("build job ID = %q, want %q", buildRec.ID, "100000001")
	}
	if buildRec.LogID != 100000001 {
		t.Errorf("build job LogID = %d, want 100000001", buildRec.LogID)
	}
	if buildRec.State != "completed" {
		t.Errorf("build job State = %q, want %q", buildRec.State, "completed")
	}
	if buildRec.Result != "failed" {
		t.Errorf("build job Result = %q, want %q", buildRec.Result, "failed")
	}
	if buildRec.Order != 1 {
		t.Errorf("build job Order = %d, want 1", buildRec.Order)
	}
	// StartTime and FinishTime should be set
	if buildRec.StartTime == nil {
		t.Errorf("build job StartTime is nil, want non-nil")
	}
	if buildRec.FinishTime == nil {
		t.Errorf("build job FinishTime is nil, want non-nil")
	}

	// Second job: Deploy (skipped → no times)
	deployRec := jobRecs[1]
	if deployRec.ID != "100000002" {
		t.Errorf("deploy job ID = %q, want %q", deployRec.ID, "100000002")
	}
	if deployRec.LogID != 100000002 {
		t.Errorf("deploy job LogID = %d, want 100000002", deployRec.LogID)
	}
	if deployRec.Result != "skipped" {
		t.Errorf("deploy job Result = %q, want %q", deployRec.Result, "skipped")
	}
	if deployRec.Order != 2 {
		t.Errorf("deploy job Order = %d, want 2", deployRec.Order)
	}
	if deployRec.StartTime != nil {
		t.Errorf("deploy job StartTime = %v, want nil (skipped)", *deployRec.StartTime)
	}
	if deployRec.FinishTime != nil {
		t.Errorf("deploy job FinishTime = %v, want nil (skipped)", *deployRec.FinishTime)
	}

	// ── Step record invariants ────────────────────────────────────────────────

	for _, tr := range taskRecs {
		if tr.Type != "Task" {
			t.Errorf("step %q: Type = %q, want %q", tr.ID, tr.Type, "Task")
		}
		if tr.ParentID == "" {
			t.Errorf("step %q: ParentID is empty, want job ID", tr.ID)
		}
		if tr.LogID != 0 {
			t.Errorf("step %q: LogID = %d, want 0 (steps share job log)", tr.ID, tr.LogID)
		}
		if tr.Order == 0 {
			t.Errorf("step %q: Order = 0, want > 0", tr.ID)
		}
	}

	// Build the step map for targeted assertions.
	stepByID := make(map[string]provider.TimelineRecord, len(taskRecs))
	for _, tr := range taskRecs {
		stepByID[tr.ID] = tr
	}

	// Step IDs follow the "jobID-stepNumber" pattern.
	checkoutStep := stepByID["100000001-1"]
	if checkoutStep.ParentID != "100000001" {
		t.Errorf("checkout step ParentID = %q, want %q", checkoutStep.ParentID, "100000001")
	}
	if checkoutStep.Name != "Checkout" {
		t.Errorf("checkout step Name = %q, want %q", checkoutStep.Name, "Checkout")
	}
	if checkoutStep.Result != "succeeded" {
		t.Errorf("checkout step Result = %q, want %q", checkoutStep.Result, "succeeded")
	}
	if checkoutStep.State != "completed" {
		t.Errorf("checkout step State = %q, want %q", checkoutStep.State, "completed")
	}
	if checkoutStep.Order != 1 {
		t.Errorf("checkout step Order = %d, want 1", checkoutStep.Order)
	}

	// Failed step
	buildStep := stepByID["100000001-2"]
	if buildStep.Result != "failed" {
		t.Errorf("build step Result = %q, want %q", buildStep.Result, "failed")
	}

	// Skipped step within the Build job
	skipStep := stepByID["100000001-3"]
	if skipStep.Result != "skipped" {
		t.Errorf("skipped step Result = %q, want %q", skipStep.Result, "skipped")
	}
	if skipStep.StartTime != nil {
		t.Errorf("skipped step StartTime = %v, want nil", *skipStep.StartTime)
	}
	if skipStep.FinishTime != nil {
		t.Errorf("skipped step FinishTime = %v, want nil", *skipStep.FinishTime)
	}

	// Step IDs are unique across the entire slice.
	seen := make(map[string]bool)
	for _, r := range got.Records {
		if seen[r.ID] {
			t.Errorf("duplicate record ID %q", r.ID)
		}
		seen[r.ID] = true
	}

	// Steps in Deploy job have ParentID == "100000002"
	deployStep1 := stepByID["100000002-1"]
	if deployStep1.ParentID != "100000002" {
		t.Errorf("deploy step1 ParentID = %q, want %q", deployStep1.ParentID, "100000002")
	}
}

func TestMapTimeline_EmptyJobs(t *testing.T) {
	var run github.WorkflowRun
	if err := json.Unmarshal([]byte(timelineFixtureRunJSON), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}

	got := github.MapTimeline(run, []github.Job{}, testScope, testScopeDisplay)

	assertIdentityInvariant(t, "Timeline(empty)", got.Identity)
	if len(got.Records) != 0 {
		t.Errorf("Records len = %d, want 0 for empty jobs", len(got.Records))
	}
}

// ── mapTimelineState (table tests) ───────────────────────────────────────────

func TestMapTimelineState(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"in_progress", "inProgress"},
		{"IN_PROGRESS", "inProgress"}, // case-insensitive
		{"completed", "completed"},
		{"COMPLETED", "completed"},
		{"queued", "pending"},
		{"waiting", "pending"},
		{"pending", "pending"},
		{"requested", "pending"},
		{"", "pending"},        // unknown → pending
		{"unknown", "pending"}, // unknown → pending
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("input=%q", tc.input), func(t *testing.T) {
			got := github.MapTimelineStateExported(tc.input)
			if got != tc.want {
				t.Errorf("mapTimelineState(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── mapTimelineResult (table tests) ──────────────────────────────────────────

func TestMapTimelineResult(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"success", "succeeded"},
		{"SUCCESS", "succeeded"}, // case-insensitive
		{"failure", "failed"},
		{"timed_out", "failed"},
		{"startup_failure", "failed"},
		{"cancelled", "canceled"}, // GitHub's double-L → view's single-L
		{"CANCELLED", "canceled"},
		{"skipped", "skipped"},
		{"neutral", "succeededwithissues"},
		{"action_required", ""},
		{"stale", ""},
		{"", ""}, // null conclusion → empty → default muted glyph
		{"unknown_future_value", ""},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("input=%q", tc.input), func(t *testing.T) {
			got := github.MapTimelineResultExported(tc.input)
			if got != tc.want {
				t.Errorf("mapTimelineResult(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
