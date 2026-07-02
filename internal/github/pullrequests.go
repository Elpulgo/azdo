package github

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// prSearchNestedPR is the "pull_request" sub-object returned inside each item
// by GET /search/issues when the result set contains pull requests. It exposes
// merged_at, which is absent from the top-level issue-shape of the search item.
type prSearchNestedPR struct {
	MergedAt *time.Time `json:"merged_at"`
}

// prEnrichConcurrency bounds the number of concurrent GET /pulls/{n} requests
// used by enrichBranches to backfill head/base for search-sourced PRs.
const prEnrichConcurrency = 8

// prSearchItem is the per-item wire type for GET /search/issues results filtered
// to pull requests via the "is:pr" qualifier. The top-level shape is issue-like;
// the nested "pull_request" sub-object adds merged_at.
//
// Fidelity note: /search/issues items do NOT carry Draft, Head, or Base. Those
// fields are zero in the PullRequest returned by toPullRequest. This partial map
// is sufficient for list views. N+1 GET /repos/.../pulls/{n} enrichment is
// explicitly avoided per spec task 9.
//
// Resolves spec ## Unknowns "search-PR fidelity": the partial mapping is
// accepted for list views; merged_at is captured from the nested sub-object.
type prSearchItem struct {
	Number      int               `json:"number"`
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	State       string            `json:"state"`
	User        User              `json:"user"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ClosedAt    *time.Time        `json:"closed_at"`
	HTMLURL     string            `json:"html_url"`
	PullRequest *prSearchNestedPR `json:"pull_request"`
}

// toPullRequest converts a prSearchItem to the wire PullRequest type. Draft,
// Head, Base, and RequestedReviewers are left zero — they are not available
// from the /search/issues endpoint.
func (item prSearchItem) toPullRequest() PullRequest {
	var mergedAt *time.Time
	if item.PullRequest != nil {
		mergedAt = item.PullRequest.MergedAt
	}
	return PullRequest{
		Number:    item.Number,
		Title:     item.Title,
		Body:      item.Body,
		State:     item.State,
		User:      item.User,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
		ClosedAt:  item.ClosedAt,
		MergedAt:  mergedAt,
		HTMLURL:   item.HTMLURL,
		// Draft, Head, Base: not available in /search/issues items; left zero.
		// RequestedReviewers: not available in /search/issues items; left nil.
	}
}

// prSearchResponse is the JSON envelope for GET /search/issues when querying
// pull requests (using the "is:pr" qualifier).
type prSearchResponse struct {
	TotalCount int            `json:"total_count"`
	Items      []prSearchItem `json:"items"`
}

// ListPullRequests returns up to top pull requests for the repository sorted
// by most recently updated. opts.States is translated to the GitHub state
// parameter (open/closed/all) via mapStateParam.
//
// top is capped at issuePerPageCap (100); pagination is not yet implemented.
func (c *Client) ListPullRequests(top int, opts provider.ListOpts) ([]PullRequest, error) {
	if top <= 0 {
		top = issuePerPageCap
	}
	if top > issuePerPageCap {
		top = issuePerPageCap
	}

	state := mapStateParam(opts.States)
	path := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=%d&sort=updated&direction=desc",
		c.owner, c.repo, state, top)

	var prs []PullRequest
	if err := c.getJSON(path, &prs); err != nil {
		return nil, fmt.Errorf("github: list pull requests: %w", err)
	}
	return prs, nil
}

// ListMyPullRequests returns up to top pull requests authored by the
// authenticated user in this repository, using the GET /search/issues endpoint.
//
// The query includes "is:pr" (required — without it issues also appear) and
// "author:@me" (GitHub resolves @me server-side to the token owner's login).
//
// Fidelity limitation: search items are issue-shaped. The returned PullRequest
// values have Draft=false and zero Head/Base; merged_at is captured from the
// nested "pull_request" sub-object. See prSearchItem for details.
//
// top is capped at issuePerPageCap (100). The /search/issues endpoint has a
// rate limit of 30 requests/minute (authenticated).
func (c *Client) ListMyPullRequests(top int, opts provider.ListOpts) ([]PullRequest, error) {
	return c.searchPullRequests(top, opts, "author:@me")
}

// ListPullRequestsAsReviewer returns up to top pull requests where the
// authenticated user has been requested as a reviewer, using /search/issues.
//
// Same fidelity limitations as ListMyPullRequests apply.
func (c *Client) ListPullRequestsAsReviewer(top int, opts provider.ListOpts) ([]PullRequest, error) {
	return c.searchPullRequests(top, opts, "review-requested:@me")
}

// searchPullRequests is the shared implementation for ListMyPullRequests and
// ListPullRequestsAsReviewer. qualifier is the search term that distinguishes
// the two: "author:@me" vs "review-requested:@me".
func (c *Client) searchPullRequests(top int, opts provider.ListOpts, qualifier string) ([]PullRequest, error) {
	if top <= 0 {
		top = issuePerPageCap
	}
	if top > issuePerPageCap {
		top = issuePerPageCap
	}

	// "is:pr" is required — /search/issues returns both issues and PRs by default.
	q := fmt.Sprintf("repo:%s/%s is:pr %s", c.owner, c.repo, qualifier)

	state := mapStateParam(opts.States)
	if state == "open" || state == "closed" {
		q += " state:" + state
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("per_page", fmt.Sprintf("%d", top))

	path := "/search/issues?" + params.Encode()

	var envelope prSearchResponse
	if err := c.getJSON(path, &envelope); err != nil {
		return nil, fmt.Errorf("github: search pull requests (%s): %w", qualifier, err)
	}

	prs := make([]PullRequest, len(envelope.Items))
	for i, item := range envelope.Items {
		prs[i] = item.toPullRequest()
	}
	c.enrichBranches(prs)
	return prs, nil
}

// enrichBranches fills in Head/Base (source/target branches) for search-sourced
// PRs, which GET /search/issues does not return. Without this the PR list rows
// and detail header render an empty "→" for the my-PRs and reviewer tabs.
//
// This is a bounded-concurrency N+1 over the search result (each PR fetched via
// GET /pulls/{n}); the spec originally deferred this enrichment, but the empty
// branch display reads as a bug. The result set is capped at issuePerPageCap and
// concurrency at prEnrichConcurrency. It is best-effort: a per-PR fetch error
// leaves that PR's branches empty rather than failing the whole list.
func (c *Client) enrichBranches(prs []PullRequest) {
	sem := make(chan struct{}, prEnrichConcurrency)
	var wg sync.WaitGroup
	for i := range prs {
		if prs[i].Head.Ref != "" && prs[i].Base.Ref != "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			full, err := c.GetPullRequest(prs[idx].Number)
			if err != nil {
				return // best-effort: leave branches empty on failure
			}
			prs[idx].Head = full.Head
			prs[idx].Base = full.Base
		}(i)
	}
	wg.Wait()
}

// GetPullRequest fetches a single pull request via
// GET /repos/{owner}/{repo}/pulls/{number}. Used to enrich search-sourced PRs
// (which lack head/base) with their source and target branches.
func (c *Client) GetPullRequest(number int) (PullRequest, error) {
	if number <= 0 {
		return PullRequest{}, fmt.Errorf("github: get pull request: invalid number %d", number)
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", c.owner, c.repo, number)
	var pr PullRequest
	if err := c.getJSON(path, &pr); err != nil {
		return PullRequest{}, fmt.Errorf("github: get pull request #%d: %w", number, err)
	}
	return pr, nil
}

// GetPRThreads returns the flat list of review comments for the given pull
// request number. Comments are returned in GitHub's delivery order (typically
// root comments first, followed by replies).
//
// The adapter groups comments into provider.Thread values via MapReviewThreads.
// Grouping is intentionally NOT done here, keeping this method thin.
//
// per_page is capped at issuePerPageCap (100); pagination is not implemented.
func (c *Client) GetPRThreads(number int) ([]ReviewComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?per_page=%d",
		c.owner, c.repo, number, issuePerPageCap)

	var comments []ReviewComment
	if err := c.getJSON(path, &comments); err != nil {
		return nil, fmt.Errorf("github: get PR threads: %w", err)
	}
	return comments, nil
}

// GetPRFiles returns the files changed in the given pull request.
// The adapter maps each file via MapPRFile to produce []provider.IterationChange.
//
// Per spec Decision #5, one synthetic iteration covers the whole PR; this method
// provides the file list for that synthetic iteration.
//
// per_page is capped at issuePerPageCap (100); pagination is not implemented.
func (c *Client) GetPRFiles(number int) ([]PRFile, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=%d",
		c.owner, c.repo, number, issuePerPageCap)

	var files []PRFile
	if err := c.getJSON(path, &files); err != nil {
		return nil, fmt.Errorf("github: get PR files: %w", err)
	}
	return files, nil
}

// submitReviewBody is the JSON body for
// POST /repos/{owner}/{repo}/pulls/{number}/reviews.
type submitReviewBody struct {
	Event string `json:"event"`
}

// VotePullRequest submits a GitHub review on the given pull request, mapping
// the neutral vote integer to a GitHub review event:
//
//	vote > 0  → "APPROVE"          (neutral VoteKindApproved,  voteIntFromKind = 10)
//	vote < 0  → "REQUEST_CHANGES"  (neutral VoteKindRejected,  voteIntFromKind = −10)
//	vote == 0 → "COMMENT"          (neutral no-vote / reset;   voteIntFromKind = 0)
//
// Thresholds align with voteIntFromKind in mapping_pr.go so any VoteKind
// round-trips correctly through the adapter (e.g. vote=5 → >0 → APPROVE;
// vote=−5 → <0 → REQUEST_CHANGES).
//
// Note: GitHub's "COMMENT" event requires a non-empty body. Submitting vote==0
// without a body will be rejected by the API. If a body-carrying comment is
// needed, use AddPRComment instead.
func (c *Client) VotePullRequest(number int, vote int) error {
	var event string
	switch {
	case vote > 0:
		event = "APPROVE"
	case vote < 0:
		event = "REQUEST_CHANGES"
	default:
		event = "COMMENT"
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", c.owner, c.repo, number)
	payload := submitReviewBody{Event: event}
	if err := c.doJSON("POST", path, payload, nil); err != nil {
		return fmt.Errorf("github: vote pull request: %w", err)
	}
	return nil
}

// fileContentResponse is the JSON body returned by
// GET /repos/{owner}/{repo}/contents/{path}.
type fileContentResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// GetFileContent returns the raw decoded content of a file at the given ref
// (branch name, tag, or commit SHA).
//
// GitHub's Contents API base64-encodes file content and wraps it at 60
// characters per line. This method strips those newlines before decoding.
//
// Known limitation: the Contents API returns no inline content for files
// larger than 1 MB (the "content" field is empty, download_url is present).
// Files over 1 MB silently return an empty string from this method. The Git
// Blobs API (/git/blobs/{sha}) is the correct fallback for large files, but
// it is not implemented here.
//
// filePath segments and the ref query value are individually URL-escaped.
func (c *Client) GetFileContent(filePath string, branchName string) (string, error) {
	// URL-escape each path segment independently to preserve "/" as a separator.
	segments := strings.Split(strings.TrimPrefix(filePath, "/"), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	escapedFilePath := strings.Join(segments, "/")

	path := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s",
		c.owner, c.repo, escapedFilePath, url.QueryEscape(branchName))

	var resp fileContentResponse
	if err := c.getJSON(path, &resp); err != nil {
		return "", fmt.Errorf("github: get file content: %w", err)
	}

	if resp.Encoding != "base64" {
		// Non-base64 encoding (or empty content for >1 MB files): return as-is.
		return resp.Content, nil
	}

	// GitHub wraps base64 at 60 characters per line. Strip all newlines before
	// decoding so base64.StdEncoding handles the full string.
	cleaned := strings.ReplaceAll(resp.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("github: decode file content: %w", err)
	}
	return string(decoded), nil
}

// addCodeCommentBody is the JSON body for
// POST /repos/{owner}/{repo}/pulls/{number}/comments (inline code comment).
type addCodeCommentBody struct {
	Body     string `json:"body"`
	CommitID string `json:"commit_id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
}

