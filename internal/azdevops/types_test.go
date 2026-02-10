package azdevops

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPipelineRunUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		want     PipelineRun
		wantErr  bool
	}{
		{
			name: "complete pipeline run with all fields",
			jsonData: `{
				"id": 12345,
				"buildNumber": "20240206.1",
				"status": "completed",
				"result": "succeeded",
				"sourceBranch": "refs/heads/main",
				"sourceVersion": "abc123def456",
				"queueTime": "2024-02-06T10:00:00Z",
				"startTime": "2024-02-06T10:01:00Z",
				"finishTime": "2024-02-06T10:15:00Z",
				"definition": {
					"id": 42,
					"name": "CI-Pipeline"
				},
				"project": {
					"id": "proj-123",
					"name": "MyProject"
				},
				"_links": {
					"web": {
						"href": "https://dev.azure.com/org/proj/_build/results?buildId=12345"
					}
				}
			}`,
			want: PipelineRun{
				ID:            12345,
				BuildNumber:   "20240206.1",
				Status:        "completed",
				Result:        "succeeded",
				SourceBranch:  "refs/heads/main",
				SourceVersion: "abc123def456",
				QueueTime:     parseTime(t, "2024-02-06T10:00:00Z"),
				StartTime:     parseTimePtr(t, "2024-02-06T10:01:00Z"),
				FinishTime:    parseTimePtr(t, "2024-02-06T10:15:00Z"),
				Definition: PipelineDefinition{
					ID:   42,
					Name: "CI-Pipeline",
				},
				Project: Project{
					ID:   "proj-123",
					Name: "MyProject",
				},
				Links: Links{
					Web: Link{Href: "https://dev.azure.com/org/proj/_build/results?buildId=12345"},
				},
			},
			wantErr: false,
		},
		{
			name: "in-progress pipeline run without finish time",
			jsonData: `{
				"id": 12346,
				"buildNumber": "20240206.2",
				"status": "inProgress",
				"result": null,
				"sourceBranch": "refs/heads/feature/test",
				"sourceVersion": "def456abc123",
				"queueTime": "2024-02-06T11:00:00Z",
				"startTime": "2024-02-06T11:01:00Z",
				"definition": {
					"id": 42,
					"name": "CI-Pipeline"
				},
				"project": {
					"id": "proj-123",
					"name": "MyProject"
				},
				"_links": {
					"web": {
						"href": "https://dev.azure.com/org/proj/_build/results?buildId=12346"
					}
				}
			}`,
			want: PipelineRun{
				ID:            12346,
				BuildNumber:   "20240206.2",
				Status:        "inProgress",
				Result:        "",
				SourceBranch:  "refs/heads/feature/test",
				SourceVersion: "def456abc123",
				QueueTime:     parseTime(t, "2024-02-06T11:00:00Z"),
				StartTime:     parseTimePtr(t, "2024-02-06T11:01:00Z"),
				FinishTime:    nil,
				Definition: PipelineDefinition{
					ID:   42,
					Name: "CI-Pipeline",
				},
				Project: Project{
					ID:   "proj-123",
					Name: "MyProject",
				},
				Links: Links{
					Web: Link{Href: "https://dev.azure.com/org/proj/_build/results?buildId=12346"},
				},
			},
			wantErr: false,
		},
		{
			name: "failed pipeline run",
			jsonData: `{
				"id": 12347,
				"buildNumber": "20240206.3",
				"status": "completed",
				"result": "failed",
				"sourceBranch": "refs/heads/main",
				"sourceVersion": "xyz789",
				"queueTime": "2024-02-06T12:00:00Z",
				"startTime": "2024-02-06T12:01:00Z",
				"finishTime": "2024-02-06T12:05:00Z",
				"definition": {
					"id": 42,
					"name": "CI-Pipeline"
				},
				"project": {
					"id": "proj-123",
					"name": "MyProject"
				},
				"_links": {
					"web": {
						"href": "https://dev.azure.com/org/proj/_build/results?buildId=12347"
					}
				}
			}`,
			want: PipelineRun{
				ID:            12347,
				BuildNumber:   "20240206.3",
				Status:        "completed",
				Result:        "failed",
				SourceBranch:  "refs/heads/main",
				SourceVersion: "xyz789",
				QueueTime:     parseTime(t, "2024-02-06T12:00:00Z"),
				StartTime:     parseTimePtr(t, "2024-02-06T12:01:00Z"),
				FinishTime:    parseTimePtr(t, "2024-02-06T12:05:00Z"),
				Definition: PipelineDefinition{
					ID:   42,
					Name: "CI-Pipeline",
				},
				Project: Project{
					ID:   "proj-123",
					Name: "MyProject",
				},
				Links: Links{
					Web: Link{Href: "https://dev.azure.com/org/proj/_build/results?buildId=12347"},
				},
			},
			wantErr: false,
		},
		{
			name: "canceled pipeline run",
			jsonData: `{
				"id": 12348,
				"buildNumber": "20240206.4",
				"status": "completed",
				"result": "canceled",
				"sourceBranch": "refs/heads/feature/branch",
				"sourceVersion": "abc999",
				"queueTime": "2024-02-06T13:00:00Z",
				"startTime": "2024-02-06T13:01:00Z",
				"finishTime": "2024-02-06T13:03:00Z",
				"definition": {
					"id": 43,
					"name": "Deploy-Pipeline"
				},
				"project": {
					"id": "proj-123",
					"name": "MyProject"
				},
				"_links": {
					"web": {
						"href": "https://dev.azure.com/org/proj/_build/results?buildId=12348"
					}
				}
			}`,
			want: PipelineRun{
				ID:            12348,
				BuildNumber:   "20240206.4",
				Status:        "completed",
				Result:        "canceled",
				SourceBranch:  "refs/heads/feature/branch",
				SourceVersion: "abc999",
				QueueTime:     parseTime(t, "2024-02-06T13:00:00Z"),
				StartTime:     parseTimePtr(t, "2024-02-06T13:01:00Z"),
				FinishTime:    parseTimePtr(t, "2024-02-06T13:03:00Z"),
				Definition: PipelineDefinition{
					ID:   43,
					Name: "Deploy-Pipeline",
				},
				Project: Project{
					ID:   "proj-123",
					Name: "MyProject",
				},
				Links: Links{
					Web: Link{Href: "https://dev.azure.com/org/proj/_build/results?buildId=12348"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PipelineRun
			err := json.Unmarshal([]byte(tt.jsonData), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Compare fields
			if got.ID != tt.want.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.want.ID)
			}
			if got.BuildNumber != tt.want.BuildNumber {
				t.Errorf("BuildNumber = %v, want %v", got.BuildNumber, tt.want.BuildNumber)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %v, want %v", got.Status, tt.want.Status)
			}
			if got.Result != tt.want.Result {
				t.Errorf("Result = %v, want %v", got.Result, tt.want.Result)
			}
			if got.SourceBranch != tt.want.SourceBranch {
				t.Errorf("SourceBranch = %v, want %v", got.SourceBranch, tt.want.SourceBranch)
			}
			if got.SourceVersion != tt.want.SourceVersion {
				t.Errorf("SourceVersion = %v, want %v", got.SourceVersion, tt.want.SourceVersion)
			}
			if !got.QueueTime.Equal(tt.want.QueueTime) {
				t.Errorf("QueueTime = %v, want %v", got.QueueTime, tt.want.QueueTime)
			}
			if !timePointersEqual(got.StartTime, tt.want.StartTime) {
				t.Errorf("StartTime = %v, want %v", formatTimePtr(got.StartTime), formatTimePtr(tt.want.StartTime))
			}
			if !timePointersEqual(got.FinishTime, tt.want.FinishTime) {
				t.Errorf("FinishTime = %v, want %v", formatTimePtr(got.FinishTime), formatTimePtr(tt.want.FinishTime))
			}
			if got.Definition.ID != tt.want.Definition.ID {
				t.Errorf("Definition.ID = %v, want %v", got.Definition.ID, tt.want.Definition.ID)
			}
			if got.Definition.Name != tt.want.Definition.Name {
				t.Errorf("Definition.Name = %v, want %v", got.Definition.Name, tt.want.Definition.Name)
			}
			if got.Project.ID != tt.want.Project.ID {
				t.Errorf("Project.ID = %v, want %v", got.Project.ID, tt.want.Project.ID)
			}
			if got.Project.Name != tt.want.Project.Name {
				t.Errorf("Project.Name = %v, want %v", got.Project.Name, tt.want.Project.Name)
			}
			if got.Links.Web.Href != tt.want.Links.Web.Href {
				t.Errorf("Links.Web.Href = %v, want %v", got.Links.Web.Href, tt.want.Links.Web.Href)
			}
		})
	}
}

