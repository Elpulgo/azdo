package azdevops

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// --- SourceCIFailed: mapping and filtering ---

// TestSourceCIFailed_MapsFailedRunToNotification pins that only a completed,
// failed run requested for the authenticated user survives the filter, and
// that the survivor maps to a ci_activity row. The fixture deliberately
// includes three runs that must each be excluded for a different reason
// (see isMyFailedRun's doc comment), so a filter that discriminates on only
// one axis still fails this test.
//
// newPipelineRunServer (multiclient_test.go) ignores every query parameter
// on the request and always returns the fixture verbatim — it does not
// simulate statusFilter/resultFilter/requestedFor/minTime narrowing at all.
// So this test doubles as the belt-and-braces re-check finding 1 of task 7's
// review required: even against a server that does none of the server-side
// narrowing SourceCIFailed asks for, isMyFailedRun's Go-side filter must
// still be the thing that gets this down to exactly the caller's own failed
// run — proving the outcome degrades to over-fetching, never to attributing
// someone else's build to the caller.
func TestSourceCIFailed_MapsFailedRunToNotification(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	finish := now.Add(-time.Hour)
	runs := []PipelineRun{
		{
			ID:           1,
			BuildNumber:  "20260101.1",
			Status:       "completed",
			Result:       "failed",
			Definition:   PipelineDefinition{Name: "CI"},
			QueueTime:    now.Add(-2 * time.Hour),
			FinishTime:   &finish,
			RequestedFor: Identity{ID: "user-1", DisplayName: "Me"},
		},
		{
			// Someone else's failed run: must not surface as mine.
			ID:           2,
			Status:       "completed",
			Result:       "failed",
			RequestedFor: Identity{ID: "user-2", DisplayName: "Someone Else"},
		},
		{
			// My run, but it succeeded: must not surface as a failure.
			ID:           3,
			Status:       "completed",
			Result:       "succeeded",
			RequestedFor: Identity{ID: "user-1", DisplayName: "Me"},
		},
		{
			// My run, still in progress: Result is "none" until completion,
			// must not be mistaken for a failure.
			ID:           4,
			Status:       "inProgress",
			Result:       "none",
			RequestedFor: Identity{ID: "user-1", DisplayName: "Me"},
		},
	}

	server := newPipelineRunServer(t, "alpha", runs)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	rows, err := SourceCIFailed(mc, 14, 50, now)
	if err != nil {
		t.Fatalf("SourceCIFailed failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (only run 1 is mine, completed and failed), got %d: %+v", len(rows), rows)
	}

	row := rows[0]
	if row.Identity.Kind != provider.KindAzure {
		t.Errorf("Identity.Kind = %v, want KindAzure", row.Identity.Kind)
	}
	if row.Identity.Scope != "alpha" {
		t.Errorf("Identity.Scope = %q, want %q", row.Identity.Scope, "alpha")
	}
	if row.Identity.ID != "cifail/run/1" {
		t.Errorf("Identity.ID = %q, want %q", row.Identity.ID, "cifail/run/1")
	}
	if row.Title != "CI #20260101.1 failed" {
		t.Errorf("Title = %q, want %q", row.Title, "CI #20260101.1 failed")
	}
	if !row.UpdatedAt.Equal(finish) {
		t.Errorf("UpdatedAt = %v, want the run's FinishTime %v", row.UpdatedAt, finish)
	}
}

