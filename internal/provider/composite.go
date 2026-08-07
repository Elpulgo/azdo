package provider

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// CompositeProvider fans out list calls across multiple Provider backends and
// routes detail/mutation/URL calls to the backend that owns the given scope.
//
// Backends are stored in registration order; the scope→backend routing index
// is built once at construction time from each backend's Scopes() output.
//
// Scope collision: when two backends claim the same scope string, the
// first-registered backend wins for all routed calls ("first" in registration
// order, not alphabetically).
//
// A single-backend CompositeProvider is transparent: list methods fan out to
// that one backend, run the (idempotent) sort, and return the same slice.
//
// Kind() returns the sole backend's Kind when there is one backend, or the
// first backend's Kind when there are multiple. No consumer reads Kind() for
// rendering — per-row Identity.Kind drives glyphs.
type CompositeProvider struct {
	backends []Provider
	// routing maps scope → backend index in backends slice.
	// Built at construction time; collisions resolved first-backend-wins.
	routing map[string]Provider
}

// NewCompositeProvider constructs a CompositeProvider wrapping the given
// backends in registration order. At least one backend must be provided.
// The scope→backend routing index is built once here; collision resolution is
// first-registered backend wins.
func NewCompositeProvider(backends ...Provider) *CompositeProvider {
	routing := make(map[string]Provider, len(backends))
	for _, b := range backends {
		for _, scope := range b.Scopes() {
			if _, exists := routing[scope]; !exists {
				routing[scope] = b
			}
		}
	}
	return &CompositeProvider{
		backends: backends,
		routing:  routing,
	}
}

// compile-time assertion: CompositeProvider must satisfy provider.Provider.
var _ Provider = (*CompositeProvider)(nil)

// compile-time assertion: CompositeProvider must satisfy
// provider.NotificationSource. It implements this interface
// unconditionally, fanning out only to the backends that themselves
// implement it (see NotificationSource's doc comment).
var _ NotificationSource = (*CompositeProvider)(nil)

// backendFor returns the backend responsible for the given scope.
// Returns nil when the scope is not registered.
func (cp *CompositeProvider) backendFor(scope string) Provider {
	return cp.routing[scope]
}

// routeErr returns a descriptive error for an unknown scope.
func routeErr(scope string) error {
	return fmt.Errorf("composite: no backend registered for scope %q", scope)
}

// --- Cross-cutting ---

// Kind returns the sole backend's Kind when one backend is configured, or the
// first backend's Kind when multiple backends are present.
func (cp *CompositeProvider) Kind() Kind {
	if len(cp.backends) == 0 {
		return 0
	}
	return cp.backends[0].Kind()
}

// IsMultiProject returns true when the union of all backends' scopes spans more
// than one scope, which signals the list views to show the Project column.
func (cp *CompositeProvider) IsMultiProject() bool {
	return len(cp.routing) > 1
}

// Scopes returns the union of all backends' scopes in registration order.
// When two backends claim the same scope string, only the first occurrence is
// included (first-registered wins).
func (cp *CompositeProvider) Scopes() []string {
	seen := make(map[string]struct{})
	var out []string
	for _, b := range cp.backends {
		for _, scope := range b.Scopes() {
			if _, exists := seen[scope]; !exists {
				seen[scope] = struct{}{}
				out = append(out, scope)
			}
		}
	}
	return out
}

// --- Pull-request list methods ---

// ListPullRequests fans out to all backends concurrently, merges, and sorts by
// CreationDate descending. Returns *PartialError on partial failure; plain error
// when all backends fail.
func (cp *CompositeProvider) ListPullRequests(top int, opts ListOpts) ([]PullRequest, error) {
	type result struct {
		prs []PullRequest
		err error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(cp.backends))

	for _, b := range cp.backends {
		wg.Add(1)
		go func(backend Provider) {
			defer wg.Done()
			prs, err := backend.ListPullRequests(top, opts)
			ch <- result{prs, err}
		}(b)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []PullRequest
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.prs...)
	}

	return mergePRs(all, errs, len(cp.backends))
}

