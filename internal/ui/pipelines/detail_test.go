package pipelines

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
)

func TestDetailModel_HasStyles(t *testing.T) {
	run := azdevops.PipelineRun{ID: 123, Definition: azdevops.PipelineDefinition{Name: "Test"}}
	m := NewDetailModel(nil, run)

	if m.styles == nil {
		t.Error("Expected detail model to have styles initialized")
	}
}

func TestDetailModel_WithStyles(t *testing.T) {
	run := azdevops.PipelineRun{ID: 123, Definition: azdevops.PipelineDefinition{Name: "Test"}}
	customStyles := styles.NewStyles(styles.GetThemeByNameWithFallback("nord"))
	m := NewDetailModelWithStyles(nil, run, customStyles)

	if m.styles != customStyles {
		t.Error("Expected detail model to use provided custom styles")
	}
}

func TestRecordIconWithStyles(t *testing.T) {
	themes := []string{"dark", "gruvbox", "nord", "dracula"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			tests := []struct {
				state, result string
				wantContains  string
			}{
				{"inprogress", "", "●"},
				{"completed", "succeeded", "✓"},
				{"completed", "failed", "✗"},
				{"pending", "", "○"},
			}

			for _, tt := range tests {
				got := recordIconWithStyles(tt.state, tt.result, s)
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("recordIconWithStyles(%q, %q) with theme %s = %q, want to contain %q",
						tt.state, tt.result, themeName, got, tt.wantContains)
				}
			}
		})
	}
}

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
			got := recordIconWithStyles(tt.state, tt.result, styles.DefaultStyles())
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("recordIconWithStyles(%q, %q) = %q, want to contain %q",
					tt.state, tt.result, got, tt.wantContains)
			}
		})
	}
}

