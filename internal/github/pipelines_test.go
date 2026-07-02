package github

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// ListPipelineRuns
// ---------------------------------------------------------------------------

func TestClient_ListPipelineRuns_PathAndEnvelopeUnwrap(t *testing.T) {
	// Fixture: envelope with two workflow_runs.
	const fixture = `{
		"total_count": 2,
		"workflow_runs": [
			{
				"id": 1001,
				"name": "CI",
				"status": "completed",
				"conclusion": "success",
				"run_number": 42,
				"head_branch": "main",
				"head_sha": "abc123",
				"created_at": "2024-06-01T10:00:00Z",
				"updated_at": "2024-06-01T10:30:00Z",
				"html_url": "https://github.com/o/r/actions/runs/1001"
			},
			{
				"id": 1002,
				"name": "CI",
				"status": "in_progress",
				"conclusion": null,
				"run_number": 43,
				"head_branch": "feature/x",
				"head_sha": "def456",
				"created_at": "2024-06-02T09:00:00Z",
				"updated_at": "2024-06-02T09:15:00Z",
				"html_url": "https://github.com/o/r/actions/runs/1002"
			}
		]
	}`

	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	runs, err := c.ListPipelineRuns(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	// Assert correct endpoint path.
	if capturedPath != "/repos/o/r/actions/runs" {
		t.Errorf("path = %q, want /repos/o/r/actions/runs", capturedPath)
	}

	// Assert the envelope is unwrapped — both runs returned.
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].ID != 1001 {
		t.Errorf("runs[0].ID = %d, want 1001", runs[0].ID)
	}
	if runs[0].Name != "CI" {
		t.Errorf("runs[0].Name = %q, want CI", runs[0].Name)
	}
	if runs[0].Status != "completed" {
		t.Errorf("runs[0].Status = %q, want completed", runs[0].Status)
	}
	if runs[1].ID != 1002 {
		t.Errorf("runs[1].ID = %d, want 1002", runs[1].ID)
	}
	if runs[1].Status != "in_progress" {
		t.Errorf("runs[1].Status = %q, want in_progress", runs[1].Status)
	}
	// Conclusion is null for in-progress run — pointer must be nil.
	if runs[1].Conclusion != nil {
		t.Errorf("runs[1].Conclusion = %v, want nil for in-progress run", runs[1].Conclusion)
	}
}

func TestClient_ListPipelineRuns_TopCappedAt100(t *testing.T) {
	var capturedPerPage string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPerPage = r.URL.Query().Get("per_page")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.ListPipelineRuns(9999, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if capturedPerPage != "100" {
		t.Errorf("per_page = %q, want 100 (capped from 9999)", capturedPerPage)
	}
}

func TestClient_ListPipelineRuns_SingleStatusMapped_Succeeded(t *testing.T) {
	var capturedStatus string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStatus = r.URL.Query().Get("status")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		Statuses: []provider.RunStatus{provider.RunStatusSucceeded},
	}
	_, err := c.ListPipelineRuns(10, opts)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if capturedStatus != "success" {
		t.Errorf("status param = %q, want %q for RunStatusSucceeded", capturedStatus, "success")
	}
}

func TestClient_ListPipelineRuns_SingleStatusMapped_Running(t *testing.T) {
	var capturedStatus string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStatus = r.URL.Query().Get("status")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	opts := provider.ListOpts{
		Statuses: []provider.RunStatus{provider.RunStatusRunning},
	}
	_, err := c.ListPipelineRuns(10, opts)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if capturedStatus != "in_progress" {
		t.Errorf("status param = %q, want in_progress for RunStatusRunning", capturedStatus)
	}
}

func TestClient_ListPipelineRuns_EmptyStatuses_NoStatusParam(t *testing.T) {
	var capturedRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// Empty Statuses → no status param.
	_, err := c.ListPipelineRuns(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if strings.Contains(capturedRawQuery, "status=") {
		t.Errorf("raw query = %q: status param must be omitted for empty Statuses", capturedRawQuery)
	}
}

func TestClient_ListPipelineRuns_MultipleStatuses_NoStatusParam(t *testing.T) {
	var capturedRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// Multiple statuses cannot be expressed in one status= param → omit it.
	opts := provider.ListOpts{
		Statuses: []provider.RunStatus{
			provider.RunStatusSucceeded,
			provider.RunStatusFailed,
		},
	}
	_, err := c.ListPipelineRuns(10, opts)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if strings.Contains(capturedRawQuery, "status=") {
		t.Errorf("raw query = %q: status param must be omitted for multiple Statuses", capturedRawQuery)
	}
}

func TestClient_ListPipelineRuns_UnmappableStatus_NoStatusParam(t *testing.T) {
	var capturedRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// RunStatusCanceling has no GitHub equivalent → omit status param.
	opts := provider.ListOpts{
		Statuses: []provider.RunStatus{provider.RunStatusCanceling},
	}
	_, err := c.ListPipelineRuns(10, opts)
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}

	if strings.Contains(capturedRawQuery, "status=") {
		t.Errorf("raw query = %q: status param must be omitted for RunStatusCanceling (no GitHub equivalent)", capturedRawQuery)
	}
}

