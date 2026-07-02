package github

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
)

// issuePerPageCap is the maximum page size accepted by GET /repos/.../issues
// and GET /search/issues. The GitHub REST API hard-caps per_page at 100.
// Callers that need more than 100 items must paginate via the Link header —
// pagination is noted as an Unknown in the Phase 3 spec and is not implemented
// here; requests with top > 100 are silently capped at 100.
const issuePerPageCap = 100

// issueSearchResponse is the envelope returned by GET /search/issues.
// The Items slice contains the actual Issue objects; TotalCount is informational.
type issueSearchResponse struct {
	TotalCount int     `json:"total_count"`
	Items      []Issue `json:"items"`
}

// mapStateParam translates a slice of neutral StateCategory values into a
// GitHub REST state query parameter value ("open", "closed", or "all").
//
// Mapping rules:
//   - All categories are open-like (New, Active, Resolved, ReadyForTest, Unknown)  → "open"
//   - All categories are closed-like (ClosedDone, Removed)                         → "closed"
//   - Empty slice or a mix of open and closed                                       → "all"
func mapStateParam(states []provider.StateCategory) string {
	if len(states) == 0 {
		return "all"
	}
	openSet := map[provider.StateCategory]bool{
		provider.StateCategoryNew:          true,
		provider.StateCategoryActive:       true,
		provider.StateCategoryResolved:     true,
		provider.StateCategoryReadyForTest: true,
		provider.StateCategoryUnknown:      true,
	}
	closedSet := map[provider.StateCategory]bool{
		provider.StateCategoryClosedDone: true,
		provider.StateCategoryRemoved:    true,
	}

	allOpen, allClosed := true, true
	for _, s := range states {
		if !openSet[s] {
			allOpen = false
		}
		if !closedSet[s] {
			allClosed = false
		}
	}
	switch {
	case allOpen:
		return "open"
	case allClosed:
		return "closed"
	default:
		return "all"
	}
}

// ListWorkItems returns up to top real issues for the repository, sorted by
// most recently updated. Pull requests are filtered out (see Issue.PullRequest).
//
// top is capped at issuePerPageCap (100); pagination is not yet implemented
// (noted as an Unknown in the Phase 3 spec).
//
// opts.States is mapped to the GitHub state query parameter:
//   - All-open categories  → state=open
//   - All-closed categories → state=closed
//   - Empty or mixed       → state=all
func (c *Client) ListWorkItems(top int, opts provider.ListOpts) ([]Issue, error) {
	if top <= 0 {
		top = issuePerPageCap
	}
	if top > issuePerPageCap {
		top = issuePerPageCap
	}

	state := mapStateParam(opts.States)
	path := fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=%d&sort=updated&direction=desc",
		c.owner, c.repo, state, top)

	var raw []Issue
	if err := c.getJSON(path, &raw); err != nil {
		return nil, fmt.Errorf("github: list work items: %w", err)
	}

	// Filter out pull requests. GitHub's /issues endpoint returns every PR as an
	// "issue" too; PullRequest is non-nil for those objects.
	result := make([]Issue, 0, len(raw))
	for _, issue := range raw {
		if issue.PullRequest == nil {
			result = append(result, issue)
		}
	}
	return result, nil
}