func TestPipelineRunList(t *testing.T) {
	jsonData := `{
		"count": 2,
		"value": [
			{
				"id": 1,
				"buildNumber": "20240206.1",
				"status": "completed",
				"result": "succeeded",
				"sourceBranch": "refs/heads/main",
				"sourceVersion": "abc123",
				"queueTime": "2024-02-06T10:00:00Z",
				"startTime": "2024-02-06T10:01:00Z",
				"finishTime": "2024-02-06T10:15:00Z",
				"definition": {
					"id": 42,
					"name": "CI-Pipeline"
				},
				"project": {
					"id": "proj-123",
					"name": "MyProject"
				},
				"_links": {
					"web": {
						"href": "https://dev.azure.com/org/proj/_build/results?buildId=1"
					}
				}
			},
			{
				"id": 2,
				"buildNumber": "20240206.2",
				"status": "inProgress",
				"result": null,
				"sourceBranch": "refs/heads/feature/test",
				"sourceVersion": "def456",
				"queueTime": "2024-02-06T11:00:00Z",
				"startTime": "2024-02-06T11:01:00Z",
				"definition": {
					"id": 42,
					"name": "CI-Pipeline"
				},
				"project": {
					"id": "proj-123",
					"name": "MyProject"
				},
				"_links": {
					"web": {
						"href": "https://dev.azure.com/org/proj/_build/results?buildId=2"
					}
				}
			}
		]
	}`

	var response PipelineRunsResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("Count = %v, want 2", response.Count)
	}

	if len(response.Value) != 2 {
		t.Fatalf("len(Value) = %v, want 2", len(response.Value))
	}

	// Check first run
	run1 := response.Value[0]
	if run1.ID != 1 {
		t.Errorf("Value[0].ID = %v, want 1", run1.ID)
	}
	if run1.Status != "completed" {
		t.Errorf("Value[0].Status = %v, want completed", run1.Status)
	}
	if run1.Result != "succeeded" {
		t.Errorf("Value[0].Result = %v, want succeeded", run1.Result)
	}

	// Check second run
	run2 := response.Value[1]
	if run2.ID != 2 {
		t.Errorf("Value[1].ID = %v, want 2", run2.ID)
	}
	if run2.Status != "inProgress" {
		t.Errorf("Value[1].Status = %v, want inProgress", run2.Status)
	}
	if run2.FinishTime != nil {
		t.Errorf("Value[1].FinishTime = %v, want nil", run2.FinishTime)
	}
}

