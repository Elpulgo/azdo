package azdevops

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// mentionCandidateQueryTop bounds each project's stage-1 WIQL query
// independently of mentionCandidateLimit below, matching the $top convention
// every other WIQL caller in this package uses (see
// Client.ListRecentlyMentionedWorkItems).
const mentionCandidateQueryTop = 50

// mentionCandidateLimit bounds how many candidate work items stage 2 will
// fetch comments for, merged across every project. Stage 2 costs one HTTP
// call per candidate — that per-mention comment fetch is the entire reason
// this source has two stages (a WIQL-only design has no per-mention
// timestamp) — so an org where the user is heavily @mentioned could
// otherwise fan out unboundedly. The spec's own "Unknowns" section records
// this constant as an explicit guess pending real-world measurement against
// a busy org; it exists to cap cost, not because 50 is a meaningful number.
// When candidates are truncated to this limit, it is logged (see
// SourceMentioned) rather than silently dropped.
const mentionCandidateLimit = 50

// SourceMentioned implements the "@mentions in work-item discussions"
// notification source (task 5 of the phase-2 notifications spec) →
// provider.NotificationReasonMentioned, per decision 3's two-stage design:
//
//  1. Stage 1 narrows to candidate work item ids via one WIQL query per
//     project (Client.ListRecentlyMentionedWorkItems, the @RecentMentions
//     macro). This is allowed to over-match.
//  2. Stage 2 fetches comments for each candidate (bounded by
//     mentionCandidateLimit) and keeps only work items where some comment
//     carries a mentions[].targetId equal to GetCurrentUserID(). The row's
//     activity stamp is the newest *matching* comment's createdDate, not the
//     work item's ChangedDate — an unrelated edit after the mention must not
//     advance the stamp, which is the entire reason stage 2 exists (a
//     WIQL-only design has no way to tell the two apart).
//
// The row's Read/Done fields are left at their zero value, matching
// SourceReviewRequested: this function only computes the freshly-queried
// subject, the shape Reconcile expects as input. Folding local triage state
// in is composition-layer work (task 8), not this source's job.
//
// now is threaded through to the stamp clamp so a stamp can never land in
// the future, mirroring prActivityStamp; it is never read from time.Now()
// internally.
//
// Partial failure degrades rather than emptying the feed, matching task 4:
// rows successfully confirmed are returned alongside a *PartialError
// describing what failed. Failed/Total here count every unit of work this
// two-stage source attempts — each project's stage-1 query plus each
// candidate's stage-2 comments fetch — not backends, since a single-backend
// source has no backend fraction to report. A project or candidate that
// fails is simply missing from the result; it is not retried within this
// call. Only when every stage-1 project fails does this function return
// (nil, err) with no *PartialError, since stage 2 has nothing to run against
// in that case.
func SourceMentioned(mc *MultiClient, now time.Time) ([]provider.Notification, error) {
	if mc == nil {
		return nil, fmt.Errorf("no client configured")
	}

	userID, err := resolveMentionUserID(mc)
	if err != nil {
		return nil, err
	}

	candidates, stage1Errs := queryMentionCandidates(mc)

	projectCount := len(mc.Projects())
	if projectCount > 0 && len(stage1Errs) == projectCount {
		return nil, fmt.Errorf("all projects failed to query mention candidates: %v", stage1Errs)
	}

	candidates, truncated, beforeTruncation := boundMentionCandidates(candidates)
	if truncated {
		slog.Warn("azdevops: mention source candidate fan-out truncated",
			"limit", mentionCandidateLimit,
			"candidates_before_truncation", beforeTruncation,
			"candidates_processed", len(candidates))
	}

	rows, stage2Errs := confirmAndMapMentions(mc, candidates, userID, now)

	failed := len(stage1Errs) + len(stage2Errs)
	total := projectCount + len(candidates)
	if failed == 0 {
		return rows, nil
	}

	errs := make([]error, 0, failed)
	errs = append(errs, stage1Errs...)
	errs = append(errs, stage2Errs...)
	return rows, &PartialError{Failed: failed, Total: total, Errors: errs}
}

// resolveMentionUserID fetches the authenticated user's id from any one
// project client (all share the same PAT/org), matching the pattern
// MultiClient.ListPullRequestsAsReviewer already uses. Stage 2 cannot
// confirm a single mention without it, so a failure here aborts before
// stage 1 does any work.
func resolveMentionUserID(mc *MultiClient) (string, error) {
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

// queryMentionCandidates fans stage 1 out to every project concurrently,
// tagging each surviving candidate with ProjectName/ProjectDisplayName the
// same way MultiClient's own fan-out methods do, and returns every
// individual project error rather than collapsing them — SourceMentioned
// counts these toward its combined PartialError.
func queryMentionCandidates(mc *MultiClient) ([]WorkItem, []error) {
	type result struct {
		project string
		items   []WorkItem
		err     error
	}

	projects := mc.Projects()
	var wg sync.WaitGroup
	ch := make(chan result, len(projects))

	for _, project := range projects {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			c := mc.ClientFor(p)
			if c == nil {
				ch <- result{project: p, err: fmt.Errorf("no client for project %q", p)}
				return
			}
			items, err := c.ListRecentlyMentionedWorkItems(mentionCandidateQueryTop)
			ch <- result{project: p, items: items, err: err}
		}(project)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []WorkItem
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		for i := range r.items {
			r.items[i].ProjectName = r.project
			r.items[i].ProjectDisplayName = mc.DisplayNameFor(r.project)
		}
		all = append(all, r.items...)
	}

	return all, errs
}

