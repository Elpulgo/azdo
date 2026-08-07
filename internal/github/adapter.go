package github

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Adapter wraps a MultiClient and satisfies provider.Provider.
//
// List methods delegate to the MultiClient (which maps wire→neutral inside
// each fan-out goroutine). Detail, mutation, and URL methods route to the
// per-repo Client via ClientFor(scope) and map the single result before
// returning.
//
// The repositoryID parameter accepted by several interface methods is REDUNDANT
// for GitHub: the scope ("owner/repo") already fully identifies the repository.
// It is accepted for interface compliance and ignored internally. This is
// documented on each affected method.
//
// Adapter also satisfies provider.NotificationSource via nc, the user-scoped
// NotificationsClient. Unlike mc, nc is not part of the per-repo fan-out map:
// GET /notifications is user-level, so it is a separate field set once at
// construction — see NewAdapterWithNotifications.
type Adapter struct {
	mc *MultiClient

	// nc is the user-scoped notifications client. It is set once at
	// construction by NewAdapterWithNotifications and never mutated
	// afterwards: Adapter is read concurrently by Bubble Tea's
	// goroutine-per-tea.Cmd model, and a setter would make this field a data
	// race with no lock available to guard it — locks are kept off the
	// mark-read/mark-done path deliberately, so introducing one here (even
	// just to guard a field write) would undermine that. NewAdapter leaves
	// this nil; a nil nc is a reachable, supported state (see
	// NewAdapterWithNotifications's doc comment), not defensive padding.
	nc *NotificationsClient
}

// NewAdapter creates an Adapter wrapping the given MultiClient.
// A nil MultiClient is allowed (the Adapter still satisfies the interface; all
// methods that require a live client return a descriptive error).
//
// nc (the notifications client) is left nil. Use NewAdapterWithNotifications
// to construct an Adapter that also supports provider.NotificationSource.
func NewAdapter(mc *MultiClient) *Adapter {
	return &Adapter{mc: mc}
}

// NewAdapterWithNotifications creates an Adapter wrapping both the given
// MultiClient and the given user-scoped NotificationsClient.
//
// mc may be nil (see NewAdapter). nc may also be nil: this is a reachable
// state, not defensive padding — Adapter satisfies provider.NotificationSource
// at compile time whether or not a notifications client was supplied, so a
// GitHub config that never built one (e.g. a token missing the notifications
// scope, discovered lazily) still shows the notifications tab and still calls
// List. List/MarkRead/MarkDone all return a descriptive error rather than
// panicking when nc is nil, matching NewAdapter(nil)'s documented contract for
// the rest of the interface.
func NewAdapterWithNotifications(mc *MultiClient, nc *NotificationsClient) *Adapter {
	return &Adapter{mc: mc, nc: nc}
}

// Kind returns provider.KindGitHub to identify the GitHub backend.
func (a *Adapter) Kind() provider.Kind {
	return provider.KindGitHub
}

// IsMultiProject returns true when more than one repo is configured.
func (a *Adapter) IsMultiProject() bool {
	if a.mc == nil {
		return false
	}
	return a.mc.IsMultiProject()
}

// Scopes returns the GitHub "owner/repo" slugs this adapter spans, sorted.
// Returns nil when no client is configured.
func (a *Adapter) Scopes() []string {
	if a.mc == nil {
		return nil
	}
	return a.mc.Scopes()
}

// --------------------------------------------------------------------------
// Pull-request list surface — delegates to MultiClient (already neutral)
// --------------------------------------------------------------------------

// ListPullRequests returns up to top active pull requests across all repos,
// sorted by CreationDate descending. opts carries neutral filter intent.
func (a *Adapter) ListPullRequests(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	return a.mc.ListPullRequests(top, opts)
}

// ListMyPullRequests returns up to top pull requests authored by the
// authenticated user, sorted by CreationDate descending.
func (a *Adapter) ListMyPullRequests(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	return a.mc.ListMyPullRequests(top, opts)
}

// ListPullRequestsAsReviewer returns up to top pull requests where the
// authenticated user is a requested reviewer, sorted by CreationDate descending.
func (a *Adapter) ListPullRequestsAsReviewer(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	return a.mc.ListPullRequestsAsReviewer(top, opts)
}

// --------------------------------------------------------------------------
// Pull-request detail / mutation surface
// --------------------------------------------------------------------------

