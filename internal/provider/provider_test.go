package provider_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// stubProvider is a compile-time conformance probe that claims to implement
// provider.Provider. Every method returns the zero value of its return type.
// If provider.Provider does not yet exist, or any method signature changes,
// this file fails to compile — that is the intentional TDD gate.
type stubProvider struct{}

func (s stubProvider) Kind() provider.Kind { return 0 }

// --- Pull-request surface ---

func (s stubProvider) ListPullRequests(top int) ([]provider.PullRequest, error) { return nil, nil }
func (s stubProvider) ListMyPullRequests(top int) ([]provider.PullRequest, error) { return nil, nil }
func (s stubProvider) ListPullRequestsAsReviewer(top int) ([]provider.PullRequest, error) {
	return nil, nil
}
func (s stubProvider) GetPRThreads(repositoryID string, pullRequestID int) ([]provider.Thread, error) {
	return nil, nil
}
func (s stubProvider) GetPRIterations(repositoryID string, pullRequestID int) ([]provider.Iteration, error) {
	return nil, nil
}
func (s stubProvider) GetPRIterationChanges(repositoryID string, pullRequestID int, iterationID int) ([]provider.IterationChange, error) {
	return nil, nil
}
func (s stubProvider) VotePullRequest(repositoryID string, pullRequestID int, vote int) error {
	return nil
}
func (s stubProvider) GetFileContent(repositoryID string, filePath string, branchName string) (string, error) {
	return "", nil
}
func (s stubProvider) AddPRCodeComment(repositoryID string, pullRequestID int, filePath string, line int, content string) (*provider.Thread, error) {
	return nil, nil
}
func (s stubProvider) AddPRComment(repositoryID string, pullRequestID int, content string) (*provider.Thread, error) {
	return nil, nil
}
func (s stubProvider) ReplyToThread(repositoryID string, pullRequestID int, threadID int, content string) (*provider.Comment, error) {
	return nil, nil
}
func (s stubProvider) UpdateThreadStatus(repositoryID string, pullRequestID int, threadID int, status string) error {
	return nil
}

// --- Work-item surface ---

func (s stubProvider) ListWorkItems(top int) ([]provider.WorkItem, error) { return nil, nil }
func (s stubProvider) ListMyWorkItems(top int) ([]provider.WorkItem, error) { return nil, nil }
func (s stubProvider) GetWorkItemTypeStates(workItemType string) ([]provider.WorkItemTypeState, error) {
	return nil, nil
}
func (s stubProvider) UpdateWorkItemState(id int, state string) error { return nil }
func (s stubProvider) GetWorkItemComments(id int) ([]provider.WorkItemComment, error) {
	return nil, nil
}
func (s stubProvider) AddWorkItemComment(id int, text string) (*provider.WorkItemComment, error) {
	return nil, nil
}

// --- Pipeline surface ---

func (s stubProvider) ListPipelineRuns(top int) ([]provider.PipelineRun, error) { return nil, nil }
func (s stubProvider) GetBuildTimeline(buildID int) (*provider.Timeline, error) { return nil, nil }
func (s stubProvider) GetBuildLogContent(buildID, logID int) (string, error)    { return "", nil }

// --- Web URL helpers ---

func (s stubProvider) WorkItemURL(id int) string      { return "" }
func (s stubProvider) PRURL(repositoryID string, prID int) string { return "" }
func (s stubProvider) PipelineURL(id int) string      { return "" }

// --- Multi-project ---

func (s stubProvider) IsMultiProject() bool { return false }

// compile-time assertion: stubProvider must satisfy provider.Provider.
var _ provider.Provider = stubProvider{}

// TestProviderInterfaceExists passes trivially once the file compiles.
// The real gate is the compile-time var _ assertion above.
func TestProviderInterfaceExists(t *testing.T) {
	t.Log("provider.Provider interface compiles and stubProvider satisfies it")
}
