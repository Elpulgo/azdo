package github

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MultiClient fans out requests across multiple per-repo Clients.
//
// It mirrors the azdevops.MultiClient fan-out shape: goroutine-per-repo,
// buffered channel, sync.WaitGroup, merge+sort by date desc,
// *provider.PartialError on partial failure, plain error when all repos fail.
//
// KEY DIFFERENCE from azdevops.MultiClient: wire→neutral mapping happens INSIDE
// each fan-out goroutine (not in the Adapter), so the per-repo scope/scopeDisplay
// and the shared LabelConvention are available at the mapping boundary. The
// Adapter's list methods receive already-neutral slices.
type MultiClient struct {
	clients      map[string]*Client // keyed by "owner/repo"
	displayNames map[string]string  // scope → human-readable display name (optional)
	conv         LabelConvention    // label convention applied by MapWorkItem
}

// NewMultiClient creates per-repo Clients for each entry in repos.
//
// Each repos entry must be "owner/repo" — any other format returns an error.
// Require at least one repo; an empty slice returns an error.
//
// conv is the label convention used by MapWorkItem to derive ItemType, Priority,
// and Tags from issue labels. If conv is the zero value (both TypePrefix and
// PriorityPrefix are empty), NewMultiClient silently defaults to
// DefaultLabelConvention(). This avoids the footgun where an accidental zero
// value routes all issue labels to Tags. Phase 4 callers that want custom
// prefixes must pass a non-zero LabelConvention.
//
// displayNames is an optional scope → display-name map for UI rendering. Pass
// nil to fall back to the scope string itself.
func NewMultiClient(repos []string, token string, conv LabelConvention, displayNames map[string]string) (*MultiClient, error) {
	if len(repos) == 0 {
		return nil, fmt.Errorf("github: NewMultiClient: at least one repo is required")
	}

	// Zero-value LabelConvention defaults to the conventional prefixes.
	if conv.TypePrefix == "" && conv.PriorityPrefix == "" {
		conv = DefaultLabelConvention()
	}

	clients := make(map[string]*Client, len(repos))
	for _, r := range repos {
		parts := strings.SplitN(r, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("github: NewMultiClient: malformed repo %q: expected \"owner/repo\"", r)
		}
		clients[r] = NewClient(parts[0], parts[1], token)
	}

	return &MultiClient{
		clients:      clients,
		displayNames: displayNames,
		conv:         conv,
	}, nil
}

// ClientFor returns the per-repo Client for the given scope ("owner/repo").
// Returns nil when the scope is not configured. Used by the Adapter for detail
// and mutation methods, and by tests to call SetBaseURL after construction.
func (mc *MultiClient) ClientFor(scope string) *Client {
	return mc.clients[scope]
}

// DisplayNameFor returns the display name for the given scope. Falls back to
// the scope string itself when no display name is configured.
func (mc *MultiClient) DisplayNameFor(scope string) string {
	if mc.displayNames != nil {
		if dn, ok := mc.displayNames[scope]; ok {
			return dn
		}
	}
	return scope
}

// IsMultiProject returns true when more than one repo is configured.
func (mc *MultiClient) IsMultiProject() bool { return len(mc.clients) > 1 }

// Scopes returns the sorted list of configured "owner/repo" scopes.
// Sorting ensures deterministic output for callers that iterate over scopes.
func (mc *MultiClient) Scopes() []string {
	scopes := make([]string, 0, len(mc.clients))
	for s := range mc.clients {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)
	return scopes
}

// --------------------------------------------------------------------------
// Work-item fan-out
// --------------------------------------------------------------------------

// ListWorkItems fetches issues from all repos concurrently, maps each to a
// neutral provider.WorkItem (stamping identity and applying conv), merges and
// sorts by ChangedDate descending.
//
// Returns *provider.PartialError when some (but not all) repos fail; a plain
// error when all repos fail.
func (mc *MultiClient) ListWorkItems(top int, opts provider.ListOpts) ([]provider.WorkItem, error) {
	type result struct {
		items []provider.WorkItem
		err   error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(mc.clients))

	for scope, client := range mc.clients {
		wg.Add(1)
		go func(s string, c *Client) {
			defer wg.Done()
			wire, err := c.ListWorkItems(top, opts)
			if err != nil {
				ch <- result{err: err}
				return
			}
			scopeDisplay := mc.DisplayNameFor(s)
			items := make([]provider.WorkItem, len(wire))
			for i, issue := range wire {
				items[i] = MapWorkItem(issue, mc.conv, s, scopeDisplay)
			}
			ch <- result{items: items}
		}(scope, client)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []provider.WorkItem
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.items...)
	}

	return mergeWorkItems(all, errs, len(mc.clients))
}