// GetPRThreads returns the comment threads for the given pull request.
// scope routes to the correct per-repo Client.
// repositoryID is redundant for GitHub (scope already identifies the repo)
// and is ignored.
func (a *Adapter) GetPRThreads(scope, repositoryID string, pullRequestID int) ([]provider.Thread, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	// GetPRThreads returns a flat []ReviewComment; MapReviewThreads groups them.
	wire, err := c.GetPRThreads(pullRequestID)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	return MapReviewThreads(wire, scope, scopeDisplay), nil
}

// GetPRIterations returns a single synthetic iteration representing the whole PR.
//
// GitHub has no per-push iteration concept. A single stable iteration with
// ID=1 is returned so that the diff/files view can call
// GetPRIterationChanges(iterationID=1) without special-casing the GitHub backend.
// No HTTP call is made.
//
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) GetPRIterations(scope, repositoryID string, pullRequestID int) ([]provider.Iteration, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	if a.mc.ClientFor(scope) == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	return []provider.Iteration{
		{
			ID:          1,
			Description: "Whole PR (synthetic — GitHub has no per-push iterations)",
		},
	}, nil
}

// GetPRIterationChanges returns the files changed in the pull request.
//
// iterationID is ignored: GitHub has only one synthetic iteration (ID=1) per PR,
// so the same file list is always returned regardless of iterationID.
// Files are fetched via GET /pulls/{prID}/files and mapped with MapPRFile.
//
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) GetPRIterationChanges(scope, repositoryID string, pullRequestID int, iterationID int) ([]provider.IterationChange, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	files, err := c.GetPRFiles(pullRequestID)
	if err != nil {
		return nil, err
	}
	result := make([]provider.IterationChange, len(files))
	for i, f := range files {
		result[i] = MapPRFile(f, i+1) // changeID is 1-based sequential index
	}
	return result, nil
}

// VotePullRequest submits a reviewer vote on the given pull request.
// scope routes to the correct per-repo Client.
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) VotePullRequest(scope, repositoryID string, pullRequestID int, vote int) error {
	if a.mc == nil {
		return fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return fmt.Errorf("no client for scope %q", scope)
	}
	return c.VotePullRequest(pullRequestID, vote)
}

// GetFileContent returns the raw decoded file content at the given branch ref.
// scope routes to the correct per-repo Client.
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) GetFileContent(scope, repositoryID string, filePath string, branchName string) (string, error) {
	if a.mc == nil {
		return "", fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return "", fmt.Errorf("no client for scope %q", scope)
	}
	return c.GetFileContent(filePath, branchName)
}

// AddPRCodeComment creates an inline code comment on the given file and line.
//
// The wire ReviewComment returned by the Client is passed to MapReviewThreads
// as a single-element slice (it is a root comment with InReplyToID==nil) to
// produce a single provider.Thread, which is returned. This reuses the same
// mapping logic as GetPRThreads.
//
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) AddPRCodeComment(scope, repositoryID string, pullRequestID int, filePath string, line int, content string) (*provider.Thread, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.AddPRCodeComment(pullRequestID, filePath, line, content)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	// AddPRCodeComment returns a root ReviewComment (InReplyToID==nil).
	// MapReviewThreads produces exactly one thread for a root-only input.
	threads := MapReviewThreads([]ReviewComment{wire}, scope, scopeDisplay)
	if len(threads) == 0 {
		return nil, fmt.Errorf("github: AddPRCodeComment: mapper produced no threads for created comment")
	}
	return &threads[0], nil
}

// AddPRComment creates a general (non-file) comment on the pull request.
//
// GitHub models general PR comments as issue comments (IssueComment wire type).
// The returned IssueComment is synthesized into a single-comment provider.Thread
// with FilePath="" and Line=0, making it indistinguishable from a general PR
// thread in the neutral model.
//
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) AddPRComment(scope, repositoryID string, pullRequestID int, content string) (*provider.Thread, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.AddPRComment(pullRequestID, content)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	// Synthesize a Thread wrapping a single general comment.
	// IssueComment has no FilePath/Line concept; both are zero.
	id := fmt.Sprintf("%d", wire.ID)
	comment := provider.Comment{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           id,
		},
		ParentCommentID: 0,
		Content:         wire.Body,
		PublishedDate:   wire.CreatedAt,
		LastUpdatedDate: wire.UpdatedAt,
		CommentType:     "text",
		AuthorName:      wire.User.Login,
		AuthorID:        fmt.Sprintf("%d", wire.User.ID),
	}
	thread := provider.Thread{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           id,
		},
		PublishedDate:   wire.CreatedAt,
		LastUpdatedDate: wire.UpdatedAt,
		Status:          "active",
		FilePath:        "",
		Line:            0,
		Comments:        []provider.Comment{comment},
		IsDeleted:       false,
	}
	return &thread, nil
}