func TestBranchShortName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "full refs/heads branch name",
			input: "refs/heads/main",
			want:  "main",
		},
		{
			name:  "full refs/heads feature branch",
			input: "refs/heads/feature/my-feature",
			want:  "feature/my-feature",
		},
		{
			name:  "already short branch name",
			input: "main",
			want:  "main",
		},
		{
			name:  "refs/tags",
			input: "refs/tags/v1.0.0",
			want:  "v1.0.0",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := PipelineRun{SourceBranch: tt.input}
			got := run.BranchShortName()
			if got != tt.want {
				t.Errorf("BranchShortName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name string
		run  PipelineRun
		want string
	}{
		{
			name: "completed run with duration",
			run: PipelineRun{
				Status:     "completed",
				StartTime:  parseTimePtr(t, "2024-02-06T10:00:00Z"),
				FinishTime: parseTimePtr(t, "2024-02-06T10:05:00Z"),
			},
			want: "5m0s",
		},
		{
			name: "in-progress run without finish time",
			run: PipelineRun{
				Status:     "inProgress",
				StartTime:  parseTimePtr(t, "2024-02-06T10:00:00Z"),
				FinishTime: nil,
			},
			want: "-",
		},
		{
			name: "queued run without start time",
			run: PipelineRun{
				Status:     "notStarted",
				StartTime:  nil,
				FinishTime: nil,
			},
			want: "-",
		},
		{
			name: "completed with hours",
			run: PipelineRun{
				Status:     "completed",
				StartTime:  parseTimePtr(t, "2024-02-06T10:00:00Z"),
				FinishTime: parseTimePtr(t, "2024-02-06T12:30:45Z"),
			},
			want: "2h30m45s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.run.Duration()
			if got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper functions

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("Failed to parse time %s: %v", s, err)
	}
	return tm
}

func parseTimePtr(t *testing.T, s string) *time.Time {
	t.Helper()
	tm := parseTime(t, s)
	return &tm
}

func timePointersEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "nil"
	}
	return t.Format(time.RFC3339)
}