// boundMentionCandidates sorts candidates by id (so the cap is deterministic
// regardless of the goroutine-fan-out and map-iteration order that produced
// the input) and truncates to mentionCandidateLimit. Returns the bounded
// slice, whether truncation occurred, and the pre-truncation count for the
// caller's log line.
func boundMentionCandidates(candidates []WorkItem) ([]WorkItem, bool, int) {
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	total := len(candidates)
	if total <= mentionCandidateLimit {
		return candidates, false, total
	}
	return candidates[:mentionCandidateLimit], true, total
}

// mentionConfirmResult is one candidate's stage-2 outcome.
type mentionConfirmResult struct {
	item    WorkItem
	stamp   time.Time
	matched bool
	err     error
}

// confirmAndMapMentions runs stage 2: one comments fetch per candidate,
// concurrently, keeping only work items with at least one comment whose
// mentions[].targetId equals userID, and mapping survivors to
// provider.Notification rows stamped with the newest matching comment's
// createdDate.
func confirmAndMapMentions(mc *MultiClient, candidates []WorkItem, userID string, now time.Time) ([]provider.Notification, []error) {
	var wg sync.WaitGroup
	ch := make(chan mentionConfirmResult, len(candidates))

	for _, wi := range candidates {
		wg.Add(1)
		go func(item WorkItem) {
			defer wg.Done()
			c := mc.ClientFor(item.ProjectName)
			if c == nil {
				ch <- mentionConfirmResult{item: item, err: fmt.Errorf("no client for project %q", item.ProjectName)}
				return
			}
			comments, err := c.GetWorkItemComments(item.ID)
			if err != nil {
				ch <- mentionConfirmResult{item: item, err: err}
				return
			}
			stamp, matched := newestMatchingCommentStamp(comments, userID)
			ch <- mentionConfirmResult{item: item, stamp: stamp, matched: matched}
		}(wi)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var rows []provider.Notification
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		if !r.matched {
			continue
		}
		rows = append(rows, mapMentioned(mc, r.item, r.stamp, now))
	}

	return rows, errs
}

// newestMatchingCommentStamp scans every comment for a mention whose
// targetId exact-matches userID (probe result (c): plain ==, no
// normalising) and returns the newest createdDate among the matches. It does
// not rely on the comments API's order=desc response ordering — every
// comment is inspected and the maximum is tracked explicitly — so a
// candidate is never mis-stamped if that ordering assumption ever stops
// holding. found is false when no comment mentions userID at all, which
// happens whenever stage 1 over-matched (decision 3 expects this).
func newestMatchingCommentStamp(comments []WorkItemComment, userID string) (stamp time.Time, found bool) {
	for _, comment := range comments {
		for _, m := range comment.Mentions {
			if m.TargetID == userID {
				if !found || comment.CreatedDate.After(stamp) {
					stamp = comment.CreatedDate
				}
				found = true
				break
			}
		}
	}
	return stamp, found
}

// mapMentioned maps a confirmed-mentioned WorkItem (already tagged with
// ProjectName/ProjectDisplayName by queryMentionCandidates) to a
// provider.Notification for the mentioned reason. stamp is the newest
// matching comment's createdDate (see newestMatchingCommentStamp) — never
// WorkItem.Fields.ChangedDate, which would resurrect the row on any
// unrelated edit and defeat the entire point of stage 2.
//
// now clamps a future stamp the same way prActivityStamp does: a comment's
// createdDate is server-set, but a build-agent or client clock skew is not
// impossible, and Reconcile stores whatever this returns as LastActivity
// unconditionally on its "newer" branch.
func mapMentioned(mc *MultiClient, wi WorkItem, stamp time.Time, now time.Time) provider.Notification {
	scope := wi.ProjectName
	scopeDisplay := wi.ProjectDisplayName
	if scopeDisplay == "" {
		scopeDisplay = scope
	}

	if stamp.After(now) {
		stamp = now
	}

	return provider.Notification{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           NotifKey("mention", "wi", wi.ID),
		},
		Title:     wi.Fields.Title,
		Reason:    provider.NotificationReasonMentioned,
		UpdatedAt: stamp,
		WebURL:    mentionedWebURL(mc, wi),
	}
}

// mentionedWebURL resolves a mentioned work item's browser URL, following
// the same degradation ladder as reviewRequestedWebURL (see that function's
// doc comment for the full rationale):
//
//   - No client for the work item's scope (an unconfigured or unresolvable
//     project): "" — there is nothing to build even a project page from.
//   - WorkItem.ID <= 0 (convention 11: guard positive identifiers with
//     <= 0, not == 0) — the deep link's trailing id segment would not
//     validate: fall back to the project page.
//   - Otherwise, the same "https://dev.azure.com/{org}/{project}/_workitems/
//     edit/{id}" deep link Adapter.WorkItemURL already builds for the work
//     item pane's `o` (open in browser) action.
func mentionedWebURL(mc *MultiClient, wi WorkItem) string {
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
