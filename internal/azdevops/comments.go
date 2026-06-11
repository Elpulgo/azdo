package azdevops

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// commentsAPIVersion is the api-version for the Work Item Comments endpoints.
// Unlike the rest of the client (which pins 7.1), the comments API is still a
// preview API and requires the -preview suffix.
const commentsAPIVersion = "7.1-preview.4"

// commentsTopLimit caps how many comments are fetched in a single request.
const commentsTopLimit = 200

// WorkItemComment is a single comment from a work item's Discussion section.
type WorkItemComment struct {
	ID          int       `json:"id"`
	Text        string    `json:"text"`
	CreatedBy   Identity  `json:"createdBy"`
	CreatedDate time.Time `json:"createdDate"`
}

// workItemCommentsResponse is the CommentList wrapper returned by the GET endpoint.
type workItemCommentsResponse struct {
	TotalCount int               `json:"totalCount"`
	Count      int               `json:"count"`
	Comments   []WorkItemComment `json:"comments"`
}

// GetWorkItemComments returns up to commentsTopLimit comments for a work item,
// sorted newest first (server-side via order=desc).
func (c *Client) GetWorkItemComments(id int) ([]WorkItemComment, error) {
	path := fmt.Sprintf("/wit/workItems/%d/comments?api-version=%s&order=desc&$top=%d",
		id, commentsAPIVersion, commentsTopLimit)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get work item comments: %w", err)
	}

	var response workItemCommentsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for work item comments: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return response.Comments, nil
}

// AddWorkItemComment posts a new comment to a work item and returns the created
// comment. The text must be non-empty; createdBy is set server-side from the PAT.
func (c *Client) AddWorkItemComment(id int, text string) (*WorkItemComment, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}

	path := fmt.Sprintf("/wit/workItems/%d/comments?api-version=%s", id, commentsAPIVersion)

	payload := fmt.Sprintf(`{"text": %s}`, escapeJSONString(text))
	body, err := c.post(path, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to add work item comment: %w", err)
	}

	var comment WorkItemComment
	if err := json.Unmarshal(body, &comment); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for added work item comment: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return &comment, nil
}
