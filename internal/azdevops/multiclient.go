package azdevops

import (
	"fmt"
	"sort"
	"sync"
)

// MultiClient wraps multiple project-scoped clients for concurrent fetching.
type MultiClient struct {
	org     string
	pat     string
	clients map[string]*Client // project name → client
}

// NewMultiClient creates clients for each project.
func NewMultiClient(org string, projects []string, pat string) (*MultiClient, error) {
	if len(projects) == 0 {
		return nil, fmt.Errorf("at least one project is required")
	}

	clients := make(map[string]*Client, len(projects))
	for _, project := range projects {
		c, err := NewClient(org, project, pat)
		if err != nil {
			return nil, fmt.Errorf("failed to create client for project %q: %w", project, err)
		}
		clients[project] = c
	}
	return &MultiClient{org: org, pat: pat, clients: clients}, nil
}

// ClientFor returns the project-specific client (for detail views).
func (mc *MultiClient) ClientFor(project string) *Client {
	return mc.clients[project]
}

// GetOrg returns the organization name.
func (mc *MultiClient) GetOrg() string { return mc.org }

// IsMultiProject returns true if more than one project is configured.
func (mc *MultiClient) IsMultiProject() bool { return len(mc.clients) > 1 }

// Projects returns the list of project names.
func (mc *MultiClient) Projects() []string {
	projects := make([]string, 0, len(mc.clients))
	for p := range mc.clients {
		projects = append(projects, p)
	}
	return projects
}

// ListPipelineRuns fetches pipeline runs from all projects concurrently,
// merges and sorts by QueueTime descending.
func (mc *MultiClient) ListPipelineRuns(top int) ([]PipelineRun, error) {
	type result struct {
		runs []PipelineRun
		err  error
	}

	ch := make(chan result, len(mc.clients))
	for _, client := range mc.clients {
		go func(c *Client) {
			runs, err := c.ListPipelineRuns(top)
			ch <- result{runs, err}
		}(client)
	}

	var allRuns []PipelineRun
	var errs []error
	for range mc.clients {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		allRuns = append(allRuns, r.runs...)
	}

	if len(errs) == len(mc.clients) {
		return nil, fmt.Errorf("all projects failed: %v", errs)
	}

	sort.Slice(allRuns, func(i, j int) bool {
		return allRuns[i].QueueTime.After(allRuns[j].QueueTime)
	})

	return allRuns, nil
}

// ListPullRequests fetches PRs from all projects concurrently,
// tags each with ProjectName, merges and sorts by CreationDate descending.
func (mc *MultiClient) ListPullRequests(top int) ([]PullRequest, error) {
	type result struct {
		project string
		prs     []PullRequest
		err     error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(mc.clients))

	for project, client := range mc.clients {
		wg.Add(1)
		go func(p string, c *Client) {
			defer wg.Done()
			prs, err := c.ListPullRequests(top)
			ch <- result{p, prs, err}
		}(project, client)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var allPRs []PullRequest
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		for i := range r.prs {
			r.prs[i].ProjectName = r.project
		}
		allPRs = append(allPRs, r.prs...)
	}

	if len(errs) == len(mc.clients) {
		return nil, fmt.Errorf("all projects failed: %v", errs)
	}

	sort.Slice(allPRs, func(i, j int) bool {
		return allPRs[i].CreationDate.After(allPRs[j].CreationDate)
	})

	return allPRs, nil
}

// ListWorkItems fetches work items from all projects concurrently,
// tags each with ProjectName, merges and sorts by ChangedDate descending.
func (mc *MultiClient) ListWorkItems(top int) ([]WorkItem, error) {
	type result struct {
		project string
		items   []WorkItem
		err     error
	}

	var wg sync.WaitGroup
	ch := make(chan result, len(mc.clients))

	for project, client := range mc.clients {
		wg.Add(1)
		go func(p string, c *Client) {
			defer wg.Done()
			items, err := c.ListWorkItems(top)
			ch <- result{p, items, err}
		}(project, client)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var allItems []WorkItem
	var errs []error
	for r := range ch {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		for i := range r.items {
			r.items[i].ProjectName = r.project
		}
		allItems = append(allItems, r.items...)
	}

	if len(errs) == len(mc.clients) {
		return nil, fmt.Errorf("all projects failed: %v", errs)
	}

	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].Fields.ChangedDate.After(allItems[j].Fields.ChangedDate)
	})

	return allItems, nil
}
