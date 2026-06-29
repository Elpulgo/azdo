package pipelines

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestStatusIconWithStyles(t *testing.T) {
	themes := []string{"dark", "gruvbox", "nord", "dracula"}

	for _, themeName := range themes {
		t.Run(themeName, func(t *testing.T) {
			s := styles.NewStyles(styles.GetThemeByNameWithFallback(themeName))

			tests := []struct {
				runStatus    provider.RunStatus
				wantContains string
			}{
				{provider.RunStatusRunning, "Running"},
				{provider.RunStatusSucceeded, "Success"},
				{provider.RunStatusFailed, "Failed"},
			}

			for _, tt := range tests {
				got := statusIconWithStyles(tt.runStatus, s)
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("statusIconWithStyles(%v) with theme %s = %q, want to contain %q",
						tt.runStatus, themeName, got, tt.wantContains)
				}
			}
		})
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		name         string
		runStatus    provider.RunStatus
		wantContains string
	}{
		{
			name:         "Running shows Running",
			runStatus:    provider.RunStatusRunning,
			wantContains: "Running",
		},
		{
			name:         "Queued shows Queued",
			runStatus:    provider.RunStatusQueued,
			wantContains: "Queued",
		},
		{
			name:         "Canceling shows Cancel",
			runStatus:    provider.RunStatusCanceling,
			wantContains: "Cancel",
		},
		{
			name:         "Succeeded shows Success",
			runStatus:    provider.RunStatusSucceeded,
			wantContains: "Success",
		},
		{
			name:         "Failed shows Failed",
			runStatus:    provider.RunStatusFailed,
			wantContains: "Failed",
		},
		{
			name:         "Canceled shows Cancel",
			runStatus:    provider.RunStatusCanceled,
			wantContains: "Cancel",
		},
		{
			name:         "PartiallySucceeded shows Partial",
			runStatus:    provider.RunStatusPartiallySucceeded,
			wantContains: "Partial",
		},
		{
			name:         "Unknown shows hollow circle glyph",
			runStatus:    provider.RunStatusUnknown,
			wantContains: "○",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusIconWithStyles(tt.runStatus, styles.DefaultStyles())

			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("statusIconWithStyles(%v) = %q, want to contain %q",
					tt.runStatus, got, tt.wantContains)
			}
		})
	}
}