// ListMyPullRequests fans out to all backends concurrently, merges, and sorts
// by CreationDate descending.
func (cp *CompositeProvider) ListMyPullRequests(top int, opts ListOpts) ([]PullRequest, error) {
	type result struct {
		prs []PullRequest
		err error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(cp.backends))

	for _, b := range cp.backends {
		wg.Add(1)
		go func(backend Provider) {
			defer wg.Done()
			prs, err := backend.ListMyPullRequests(top, opts)
			ch <- result{prs, err}
		}(b)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []PullRequest
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.prs...)
	}

	return mergePRs(all, errs, len(cp.backends))
}

// ListPullRequestsAsReviewer fans out to all backends concurrently, merges,
// and sorts by CreationDate descending.
func (cp *CompositeProvider) ListPullRequestsAsReviewer(top int, opts ListOpts) ([]PullRequest, error) {
	type result struct {
		prs []PullRequest
		err error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(cp.backends))

	for _, b := range cp.backends {
		wg.Add(1)
		go func(backend Provider) {
			defer wg.Done()
			prs, err := backend.ListPullRequestsAsReviewer(top, opts)
			ch <- result{prs, err}
		}(b)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []PullRequest
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.prs...)
	}

	return mergePRs(all, errs, len(cp.backends))
}

// mergePRs sorts PRs by CreationDate descending and applies partial-error logic.
func mergePRs(all []PullRequest, errs []error, total int) ([]PullRequest, error) {
	if len(errs) == total {
		return nil, fmt.Errorf("composite: all backends failed: %v", errs)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreationDate.After(all[j].CreationDate)
	})
	if len(errs) > 0 {
		return all, &PartialError{Failed: len(errs), Total: total, Errors: errs}
	}
	return all, nil
}

// --- PR detail/mutation methods ---

// GetPRThreads delegates to the backend registered for scope.
func (cp *CompositeProvider) GetPRThreads(scope, repositoryID string, pullRequestID int) ([]Thread, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.GetPRThreads(scope, repositoryID, pullRequestID)
}

// GetPRIterations delegates to the backend registered for scope.
func (cp *CompositeProvider) GetPRIterations(scope, repositoryID string, pullRequestID int) ([]Iteration, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.GetPRIterations(scope, repositoryID, pullRequestID)
}

// GetPRIterationChanges delegates to the backend registered for scope.
func (cp *CompositeProvider) GetPRIterationChanges(scope, repositoryID string, pullRequestID int, iterationID int) ([]IterationChange, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.GetPRIterationChanges(scope, repositoryID, pullRequestID, iterationID)
}

// VotePullRequest delegates to the backend registered for scope.
func (cp *CompositeProvider) VotePullRequest(scope, repositoryID string, pullRequestID int, vote int) error {
	b := cp.backendFor(scope)
	if b == nil {
		return routeErr(scope)
	}
	return b.VotePullRequest(scope, repositoryID, pullRequestID, vote)
}

// GetFileContent delegates to the backend registered for scope.
func (cp *CompositeProvider) GetFileContent(scope, repositoryID string, filePath string, branchName string) (string, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return "", routeErr(scope)
	}
	return b.GetFileContent(scope, repositoryID, filePath, branchName)
}

// AddPRCodeComment delegates to the backend registered for scope.
func (cp *CompositeProvider) AddPRCodeComment(scope, repositoryID string, pullRequestID int, filePath string, line int, content string) (*Thread, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.AddPRCodeComment(scope, repositoryID, pullRequestID, filePath, line, content)
}

// AddPRComment delegates to the backend registered for scope.
func (cp *CompositeProvider) AddPRComment(scope, repositoryID string, pullRequestID int, content string) (*Thread, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.AddPRComment(scope, repositoryID, pullRequestID, content)
}

// ReplyToThread delegates to the backend registered for scope.
func (cp *CompositeProvider) ReplyToThread(scope, repositoryID string, pullRequestID int, threadID int, content string) (*Comment, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.ReplyToThread(scope, repositoryID, pullRequestID, threadID, content)
}