func TestVisualDepthIndentation(t *testing.T) {
	// Indentation is based on visual depth (2 spaces per level)
	// not record type, so root-level Jobs get no indentation
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "phase-1", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase", Order: 1},
			{ID: "job-1", ParentID: strPtr("phase-1"), Type: "Job", Name: "Build Job", Order: 1},
			{ID: "task-1", ParentID: strPtr("job-1"), Type: "Task", Name: "npm install", Order: 1},
			// Root-level Job (like Azure DevOps "Finalize build")
			{ID: "finalize", ParentID: nil, Type: "Job", Name: "Finalize build", Order: 2},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Expand everything to check depths
	model.ToggleExpand() // expand Build stage
	model.MoveDown()
	model.ToggleExpand() // expand Build Job

	// Visual depths: Build=0, Build Job=1, npm install=2, Finalize build=0
	expected := []struct {
		name  string
		depth int
	}{
		{"Build", 0},
		{"Build Job", 1},
		{"npm install", 2},
		{"Finalize build", 0},
	}

	if len(model.flatItems) != len(expected) {
		t.Fatalf("Expected %d flat items, got %d", len(expected), len(model.flatItems))
	}

	for i, exp := range expected {
		node := model.flatItems[i]
		if node.Record.Name != exp.name {
			t.Errorf("flatItems[%d].Name = %q, want %q", i, node.Record.Name, exp.name)
		}
		if node.VisualDepth != exp.depth {
			t.Errorf("flatItems[%d] (%s) VisualDepth = %d, want %d", i, exp.name, node.VisualDepth, exp.depth)
		}
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

	// Collapsed by default: only root stages visible
	flat := flattenTree(tree)
	if len(flat) != 2 {
		t.Fatalf("Expected 2 flat items (collapsed), got %d", len(flat))
	}
	if flat[0].Record.Name != "Build" {
		t.Errorf("flat[0].Name = %q, want Build", flat[0].Record.Name)
	}
	if flat[1].Record.Name != "Test" {
		t.Errorf("flat[1].Name = %q, want Test", flat[1].Record.Name)
	}

	// Expand all nodes and verify full tree
	var expandAll func(nodes []*TimelineNode)
	expandAll = func(nodes []*TimelineNode) {
		for _, n := range nodes {
			n.Expanded = true
			expandAll(n.Children)
		}
	}
	expandAll(tree)

	flat = flattenTree(tree)
	if len(flat) != 4 {
		t.Fatalf("Expected 4 flat items (expanded), got %d", len(flat))
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

	// Expand stage to reveal job
	model.ToggleExpand()

	// Move down to job
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

	// Expand stage and move to task with log
	model.ToggleExpand()
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

	// Expand stage and move to task with log
	model.ToggleExpand()
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
	if percent != 0 {
		t.Errorf("GetScrollPercent() at top = %f, want 0", percent)
	}

	// After scrolling down, percent should increase
	for i := 0; i < 30; i++ {
		model.MoveDown()
	}
	newPercent := model.GetScrollPercent()
	if newPercent <= percent {
		t.Errorf("GetScrollPercent() should increase after scrolling down, was %f now %f", percent, newPercent)
	}

	// At selection 30 out of 49 (0-indexed), percent should be ~61%
	expectedPercent := float64(30) / float64(49) * 100
	if newPercent < expectedPercent-1 || newPercent > expectedPercent+1 {
		t.Errorf("GetScrollPercent() at index 30/49 = %f, want ~%f", newPercent, expectedPercent)
	}

	// Move to the end - should be 100%
	for i := 0; i < 20; i++ {
		model.MoveDown()
	}
	endPercent := model.GetScrollPercent()
	if endPercent != 100 {
		t.Errorf("GetScrollPercent() at end = %f, want 100", endPercent)
	}
}

func TestTimelineNode_HasChildren(t *testing.T) {
	node := &TimelineNode{
		Record:   azdevops.TimelineRecord{ID: "stage-1", Type: "Stage", Name: "Build"},
		Children: []*TimelineNode{},
	}

	if node.HasChildren() {
		t.Error("Node with empty children slice should not have children")
	}

	node.Children = []*TimelineNode{
		{Record: azdevops.TimelineRecord{ID: "job-1", Type: "Job", Name: "Build Job"}},
	}

	if !node.HasChildren() {
		t.Error("Node with children should have children")
	}
}

func TestDetailModel_NodesStartCollapsed(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1},
			{ID: "task-1", ParentID: strPtr("job-1"), Type: "Task", Name: "npm install", Order: 1},
			{ID: "stage-2", ParentID: nil, Type: "Stage", Name: "Test", Order: 2},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Stages should start collapsed — flatItems should only contain the 2 stages
	if len(model.flatItems) != 2 {
		t.Errorf("Expected 2 flat items (collapsed stages), got %d", len(model.flatItems))
	}

	if model.flatItems[0].Record.Name != "Build" {
		t.Errorf("First flat item = %q, want Build", model.flatItems[0].Record.Name)
	}
	if model.flatItems[1].Record.Name != "Test" {
		t.Errorf("Second flat item = %q, want Test", model.flatItems[1].Record.Name)
	}
}

func TestDetailModel_ToggleExpand(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1},
			{ID: "task-1", ParentID: strPtr("job-1"), Type: "Task", Name: "npm install", Order: 1},
			{ID: "task-2", ParentID: strPtr("job-1"), Type: "Task", Name: "npm build", Order: 2},
			{ID: "stage-2", ParentID: nil, Type: "Stage", Name: "Test", Order: 2},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Initially collapsed: only 2 stages visible
	if len(model.flatItems) != 2 {
		t.Fatalf("Expected 2 flat items initially, got %d", len(model.flatItems))
	}

	// Toggle expand on stage-1 (selected by default)
	model.ToggleExpand()

	// After expanding stage-1: stage-1, job-1, stage-2 (job is collapsed so tasks hidden)
	if len(model.flatItems) != 3 {
		t.Fatalf("Expected 3 flat items after expanding stage, got %d", len(model.flatItems))
	}

	expectedOrder := []string{"Build", "Build Job", "Test"}
	for i, name := range expectedOrder {
		if model.flatItems[i].Record.Name != name {
			t.Errorf("flatItems[%d] = %q, want %q", i, model.flatItems[i].Record.Name, name)
		}
	}

	// Move to job-1 and expand it
	model.MoveDown()
	model.ToggleExpand()

	// After expanding job-1: stage-1, job-1, task-1, task-2, stage-2
	if len(model.flatItems) != 5 {
		t.Fatalf("Expected 5 flat items after expanding job, got %d", len(model.flatItems))
	}

	expectedOrder = []string{"Build", "Build Job", "npm install", "npm build", "Test"}
	for i, name := range expectedOrder {
		if model.flatItems[i].Record.Name != name {
			t.Errorf("flatItems[%d] = %q, want %q", i, model.flatItems[i].Record.Name, name)
		}
	}

	// Collapse stage-1 again (move back to it)
	model.selectedIndex = 0
	model.ToggleExpand()

	// After collapsing stage-1: stage-1, stage-2
	if len(model.flatItems) != 2 {
		t.Fatalf("Expected 2 flat items after collapsing stage, got %d", len(model.flatItems))
	}
}

func TestDetailModel_ToggleExpandPreservesSelection(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1},
			{ID: "stage-2", ParentID: nil, Type: "Stage", Name: "Test", Order: 2},
			{ID: "job-2", ParentID: strPtr("stage-2"), Type: "Job", Name: "Test Job", Order: 1},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Select stage-2 (index 1)
	model.MoveDown()

	// Expand stage-2
	model.ToggleExpand()

	// Selection should stay on stage-2 (still index 1)
	selected := model.SelectedItem()
	if selected == nil || selected.Record.Name != "Test" {
		name := ""
		if selected != nil {
			name = selected.Record.Name
		}
		t.Errorf("After expanding, selected should be Test, got %q", name)
	}
}

