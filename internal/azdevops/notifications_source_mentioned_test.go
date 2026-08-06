package azdevops

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// mentionServerFixture configures a per-project httptest server backing all
// three requests SourceMentioned's two-stage pipeline can issue against a
// single project client:
//   - stage 1's WIQL ids response (ids)
//   - stage 1's bulk GetWorkItems response, keyed by id (items)
//   - stage 2's per-candidate comments response, keyed by id (comments)
//
// commentHits records every id a comments request actually arrived for, in
// request order — TestSourceMentioned_CandidateFanOutBounded_InterleavesAcrossProjects
// uses this to prove the bound is enforced by actual HTTP call count, not
// merely by the returned row count.
type mentionServerFixture struct {
	ids                []int
	items              map[int]WorkItem
	comments           map[int][]WorkItemComment
	failCommentsForIDs map[int]bool

	mu          sync.Mutex
	commentHits []int
}

var commentsPathRe = regexp.MustCompile(`/workItems/(-?\d+)/comments`)

func newMentionServer(t *testing.T, f *mentionServerFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if m := commentsPathRe.FindStringSubmatch(path); m != nil {
			id, _ := strconv.Atoi(m[1])
			f.mu.Lock()
			f.commentHits = append(f.commentHits, id)
			f.mu.Unlock()

			if f.failCommentsForIDs[id] {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"message":"boom"}`))
				return
			}

			resp := struct {
				TotalCount int               `json:"totalCount"`
				Count      int               `json:"count"`
				Comments   []WorkItemComment `json:"comments"`
			}{Comments: f.comments[id]}
			resp.Count = len(resp.Comments)
			resp.TotalCount = resp.Count
			json.NewEncoder(w).Encode(resp)
			return
		}

		if strings.Contains(path, "/wit/wiql") {
			refs := make([]WorkItemReference, len(f.ids))
			for i, id := range f.ids {
				refs[i] = WorkItemReference{ID: id}
			}
			resp := struct {
				WorkItems []WorkItemReference `json:"workItems"`
			}{WorkItems: refs}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Stage 1's bulk GetWorkItems.
		value := make([]WorkItem, 0, len(f.ids))
		for _, id := range f.ids {
			if wi, ok := f.items[id]; ok {
				value = append(value, wi)
			}
		}
		resp := struct {
			Value []WorkItem `json:"value"`
		}{Value: value}
		json.NewEncoder(w).Encode(resp)
	}))
}

// newSequentialMentionFixture builds a fixture of `count` candidates with ids
// start..start+count-1, each carrying a title but no comments (so none ever
// match) — used purely to exercise stage 1's fan-out volume for the
// candidate-bound tests, where the row count is deliberately uninteresting.
func newSequentialMentionFixture(start, count int) *mentionServerFixture {
	f := &mentionServerFixture{
		ids:      make([]int, count),
		items:    make(map[int]WorkItem, count),
		comments: make(map[int][]WorkItemComment, count),
	}
	for i := 0; i < count; i++ {
		id := start + i
		f.ids[i] = id
		f.items[id] = WorkItem{ID: id, Fields: WorkItemFields{Title: "Untitled"}}
	}
	return f
}

func (f *mentionServerFixture) hitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.commentHits)
}

// --- SourceMentioned: matching and stamping ---

func TestSourceMentioned_MatchesAndStampsFromComment(t *testing.T) {
	changedDate := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	mentionDate := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

	fixture := &mentionServerFixture{
		ids: []int{42},
		items: map[int]WorkItem{
			42: {ID: 42, Fields: WorkItemFields{Title: "Fix the widget", ChangedDate: changedDate}},
		},
		comments: map[int][]WorkItemComment{
			42: {{ID: 1, CreatedDate: mentionDate, Mentions: []CommentMention{{TargetID: "user-1"}}}},
		},
	}
	server := newMentionServer(t, fixture)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	now := changedDate.Add(time.Hour)
	result, err := SourceMentioned(mc, now)
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
	rows := result.Rows
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row.Identity.Kind != provider.KindAzure {
		t.Errorf("Identity.Kind = %v, want KindAzure", row.Identity.Kind)
	}
	if row.Identity.Scope != "alpha" {
		t.Errorf("Identity.Scope = %q, want %q", row.Identity.Scope, "alpha")
	}
	if row.Identity.ID != "mention/wi/42" {
		t.Errorf("Identity.ID = %q, want %q", row.Identity.ID, "mention/wi/42")
	}
	if row.Title != "Fix the widget" {
		t.Errorf("Title = %q, want %q", row.Title, "Fix the widget")
	}
	if row.Reason != provider.NotificationReasonMentioned {
		t.Errorf("Reason = %v, want NotificationReasonMentioned", row.Reason)
	}
	if !row.UpdatedAt.Equal(mentionDate) {
		t.Errorf("UpdatedAt = %v, want the mention's createdDate %v (not ChangedDate %v)", row.UpdatedAt, mentionDate, changedDate)
	}
}

// TestSourceMentioned_UnrelatedEditAfterMention_DoesNotResurrectDismissedRow
// pins the exact spec requirement: the activity stamp is the newest
// *matching* comment's createdDate, not the work item's ChangedDate — an
// unrelated edit after the mention must not advance the stamp, which is the
// entire reason stage 2 exists. It proves this end to end through Reconcile:
// a dismissed (done) row must stay dismissed across a poll where only
// ChangedDate moved and no new matching comment appeared. If SourceMentioned
// used wi.Fields.ChangedDate instead, the second poll's stamp would be newer
// than the stored LastActivity and Reconcile would resurrect the row.
func TestSourceMentioned_UnrelatedEditAfterMention_DoesNotResurrectDismissedRow(t *testing.T) {
	mentionDate := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	initialChanged := time.Date(2026, 1, 15, 9, 5, 0, 0, time.UTC)
	comments := map[int][]WorkItemComment{
		42: {{ID: 1, CreatedDate: mentionDate, Mentions: []CommentMention{{TargetID: "user-1"}}}},
	}

	firstFixture := &mentionServerFixture{
		ids:      []int{42},
		items:    map[int]WorkItem{42: {ID: 42, Fields: WorkItemFields{Title: "Fix widget", ChangedDate: initialChanged}}},
		comments: comments,
	}
	firstServer := newMentionServer(t, firstFixture)
	defer firstServer.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": firstServer})
	setUserIDs(mc, "user-1")

	now := initialChanged.Add(time.Hour)
	firstResult, err := SourceMentioned(mc, now)
	if err != nil {
		t.Fatalf("SourceMentioned (first poll) failed: %v", err)
	}
	rows := firstResult.Rows
	if len(rows) != 1 {
		t.Fatalf("expected 1 row from the first poll, got %d", len(rows))
	}

	rows, state := Reconcile(rows, TriageState{}, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first reconcile, got %d", len(rows))
	}
	entry := state[rows[0].Identity.ID]
	entry.Done = true
	state[rows[0].Identity.ID] = entry

	// Second poll: the work item is edited (ChangedDate advances well past
	// the mention) but no new comment is posted.
	laterChanged := initialChanged.Add(24 * time.Hour)
	secondFixture := &mentionServerFixture{
		ids:      []int{42},
		items:    map[int]WorkItem{42: {ID: 42, Fields: WorkItemFields{Title: "Fix widget", ChangedDate: laterChanged}}},
		comments: comments, // unchanged — no new mention
	}
	secondServer := newMentionServer(t, secondFixture)
	defer secondServer.Close()
	mc2 := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": secondServer})
	setUserIDs(mc2, "user-1")

	now2 := laterChanged.Add(time.Minute)
	secondResult, err := SourceMentioned(mc2, now2)
	if err != nil {
		t.Fatalf("SourceMentioned (second poll) failed: %v", err)
	}
	rows2 := secondResult.Rows
	if len(rows2) != 1 {
		t.Fatalf("expected 1 row from the second poll, got %d", len(rows2))
	}
	if !rows2[0].UpdatedAt.Equal(mentionDate) {
		t.Fatalf("second poll's UpdatedAt = %v, want it still pinned to the mention's createdDate %v (ChangedDate %v must not leak in)",
			rows2[0].UpdatedAt, mentionDate, laterChanged)
	}

	rows3, _ := Reconcile(rows2, state, now2)
	if len(rows3) != 0 {
		t.Fatalf("expected the dismissed row to stay dropped after an unrelated edit with no new mention, got %d rows", len(rows3))
	}
}

func TestSourceMentioned_NonMatchingTargetID_Excluded(t *testing.T) {
	fixture := &mentionServerFixture{
		ids:   []int{42},
		items: map[int]WorkItem{42: {ID: 42, Fields: WorkItemFields{Title: "Fix widget"}}},
		comments: map[int][]WorkItemComment{
			42: {{ID: 1, CreatedDate: time.Now(), Mentions: []CommentMention{{TargetID: "someone-else"}}}},
		},
	}
	server := newMentionServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("expected stage 1's over-match to be filtered out by stage 2, got %d rows", len(result.Rows))
	}
}

func TestSourceMentioned_NegativeID_ProducesEmptyIdentityID(t *testing.T) {
	fixture := &mentionServerFixture{
		ids:   []int{-1},
		items: map[int]WorkItem{-1: {ID: -1, Fields: WorkItemFields{Title: "Broken"}}},
		comments: map[int][]WorkItemComment{
			-1: {{ID: 1, CreatedDate: time.Now(), Mentions: []CommentMention{{TargetID: "user-1"}}}},
		},
	}
	server := newMentionServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
	rows := result.Rows
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Identity.ID != "" {
		t.Errorf("Identity.ID = %q, want empty for a non-positive work item id", rows[0].Identity.ID)
	}
}

func TestSourceMentioned_NilMultiClient_ReturnsError(t *testing.T) {
	_, err := SourceMentioned(nil, time.Now())
	if err == nil {
		t.Fatal("expected error for nil MultiClient")
	}
}

// --- Partial failure ---

func TestSourceMentioned_AllProjectsFailStage1_ReturnsPlainError(t *testing.T) {
	errServer := newErrorServer(t)
	defer errServer.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": errServer})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err == nil {
		t.Fatal("expected error when every project fails stage 1")
	}
	var partialErr *PartialError
	if errors.As(err, &partialErr) {
		t.Fatalf("expected a plain error (nothing survived to report partially), got a *PartialError: %v", err)
	}
	if result.Rows != nil {
		t.Fatalf("expected nil rows when every project fails, got %d", len(result.Rows))
	}
}

func TestSourceMentioned_PartialStage1Failure_ReturnsSurvivingRows(t *testing.T) {
	errServer := newErrorServer(t)
	defer errServer.Close()

	fixture := &mentionServerFixture{
		ids:   []int{7},
		items: map[int]WorkItem{7: {ID: 7, Fields: WorkItemFields{Title: "Survivor"}}},
		comments: map[int][]WorkItemComment{
			7: {{ID: 1, CreatedDate: time.Now(), Mentions: []CommentMention{{TargetID: "user-1"}}}},
		},
	}
	okServer := newMentionServer(t, fixture)
	defer okServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": errServer,
		"beta":  okServer,
	})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err == nil {
		t.Fatal("expected a *PartialError when one project fails stage 1")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a *PartialError, got: %v", err)
	}
	rows := result.Rows
	if len(rows) != 1 {
		t.Fatalf("expected the surviving project's confirmed mention, got %d rows", len(rows))
	}
	if rows[0].Identity.Scope != "beta" {
		t.Errorf("Identity.Scope = %q, want %q", rows[0].Identity.Scope, "beta")
	}
}

func TestSourceMentioned_PartialStage2Failure_ReturnsSurvivingRows(t *testing.T) {
	fixture := &mentionServerFixture{
		ids: []int{7, 8},
		items: map[int]WorkItem{
			7: {ID: 7, Fields: WorkItemFields{Title: "Fails"}},
			8: {ID: 8, Fields: WorkItemFields{Title: "Survives"}},
		},
		comments: map[int][]WorkItemComment{
			8: {{ID: 1, CreatedDate: time.Now(), Mentions: []CommentMention{{TargetID: "user-1"}}}},
		},
		failCommentsForIDs: map[int]bool{7: true},
	}
	server := newMentionServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err == nil {
		t.Fatal("expected a *PartialError when one candidate's comments fetch fails")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a *PartialError, got: %v", err)
	}
	rows := result.Rows
	if len(rows) != 1 {
		t.Fatalf("expected the surviving candidate's row, got %d rows", len(rows))
	}
	if rows[0].Identity.ID != "mention/wi/8" {
		t.Errorf("Identity.ID = %q, want %q", rows[0].Identity.ID, "mention/wi/8")
	}
}

// --- Bounded fan-out ---

// TestSourceMentioned_CandidateFanOutBounded_InterleavesAcrossProjects pins
// the spec's bound requirement: stage 2 caps at mentionCandidateLimit
// candidates merged across every project, and the truncation is surfaced as
// structured data on the returned SourceMentionedResult (never silent, never
// logged — this repo has no logging facility). alpha's and beta's 30
// candidates each merge to 60, over the 50 limit; the merge round-robins
// alphabetically by project ("alpha" before "beta"), so with equal-sized
// per-project lists the cap lands exactly halfway through each — asserted
// via each project's own comments-request hit count, which proves the bound
// by actual HTTP call volume rather than only by the returned row count.
func TestSourceMentioned_CandidateFanOutBounded_InterleavesAcrossProjects(t *testing.T) {
	const alphaCount = 30
	const betaCount = 30 // 60 total > mentionCandidateLimit (50)
	const wantPerProject = mentionCandidateLimit / 2

	alphaFixture := newSequentialMentionFixture(1, alphaCount) // ids 1..30
	betaFixture := newSequentialMentionFixture(31, betaCount)  // ids 31..60

	alphaServer := newMentionServer(t, alphaFixture)
	defer alphaServer.Close()
	betaServer := newMentionServer(t, betaFixture)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("expected 0 rows (no fixture comment mentions the user), got %d", len(result.Rows))
	}
	if result.CandidateLimit != mentionCandidateLimit {
		t.Errorf("CandidateLimit = %d, want %d", result.CandidateLimit, mentionCandidateLimit)
	}
	if result.CandidatesDropped != alphaCount+betaCount-mentionCandidateLimit {
		t.Errorf("CandidatesDropped = %d, want %d", result.CandidatesDropped, alphaCount+betaCount-mentionCandidateLimit)
	}

	alphaHits := alphaFixture.hitCount()
	betaHits := betaFixture.hitCount()

	if alphaHits+betaHits != mentionCandidateLimit {
		t.Fatalf("expected exactly %d total comments fetches across projects (the bound), got %d (alpha=%d, beta=%d)",
			mentionCandidateLimit, alphaHits+betaHits, alphaHits, betaHits)
	}
	if alphaHits != wantPerProject {
		t.Errorf("expected round-robin to give alpha %d of its candidates, got %d", wantPerProject, alphaHits)
	}
	if betaHits != wantPerProject {
		t.Errorf("expected round-robin to give beta %d of its candidates (fair split, not crowded out), got %d", wantPerProject, betaHits)
	}
}

func TestSourceMentioned_BelowCandidateLimit_NoCandidatesDropped(t *testing.T) {
	fixture := newSequentialMentionFixture(1, 5)
	server := newMentionServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}

	if result.CandidatesDropped != 0 {
		t.Errorf("CandidatesDropped = %d, want 0 below the candidate limit", result.CandidatesDropped)
	}
	if result.CandidateLimit != mentionCandidateLimit {
		t.Errorf("CandidateLimit = %d, want %d (always populated, even without truncation)", result.CandidateLimit, mentionCandidateLimit)
	}
}

// TestSourceMentioned_LowIDRecentMentionSurvivesHighIDStaleTruncation proves
// the fix for defect (a): the candidate bound is a function of stage 1's
// query order (ChangedDate DESC), never of the work-item id itself. Azure
// DevOps ids are assigned at creation and never reused, so a low id can
// belong to a work item created long ago that just received a brand-new
// mention (query order: near the front) while a high id can belong to a
// work item created yesterday whose only mention is already stale (query
// order: near the back). Sorting the merged candidates by id — ascending
// (the original defect) or descending (the naive "fix") — gets this
// backwards either way; only preserving query order and truncating the tail
// gets it right.
func TestSourceMentioned_LowIDRecentMentionSurvivesHighIDStaleTruncation(t *testing.T) {
	const total = mentionCandidateLimit + 1 // exactly one candidate must be dropped
	const recentLowID = 5
	const staleHighID = 9999

	fixture := &mentionServerFixture{
		ids:      make([]int, total),
		items:    make(map[int]WorkItem, total),
		comments: make(map[int][]WorkItemComment, total),
	}

	// Position 0: the low, recently-mentioned id — first in stage 1's
	// ChangedDate-DESC order.
	fixture.ids[0] = recentLowID
	fixture.items[recentLowID] = WorkItem{ID: recentLowID, Fields: WorkItemFields{Title: "Old item, new mention"}}
	fixture.comments[recentLowID] = []WorkItemComment{
		{ID: 1, CreatedDate: time.Now(), Mentions: []CommentMention{{TargetID: "user-1"}}},
	}

	// Middle filler, positions 1..total-2: ids chosen between the two so an
	// id-based sort in either direction would reorder everything relative to
	// array position. None of these ever match, they exist purely to pad the
	// fan-out past the cap.
	for i := 1; i < total-1; i++ {
		id := 100 + i
		fixture.ids[i] = id
		fixture.items[id] = WorkItem{ID: id, Fields: WorkItemFields{Title: "Untitled"}}
	}

	// Last position: the high, stale id — last in stage 1's order, past the
	// cap once the filler pushes the total over mentionCandidateLimit.
	fixture.ids[total-1] = staleHighID
	fixture.items[staleHighID] = WorkItem{ID: staleHighID, Fields: WorkItemFields{Title: "New item, stale mention"}}
	fixture.comments[staleHighID] = []WorkItemComment{
		{ID: 1, CreatedDate: time.Now(), Mentions: []CommentMention{{TargetID: "user-1"}}},
	}

	server := newMentionServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	result, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}

	if result.CandidatesDropped != 1 {
		t.Fatalf("CandidatesDropped = %d, want 1", result.CandidatesDropped)
	}

	var sawLow, sawHigh bool
	for _, row := range result.Rows {
		switch row.Identity.ID {
		case NotifKey("mention", "wi", recentLowID):
			sawLow = true
		case NotifKey("mention", "wi", staleHighID):
			sawHigh = true
		}
	}
	if !sawLow {
		t.Error("expected the low-id, recently-mentioned work item to survive truncation")
	}
	if sawHigh {
		t.Error("expected the high-id, stale-mention work item to be dropped by truncation")
	}
}

// --- interleaveMentionCandidates ---

func TestInterleaveMentionCandidates_RoundRobinsAcrossProjects(t *testing.T) {
	byProject := map[string][]WorkItem{
		"alpha": {{ID: 1}, {ID: 2}, {ID: 3}},
		"beta":  {{ID: 10}, {ID: 20}},
	}

	got := interleaveMentionCandidates(byProject)

	want := []int{1, 10, 2, 20, 3}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("got[%d].ID = %d, want %d", i, got[i].ID, id)
		}
	}
}

func TestInterleaveMentionCandidates_DeterministicProjectOrder(t *testing.T) {
	byProject := map[string][]WorkItem{
		"zeta":  {{ID: 100}},
		"alpha": {{ID: 1}},
	}

	// Map iteration order is randomized per run, so repeat enough times to
	// catch a non-deterministic implementation.
	for i := 0; i < 20; i++ {
		got := interleaveMentionCandidates(byProject)
		if len(got) != 2 || got[0].ID != 1 || got[1].ID != 100 {
			t.Fatalf("run %d: got %v, want [alpha's 1, zeta's 100] in that order every time", i, got)
		}
	}
}

// --- boundMentionCandidates ---

func TestBoundMentionCandidates_WithinLimit_NoTruncation(t *testing.T) {
	candidates := []WorkItem{{ID: 3}, {ID: 1}, {ID: 2}}
	bounded, truncated, total := boundMentionCandidates(candidates)

	if truncated {
		t.Error("truncated = true, want false")
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(bounded) != 3 {
		t.Fatalf("expected all 3 candidates, got %d", len(bounded))
	}
	for i, want := range []int{3, 1, 2} {
		if bounded[i].ID != want {
			t.Errorf("bounded[%d].ID = %d, want %d (order must be preserved, not re-sorted)", i, bounded[i].ID, want)
		}
	}
}

func TestBoundMentionCandidates_OverLimit_KeepsLeadingCandidatesInOrder(t *testing.T) {
	candidates := make([]WorkItem, mentionCandidateLimit+10)
	for i := range candidates {
		// Ids deliberately run opposite to position (high id first, low id
		// last) so this only passes if boundMentionCandidates keeps the
		// leading elements positionally rather than re-sorting by id in
		// either direction.
		candidates[i] = WorkItem{ID: len(candidates) - i}
	}

	bounded, truncated, total := boundMentionCandidates(candidates)
	if !truncated {
		t.Fatal("truncated = false, want true")
	}
	if total != mentionCandidateLimit+10 {
		t.Errorf("total = %d, want %d", total, mentionCandidateLimit+10)
	}
	if len(bounded) != mentionCandidateLimit {
		t.Fatalf("len(bounded) = %d, want %d", len(bounded), mentionCandidateLimit)
	}
	for i := range bounded {
		if bounded[i].ID != candidates[i].ID {
			t.Fatalf("bounded[%d].ID = %d, want %d (leading elements must survive in original order, not re-sorted)",
				i, bounded[i].ID, candidates[i].ID)
		}
	}
}

// --- newestMatchingCommentStamp ---

func TestNewestMatchingCommentStamp(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	middle := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		comments  []WorkItemComment
		userID    string
		wantFound bool
		wantStamp time.Time
	}{
		{
			name:      "no comments",
			comments:  nil,
			userID:    "user-1",
			wantFound: false,
		},
		{
			name: "no matching mention",
			comments: []WorkItemComment{
				{CreatedDate: older, Mentions: []CommentMention{{TargetID: "someone-else"}}},
			},
			userID:    "user-1",
			wantFound: false,
		},
		{
			name: "single match",
			comments: []WorkItemComment{
				{CreatedDate: middle, Mentions: []CommentMention{{TargetID: "user-1"}}},
			},
			userID:    "user-1",
			wantFound: true,
			wantStamp: middle,
		},
		{
			name: "multiple matches out of order: picks the newest",
			comments: []WorkItemComment{
				{CreatedDate: older, Mentions: []CommentMention{{TargetID: "user-1"}}},
				{CreatedDate: newest, Mentions: []CommentMention{{TargetID: "user-1"}}},
				{CreatedDate: middle, Mentions: []CommentMention{{TargetID: "user-1"}}},
			},
			userID:    "user-1",
			wantFound: true,
			wantStamp: newest,
		},
		{
			name: "one comment mentions several users, one of them matches",
			comments: []WorkItemComment{
				{CreatedDate: older, Mentions: []CommentMention{{TargetID: "other-1"}, {TargetID: "user-1"}, {TargetID: "other-2"}}},
			},
			userID:    "user-1",
			wantFound: true,
			wantStamp: older,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stamp, found := newestMatchingCommentStamp(tt.comments, tt.userID)
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if found && !stamp.Equal(tt.wantStamp) {
				t.Errorf("stamp = %v, want %v", stamp, tt.wantStamp)
			}
		})
	}
}

// --- mapMentioned ---

func TestMapMentioned_ClampsFutureStamp(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)

	wi := WorkItem{ID: 42, ProjectName: "alpha", Fields: WorkItemFields{Title: "Widget"}}
	server := newPRServer(t, nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	row := mapMentioned(mc, wi, future, now)
	if !row.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want it clamped to now (%v), not the future stamp %v", row.UpdatedAt, now, future)
	}
}

func TestMapMentioned_WebURLIsLegalEmptyString(t *testing.T) {
	server := newPRServer(t, nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	wi := WorkItem{ID: 42, ProjectName: "does-not-exist", Fields: WorkItemFields{Title: "Widget"}}
	row := mapMentioned(mc, wi, time.Now(), time.Now())
	if row.WebURL != "" {
		t.Errorf("WebURL = %q, want empty string (legal degraded result)", row.WebURL)
	}
}

// --- mentionedWebURL degradation ladder ---

func TestMentionedWebURL_DegradationLadder(t *testing.T) {
	server := newPRServer(t, nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	tests := []struct {
		name string
		wi   WorkItem
		want string
	}{
		{
			name: "resolvable scope and positive id: deep link",
			wi:   WorkItem{ID: 42, ProjectName: "alpha"},
			want: "https://dev.azure.com/testorg/alpha/_workitems/edit/42",
		},
		{
			name: "non-positive id: falls back to project page",
			wi:   WorkItem{ID: 0, ProjectName: "alpha"},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "negative id: falls back to project page",
			wi:   WorkItem{ID: -5, ProjectName: "alpha"},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "unresolvable scope: empty",
			wi:   WorkItem{ID: 42, ProjectName: "unknown-project"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mentionedWebURL(mc, tt.wi)
			if got != tt.want {
				t.Errorf("mentionedWebURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMentionedWebURL_NilMultiClient(t *testing.T) {
	got := mentionedWebURL(nil, WorkItem{ID: 42, ProjectName: "alpha"})
	if got != "" {
		t.Errorf("mentionedWebURL(nil, ...) = %q, want empty", got)
	}
}