// ---------------------------------------------------------------------------
// GetBuildTimeline
// ---------------------------------------------------------------------------

func TestClient_GetBuildTimeline_TwoSequentialGETs(t *testing.T) {
	// Fixture for GET /repos/o/r/actions/runs/55.
	const runFixture = `{
		"id": 55,
		"name": "CI",
		"status": "completed",
		"conclusion": "success",
		"run_number": 10,
		"head_branch": "main",
		"head_sha": "sha1234",
		"created_at": "2024-05-01T08:00:00Z",
		"updated_at": "2024-05-01T08:45:00Z",
		"run_started_at": "2024-05-01T08:01:00Z",
		"html_url": "https://github.com/o/r/actions/runs/55"
	}`

	// Fixture for GET /repos/o/r/actions/runs/55/jobs.
	// Contains one job with two steps.
	const jobsFixture = `{
		"total_count": 1,
		"jobs": [
			{
				"id": 200,
				"name": "build",
				"status": "completed",
				"conclusion": "success",
				"started_at": "2024-05-01T08:02:00Z",
				"completed_at": "2024-05-01T08:40:00Z",
				"steps": [
					{
						"name": "Checkout",
						"status": "completed",
						"conclusion": "success",
						"number": 1,
						"started_at": "2024-05-01T08:02:30Z",
						"completed_at": "2024-05-01T08:03:00Z"
					},
					{
						"name": "Run tests",
						"status": "completed",
						"conclusion": "success",
						"number": 2,
						"started_at": "2024-05-01T08:03:01Z",
						"completed_at": "2024-05-01T08:40:00Z"
					}
				]
			}
		]
	}`

	callCount := 0
	var capturedPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		capturedPaths = append(capturedPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		switch callCount {
		case 1:
			// First GET: the run itself.
			w.Write([]byte(runFixture))
		case 2:
			// Second GET: the run's jobs.
			w.Write([]byte(jobsFixture))
		default:
			t.Errorf("unexpected call #%d to %s", callCount, r.URL.Path)
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	run, jobs, err := c.GetBuildTimeline(55)
	if err != nil {
		t.Fatalf("GetBuildTimeline(55) error = %v", err)
	}

	// Assert exactly two sequential GETs were made.
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (run GET + jobs GET)", callCount)
	}
	if len(capturedPaths) >= 1 && capturedPaths[0] != "/repos/o/r/actions/runs/55" {
		t.Errorf("first path = %q, want /repos/o/r/actions/runs/55", capturedPaths[0])
	}
	if len(capturedPaths) >= 2 && capturedPaths[1] != "/repos/o/r/actions/runs/55/jobs" {
		t.Errorf("second path = %q, want /repos/o/r/actions/runs/55/jobs", capturedPaths[1])
	}

	// Assert the decoded run.
	if run.ID != 55 {
		t.Errorf("run.ID = %d, want 55", run.ID)
	}
	if run.Name != "CI" {
		t.Errorf("run.Name = %q, want CI", run.Name)
	}
	if run.Status != "completed" {
		t.Errorf("run.Status = %q, want completed", run.Status)
	}
	if run.Conclusion == nil || *run.Conclusion != "success" {
		t.Errorf("run.Conclusion = %v, want *\"success\"", run.Conclusion)
	}
	if run.HeadBranch != "main" {
		t.Errorf("run.HeadBranch = %q, want main", run.HeadBranch)
	}

	// Assert the decoded jobs slice.
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	job := jobs[0]
	if job.ID != 200 {
		t.Errorf("job.ID = %d, want 200", job.ID)
	}
	if job.Name != "build" {
		t.Errorf("job.Name = %q, want build", job.Name)
	}
	if job.Status != "completed" {
		t.Errorf("job.Status = %q, want completed", job.Status)
	}

	// Assert the steps are decoded inside the job.
	if len(job.Steps) != 2 {
		t.Fatalf("len(job.Steps) = %d, want 2", len(job.Steps))
	}
	if job.Steps[0].Name != "Checkout" {
		t.Errorf("job.Steps[0].Name = %q, want Checkout", job.Steps[0].Name)
	}
	if job.Steps[0].Number != 1 {
		t.Errorf("job.Steps[0].Number = %d, want 1", job.Steps[0].Number)
	}
	if job.Steps[1].Name != "Run tests" {
		t.Errorf("job.Steps[1].Name = %q, want Run tests", job.Steps[1].Name)
	}
	if job.Steps[1].Number != 2 {
		t.Errorf("job.Steps[1].Number = %d, want 2", job.Steps[1].Number)
	}
}

func TestClient_GetBuildTimeline_RunFetchError_ReturnsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	run, jobs, err := c.GetBuildTimeline(999)
	if err == nil {
		t.Fatal("expected error for 404 run fetch, got nil")
	}
	if !strings.Contains(err.Error(), "get build timeline") {
		t.Errorf("error = %q: expected wrapping message", err.Error())
	}
	// Zero run and nil jobs on error.
	if run.ID != 0 {
		t.Errorf("run.ID = %d, want 0 on error", run.ID)
	}
	if jobs != nil {
		t.Errorf("jobs = %v, want nil on error", jobs)
	}
}

