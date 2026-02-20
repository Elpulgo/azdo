package pipelines

import (
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel_HasStyles(t *testing.T) {
	m := NewModel(nil)

	if m.styles == nil {
		t.Error("Expected model to have styles initialized")
	}
}

func TestNewModelWithStyles(t *testing.T) {
	customStyles := styles.NewStyles(styles.GetThemeByNameWithFallback("gruvbox"))
	m := NewModelWithStyles(nil, customStyles)

	if m.styles != customStyles {
		t.Error("Expected model to use provided custom styles")
	}
}

func TestStatusIconWithStyles(t *testing.T) {
	themes := []string{"dark", "gruvbox", "nord", "dracula"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			tests := []struct {
				status, result string
				wantContains   string
			}{
				{"inProgress", "", "Running"},
				{"completed", "succeeded", "Success"},
				{"completed", "failed", "Failed"},
			}

			for _, tt := range tests {
				got := statusIconWithStyles(tt.status, tt.result, s)
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("statusIconWithStyles(%q, %q) with theme %s = %q, want to contain %q",
						tt.status, tt.result, themeName, got, tt.wantContains)
				}
			}
		})
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		result         string
		wantContains   string
		wantNotContain string
	}{
		{
			name:         "inProgress status shows Running",
			status:       "inProgress",
			result:       "",
			wantContains: "Running",
		},
		{
			name:         "InProgress (capitalized) shows Running",
			status:       "InProgress",
			result:       "",
			wantContains: "Running",
		},
		{
			name:         "notStarted status shows Queued",
			status:       "notStarted",
			result:       "",
			wantContains: "Queued",
		},
		{
			name:         "NotStarted (capitalized) shows Queued",
			status:       "NotStarted",
			result:       "",
			wantContains: "Queued",
		},
		{
			name:         "canceling status shows Cancel",
			status:       "canceling",
			result:       "",
			wantContains: "Cancel",
		},
		{
			name:         "succeeded result shows Success",
			status:       "completed",
			result:       "succeeded",
			wantContains: "Success",
		},
		{
			name:         "failed result shows Failed",
			status:       "completed",
			result:       "failed",
			wantContains: "Failed",
		},
		{
			name:         "canceled result shows Cancel",
			status:       "completed",
			result:       "canceled",
			wantContains: "Cancel",
		},
		{
			name:         "partiallySucceeded result shows Partial",
			status:       "completed",
			result:       "partiallySucceeded",
			wantContains: "Partial",
		},
		{
			name:         "empty status and result shows debug format",
			status:       "",
			result:       "",
			wantContains: "/",
		},
		{
			name:         "unrecognized status shows debug format",
			status:       "somethingElse",
			result:       "",
			wantContains: "somethingElse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusIconWithStyles(tt.status, tt.result, styles.DefaultStyles())

			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("statusIconWithStyles(%q, %q) = %q, want to contain %q",
					tt.status, tt.result, got, tt.wantContains)
			}

			if tt.wantNotContain != "" && strings.Contains(got, tt.wantNotContain) {
				t.Errorf("statusIconWithStyles(%q, %q) = %q, should NOT contain %q",
					tt.status, tt.result, got, tt.wantNotContain)
			}
		})
	}
}

