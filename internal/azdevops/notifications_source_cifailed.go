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
// It calls the dedicated Client.ListMyFailedPipelineRuns (pipelines.go),
// **not** MultiClient.ListPipelineRuns: ListPipelineRuns fetches the N most
// recent builds in the project across all pipelines, all users and all
// results, and its $top window is spent before any "mine and failed"
// filtering could happen — on a busy project that lets a still-unread
// failure age out of the window and disappear from the feed. This is the
// only one of the four phase-2 sources whose narrowing was not server-side
// (review uses searchCriteria.reviewerId, assigned and mentioned use WIQL),
// which is why task 7 was reopened. ListPipelineRuns itself is untouched —
// the pipelines pane still calls it directly.
//
// lookbackDays bounds ListMyFailedPipelineRuns' minTime the same way it
// bounds SourceAssigned's WIQL window (decision 6): without it, this source
// would be the one source in the package for which
// notifications.azure.lookback_days (task 11) is silently not honoured.
//
// **Belt-and-braces re-check.** Even though ListMyFailedPipelineRuns asks
// the server to narrow by statusFilter, resultFilter and requestedFor, this
// function still re-applies isMyFailedRun in Go over every row the server
// returns. If a server parameter is ever ignored or mis-typed, degrading to
// "we fetched more than we needed" is acceptable; silently attributing
// someone else's failed run to the caller is not. This is pinned by
// TestSourceCIFailed_MapsFailedRunToNotification, whose fixture server
// (newPipelineRunServer) ignores every query parameter and returns a mixed
// bag of runs — other users', successful, in-progress — and asserts only
// the caller's own failed run survives.
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
// A partial multi-project failure (MultiClient.ListMyFailedPipelineRuns
// returning runs alongside a *PartialError) does not discard the rows it did
// get, matching every other source's degrade-not-blank contract: the runs
// from surviving projects are filtered, mapped and returned alongside the
// error.
//
// userID is resolved once by the caller (runSourcesConcurrently, via
// resolveAuthenticatedUserID) rather than by this function: SourceReviewRequested
// and SourceMentioned need the very same id at the very same time, and each
// independently calling resolveAuthenticatedUserID would race
// Client.userID's unsynchronized cache field when they land on the same
// *Client in the common single-project case (task 8 review, 🔴 finding 2).
// userID is used both as ListMyFailedPipelineRuns' requestedFor parameter
// and, unchanged from before, re-checked in Go by isMyFailedRun below.
func SourceCIFailed(mc *MultiClient, userID string, lookbackDays, top int, now time.Time) ([]provider.Notification, error) {
	if mc == nil {
		return nil, fmt.Errorf("no client configured")
	}

	runs, err := mc.ListMyFailedPipelineRuns(userID, lookbackDays, top)
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

// mapFailedRuns filters runs down to isMyFailedRun matches and maps each
// survivor to a provider.Notification.
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
// leaning on it implicitly, and rules out a "canceled" or
// "partiallySucceeded" run ever being mistaken for a failure by some future
// change to Result's own value set. Excluding "canceled" is still the right
// default, but not for the reason an earlier version of this comment gave:
// cancellation is not only something the user deliberately did. A job that
// exceeds timeoutInMinutes and a run the system cancels (agent lost, pool
// drained) both land on Status "completed", Result "canceled" too —
// cancellations are predominantly deliberate, but timeouts land in this same
// bucket and are deliberately not surfaced as failures here.
//
// The comparison is a plain ==, no normalising — an extrapolation from
// decision 3's probe (c), which established that CommentMention.targetId
// string-equals GetCurrentUserID()'s value verbatim; probe (c) itself only
// ran against the Comments payload, not against PipelineRun.RequestedFor.
// The List Builds 7.1 requestedFor query parameter is what actually
// establishes identity-id semantics for this payload: Microsoft documents it
// as filtering builds by "the ID of the user who requested the build",
// which ListMyFailedPipelineRuns already relies on server-side — this
// function's == re-checks that same identity comparison in Go, on the same
// field, as the belt-and-braces guard described in SourceCIFailed's doc
// comment.
//
// An empty userID matches nothing (returns false unconditionally): passing
// one through to this function is always a caller bug, since
// resolveCIFailedUserID rejects an empty id before either of this
// function's callers ever run.
func isMyFailedRun(run PipelineRun, userID string) bool {
	if userID == "" {
		return false
	}
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
		Title:     ciFailedTitle(run),
		Reason:    provider.NotificationReasonCIActivity,
		UpdatedAt: ciFailedActivityStamp(run, now),
		WebURL:    ciFailedWebURL(mc, run),
	}
}

// ciFailedTitle renders the row's Title so the failure is visible in the
// text this source controls, not only in a shared reason glyph/label/colour.
// GitHub emits ci_activity for successful runs too, so in the merged feed a
// green GitHub run and a broken Azure build would otherwise be
// typographically identical; decision 8 collapsed the *reason*, it did not
// say the failure itself should become invisible in the row.
//
// Falls back to "Run <id> failed" only when Definition.Name and BuildNumber
// are *both* empty, which is the one case where the format string would
// render nothing identifying at all (" # failed"). One of the two being empty
// is deliberately left alone — " #20260101.1 failed" and "Nightly # failed"
// still name the run well enough to act on, and substituting the id there
// would replace the identifier the user recognises with one they do not. Both
// partial cases are pinned in TestCIFailedTitle.
func ciFailedTitle(run PipelineRun) string {
	if run.Definition.Name == "" && run.BuildNumber == "" {
		return fmt.Sprintf("Run %d failed", run.ID)
	}
	return fmt.Sprintf("%s #%s failed", run.Definition.Name, run.BuildNumber)
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
// FinishTime is load-bearing, not a defensive clamp target only: Azure's
// *Rerun failed jobs* / *Rerun stage* actions re-execute inside the **same
// run id** on YAML pipelines (classic pipelines queue a new id instead), and
// on completion the server recomputes both Result and FinishTime for that
// id. So a run this source has already surfaced and the caller dismissed
// can fail again — same Identity.ID, later FinishTime — and Reconcile's
// newer-stamp branch is exactly what resurfaces it as unread. That is
// correct and wanted (decision 7's auto-expiry is the same mechanism in
// reverse: a rerun that goes green stops matching isMyFailedRun and the row
// disappears). Do not swap this stamp for QueueTime or any other field that
// is set once at run creation and never revised — that would silently make
// a re-failed build stop resurfacing, with no test catching the change:
// TestSourceCIFailed_RerunFailsAgain_ResurfacesDismissedRow pins this by
// advancing FinishTime on the same run id and asserting the row comes back.
//
// The stamp is clamped against now the same way every other source's stamp
// is: a build agent's clock can be skewed, and Reconcile stores whatever
// this returns as LastActivity unconditionally on its "newer" branch.
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
