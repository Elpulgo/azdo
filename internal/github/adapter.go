package github

import (
	"fmt"

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
type Adapter struct {
	mc *MultiClient
}

// NewAdapter creates an Adapter wrapping the given MultiClient.
// A nil MultiClient is allowed (the Adapter still satisfies the interface; all
// methods that require a live client return a descriptive error).
func NewAdapter(mc *MultiClient) *Adapter {
	return &Adapter{mc: mc}
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
// GitHub has no per-push iteration concept (Decision #5 in the spec). A single
// stable iteration with ID=1 is returned so that the diff/files view can call
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
			Description: "Whole PR (synthetic — GitHub has no per-push iterations; see spec Decision #5)",
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
