package azdevops

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Adapter wraps a MultiClient and satisfies provider.Provider.
// It maps azdevops wire types to neutral provider types at the boundary,
// stamping Identity (Kind, Scope, ScopeDisplay, ID) on every returned entity.
// Metrics methods (MetricsWorkItems, WorkItemUpdates, GetOrg) are not part of
// the interface — they remain on the concrete *MultiClient (Decision 5).
type Adapter struct {
	mc *MultiClient
}

// NewAdapter creates a new Adapter wrapping the given MultiClient.
// A nil MultiClient is allowed (adapter still satisfies the interface; all
// methods that require a live client will return an error or zero value).
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

// --- Pull-request surface ---

// ListPullRequests returns up to top active pull requests across all projects,
// mapping wire types to neutral types with identity stamped per project.
func (a *Adapter) ListPullRequests(top int) ([]provider.PullRequest, error) {
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
func (a *Adapter) ListMyPullRequests(top int) ([]provider.PullRequest, error) {
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
func (a *Adapter) ListPullRequestsAsReviewer(top int) ([]provider.PullRequest, error) {
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
// mapped to neutral types.
func (a *Adapter) ListWorkItems(top int) ([]provider.WorkItem, error) {
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
// user, mapped to neutral types.
func (a *Adapter) ListMyWorkItems(top int) ([]provider.WorkItem, error) {
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
// mapped to neutral types.
func (a *Adapter) ListPipelineRuns(top int) ([]provider.PipelineRun, error) {
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
// Returns "" when the client is nil or scope does not match a known project.
func (a *Adapter) PRURL(scope, repositoryID string, prID int) string {
	if a.mc == nil {
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
// does not match a known project, or threadID is zero.
func (a *Adapter) PRThreadWebURL(scope, repositoryID string, prID int, threadID int) string {
	if a.mc == nil {
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
