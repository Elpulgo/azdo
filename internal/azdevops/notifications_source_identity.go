package azdevops

import "fmt"

// resolveAuthenticatedUserID fetches the authenticated user's id from any one
// project client — every client in a MultiClient shares the same PAT and org,
// so connectionData returns the same identity regardless of which one asks,
// and the first usable client wins.
//
// Called exactly once per poll, by runSourcesConcurrently
// (adapter_notifications.go), which threads the result into all three
// sources that need it: SourceReviewRequested passes it to
// ListPullRequestsAsReviewerForUser, SourceMentioned compares it against
// CommentMention.targetId, and SourceCIFailed passes it as
// ListMyFailedPipelineRuns' requestedFor parameter and re-checks it in Go via
// isMyFailedRun. Resolving it once and sharing it, rather than letting each
// source call this independently, is what keeps three concurrent goroutines
// from racing Client.userID's unsynchronized cache field when they land on
// the same *Client in the common single-project case (task 8 review, 🔴
// finding 2) — see runSourcesConcurrently's doc comment for the full
// rationale. All three comparisons are plain string equality, so all three
// abort rather than proceed without an id.
func resolveAuthenticatedUserID(mc *MultiClient) (string, error) {
	for _, p := range mc.Projects() {
		c := mc.ClientFor(p)
		if c == nil {
			continue
		}
		id, err := c.GetCurrentUserID()
		if err != nil {
			return "", fmt.Errorf("failed to get current user ID: %w", err)
		}
		return guardNonEmptyUserID(id)
	}
	return "", fmt.Errorf("no client configured")
}

// guardNonEmptyUserID rejects an empty authenticated-user id before it can
// reach an identity comparison.
//
// Both call sites compare with ==, so an empty id does not fail closed, it
// fails *open*: SourceMentioned would match any mention payload carrying no
// resolved targetId, and SourceCIFailed's isMyFailedRun would attribute every
// run with an absent requestedFor — a scheduled or system-triggered build —
// to the caller.
//
// Client.GetCurrentUserID already rejects an empty AuthenticatedUser.ID
// (client.go:234), so resolveAuthenticatedUserID cannot reach this branch
// through a real client today; the guard is defence in depth against
// SetUserID (client.go:40), which writes the cache unchecked. It is split out
// as a separate function precisely so that unreachability does not also mean
// untested — TestGuardNonEmptyUserID exercises it directly.
func guardNonEmptyUserID(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("resolved an empty user ID")
	}
	return id, nil
}
