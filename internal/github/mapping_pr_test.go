package github_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// ── MapPullRequest ────────────────────────────────────────────────────────────

func TestMapPullRequest_Open_Active(t *testing.T) {
	const raw = `{
		"number": 42,
		"title": "Add feature X",
		"body": "Implements the new feature.",
		"state": "open",
		"draft": false,
		"user": {"login": "alice", "id": 1001},
		"requested_reviewers": [],
		"head": {"ref": "feature/x"},
		"base": {"ref": "main"},
		"created_at": "2026-04-01T10:00:00Z",
		"updated_at": "2026-04-02T08:00:00Z",
		"closed_at": null,
		"merged_at": null,
		"html_url": "https://github.com/octo/repo/pull/42"
	}`

	var pr github.PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPullRequest(pr, testScope, testScopeDisplay)

	// Identity invariant
	assertIdentityInvariant(t, "PR(open)", got.Identity)
	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Identity.Scope != testScope {
		t.Errorf("Scope = %q, want %q", got.Identity.Scope, testScope)
	}
	if got.Identity.ScopeDisplay != testScopeDisplay {
		t.Errorf("ScopeDisplay = %q, want %q", got.Identity.ScopeDisplay, testScopeDisplay)
	}
	if got.Identity.ID != "42" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "42")
	}

	// Core fields
	if got.Title != "Add feature X" {
		t.Errorf("Title = %q, want %q", got.Title, "Add feature X")
	}
	if got.Description != "Implements the new feature." {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Status != "open" {
		t.Errorf("Status = %q, want %q", got.Status, "open")
	}
	if got.StatusCategory != provider.StateCategoryActive {
		t.Errorf("StatusCategory = %v, want StateCategoryActive", got.StatusCategory)
	}
	if got.IsDraft {
		t.Error("IsDraft should be false")
	}

	// Refs
	if got.SourceRefName != "feature/x" {
		t.Errorf("SourceRefName = %q, want %q", got.SourceRefName, "feature/x")
	}
	if got.TargetRefName != "main" {
		t.Errorf("TargetRefName = %q, want %q", got.TargetRefName, "main")
	}

	// Author
	if got.CreatedByName != "alice" {
		t.Errorf("CreatedByName = %q, want %q", got.CreatedByName, "alice")
	}
	if got.CreatedByID != "1001" {
		t.Errorf("CreatedByID = %q, want %q", got.CreatedByID, "1001")
	}

	// Repository (both fields use scope)
	if got.RepositoryID != testScope {
		t.Errorf("RepositoryID = %q, want %q", got.RepositoryID, testScope)
	}
	if got.RepositoryName != testScope {
		t.Errorf("RepositoryName = %q, want %q", got.RepositoryName, testScope)
	}

	// Web URL
	if got.WebURL != "https://github.com/octo/repo/pull/42" {
		t.Errorf("WebURL = %q", got.WebURL)
	}

	// Date
	wantCreated := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if !got.CreationDate.Equal(wantCreated) {
		t.Errorf("CreationDate = %v, want %v", got.CreationDate, wantCreated)
	}

	// Reviewers left empty (populated separately via MapReviewers)
	if len(got.Reviewers) != 0 {
		t.Errorf("Reviewers = %v, want empty (caller populates via MapReviewers)", got.Reviewers)
	}
}

func TestMapPullRequest_Merged_ClosedDone(t *testing.T) {
	const raw = `{
		"number": 7,
		"title": "Merge something",
		"body": "",
		"state": "closed",
		"draft": false,
		"user": {"login": "bob", "id": 2},
		"requested_reviewers": [],
		"head": {"ref": "fix/bug"},
		"base": {"ref": "main"},
		"created_at": "2026-03-01T00:00:00Z",
		"updated_at": "2026-03-05T00:00:00Z",
		"closed_at": "2026-03-05T12:00:00Z",
		"merged_at": "2026-03-05T12:00:00Z",
		"html_url": "https://github.com/octo/repo/pull/7"
	}`

	var pr github.PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPullRequest(pr, testScope, testScopeDisplay)

	if got.StatusCategory != provider.StateCategoryClosedDone {
		t.Errorf("StatusCategory = %v, want StateCategoryClosedDone (merged PR)", got.StatusCategory)
	}
	if got.Status != "closed" {
		t.Errorf("Status = %q, want %q", got.Status, "closed")
	}
	assertIdentityInvariant(t, "PR(merged)", got.Identity)
}