// ReplyToThread posts a reply to an existing review thread.
//
// threadID is the root-comment database ID (stamped as thread Identity.ID by
// MapReviewThreads). It is passed as rootCommentID to the Client and as
// parentCommentID to mapReviewComment so the reply is correctly parented.
//
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) ReplyToThread(scope, repositoryID string, pullRequestID int, threadID int, content string) (*provider.Comment, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.ReplyToThread(pullRequestID, threadID, content)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	// threadID is the root comment's database ID = ParentCommentID for replies.
	comment := mapReviewComment(wire, threadID, scope, scopeDisplay)
	return &comment, nil
}

// UpdateThreadStatus resolves or unresolves a pull request review thread via
// the GitHub GraphQL API.
// scope routes to the correct per-repo Client.
// repositoryID is ignored (see Adapter doc).
func (a *Adapter) UpdateThreadStatus(scope, repositoryID string, pullRequestID int, threadID int, status string) error {
	if a.mc == nil {
		return fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return fmt.Errorf("no client for scope %q", scope)
	}
	return c.UpdateThreadStatus(pullRequestID, threadID, status)
}

// --------------------------------------------------------------------------
// Work-item list surface — delegates to MultiClient (already neutral)
// --------------------------------------------------------------------------

// ListWorkItems returns up to top work items across all repos, sorted by
// ChangedDate descending. opts carries neutral filter intent.
func (a *Adapter) ListWorkItems(top int, opts provider.ListOpts) ([]provider.WorkItem, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	return a.mc.ListWorkItems(top, opts)
}

// ListMyWorkItems returns up to top work items assigned to the authenticated
// user, sorted by ChangedDate descending.
func (a *Adapter) ListMyWorkItems(top int, opts provider.ListOpts) ([]provider.WorkItem, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	return a.mc.ListMyWorkItems(top, opts)
}

// --------------------------------------------------------------------------
// Work-item detail / mutation surface
// --------------------------------------------------------------------------

// GetWorkItemTypeStates returns the valid states for the given work item type.
// GitHub issues support exactly two states (open, closed); the Client returns
// them as neutral provider.WorkItemTypeState values directly.
// scope routes to the correct per-repo Client.
func (a *Adapter) GetWorkItemTypeStates(scope, workItemType string) ([]provider.WorkItemTypeState, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	return c.GetWorkItemTypeStates(workItemType)
}

// UpdateWorkItemState transitions the given issue to the specified state
// ("open" or "closed"). scope routes to the correct per-repo Client.
func (a *Adapter) UpdateWorkItemState(scope string, id int, state string) error {
	if a.mc == nil {
		return fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return fmt.Errorf("no client for scope %q", scope)
	}
	return c.UpdateWorkItemState(id, state)
}

// GetWorkItemComments returns the comments for the given issue, in the order
// returned by GitHub (chronological, oldest first).
// scope routes to the correct per-repo Client.
func (a *Adapter) GetWorkItemComments(scope string, id int) ([]provider.WorkItemComment, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.GetWorkItemComments(id)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	result := make([]provider.WorkItemComment, len(wire))
	for i, wc := range wire {
		result[i] = MapWorkItemComment(wc, scope, scopeDisplay)
	}
	return result, nil
}

// AddWorkItemComment posts a new comment on the given issue and returns the
// created comment mapped to a neutral provider.WorkItemComment.
// scope routes to the correct per-repo Client.
func (a *Adapter) AddWorkItemComment(scope string, id int, text string) (*provider.WorkItemComment, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.AddWorkItemComment(id, text)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	mapped := MapWorkItemComment(wire, scope, scopeDisplay)
	return &mapped, nil
}

// --------------------------------------------------------------------------
// Pipeline list surface — delegates to MultiClient (already neutral)
// --------------------------------------------------------------------------

// ListPipelineRuns returns up to top pipeline runs across all repos, sorted by
// QueueTime descending. opts carries neutral filter intent.
func (a *Adapter) ListPipelineRuns(top int, opts provider.ListOpts) ([]provider.PipelineRun, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	return a.mc.ListPipelineRuns(top, opts)
}

// --------------------------------------------------------------------------
// Pipeline detail surface
// --------------------------------------------------------------------------

