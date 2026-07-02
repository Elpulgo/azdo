package provider

import (
	"fmt"
	"sort"
	"sync"
)

// CompositeProvider fans out list calls across multiple Provider backends and
// routes detail/mutation/URL calls to the backend that owns the given scope.
//
// # Construction
//
// Use NewCompositeProvider. Backends are stored in registration order; the
// scope→backend routing index is built once at construction time from each
// backend's Scopes() output.
//
// # Scope collision (Decision D3)
//
// When two backends claim the same scope string, the first-registered backend
// wins for all routed calls. "First" means first backend in registration order,
// not first scope string alphabetically.
//
// # Single-backend transparency (Decision D1)
//
// A single-backend CompositeProvider is transparent: list methods fan out to
// that one backend, collect its results, run the sort (which is idempotent over
// already-sorted data), and return the same slice.
//
// # Kind (Decision D4)
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
// first-registered backend wins (see D3 above).
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
// first backend's Kind when multiple backends are present (Decision D4).
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
// included (first-registered wins, per D3).
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