func TestMapPullRequest_ClosedUnmerged_Removed(t *testing.T) {
	const raw = `{
		"number": 99,
		"title": "Abandoned PR",
		"body": "",
		"state": "closed",
		"draft": false,
		"user": {"login": "charlie", "id": 3},
		"requested_reviewers": [],
		"head": {"ref": "abandoned"},
		"base": {"ref": "main"},
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-10T00:00:00Z",
		"closed_at": "2026-01-10T00:00:00Z",
		"merged_at": null,
		"html_url": "https://github.com/octo/repo/pull/99"
	}`

	var pr github.PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPullRequest(pr, testScope, testScopeDisplay)

	// closed-without-merge → abandoned → StateCategoryRemoved
	if got.StatusCategory != provider.StateCategoryRemoved {
		t.Errorf("StatusCategory = %v, want StateCategoryRemoved (closed without merge)", got.StatusCategory)
	}
	assertIdentityInvariant(t, "PR(closed-unmerged)", got.Identity)
}

func TestMapPullRequest_Draft(t *testing.T) {
	const raw = `{
		"number": 5,
		"title": "WIP",
		"body": "",
		"state": "open",
		"draft": true,
		"user": {"login": "dev", "id": 9},
		"requested_reviewers": [],
		"head": {"ref": "wip"},
		"base": {"ref": "main"},
		"created_at": "2026-06-01T00:00:00Z",
		"updated_at": "2026-06-01T00:00:00Z",
		"closed_at": null,
		"merged_at": null,
		"html_url": "https://github.com/octo/repo/pull/5"
	}`

	var pr github.PullRequest
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPullRequest(pr, testScope, testScopeDisplay)

	if !got.IsDraft {
		t.Error("IsDraft should be true for a draft PR")
	}
}

// ── MapReviewers ──────────────────────────────────────────────────────────────

func TestMapReviewers_ApprovedAndRejected(t *testing.T) {
	// alice approved; bob requested changes; charlie has no review (requested only).
	reviewsJSON := `[
		{"id": 101, "state": "APPROVED",            "user": {"login": "alice", "id": 1}, "body": "", "submitted_at": "2026-05-01T09:00:00Z"},
		{"id": 102, "state": "CHANGES_REQUESTED",   "user": {"login": "bob",   "id": 2}, "body": "", "submitted_at": "2026-05-01T10:00:00Z"}
	]`
	requestedJSON := `[{"login": "charlie", "id": 3}]`

	var reviews []github.Review
	if err := json.Unmarshal([]byte(reviewsJSON), &reviews); err != nil {
		t.Fatalf("reviews json.Unmarshal: %v", err)
	}
	var requested []github.User
	if err := json.Unmarshal([]byte(requestedJSON), &requested); err != nil {
		t.Fatalf("requested json.Unmarshal: %v", err)
	}

	got := github.MapReviewers(reviews, requested)

	if len(got) != 3 {
		t.Fatalf("len(reviewers) = %d, want 3", len(got))
	}

	// alice → Approved
	alice := findReviewer(got, "1")
	if alice == nil {
		t.Fatal("alice (id=1) not found in reviewers")
	}
	if alice.Kind != provider.VoteKindApproved {
		t.Errorf("alice.Kind = %v, want VoteKindApproved", alice.Kind)
	}
	if alice.Vote != 10 {
		t.Errorf("alice.Vote = %d, want 10 (consistent with VoteKindApproved)", alice.Vote)
	}
	if alice.DisplayName != "alice" {
		t.Errorf("alice.DisplayName = %q, want %q", alice.DisplayName, "alice")
	}

	// bob → Rejected
	bob := findReviewer(got, "2")
	if bob == nil {
		t.Fatal("bob (id=2) not found in reviewers")
	}
	if bob.Kind != provider.VoteKindRejected {
		t.Errorf("bob.Kind = %v, want VoteKindRejected", bob.Kind)
	}
	if bob.Vote != -10 {
		t.Errorf("bob.Vote = %d, want -10 (consistent with VoteKindRejected)", bob.Vote)
	}

	// charlie → NoVote (requested only)
	charlie := findReviewer(got, "3")
	if charlie == nil {
		t.Fatal("charlie (id=3) not found in reviewers")
	}
	if charlie.Kind != provider.VoteKindNoVote {
		t.Errorf("charlie.Kind = %v, want VoteKindNoVote", charlie.Kind)
	}
	if charlie.Vote != 0 {
		t.Errorf("charlie.Vote = %d, want 0", charlie.Vote)
	}
}