func TestDetailModel_ToggleDoesNothingOnLeafNode(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "task-1", ParentID: strPtr("stage-1"), Type: "Task", Name: "npm install", Order: 1},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Expand stage to see the task
	model.ToggleExpand()

	// Move to the task (leaf node)
	model.MoveDown()

	prevLen := len(model.flatItems)
	model.ToggleExpand()

	// Nothing should change
	if len(model.flatItems) != prevLen {
		t.Errorf("ToggleExpand on leaf node should not change flatItems count, was %d now %d", prevLen, len(model.flatItems))
	}
}

func TestDetailModel_RenderShowsExpandIndicator(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", State: "completed", Result: "succeeded", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", State: "completed", Result: "succeeded", Order: 1},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModelWithStyles(nil, run, styles.DefaultStyles())
	model.SetTimeline(timeline)

	// Collapsed stage should show ▶
	rendered := model.renderRecord(model.flatItems[0], false)
	if !strings.Contains(rendered, "▶") {
		t.Errorf("Collapsed node should show ▶, got %q", rendered)
	}

	// Expand and check for ▼
	model.ToggleExpand()
	rendered = model.renderRecord(model.flatItems[0], false)
	if !strings.Contains(rendered, "▼") {
		t.Errorf("Expanded node should show ▼, got %q", rendered)
	}
}

func TestDetailModel_CollapseAdjustsSelectionIfBeyondBounds(t *testing.T) {
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1},
			{ID: "task-1", ParentID: strPtr("job-1"), Type: "Task", Name: "npm install", Order: 1},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Expand everything
	model.ToggleExpand() // expand stage
	model.MoveDown()
	model.ToggleExpand() // expand job

	// Now at index 1, move to task at index 2
	model.MoveDown()
	if model.SelectedIndex() != 2 {
		t.Fatalf("Expected selected index 2, got %d", model.SelectedIndex())
	}

	// Now collapse stage-1 from index 0
	model.selectedIndex = 0
	model.ToggleExpand()

	// After collapse, only 1 item (stage-1). Selection should clamp.
	if model.selectedIndex >= len(model.flatItems) {
		t.Errorf("Selection %d should be within bounds (len=%d)", model.selectedIndex, len(model.flatItems))
	}
}

func TestFlattenTreeRespectsExpanded(t *testing.T) {
	// Build a tree manually with Expanded flags
	tree := []*TimelineNode{
		{
			Record:   azdevops.TimelineRecord{ID: "s1", Type: "Stage", Name: "Build"},
			Expanded: true,
			Children: []*TimelineNode{
				{
					Record:   azdevops.TimelineRecord{ID: "j1", Type: "Job", Name: "Build Job"},
					Expanded: false,
					Children: []*TimelineNode{
						{Record: azdevops.TimelineRecord{ID: "t1", Type: "Task", Name: "npm install"}},
					},
				},
			},
		},
		{
			Record:   azdevops.TimelineRecord{ID: "s2", Type: "Stage", Name: "Test"},
			Expanded: false,
			Children: []*TimelineNode{
				{Record: azdevops.TimelineRecord{ID: "j2", Type: "Job", Name: "Test Job"}},
			},
		},
	}

	flat := flattenTree(tree)

	// s1 expanded → shows s1, j1. j1 collapsed → hides t1. s2 collapsed → hides j2.
	expectedNames := []string{"Build", "Build Job", "Test"}
	if len(flat) != len(expectedNames) {
		t.Fatalf("Expected %d flat items, got %d", len(expectedNames), len(flat))
	}

	for i, name := range expectedNames {
		if flat[i].Record.Name != name {
			t.Errorf("flat[%d] = %q, want %q", i, flat[i].Record.Name, name)
		}
	}
}