// UpdateThreadStatus delegates to the backend registered for scope.
func (cp *CompositeProvider) UpdateThreadStatus(scope, repositoryID string, pullRequestID int, threadID int, status string) error {
	b := cp.backendFor(scope)
	if b == nil {
		return routeErr(scope)
	}
	return b.UpdateThreadStatus(scope, repositoryID, pullRequestID, threadID, status)
}

// --- Work-item list methods ---

// ListWorkItems fans out to all backends concurrently, merges, and sorts by
// ChangedDate descending.
func (cp *CompositeProvider) ListWorkItems(top int, opts ListOpts) ([]WorkItem, error) {
	type result struct {
		items []WorkItem
		err   error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(cp.backends))

	for _, b := range cp.backends {
		wg.Add(1)
		go func(backend Provider) {
			defer wg.Done()
			items, err := backend.ListWorkItems(top, opts)
			ch <- result{items, err}
		}(b)
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
		all = append(all, r.items...)
	}

	return mergeWorkItems(all, errs, len(cp.backends))
}

// ListMyWorkItems fans out to all backends concurrently, merges, and sorts by
// ChangedDate descending.
func (cp *CompositeProvider) ListMyWorkItems(top int, opts ListOpts) ([]WorkItem, error) {
	type result struct {
		items []WorkItem
		err   error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(cp.backends))

	for _, b := range cp.backends {
		wg.Add(1)
		go func(backend Provider) {
			defer wg.Done()
			items, err := backend.ListMyWorkItems(top, opts)
			ch <- result{items, err}
		}(b)
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
		all = append(all, r.items...)
	}

	return mergeWorkItems(all, errs, len(cp.backends))
}

// mergeWorkItems sorts by ChangedDate descending and applies partial-error logic.
func mergeWorkItems(all []WorkItem, errs []error, total int) ([]WorkItem, error) {
	if len(errs) == total {
		return nil, fmt.Errorf("composite: all backends failed: %v", errs)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ChangedDate.After(all[j].ChangedDate)
	})
	if len(errs) > 0 {
		return all, &PartialError{Failed: len(errs), Total: total, Errors: errs}
	}
	return all, nil
}

// --- Work-item detail/mutation methods ---

// GetWorkItemTypeStates delegates to the backend registered for scope.
func (cp *CompositeProvider) GetWorkItemTypeStates(scope, workItemType string) ([]WorkItemTypeState, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.GetWorkItemTypeStates(scope, workItemType)
}

// UpdateWorkItemState delegates to the backend registered for scope.
func (cp *CompositeProvider) UpdateWorkItemState(scope string, id int, state string) error {
	b := cp.backendFor(scope)
	if b == nil {
		return routeErr(scope)
	}
	return b.UpdateWorkItemState(scope, id, state)
}

// GetWorkItemComments delegates to the backend registered for scope.
func (cp *CompositeProvider) GetWorkItemComments(scope string, id int) ([]WorkItemComment, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.GetWorkItemComments(scope, id)
}

// AddWorkItemComment delegates to the backend registered for scope.
func (cp *CompositeProvider) AddWorkItemComment(scope string, id int, text string) (*WorkItemComment, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.AddWorkItemComment(scope, id, text)
}

// --- Pipeline list methods ---

// ListPipelineRuns fans out to all backends concurrently, merges, and sorts by
// QueueTime descending.
func (cp *CompositeProvider) ListPipelineRuns(top int, opts ListOpts) ([]PipelineRun, error) {
	type result struct {
		runs []PipelineRun
		err  error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(cp.backends))

	for _, b := range cp.backends {
		wg.Add(1)
		go func(backend Provider) {
			defer wg.Done()
			runs, err := backend.ListPipelineRuns(top, opts)
			ch <- result{runs, err}
		}(b)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []PipelineRun
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.runs...)
	}

	return mergePipelineRuns(all, errs, len(cp.backends))
}

// mergePipelineRuns sorts by QueueTime descending and applies partial-error logic.
func mergePipelineRuns(all []PipelineRun, errs []error, total int) ([]PipelineRun, error) {
	if len(errs) == total {
		return nil, fmt.Errorf("composite: all backends failed: %v", errs)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].QueueTime.After(all[j].QueueTime)
	})
	if len(errs) > 0 {
		return all, &PartialError{Failed: len(errs), Total: total, Errors: errs}
	}
	return all, nil
}

