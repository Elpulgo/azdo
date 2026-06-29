package github

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapPullRequest maps a GitHub wire PullRequest to a provider.PullRequest.
//
// scope is the "owner/repo" string and scopeDisplay is its human-readable
// equivalent; both are stamped onto Identity, mirroring the azdevops convention.
//
// Reviewers are left empty in the returned struct. The caller should fetch reviews
// and requested_reviewers independently and then call MapReviewers to build the
// []provider.Reviewer slice. This split keeps MapPullRequest pure and composable:
// the client (task 9) assembles all three API calls before constructing the final
// provider.PullRequest.
//
// RepositoryID and RepositoryName are both set to scope ("owner/repo"). GitHub's
// PR wire payload carries no separate numeric repository ID, so scope is the most
// stable unique key available at this level.
func MapPullRequest(pr PullRequest, scope, scopeDisplay string) provider.PullRequest {
	// Synthesize a state_reason so we can reuse MapStateCategory.
	// GitHub PRs have no state_reason field; we derive it from merge status:
	//   open              → reason ""             → StateCategoryActive
	//   closed + merged   → reason "completed"    → StateCategoryClosedDone
	//   closed + unmerged → reason "not_planned"  → StateCategoryRemoved (abandoned)
	stateReason := ""
	if pr.State == "closed" {
		if pr.MergedAt != nil {
			stateReason = "completed"
		} else {
			stateReason = "not_planned"
		}
	}
	statusCategory := MapStateCategory(pr.State, stateReason)

	return provider.PullRequest{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", pr.Number),
		},
		Title:          pr.Title,
		Description:    pr.Body,
		Status:         pr.State, // raw "open" / "closed"
		StatusCategory: statusCategory,
		CreationDate:   pr.CreatedAt,
		SourceRefName:  pr.Head.Ref,
		TargetRefName:  pr.Base.Ref,
		IsDraft:        pr.Draft,
		CreatedByName:  pr.User.Login,
		CreatedByID:    fmt.Sprintf("%d", pr.User.ID),
		// GitHub PR wire carries no separate numeric repo ID.
		// Use scope ("owner/repo") as a stable unique key for both fields.
		RepositoryID:   scope,
		RepositoryName: scope,
		WebURL:         pr.HTMLURL,
		// Reviewers: populated by caller via MapReviewers.
	}
}

// voteIntFromKind returns an integer vote value consistent with the provided VoteKind.
// GitHub has no numeric vote system. These values mirror the Azure DevOps conventions
// so that any consumer reading Reviewer.Vote (rather than Reviewer.Kind) still renders
// correctly. Consumer audit: the UI display code (list.go voteIconWithStyles,
// detail.go reviewerVoteIconWithStyles/reviewerVoteDescription) reads only Reviewer.Kind,
// not Reviewer.Vote. Vote is populated for robustness and forward-compatibility.
func voteIntFromKind(k provider.VoteKind) int {
	switch k {
	case provider.VoteKindApproved:
		return 10
	case provider.VoteKindRejected:
		return -10
	default:
		return 0
	}
}

