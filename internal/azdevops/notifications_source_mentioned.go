package azdevops

import (
	"fmt"
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
// When candidates are truncated to this limit, the drop count is surfaced to
// the caller as structured data on SourceMentionedResult rather than logged
// — this repo has no logging facility, and a call that writes to stderr
// mid-poll would land on top of Bubble Tea's alt-screen render.
const mentionCandidateLimit = 50

// mentionStage2Concurrency bounds how many stage-2 comment fetches run
// concurrently. Without a bound, confirmAndMapMentions launches one
// goroutine per candidate — up to mentionCandidateLimit (50) simultaneous
// requests — against dev.azure.com on a client with a 30s timeout and no
// retry/backoff (client.go:65). Every other fan-out in this package is
// naturally bounded by project count (1-5); stage 2 fans out over candidate
// count instead, which needs its own explicit pool so a heavily-mentioned
// user does not get rate-limited into stage2Errs.
const mentionStage2Concurrency = 6

// SourceMentionedResult is SourceMentioned's return value: the confirmed
// mention rows plus how much stage 1's candidate fan-out was truncated, if
// at all. Truncation is never silently swallowed and never written to
// stderr/stdout — it is data the caller (task 8's composition layer) decides
// how to surface.
type SourceMentionedResult struct {
	// Rows is the set of confirmed-mentioned notifications, shaped for
	// Reconcile the same way every other source's rows are.
	Rows []provider.Notification
	// CandidatesDropped is how many stage-1 candidates were discarded by
	// the mentionCandidateLimit cap. Zero means no truncation occurred.
	CandidatesDropped int
	// CandidateLimit is the cap that applied (mentionCandidateLimit),
	// always populated regardless of whether truncation happened, so a
	// caller can render "N of Limit" without importing the constant.
	CandidateLimit int
	// CommentFetchFailures is how many stage-2 comments fetches failed.
	// Kept separate from the returned *PartialError's Failed/Total, which
	// stay project-scoped (see SourceMentioned's doc comment) so the
	// rendered "%d of %d projects failed to load" message
	// (polling/errorhandler.go) stays truthful — folding candidate-level
	// failures into that count previously produced messages like "3 of 51
	// projects failed to load" for a single-project config.
	CommentFetchFailures int
}

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
// describing what failed. The returned *PartialError's Failed/Total stay
// project-scoped — Failed is the number of projects whose stage-1 query
// failed, Total is projectCount — matching every other PartialError in this
// package and the "%d of %d projects failed to load" message it renders
// into (errors.go, polling/errorhandler.go). Stage-2 comment-fetch failures
// are real failures too, so they still land in Errors and still make this
// function return a non-nil *PartialError, but their count is carried
// separately on SourceMentionedResult.CommentFetchFailures rather than
// folded into Failed/Total: a single-project config with 50 candidates and
// 3 failed comment fetches must not render as "3 of 51 projects failed to
// load". A project or candidate that fails is simply missing from the
// result; it is not retried within this call. Only when every stage-1
// project fails does this function return (SourceMentionedResult{}, err)
// with no *PartialError, since stage 2 has nothing to run against in that
// case.
//
// Candidate fan-out across projects is bounded by mentionCandidateLimit
// (see boundMentionCandidates). Truncation, when it happens, is reported on
// the returned SourceMentionedResult rather than logged — this repo has no
// logging facility.
func SourceMentioned(mc *MultiClient, now time.Time) (SourceMentionedResult, error) {
	if mc == nil {
		return SourceMentionedResult{}, fmt.Errorf("no client configured")
	}

	userID, err := resolveMentionUserID(mc)
	if err != nil {
		return SourceMentionedResult{}, err
	}

	byProject, stage1Errs := queryMentionCandidates(mc)

	projectCount := len(mc.Projects())
	if projectCount > 0 && len(stage1Errs) == projectCount {
		return SourceMentionedResult{}, fmt.Errorf("all projects failed to query mention candidates: %v", stage1Errs)
	}

	merged := interleaveMentionCandidates(byProject)
	candidates, truncated, beforeTruncation := boundMentionCandidates(merged)

	dropped := 0
	if truncated {
		dropped = beforeTruncation - len(candidates)
	}

	rows, stage2Errs := confirmAndMapMentions(mc, candidates, userID, now)

	result := SourceMentionedResult{
		Rows:                 rows,
		CandidatesDropped:    dropped,
		CandidateLimit:       mentionCandidateLimit,
		CommentFetchFailures: len(stage2Errs),
	}

	if len(stage1Errs) == 0 && len(stage2Errs) == 0 {
		return result, nil
	}

	errs := make([]error, 0, len(stage1Errs)+len(stage2Errs))
	errs = append(errs, stage1Errs...)
	errs = append(errs, stage2Errs...)
	return result, &PartialError{Failed: len(stage1Errs), Total: projectCount, Errors: errs}
}

// resolveMentionUserID fetches the authenticated user's id from any one
// project client (all share the same PAT/org), matching the pattern
// MultiClient.ListPullRequestsAsReviewer already uses. Stage 2 cannot
// confirm a single mention without it, so a failure here aborts before
// stage 1 does any work.
//
// An empty id must never reach stage 2's targetId comparison: "" == "" would
// match a mention payload carrying no resolved target the same way it would
// match the caller. Client.GetCurrentUserID() already rejects an empty
// AuthenticatedUser.ID (client.go:234), so this path is unreachable today,
// but the guard stays here anyway because SetUserID (client.go:40) writes
// the cache unchecked and this is the identity-comparison call site that
// matters if that ever changes (matching resolveCIFailedUserID's own guard
// in notifications_source_cifailed.go).
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
		if id == "" {
			return "", fmt.Errorf("resolved an empty user ID")
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
//
// Results are kept keyed by project rather than flattened into one slice:
// each project's list preserves ListRecentlyMentionedWorkItems' own
// ChangedDate-DESC order, but WIQL returns only ids, so there is no
// cross-project timestamp to sort a flattened list by. interleaveMentionCandidates
// merges these per-project lists round-robin, which preserves each project's
// recency order without ever comparing timestamps (or ids) across projects.
func queryMentionCandidates(mc *MultiClient) (map[string][]WorkItem, []error) {
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

	byProject := make(map[string][]WorkItem, len(projects))
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
		byProject[r.project] = r.items
	}

	return byProject, errs
}

// interleaveMentionCandidates merges every project's stage-1 candidates into
// one slice by round-robin, preserving each project's own ChangedDate-DESC
// order (see queryMentionCandidates). Round-robin — rather than a single
// cross-project sort — is what keeps every project's most-recently-changed
// candidates represented once boundMentionCandidates truncates, instead of
// letting one project with many candidates crowd out another's newest items.
// It also avoids the trap a bare id-descending sort falls into: work-item id
// order is *creation* order, not activity order, and an old (low-id) work
// item can receive a brand-new mention — exactly the case this source exists
// to catch.
//
// Projects are visited in a fixed alphabetical order, so the merge is
// deterministic regardless of the goroutine fan-out and map iteration order
// that produced byProject.
func interleaveMentionCandidates(byProject map[string][]WorkItem) []WorkItem {
	projects := make([]string, 0, len(byProject))
	total := 0
	for p, items := range byProject {
		projects = append(projects, p)
		total += len(items)
	}
	sort.Strings(projects)

	merged := make([]WorkItem, 0, total)
	for i := 0; ; i++ {
		added := false
		for _, p := range projects {
			items := byProject[p]
			if i < len(items) {
				merged = append(merged, items[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	return merged
}

// boundMentionCandidates truncates an already round-robin-interleaved
// candidate slice (see interleaveMentionCandidates) to mentionCandidateLimit.
// It does not sort: the input's order already carries the meaning that
// matters — each project's most-recently-changed candidates first — so
// truncating from the tail drops the least-recently-changed candidates
// spread fairly across projects, rather than collapsing to whichever id
// happens to be numerically highest or lowest. Returns the bounded slice,
// whether truncation occurred, and the pre-truncation count for the caller.
func boundMentionCandidates(candidates []WorkItem) ([]WorkItem, bool, int) {
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
// concurrently but bounded to mentionStage2Concurrency in-flight requests at
// a time via the sem semaphore, keeping only work items with at least one
// comment whose mentions[].targetId equals userID, and mapping survivors to
// provider.Notification rows stamped with the newest matching comment's
// createdDate.
//
// The returned rows are sorted by UpdatedAt descending before returning —
// they otherwise arrive in goroutine-completion order, which is
// nondeterministic — matching SourceReviewRequested's rows, which come back
// already sorted by MultiClient.ListPullRequestsAsReviewer (CreationDate
// descending). Reconcile itself is order-independent and the composite
// re-sorts the merged feed, so this is not a correctness fix, only
// consistency with task 4 and determinism for any future order-sensitive
// test.
func confirmAndMapMentions(mc *MultiClient, candidates []WorkItem, userID string, now time.Time) ([]provider.Notification, []error) {
	var wg sync.WaitGroup
	ch := make(chan mentionConfirmResult, len(candidates))
	sem := make(chan struct{}, mentionStage2Concurrency)

	for _, wi := range candidates {
		wg.Add(1)
		go func(item WorkItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

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

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})

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
//
// comments is capped at commentsTopLimit (200, see GetWorkItemComments) with
// no pagination. order=desc normally keeps the newest matching comment
// inside that window, but a work item whose only mention sits behind 200
// newer comments falls outside it: this function then finds no match, the
// row silently stops appearing in the feed, and its local triage entry
// TTL-prunes as if the mention had never existed.
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
// matching comment's createdDate (see newestMatchingCommentStamp and
// mentionActivityStamp's fallback) — never WorkItem.Fields.ChangedDate
// directly, which would resurrect the row on any unrelated edit and defeat
// the entire point of stage 2.
func mapMentioned(mc *MultiClient, wi WorkItem, stamp time.Time, now time.Time) provider.Notification {
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
			ID:           NotifKey("mention", "wi", wi.ID),
		},
		Title:     wi.Fields.Title,
		Reason:    provider.NotificationReasonMentioned,
		UpdatedAt: mentionActivityStamp(stamp, wi, now),
		WebURL:    mentionedWebURL(mc, wi),
	}
}

// mentionActivityStamp is the row's Notification.UpdatedAt. It is normally
// just stamp — the newest matching comment's createdDate, clamped against
// now the same way prActivityStamp clamps a PR's committer date, since a
// comment's createdDate is server-set but a build-agent or client clock skew
// is not impossible, and Reconcile stores whatever this returns as
// LastActivity unconditionally on its "newer" branch.
//
// The one exception is a zero stamp: newestMatchingCommentStamp can return
// found=true with a zero createdDate if the matching comment's own
// createdDate field is itself the zero value (malformed data), and passing
// that through would give Reconcile "no activity information" rather than a
// usable stamp. wi.Fields.ChangedDate is the fallback in that case,
// mirroring prActivityStamp's fallback to PullRequest.CreationDate in
// notifications_source_review.go — it is the wrong stamp in the normal case
// (see mapMentioned's doc comment) but the right one when the correct stamp
// is missing.
func mentionActivityStamp(stamp time.Time, wi WorkItem, now time.Time) time.Time {
	if stamp.IsZero() {
		stamp = wi.Fields.ChangedDate
	}
	if stamp.After(now) {
		return now
	}
	return stamp
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