func TestViewModeNavigation(t *testing.T) {
	model := NewModel(nil)

	if model.GetViewMode() != ViewList {
		t.Errorf("Initial ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}

	// Simulate having some runs loaded
	model.list = model.list.SetItems([]azdevops.PipelineRun{
		{
			ID:          123,
			BuildNumber: "20240206.1",
			Status:      "completed",
			Result:      "succeeded",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "CI Pipeline"},
		},
	})

	// Enter should transition to detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("After Enter, ViewMode = %d, want ViewDetail (%d)", model.GetViewMode(), ViewDetail)
	}

	// Detail model should be set
	if model.list.Detail() == nil {
		t.Error("After Enter, detail model should not be nil")
	}

	// Esc should go back to list
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.GetViewMode() != ViewList {
		t.Errorf("After Esc, ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}
}

func TestViewModeNavigationToLogs(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 24

	// Load runs and enter detail view
	model.list = model.list.SetItems([]azdevops.PipelineRun{
		{
			ID:          456,
			BuildNumber: "20240206.2",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "Build Pipeline"},
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Get the detail adapter to set timeline
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &azdevops.Timeline{
		ID: "test-timeline",
		Records: []azdevops.TimelineRecord{
			{
				ID:    "task-1",
				Type:  "Task",
				Name:  "npm install",
				State: "completed",
				Log:   &azdevops.LogReference{ID: 10},
			},
		},
	}
	adapter.model.SetTimeline(timeline)

	// Enter should transition to log view (since selected item has a log)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewLogs {
		t.Errorf("After Enter on item with log, ViewMode = %d, want ViewLogs (%d)", model.GetViewMode(), ViewLogs)
	}

	// Log viewer should be set
	if model.logViewer == nil {
		t.Error("After Enter on log item, logViewer should not be nil")
	}

	// Esc should go back to detail
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("After Esc from logs, ViewMode = %d, want ViewDetail (%d)", model.GetViewMode(), ViewDetail)
	}

	// Esc again should go back to list
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.GetViewMode() != ViewList {
		t.Errorf("After Esc from detail, ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}
}

func TestViewModeNoLogDoesNotTransition(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 24

	// Load runs and enter detail view
	model.list = model.list.SetItems([]azdevops.PipelineRun{
		{
			ID:          789,
			BuildNumber: "20240206.3",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "Test Pipeline"},
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Set timeline without log reference
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &azdevops.Timeline{
		ID: "test-timeline",
		Records: []azdevops.TimelineRecord{
			{
				ID:    "stage-1",
				Type:  "Stage",
				Name:  "Build Stage",
				State: "completed",
				Log:   nil, // No log
			},
		},
	}
	adapter.model.SetTimeline(timeline)

	// Enter should NOT transition to log view (no log available)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("Enter on item without log should stay in ViewDetail, got %d", model.GetViewMode())
	}
}

func TestRunsToRowsIncludesTimestamp(t *testing.T) {
	s := styles.DefaultStyles()

	queueTime := time.Date(2024, time.February, 10, 14, 30, 0, 0, time.UTC)
	startTime := time.Date(2024, time.February, 10, 14, 31, 0, 0, time.UTC)
	finishTime := time.Date(2024, time.February, 10, 14, 36, 0, 0, time.UTC)

	items := []azdevops.PipelineRun{
		{
			ID:           123,
			BuildNumber:  "20240210.1",
			Status:       "completed",
			Result:       "succeeded",
			SourceBranch: "refs/heads/main",
			QueueTime:    queueTime,
			StartTime:    &startTime,
			FinishTime:   &finishTime,
			Definition:   azdevops.PipelineDefinition{ID: 1, Name: "CI Pipeline"},
		},
	}

	rows := runsToRows(items, s)

	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if len(row) != 6 {
		t.Fatalf("Expected 6 columns, got %d", len(row))
	}

	expectedTimestamp := "2024-02-10 14:30"
	if row[4] != expectedTimestamp {
		t.Errorf("Timestamp column = %q, want %q", row[4], expectedTimestamp)
	}

	expectedDuration := "5m0s"
	if row[5] != expectedDuration {
		t.Errorf("Duration column = %q, want %q", row[5], expectedDuration)
	}
}

func TestDetailView_EnterTogglesExpandOnNodeWithChildren(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 24

	model.list = model.list.SetItems([]azdevops.PipelineRun{
		{
			ID:          123,
			BuildNumber: "20240206.1",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "Build Pipeline"},
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Set timeline with stage containing children
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "stage-1", ParentID: nil, Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: strPtr("stage-1"), Type: "Job", Name: "Build Job", Order: 1,
				Log: &azdevops.LogReference{ID: 10}},
		},
	}
	adapter.model.SetTimeline(timeline)

	// Initially stage is collapsed, only 1 item visible
	if len(adapter.model.flatItems) != 1 {
		t.Fatalf("Expected 1 flat item (collapsed), got %d", len(adapter.model.flatItems))
	}

	// Enter on stage should expand, NOT navigate to logs
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("Enter on expandable node should stay in ViewDetail, got %d", model.GetViewMode())
	}

	// Stage should now be expanded showing job too
	if len(adapter.model.flatItems) != 2 {
		t.Errorf("Expected 2 flat items after expanding stage, got %d", len(adapter.model.flatItems))
	}
}

func TestDetailView_EnterOnLeafWithLogsOpensLogViewer(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 24

	model.list = model.list.SetItems([]azdevops.PipelineRun{
		{
			ID:          123,
			BuildNumber: "20240206.1",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "Build Pipeline"},
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Set timeline with a single task (no children, has log)
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &azdevops.Timeline{
		ID: "test",
		Records: []azdevops.TimelineRecord{
			{ID: "task-1", ParentID: nil, Type: "Task", Name: "npm install", Order: 1,
				Log: &azdevops.LogReference{ID: 10}},
		},
	}
	adapter.model.SetTimeline(timeline)

	// Enter on leaf node with log should open log viewer
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewLogs {
		t.Errorf("Enter on leaf node with log should open ViewLogs, got %d", model.GetViewMode())
	}
}

func TestFilterPipelineRun(t *testing.T) {
	run := azdevops.PipelineRun{
		BuildNumber:  "20240210.1",
		SourceBranch: "refs/heads/feature/deploy",
		Definition:   azdevops.PipelineDefinition{Name: "CI Pipeline"},
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"CI Pipeline", true},   // matches pipeline name
		{"ci pipe", true},       // case-insensitive
		{"deploy", true},        // matches branch
		{"20240210", true},      // matches build number
		{"nonexistent", false},  // no match
		{"", true},              // empty matches all
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := filterPipelineRun(run, tt.query)
			if got != tt.want {
				t.Errorf("filterPipelineRun(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestMakeColumnsHasSixColumns(t *testing.T) {
	model := NewModelWithStyles(nil, styles.DefaultStyles())
	// Trigger resize to generate columns
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Verify by checking table view contains expected headers
	model.list = model.list.SetItems([]azdevops.PipelineRun{
		{
			ID:          1,
			BuildNumber: "1",
			Definition:  azdevops.PipelineDefinition{Name: "test"},
		},
	})

	view := model.View()
	expectedTitles := []string{"Status", "Pipeline", "Branch", "Build", "Timestamp", "Duration"}
	for _, title := range expectedTitles {
		if !strings.Contains(view, title) {
			t.Errorf("View should contain column title %q", title)
		}
	}
}
