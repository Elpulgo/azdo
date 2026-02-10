package pipelines

import (
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

func TestDetailRecordIcon(t *testing.T) {
	tests := []struct {
		name         string
		recordType   string
		state        string
		result       string
		wantContains string
	}{
		// Stage icons
		{
			name:         "stage completed succeeded",
			recordType:   "Stage",
			state:        "completed",
			result:       "succeeded",
			wantContains: "✓",
		},
		{
			name:         "stage in progress",
			recordType:   "Stage",
			state:        "inProgress",
			result:       "",
			wantContains: "●",
		},
		{
			name:         "stage pending",
			recordType:   "Stage",
			state:        "pending",
			result:       "",
			wantContains: "○",
		},

		// Job icons
		{
			name:         "job completed failed",
			recordType:   "Job",
			state:        "completed",
			result:       "failed",
			wantContains: "✗",
		},

		// Task icons
		{
			name:         "task completed skipped",
			recordType:   "Task",
			state:        "completed",
			result:       "skipped",
			wantContains: "○",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recordIcon(tt.state, tt.result)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("recordIcon(%q, %q) = %q, want to contain %q",
					tt.state, tt.result, got, tt.wantContains)
			}
		})
	}
}

func TestDetailIndentation(t *testing.T) {
	tests := []struct {
		name       string
		recordType string
		want       string
	}{
		{
			name:       "stage has no indentation",
			recordType: "Stage",
			want:       "",
		},
		{
			name:       "job has 2 space indentation",
			recordType: "Job",
			want:       "  ",
		},
		{
			name:       "task has 4 space indentation",
			recordType: "Task",
			want:       "    ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indentForType(tt.recordType)
			if got != tt.want {
				t.Errorf("indentForType(%q) = %q, want %q", tt.recordType, got, tt.want)
			}
		})
	}
}

func TestBuildTimelineTree(t *testing.T) {
	// Create a sample timeline with nested structure
	timeline := &azdevops.Timeline{
		ID:       "test-timeline",
		ChangeID: 1,
		Records: []azdevops.TimelineRecord{
			{
				ID:       "stage-1",
				ParentID: nil,
				Type:     "Stage",
				Name:     "Build",
				State:    "completed",
				Result:   "succeeded",
				Order:    1,
			},
			{
				ID:       "job-1",
				ParentID: strPtr("stage-1"),
				Type:     "Job",
				Name:     "Build Job",
				State:    "completed",
				Result:   "succeeded",
				Order:    1,
			},
			{
				ID:       "task-1",
				ParentID: strPtr("job-1"),
				Type:     "Task",
				Name:     "npm install",
				State:    "completed",
				Result:   "succeeded",
				Order:    1,
			},
			{
				ID:       "task-2",
				ParentID: strPtr("job-1"),
				Type:     "Task",
				Name:     "npm build",
				State:    "completed",
				Result:   "succeeded",
				Order:    2,
			},
			{
				ID:       "stage-2",
				ParentID: nil,
				Type:     "Stage",
				Name:     "Test",
				State:    "inProgress",
				Result:   "",
				Order:    2,
			},
		},
	}

	tree := buildTimelineTree(timeline)

	// Should have 2 root stages
	if len(tree) != 2 {
		t.Fatalf("Expected 2 root nodes, got %d", len(tree))
	}

	// First stage should be "Build"
	if tree[0].Record.Name != "Build" {
		t.Errorf("First stage name = %q, want Build", tree[0].Record.Name)
	}

	// Build stage should have 1 child (job)
	if len(tree[0].Children) != 1 {
		t.Errorf("Build stage children = %d, want 1", len(tree[0].Children))
	}

	// Job should have 2 children (tasks)
	if len(tree[0].Children[0].Children) != 2 {
		t.Errorf("Build job children = %d, want 2", len(tree[0].Children[0].Children))
	}

	// Second stage should be "Test"
	if tree[1].Record.Name != "Test" {
		t.Errorf("Second stage name = %q, want Test", tree[1].Record.Name)
	}
}

func TestFlattenTree(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1},
			{ID: "task-1", ParentID: strPtr("job-1"), Type: "Task", Name: "npm install", Order: 1},
			{ID: "stage-2", ParentID: nil, Type: "Stage", Name: "Test", Order: 2},
		},
	}

	tree := buildTimelineTree(timeline)
	flat := flattenTree(tree)

	// Should have 4 items in order: stage-1, job-1, task-1, stage-2
	if len(flat) != 4 {
		t.Fatalf("Expected 4 flat items, got %d", len(flat))
	}

	expectedOrder := []string{"Build", "Build Job", "npm install", "Test"}
	for i, item := range flat {
		if item.Record.Name != expectedOrder[i] {
			t.Errorf("flat[%d].Name = %q, want %q", i, item.Record.Name, expectedOrder[i])
		}
	}
}

func TestFormatDuration(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		startTime  *time.Time
		finishTime *time.Time
		want       string
	}{
		{
			name:       "no start time",
			startTime:  nil,
			finishTime: nil,
			want:       "-",
		},
		{
			name:       "in progress (no finish time)",
			startTime:  timePtr(now.Add(-5 * time.Minute)),
			finishTime: nil,
			want:       "-",
		},
		{
			name:       "completed 30 seconds",
			startTime:  timePtr(now.Add(-30 * time.Second)),
			finishTime: timePtr(now),
			want:       "30s",
		},
		{
			name:       "completed 2 minutes 30 seconds",
			startTime:  timePtr(now.Add(-150 * time.Second)),
			finishTime: timePtr(now),
			want:       "2m30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRecordDuration(tt.startTime, tt.finishTime)
			if got != tt.want {
				t.Errorf("formatRecordDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetailModel_SelectedItem(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1, Log: &azdevops.LogReference{ID: 5}},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Initial selection should be index 0
	if model.SelectedIndex() != 0 {
		t.Errorf("Initial SelectedIndex() = %d, want 0", model.SelectedIndex())
	}

	// Move down
	model.MoveDown()
	if model.SelectedIndex() != 1 {
		t.Errorf("After MoveDown, SelectedIndex() = %d, want 1", model.SelectedIndex())
	}

	// Selected item should have a log
	selected := model.SelectedItem()
	if selected == nil {
		t.Fatal("SelectedItem() returned nil")
	}
	if selected.Record.Log == nil {
		t.Error("Selected item should have a Log reference")
	}
	if selected.Record.Log.ID != 5 {
		t.Errorf("Selected log ID = %d, want 5", selected.Record.Log.ID)
	}
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