func TestMapReviewers_LatestReviewWins(t *testing.T) {
	// alice first commented (not a real vote), then approved. Only the latest should win.
	const reviewsJSON = `[
		{"id": 10, "state": "COMMENTED", "user": {"login": "alice", "id": 1}, "body": "looks ok", "submitted_at": "2026-05-01T08:00:00Z"},
		{"id": 11, "state": "APPROVED",  "user": {"login": "alice", "id": 1}, "body": "",          "submitted_at": "2026-05-01T09:00:00Z"}
	]`

	var reviews []github.Review
	if err := json.Unmarshal([]byte(reviewsJSON), &reviews); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapReviewers(reviews, nil)

	if len(got) != 1 {
		t.Fatalf("len(reviewers) = %d, want 1 (latest review per user)", len(got))
	}
	if got[0].Kind != provider.VoteKindApproved {
		t.Errorf("Kind = %v, want VoteKindApproved (latest submission wins)", got[0].Kind)
	}
}

func TestMapReviewers_PendingIgnored(t *testing.T) {
	// PENDING reviews must be ignored entirely.
	const reviewsJSON = `[
		{"id": 20, "state": "PENDING", "user": {"login": "dave", "id": 4}, "body": "", "submitted_at": "2026-05-01T08:00:00Z"}
	]`

	var reviews []github.Review
	if err := json.Unmarshal([]byte(reviewsJSON), &reviews); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapReviewers(reviews, nil)

	if len(got) != 0 {
		t.Errorf("len(reviewers) = %d, want 0 (PENDING should be ignored)", len(got))
	}
}

func TestMapReviewers_RequestedNotDuplicated(t *testing.T) {
	// alice already has a review; she should not appear again from requested list.
	const reviewsJSON = `[
		{"id": 30, "state": "APPROVED", "user": {"login": "alice", "id": 1}, "body": "", "submitted_at": "2026-05-01T09:00:00Z"}
	]`
	const requestedJSON = `[{"login": "alice", "id": 1}, {"login": "bob", "id": 2}]`

	var reviews []github.Review
	if err := json.Unmarshal([]byte(reviewsJSON), &reviews); err != nil {
		t.Fatalf("reviews json.Unmarshal: %v", err)
	}
	var requested []github.User
	if err := json.Unmarshal([]byte(requestedJSON), &requested); err != nil {
		t.Fatalf("requested json.Unmarshal: %v", err)
	}

	got := github.MapReviewers(reviews, requested)

	if len(got) != 2 {
		t.Fatalf("len(reviewers) = %d, want 2 (alice from review + bob from requested)", len(got))
	}

	alice := findReviewer(got, "1")
	if alice == nil {
		t.Fatal("alice not found")
	}
	if alice.Kind != provider.VoteKindApproved {
		t.Errorf("alice.Kind = %v, want VoteKindApproved", alice.Kind)
	}

	bob := findReviewer(got, "2")
	if bob == nil {
		t.Fatal("bob not found")
	}
	if bob.Kind != provider.VoteKindNoVote {
		t.Errorf("bob.Kind = %v, want VoteKindNoVote", bob.Kind)
	}
}

