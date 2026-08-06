package azdevops

import (
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
func SourceReviewRequested(mc *MultiClient, top int) ([]provider.Notification, error) {
	if mc == nil {
		return nil, fmt.Errorf("no client configured")
	}

	prs, err := mc.ListPullRequestsAsReviewer(top)
	if err != nil {
		return nil, err
	}

	rows := make([]provider.Notification, 0, len(prs))
	for _, pr := range prs {
		rows = append(rows, mapReviewRequested(mc, pr))
	}
	return rows, nil
}

// mapReviewRequested maps a single wire PullRequest (already tagged with
// ProjectName/ProjectDisplayName by MultiClient's fan-out) to a
// provider.Notification for the review_requested reason.
func mapReviewRequested(mc *MultiClient, pr PullRequest) provider.Notification {
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
		UpdatedAt: prActivityStamp(pr),
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
func prActivityStamp(pr PullRequest) time.Time {
	if pr.LastMergeSourceCommit != nil && !pr.LastMergeSourceCommit.Committer.Date.IsZero() {
		return pr.LastMergeSourceCommit.Committer.Date
	}
	return pr.CreationDate
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