// AddPRCodeComment creates an inline code comment on the given file and line
// in the pull request.
//
// GitHub's review-comment API requires the head commit SHA (commit_id). This
// method fetches the PR via GET /repos/{o}/{r}/pulls/{number} to obtain
// head.sha, then posts the comment. This adds one extra round-trip per call.
//
// Side is always "RIGHT" (new-file side). The LEFT (base-file) side is not
// exposed via this method.
func (c *Client) AddPRCodeComment(number int, filePath string, line int, content string) (ReviewComment, error) {
	// Fetch the PR to obtain the head SHA required by the review-comment API.
	prPath := fmt.Sprintf("/repos/%s/%s/pulls/%d", c.owner, c.repo, number)
	var pr PullRequest
	if err := c.getJSON(prPath, &pr); err != nil {
		return ReviewComment{}, fmt.Errorf("github: add PR code comment (fetch PR): %w", err)
	}

	commentPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", c.owner, c.repo, number)
	payload := addCodeCommentBody{
		Body:     content,
		CommitID: pr.Head.SHA,
		Path:     filePath,
		Line:     line,
		Side:     "RIGHT",
	}

	var created ReviewComment
	if err := c.doJSON("POST", commentPath, payload, &created); err != nil {
		return ReviewComment{}, fmt.Errorf("github: add PR code comment: %w", err)
	}
	return created, nil
}