// GetBuildTimeline returns the timeline for the given workflow run.
//
// Two sequential GETs are made (run + jobs); the wire pair is mapped to a
// provider.Timeline via MapTimeline. scope routes to the correct per-repo Client.
func (a *Adapter) GetBuildTimeline(scope string, buildID int) (*provider.Timeline, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	run, jobs, err := c.GetBuildTimeline(buildID)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	tl := MapTimeline(run, jobs, scope, scopeDisplay)
	return &tl, nil
}

// GetBuildLogContent returns the plaintext log for the given job.
//
// logID is the GitHub job ID (as stamped by MapTimeline on Job records).
// A logID of 0 indicates a Step record; steps share their parent Job's log.
// scope routes to the correct per-repo Client.
func (a *Adapter) GetBuildLogContent(scope string, buildID, logID int) (string, error) {
	if a.mc == nil {
		return "", fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return "", fmt.Errorf("no client for scope %q", scope)
	}
	return c.GetBuildLogContent(buildID, logID)
}

// --------------------------------------------------------------------------
// Web URL helpers — route via ClientFor(scope) and delegate to Client builders
// --------------------------------------------------------------------------

// WorkItemURL returns the github.com browser URL for the given issue.
// Returns "" when the client is nil or scope is unknown.
func (a *Adapter) WorkItemURL(scope string, id int) string {
	if a.mc == nil {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return c.WorkItemURL(id)
}

// PRURL returns the github.com browser URL for the given pull request.
// repositoryID is ignored for GitHub (scope identifies the repo).
// Returns "" when the client is nil or scope is unknown.
func (a *Adapter) PRURL(scope, repositoryID string, prID int) string {
	if a.mc == nil {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return c.PRURL(prID)
}

// PRThreadWebURL returns the github.com browser URL anchored to a specific
// review comment thread. repositoryID is ignored for GitHub.
// Returns "" when the client is nil, scope is unknown, or prID/threadID are invalid.
func (a *Adapter) PRThreadWebURL(scope, repositoryID string, prID int, threadID int) string {
	if a.mc == nil {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return c.PRThreadWebURL(prID, threadID)
}

// PipelineURL returns the github.com browser URL for the given Actions workflow run.
// Returns "" when the client is nil or scope is unknown.
func (a *Adapter) PipelineURL(scope string, id int) string {
	if a.mc == nil {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return c.PipelineURL(id)
}

// --------------------------------------------------------------------------
// provider.NotificationSource
// --------------------------------------------------------------------------

// List returns the caller's notification inbox, shaped by opts and mapped to
// the neutral provider.Notification type via MapNotification.
//
// opts.Max is accepted for interface compliance but ignored here: since
// decision 15 / task 12, NotifOpts.Max is honoured exclusively by
// provider.CompositeProvider.List, after it has merged every capable
// backend's rows and sorted them newest-first. Since decision C (review of
// task 12), CompositeProvider.List also zeroes opts.Max before calling this
// method, so opts.Max arrives here as 0 on every call reaching this adapter
// through the composite — but this method's own indifference to the field
// does not depend on that, and holds the same for a direct caller bypassing
// the composite with a nonzero Max. Truncating per-backend here
// (phase 1's original behaviour) would double-apply the cap once a second
// backend exists — each backend independently discarding rows outside its own
// Max before the composite ever sees the full picture, based on this
// backend's own (here, whatever nc.List/GitHub's API happens to return)
// order rather than the composite's canonical UpdatedAt/Kind/Scope/ID total
// order. That can silently drop a row that belongs in the true global top-Max
// in favour of one that does not, while still returning a plausible-looking,
// correctly-sized slice — worse than the over-fetch it would "fix", since the
// feed then looks correct while being wrong. It is therefore never forwarded
// into NotificationListOpts either. The returned slice is consequently no
// longer bounded by opts.Max at all: what bounds it is nc.List's own walk,
// capped at maxNotificationPages pages (internal/github/notifications.go).
// That page count is an unconditional bound; the size of each page is not.
// notifPerPageCap (100) sets per_page on the first request only (buildPath,
// notifications.go); pages 2..N are fetched by following the server's own
// Link rel="next" URL verbatim (nextPageURL, client.go), with no per-page
// rewrite. So "at most maxNotificationPages × notifPerPageCap rows" holds
// only conditionally — for as long as the server's own next-links keep
// returning close to notifPerPageCap rows per page, which GitHub's real API
// does but which nothing here enforces. A GHES instance or an intermediary
// caching proxy that ignores per_page in its Link header can return
// arbitrarily more rows per page; the walk still stops at maxNotificationPages
// pages, just with more rows in each. maxNotificationPages exists precisely
// to survive that kind of server misbehaviour (see its own doc comment,
// notifications.go), so the bound stated next to it must not assume the
// well-behaved case it was written to distrust.
//
// An unsolicited-304 error surfaced by nc.List (a 304 with no matching cache)
// is returned unchanged, never translated into an empty slice: an emptied
// attention feed reads as "you're clear", which must never be shown when the
// fetch actually failed.
//
// scopeDisplay is resolved per-row from the thread's own repository
// (MapNotification is the adapter boundary for the Scope/ScopeDisplay
// fallback): a.mc.DisplayNameFor(scope) is used when mc is configured, and left
// empty (letting the mapper fall back to Scope itself) when it is not — most
// inbox rows come from repos that are not configured at all, so mc cannot
// resolve a display name for them anyway.
func (a *Adapter) List(opts provider.NotifOpts) ([]provider.Notification, error) {
	if a.nc == nil {
		return nil, fmt.Errorf("github: notifications: no notifications client configured")
	}

	wire, err := a.nc.List(NotificationListOpts{
		Participating: opts.ParticipatingOnly,
		Since:         opts.Since,
	})
	if err != nil {
		return nil, err
	}

	out := make([]provider.Notification, len(wire))
	for i, thread := range wire {
		scopeDisplay := ""
		if a.mc != nil {
			scopeDisplay = a.mc.DisplayNameFor(thread.Repository.FullName)
		}
		out[i] = MapNotification(thread, scopeDisplay)
	}

	return out, nil
}

// MarkRead marks the given notification as read via
// PATCH /notifications/threads/{id}, forwarding id.ID straight through to
// nc.MarkRead.
//
// id.Kind must be provider.KindGitHub. A mismatched Kind (e.g. an Azure
// identity reaching the GitHub adapter) is rejected as a caller bug: a bare
// native id from one backend must never be mistaken for another's, and
// silently issuing id.ID against GitHub's API for an identity that does not
// actually belong to GitHub would be exactly that mistake, just deferred to
// runtime instead of caught here. The zero Kind is rejected by the same check —
// the composite routes by Identity.Kind, so only KindGitHub should ever arrive
// here and an unstamped identity is the same caller bug. Both kinds are
// formatted with %q rather than %v: Kind.String() returns "" for the zero
// value, so %v renders it as a blank hole in the middle of the sentence
// ("identity kind  is not github").
//
// This method takes no lock of its own, and in particular never the fetch
// mutex nc.List holds for the duration of its (possibly multi-page) walk — the
// entire benefit is a lock-free marker, and wrapping this call in any lock
// shared with List would reinstate a stall of up to maxNotificationPages times
// the HTTP timeout on a call a tea.Cmd user expects to feel instant.
func (a *Adapter) MarkRead(id provider.Identity) error {
	if a.nc == nil {
		return fmt.Errorf("github: mark read: no notifications client configured")
	}
	if id.Kind != provider.KindGitHub {
		return fmt.Errorf("github: mark read: identity kind %q is not %q", id.Kind, provider.KindGitHub)
	}
	return a.nc.MarkRead(id.ID)
}

// MarkDone marks the given notification as done via
// DELETE /notifications/threads/{id}, forwarding id.ID straight through to
// nc.MarkDone. See MarkRead's doc comment: the nil-nc guard, the Kind check,
// and the no-lock contract are all shared and not repeated here.
func (a *Adapter) MarkDone(id provider.Identity) error {
	if a.nc == nil {
		return fmt.Errorf("github: mark done: no notifications client configured")
	}
	if id.Kind != provider.KindGitHub {
		return fmt.Errorf("github: mark done: identity kind %q is not %q", id.Kind, provider.KindGitHub)
	}
	return a.nc.MarkDone(id.ID)
}

// PollInterval forwards nc's cadence hint (parsed from GitHub's
// X-Poll-Interval response header) so Adapter satisfies PollIntervalHinter:
// the composite provider holds *Adapter, never *NotificationsClient directly,
// so the hint needs a route from here.
//
// Returns 0 when nc is nil (no notifications client configured) — there is no
// error return on this method to report that condition through, and 0 is a
// safe "no hint" value for a caller applying the max(hint, configured) rule;
// it never falls back to nc's internal default on its own, since there is no
// nc to ask.
func (a *Adapter) PollInterval() time.Duration {
	if a.nc == nil {
		return 0
	}
	return a.nc.PollInterval()
}
