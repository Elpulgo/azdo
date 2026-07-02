package github

import "fmt"

// defaultWebBaseURL is the github.com web host used to build browser URLs.
// GitHub Enterprise deployments would use a different web host — that is a
// Phase-4 config concern; Phase 3 targets github.com only.
const defaultWebBaseURL = "https://github.com"

// WorkItemURL returns the github.com browser URL for the given issue:
//
//	https://github.com/{owner}/{repo}/issues/{id}
//
// Returns "" when id <= 0.
func (c *Client) WorkItemURL(id int) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/issues/%d", defaultWebBaseURL, c.owner, c.repo, id)
}

// PRURL returns the github.com browser URL for the given pull request:
//
//	https://github.com/{owner}/{repo}/pull/{prID}
//
// Returns "" when prID <= 0.
func (c *Client) PRURL(prID int) string {
	if prID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pull/%d", defaultWebBaseURL, c.owner, c.repo, prID)
}

// PRThreadWebURL returns the github.com browser URL anchored to a specific
// review comment thread:
//
//	https://github.com/{owner}/{repo}/pull/{prID}#discussion_r{threadID}
//
// threadID is the root review-comment database id (stamped as thread Identity.ID
// by MapReviewThreads). The #discussion_r fragment is how github.com anchors to
// review comments. Returns "" when prID <= 0 or threadID <= 0.
func (c *Client) PRThreadWebURL(prID int, threadID int) string {
	if prID <= 0 || threadID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pull/%d#discussion_r%d", defaultWebBaseURL, c.owner, c.repo, prID, threadID)
}

// PipelineURL returns the github.com browser URL for the given Actions workflow run:
//
//	https://github.com/{owner}/{repo}/actions/runs/{id}
//
// Returns "" when id <= 0.
func (c *Client) PipelineURL(id int) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/actions/runs/%d", defaultWebBaseURL, c.owner, c.repo, id)
}