func TestMapReviewers_NoVoteFromCommentedState(t *testing.T) {
	// COMMENTED, DISMISSED → NoVote (not a definitive approve or reject).
	const reviewsJSON = `[
		{"id": 40, "state": "COMMENTED",  "user": {"login": "eve",   "id": 5}, "body": "nice", "submitted_at": "2026-05-01T09:00:00Z"},
		{"id": 41, "state": "DISMISSED",  "user": {"login": "frank", "id": 6}, "body": "",     "submitted_at": "2026-05-01T09:00:00Z"}
	]`

	var reviews []github.Review
	if err := json.Unmarshal([]byte(reviewsJSON), &reviews); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapReviewers(reviews, nil)

	if len(got) != 2 {
		t.Fatalf("len(reviewers) = %d, want 2", len(got))
	}
	for _, r := range got {
		if r.Kind != provider.VoteKindNoVote {
			t.Errorf("reviewer %q: Kind = %v, want VoteKindNoVote (COMMENTED/DISMISSED)", r.DisplayName, r.Kind)
		}
		if r.Vote != 0 {
			t.Errorf("reviewer %q: Vote = %d, want 0", r.DisplayName, r.Vote)
		}
	}
}

// ── MapReviewThreads ──────────────────────────────────────────────────────────

func TestMapReviewThreads_SingleThreadWithReplies(t *testing.T) {
	// Root + two replies → one thread, three comments.
	const raw = `[
		{"id": 1, "in_reply_to_id": null, "path": "src/main.go", "line": 10,   "original_line": 10, "body": "Fix this",   "user": {"login": "alice", "id": 1}, "created_at": "2026-05-01T09:00:00Z", "updated_at": "2026-05-01T09:00:00Z", "html_url": ""},
		{"id": 2, "in_reply_to_id": 1,    "path": "src/main.go", "line": null, "original_line": 10, "body": "Will fix",   "user": {"login": "bob",   "id": 2}, "created_at": "2026-05-01T09:30:00Z", "updated_at": "2026-05-01T09:30:00Z", "html_url": ""},
		{"id": 3, "in_reply_to_id": 1,    "path": "src/main.go", "line": null, "original_line": 10, "body": "Done now",   "user": {"login": "alice", "id": 1}, "created_at": "2026-05-01T10:00:00Z", "updated_at": "2026-05-01T10:00:00Z", "html_url": ""}
	]`

	var comments []github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	threads := github.MapReviewThreads(comments, testScope, testScopeDisplay)

	if len(threads) != 1 {
		t.Fatalf("len(threads) = %d, want 1", len(threads))
	}
	th := threads[0]

	// Identity
	assertIdentityInvariant(t, "Thread", th.Identity)
	if th.Identity.Kind != provider.KindGitHub {
		t.Errorf("Kind = %v, want KindGitHub", th.Identity.Kind)
	}
	if th.Identity.ID != "1" {
		t.Errorf("Identity.ID = %q, want %q", th.Identity.ID, "1")
	}

	// Thread fields
	if th.FilePath != "src/main.go" {
		t.Errorf("FilePath = %q, want %q", th.FilePath, "src/main.go")
	}
	if th.Line != 10 {
		t.Errorf("Line = %d, want 10", th.Line)
	}
	if th.Status != "active" {
		t.Errorf("Status = %q, want %q", th.Status, "active")
	}
	if th.IsDeleted {
		t.Error("IsDeleted should be false")
	}

	// Three comments in input order
	if len(th.Comments) != 3 {
		t.Fatalf("len(comments) = %d, want 3", len(th.Comments))
	}

	// Root comment: ParentCommentID = 0
	root := th.Comments[0]
	if root.Identity.ID != "1" {
		t.Errorf("root comment ID = %q, want %q", root.Identity.ID, "1")
	}
	if root.ParentCommentID != 0 {
		t.Errorf("root.ParentCommentID = %d, want 0", root.ParentCommentID)
	}
	if root.Content != "Fix this" {
		t.Errorf("root.Content = %q, want %q", root.Content, "Fix this")
	}
	if root.AuthorName != "alice" {
		t.Errorf("root.AuthorName = %q, want %q", root.AuthorName, "alice")
	}

	// Reply 1: ParentCommentID = root's ID (1)
	r1 := th.Comments[1]
	if r1.Identity.ID != "2" {
		t.Errorf("reply1 ID = %q, want %q", r1.Identity.ID, "2")
	}
	if r1.ParentCommentID != 1 {
		t.Errorf("reply1.ParentCommentID = %d, want 1", r1.ParentCommentID)
	}

	// Reply 2
	r2 := th.Comments[2]
	if r2.Identity.ID != "3" {
		t.Errorf("reply2 ID = %q, want %q", r2.Identity.ID, "3")
	}
	if r2.ParentCommentID != 1 {
		t.Errorf("reply2.ParentCommentID = %d, want 1", r2.ParentCommentID)
	}

	// LastUpdatedDate = latest comment's UpdatedAt
	wantLast := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	if !th.LastUpdatedDate.Equal(wantLast) {
		t.Errorf("LastUpdatedDate = %v, want %v", th.LastUpdatedDate, wantLast)
	}
}

