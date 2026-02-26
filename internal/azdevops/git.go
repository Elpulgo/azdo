package azdevops

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PullRequest represents a pull request in Azure DevOps
type PullRequest struct {
	ID            int        `json:"pullRequestId"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Status        string     `json:"status"` // "active", "completed", "abandoned"
	CreationDate  time.Time  `json:"creationDate"`
	SourceRefName string     `json:"sourceRefName"` // e.g., "refs/heads/feature/my-feature"
	TargetRefName string     `json:"targetRefName"` // e.g., "refs/heads/main"
	IsDraft       bool       `json:"isDraft"`
	CreatedBy     Identity   `json:"createdBy"`
	Repository    Repository `json:"repository"`
	Reviewers     []Reviewer `json:"reviewers"`
	ProjectName        string     `json:"-"` // Set by MultiClient, not from API
	ProjectDisplayName string     `json:"-"` // Set by MultiClient, display name for UI
}

// Identity represents a user identity in Azure DevOps
type Identity struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"` // typically email
}

// Repository represents a Git repository in Azure DevOps
type Repository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Reviewer represents a reviewer on a pull request
type Reviewer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Vote        int    `json:"vote"` // 10: approved, 5: approved with suggestions, 0: no vote, -5: waiting, -10: rejected
}

// PullRequestsResponse represents the API response for listing pull requests
type PullRequestsResponse struct {
	Count int           `json:"count"`
	Value []PullRequest `json:"value"`
}

// SourceBranchShortName returns the short branch name without the refs/heads/ prefix
func (pr *PullRequest) SourceBranchShortName() string {
	if pr.SourceRefName == "" {
		return ""
	}

	if strings.HasPrefix(pr.SourceRefName, "refs/heads/") {
		return strings.TrimPrefix(pr.SourceRefName, "refs/heads/")
	}

	return pr.SourceRefName
}

// TargetBranchShortName returns the short branch name without the refs/heads/ prefix
func (pr *PullRequest) TargetBranchShortName() string {
	if pr.TargetRefName == "" {
		return ""
	}

	if strings.HasPrefix(pr.TargetRefName, "refs/heads/") {
		return strings.TrimPrefix(pr.TargetRefName, "refs/heads/")
	}

	return pr.TargetRefName
}

// VoteDescription returns a human-readable description of the reviewer's vote
func (r *Reviewer) VoteDescription() string {
	switch r.Vote {
	case 10:
		return "Approved"
	case 5:
		return "Approved with suggestions"
	case 0:
		return "No vote"
	case -5:
		return "Waiting for author"
	case -10:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// ListPullRequests retrieves active pull requests across all repositories in the project
// top: maximum number of pull requests to return (typically 25-100)
// Results are ordered by creation date descending (most recent first)
func (c *Client) ListPullRequests(top int) ([]PullRequest, error) {
	path := fmt.Sprintf("/git/pullrequests?api-version=7.1&$top=%d&searchCriteria.status=active", top)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	var response PullRequestsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for pull requests: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return response.Value, nil
}

// Thread represents a comment thread on a pull request
type Thread struct {
	ID              int            `json:"id"`
	PublishedDate   time.Time      `json:"publishedDate"`
	LastUpdatedDate time.Time      `json:"lastUpdatedDate"`
	Status          string         `json:"status"` // "active", "fixed", "wontFix", "closed", "pending"
	ThreadContext   *ThreadContext `json:"threadContext"`
	Comments        []Comment      `json:"comments"`
	IsDeleted       bool           `json:"isDeleted"`
}

// ThreadContext contains location information for code comments
type ThreadContext struct {
	FilePath       string        `json:"filePath"`
	RightFileStart *FilePosition `json:"rightFileStart"`
	RightFileEnd   *FilePosition `json:"rightFileEnd"`
}

// FilePosition represents a position in a file
type FilePosition struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}

// Comment represents a single comment in a thread
type Comment struct {
	ID              int       `json:"id"`
	ParentCommentID int       `json:"parentCommentId"`
	Content         string    `json:"content"`
	PublishedDate   time.Time `json:"publishedDate"`
	LastUpdatedDate time.Time `json:"lastUpdatedDate"`
	CommentType     string    `json:"commentType"` // "text", "system"
	Author          Identity  `json:"author"`
}

// ThreadsResponse represents the API response for listing threads
type ThreadsResponse struct {
	Count int      `json:"count"`
	Value []Thread `json:"value"`
}

// IsCodeComment returns true if this thread is attached to a specific code location
func (t *Thread) IsCodeComment() bool {
	return t.ThreadContext != nil && t.ThreadContext.FilePath != ""
}

// StatusDescription returns a human-readable description of the thread status
func (t *Thread) StatusDescription() string {
	switch t.Status {
	case "active":
		return "Active"
	case "fixed":
		return "Resolved"
	case "wontFix":
		return "Won't fix"
	case "closed":
		return "Closed"
	case "pending":
		return "Pending"
	case "":
		return "Unknown"
	default:
		return "Unknown"
	}
}

// GetPRThreads retrieves comment threads for a pull request
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
func (c *Client) GetPRThreads(repositoryID string, pullRequestID int) ([]Thread, error) {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/threads?api-version=7.1", repositoryID, pullRequestID)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR threads: %w", err)
	}

	var response ThreadsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for PR threads: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return response.Value, nil
}

