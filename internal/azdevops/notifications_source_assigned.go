package azdevops

import (
	"errors"
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// assignedQueryTop bounds each project's WIQL query, matching the $top
// convention every other WIQL caller in this package uses (see
// mentionCandidateQueryTop, ListMyWorkItems, ListRecentlyMentionedWorkItems).
const assignedQueryTop = 50

// SourceAssigned implements the "recently assigned work items" notification
// source (task 6 of the phase-2 notifications spec) →
// provider.NotificationReasonAssigned, per decision 6: a bounded lookback
// window (`[System.AssignedTo] = @Me AND [System.ChangedDate] >=
// @Today-lookbackDays`, see Client.ListRecentlyAssignedWorkItems), no
// snapshot and no delta state. The window is the only state this source
// relies on: the same poll run twice against unchanged data returns
// identical rows, and it is Reconcile (notifications_reconcile.go) — not
// this source — that decides read/unread from there. That idempotence is
// what replaces the snapshot decision 6 rejected, and it is also what keeps
// a fresh install with an empty local state file bounded to at most the
// window's worth of items rather than flooding the feed with every work
// item ever assigned.
//
// lookbackDays is N — task 11 wires it to config's
// notifications.azure.lookback_days; this function takes it as a plain
// parameter and adds no config plumbing of its own.
//
// The row's Read/Done fields are left at their zero value here, matching
// SourceReviewRequested and SourceMentioned: this function only computes the
// freshly-queried subject, the shape Reconcile expects as input.
//
// now is threaded through to assignedActivityStamp so a stamp can never land
// in the future; it is never read from time.Now() internally, matching every
// other source in this package.
//
// A partial multi-project failure (MultiClient.ListRecentlyAssignedWorkItems
// returning rows alongside a *PartialError) does not discard the rows it did
// get, matching SourceReviewRequested's and SourceMentioned's
// degrade-not-blank contract: the rows from surviving projects are mapped
// and returned alongside the error.
func SourceAssigned(mc *MultiClient, lookbackDays int, now time.Time) ([]provider.Notification, error) {
	if mc == nil {
		return nil, fmt.Errorf("no client configured")
	}

	items, err := mc.ListRecentlyAssignedWorkItems(lookbackDays, assignedQueryTop)
	if err != nil {
		var partialErr *PartialError
		if !errors.As(err, &partialErr) {
			return nil, err
		}
		// Partial failure: still map and return the rows the surviving
		// projects gave us, alongside the error.
		rows := make([]provider.Notification, 0, len(items))
		for _, wi := range items {
			rows = append(rows, mapAssigned(mc, wi, now))
		}
		return rows, err
	}

	rows := make([]provider.Notification, 0, len(items))
	for _, wi := range items {
		rows = append(rows, mapAssigned(mc, wi, now))
	}
	return rows, nil
}

// mapAssigned maps a single wire WorkItem (already tagged with
// ProjectName/ProjectDisplayName by MultiClient's fan-out) to a
// provider.Notification for the assigned reason.
func mapAssigned(mc *MultiClient, wi WorkItem, now time.Time) provider.Notification {
	scope := wi.ProjectName
	scopeDisplay := wi.ProjectDisplayName
	if scopeDisplay == "" {
		scopeDisplay = scope
	}

	return provider.Notification{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           NotifKey("assigned", "wi", wi.ID),
		},
		Title:     wi.Fields.Title,
		Reason:    provider.NotificationReasonAssigned,
		UpdatedAt: assignedActivityStamp(wi, now),
		WebURL:    assignedWebURL(mc, wi),
	}
}

// assignedActivityStamp is the row's Notification.UpdatedAt: the work
// item's ChangedDate, the same timestamp
// Client.ListRecentlyAssignedWorkItems' WIQL query filters and orders by.
// Unlike mentionActivityStamp, which treats ChangedDate as the *wrong*
// stamp — reachable only as a fallback, because "the item changed" is not
// "you were mentioned" — this source makes ChangedDate the primary stamp
// (CreatedDate remains a fallback for the zero case, below): for "recently
// assigned to me", any edit to the item genuinely is
// new activity worth surfacing, so there is no stage-2 confirmation step to
// prefer over it. This is a deliberate choice, not an oversight: Azure
// exposes no assignment-change timestamp on its own (no "assigned on" field
// on WorkItemFields) short of calling the per-item `/updates` endpoint once
// per candidate, which this source does not do.
//
// The stamp is clamped against now the same way prActivityStamp and
// mentionActivityStamp clamp theirs — a client or server clock skew is not
// something this function can rule out, and Reconcile stores whatever this
// returns as LastActivity unconditionally on its "newer" branch. CreatedDate
// is the fallback should ChangedDate ever arrive zero (malformed data), so
// Reconcile receives a usable stamp instead of "no activity information" —
// mirroring prActivityStamp's CreationDate fallback and
// mentionActivityStamp's ChangedDate fallback.
func assignedActivityStamp(wi WorkItem, now time.Time) time.Time {
	stamp := wi.Fields.ChangedDate
	if stamp.IsZero() {
		stamp = wi.Fields.CreatedDate
	}
	if stamp.After(now) {
		return now
	}
	return stamp
}

// assignedWebURL resolves a recently-assigned work item's browser URL,
// following the same degradation ladder as mentionedWebURL and
// reviewRequestedWebURL (see either's doc comment for the full rationale):
//
//   - No client for the work item's scope (an unconfigured or unresolvable
//     project): "" — there is nothing to build even a project page from.
//   - WorkItem.ID <= 0 (convention 11: guard positive identifiers with
//     <= 0, not == 0) — the deep link's trailing id segment would not
//     validate: fall back to the project page.
//   - Otherwise, the same "https://dev.azure.com/{org}/{project}/_workitems/
//     edit/{id}" deep link Adapter.WorkItemURL already builds for the work
//     item pane's `o` (open in browser) action.
func assignedWebURL(mc *MultiClient, wi WorkItem) string {
	if mc == nil {
		return ""
	}
	c := mc.ClientFor(wi.ProjectName)
	if c == nil {
		return ""
	}

	projectPage := fmt.Sprintf("https://dev.azure.com/%s/%s", c.GetOrg(), c.GetProject())
	if wi.ID <= 0 {
		return projectPage
	}

	return fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d",
		c.GetOrg(), c.GetProject(), wi.ID)
}