func TestMapReviewThreads_TwoIndependentRoots(t *testing.T) {
	// Two root comments on different files/lines → two threads.
	const raw = `[
		{"id": 10, "in_reply_to_id": null, "path": "a.go", "line": 5,  "original_line": 5,  "body": "first",  "user": {"login": "u1", "id": 1}, "created_at": "2026-05-01T09:00:00Z", "updated_at": "2026-05-01T09:00:00Z", "html_url": ""},
		{"id": 20, "in_reply_to_id": null, "path": "b.go", "line": 15, "original_line": 15, "body": "second", "user": {"login": "u2", "id": 2}, "created_at": "2026-05-01T10:00:00Z", "updated_at": "2026-05-01T10:00:00Z", "html_url": ""}
	]`

	var comments []github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	threads := github.MapReviewThreads(comments, testScope, testScopeDisplay)

	if len(threads) != 2 {
		t.Fatalf("len(threads) = %d, want 2", len(threads))
	}

	// First thread
	if threads[0].Identity.ID != "10" {
		t.Errorf("thread[0].ID = %q, want %q", threads[0].Identity.ID, "10")
	}
	if threads[0].FilePath != "a.go" {
		t.Errorf("thread[0].FilePath = %q, want %q", threads[0].FilePath, "a.go")
	}
	if threads[0].Line != 5 {
		t.Errorf("thread[0].Line = %d, want 5", threads[0].Line)
	}
	if len(threads[0].Comments) != 1 {
		t.Errorf("thread[0] comment count = %d, want 1", len(threads[0].Comments))
	}

	// Second thread
	if threads[1].Identity.ID != "20" {
		t.Errorf("thread[1].ID = %q, want %q", threads[1].Identity.ID, "20")
	}
	if threads[1].FilePath != "b.go" {
		t.Errorf("thread[1].FilePath = %q, want %q", threads[1].FilePath, "b.go")
	}
	if threads[1].Line != 15 {
		t.Errorf("thread[1].Line = %d, want 15", threads[1].Line)
	}

	// Both threads have "active" status
	for i, th := range threads {
		if th.Status != "active" {
			t.Errorf("thread[%d].Status = %q, want %q", i, th.Status, "active")
		}
	}
}