// MapReviewers builds a []provider.Reviewer from submitted reviews and requested users.
//
// Algorithm:
//  1. Aggregate reviews by user ID, keeping the latest submission per user (by
//     SubmittedAt). PENDING reviews are ignored — they express no intent.
//  2. Map each latest review to a Reviewer with the derived Kind and Vote.
//  3. Append requested_reviewers who have no submitted review yet (Kind: VoteKindNoVote,
//     Vote: 0). Users already covered by a review are skipped.
//
// Reviewer order: reviews appear in first-seen order; requested-only reviewers are
// appended after all reviewed entries in the order they appear in requested.
func MapReviewers(reviews []Review, requested []User) []provider.Reviewer {
	// Track user order (first time we see each user in reviews, excluding PENDING).
	var userOrder []int64
	latestByUser := make(map[int64]Review)

	for _, r := range reviews {
		if r.State == "PENDING" {
			continue
		}
		existing, ok := latestByUser[r.User.ID]
		if !ok {
			userOrder = append(userOrder, r.User.ID)
			latestByUser[r.User.ID] = r
		} else if r.SubmittedAt.After(existing.SubmittedAt) {
			latestByUser[r.User.ID] = r
		}
	}

	result := make([]provider.Reviewer, 0, len(userOrder)+len(requested))

	for _, uid := range userOrder {
		r := latestByUser[uid]
		kind := MapVoteKind(r.State)
		result = append(result, provider.Reviewer{
			ID:          fmt.Sprintf("%d", r.User.ID),
			DisplayName: r.User.Login,
			Kind:        kind,
			Vote:        voteIntFromKind(kind),
		})
	}

	// Track which users already have a review entry so we don't duplicate.
	reviewedIDs := make(map[int64]bool, len(userOrder))
	for _, uid := range userOrder {
		reviewedIDs[uid] = true
	}

	for _, u := range requested {
		if reviewedIDs[u.ID] {
			continue
		}
		result = append(result, provider.Reviewer{
			ID:          fmt.Sprintf("%d", u.ID),
			DisplayName: u.Login,
			Kind:        provider.VoteKindNoVote,
			Vote:        0,
		})
	}

	return result
}

// MapReviewThreads maps a flat list of GitHub review comments to []provider.Thread.
//
// GitHub's review-comment API returns a flat list. A comment with InReplyToID == nil
// is the thread root; replies carry InReplyToID pointing to their root's ID. This
// function reconstructs the tree structure:
//   - Root comments (InReplyToID == nil) start a new thread.
//   - Replies are appended to the thread whose root ID matches their InReplyToID.
//   - If a reply's InReplyToID matches no known root (defensive case), a new thread
//     is created keyed on that ID so no comments are silently dropped.
//
// Thread ordering: roots appear in the order they are first encountered in the input;
// replies are appended in input order to their respective thread.
//
// Thread.Status is always "active". GitHub REST review-comments carry no
// resolved/unresolved state (that requires the GraphQL API). Resolved status will
// be layered in during task 9. Setting "active" is consistent with the UI's default
// rendering path and avoids hiding unresolved threads.
func MapReviewThreads(comments []ReviewComment, scope, scopeDisplay string) []provider.Thread {
	type threadData struct {
		rootID  int64
		root    ReviewComment
		replies []ReviewComment
	}

	// Maintain insertion order for deterministic output.
	var threadOrder []int64
	threadMap := make(map[int64]*threadData)

	for _, c := range comments {
		if c.InReplyToID == nil {
			// Root comment → start a new thread.
			td := &threadData{rootID: c.ID, root: c}
			threadMap[c.ID] = td
			threadOrder = append(threadOrder, c.ID)
		} else {
			replyTo := *c.InReplyToID
			td, ok := threadMap[replyTo]
			if !ok {
				// Defensive: reply references a root we have not seen. Create a
				// synthetic thread keyed on replyTo so the comment is not dropped.
				// The reply itself becomes the effective root of this orphan thread.
				td = &threadData{rootID: replyTo, root: c}
				threadMap[replyTo] = td
				threadOrder = append(threadOrder, replyTo)
				// c is now the thread root; do not add it to replies.
			} else {
				td.replies = append(td.replies, c)
			}
		}
	}

	threads := make([]provider.Thread, 0, len(threadOrder))
	for _, rootID := range threadOrder {
		td := threadMap[rootID]

		// Map root comment. ParentCommentID = 0 (this is the thread root).
		rootComment := mapReviewComment(td.root, 0, scope, scopeDisplay)

		threadComments := []provider.Comment{rootComment}
		lastUpdated := td.root.UpdatedAt

		for _, reply := range td.replies {
			// ParentCommentID = the root comment's ID (from InReplyToID).
			parentID := int(derefInt64(reply.InReplyToID))
			c := mapReviewComment(reply, parentID, scope, scopeDisplay)
			threadComments = append(threadComments, c)
			if reply.UpdatedAt.After(lastUpdated) {
				lastUpdated = reply.UpdatedAt
			}
		}

		// Line: prefer the current-diff anchor; fall back to OriginalLine for
		// comments anchored to an outdated diff position.
		line := derefInt(td.root.Line)
		if line == 0 {
			line = derefInt(td.root.OriginalLine)
		}

		threads = append(threads, provider.Thread{
			Identity: provider.Identity{
				Kind:         provider.KindGitHub,
				Scope:        scope,
				ScopeDisplay: scopeDisplay,
				ID:           fmt.Sprintf("%d", rootID),
			},
			PublishedDate:   td.root.CreatedAt,
			LastUpdatedDate: lastUpdated,
			// GitHub REST carries no resolved state on review comments.
			// Default to "active"; GraphQL-based resolve state layered in later.
			Status:    "active",
			FilePath:  td.root.Path,
			Line:      line,
			Comments:  threadComments,
			IsDeleted: false,
		})
	}

	return threads
}