// --- Pipeline detail methods ---

// GetBuildTimeline delegates to the backend registered for scope.
func (cp *CompositeProvider) GetBuildTimeline(scope string, buildID int) (*Timeline, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return nil, routeErr(scope)
	}
	return b.GetBuildTimeline(scope, buildID)
}

// GetBuildLogContent delegates to the backend registered for scope.
func (cp *CompositeProvider) GetBuildLogContent(scope string, buildID, logID int) (string, error) {
	b := cp.backendFor(scope)
	if b == nil {
		return "", routeErr(scope)
	}
	return b.GetBuildLogContent(scope, buildID, logID)
}

// --- Web URL helpers ---

// WorkItemURL delegates to the backend registered for scope. Returns "" for
// unknown scopes.
func (cp *CompositeProvider) WorkItemURL(scope string, id int) string {
	b := cp.backendFor(scope)
	if b == nil {
		return ""
	}
	return b.WorkItemURL(scope, id)
}

// PRURL delegates to the backend registered for scope. Returns "" for unknown
// scopes.
func (cp *CompositeProvider) PRURL(scope, repositoryID string, prID int) string {
	b := cp.backendFor(scope)
	if b == nil {
		return ""
	}
	return b.PRURL(scope, repositoryID, prID)
}

// PRThreadWebURL delegates to the backend registered for scope. Returns "" for
// unknown scopes.
func (cp *CompositeProvider) PRThreadWebURL(scope, repositoryID string, prID int, threadID int) string {
	b := cp.backendFor(scope)
	if b == nil {
		return ""
	}
	return b.PRThreadWebURL(scope, repositoryID, prID, threadID)
}

// PipelineURL delegates to the backend registered for scope. Returns "" for
// unknown scopes.
func (cp *CompositeProvider) PipelineURL(scope string, id int) string {
	b := cp.backendFor(scope)
	if b == nil {
		return ""
	}
	return b.PipelineURL(scope, id)
}

// --- Notifications (ADR 0001) ---

// capableNotificationBackends returns the backends, in registration order,
// that implement NotificationSource. Notifications cannot reuse the
// scope-routed fan-out (GET /notifications is user-level, not per-repo), so
// list fan-out and mark routing both filter to this set independently rather
// than sharing routing/backendFor.
func (cp *CompositeProvider) capableNotificationBackends() []NotificationSource {
	var out []NotificationSource
	for _, b := range cp.backends {
		if src, ok := b.(NotificationSource); ok {
			out = append(out, src)
		}
	}
	return out
}

// HasNotifications reports whether at least one backend implements
// NotificationSource. The notifications tab is hidden on this capability
// check alone, never on the feed being empty.
//
// This is the only correct capability check for a *CompositeProvider:
// asserting a *CompositeProvider to NotificationSource always succeeds (it
// implements the interface unconditionally, see the var _ assertion above),
// so a type assertion against the composite reports support even when no
// backend can supply an inbox. Assert the concrete backend, or call this.
func (cp *CompositeProvider) HasNotifications() bool {
	for _, b := range cp.backends {
		if _, ok := b.(NotificationSource); ok {
			return true
		}
	}
	return false
}

// NotificationsPollInterval reports the largest polling-cadence hint among
// capable backends that also implement PollIntervalHinter. It is deliberately
// a max, not a first-match or an average: the poller treats the hint as a
// floor to raise the configured interval to, never to lower it, so the
// composite must surface the most conservative (largest) hint across every
// backend it fans out to. A capable backend that does not implement
// PollIntervalHinter simply does not contribute — not the same as contributing 0.
//
// Returns 0 when no capable backend implements PollIntervalHinter, which the
// caller must treat as "no hint available" and fall back to the configured
// interval.
func (cp *CompositeProvider) NotificationsPollInterval() time.Duration {
	var max time.Duration
	for _, b := range cp.capableNotificationBackends() {
		hinter, ok := b.(PollIntervalHinter)
		if !ok {
			continue
		}
		if hint := hinter.PollInterval(); hint > max {
			max = hint
		}
	}
	return max
}