func TestMapReviewThreads_NullLineFallsBackToOriginalLine(t *testing.T) {
	// Root comment has Line == null; should fall back to OriginalLine.
	const raw = `[
		{"id": 100, "in_reply_to_id": null, "path": "legacy.go", "line": null, "original_line": 42, "body": "outdated anchor", "user": {"login": "u", "id": 7}, "created_at": "2026-05-01T09:00:00Z", "updated_at": "2026-05-01T09:00:00Z", "html_url": ""}
	]`

	var comments []github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	threads := github.MapReviewThreads(comments, testScope, testScopeDisplay)

	if len(threads) != 1 {
		t.Fatalf("len(threads) = %d, want 1", len(threads))
	}
	if threads[0].Line != 42 {
		t.Errorf("Line = %d, want 42 (fallback from OriginalLine)", threads[0].Line)
	}
}

func TestMapReviewThreads_DefensiveNewThreadForOrphanReply(t *testing.T) {
	// A reply whose InReplyToID references a root we have not seen.
	// The reply must not be dropped; a new thread is created for it.
	const raw = `[
		{"id": 200, "in_reply_to_id": 999, "path": "orphan.go", "line": 7, "original_line": 7, "body": "reply to unknown root", "user": {"login": "orphan", "id": 8}, "created_at": "2026-05-01T09:00:00Z", "updated_at": "2026-05-01T09:00:00Z", "html_url": ""}
	]`

	var comments []github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	threads := github.MapReviewThreads(comments, testScope, testScopeDisplay)

	if len(threads) != 1 {
		t.Fatalf("len(threads) = %d, want 1 (orphan reply should form its own thread)", len(threads))
	}
	th := threads[0]

	// Thread is keyed on InReplyToID (999), not the comment's own ID (200).
	if th.Identity.ID != "999" {
		t.Errorf("defensive thread ID = %q, want %q (keyed on InReplyToID)", th.Identity.ID, "999")
	}
	// The comment itself is present.
	if len(th.Comments) != 1 {
		t.Fatalf("defensive thread comment count = %d, want 1", len(th.Comments))
	}
	if th.Comments[0].Content != "reply to unknown root" {
		t.Errorf("comment content = %q", th.Comments[0].Content)
	}
	// Identity must be valid.
	assertIdentityInvariant(t, "defensive thread", th.Identity)
	// Status still "active"
	if th.Status != "active" {
		t.Errorf("Status = %q, want %q", th.Status, "active")
	}
	// FilePath comes from the orphan comment's Path
	if th.FilePath != "orphan.go" {
		t.Errorf("FilePath = %q, want %q", th.FilePath, "orphan.go")
	}
}

func TestMapReviewThreads_ReplyBeforeRoot(t *testing.T) {
	// Input is ordered [reply(id=2, in_reply_to_id=1), root(id=1, in_reply_to_id=nil)].
	// The single-pass implementation would misgroup this: the reply is seen first and
	// becomes an orphan thread root, then the real root overwrites the map entry and
	// gets re-appended to threadOrder, producing two threads with the same ID and
	// silently dropping the reply. The two-pass implementation must produce exactly one
	// thread with the root at Comments[0] and the reply at Comments[1].
	const raw = `[
		{"id": 2, "in_reply_to_id": 1,    "path": "src/main.go", "line": null, "original_line": 5, "body": "A reply",       "user": {"login": "bob",   "id": 2}, "created_at": "2026-05-01T10:00:00Z", "updated_at": "2026-05-01T10:00:00Z", "html_url": ""},
		{"id": 1, "in_reply_to_id": null, "path": "src/main.go", "line": 5,    "original_line": 5, "body": "Root comment",  "user": {"login": "alice", "id": 1}, "created_at": "2026-05-01T09:00:00Z", "updated_at": "2026-05-01T09:00:00Z", "html_url": ""}
	]`

	var comments []github.ReviewComment
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	threads := github.MapReviewThreads(comments, testScope, testScopeDisplay)

	if len(threads) != 1 {
		t.Fatalf("len(threads) = %d, want 1 (reply-before-root must not produce duplicate threads)", len(threads))
	}
	th := threads[0]

	if th.Identity.ID != "1" {
		t.Errorf("Identity.ID = %q, want %q (thread keyed on root's ID)", th.Identity.ID, "1")
	}
	if len(th.Comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2 (root + one reply)", len(th.Comments))
	}

	// Comments[0] must be the root (id=1, parentCommentID=0).
	root := th.Comments[0]
	if root.Identity.ID != "1" {
		t.Errorf("Comments[0].Identity.ID = %q, want %q (root must be first)", root.Identity.ID, "1")
	}
	if root.ParentCommentID != 0 {
		t.Errorf("Comments[0].ParentCommentID = %d, want 0 (root has no parent)", root.ParentCommentID)
	}

	// Comments[1] must be the reply (id=2, parentCommentID=1).
	reply := th.Comments[1]
	if reply.Identity.ID != "2" {
		t.Errorf("Comments[1].Identity.ID = %q, want %q (reply must be second)", reply.Identity.ID, "2")
	}
	if reply.ParentCommentID != 1 {
		t.Errorf("Comments[1].ParentCommentID = %d, want 1 (reply's parent is root id=1)", reply.ParentCommentID)
	}
}

