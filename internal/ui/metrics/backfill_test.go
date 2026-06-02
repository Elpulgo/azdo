package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
)

func bfNow() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

// makeWorkItem builds a minimal WorkItem fixture suitable for backfill tests.
func makeWorkItem(id int, project, state, assignee string) azdevops.WorkItem {
	wi := azdevops.WorkItem{
		ID:          id,
		ProjectName: project,
	}
	wi.Fields.State = state
	wi.Fields.IterationPath = "sprint-1"
	wi.Fields.Tags = "sprint-1"
	wi.Fields.StoryPoints = 3
	if assignee != "" {
		wi.Fields.AssignedTo = &azdevops.Identity{DisplayName: assignee}
	}
	return wi
}

func TestBuildBackfillRows_HappyPath_SynthesizesRowsTaggedSourceUpdates(t *testing.T) {
	now := bfNow()
	items := []azdevops.WorkItem{
		makeWorkItem(101, "proj-a", "Closed", "Alice"),
	}
	fetch := func(project string, id int) ([]azdevops.WorkItemStateTransition, error) {
		if project != "proj-a" || id != 101 {
			t.Fatalf("unexpected fetch call: project=%q id=%d", project, id)
		}
		return []azdevops.WorkItemStateTransition{
			{State: "Active", At: now.AddDate(0, 0, -10)},
			{State: "Ready for Test", At: now.AddDate(0, 0, -5)},
			{State: "Closed", At: now.AddDate(0, 0, -2)},
		}, nil
	}

	rows, skipped := buildBackfillRows(items, fetch, now, 2)

	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(rows) == 0 {
		t.Fatal("rows empty; expected synthesized backfill rows")
	}
	for _, r := range rows {
		if r.Source != coremetrics.SourceUpdates {
			t.Errorf("row %v Source = %q, want %q", r, r.Source, coremetrics.SourceUpdates)
		}
		if r.ID != 101 {
			t.Errorf("row ID = %d, want 101", r.ID)
		}
		if r.Project != "proj-a" {
			t.Errorf("row Project = %q, want proj-a", r.Project)
		}
		if r.AssignedTo != "Alice" {
			t.Errorf("row AssignedTo = %q, want Alice", r.AssignedTo)
		}
	}
}

func TestBuildBackfillRows_FetchError_IncrementsSkipped(t *testing.T) {
	now := bfNow()
	items := []azdevops.WorkItem{
		makeWorkItem(1, "proj-a", "Active", "Alice"),
		makeWorkItem(2, "proj-a", "Closed", "Bob"),
	}
	fetch := func(project string, id int) ([]azdevops.WorkItemStateTransition, error) {
		if id == 1 {
			return nil, errors.New("network timeout")
		}
		return []azdevops.WorkItemStateTransition{
			{State: "Active", At: now.AddDate(0, 0, -10)},
			{State: "Closed", At: now.AddDate(0, 0, -3)},
		}, nil
	}

	rows, skipped := buildBackfillRows(items, fetch, now, 2)

	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	for _, r := range rows {
		if r.ID == 1 {
			t.Errorf("got row for failed-fetch item 1: %+v", r)
		}
	}
	if len(rows) == 0 {
		t.Error("expected at least some rows from the successful item 2")
	}
}

func TestBuildBackfillRows_EmptyItems_ReturnsNothing(t *testing.T) {
	rows, skipped := buildBackfillRows(nil, func(string, int) ([]azdevops.WorkItemStateTransition, error) {
		t.Fatal("fetch should not be called for empty items")
		return nil, nil
	}, bfNow(), 4)
	if rows != nil {
		t.Errorf("rows = %v, want nil", rows)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}

func TestBuildBackfillRows_NoTransitions_ProducesNoRows(t *testing.T) {
	items := []azdevops.WorkItem{makeWorkItem(7, "proj-a", "New", "Alice")}
	fetch := func(string, int) ([]azdevops.WorkItemStateTransition, error) {
		return nil, nil // item exists but has no recorded state transitions
	}
	rows, skipped := buildBackfillRows(items, fetch, bfNow(), 2)
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0 (empty transition list is not a failure)", skipped)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestBuildBackfillRows_OnlyEmitsWithin90DayWindow(t *testing.T) {
	now := bfNow()
	items := []azdevops.WorkItem{makeWorkItem(42, "proj-a", "Closed", "Carol")}
	fetch := func(string, int) ([]azdevops.WorkItemStateTransition, error) {
		return []azdevops.WorkItemStateTransition{
			{State: "Active", At: now.AddDate(0, 0, -200)},
			{State: "Closed", At: now.AddDate(0, 0, -100)},
		}, nil
	}
	rows, _ := buildBackfillRows(items, fetch, now, 2)

	cutoff := now.AddDate(0, 0, -90)
	for _, r := range rows {
		ts, _ := time.Parse("2006-01-02", r.TS)
		if ts.Before(cutoff) {
			t.Errorf("row TS %s is before 90-day cutoff %s", r.TS, cutoff.Format("2006-01-02"))
		}
		if !ts.Before(now) {
			t.Errorf("row TS %s is not before now %s", r.TS, now.Format("2006-01-02"))
		}
	}
}
