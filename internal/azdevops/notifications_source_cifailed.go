package azdevops

import (
	"errors"
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// SourceCIFailed implements the "my failed pipeline runs" notification
// source (task 7 of the phase-2 notifications spec) →
// provider.NotificationReasonCIActivity — asserted by name in
// notifications_source_cifailed_test.go so a future ci_failed member cannot
// be silently swapped in for it. Decision 8 is explicit that ci_failed is
// not, and must not become, a member of provider.NotificationReason; the
// config's own sources.ci_failed toggle (decision 13's closing note) names
// what this source queries, not the reason it emits.
//
// It reuses MultiClient.ListPipelineRuns (multiclient.go:69) verbatim — no
// new client method — and filters the merged, already
// QueueTime-descending-sorted result down to runs that are both:
//
//  1. completed with a failed result (see isMyFailedRun's doc comment for
//     why both Status and Result are checked, not Result alone); and
//  2. requested for the authenticated user, compared via
//     PipelineRun.RequestedFor.ID == GetCurrentUserID() — a plain ==, no
//     normalising, the same exact-match comparison probe result (c) of
//     decision 3 established for CommentMention.targetId. RequestedFor
//     carries an id (not only a display name), so this source, like
//     SourceMentioned, compares by identity rather than scraping a
//     display name.
//
// Filtering here (in Go, after the fetch) rather than passing a
// requestedFor query parameter to ListPipelineRuns matches this task's own
// wording ("filters MultiClient.ListPipelineRuns") and keeps
// ListPipelineRuns's existing signature and every other caller (the
// pipelines pane) untouched.
//
// Unlike every other source in this package, each failed run is its own
// permanently stable subject: a specific run's FinishTime never moves once
// set, so ciFailedActivityStamp never has "new activity on the same
// subject" to report, and Reconcile's newer-stamp branch never fires for an
// already-seen run. A retried/rerun build that later turns green simply
// stops matching isMyFailedRun and stops appearing in this source's rows —
// decision 7's "implicit in recomputation" auto-expiry, the same mechanism
// every other source relies on, needs no special-casing here.
//
// The row's Read/Done fields are left at their zero value here, matching
// every other source: this function only computes the freshly-queried
// subject, the shape Reconcile (notifications_reconcile.go) expects as
// input.
//
// now is threaded through to ciFailedActivityStamp so a stamp can never
// land in the future; it is never read from time.Now() internally, matching
// every other source in this package.
//
// A partial multi-project failure (MultiClient.ListPipelineRuns returning
// runs alongside a *PartialError) does not discard the rows it did get,
// matching every other source's degrade-not-blank contract: the runs from
// surviving projects are filtered, mapped and returned alongside the error.
func SourceCIFailed(mc *MultiClient, top int, now time.Time) ([]provider.Notification, error) {
	if mc == nil {
		return nil, fmt.Errorf("no client configured")
	}

	userID, err := resolveCIFailedUserID(mc)
	if err != nil {
		return nil, err
	}

	runs, err := mc.ListPipelineRuns(top)
	if err != nil {
		var partialErr *PartialError
		if !errors.As(err, &partialErr) {
			return nil, err
		}
		// Partial failure: still filter, map and return the rows the
		// surviving projects gave us, alongside the error.
		return mapFailedRuns(mc, runs, userID, now), err
	}

	return mapFailedRuns(mc, runs, userID, now), nil
}

// resolveCIFailedUserID fetches the authenticated user's id from any one
// project client (all share the same PAT/org), matching
// resolveMentionUserID's pattern in notifications_source_mentioned.go:
// filtering "requested for me" happens in this source's own Go code (see
// SourceCIFailed's doc comment), not server-side, so it needs the id up
// front the same way stage 2 of the mention source does.
func resolveCIFailedUserID(mc *MultiClient) (string, error) {
	for _, p := range mc.Projects() {
		c := mc.ClientFor(p)
		if c == nil {
			continue
		}
		id, err := c.GetCurrentUserID()
		if err != nil {
			return "", fmt.Errorf("failed to get current user ID: %w", err)
		}
		return id, nil
	}
	return "", fmt.Errorf("no client configured")
}

// mapFailedRuns filters runs down to isMyFailedRun matches and maps each
// survivor to a provider.Notification. It does not re-sort: ListPipelineRuns
// already returns its merged result QueueTime-descending, and filtering
// (unlike a batch-then-id-order fetch) never reorders what it keeps.
func mapFailedRuns(mc *MultiClient, runs []PipelineRun, userID string, now time.Time) []provider.Notification {
	rows := make([]provider.Notification, 0, len(runs))
	for _, run := range runs {
		if !isMyFailedRun(run, userID) {
			continue
		}
		rows = append(rows, mapCIFailed(mc, run, now))
	}
	return rows
}

// isMyFailedRun reports whether run is a completed, failed run requested
// for userID.
//
// Both Status and Result are checked, not Result alone: Result only ever
// carries a terminal value ("succeeded", "failed", "canceled",
// "partiallySucceeded") once a run reaches Status "completed" — while a run
// is "inProgress", "canceling", "postponed" or "notStarted", Result is
// "none". Checking Status explicitly documents that dependency instead of
// leaning on it implicitly, and rules out a "canceled" run (a Status/Result
// pair genuinely different from "failed" — a build the user aborted, not
// one that broke) or a "partiallySucceeded" run ever being mistaken for a
// failure by some future change to Result's own value set.
//
// The comparison is a plain ==, no normalising — see SourceCIFailed's doc
// comment for why that is the comparison the data supports.
func isMyFailedRun(run PipelineRun, userID string) bool {
	if run.Status != "completed" || run.Result != "failed" {
		return false
	}
	return run.RequestedFor.ID == userID
}

// mapCIFailed maps a single wire PipelineRun (already tagged with
// ProjectName/ProjectDisplayName by MultiClient's fan-out) to a
// provider.Notification for the ci_activity reason (decision 8 — ci_failed
// is a source name, not a provider.NotificationReason member).
func mapCIFailed(mc *MultiClient, run PipelineRun, now time.Time) provider.Notification {
	scope := run.ProjectName
	scopeDisplay := run.ProjectDisplayName
	if scopeDisplay == "" {
		scopeDisplay = scope
	}

	return provider.Notification{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           NotifKey("cifail", "run", run.ID),
		},
		Title:     fmt.Sprintf("%s #%s", run.Definition.Name, run.BuildNumber),
		Reason:    provider.NotificationReasonCIActivity,
		UpdatedAt: ciFailedActivityStamp(run, now),
		WebURL:    ciFailedWebURL(mc, run),
	}
}

