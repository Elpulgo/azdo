package azdevops

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PullRequest represents a pull request in Azure DevOps
type PullRequest struct {
	ID            int          `json:"pullRequestId"`
	Title         string       `json:"title"`
	Description   string       `json:"description"`
	Status        string       `json:"status"` // "active", "completed", "abandoned"
	CreationDate  time.Time    `json:"creationDate"`
	SourceRefName string       `json:"sourceRefName"` // e.g., "refs/heads/feature/my-feature"
	TargetRefName string       `json:"targetRefName"` // e.g., "refs/heads/main"
	IsDraft       bool         `json:"isDraft"`
	CreatedBy     Identity     `json:"createdBy"`
	Repository    Repository   `json:"repository"`
	Reviewers     []Reviewer   `json:"reviewers"`
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
		return nil, fmt.Errorf("failed to unmarshal pull requests response: %w", err)
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
	default:
		return t.Status
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
		return nil, fmt.Errorf("failed to unmarshal threads response: %w", err)
	}

	return response.Value, nil
}

// Vote values for pull request reviews
const (
	VoteApprove                = 10 // Approved
	VoteApproveWithSuggestions = 5  // Approved with suggestions
	VoteNoVote                 = 0  // No vote
	VoteWaitForAuthor          = -5 // Waiting for author
	VoteReject                 = -10 // Rejected
)

// VotePullRequest sets the current user's vote on a pull request
// repositoryID: the ID of the repository
// pullRequestID: the ID of the pull request
// vote: the vote value (use VoteApprove, VoteReject, etc. constants)
func (c *Client) VotePullRequest(repositoryID string, pullRequestID int, vote int) error {
	path := fmt.Sprintf("/git/repositories/%s/pullRequests/%d/reviewers/me?api-version=7.1", repositoryID, pullRequestID)

	payload := fmt.Sprintf(`{"vote": %d}`, vote)
	_, err := c.put(path, strings.NewReader(payload))
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
		return nil, fmt.Errorf("failed to unmarshal thread response: %w", err)
	}

	return &thread, nil
}

// escapeJSONString escapes a string for use in JSON
func escapeJSONString(s string) string {
	// Use json.Marshal to properly escape the string
	b, _ := json.Marshal(s)
	return string(b)
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
	firstComment := thread.Comments[0]
	content := strings.TrimSpace(firstComment.Content)
	// Filter threads where first comment starts with "Microsoft.VisualStudio"
	return strings.HasPrefix(content, "Microsoft.VisualStudio")
}