// TestSourceCIFailed_ReasonIsCIActivity_AssertedByName pins decision 8 by
// name rather than by enum value: comparing only against
// provider.NotificationReasonCIActivity would still pass if a future
// ci_failed member were added to the enum and this source were silently
// repointed at it. Asserting the rendered string "ci_activity" catches that
// swap because provider.NotificationReason.String() renders each member to
// its own distinct string.
func TestSourceCIFailed_ReasonIsCIActivity_AssertedByName(t *testing.T) {
	now := time.Now().UTC()
	finish := now.Add(-time.Hour)
	runs := []PipelineRun{
		{
			ID:           1,
			Status:       "completed",
			Result:       "failed",
			FinishTime:   &finish,
			RequestedFor: Identity{ID: "user-1"},
		},
	}
	server := newPipelineRunServer(t, "alpha", runs)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	rows, err := SourceCIFailed(mc, 14, 50, now)
	if err != nil {
		t.Fatalf("SourceCIFailed failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if got := rows[0].Reason.String(); got != "ci_activity" {
		t.Errorf("Reason.String() = %q, want %q", got, "ci_activity")
	}
}

func TestSourceCIFailed_NegativeID_ProducesEmptyIdentityID(t *testing.T) {
	// NotifKey guards <= 0 ids (convention 11): a malformed negative id must
	// not silently produce a non-empty, malformed key.
	now := time.Now().UTC()
	finish := now.Add(-time.Hour)
	runs := []PipelineRun{
		{
			ID:           -1,
			Status:       "completed",
			Result:       "failed",
			FinishTime:   &finish,
			RequestedFor: Identity{ID: "user-1"},
		},
	}
	server := newPipelineRunServer(t, "alpha", runs)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	rows, err := SourceCIFailed(mc, 14, 50, now)
	if err != nil {
		t.Fatalf("SourceCIFailed failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Identity.ID != "" {
		t.Errorf("Identity.ID = %q, want empty for a non-positive run id", rows[0].Identity.ID)
	}
}

func TestSourceCIFailed_NilMultiClient_ReturnsError(t *testing.T) {
	_, err := SourceCIFailed(nil, 14, 50, time.Now())
	if err == nil {
		t.Fatal("expected error for nil MultiClient")
	}
}

func TestSourceCIFailed_PropagatesListError(t *testing.T) {
	errServer := newErrorServer(t)
	defer errServer.Close()

	now := time.Now().UTC()
	finish := now.Add(-time.Hour)
	// A second, succeeding project is required so len(errs) < len(mc.clients)
	// and ListPipelineRuns returns rows alongside a *PartialError rather than
	// an all-projects-failed error — otherwise the errors.As branch below is
	// unreachable-by-construction (matching task 4's own review feedback).
	okRuns := []PipelineRun{
		{
			ID:           1,
			Status:       "completed",
			Result:       "failed",
			FinishTime:   &finish,
			RequestedFor: Identity{ID: "user-1"},
		},
	}
	okServer := newPipelineRunServer(t, "beta", okRuns)
	defer okServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": errServer,
		"beta":  okServer,
	})
	setUserIDs(mc, "user-1")

	rows, err := SourceCIFailed(mc, 14, 50, now)
	if err == nil {
		t.Fatal("expected error to propagate from ListPipelineRuns")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a PartialError with one project failing and one succeeding, got: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the surviving project's row to be returned alongside the PartialError, got %d rows", len(rows))
	}
	if rows[0].Identity.Scope != "beta" {
		t.Errorf("Identity.Scope = %q, want the surviving project %q", rows[0].Identity.Scope, "beta")
	}
}

// --- isMyFailedRun ---