// AddPRComment creates a general (non-file) comment on the pull request.
//
// GitHub models pull requests as issues, so general PR comments are posted to
// the issue-comment endpoint. The created IssueComment is returned as echoed
// by GitHub. addCommentBody (from workitems.go, same package) is reused.
func (c *Client) AddPRComment(number int, content string) (IssueComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.owner, c.repo, number)
	payload := addCommentBody{Body: content}

	var created IssueComment
	if err := c.doJSON("POST", path, payload, &created); err != nil {
		return IssueComment{}, fmt.Errorf("github: add PR comment: %w", err)
	}
	return created, nil
}

// replyToThreadBody is the JSON body for posting a reply to a review thread via
// POST /repos/{owner}/{repo}/pulls/{number}/comments.
type replyToThreadBody struct {
	Body      string `json:"body"`
	InReplyTo int    `json:"in_reply_to"`
}

// ReplyToThread posts a reply to an existing review thread.
//
// rootCommentID is the database ID of the root (first) comment in the thread.
// This is the value that MapReviewThreads stamps as the thread Identity.ID, so
// the neutral threadID the adapter passes is directly usable here.
//
// Returns the created ReviewComment as echoed by GitHub.
func (c *Client) ReplyToThread(number int, rootCommentID int, content string) (ReviewComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", c.owner, c.repo, number)
	payload := replyToThreadBody{
		Body:      content,
		InReplyTo: rootCommentID,
	}

	var created ReviewComment
	if err := c.doJSON("POST", path, payload, &created); err != nil {
		return ReviewComment{}, fmt.Errorf("github: reply to thread: %w", err)
	}
	return created, nil
}

