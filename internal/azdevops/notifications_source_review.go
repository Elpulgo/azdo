package azdevops

import (
	"errors"
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// SourceReviewRequested implements the "PRs awaiting my review" notification
// source (task 4 of the phase-2 notifications spec) → provider.
// NotificationReasonReviewRequested. It reuses MultiClient.
// ListPullRequestsAsReviewer verbatim — no new client method — and maps every
// returned PR to a provider.Notification.
//
// top bounds ListPullRequestsAsReviewer's own per-project page size, the same
// contract every other MultiClient caller uses.
//
// The row's Read/Done fields are left at their zero value here: this function
// only computes the freshly-queried subject, the same shape Reconcile (see
// notifications_reconcile.go) expects as input. Folding local triage state in
// is composition-layer work (task 8), not this source's job.
//
// now is threaded through to prActivityStamp so a stamp can never land in the
// future (see prActivityStamp's doc comment) while staying testable — it is
// never read from time.Now() internally, matching Reconcile's own style.
//
// A partial multi-project failure (MultiClient.ListPullRequestsAsReviewer
// returning rows alongside a *PartialError) must not discard the rows it did
// get: the rest of the codebase honours that rows-plus-PartialError contract
// explicitly (internal/ui/pullrequests/list.go:154 and :193), and dropping
// the rows here would also starve Reconcile of LastSeen updates for every
// subject a healthy project still returned, TTL-pruning and resurrecting
// dismissed rows on sustained partial failure.
func SourceReviewRequested(mc *MultiClient, top int, now time.Time) ([]provider.Notification, error) {
	if mc == nil {
		return nil, fmt.Errorf("no client configured")
	}

	prs, err := mc.ListPullRequestsAsReviewer(top)
	if err != nil {
		var partialErr *PartialError
		if !errors.As(err, &partialErr) {
			return nil, err
		}
		// Partial failure: still map and return the rows the surviving
		// projects gave us, alongside the error.
		rows := make([]provider.Notification, 0, len(prs))
		for _, pr := range prs {
			rows = append(rows, mapReviewRequested(mc, pr, now))
		}
		return rows, err
	}

	rows := make([]provider.Notification, 0, len(prs))
	for _, pr := range prs {
		rows = append(rows, mapReviewRequested(mc, pr, now))
	}
	return rows, nil
}

// mapReviewRequested maps a single wire PullRequest (already tagged with
// ProjectName/ProjectDisplayName by MultiClient's fan-out) to a
// provider.Notification for the review_requested reason.
func mapReviewRequested(mc *MultiClient, pr PullRequest, now time.Time) provider.Notification {
	scope := pr.ProjectName
	scopeDisplay := pr.ProjectDisplayName
	if scopeDisplay == "" {
		scopeDisplay = scope
	}

	return provider.Notification{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           NotifKey("review", "pr", pr.ID),
		},
		Title:     pr.Title,
		Reason:    provider.NotificationReasonReviewRequested,
		UpdatedAt: prActivityStamp(pr, now),
		WebURL:    reviewRequestedWebURL(mc, pr),
	}
}

// prActivityStamp is the PR's last-update timestamp, used as the row's
// Notification.UpdatedAt so a new push resurrects a dismissed row (decision 2
// of the phase-2 notifications spec — Reconcile treats a newer UpdatedAt than
// the stored LastActivity as new activity and clears Read/Done).
//
// Azure's GetPullRequests payload carries no top-level "last updated" field.
// LastMergeSourceCommit is the closest available proxy: Azure DevOps reruns
// its mergeability check — and therefore refreshes this field — on every push
// to the source branch, not only at completion (see PullRequest.
// LastMergeSourceCommit's doc comment). When it is absent (nil, or a zero
// Committer.Date — e.g. a brand-new PR before the first check has run),
// CreationDate is the fallback so the row still gets a usable stamp instead of
// the zero time, which Reconcile treats as "no activity information" rather
// than as the epoch.
//
// now is clamped against: a committer date is user-settable
// (GIT_COMMITTER_DATE, rebase --committer-date-is-author-date) or wrong under
// build-agent clock skew, and Reconcile stores whatever this returns as
// LastActivity unconditionally on its "newer" branch. An unclamped future
// stamp would raise the bar past anything a real push could ever clear,
// permanently freezing the row (see this task's review feedback and the
// spec's "Unknowns" entry on resurrection-by-token, which covers the
// regression variant this clamp does not address). now is a parameter, never
// time.Now() read internally, so this stays as testable as Reconcile itself.
func prActivityStamp(pr PullRequest, now time.Time) time.Time {
	stamp := pr.CreationDate
	if pr.LastMergeSourceCommit != nil && !pr.LastMergeSourceCommit.Committer.Date.IsZero() {
		stamp = pr.LastMergeSourceCommit.Committer.Date
	}
	if stamp.After(now) {
		return now
	}
	return stamp
}

// reviewRequestedWebURL resolves a review-requested PR's browser URL,
// following phase 1's degradation ladder (see github.NotificationWebURL):
// unresolvable inputs fall back to the project's landing page rather than
// guessing a deep link, and "" is a legal result when even the project page
// cannot be built.
//
//   - No client for the PR's scope (an unconfigured or unresolvable project):
//     "" — there is nothing to build even a project page from.
//   - Repository.ID empty, or PR.ID <= 0 (convention 11: guard positive
//     identifiers with <= 0, not == 0) — the deep link's trailing id segment
//     would not validate: fall back to the project page.
//   - Otherwise, the same "https://dev.azure.com/{org}/{project}/_git/{repo}/
//     pullrequest/{id}" deep link Adapter.PRURL already builds for the PR
//     pane's `o` (open in browser) action.
func reviewRequestedWebURL(mc *MultiClient, pr PullRequest) string {
	if mc == nil {
		return ""
	}
	c := mc.ClientFor(pr.ProjectName)
	if c == nil {
		return ""
	}

	projectPage := fmt.Sprintf("https://dev.azure.com/%s/%s", c.GetOrg(), c.GetProject())
	if pr.Repository.ID == "" || pr.ID <= 0 {
		return projectPage
	}

	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d",
		c.GetOrg(), c.GetProject(), pr.Repository.ID, pr.ID)
}