// ---------------------------------------------------------------------------
// GetBuildLogContent
// ---------------------------------------------------------------------------

func TestClient_GetBuildLogContent_ReturnsPlaintextBody(t *testing.T) {
	const logContent = "2024-06-01T10:00:00.000Z Run tests\n2024-06-01T10:01:00.000Z Tests passed\n"

	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(logContent))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	// runID=55, logID=200 (job ID). The path must use logID, not runID.
	content, err := c.GetBuildLogContent(55, 200)
	if err != nil {
		t.Fatalf("GetBuildLogContent(55, 200) error = %v", err)
	}

	// Assert the path uses logID (job ID), NOT runID.
	wantPath := "/repos/o/r/actions/jobs/200/logs"
	if capturedPath != wantPath {
		t.Errorf("path = %q, want %q", capturedPath, wantPath)
	}

	// Assert the plaintext body is returned as-is.
	if content != logContent {
		t.Errorf("content = %q, want %q", content, logContent)
	}
}

func TestClient_GetBuildLogContent_PathUsesLogIDNotRunID(t *testing.T) {
	// runID and logID are deliberately different to confirm which one appears in
	// the path.
	const runID = 999
	const logID = 42

	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("log line\n"))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.GetBuildLogContent(runID, logID)
	if err != nil {
		t.Fatalf("GetBuildLogContent() error = %v", err)
	}

	// Path must contain logID (42), not runID (999).
	if !strings.Contains(capturedPath, "/42/") {
		t.Errorf("path = %q: must contain logID 42, not runID 999", capturedPath)
	}
	if strings.Contains(capturedPath, "/999/") || strings.Contains(capturedPath, "/999") {
		t.Errorf("path = %q: must NOT contain runID 999 (runID is unused)", capturedPath)
	}
	// Path must use the jobs endpoint.
	if !strings.Contains(capturedPath, "/actions/jobs/") {
		t.Errorf("path = %q: must use /actions/jobs/ endpoint", capturedPath)
	}
}

func TestClient_GetBuildLogContent_LogIDZero_ReturnsError_NoRequest(t *testing.T) {
	// requestCount must remain 0 — logID==0 must short-circuit before any HTTP call.
	requestCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("should not reach here"))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.GetBuildLogContent(55, 0)
	if err == nil {
		t.Fatal("expected error for logID==0, got nil")
	}
	// Error must be descriptive.
	if !strings.Contains(err.Error(), "logID") {
		t.Errorf("error = %q: must mention logID", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "step") {
		t.Errorf("error = %q: must mention Step (logID==0 means Step record)", err.Error())
	}
	// No HTTP request must have been made.
	if requestCount != 0 {
		t.Errorf("requestCount = %d, want 0 (logID==0 must not trigger an HTTP call)", requestCount)
	}
}

// ---------------------------------------------------------------------------
// mapRunStatusParam (unit-tested directly as an unexported function inside the
// same package)
// ---------------------------------------------------------------------------

func TestMapRunStatusParam(t *testing.T) {
	cases := []struct {
		name   string
		status provider.RunStatus
		want   string
	}{
		{name: "RunStatusRunning → in_progress", status: provider.RunStatusRunning, want: "in_progress"},
		{name: "RunStatusQueued → queued", status: provider.RunStatusQueued, want: "queued"},
		{name: "RunStatusPending → waiting", status: provider.RunStatusPending, want: "waiting"},
		{name: "RunStatusSucceeded → success", status: provider.RunStatusSucceeded, want: "success"},
		{name: "RunStatusFailed → failure", status: provider.RunStatusFailed, want: "failure"},
		{name: "RunStatusCanceled → cancelled", status: provider.RunStatusCanceled, want: "cancelled"},
		// No clean mapping — must return "".
		{name: "RunStatusCanceling → ''", status: provider.RunStatusCanceling, want: ""},
		{name: "RunStatusPartiallySucceeded → ''", status: provider.RunStatusPartiallySucceeded, want: ""},
		{name: "RunStatusSucceededWithIssues → ''", status: provider.RunStatusSucceededWithIssues, want: ""},
		{name: "RunStatusUnknown → ''", status: provider.RunStatusUnknown, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapRunStatusParam(tc.status)
			if got != tc.want {
				t.Errorf("mapRunStatusParam(%v) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