// TestStatusIconViaRun verifies that runsToRows uses run.RunStatus (the enum field)
// so that end-to-end mapping from wire status/result flows through to display correctly.
func TestStatusIconViaRun(t *testing.T) {
	s := styles.DefaultStyles()

	tests := []struct {
		name         string
		status       string
		result       string
		wantContains string
	}{
		{"inProgress → Running", "inProgress", "", "Running"},
		{"notStarted → Queued", "notStarted", "", "Queued"},
		{"canceling → Cancel", "canceling", "", "Cancel"},
		{"succeeded → Success", "completed", "succeeded", "Success"},
		{"failed → Failed", "completed", "failed", "Failed"},
		{"canceled → Cancel", "completed", "canceled", "Cancel"},
		{"partiallySucceeded → Partial", "completed", "partiallySucceeded", "Partial"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := provider.PipelineRun{
				Identity:  provider.Identity{ID: "1", Scope: "proj"},
				Status:    tt.status,
				Result:    tt.result,
				RunStatus: azdevops.MapRunStatus(tt.status, tt.result),
			}
			rows := runsToRows([]provider.PipelineRun{run}, s)
			if len(rows) != 1 {
				t.Fatalf("expected 1 row, got %d", len(rows))
			}
			statusCell := rows[0][0]
			if !strings.Contains(statusCell, tt.wantContains) {
				t.Errorf("status cell for %q/%q = %q, want to contain %q",
					tt.status, tt.result, statusCell, tt.wantContains)
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
	model.list = model.list.SetItems([]provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "123", Scope: "proj"},
			BuildNumber:    "20240206.1",
			Status:         "completed",
			Result:         "succeeded",
			DefinitionName: "CI Pipeline",
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
	model.list = model.list.SetItems([]provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "456", Scope: "proj"},
			BuildNumber:    "20240206.2",
			DefinitionName: "Build Pipeline",
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Get the detail adapter to set timeline
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &provider.Timeline{
		Identity: provider.Identity{ID: "test-timeline", Scope: "proj"},
		Records: []provider.TimelineRecord{
			{
				ID:    "task-1",
				Type:  "Task",
				Name:  "npm install",
				State: "completed",
				LogID: 10,
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
	model.list = model.list.SetItems([]provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "789", Scope: "proj"},
			BuildNumber:    "20240206.3",
			DefinitionName: "Test Pipeline",
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Set timeline without log reference
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &provider.Timeline{
		Identity: provider.Identity{ID: "test-timeline", Scope: "proj"},
		Records: []provider.TimelineRecord{
			{
				ID:    "stage-1",
				Type:  "Stage",
				Name:  "Build Stage",
				State: "completed",
				LogID: 0, // No log
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

	items := []provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "123", Scope: "proj"},
			BuildNumber:    "20240210.1",
			Status:         "completed",
			Result:         "succeeded",
			SourceBranch:   "refs/heads/main",
			QueueTime:      queueTime,
			StartTime:      &startTime,
			FinishTime:     &finishTime,
			DefinitionName: "CI Pipeline",
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

	model.list = model.list.SetItems([]provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "123", Scope: "proj"},
			BuildNumber:    "20240206.1",
			DefinitionName: "Build Pipeline",
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Set timeline with stage containing children
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &provider.Timeline{
		Identity: provider.Identity{ID: "test", Scope: "proj"},
		Records: []provider.TimelineRecord{
			{ID: "stage-1", ParentID: "", Type: "Stage", Name: "Build", Order: 1},
			{ID: "job-1", ParentID: "stage-1", Type: "Job", Name: "Build Job", Order: 1,
				LogID: 10},
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

	model.list = model.list.SetItems([]provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "123", Scope: "proj"},
			BuildNumber:    "20240206.1",
			DefinitionName: "Build Pipeline",
		},
	})

	// Enter detail view
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Set timeline with a single task (no children, has log)
	adapter := model.list.Detail().(*detailAdapter)
	timeline := &provider.Timeline{
		Identity: provider.Identity{ID: "test", Scope: "proj"},
		Records: []provider.TimelineRecord{
			{ID: "task-1", ParentID: "", Type: "Task", Name: "npm install", Order: 1,
				LogID: 10},
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
	run := provider.PipelineRun{
		BuildNumber:    "20240210.1",
		SourceBranch:   "refs/heads/feature/deploy",
		DefinitionName: "CI Pipeline",
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
	model.list = model.list.SetItems([]provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "1", Scope: "proj"},
			BuildNumber:    "1",
			DefinitionName: "test",
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

func TestRunsToRowsMulti_IncludesProjectColumn(t *testing.T) {
	s := styles.DefaultStyles()
	items := []provider.PipelineRun{
		{
			Identity:       provider.Identity{ID: "1", Scope: "alpha", ScopeDisplay: "alpha"},
			BuildNumber:    "20240210.1",
			Status:         "completed",
			Result:         "succeeded",
			SourceBranch:   "refs/heads/main",
			QueueTime:      time.Now(),
			DefinitionName: "CI",
		},
	}

	rows := runsToRowsMulti(items, s)
	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if len(row) != 7 {
		t.Fatalf("Expected 7 columns (with Project), got %d", len(row))
	}
	if row[0] != "alpha" {
		t.Errorf("Project column = %q, want 'alpha'", row[0])
	}
}

func TestUpdate_PipelineRunsMsg_BubblesCriticalError(t *testing.T) {
	model := NewModel(nil)
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Send a pipelineRunsMsg with a critical error (HTTP 400)
	criticalErr := fmt.Errorf("all projects failed: [HTTP request failed with status 400]")
	model, cmd := model.Update(pipelineRunsMsg{runs: nil, err: criticalErr})


	if cmd == nil {
		t.Fatal("Expected a command to be returned for critical error, got nil")
	}

	// Execute the command and verify it produces a CriticalErrorMsg
	msg := cmd()
	if _, ok := msg.(components.CriticalErrorMsg); !ok {
		t.Errorf("Expected CriticalErrorMsg, got %T", msg)
	}

	// Critical error should NOT show inline in the list view
	view := model.View()
	if strings.Contains(view, "Error loading") {
		t.Error("Critical error should not be displayed inline in the list view")
	}
}

func TestUpdate_PipelineRunsMsg_NonCriticalErrorShowsInline(t *testing.T) {
	model := NewModel(nil)
	model.list, _ = model.list.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Send a pipelineRunsMsg with a non-critical error
	transientErr := fmt.Errorf("connection timeout")
	model, cmd := model.Update(pipelineRunsMsg{runs: nil, err: transientErr})

	if cmd != nil {
		t.Error("Expected nil command for non-critical error, got non-nil")
	}

	// Non-critical error should still show inline
	view := model.View()
	if !strings.Contains(view, "Error loading") {
		t.Error("Non-critical error should be displayed inline in the list view")
	}
}

func TestUpdate_PipelineRunsMsg_NoCmdForSuccess(t *testing.T) {
	model := NewModel(nil)

	// Send a successful pipelineRunsMsg
	_, cmd := model.Update(pipelineRunsMsg{runs: []provider.PipelineRun{}, err: nil})

	if cmd != nil {
		t.Error("Expected nil command for successful fetch, got non-nil")
	}
}

func TestFilterPipelineRunMulti_MatchesProjectName(t *testing.T) {
	run := provider.PipelineRun{
		Identity:       provider.Identity{ID: "1", Scope: "alpha", ScopeDisplay: "alpha"},
		BuildNumber:    "20240210.1",
		SourceBranch:   "refs/heads/main",
		DefinitionName: "CI",
	}

	if !filterPipelineRunMulti(run, "alpha") {
		t.Error("filterPipelineRunMulti should match project name 'alpha'")
	}
	if filterPipelineRunMulti(run, "beta") {
		t.Error("filterPipelineRunMulti should not match 'beta'")
	}
}
