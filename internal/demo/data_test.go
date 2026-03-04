package demo

import (
	"testing"
)

func TestMockPullRequests(t *testing.T) {
	prs := mockPullRequests()

	if len(prs) < 6 {
		t.Errorf("expected at least 6 PRs, got %d", len(prs))
	}

	for i, pr := range prs {
		if pr.ID == 0 {
			t.Errorf("PR[%d]: ID should not be zero", i)
		}
		if pr.Title == "" {
			t.Errorf("PR[%d]: Title should not be empty", i)
		}
		if pr.Status != "active" {
			t.Errorf("PR[%d]: Status should be 'active', got %q", i, pr.Status)
		}
		if pr.SourceRefName == "" {
			t.Errorf("PR[%d]: SourceRefName should not be empty", i)
		}
		if pr.TargetRefName == "" {
			t.Errorf("PR[%d]: TargetRefName should not be empty", i)
		}
		if pr.CreatedBy.DisplayName == "" {
			t.Errorf("PR[%d]: CreatedBy.DisplayName should not be empty", i)
		}
		if pr.Repository.ID == "" {
			t.Errorf("PR[%d]: Repository.ID should not be empty", i)
		}
		if pr.CreationDate.IsZero() {
			t.Errorf("PR[%d]: CreationDate should not be zero", i)
		}
	}

	// Check that at least one PR has reviewers
	hasReviewers := false
	for _, pr := range prs {
		if len(pr.Reviewers) > 0 {
			hasReviewers = true
			break
		}
	}
	if !hasReviewers {
		t.Error("at least one PR should have reviewers")
	}

	// Check that at least one PR is a draft
	hasDraft := false
	for _, pr := range prs {
		if pr.IsDraft {
			hasDraft = true
			break
		}
	}
	if !hasDraft {
		t.Error("at least one PR should be a draft")
	}
}

func TestMockWorkItems(t *testing.T) {
	items := mockWorkItems()

	if len(items) < 8 {
		t.Errorf("expected at least 8 work items, got %d", len(items))
	}

	types := make(map[string]bool)
	states := make(map[string]bool)

	for i, wi := range items {
		if wi.ID == 0 {
			t.Errorf("WorkItem[%d]: ID should not be zero", i)
		}
		if wi.Fields.Title == "" {
			t.Errorf("WorkItem[%d]: Title should not be empty", i)
		}
		if wi.Fields.WorkItemType == "" {
			t.Errorf("WorkItem[%d]: WorkItemType should not be empty", i)
		}
		if wi.Fields.State == "" {
			t.Errorf("WorkItem[%d]: State should not be empty", i)
		}
		if wi.Fields.Priority < 1 || wi.Fields.Priority > 4 {
			t.Errorf("WorkItem[%d]: Priority should be 1-4, got %d", i, wi.Fields.Priority)
		}
		if wi.Fields.ChangedDate.IsZero() {
			t.Errorf("WorkItem[%d]: ChangedDate should not be zero", i)
		}

		types[wi.Fields.WorkItemType] = true
		states[wi.Fields.State] = true
	}

	// Should have multiple work item types
	if len(types) < 2 {
		t.Errorf("expected at least 2 work item types, got %d: %v", len(types), types)
	}

	// Should have multiple states
	if len(states) < 2 {
		t.Errorf("expected at least 2 states, got %d: %v", len(states), states)
	}
}

func TestMockPipelineRuns(t *testing.T) {
	runs := mockPipelineRuns()

	if len(runs) < 8 {
		t.Errorf("expected at least 8 pipeline runs, got %d", len(runs))
	}

	results := make(map[string]bool)
	statuses := make(map[string]bool)

	for i, run := range runs {
		if run.ID == 0 {
			t.Errorf("PipelineRun[%d]: ID should not be zero", i)
		}
		if run.BuildNumber == "" {
			t.Errorf("PipelineRun[%d]: BuildNumber should not be empty", i)
		}
		if run.Definition.Name == "" {
			t.Errorf("PipelineRun[%d]: Definition.Name should not be empty", i)
		}
		if run.SourceBranch == "" {
			t.Errorf("PipelineRun[%d]: SourceBranch should not be empty", i)
		}
		if run.QueueTime.IsZero() {
			t.Errorf("PipelineRun[%d]: QueueTime should not be zero", i)
		}

		results[run.Result] = true
		statuses[run.Status] = true
	}

	// Should have multiple results
	if len(results) < 2 {
		t.Errorf("expected at least 2 result types, got %d: %v", len(results), results)
	}
}