// Vote values for pull request reviews
const (
	VoteApprove                = 10  // Approved
	VoteApproveWithSuggestions = 5   // Approved with suggestions
	VoteNoVote                 = 0   // No vote
	VoteWaitForAuthor          = -5  // Waiting for author
	VoteReject                 = -10 // Rejected
)

// VotePullRequest sets the current user's vote on a pull request
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// vote: the vote value (use VoteApprove, VoteReject, etc. constants)
func (c *Client) VotePullRequest(repositoryID string, pullRequestID int, vote int) error {
	userID, err := c.GetCurrentUserID()
	if err != nil {
		return fmt.Errorf("failed to get current user ID: %w", err)
	}

	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/reviewers/%s?api-version=7.1", repositoryID, pullRequestID, userID)

	payload := fmt.Sprintf(`{"vote": %d}`, vote)
	_, err = c.put(path, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to vote on PR: %w", err)
	}

	return nil
}

// AddPRComment adds a general comment thread to a pull request
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// comment: the comment text
func (c *Client) AddPRComment(repositoryID string, pullRequestID int, comment string) (*Thread, error) {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/threads?api-version=7.1", repositoryID, pullRequestID)

	// Create a new thread with the comment
	payload := fmt.Sprintf(`{
		"comments": [
			{
				"parentCommentId": 0,
				"content": %s,
				"commentType": "text"
			}
		],
		"status": "active"
	}`, escapeJSONString(comment))

	body, err := c.post(path, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to add PR comment: %w", err)
	}

	var thread Thread
	err = json.Unmarshal(body, &thread)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for thread: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return &thread, nil
}

// escapeJSONString escapes a string for use in JSON
func escapeJSONString(s string) string {
	// Use json.Marshal to properly escape the string
	b, _ := json.Marshal(s)
	return string(b)
}

// Iteration represents a single iteration (push) on a pull request
type Iteration struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

// IterationsResponse represents the API response for listing iterations
type IterationsResponse struct {
	Count int         `json:"count"`
	Value []Iteration `json:"value"`
}

// IterationChange represents a file changed in a PR iteration
type IterationChange struct {
	ChangeID     int        `json:"changeId"`
	Item         ChangeItem `json:"item"`
	ChangeType   string     `json:"changeType"` // "add", "edit", "delete", "rename"
	OriginalPath string     `json:"originalPath,omitempty"`
}

// ChangeItem represents the item details in an iteration change
type ChangeItem struct {
	ObjectID      string `json:"objectId"`
	Path          string `json:"path"`
	GitObjectType string `json:"gitObjectType,omitempty"` // "blob" for files, "tree" for folders
}

// IterationChangesResponse represents the API response for iteration changes
type IterationChangesResponse struct {
	ChangeEntries []IterationChange `json:"changeEntries"`
}

// GetPRIterations retrieves iterations for a pull request
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
func (c *Client) GetPRIterations(repositoryID string, pullRequestID int) ([]Iteration, error) {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/iterations?api-version=7.1", repositoryID, pullRequestID)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR iterations: %w", err)
	}

	var response IterationsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PR iterations response: %w", err)
	}

	return response.Value, nil
}

// GetPRIterationChanges retrieves files changed in a specific PR iteration
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// iterationID: the iteration to get changes for
func (c *Client) GetPRIterationChanges(repositoryID string, pullRequestID int, iterationID int) ([]IterationChange, error) {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/iterations/%d/changes?api-version=7.1&$compareTo=0",
		repositoryID, pullRequestID, iterationID)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR iteration changes: %w", err)
	}

	var response IterationChangesResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PR iteration changes response: %w", err)
	}

	return response.ChangeEntries, nil
}

// GetFileContent retrieves raw file content at a specific branch version
// repositoryID: the ID of the repository
// filePath: the path of the file in the repository
// branchName: the short branch name (e.g., "main", not "refs/heads/main")
func (c *Client) GetFileContent(repositoryID string, filePath string, branchName string) (string, error) {
	path := fmt.Sprintf("/git/repositories/%s/items?path=%s&versionType=branch&version=%s&api-version=7.1",
		repositoryID, filePath, branchName)

	// Use doRequest directly to set Accept header for raw text
	url := c.baseURL + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Accept", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", formatHTTPError(resp.StatusCode, respBody)
	}

	return string(respBody), nil
}