// graphqlRequest is the JSON body sent to POST /graphql for any GraphQL query
// or mutation.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// graphqlError is a single entry in a GraphQL response's top-level "errors" array.
type graphqlError struct {
	Message string `json:"message"`
}

// graphql sends a GraphQL query or mutation to POST {baseURL}/graphql. The
// base URL is shared with the REST client; the /graphql path is appended here.
//
// Unlike the REST API, GraphQL always returns HTTP 200; errors are in the
// response "errors" field and must be checked by the caller after decoding.
func (c *Client) graphql(query string, variables map[string]any, dst any) error {
	payload := graphqlRequest{Query: query, Variables: variables}
	if err := c.doJSON("POST", "/graphql", payload, dst); err != nil {
		return fmt.Errorf("github: graphql: %w", err)
	}
	return nil
}

// reviewThreadsResponse is the GraphQL response shape for the review-threads
// listing query used inside UpdateThreadStatus.
type reviewThreadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []reviewThreadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []graphqlError `json:"errors,omitempty"`
}

// reviewThreadNode is a single node returned by the review-threads GraphQL query.
type reviewThreadNode struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	Comments   struct {
		Nodes []struct {
			DatabaseID int64 `json:"databaseId"`
		} `json:"nodes"`
	} `json:"comments"`
}

