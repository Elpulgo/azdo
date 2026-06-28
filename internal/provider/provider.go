package provider

// Provider is the backend-neutral interface that every view depends on.
// It covers the PR, work-item, pipeline, and log surfaces that the views
// call today. Metrics methods are intentionally excluded (Decision 5 —
// they stay on a concrete *azdevops.MultiClient).
//
// Web URLs are exposed as per-entity methods to avoid type-unsafe any
// parameters (Decision 6). Each method accepts the minimum identifying
// information needed to construct the URL for that entity type.
type Provider interface {
	// Kind returns the backend kind (e.g. KindAzure) so callers can
	// branch on origin when needed without type-asserting the concrete type.
	Kind() Kind

	// --- Pull-request surface ---

	// ListPullRequests returns up to top active pull requests across all
	// projects the provider is configured for.
	ListPullRequests(top int) ([]PullRequest, error)

	// ListMyPullRequests returns up to top pull requests created by the
	// authenticated user.
	ListMyPullRequests(top int) ([]PullRequest, error)

	// ListPullRequestsAsReviewer returns up to top pull requests where the
	// authenticated user is listed as a reviewer.
	ListPullRequestsAsReviewer(top int) ([]PullRequest, error)

	// GetPRThreads returns the comment threads for the given pull request.
	GetPRThreads(repositoryID string, pullRequestID int) ([]Thread, error)

	// GetPRIterations returns all iterations (pushes) for the given pull request.
	GetPRIterations(repositoryID string, pullRequestID int) ([]Iteration, error)

	// GetPRIterationChanges returns the files changed between the given
	// iteration and its base.
	GetPRIterationChanges(repositoryID string, pullRequestID int, iterationID int) ([]IterationChange, error)

	// VotePullRequest submits a reviewer vote on the given pull request.
	// vote should be one of the Vote* constants from the azdevops package
	// (10 approve, 5 approve-with-suggestions, 0 reset, -5 wait, -10 reject).
	// This surface leaks a wire-level convention; it will be sealed in Phase 1.
	VotePullRequest(repositoryID string, pullRequestID int, vote int) error

	// GetFileContent returns the raw file content at the given branch ref.
	GetFileContent(repositoryID string, filePath string, branchName string) (string, error)

	// AddPRCodeComment creates a new inline code comment on the given line.
	AddPRCodeComment(repositoryID string, pullRequestID int, filePath string, line int, content string) (*Thread, error)

	// AddPRComment creates a new general (non-file) comment thread on the PR.
	AddPRComment(repositoryID string, pullRequestID int, content string) (*Thread, error)

	// ReplyToThread posts a reply to an existing comment thread.
	ReplyToThread(repositoryID string, pullRequestID int, threadID int, content string) (*Comment, error)

	// UpdateThreadStatus sets the status of a comment thread (e.g. "fixed").
	UpdateThreadStatus(repositoryID string, pullRequestID int, threadID int, status string) error

	// --- Work-item surface ---

	// ListWorkItems returns up to top work items across all configured projects.
	ListWorkItems(top int) ([]WorkItem, error)

	// ListMyWorkItems returns up to top work items assigned to the authenticated user.
	ListMyWorkItems(top int) ([]WorkItem, error)

	// GetWorkItemTypeStates returns the valid states for the given work item type.
	GetWorkItemTypeStates(workItemType string) ([]WorkItemTypeState, error)

	// UpdateWorkItemState transitions the given work item to the specified state.
	UpdateWorkItemState(id int, state string) error

	// GetWorkItemComments returns the discussion comments for the given work item,
	// ordered newest first.
	GetWorkItemComments(id int) ([]WorkItemComment, error)

	// AddWorkItemComment posts a new comment on the given work item.
	AddWorkItemComment(id int, text string) (*WorkItemComment, error)

	// --- Pipeline surface ---

	// ListPipelineRuns returns up to top recent pipeline/build runs.
	ListPipelineRuns(top int) ([]PipelineRun, error)

	// GetBuildTimeline returns the timeline (stages, jobs, tasks) for the given build.
	GetBuildTimeline(buildID int) (*Timeline, error)

	// GetBuildLogContent returns the raw log text for the given log within a build.
	GetBuildLogContent(buildID, logID int) (string, error)

	// --- Web URL helpers (Decision 6) ---
	// Per-entity methods are used instead of a single WebURL(ref any) to keep
	// the interface type-safe. Each method returns an empty string when the
	// URL cannot be constructed (e.g. missing org or project).

	// WorkItemURL returns the browser URL for the given work item ID.
	WorkItemURL(id int) string

	// PRURL returns the browser URL for the given pull request in the given
	// repository.
	PRURL(repositoryID string, prID int) string

	// PipelineURL returns the browser URL for the given pipeline build.
	PipelineURL(id int) string

	// --- Multi-project helpers ---

	// IsMultiProject returns true when the provider spans more than one project,
	// which the list views use to decide whether to show a Project column.
	IsMultiProject() bool
}