// ListMyWorkItems fetches issues assigned to the authenticated user from all
// repos concurrently, maps to neutral, merges and sorts by ChangedDate desc.
func (mc *MultiClient) ListMyWorkItems(top int, opts provider.ListOpts) ([]provider.WorkItem, error) {
	type result struct {
		items []provider.WorkItem
		err   error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(mc.clients))

	for scope, client := range mc.clients {
		wg.Add(1)
		go func(s string, c *Client) {
			defer wg.Done()
			wire, err := c.ListMyWorkItems(top, opts)
			if err != nil {
				ch <- result{err: err}
				return
			}
			scopeDisplay := mc.DisplayNameFor(s)
			items := make([]provider.WorkItem, len(wire))
			for i, issue := range wire {
				items[i] = MapWorkItem(issue, mc.conv, s, scopeDisplay)
			}
			ch <- result{items: items}
		}(scope, client)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []provider.WorkItem
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.items...)
	}

	return mergeWorkItems(all, errs, len(mc.clients))
}

// mergeWorkItems sorts all by ChangedDate desc, applies partial-error logic,
// and returns the merged slice.
func mergeWorkItems(all []provider.WorkItem, errs []error, total int) ([]provider.WorkItem, error) {
	if len(errs) == total {
		return nil, fmt.Errorf("github: all repos failed: %v", errs)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ChangedDate.After(all[j].ChangedDate)
	})
	if len(errs) > 0 {
		return all, &provider.PartialError{Failed: len(errs), Total: total, Errors: errs}
	}
	return all, nil
}

// --------------------------------------------------------------------------
// Pull-request fan-out
// --------------------------------------------------------------------------

// ListPullRequests fetches pull requests from all repos concurrently, maps to
// neutral, merges and sorts by CreationDate descending.
func (mc *MultiClient) ListPullRequests(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	return mc.fanOutPRs(func(c *Client) ([]PullRequest, error) {
		return c.ListPullRequests(top, opts)
	})
}

// ListMyPullRequests fetches PRs authored by the authenticated user from all
// repos concurrently, maps to neutral, merges and sorts by CreationDate desc.
func (mc *MultiClient) ListMyPullRequests(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	return mc.fanOutPRs(func(c *Client) ([]PullRequest, error) {
		return c.ListMyPullRequests(top, opts)
	})
}

// ListPullRequestsAsReviewer fetches PRs where the authenticated user is a
// requested reviewer from all repos concurrently, maps to neutral, merges and
// sorts by CreationDate desc.
func (mc *MultiClient) ListPullRequestsAsReviewer(top int, opts provider.ListOpts) ([]provider.PullRequest, error) {
	return mc.fanOutPRs(func(c *Client) ([]PullRequest, error) {
		return c.ListPullRequestsAsReviewer(top, opts)
	})
}

// fanOutPRs is the shared implementation for the three PR list methods. fetch is
// called once per repo to obtain the wire slice; results are mapped and merged.
// Reviewers are NOT populated here — the list/search payloads don't carry review
// data; the mapper leaves Reviewers empty, consistent with MapPullRequest's
// documented contract.
func (mc *MultiClient) fanOutPRs(fetch func(*Client) ([]PullRequest, error)) ([]provider.PullRequest, error) {
	type result struct {
		prs []provider.PullRequest
		err error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(mc.clients))

	for scope, client := range mc.clients {
		wg.Add(1)
		go func(s string, c *Client) {
			defer wg.Done()
			wire, err := fetch(c)
			if err != nil {
				ch <- result{err: err}
				return
			}
			scopeDisplay := mc.DisplayNameFor(s)
			prs := make([]provider.PullRequest, len(wire))
			for i, pr := range wire {
				prs[i] = MapPullRequest(pr, s, scopeDisplay)
			}
			ch <- result{prs: prs}
		}(scope, client)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []provider.PullRequest
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.prs...)
	}

	if len(errs) == len(mc.clients) {
		return nil, fmt.Errorf("github: all repos failed: %v", errs)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreationDate.After(all[j].CreationDate)
	})
	if len(errs) > 0 {
		return all, &provider.PartialError{Failed: len(errs), Total: len(mc.clients), Errors: errs}
	}
	return all, nil
}

// --------------------------------------------------------------------------
// Pipeline fan-out
// --------------------------------------------------------------------------

// ListPipelineRuns fetches Actions workflow runs from all repos concurrently,
// maps to neutral provider.PipelineRun, merges and sorts by QueueTime desc.
func (mc *MultiClient) ListPipelineRuns(top int, opts provider.ListOpts) ([]provider.PipelineRun, error) {
	type result struct {
		runs []provider.PipelineRun
		err  error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(mc.clients))

	for scope, client := range mc.clients {
		wg.Add(1)
		go func(s string, c *Client) {
			defer wg.Done()
			wire, err := c.ListPipelineRuns(top, opts)
			if err != nil {
				ch <- result{err: err}
				return
			}
			scopeDisplay := mc.DisplayNameFor(s)
			runs := make([]provider.PipelineRun, len(wire))
			for i, run := range wire {
				runs[i] = MapPipelineRun(run, s, scopeDisplay)
			}
			ch <- result{runs: runs}
		}(scope, client)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []provider.PipelineRun
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.runs...)
	}

	if len(errs) == len(mc.clients) {
		return nil, fmt.Errorf("github: all repos failed: %v", errs)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].QueueTime.After(all[j].QueueTime)
	})
	if len(errs) > 0 {
		return all, &provider.PartialError{Failed: len(errs), Total: len(mc.clients), Errors: errs}
	}
	return all, nil
}
