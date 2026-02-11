package pipelines

import (
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		result         string
		wantContains   string
		wantNotContain string
	}{
		// In progress status
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

		// Not started status
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

		// Canceling status
		{
			name:         "canceling status shows Cancel",
			status:       "canceling",
			result:       "",
			wantContains: "Cancel",
		},

		// Result-based status (completed builds)
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

		// Unknown/default cases - now shows status/result for debugging
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
			got := statusIcon(tt.status, tt.result)

			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("statusIcon(%q, %q) = %q, want to contain %q",
					tt.status, tt.result, got, tt.wantContains)
			}

			if tt.wantNotContain != "" && strings.Contains(got, tt.wantNotContain) {
				t.Errorf("statusIcon(%q, %q) = %q, should NOT contain %q",
					tt.status, tt.result, got, tt.wantNotContain)
			}
		})
	}
}

func TestViewModeNavigation(t *testing.T) {
	// Create a model with no client (we won't make API calls)
	model := NewModel(nil)

	// Initial state should be ViewList
	if model.GetViewMode() != ViewList {
		t.Errorf("Initial ViewMode = %d, want ViewList (%d)", model.GetViewMode(), ViewList)
	}

	// Simulate having some runs loaded
	model.runs = []azdevops.PipelineRun{
		{
			ID:          123,
			BuildNumber: "20240206.1",
			Status:      "completed",
			Result:      "succeeded",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "CI Pipeline"},
		},
	}
	model.table.SetRows(model.runsToRows())

	// Enter should transition to detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("After Enter, ViewMode = %d, want ViewDetail (%d)", model.GetViewMode(), ViewDetail)
	}

	// Detail model should be set
	if model.detail == nil {
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
	model.runs = []azdevops.PipelineRun{
		{
			ID:          456,
			BuildNumber: "20240206.2",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "Build Pipeline"},
		},
	}
	model.table.SetRows(model.runsToRows())

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Simulate timeline with a log reference
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
	model.detail.SetTimeline(timeline)

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
	model.runs = []azdevops.PipelineRun{
		{
			ID:          789,
			BuildNumber: "20240206.3",
			Definition:  azdevops.PipelineDefinition{ID: 1, Name: "Test Pipeline"},
		},
	}
	model.table.SetRows(model.runsToRows())

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Simulate timeline without log reference
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
	model.detail.SetTimeline(timeline)

	// Enter should NOT transition to log view (no log available)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.GetViewMode() != ViewDetail {
		t.Errorf("Enter on item without log should stay in ViewDetail, got %d", model.GetViewMode())
	}
}

func TestRunsToRowsIncludesTimestamp(t *testing.T) {
	model := NewModel(nil)

	queueTime := time.Date(2024, time.February, 10, 14, 30, 0, 0, time.UTC)
	startTime := time.Date(2024, time.February, 10, 14, 31, 0, 0, time.UTC)
	finishTime := time.Date(2024, time.February, 10, 14, 36, 0, 0, time.UTC)

	model.runs = []azdevops.PipelineRun{
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

	rows := model.runsToRows()

	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	// Row should have 6 columns: Status, Pipeline, Branch, Build, Timestamp, Duration
	if len(row) != 6 {
		t.Fatalf("Expected 6 columns, got %d", len(row))
	}

	// Check timestamp column (index 4)
	expectedTimestamp := "2024-02-10 14:30"
	if row[4] != expectedTimestamp {
		t.Errorf("Timestamp column = %q, want %q", row[4], expectedTimestamp)
	}

	// Check duration column (index 5)
	expectedDuration := "5m0s"
	if row[5] != expectedDuration {
		t.Errorf("Duration column = %q, want %q", row[5], expectedDuration)
	}
}

func TestMakeColumnsHasSixColumns(t *testing.T) {
	columns := makeColumns(120)

	if len(columns) != 6 {
		t.Fatalf("Expected 6 columns, got %d", len(columns))
	}

	// Verify column titles
	expectedTitles := []string{"Status", "Pipeline", "Branch", "Build", "Timestamp", "Duration"}
	for i, expected := range expectedTitles {
		if columns[i].Title != expected {
			t.Errorf("Column %d title = %q, want %q", i, columns[i].Title, expected)
		}
	}
}