// List fans out to every backend that implements NotificationSource
// concurrently, merges the results, and sorts by UpdatedAt descending. A
// failing backend never empties the feed: the other backends' rows are still
// returned alongside a *PartialError. total is the count of capable backends,
// never len(cp.backends) — copying the PR/work-item/pipeline fan-outs' total
// verbatim would make an incapable Azure backend count toward "all backends
// failed" and turn a single GitHub 403 into an emptied feed. Zero capable
// backends returns (nil, nil), not the all-failed error, which len(errs) ==
// total would otherwise satisfy vacuously at 0 == 0.
//
// opts is not forwarded to backends verbatim: ParticipatingOnly and Since
// reach every backend unchanged, but Max is zeroed first (decision C — see
// the backendOpts comment below and mergeNotifications' own doc comment).
//
// A backend that returns rows *and* a non-nil error has its rows discarded:
// the error is recorded and the partial rows are not merged, because a
// backend reporting failure cannot vouch for the completeness or ordering of
// what it did return. No shipped implementation reaches this — github.Adapter
// returns (nil, err) on every error path — but the choice is deliberate.
func (cp *CompositeProvider) List(opts NotifOpts) ([]Notification, error) {
	capable := cp.capableNotificationBackends()

	// backendOpts carries ParticipatingOnly and Since through unchanged, but
	// zeroes Max (decision C, phase-2 notifications spec, review of task 12):
	// Max is honoured exclusively by this method's own post-merge cap via
	// mergeNotifications, applied once below after every backend's rows are
	// merged and sorted. Forwarding opts.Max verbatim would let a backend
	// that itself honours Max double-apply the cap the moment it does so — a
	// *CompositeProvider is itself a NotificationSource (see the var _
	// assertion above) and is the one implementation that does honour Max,
	// so a composite nested inside another composite would otherwise
	// reproduce exactly the double-apply task 12 removed at the adapter
	// layer. Zeroing it here makes a backend that happens to honour Max
	// harmless rather than wrong, regardless of whether its author knew this
	// rule existed.
	backendOpts := opts
	backendOpts.Max = 0

	type result struct {
		notifs []Notification
		err    error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(capable))

	for _, b := range capable {
		wg.Add(1)
		go func(backend NotificationSource) {
			defer wg.Done()
			notifs, err := backend.List(backendOpts)
			ch <- result{notifs, err}
		}(b)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []Notification
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.notifs...)
	}

	return mergeNotifications(all, errs, len(capable), opts.Max)
}