// ciFailedActivityStamp is the row's Notification.UpdatedAt: the run's
// FinishTime, the natural "when did this failure happen" stamp for a
// completed run. FinishTime is a *time.Time (nil while a run has not
// finished); a run isMyFailedRun already restricted to Status "completed"
// should always carry one, but QueueTime is the fallback should it ever
// arrive nil or zero (malformed data), mirroring prActivityStamp's and
// assignedActivityStamp's own fallback-to-an-earlier-but-always-populated
// field pattern.
//
// The stamp is clamped against now the same way every other source's stamp
// is: a build agent's clock can be skewed, and Reconcile stores whatever
// this returns as LastActivity unconditionally on its "newer" branch. Since
// each failed run is its own permanently stable subject (see SourceCIFailed's
// doc comment), this clamp is defensive rather than load-bearing the way it
// is for a PR's committer date or a work item's ChangedDate — but the
// contract every source in this package upholds ("never read time.Now()
// internally, now is always a parameter") stays uniform regardless.
func ciFailedActivityStamp(run PipelineRun, now time.Time) time.Time {
	stamp := run.QueueTime
	if run.FinishTime != nil && !run.FinishTime.IsZero() {
		stamp = *run.FinishTime
	}
	if stamp.After(now) {
		return now
	}
	return stamp
}

// ciFailedWebURL resolves a failed run's browser URL, following the same
// degradation ladder as reviewRequestedWebURL, assignedWebURL and
// mentionedWebURL (see any one of their doc comments for the full
// rationale), and the same "https://dev.azure.com/{org}/{project}/
// _build/results?buildId={id}" deep link Adapter.PipelineURL already
// builds for the pipelines pane's `o` (open in browser) action:
//
//   - No client for the run's scope (an unconfigured or unresolvable
//     project): "" — there is nothing to build even a project page from.
//   - PipelineRun.ID <= 0 (convention 11: guard positive identifiers with
//     <= 0, not == 0) — the deep link's buildId query value would not
//     validate: fall back to the project page.
//   - Otherwise, the deep link.
func ciFailedWebURL(mc *MultiClient, run PipelineRun) string {
	if mc == nil {
		return ""
	}
	c := mc.ClientFor(run.ProjectName)
	if c == nil {
		return ""
	}

	projectPage := fmt.Sprintf("https://dev.azure.com/%s/%s", c.GetOrg(), c.GetProject())
	if run.ID <= 0 {
		return projectPage
	}

	return fmt.Sprintf("https://dev.azure.com/%s/%s/_build/results?buildId=%d",
		c.GetOrg(), c.GetProject(), run.ID)
}
