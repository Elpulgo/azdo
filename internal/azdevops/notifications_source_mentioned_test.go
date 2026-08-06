package azdevops

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
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
// request order — TestSourceMentioned_CandidateFanOutBoundedAndLogged uses
// this to prove the bound is enforced by actual HTTP call count, not merely
// by the returned row count.
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

// captureLogs redirects the slog default logger to a buffer for the
// duration of the test and restores it on cleanup.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
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
	rows, err := SourceMentioned(mc, now)
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
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
	rows, err := SourceMentioned(mc, now)
	if err != nil {
		t.Fatalf("SourceMentioned (first poll) failed: %v", err)
	}
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
	rows2, err := SourceMentioned(mc2, now2)
	if err != nil {
		t.Fatalf("SourceMentioned (second poll) failed: %v", err)
	}
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

	rows, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected stage 1's over-match to be filtered out by stage 2, got %d rows", len(rows))
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

	rows, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
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

	rows, err := SourceMentioned(mc, time.Now())
	if err == nil {
		t.Fatal("expected error when every project fails stage 1")
	}
	var partialErr *PartialError
	if errors.As(err, &partialErr) {
		t.Fatalf("expected a plain error (nothing survived to report partially), got a *PartialError: %v", err)
	}
	if rows != nil {
		t.Fatalf("expected nil rows when every project fails, got %d", len(rows))
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

	rows, err := SourceMentioned(mc, time.Now())
	if err == nil {
		t.Fatal("expected a *PartialError when one project fails stage 1")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a *PartialError, got: %v", err)
	}
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

	rows, err := SourceMentioned(mc, time.Now())
	if err == nil {
		t.Fatal("expected a *PartialError when one candidate's comments fetch fails")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a *PartialError, got: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the surviving candidate's row, got %d rows", len(rows))
	}
	if rows[0].Identity.ID != "mention/wi/8" {
		t.Errorf("Identity.ID = %q, want %q", rows[0].Identity.ID, "mention/wi/8")
	}
}

// --- Bounded fan-out ---

// TestSourceMentioned_CandidateFanOutBoundedAndLogged pins the spec's bound
// requirement: stage 2 caps at mentionCandidateLimit candidates merged
// across every project, and the truncation is logged (never silent).
// alpha's 30 lower-numbered ids and beta's 30 higher-numbered ids merge to
// 60, over the 50 limit; boundMentionCandidates sorts by id, so the 20
// dropped must be beta's ids 51-60 specifically — asserted via each
// project's own comments-request hit count, which proves the bound by
// actual HTTP call volume rather than only by the returned row count.
func TestSourceMentioned_CandidateFanOutBoundedAndLogged(t *testing.T) {
	const alphaCount = 30
	const betaCount = 30 // 60 total > mentionCandidateLimit (50)

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

	logBuf := captureLogs(t)

	rows, err := SourceMentioned(mc, time.Now())
	if err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (no fixture comment mentions the user), got %d", len(rows))
	}

	alphaHits := alphaFixture.hitCount()
	betaHits := betaFixture.hitCount()

	if alphaHits+betaHits != mentionCandidateLimit {
		t.Fatalf("expected exactly %d total comments fetches across projects (the bound), got %d (alpha=%d, beta=%d)",
			mentionCandidateLimit, alphaHits+betaHits, alphaHits, betaHits)
	}
	if alphaHits != alphaCount {
		t.Errorf("expected all %d of alpha's lower-numbered ids to survive the id-ascending cap, got %d", alphaCount, alphaHits)
	}
	if betaHits != mentionCandidateLimit-alphaCount {
		t.Errorf("expected only the lowest %d of beta's ids to survive the cap, got %d", mentionCandidateLimit-alphaCount, betaHits)
	}

	logged := logBuf.String()
	if !strings.Contains(logged, "truncated") {
		t.Fatalf("expected a log line when candidate fan-out is truncated, got: %q", logged)
	}
	if !strings.Contains(logged, "candidates_before_truncation=60") {
		t.Errorf("expected the log line to record the pre-truncation candidate count (60), got: %q", logged)
	}
	if !strings.Contains(logged, "candidates_processed=50") {
		t.Errorf("expected the log line to record the post-truncation count (50), got: %q", logged)
	}
}

func TestSourceMentioned_BelowCandidateLimit_NoTruncationLog(t *testing.T) {
	fixture := newSequentialMentionFixture(1, 5)
	server := newMentionServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	logBuf := captureLogs(t)

	if _, err := SourceMentioned(mc, time.Now()); err != nil {
		t.Fatalf("SourceMentioned failed: %v", err)
	}

	if logBuf.Len() != 0 {
		t.Errorf("expected no log output below the candidate limit, got: %q", logBuf.String())
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
	for i, want := range []int{1, 2, 3} {
		if bounded[i].ID != want {
			t.Errorf("bounded[%d].ID = %d, want %d (must be sorted ascending)", i, bounded[i].ID, want)
		}
	}
}

func TestBoundMentionCandidates_OverLimit_TruncatesToLowestIDs(t *testing.T) {
	candidates := make([]WorkItem, mentionCandidateLimit+10)
	for i := range candidates {
		// Deliberately reversed insertion order so a correct implementation
		// must sort, not merely slice, to keep the lowest ids.
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
	if bounded[0].ID != 1 || bounded[len(bounded)-1].ID != mentionCandidateLimit {
		t.Errorf("bounded ids = [%d..%d], want [1..%d]", bounded[0].ID, bounded[len(bounded)-1].ID, mentionCandidateLimit)
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