// mergeNotifications sorts notifications newest-first, caps the result at
// maxItems, and applies partial-failure logic. maxItems is NotifOpts.Max;
// zero means no cap.
//
// The sort is a total order: UpdatedAt descending, then Identity.Kind, Scope
// and ID ascending. UpdatedAt alone is not enough — GitHub's updated_at is
// second-resolution, so ties are routine, and the input order is itself
// non-deterministic (the fan-out channel is drained in goroutine-completion
// order). Tied rows must be canonicalised, not just stabilised, or a
// stationary cursor selects a different row between polls. sort.SliceStable
// additionally keeps rows identical in all four keys — a duplicate from an
// overlapping page — in their input order.
//
// The maxItems cap is applied here and only here (decision 15 / task 12):
// List zeroes opts.Max before forwarding to each capable backend (decision
// C), so no backend ever receives a nonzero Max to truncate on — each
// backend's List returns its rows uncapped, so without this cap the merged
// length would be whatever each backend's own natural fetch returns,
// unbounded by maxItems, and with N capable backends that easily exceeds a
// single-backend-sized maxItems. A backend that truncated to Max on its own
// (phase 1's original, since-removed per-adapter behaviour) would
// double-apply the cap the moment a second capable backend exists — each
// backend keeping only its own top-Max in its own order before this function
// ever sees the full picture, which can silently drop a row
// that belongs in the true global top-Max while still returning a
// plausible-looking, correctly-sized slice. The cap is applied *after* the
// sort — truncating before it would keep an arbitrary maxItems rather than
// the newest maxItems — and on both the clean and the partial-error return.
// The full slice expression makes the truncation irreversible: what Max
// removed is gone, rather than recoverable via all[:cap(all)] or overwritable
// by a caller's append.
//
// Unlike mergePRs/mergeWorkItems/mergePipelineRuns, the all-failed path
// preserves the error chain with errors.Join rather than flattening it through
// fmt.Errorf("%v", ...): callers recover *github.APIError (and its
// RequiredScopes/GrantedScopes) via errors.As off this exact path to render
// the missing-scope error in-view, and %v would destroy that chain.
func mergeNotifications(all []Notification, errs []error, total, maxItems int) ([]Notification, error) {
	// Zero capable backends is not "all failed" — len(errs) == total is
	// satisfied vacuously at 0 == 0, which would otherwise report "all
	// backends failed" for a config with no notification-capable backend.
	if total == 0 {
		return all, nil
	}
	if len(errs) == total {
		return nil, fmt.Errorf("composite: all notification backends failed: %w", errors.Join(errs...))
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if a.Identity.Kind != b.Identity.Kind {
			return a.Identity.Kind < b.Identity.Kind
		}
		if a.Identity.Scope != b.Identity.Scope {
			return a.Identity.Scope < b.Identity.Scope
		}
		return a.Identity.ID < b.Identity.ID
	})
	if maxItems > 0 && len(all) > maxItems {
		all = all[:maxItems:maxItems]
	}
	if len(errs) > 0 {
		return all, &PartialError{Failed: len(errs), Total: total, Errors: errs}
	}
	return all, nil
}

// notificationMarkRouteErr returns a descriptive error naming the identity
// kind when no capable backend claims it. %q relies on Kind's Stringer so a
// zero/unset Kind renders as "" rather than as fmt's default numeric %v,
// which would read as a blank hole in the message.
func notificationMarkRouteErr(kind Kind) error {
	return fmt.Errorf("composite: no capable notification backend registered for kind %q", kind)
}

// notificationBackendFor routes by Identity.Kind, never by backendFor(scope):
// backendFor is built from configured Scopes(), which is nil/absent for most
// of the inbox, so routing by scope would silently no-op mark-read/mark-done
// on exactly the rows the pane exists to surface.
//
// It filters to capable backends first, then matches Kind() == kind, first
// match wins: asserting capability after matching kind would let a GitHub row
// route to an Azure backend that cannot mark it simply because Azure happened
// to register first.
//
// A zero/unset kind is rejected by an explicit guard rather than left to fall
// through. "No registered backend has a zero Kind" is an invariant about
// callers, not a property of this function, and the package already contains
// a counterexample: CompositeProvider.Kind() returns 0 on an empty backend
// list, and the composite satisfies NotificationSource unconditionally. A
// capable backend reporting Kind() == 0 would otherwise accept a fully
// zero-valued Identity and return a nil error — an optimistic row update whose
// rollback never fires, for a mark that targeted nothing.
func (cp *CompositeProvider) notificationBackendFor(kind Kind) (NotificationSource, error) {
	if kind == 0 {
		return nil, notificationMarkRouteErr(kind)
	}
	for _, b := range cp.backends {
		src, ok := b.(NotificationSource)
		if !ok {
			continue
		}
		if b.Kind() != kind {
			continue
		}
		return src, nil
	}
	return nil, notificationMarkRouteErr(kind)
}

// MarkRead routes to the capable backend whose Kind matches id.Kind and marks
// the notification read there.
func (cp *CompositeProvider) MarkRead(id Identity) error {
	b, err := cp.notificationBackendFor(id.Kind)
	if err != nil {
		return err
	}
	return b.MarkRead(id)
}

// MarkDone routes to the capable backend whose Kind matches id.Kind and marks
// the notification done there.
func (cp *CompositeProvider) MarkDone(id Identity) error {
	b, err := cp.notificationBackendFor(id.Kind)
	if err != nil {
		return err
	}
	return b.MarkDone(id)
}
