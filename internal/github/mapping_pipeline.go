package github

import (
	"fmt"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapPipelineRun maps a GitHub Actions WorkflowRun wire type to a
// provider.PipelineRun.
//
// scope is the "owner/repo" string and scopeDisplay is its human-readable
// equivalent; both are stamped onto Identity, mirroring the azdevops convention.
//
// DefinitionID is always 0: the GitHub wire type used here (WorkflowRun) does not
// carry workflow_id in the fields defined for this phase. No UI consumer reads
// DefinitionID as a grouping or filter key (grep of internal/ui/pipelines/
// confirms it is absent from list.go and detail.go); it is present on the neutral
// struct for future use.
//
// FinishTime is approximated from UpdatedAt when status == "completed". GitHub
// Actions exposes no explicit finished_at timestamp on the run object. For
// in-progress or queued runs FinishTime is nil. A local copy of run.UpdatedAt is
// taken before &-addressing to avoid taking the address of a struct field on a
// value receiver.
//
// StartTime maps to run_started_at (already *time.Time on the wire type). It is
// nil for queued runs that have not yet been dispatched to a runner.
func MapPipelineRun(run WorkflowRun, scope, scopeDisplay string) provider.PipelineRun {
	var finishTime *time.Time
	if strings.ToLower(run.Status) == "completed" {
		// Copy into a local variable before taking its address — avoids aliasing
		// issues when WorkflowRun is passed by value and Go reuses the stack frame.
		updatedAt := run.UpdatedAt
		finishTime = &updatedAt
	}

	return provider.PipelineRun{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", run.ID),
		},
		BuildNumber:    fmt.Sprintf("%d", run.RunNumber),
		Status:         run.Status,
		Result:         derefString(run.Conclusion),
		RunStatus:      MapRunStatus(run.Status, derefString(run.Conclusion)),
		SourceBranch:   run.HeadBranch,
		SourceVersion:  run.HeadSHA,
		QueueTime:      run.CreatedAt,
		StartTime:      run.RunStartedAt,
		FinishTime:     finishTime,
		DefinitionID:   0, // workflow_id not present in the WorkflowRun wire type used by this phase
		DefinitionName: run.Name,
		WebURL:         run.HTMLURL,
	}
}

// MapTimeline maps a GitHub Actions WorkflowRun and its Jobs to a
// provider.Timeline.
//
// GitHub jobs/steps form a 2-level tree: jobs are roots, steps are their
// children. This mapper flattens that structure into a []TimelineRecord that
// the UI's buildTimelineTree can reconstruct into the correct hierarchy.
//
// Job records:
//   - ID:       fmt.Sprintf("%d", job.ID)
//   - ParentID: "" (jobs are timeline roots)
//   - Type:     "Job" (rendered, not filtered — see isFilteredRecordType)
//   - LogID:    int(job.ID)  (per Decision 6, logs are per-job)
//
// Step records:
//   - ID:       fmt.Sprintf("%d-%d", job.ID, step.Number)
//   - ParentID: fmt.Sprintf("%d", job.ID)
//   - Type:     "Task" (rendered, not filtered)
//   - LogID:    0  (steps share their job's log)
//
// State and Result strings are translated to the Azure DevOps vocabulary that
// recordIconWithStyles in detail.go expects (see mapTimelineState /
// mapTimelineResult below).
func MapTimeline(run WorkflowRun, jobs []Job, scope, scopeDisplay string) provider.Timeline {
	records := make([]provider.TimelineRecord, 0, len(jobs)*4) // rough pre-alloc

	for j, job := range jobs {
		jobID := fmt.Sprintf("%d", job.ID)

		records = append(records, provider.TimelineRecord{
			ID:         jobID,
			ParentID:   "",
			Type:       "Job",
			Name:       job.Name,
			State:      mapTimelineState(job.Status),
			Result:     mapTimelineResult(derefString(job.Conclusion)),
			Order:      j + 1,
			StartTime:  job.StartedAt,
			FinishTime: job.CompletedAt,
			LogID:      int(job.ID),
		})

		for _, step := range job.Steps {
			records = append(records, provider.TimelineRecord{
				ID:         fmt.Sprintf("%d-%d", job.ID, step.Number),
				ParentID:   jobID,
				Type:       "Task",
				Name:       step.Name,
				State:      mapTimelineState(step.Status),
				Result:     mapTimelineResult(derefString(step.Conclusion)),
				Order:      step.Number,
				StartTime:  step.StartedAt,
				FinishTime: step.CompletedAt,
				LogID:      0, // steps share the job log; LogID 0 means no per-step log fetch
			})
		}
	}

	return provider.Timeline{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", run.ID),
		},
		Records: records,
	}
}

// mapTimelineState translates a GitHub Actions job/step status string into the
// Azure DevOps-style state string that recordIconWithStyles in detail.go expects.
//
// View switch (detail.go:587–606):
//
//	stateLower == "inprogress"  → in-progress glyph (●)
//	stateLower == "pending"     → pending glyph   (○)
//	(otherwise falls through to result)
//
// GitHub status → neutral state:
//
//	"in_progress"                                → "inProgress"
//	"completed"                                  → "completed"
//	"queued"/"waiting"/"pending"/"requested"     → "pending"
//	<default>                                    → "pending"
func mapTimelineState(ghStatus string) string {
	switch strings.ToLower(ghStatus) {
	case "in_progress":
		return "inProgress"
	case "completed":
		return "completed"
	case "queued", "waiting", "pending", "requested":
		return "pending"
	default:
		return "pending"
	}
}

// mapTimelineResult translates a GitHub Actions conclusion string into the
// Azure DevOps-style result string that recordIconWithStyles in detail.go
// expects.
//
// View switch (detail.go:587–606):
//
//	resultLower == "succeeded"              → ✓ (Success style)
//	resultLower == "succeededwithissues"    → ◐ (Warning style)
//	resultLower == "failed"                 → ✗ (Error style)
//	resultLower == "canceled"|"skipped"|"abandoned" → ○ (Muted style)
//	default                                 → ○ (Muted style)
//
// GitHub conclusion → neutral result:
//
//	"success"                               → "succeeded"
//	"failure"/"timed_out"/"startup_failure" → "failed"
//	"cancelled"                             → "canceled"  (GitHub double-L → view's single-L)
//	"skipped"                               → "skipped"
//	"neutral"                               → "succeededwithissues"
//	""/"action_required"/"stale"/<unknown>  → ""  (renders as default muted glyph)
func mapTimelineResult(ghConclusion string) string {
	switch strings.ToLower(ghConclusion) {
	case "success":
		return "succeeded"
	case "failure", "timed_out", "startup_failure":
		return "failed"
	case "cancelled":
		// GitHub spells it "cancelled" (British English); the view expects "canceled".
		return "canceled"
	case "skipped":
		return "skipped"
	case "neutral":
		return "succeededwithissues"
	default:
		// Covers "", "action_required", "stale", and any unknown future value.
		// An empty result causes the view to fall through to the default muted glyph.
		return ""
	}
}