// ── MapPRFile ─────────────────────────────────────────────────────────────────

func TestMapPRFile_Added(t *testing.T) {
	const raw = `{"filename": "src/new.go", "status": "added", "changes": 50}`

	var f github.PRFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPRFile(f, 1)

	if got.ChangeID != 1 {
		t.Errorf("ChangeID = %d, want 1", got.ChangeID)
	}
	if got.Path != "src/new.go" {
		t.Errorf("Path = %q, want %q", got.Path, "src/new.go")
	}
	if got.GitObjectType != "blob" {
		t.Errorf("GitObjectType = %q, want %q", got.GitObjectType, "blob")
	}
	if got.ChangeType != "add" {
		t.Errorf("ChangeType = %q, want %q", got.ChangeType, "add")
	}
	if got.OriginalPath != "" {
		t.Errorf("OriginalPath = %q, want empty for added file", got.OriginalPath)
	}
}

func TestMapPRFile_Removed(t *testing.T) {
	const raw = `{"filename": "src/old.go", "status": "removed", "changes": 20}`

	var f github.PRFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPRFile(f, 2)

	if got.ChangeType != "delete" {
		t.Errorf("ChangeType = %q, want %q", got.ChangeType, "delete")
	}
}

func TestMapPRFile_Modified(t *testing.T) {
	const raw = `{"filename": "src/main.go", "status": "modified", "changes": 10}`

	var f github.PRFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPRFile(f, 3)

	if got.ChangeType != "edit" {
		t.Errorf("ChangeType = %q, want %q", got.ChangeType, "edit")
	}
}

func TestMapPRFile_Renamed(t *testing.T) {
	const raw = `{"filename": "src/renamed.go", "status": "renamed", "previous_filename": "src/original.go", "changes": 5}`

	var f github.PRFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapPRFile(f, 4)

	if got.ChangeType != "rename" {
		t.Errorf("ChangeType = %q, want %q", got.ChangeType, "rename")
	}
	if got.Path != "src/renamed.go" {
		t.Errorf("Path = %q, want %q", got.Path, "src/renamed.go")
	}
	if got.OriginalPath != "src/original.go" {
		t.Errorf("OriginalPath = %q, want %q", got.OriginalPath, "src/original.go")
	}
}

func TestMapPRFile_ChangeID_Sequential(t *testing.T) {
	// Caller passes index+1; verify it round-trips.
	const raw = `{"filename": "x.go", "status": "modified", "changes": 1}`

	var f github.PRFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for i := 1; i <= 5; i++ {
		got := github.MapPRFile(f, i)
		if got.ChangeID != i {
			t.Errorf("ChangeID = %d, want %d", got.ChangeID, i)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// findReviewer returns the reviewer with the given ID string, or nil.
func findReviewer(reviewers []provider.Reviewer, id string) *provider.Reviewer {
	for i := range reviewers {
		if reviewers[i].ID == id {
			return &reviewers[i]
		}
	}
	return nil
}