// mapReviewComment maps a single ReviewComment to a provider.Comment.
// parentCommentID is 0 for thread roots and the numeric root ID for replies.
func mapReviewComment(c ReviewComment, parentCommentID int, scope, scopeDisplay string) provider.Comment {
	return provider.Comment{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", c.ID),
		},
		ParentCommentID: parentCommentID,
		Content:         c.Body,
		PublishedDate:   c.CreatedAt,
		LastUpdatedDate: c.UpdatedAt,
		CommentType:     "text",
		AuthorName:      c.User.Login,
		AuthorID:        fmt.Sprintf("%d", c.User.ID),
	}
}

// MapPRFile maps a GitHub PRFile to a provider.IterationChange.
//
// changeID is supplied by the caller as index+1 (1-based). GitHub's PR files
// endpoint returns an ordered list of files with no numeric change IDs, so a
// caller-provided sequential ID is the closest equivalent.
//
// GitObjectType is always "blob": the GitHub files endpoint only returns regular
// files; directory entries are not included (unlike Azure DevOps iterations, which
// include "tree" entries that the UI filters out).
func MapPRFile(f PRFile, changeID int) provider.IterationChange {
	return provider.IterationChange{
		ChangeID:      changeID,
		Path:          f.Filename,
		GitObjectType: "blob",
		ChangeType:    mapChangeType(f.Status),
		OriginalPath:  f.PreviousFilename,
	}
}

// mapChangeType translates a GitHub file status string into the neutral change-type
// verb expected by the diff view and detail view.
//
// Neutral verbs and their display semantics (see changeTypeDisplay in detail.go):
//
//	"add"    → "+" icon (Success style)
//	"edit"   → "~" icon (Info style)
//	"delete" → "-" icon (Error style)
//	"rename" → "→" icon (Warning style)
//
// GitHub status → neutral verb:
//
//	"added"           → "add"
//	"removed"         → "delete"
//	"modified"        → "edit"
//	"renamed"         → "rename"
//	"copied"          → "add"   (destination file is new to the PR)
//	"changed"         → "edit"  (GitHub alias for modified)
//	"unchanged"       → "edit"  (rarely returned; diff will be empty)
//	<unknown/future>  → "edit"  (safe fallback — keeps diff view functional)
func mapChangeType(status string) string {
	switch status {
	case "added":
		return "add"
	case "removed":
		return "delete"
	case "modified", "changed":
		return "edit"
	case "renamed":
		return "rename"
	case "copied":
		// No direct neutral equivalent; "add" is closest since the destination
		// is a new file from the PR's perspective.
		return "add"
	case "unchanged":
		return "edit"
	default:
		return "edit"
	}
}

// derefInt returns the int value pointed to by p, or 0 when p is nil.
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// derefInt64 returns the int64 value pointed to by p, or 0 when p is nil.
func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