// resolveMutationResponse is the GraphQL response for resolveReviewThread and
// unresolveReviewThread mutations.
type resolveMutationResponse struct {
	Data   map[string]any `json:"data"`
	Errors []graphqlError `json:"errors,omitempty"`
}

// UpdateThreadStatus resolves or unresolves a pull-request review thread using
// the GitHub GraphQL API. This is the single GraphQL spot in the client per
// spec Decision #2 — GitHub has no REST endpoint for conversation resolution.
//
// Thread-matching: rootCommentID is the REST review-comment database ID. This
// is what MapReviewThreads stamps as the thread Identity.ID, so the neutral
// threadID from the adapter is directly usable. The method queries the PR's
// GraphQL review threads and matches on first-comment databaseId.
//
// Status mapping (case-insensitive):
//
//	"fixed", "resolved", "closed", "wontfix" → resolveReviewThread
//	"active", "reopened", "open"              → unresolveReviewThread
//
// No-match behaviour: if no review thread's first-comment databaseId matches
// rootCommentID, a descriptive error is returned. This surfaces the limitation
// rather than silently pretending success.
//
// Resolves spec ## Unknowns "unmatched-thread no-op": returns a descriptive
// error when no thread matches, not a silent no-op.
func (c *Client) UpdateThreadStatus(number int, rootCommentID int, status string) error {
	// Step 1: list the PR's review threads via GraphQL to get node IDs.
	const threadsQuery = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$number){reviewThreads(first:100){nodes{id isResolved comments(first:1){nodes{databaseId}}}}}}}`

	vars := map[string]any{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}

	var threadsResp reviewThreadsResponse
	if err := c.graphql(threadsQuery, vars, &threadsResp); err != nil {
		return fmt.Errorf("github: update thread status: %w", err)
	}
	if len(threadsResp.Errors) > 0 {
		return fmt.Errorf("github: update thread status: graphql error: %s", threadsResp.Errors[0].Message)
	}

	// Step 2: find the thread whose first comment databaseId matches rootCommentID.
	var threadNodeID string
	for _, node := range threadsResp.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if len(node.Comments.Nodes) > 0 && node.Comments.Nodes[0].DatabaseID == int64(rootCommentID) {
			threadNodeID = node.ID
			break
		}
	}

	if threadNodeID == "" {
		return fmt.Errorf("github: update thread status: no review thread found with first-comment databaseId %d in PR #%d: "+
			"the thread may not be visible via GraphQL or rootCommentID may not be a thread-root comment",
			rootCommentID, number)
	}

	// Step 3: resolve or unresolve via a targeted mutation.
	lower := strings.ToLower(strings.TrimSpace(status))
	var mutation string
	switch lower {
	case "fixed", "resolved", "closed", "wontfix":
		mutation = `mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{id}}}`
	case "active", "reopened", "open":
		mutation = `mutation($id:ID!){unresolveReviewThread(input:{threadId:$id}){thread{id}}}`
	default:
		return fmt.Errorf("github: update thread status: unrecognized status %q: "+
			"use fixed/resolved/closed/wontfix to resolve or active/reopened/open to unresolve", status)
	}

	mutVars := map[string]any{"id": threadNodeID}
	var mutResp resolveMutationResponse
	if err := c.graphql(mutation, mutVars, &mutResp); err != nil {
		return fmt.Errorf("github: update thread status (mutation): %w", err)
	}
	if len(mutResp.Errors) > 0 {
		msg := mutResp.Errors[0].Message
		// resolveReviewThread/unresolveReviewThread are commonly rejected for
		// fine-grained PATs even with PR write access. Point the user at the fix
		// rather than surfacing GitHub's opaque "Resource not accessible" text.
		if strings.Contains(strings.ToLower(msg), "not accessible") {
			return fmt.Errorf("github: cannot resolve review thread: %s "+
				"(resolving threads needs a classic PAT with the 'repo' scope; "+
				"fine-grained tokens are often rejected for this GraphQL mutation)", msg)
		}
		return fmt.Errorf("github: update thread status: graphql mutation error: %s", msg)
	}

	return nil
}