func TestDisplayFiltersPhaseAndCheckpoint(t *testing.T) {
	// Azure DevOps timelines include Phase and Checkpoint intermediary records.
	// These should be hidden in the UI but their children shown as if direct.
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "checkpoint-1", ParentID: strPtr("stage-1"), Type: "Checkpoint", Name: "Checkpoint", Order: 1},
			{ID: "phase-1", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase 1", Order: 2},
			{ID: "job-1", ParentID: strPtr("phase-1"), Type: "Job", Name: "Build Job", Order: 1,
				Log: &azdevops.LogReference{ID: 5}},
			{ID: "task-1", ParentID: strPtr("job-1"), Type: "Task", Name: "npm install", Order: 1},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Collapsed: only stage visible (Phase and Checkpoint hidden)
	if len(model.flatItems) != 1 {
		t.Fatalf("Expected 1 flat item (collapsed), got %d", len(model.flatItems))
	}
	if model.flatItems[0].Record.Name != "Build" {
		t.Errorf("First item = %q, want Build", model.flatItems[0].Record.Name)
	}

	// Stage should report having children (Jobs visible through Phase)
	if !model.flatItems[0].HasChildren() {
		t.Error("Stage with Phase→Job children should report HasChildren=true")
	}

	// Expand stage: should show Job (Phase/Checkpoint filtered out)
	model.ToggleExpand()
	if len(model.flatItems) != 2 {
		t.Fatalf("Expected 2 flat items after expand, got %d", len(model.flatItems))
	}
	if model.flatItems[1].Record.Name != "Build Job" {
		t.Errorf("Second item = %q, want Build Job", model.flatItems[1].Record.Name)
	}

	// Expand job: should show Task
	model.MoveDown()
	model.ToggleExpand()
	if len(model.flatItems) != 3 {
		t.Fatalf("Expected 3 flat items, got %d", len(model.flatItems))
	}
	if model.flatItems[2].Record.Name != "npm install" {
		t.Errorf("Third item = %q, want npm install", model.flatItems[2].Record.Name)
	}
}

func TestDisplayJobWithLogNoTasksIsLeaf(t *testing.T) {
	// When a Job (under a Phase) has a log but no tasks, it should be a leaf node
	// (no ▶ icon, Enter opens log)
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "phase-1", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase 1", Order: 1},
			{ID: "job-1", ParentID: strPtr("phase-1"), Type: "Job", Name: "Build Job", Order: 1,
				Log: &azdevops.LogReference{ID: 5}},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Collapsed: only stage visible
	if len(model.flatItems) != 1 {
		t.Fatalf("Expected 1 flat item (collapsed), got %d", len(model.flatItems))
	}

	// Expand stage
	model.ToggleExpand()

	// Should show stage + job (Phase filtered from display)
	if len(model.flatItems) != 2 {
		t.Fatalf("Expected 2 flat items after expand, got %d", len(model.flatItems))
	}

	// Job should report no visible children (it's a leaf with a log)
	job := model.flatItems[1]
	if job.HasChildren() {
		t.Error("Job with no tasks should not have visible children")
	}
	if job.Record.Log == nil {
		t.Error("Job should have a log")
	}
}

func TestDisplayMultipleJobsUnderPhase(t *testing.T) {
	// All jobs under a stage (through Phases) should be hidden when collapsed
	// and shown when expanded — no job should leak out as a root
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Deploy", Order: 1},
			{ID: "checkpoint-1", ParentID: strPtr("stage-1"), Type: "Checkpoint", Name: "Approval", Order: 1},
			{ID: "phase-1", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase 1", Order: 2},
			{ID: "job-1", ParentID: strPtr("phase-1"), Type: "Job", Name: "Job A", Order: 1},
			{ID: "phase-2", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase 2", Order: 3},
			{ID: "job-2", ParentID: strPtr("phase-2"), Type: "Job", Name: "Job B", Order: 1},
			{ID: "phase-3", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase 3", Order: 4},
			{ID: "job-3", ParentID: strPtr("phase-3"), Type: "Job", Name: "Job C", Order: 1},
			{ID: "phase-4", ParentID: strPtr("stage-1"), Type: "Phase", Name: "Phase 4", Order: 5},
			{ID: "job-4", ParentID: strPtr("phase-4"), Type: "Job", Name: "Job D", Order: 1,
				Log: &azdevops.LogReference{ID: 10}},
			{ID: "task-1", ParentID: strPtr("job-4"), Type: "Task", Name: "Deploy task", Order: 1},
		},
	}

	run := azdevops.PipelineRun{ID: 123, BuildNumber: "20240206.1"}
	model := NewDetailModel(nil, run)
	model.SetTimeline(timeline)

	// Collapsed: only the stage should be visible — ALL jobs hidden
	if len(model.flatItems) != 1 {
		t.Fatalf("Expected 1 flat item (collapsed stage), got %d", len(model.flatItems))
	}
	if model.flatItems[0].Record.Name != "Deploy" {
		t.Errorf("First item = %q, want Deploy", model.flatItems[0].Record.Name)
	}

	// Expand stage: should show all 4 jobs
	model.ToggleExpand()
	if len(model.flatItems) != 5 {
		t.Fatalf("Expected 5 flat items (stage + 4 jobs), got %d", len(model.flatItems))
	}

	expectedNames := []string{"Deploy", "Job A", "Job B", "Job C", "Job D"}
	for i, name := range expectedNames {
		if model.flatItems[i].Record.Name != name {
			t.Errorf("flatItems[%d] = %q, want %q", i, model.flatItems[i].Record.Name, name)
		}
	}
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