// ListMyWorkItems returns up to top issues assigned to the authenticated user
// in this repository, using the /search/issues endpoint.
//
// The query includes is:issue to exclude pull requests (the search endpoint
// also returns PRs without this qualifier) and assignee:@me, which GitHub
// resolves to the authenticated user server-side.
//
// Note: if @me proves unreliable with older GitHub Enterprise versions, the
// fallback is GET /user → login and substitute the actual login for @me.
// That fallback is not implemented here; use the token user's actual login
// as a workaround if needed.
//
// top is capped at issuePerPageCap (100). /search/issues has a rate limit of
// 30 requests/minute (authenticated), noted as an Unknown in the Phase 3 spec.
func (c *Client) ListMyWorkItems(top int, opts provider.ListOpts) ([]Issue, error) {
	if top <= 0 {
		top = issuePerPageCap
	}
	if top > issuePerPageCap {
		top = issuePerPageCap
	}

	// Build the q parameter. is:issue excludes PRs; assignee:@me scopes to the
	// authenticated user; an optional state: qualifier narrows by open/closed.
	q := fmt.Sprintf("repo:%s/%s is:issue assignee:@me", c.owner, c.repo)

	state := mapStateParam(opts.States)
	if state == "open" || state == "closed" {
		q += " state:" + state
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("per_page", fmt.Sprintf("%d", top))

	path := "/search/issues?" + params.Encode()

	var envelope issueSearchResponse
	if err := c.getJSON(path, &envelope); err != nil {
		return nil, fmt.Errorf("github: list my work items: %w", err)
	}
	return envelope.Items, nil
}

// GetWorkItemTypeStates returns the two static states that GitHub issues support:
// open and closed. No HTTP call is made — the set is fixed by the GitHub REST API.
//
// Category strings use the Azure DevOps vocabulary that the statepicker component
// already understands ("InProgress" and "Completed"). This ensures the shared
// stateIcon function renders the correct glyphs (◐ and ✓) without changes to
// the UI layer.
//
// workItemType is accepted for interface symmetry but is ignored; GitHub issues
// have no sub-types with distinct state machines.
func (c *Client) GetWorkItemTypeStates(_ string) ([]provider.WorkItemTypeState, error) {
	return []provider.WorkItemTypeState{
		{
			Name:  "open",
			Color: "",
			// InProgress matches the statepicker's stateIcon("InProgress") → "◐"
			Category: "InProgress",
		},
		{
			Name:  "closed",
			Color: "",
			// Completed matches the statepicker's stateIcon("Completed") → "✓"
			Category: "Completed",
		},
	}, nil
}

// updateIssueStateBody is the JSON body for PATCH /repos/{owner}/{repo}/issues/{number}.
type updateIssueStateBody struct {
	State       string `json:"state"`
	StateReason string `json:"state_reason"`
}

// UpdateWorkItemState updates the state of an issue via PATCH.
//
// state is mapped to GitHub's state + state_reason pair (case-insensitive):
//
//	"open"   → state=open,   state_reason=reopened
//	"closed" → state=closed, state_reason=completed
//
// Any other value returns an error rather than sending a malformed PATCH.
// state_reason "not_planned" is not used here; both close paths map to
// "completed". If you need "not_planned", call the GitHub API directly.
func (c *Client) UpdateWorkItemState(number int, state string) error {
	lower := strings.ToLower(strings.TrimSpace(state))

	var body updateIssueStateBody
	switch lower {
	case "open":
		body = updateIssueStateBody{State: "open", StateReason: "reopened"}
	case "closed":
		body = updateIssueStateBody{State: "closed", StateReason: "completed"}
	default:
		return fmt.Errorf("github: UpdateWorkItemState: unrecognized state %q: must be \"open\" or \"closed\"", state)
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%d", c.owner, c.repo, number)
	if err := c.doJSON("PATCH", path, body, nil); err != nil {
		return fmt.Errorf("github: update work item state: %w", err)
	}
	return nil
}

// GetWorkItemComments returns the comments for a given issue, in the order
// returned by GitHub (chronological, oldest first).
//
// per_page is capped at issuePerPageCap (100); pagination is not implemented.
func (c *Client) GetWorkItemComments(number int) ([]IssueComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=%d",
		c.owner, c.repo, number, issuePerPageCap)

	var comments []IssueComment
	if err := c.getJSON(path, &comments); err != nil {
		return nil, fmt.Errorf("github: get work item comments: %w", err)
	}
	return comments, nil
}

// addCommentBody is the JSON body for POST /repos/{owner}/{repo}/issues/{number}/comments.
type addCommentBody struct {
	Body string `json:"body"`
}

// AddWorkItemComment posts a new comment on an issue and returns the created
// IssueComment as echoed back by GitHub.
func (c *Client) AddWorkItemComment(number int, text string) (IssueComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.owner, c.repo, number)

	payload := addCommentBody{Body: text}
	var created IssueComment
	if err := c.doJSON("POST", path, payload, &created); err != nil {
		return IssueComment{}, fmt.Errorf("github: add work item comment: %w", err)
	}
	return created, nil
}