func TestMockThreads(t *testing.T) {
	threads := mockThreads()

	if len(threads) < 3 {
		t.Errorf("expected at least 3 threads, got %d", len(threads))
	}

	hasCodeComment := false
	hasGeneralComment := false

	for i, thread := range threads {
		if thread.ID == 0 {
			t.Errorf("Thread[%d]: ID should not be zero", i)
		}
		if len(thread.Comments) == 0 {
			t.Errorf("Thread[%d]: should have at least one comment", i)
		}
		for j, comment := range thread.Comments {
			if comment.Content == "" {
				t.Errorf("Thread[%d].Comment[%d]: Content should not be empty", i, j)
			}
			if comment.Author.DisplayName == "" {
				t.Errorf("Thread[%d].Comment[%d]: Author.DisplayName should not be empty", i, j)
			}
		}

		if thread.ThreadContext != nil && thread.ThreadContext.FilePath != "" {
			hasCodeComment = true
		} else {
			hasGeneralComment = true
		}
	}

	if !hasCodeComment {
		t.Error("expected at least one code comment thread")
	}
	if !hasGeneralComment {
		t.Error("expected at least one general comment thread")
	}
}

func TestMockTimeline(t *testing.T) {
	timeline := mockTimeline()

	if timeline.ID == "" {
		t.Error("Timeline.ID should not be empty")
	}

	if len(timeline.Records) < 5 {
		t.Errorf("expected at least 5 timeline records, got %d", len(timeline.Records))
	}

	types := make(map[string]bool)
	for _, r := range timeline.Records {
		types[r.Type] = true
		if r.Name == "" {
			t.Errorf("TimelineRecord %q: Name should not be empty", r.ID)
		}
	}

	// Should have stages, jobs, and tasks
	if !types["Stage"] {
		t.Error("expected at least one Stage record")
	}
	if !types["Job"] {
		t.Error("expected at least one Job record")
	}
	if !types["Task"] {
		t.Error("expected at least one Task record")
	}
}

func TestMockWorkItemTypeStates(t *testing.T) {
	for _, wiType := range []string{"Bug", "User Story", "Task"} {
		states := mockWorkItemTypeStates(wiType)
		if len(states) < 3 {
			t.Errorf("%s: expected at least 3 states, got %d", wiType, len(states))
		}

		for i, s := range states {
			if s.Name == "" {
				t.Errorf("%s state[%d]: Name should not be empty", wiType, i)
			}
			if s.Color == "" {
				t.Errorf("%s state[%d]: Color should not be empty", wiType, i)
			}
			if s.Category == "" {
				t.Errorf("%s state[%d]: Category should not be empty", wiType, i)
			}
		}
	}
}

func TestMockPRIterations(t *testing.T) {
	iterations := mockPRIterations()

	if len(iterations) < 2 {
		t.Errorf("expected at least 2 iterations, got %d", len(iterations))
	}

	for i, iter := range iterations {
		if iter.ID == 0 {
			t.Errorf("Iteration[%d]: ID should not be zero", i)
		}
	}
}

func TestMockIterationChanges(t *testing.T) {
	changes := mockIterationChanges()

	if len(changes) < 3 {
		t.Errorf("expected at least 3 iteration changes, got %d", len(changes))
	}

	changeTypes := make(map[string]bool)
	for i, c := range changes {
		if c.Item.Path == "" {
			t.Errorf("IterationChange[%d]: Item.Path should not be empty", i)
		}
		changeTypes[c.ChangeType] = true
	}

	// Should have at least 2 different change types
	if len(changeTypes) < 2 {
		t.Errorf("expected at least 2 change types, got %d: %v", len(changeTypes), changeTypes)
	}
}
