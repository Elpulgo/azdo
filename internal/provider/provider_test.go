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
func (s stubProvider) GetPRThreads(scope, repositoryID string, pullRequestID int) ([]provider.Thread, error) {
	return nil, nil
}
func (s stubProvider) GetPRIterations(scope, repositoryID string, pullRequestID int) ([]provider.Iteration, error) {
	return nil, nil
}
func (s stubProvider) GetPRIterationChanges(scope, repositoryID string, pullRequestID int, iterationID int) ([]provider.IterationChange, error) {
	return nil, nil
}
func (s stubProvider) VotePullRequest(scope, repositoryID string, pullRequestID int, vote int) error {
	return nil
}
func (s stubProvider) GetFileContent(scope, repositoryID string, filePath string, branchName string) (string, error) {
	return "", nil
}
func (s stubProvider) AddPRCodeComment(scope, repositoryID string, pullRequestID int, filePath string, line int, content string) (*provider.Thread, error) {
	return nil, nil
}
func (s stubProvider) AddPRComment(scope, repositoryID string, pullRequestID int, content string) (*provider.Thread, error) {
	return nil, nil
}
func (s stubProvider) ReplyToThread(scope, repositoryID string, pullRequestID int, threadID int, content string) (*provider.Comment, error) {
	return nil, nil
}
func (s stubProvider) UpdateThreadStatus(scope, repositoryID string, pullRequestID int, threadID int, status string) error {
	return nil
}

// --- Work-item surface ---

func (s stubProvider) ListWorkItems(top int) ([]provider.WorkItem, error) { return nil, nil }
func (s stubProvider) ListMyWorkItems(top int) ([]provider.WorkItem, error) { return nil, nil }
func (s stubProvider) GetWorkItemTypeStates(scope, workItemType string) ([]provider.WorkItemTypeState, error) {
	return nil, nil
}
func (s stubProvider) UpdateWorkItemState(scope string, id int, state string) error { return nil }
func (s stubProvider) GetWorkItemComments(scope string, id int) ([]provider.WorkItemComment, error) {
	return nil, nil
}
func (s stubProvider) AddWorkItemComment(scope string, id int, text string) (*provider.WorkItemComment, error) {
	return nil, nil
}

// --- Pipeline surface ---

func (s stubProvider) ListPipelineRuns(top int) ([]provider.PipelineRun, error) { return nil, nil }
func (s stubProvider) GetBuildTimeline(scope string, buildID int) (*provider.Timeline, error) {
	return nil, nil
}
func (s stubProvider) GetBuildLogContent(scope string, buildID, logID int) (string, error) {
	return "", nil
}

// --- Web URL helpers ---

func (s stubProvider) WorkItemURL(scope string, id int) string                                { return "" }
func (s stubProvider) PRURL(scope, repositoryID string, prID int) string                      { return "" }
func (s stubProvider) PRThreadWebURL(scope, repositoryID string, prID int, threadID int) string { return "" }
func (s stubProvider) PipelineURL(scope string, id int) string                                { return "" }

// --- Multi-project ---

func (s stubProvider) IsMultiProject() bool { return false }

// compile-time assertion: stubProvider must satisfy provider.Provider.
var _ provider.Provider = stubProvider{}

// TestProviderInterfaceExists passes trivially once the file compiles.
// The real gate is the compile-time var _ assertion above.
func TestProviderInterfaceExists(t *testing.T) {
	t.Log("provider.Provider interface compiles and stubProvider satisfies it")
}
