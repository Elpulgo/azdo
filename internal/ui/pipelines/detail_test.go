package pipelines

import (
	"fmt"
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

func TestDetailModel_ViewportScrolling(t *testing.T) {
	// Create a timeline with many items to test scrolling
	records := make([]azdevops.TimelineRecord, 50)
	for i := 0; i < 50; i++ {
		records[i] = azdevops.TimelineRecord{
			ID:       fmt.Sprintf("task-%d", i),
			ParentID: nil,
			Type:     "Task",
			Name:     fmt.Sprintf("Task %d", i),
			Order:    i,
		}
	}

	timeline := &azdevops.Timeline{ID: "test", Records: records}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetSize(80, 20) // Set a small height to trigger scrolling
	model.SetTimeline(timeline)

	// View should render without panic
	view := model.View()
	if view == "" {
		t.Error("View should not be empty")
	}

	// Move down many times - selection should change
	for i := 0; i < 30; i++ {
		model.MoveDown()
	}

	if model.SelectedIndex() != 30 {
		t.Errorf("After 30 MoveDown, SelectedIndex() = %d, want 30", model.SelectedIndex())
	}

	// View should still be renderable without panic
	view = model.View()
	if view == "" {
		t.Error("View should not be empty after scrolling")
	}

	// Scroll percentage should be accessible via GetScrollPercent
	percent := model.GetScrollPercent()
	if percent < 0 || percent > 100 {
		t.Errorf("GetScrollPercent() = %f, should be between 0 and 100", percent)
	}
}

func TestDetailModel_PageUpDown(t *testing.T) {
	// Create a timeline with many items
	records := make([]azdevops.TimelineRecord, 50)
	for i := 0; i < 50; i++ {
		records[i] = azdevops.TimelineRecord{
			ID:       fmt.Sprintf("task-%d", i),
			ParentID: nil,
			Type:     "Task",
			Name:     fmt.Sprintf("Task %d", i),
			Order:    i,
		}
	}

	timeline := &azdevops.Timeline{ID: "test", Records: records}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetSize(80, 20) // viewport height = 20 - 6 = 14
	model.SetTimeline(timeline)

	// Initial position should be 0
	if model.SelectedIndex() != 0 {
		t.Errorf("Initial SelectedIndex() = %d, want 0", model.SelectedIndex())
	}

	// PageDown should move selection by viewport height
	model.PageDown()
	// Should move roughly one page (viewport height is 14)
	if model.SelectedIndex() < 10 {
		t.Errorf("After PageDown, SelectedIndex() = %d, want >= 10", model.SelectedIndex())
	}

	prevIndex := model.SelectedIndex()

	// PageUp should move selection back up
	model.PageUp()
	if model.SelectedIndex() >= prevIndex {
		t.Errorf("After PageUp, SelectedIndex() = %d, should be less than %d", model.SelectedIndex(), prevIndex)
	}
}

func TestDetailModel_StatusMessage(t *testing.T) {
	// Create timeline with items that have and don't have logs
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build Stage", Order: 1},
			{ID: "task-1", ParentID: strPtr("stage-1"), Type: "Task", Name: "npm install", Order: 1, Log: &azdevops.LogReference{ID: 5}},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetSize(80, 20)
	model.SetTimeline(timeline)

	// Initially selected item (stage) has no log - status message should indicate this
	if model.SelectedItem().Record.Log != nil {
		t.Error("First item (stage) should not have a log")
	}

	// GetStatusMessage should indicate no logs available
	msg := model.GetStatusMessage()
	if msg == "" {
		t.Error("GetStatusMessage should return a message for items without logs")
	}

	// Move to task with log
	model.MoveDown()
	if model.SelectedItem().Record.Log == nil {
		t.Error("Second item (task) should have a log")
	}

	// Status message should be empty or indicate logs are available
	msg = model.GetStatusMessage()
	if strings.Contains(msg, "no log") {
		t.Error("GetStatusMessage should not say 'no log' for items with logs")
	}
}

func TestDetailModel_CanViewLogs(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build Stage", Order: 1},
			{ID: "task-1", ParentID: strPtr("stage-1"), Type: "Task", Name: "npm install", Order: 1, Log: &azdevops.LogReference{ID: 5}},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Stage should not be viewable
	if model.CanViewLogs() {
		t.Error("CanViewLogs() should return false for stage without logs")
	}

	// Task with log should be viewable
	model.MoveDown()
	if !model.CanViewLogs() {
		t.Error("CanViewLogs() should return true for task with logs")
	}
}

func TestDetailModel_GetContextItems(t *testing.T) {
	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)

	items := model.GetContextItems()

	// Should have keybinding items
	if len(items) == 0 {
		t.Error("GetContextItems() should return items")
	}

	// Should include navigation keys
	found := false
	for _, item := range items {
		if strings.Contains(item.Key, "↑↓") || strings.Contains(item.Description, "navigate") {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetContextItems() should include navigation keybinding")
	}
}

func TestDetailModel_GetScrollPercent(t *testing.T) {
	records := make([]azdevops.TimelineRecord, 50)
	for i := 0; i < 50; i++ {
		records[i] = azdevops.TimelineRecord{
			ID:       fmt.Sprintf("task-%d", i),
			ParentID: nil,
			Type:     "Task",
			Name:     fmt.Sprintf("Task %d", i),
			Order:    i,
		}
	}

	timeline := &azdevops.Timeline{ID: "test", Records: records}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetSize(80, 20)
	model.SetTimeline(timeline)

	// Initially should be at top (0%)
	percent := model.GetScrollPercent()
	if percent < 0 || percent > 100 {
		t.Errorf("GetScrollPercent() = %f, should be between 0 and 100", percent)
	}

	// After scrolling down, percent should increase
	for i := 0; i < 30; i++ {
		model.MoveDown()
	}
	newPercent := model.GetScrollPercent()
	if newPercent <= percent {
		t.Errorf("GetScrollPercent() should increase after scrolling down, was %f now %f", percent, newPercent)
	}
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
