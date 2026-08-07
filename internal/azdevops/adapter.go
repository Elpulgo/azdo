package azdevops

import (
	"fmt"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Adapter wraps a MultiClient and satisfies provider.Provider.
// It maps azdevops wire types to neutral provider types at the boundary,
// stamping Identity (Kind, Scope, ScopeDisplay, ID) on every returned entity.
// Metrics methods (MetricsWorkItems, WorkItemUpdates, GetOrg) are not part of
// the interface — they remain on the concrete *MultiClient (Decision 5).
type Adapter struct {
	mc *MultiClient

	// notifStore, notifLookbackDays and notifSources back the
	// provider.NotificationSource surface (adapter_notifications.go) only.
	// A plain NewAdapter(mc) leaves every notification-related field below
	// at its zero value; List, MarkRead and MarkDone all treat a nil
	// notifStore as "notifications not configured" the same way every other
	// method on this type treats a nil mc — see NewAdapterWithNotifications.
	notifStore        *TriageStore
	notifLookbackDays int
	notifSources      NotificationSourceToggles

	// notifThrottleMu, notifMinPollInterval, notifLastPollAt and
	// notifLastRawRows implement decision 10's self-throttle: List
	// re-reconciles and returns fresh rows on every call, but skips the
	// network round trip and the store write when called sooner than
	// notifMinPollInterval after its last real query (see list's own doc
	// comment for what "throttled" still does). This lock is intentionally
	// distinct from notifStore's own mu/writeMu (notifications_store.go):
	// List holds notifThrottleMu for its entire call, including the network
	// round trip in runSourcesConcurrently and the later notifStore.Swap,
	// while MarkRead/MarkDone only ever acquire notifStore's own mu (via
	// ApplyIfChanged) and never touch notifThrottleMu at all. Lock order for
	// the one call path that ever holds both — list — is notifThrottleMu
	// before notifStore's mu; no other code acquires them in the opposite
	// order, so there is no cycle to deadlock on.
	//
	// A throttled call still re-runs Reconcile — in memory, against
	// notifLastRawRows (the pre-Reconcile rows from the last real query) and
	// the store's *current* triage state (read via notifStore.State(), which
	// only takes notifStore's mu, honouring the same lock order) — so a mark
	// made after the last real query is reflected on the very next throttled
	// return instead of being masked until the throttle window closes (task
	// 14 review, 🔴 finding 1). The state Reconcile returns from that call is
	// discarded, not written back: a throttled call still does no network
	// work and writes nothing to the store.
	//
	// The consequence of list holding notifThrottleMu across its whole body:
	// two concurrent List calls on one *Adapter serialise, the second
	// waiting out the first's full network round trip rather than getting a
	// cheap throttled return. Reachable in this codebase — internal/app/app.go
	// hands the same CompositeProvider to both the notifications pane's own
	// fetch and polling.NewNotificationsPoller's timer, each its own
	// goroutine. This is a deliberate latency trade, not a defect (task 14
	// review, 🟡 finding 3), but its safety rests on a precondition, not a
	// guarantee: a blocked caller's now is the time it entered List, which is
	// strictly *later* than the winner's notifLastPollAt (the winner's own
	// entry-time now), not earlier or equal — so the blocked caller is only
	// throttled if the winner's entire round trip (runSourcesConcurrently
	// plus notifStore.Swap) finishes before notifMinPollInterval has elapsed
	// since the winner started. At shipped defaults (a 300-second
	// notifMinPollInterval against sub-second real-world round trips) that
	// holds by a wide margin. It is not an absolute: at
	// notifMinPollInterval=100ms with a round trip parked at ~300ms, a
	// blocked caller's own now can already be past notifLastPollAt+interval
	// by the time it acquires the lock, so it takes the real-query branch
	// too — the throttle does not prevent a second real query in that case,
	// it just delays it behind the first. Reduce notifMinPollInterval far
	// enough relative to real request latency and this stops holding. Do not
	// "fix" the serialisation with a per-caller lock or a singleflight
	// variant without accounting for the freshness trade it currently
	// gives: the blocked caller gets the cache the winner *just wrote*,
	// fresher than an immediate throttled return would have been — the one
	// edge case is cosmetic: if the winner hits the total-failure branch,
	// the loser returns the older cached rows with a nil error instead of
	// the winner's error.
	notifThrottleMu      sync.Mutex
	notifMinPollInterval time.Duration
	notifLastPollAt      time.Time
	notifLastRawRows     []provider.Notification
}

// NewAdapter creates a new Adapter wrapping the given MultiClient.
// A nil MultiClient is allowed (adapter still satisfies the interface; all
// methods that require a live client will return an error or zero value).
//
// The *Adapter this returns also satisfies provider.NotificationSource by
// method set alone — Go's structural typing does not care that notifStore is
// left at its zero value (nil) here. A caller that hands this result to
// something that type-asserts for provider.NotificationSource (e.g. the
// composite provider's Notifications tab capability check) gets a source
// that is present but permanently broken: List's nil-notifStore guard
// (adapter_notifications.go) makes every call return an error rather than
// panic, so the tab shows up and every poll immediately renders
// internal/ui/notifications' error state ("Notifications unavailable:
// azdevops: notifications: not configured") instead of ever showing a row —
// present, but useless, for the life of the session (task 8 review, 🔴
// finding 1). Use NewAdapterWithNotifications instead of this constructor
// whenever the caller wants Azure DevOps notifications to actually work;
// cmd/azdo-tui's runTUI does, and TestRunTUI_UsesAzureAdapterWithNotifications
// (cmd/azdo-tui/main_test.go) pins that it keeps doing so.
func NewAdapter(mc *MultiClient) *Adapter {
	return &Adapter{mc: mc}
}

// Kind returns the backend kind for this adapter.
func (a *Adapter) Kind() provider.Kind {
	return provider.KindAzure
}

// IsMultiProject returns true when more than one project is configured.
func (a *Adapter) IsMultiProject() bool {
	if a.mc == nil {
		return false
	}
	return a.mc.IsMultiProject()
}

// Scopes returns the Azure DevOps project API names this adapter spans.
// Returns nil when no client is configured.
func (a *Adapter) Scopes() []string {
	if a.mc == nil {
		return nil
	}
	return a.mc.Projects()
}

// --- Pull-request surface ---

// ListPullRequests returns up to top active pull requests across all projects,
// mapping wire types to neutral types with identity stamped per project.
// opts is accepted for interface compliance; PR filtering is handled by the
// REST API's searchCriteria parameters rather than WIQL, so opts fields other
// than Top are unused at this layer (they will be wired in Task 9).
func (a *Adapter) ListPullRequests(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	wire, err := a.mc.ListPullRequests(top)
	if err != nil {
		return nil, err
	}
	result := make([]provider.PullRequest, len(wire))
	for i, pr := range wire {
		result[i] = MapPullRequest(pr, pr.ProjectName, pr.ProjectDisplayName)
	}
	return result, nil
}

// ListMyPullRequests returns up to top pull requests created by the
// authenticated user, mapped to neutral types.
// opts is accepted for interface compliance; additional filtering will be
// wired in Task 9.
func (a *Adapter) ListMyPullRequests(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	wire, err := a.mc.ListMyPullRequests(top)
	if err != nil {
		return nil, err
	}
	result := make([]provider.PullRequest, len(wire))
	for i, pr := range wire {
		result[i] = MapPullRequest(pr, pr.ProjectName, pr.ProjectDisplayName)
	}
	return result, nil
}

// ListPullRequestsAsReviewer returns up to top pull requests where the
// authenticated user is a reviewer, mapped to neutral types.
// opts is accepted for interface compliance; additional filtering will be
// wired in Task 9.
func (a *Adapter) ListPullRequestsAsReviewer(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	wire, err := a.mc.ListPullRequestsAsReviewer(top)
	if err != nil {
		return nil, err
	}
	result := make([]provider.PullRequest, len(wire))
	for i, pr := range wire {
		result[i] = MapPullRequest(pr, pr.ProjectName, pr.ProjectDisplayName)
	}
	return result, nil
}

// GetPRThreads returns the comment threads for the given pull request.
// scope routes to the correct project sub-client.
func (a *Adapter) GetPRThreads(scope, repositoryID string, pullRequestID int) ([]provider.Thread, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.GetPRThreads(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	result := make([]provider.Thread, len(wire))
	for i, t := range wire {
		result[i] = MapThread(t, scope, scopeDisplay)
	}
	return result, nil
}

// GetPRIterations returns all iterations for the given pull request.
// scope routes to the correct project sub-client.
func (a *Adapter) GetPRIterations(scope, repositoryID string, pullRequestID int) ([]provider.Iteration, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.GetPRIterations(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	result := make([]provider.Iteration, len(wire))
	for i, it := range wire {
		result[i] = MapIteration(it)
	}
	return result, nil
}

// GetPRIterationChanges returns the files changed in the given PR iteration.
// scope routes to the correct project sub-client.
func (a *Adapter) GetPRIterationChanges(scope, repositoryID string, pullRequestID int, iterationID int) ([]provider.IterationChange, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.GetPRIterationChanges(repositoryID, pullRequestID, iterationID)
	if err != nil {
		return nil, err
	}
	result := make([]provider.IterationChange, len(wire))
	for i, ic := range wire {
		result[i] = MapIterationChange(ic)
	}
	return result, nil
}

// VotePullRequest submits a reviewer vote on the given pull request.
// scope routes to the correct project sub-client.
func (a *Adapter) VotePullRequest(scope, repositoryID string, pullRequestID int, vote int) error {
	if a.mc == nil {
		return fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return fmt.Errorf("no client for scope %q", scope)
	}
	return c.VotePullRequest(repositoryID, pullRequestID, vote)
}

// GetFileContent returns the raw file content at the given branch ref.
// scope routes to the correct project sub-client.
func (a *Adapter) GetFileContent(scope, repositoryID string, filePath string, branchName string) (string, error) {
	if a.mc == nil {
		return "", fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return "", fmt.Errorf("no client for scope %q", scope)
	}
	return c.GetFileContent(repositoryID, filePath, branchName)
}

// AddPRCodeComment creates a new inline code comment on the given line.
// scope routes to the correct project sub-client.
func (a *Adapter) AddPRCodeComment(scope, repositoryID string, pullRequestID int, filePath string, line int, content string) (*provider.Thread, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.AddPRCodeComment(repositoryID, pullRequestID, filePath, line, content)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	mapped := MapThread(*wire, scope, scopeDisplay)
	return &mapped, nil
}

// AddPRComment creates a new general (non-file) comment thread on the PR.
// scope routes to the correct project sub-client.
func (a *Adapter) AddPRComment(scope, repositoryID string, pullRequestID int, content string) (*provider.Thread, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.AddPRComment(repositoryID, pullRequestID, content)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	mapped := MapThread(*wire, scope, scopeDisplay)
	return &mapped, nil
}

// ReplyToThread posts a reply to an existing comment thread.
// scope routes to the correct project sub-client.
func (a *Adapter) ReplyToThread(scope, repositoryID string, pullRequestID int, threadID int, content string) (*provider.Comment, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.ReplyToThread(repositoryID, pullRequestID, threadID, content)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	mapped := MapComment(*wire, scope, scopeDisplay)
	return &mapped, nil
}

// UpdateThreadStatus sets the status of a comment thread.
// scope routes to the correct project sub-client.
func (a *Adapter) UpdateThreadStatus(scope, repositoryID string, pullRequestID int, threadID int, status string) error {
	if a.mc == nil {
		return fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return fmt.Errorf("no client for scope %q", scope)
	}
	return c.UpdateThreadStatus(repositoryID, pullRequestID, threadID, status)
}

// --- Work-item surface ---

// ListWorkItems returns up to top work items across all projects,
// mapped to neutral types. opts carries neutral filter intent; zero value
// reproduces the current default behavior. Additional WIQL filters from opts
// will be applied in Task 9 — for now the adapter accepts opts for interface
// compliance.
func (a *Adapter) ListWorkItems(top int, opts provider.ListOpts) ([]provider.WorkItem, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	wire, err := a.mc.ListWorkItems(top)
	if err != nil {
		return nil, err
	}
	result := make([]provider.WorkItem, len(wire))
	for i, wi := range wire {
		result[i] = MapWorkItem(wi, wi.ProjectName, wi.ProjectDisplayName)
	}
	return result, nil
}

// ListMyWorkItems returns up to top work items assigned to the authenticated
// user, mapped to neutral types. opts carries neutral filter intent; zero value
// reproduces the current default behavior. Additional WIQL filters from opts
// will be applied in Task 9.
func (a *Adapter) ListMyWorkItems(top int, opts provider.ListOpts) ([]provider.WorkItem, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	wire, err := a.mc.ListMyWorkItems(top)
	if err != nil {
		return nil, err
	}
	result := make([]provider.WorkItem, len(wire))
	for i, wi := range wire {
		result[i] = MapWorkItem(wi, wi.ProjectName, wi.ProjectDisplayName)
	}
	return result, nil
}

// GetWorkItemTypeStates returns the valid states for the given work item type.
// scope routes to the correct project sub-client.
func (a *Adapter) GetWorkItemTypeStates(scope, workItemType string) ([]provider.WorkItemTypeState, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.GetWorkItemTypeStates(workItemType)
	if err != nil {
		return nil, err
	}
	result := make([]provider.WorkItemTypeState, len(wire))
	for i, s := range wire {
		result[i] = MapWorkItemTypeState(s)
	}
	return result, nil
}

// UpdateWorkItemState transitions the given work item to the specified state.
// scope routes to the correct project sub-client.
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

// GetWorkItemComments returns discussion comments for the given work item,
// ordered newest first. scope routes to the correct project sub-client.
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

// AddWorkItemComment posts a new comment on the given work item.
// scope routes to the correct project sub-client.
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
	mapped := MapWorkItemComment(*wire, scope, scopeDisplay)
	return &mapped, nil
}

// --- Pipeline surface ---

// ListPipelineRuns returns up to top recent pipeline runs across all projects,
// mapped to neutral types. opts carries neutral filter intent; zero value
// reproduces the current default behavior. Status filtering from opts will be
// applied in Task 9.
func (a *Adapter) ListPipelineRuns(top int, opts provider.ListOpts) ([]provider.PipelineRun, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	wire, err := a.mc.ListPipelineRuns(top)
	if err != nil {
		return nil, err
	}
	result := make([]provider.PipelineRun, len(wire))
	for i, p := range wire {
		result[i] = MapPipelineRun(p, p.ProjectName, p.ProjectDisplayName)
	}
	return result, nil
}

// GetBuildTimeline returns the timeline for the given build.
// scope routes to the correct project sub-client.
func (a *Adapter) GetBuildTimeline(scope string, buildID int) (*provider.Timeline, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("no client configured")
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return nil, fmt.Errorf("no client for scope %q", scope)
	}
	wire, err := c.GetBuildTimeline(buildID)
	if err != nil {
		return nil, err
	}
	scopeDisplay := a.mc.DisplayNameFor(scope)
	mapped := MapTimeline(*wire, scope, scopeDisplay)
	return &mapped, nil
}

// GetBuildLogContent returns the raw log text for the given log within a build.
// scope routes to the correct project sub-client.
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

// --- Web URL helpers (Decision 6) ---

// WorkItemURL returns the browser URL for the given work item ID.
// scope is the project name used to route to the correct sub-client.
// Returns "" when the client is nil or scope does not match a known project.
func (a *Adapter) WorkItemURL(scope string, id int) string {
	if a.mc == nil {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d",
		c.GetOrg(), c.GetProject(), id)
}

// PRURL returns the browser URL for the given pull request in the given repository.
// scope is the project name used to route to the correct sub-client.
// Returns "" when the client is nil, scope does not match a known project, or
// repositoryID is empty (matching the guard in the retired inline builder).
func (a *Adapter) PRURL(scope, repositoryID string, prID int) string {
	if a.mc == nil {
		return ""
	}
	if repositoryID == "" {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d",
		c.GetOrg(), c.GetProject(), repositoryID, prID)
}

// PRThreadWebURL returns the browser URL for a specific comment thread in the
// given pull request. The URL includes ?discussionId=threadID so the browser
// anchors directly to that thread. Returns "" when the client is nil, scope
// does not match a known project, repositoryID is empty, or threadID is zero.
func (a *Adapter) PRThreadWebURL(scope, repositoryID string, prID int, threadID int) string {
	if a.mc == nil {
		return ""
	}
	if repositoryID == "" {
		return ""
	}
	if threadID == 0 {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d?discussionId=%d",
		c.GetOrg(), c.GetProject(), repositoryID, prID, threadID)
}

// PipelineURL returns the browser URL for the given pipeline build ID.
// scope is the project name used to route to the correct sub-client.
// Returns "" when the client is nil or scope does not match a known project.
func (a *Adapter) PipelineURL(scope string, id int) string {
	if a.mc == nil {
		return ""
	}
	c := a.mc.ClientFor(scope)
	if c == nil {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_build/results?buildId=%d",
		c.GetOrg(), c.GetProject(), id)
}