// ReplyToThread adds a reply comment to an existing thread
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// threadID: the ID of the thread to reply to
// content: the reply text
func (c *Client) ReplyToThread(repositoryID string, pullRequestID int, threadID int, content string) (*Comment, error) {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/threads/%d/comments?api-version=7.1",
		repositoryID, pullRequestID, threadID)

	payload := fmt.Sprintf(`{"content": %s, "parentCommentId": 1, "commentType": "text"}`,
		escapeJSONString(content))

	body, err := c.post(path, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to reply to thread: %w", err)
	}

	var comment Comment
	err = json.Unmarshal(body, &comment)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reply response: %w", err)
	}

	return &comment, nil
}

// UpdateThreadStatus updates the status of a thread (e.g., resolve it)
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// threadID: the ID of the thread to update
// status: the new status ("active", "fixed", "wontFix", "closed", "pending")
func (c *Client) UpdateThreadStatus(repositoryID string, pullRequestID int, threadID int, status string) error {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/threads/%d?api-version=7.1",
		repositoryID, pullRequestID, threadID)

	payload := fmt.Sprintf(`{"status": %s}`, escapeJSONString(status))

	_, err := c.patch(path, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to update thread status: %w", err)
	}

	return nil
}

// AddPRCodeComment creates a new comment thread attached to a specific file and line
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// filePath: the path of the file to comment on
// line: the line number in the new file to attach the comment to
// content: the comment text
func (c *Client) AddPRCodeComment(repositoryID string, pullRequestID int, filePath string, line int, content string) (*Thread, error) {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/threads?api-version=7.1",
		repositoryID, pullRequestID)

	payload := fmt.Sprintf(`{
		"comments": [
			{
				"parentCommentId": 0,
				"content": %s,
				"commentType": "text"
			}
		],
		"status": "active",
		"threadContext": {
			"filePath": %s,
			"rightFileStart": {"line": %d, "offset": 1},
			"rightFileEnd": {"line": %d, "offset": 1}
		}
	}`, escapeJSONString(content), escapeJSONString(filePath), line, line)

	body, err := c.post(path, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to add code comment: %w", err)
	}

	var thread Thread
	err = json.Unmarshal(body, &thread)
	if err != nil {
		return nil, fmt.Errorf("failed to parse code comment response: %w", err)
	}

	return &thread, nil
}

// FilterSystemThreads filters out threads that are system-generated comments
// (e.g., threads whose first comment starts with "Microsoft.VisualStudio")
func FilterSystemThreads(threads []Thread) []Thread {
	filtered := make([]Thread, 0, len(threads))
	for _, thread := range threads {
		if !isSystemThread(thread) {
			filtered = append(filtered, thread)
		}
	}
	return filtered
}

// isSystemThread returns true if the thread is a system-generated thread
func isSystemThread(thread Thread) bool {
	if len(thread.Comments) == 0 {
		return false
	}
	// Check ALL comments in the thread
	for _, comment := range thread.Comments {
		// Filter by author name (e.g., "Microsoft.VisualStudio.Services.TFS")
		if strings.HasPrefix(comment.Author.DisplayName, "Microsoft.VisualStudio") {
			return true
		}
		// Also filter by content starting with "Microsoft.VisualStudio"
		content := strings.TrimSpace(comment.Content)
		if strings.HasPrefix(content, "Microsoft.VisualStudio") {
			return true
		}
		// Filter "Policy status has been updated" comments
		if strings.Contains(content, "Policy status has been updated") {
			return true
		}
		// Filter "voted" comments (e.g., "John Doe voted -5", "Jane voted 10")
		if isVotedComment(content) {
			return true
		}
	}
	return false
}

// isVotedComment checks if the content is a vote notification comment
// e.g., "John Doe voted -5", "Jane Smith voted 10", "Bob voted 0"
func isVotedComment(content string) bool {
	// Look for pattern: "voted" followed by optional space and a number (possibly negative)
	idx := strings.Index(content, "voted")
	if idx == -1 {
		return false
	}
	// Get the text after "voted"
	after := strings.TrimSpace(content[idx+5:])
	if len(after) == 0 {
		return false
	}
	// Check if it starts with a number (possibly negative)
	if after[0] == '-' && len(after) > 1 {
		after = after[1:]
	}
	// Check if the remaining starts with a digit
	if len(after) > 0 && after[0] >= '0' && after[0] <= '9' {
		return true
	}
	return false
}