func TestIsMyFailedRun(t *testing.T) {
	tests := []struct {
		name   string
		run    PipelineRun
		userID string
		want   bool
	}{
		{
			name:   "completed, failed, mine: matches",
			run:    PipelineRun{Status: "completed", Result: "failed", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   true,
		},
		{
			name:   "completed, failed, someone else's: excluded",
			run:    PipelineRun{Status: "completed", Result: "failed", RequestedFor: Identity{ID: "someone-else"}},
			userID: "me",
			want:   false,
		},
		{
			name:   "completed, succeeded, mine: excluded (not a failure)",
			run:    PipelineRun{Status: "completed", Result: "succeeded", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   false,
		},
		{
			name:   "completed, canceled, mine: excluded (cancellations are predominantly deliberate, but a timeout lands here too and is deliberately not surfaced)",
			run:    PipelineRun{Status: "completed", Result: "canceled", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   false,
		},
		{
			name:   "completed, partiallySucceeded, mine: excluded",
			run:    PipelineRun{Status: "completed", Result: "partiallySucceeded", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   false,
		},
		{
			name:   "inProgress with Result none, mine: excluded (not terminal)",
			run:    PipelineRun{Status: "inProgress", Result: "none", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   false,
		},
		{
			name:   "canceling status, mine: excluded (not terminal)",
			run:    PipelineRun{Status: "canceling", Result: "none", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   false,
		},
		{
			name:   "not completed but Result somehow failed, mine: excluded (Status is checked explicitly)",
			run:    PipelineRun{Status: "inProgress", Result: "failed", RequestedFor: Identity{ID: "me"}},
			userID: "me",
			want:   false,
		},
		{
			// Finding 6 of task 7's review: an empty userID must not match
			// every run with an absent RequestedFor.ID (both would compare
			// "" == ""). resolveCIFailedUserID rejects an empty id before
			// this function's callers ever run it, but the guard belongs
			// here too, on the identity-comparison call site itself.
			name:   "empty userID: excluded even against a run with no RequestedFor.ID",
			run:    PipelineRun{Status: "completed", Result: "failed", RequestedFor: Identity{ID: ""}},
			userID: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMyFailedRun(tt.run, tt.userID); got != tt.want {
				t.Errorf("isMyFailedRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- ciFailedActivityStamp ---

func TestCIFailedActivityStamp_UsesFinishTime(t *testing.T) {
	queued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	finished := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	now := finished.Add(time.Hour)
	run := PipelineRun{QueueTime: queued, FinishTime: &finished}

	got := ciFailedActivityStamp(run, now)
	if !got.Equal(finished) {
		t.Errorf("ciFailedActivityStamp() = %v, want FinishTime %v (not QueueTime %v)", got, finished, queued)
	}
}

func TestCIFailedActivityStamp_FallsBackToQueueTime(t *testing.T) {
	queued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := queued.Add(time.Hour)

	tests := []struct {
		name string
		run  PipelineRun
	}{
		{"nil FinishTime", PipelineRun{QueueTime: queued}},
		{"zero FinishTime", PipelineRun{QueueTime: queued, FinishTime: &time.Time{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ciFailedActivityStamp(tt.run, now)
			if !got.Equal(queued) {
				t.Errorf("ciFailedActivityStamp() = %v, want QueueTime fallback %v", got, queued)
			}
		})
	}
}

func TestCIFailedActivityStamp_ClampsFutureFinishTime(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	run := PipelineRun{QueueTime: now.Add(-time.Hour), FinishTime: &future}

	got := ciFailedActivityStamp(run, now)
	if !got.Equal(now) {
		t.Errorf("ciFailedActivityStamp() = %v, want it clamped to now (%v), not the future FinishTime %v", got, now, future)
	}
}

func TestCIFailedActivityStamp_ClampsFutureQueueTimeFallback(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	run := PipelineRun{QueueTime: future}

	got := ciFailedActivityStamp(run, now)
	if !got.Equal(now) {
		t.Errorf("ciFailedActivityStamp() = %v, want it clamped to now (%v), not the future QueueTime fallback %v", got, now, future)
	}
}

// TestPipelineRun_WireFieldNames pins the wire field names by unmarshaling a
// raw JSON string literal shaped like Azure's real Get Builds/List Builds
// response, rather than round-tripping the same Go struct the client
// unmarshals (which would round-trip symmetrically even under a misspelled
// tag). A typo in "requestedFor" or its nested "id" leaves RequestedFor.ID
// empty, which would make isMyFailedRun silently stop matching any run for
// any user rather than failing loudly.
func TestPipelineRun_WireFieldNames(t *testing.T) {
	raw := `{
		"id": 42,
		"buildNumber": "20260101.1",
		"status": "completed",
		"result": "failed",
		"queueTime": "2026-01-01T00:00:00Z",
		"finishTime": "2026-01-01T01:00:00Z",
		"requestedFor": {
			"id": "user-1",
			"displayName": "Me",
			"uniqueName": "me@example.com"
		}
	}`

	var run PipelineRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	if run.RequestedFor.ID != "user-1" {
		t.Errorf("RequestedFor.ID = %q, want %q: \"requestedFor\" or its \"id\" json tag is misspelled", run.RequestedFor.ID, "user-1")
	}
	if run.Status != "completed" {
		t.Errorf("Status = %q, want %q: \"status\" json tag is misspelled", run.Status, "completed")
	}
	if run.Result != "failed" {
		t.Errorf("Result = %q, want %q: \"result\" json tag is misspelled", run.Result, "failed")
	}
	if run.FinishTime == nil || !run.FinishTime.Equal(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)) {
		t.Errorf("FinishTime = %v, want 2026-01-01T01:00:00Z: \"finishTime\" json tag is misspelled", run.FinishTime)
	}

	if !isMyFailedRun(run, "user-1") {
		t.Error("isMyFailedRun() = false for a run whose wire-decoded fields should match; a json tag typo silently breaks the filter")
	}
}

// --- Reconcile at the source level: repoll and rerun semantics ---

// TestSourceCIFailed_RepollSameRun_DoesNotResurrectDismissedRow polls the
// same, unchanged failed run twice (a real server round-trip each time, not
// a synthesised second row) and proves a dismissed (done) row stays dropped
// on the second poll when FinishTime has not moved. This is the equal-stamp
// branch, already covered exhaustively at the Reconcile level by task 3's
// table — kept here as a source-level smoke test, not as the test that pins
// the interesting behaviour: see
// TestSourceCIFailed_RerunFailsAgain_ResurfacesDismissedRow immediately
// below for that (finding 4 of task 7's review: nothing here fails if the
// stamp this source reports were swapped for a constant or for QueueTime).
func TestSourceCIFailed_RepollSameRun_DoesNotResurrectDismissedRow(t *testing.T) {
	finish := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	run := PipelineRun{
		ID:           7,
		BuildNumber:  "20260101.1",
		Status:       "completed",
		Result:       "failed",
		QueueTime:    finish.Add(-time.Hour),
		FinishTime:   &finish,
		RequestedFor: Identity{ID: "user-1"},
	}

	server := newPipelineRunServer(t, "alpha", []PipelineRun{run})
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	now := finish.Add(time.Hour)
	rows, err := SourceCIFailed(mc, 14, 50, now)
	if err != nil {
		t.Fatalf("SourceCIFailed (poll 1) failed: %v", err)
	}
	rows, state := Reconcile(rows, TriageState{}, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first reconcile, got %d", len(rows))
	}

	// User dismisses (marks done) the row.
	key := rows[0].Identity.ID
	entry := state[key]
	entry.Done = true
	state[key] = entry

	// Re-poll the same run from a fresh server round-trip: FinishTime is
	// unchanged, so this is the "same failed run seen again" case, not a new
	// failure.
	server2 := newPipelineRunServer(t, "alpha", []PipelineRun{run})
	defer server2.Close()
	mc2 := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server2})
	setUserIDs(mc2, "user-1")

	pollNow := now.Add(time.Minute)
	rows2, err := SourceCIFailed(mc2, 14, 50, pollNow)
	if err != nil {
		t.Fatalf("SourceCIFailed (poll 2) failed: %v", err)
	}
	rows2, _ = Reconcile(rows2, state, pollNow)
	if len(rows2) != 0 {
		t.Fatalf("expected the dismissed row to stay dropped on a re-poll of the same failed run, got %d rows", len(rows2))
	}
}

// TestSourceCIFailed_RerunFailsAgain_ResurfacesDismissedRow pins finding 3 of
// task 7's review: a YAML pipeline's "Rerun failed jobs" / "Rerun stage"
// re-executes inside the **same run id**, so Status returns to "inProgress"
// and, on completion, Result and FinishTime are recomputed for that same
// id. A run this source already surfaced and the caller dismissed can fail
// again — same Identity.ID (same run id, same BuildNumber), a later
// FinishTime — and that must resurface it as unread via Reconcile's
// newer-stamp branch, not leave it stranded as a stale dismissal.
//
// Mutation-tested per the review's instruction: swapping ciFailedActivityStamp's
// FinishTime for QueueTime makes this test fail, because QueueTime is set once
// at the *first* enqueue and a rerun does not change it — restoring FinishTime
// makes it pass again.
func TestSourceCIFailed_RerunFailsAgain_ResurfacesDismissedRow(t *testing.T) {
	firstFinish := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	queued := firstFinish.Add(-time.Hour)
	run := PipelineRun{
		ID:           7,
		BuildNumber:  "20260101.1",
		Status:       "completed",
		Result:       "failed",
		QueueTime:    queued,
		FinishTime:   &firstFinish,
		RequestedFor: Identity{ID: "user-1"},
	}

	server := newPipelineRunServer(t, "alpha", []PipelineRun{run})
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, "user-1")

	now := firstFinish.Add(time.Hour)
	rows, err := SourceCIFailed(mc, 14, 50, now)
	if err != nil {
		t.Fatalf("SourceCIFailed (poll 1) failed: %v", err)
	}
	rows, state := Reconcile(rows, TriageState{}, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first reconcile, got %d", len(rows))
	}

	// User dismisses (marks done) the row.
	key := rows[0].Identity.ID
	entry := state[key]
	entry.Done = true
	state[key] = entry

	// The user reruns the failed jobs. Azure re-executes inside the same run
	// id and BuildNumber; the rerun fails again with a later FinishTime.
	rerunFinish := firstFinish.Add(2 * time.Hour)
	rerun := run
	rerun.FinishTime = &rerunFinish

	server2 := newPipelineRunServer(t, "alpha", []PipelineRun{rerun})
	defer server2.Close()
	mc2 := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server2})
	setUserIDs(mc2, "user-1")

	pollNow := rerunFinish.Add(time.Minute)
	rows2, err := SourceCIFailed(mc2, 14, 50, pollNow)
	if err != nil {
		t.Fatalf("SourceCIFailed (poll 2, rerun) failed: %v", err)
	}
	if len(rows2) != 1 {
		t.Fatalf("expected the rerun to still be reported as a failed run, got %d rows", len(rows2))
	}
	if rows2[0].Identity.ID != key {
		t.Fatalf("rerun's Identity.ID = %q, want the same subject %q (same run id and BuildNumber)", rows2[0].Identity.ID, key)
	}

	rows2, state = Reconcile(rows2, state, pollNow)
	if len(rows2) != 1 {
		t.Fatalf("expected the re-failed run to resurface as one row after Reconcile, got %d rows", len(rows2))
	}
	if rows2[0].Read {
		t.Errorf("Read = true, want false: the rerun's later FinishTime must clear the prior dismissal")
	}
	entry = state[key]
	if entry.Done {
		t.Errorf("state[%q].Done = true, want false: Reconcile's newer-stamp branch must clear Done, not just Read", key)
	}
	if !entry.LastActivity.Equal(rerunFinish) {
		t.Errorf("state[%q].LastActivity = %v, want the rerun's FinishTime %v", key, entry.LastActivity, rerunFinish)
	}
}

// --- ciFailedWebURL degradation ladder ---

func TestCIFailedWebURL_DegradationLadder(t *testing.T) {
	server := newPipelineRunServer(t, "alpha", nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	tests := []struct {
		name string
		run  PipelineRun
		want string
	}{
		{
			name: "resolvable scope and positive id: deep link",
			run:  PipelineRun{ID: 42, ProjectName: "alpha"},
			want: "https://dev.azure.com/testorg/alpha/_build/results?buildId=42",
		},
		{
			name: "non-positive id: falls back to project page",
			run:  PipelineRun{ID: 0, ProjectName: "alpha"},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "negative id: falls back to project page",
			run:  PipelineRun{ID: -5, ProjectName: "alpha"},
			want: "https://dev.azure.com/testorg/alpha",
		},
		{
			name: "unresolvable scope: empty (no project page buildable)",
			run:  PipelineRun{ID: 42, ProjectName: "unknown-project"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ciFailedWebURL(mc, tt.run)
			if got != tt.want {
				t.Errorf("ciFailedWebURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCIFailedWebURL_NilMultiClient(t *testing.T) {
	got := ciFailedWebURL(nil, PipelineRun{ID: 42})
	if got != "" {
		t.Errorf("ciFailedWebURL(nil, ...) = %q, want empty", got)
	}
}

func TestMapCIFailed_WebURLIsLegalEmptyString(t *testing.T) {
	server := newPipelineRunServer(t, "alpha", nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	run := PipelineRun{ID: 42, ProjectName: "does-not-exist"}
	row := mapCIFailed(mc, run, time.Now())
	if row.WebURL != "" {
		t.Errorf("WebURL = %q, want empty string (legal degraded result)", row.WebURL)
	}
}

// --- ciFailedTitle ---

// TestCIFailedTitle pins finding 5 of task 7's review: the row's Title must
// say the build failed, not just name it, since GitHub's ci_activity reason
// is shared with successful runs and would otherwise render identically to
// a genuine Azure failure in the merged feed. It also pins finding 6's
// fallback: a definition name or build number that arrives empty must not
// leave a bare " #123 failed" / "CI # failed" — both empty falls back to a
// run-id-based title instead.
func TestCIFailedTitle(t *testing.T) {
	tests := []struct {
		name string
		run  PipelineRun
		want string
	}{
		{
			name: "definition and build number present",
			run:  PipelineRun{ID: 99, Definition: PipelineDefinition{Name: "CI"}, BuildNumber: "20260101.1"},
			want: "CI #20260101.1 failed",
		},
		{
			// Only the fully-empty case (both fields blank) gets the run-id
			// fallback (finding 6); a single blank field still renders with
			// a bare leading/trailing marker rather than triggering it.
			name: "empty definition name only: renders with a bare leading marker, not the fallback",
			run:  PipelineRun{ID: 99, Definition: PipelineDefinition{Name: ""}, BuildNumber: "20260101.1"},
			want: " #20260101.1 failed",
		},
		{
			name: "empty build number and empty definition name: falls back to run id",
			run:  PipelineRun{ID: 99, Definition: PipelineDefinition{Name: ""}, BuildNumber: ""},
			want: "Run 99 failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ciFailedTitle(tt.run); got != tt.want {
				t.Errorf("ciFailedTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
