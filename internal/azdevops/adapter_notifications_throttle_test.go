package azdevops

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// notifThrottled — white-box table test on the predicate itself.
// ---------------------------------------------------------------------------

// TestAdapter_notifThrottled pins every case notifThrottled's own doc
// comment claims: the interval<=0 disable check runs independently of the
// Before comparison (not merely as a special case that happens to agree with
// it — the "zero interval, now before lastPollAt" case below is what forces
// that), a zero notifLastPollAt is never throttled without a dedicated
// guard, and the window boundary itself is exclusive (exactly at the
// boundary is not throttled, matching time.Time.Before's own semantics).
func TestAdapter_notifThrottled(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name            string
		minPollInterval time.Duration
		notifLastPollAt time.Time
		now             time.Time
		want            bool
	}{
		{
			name:            "first call, zero lastPollAt is never throttled",
			minPollInterval: 5 * time.Minute,
			notifLastPollAt: time.Time{},
			now:             now,
			want:            false,
		},
		{
			name:            "just before the window closes is throttled",
			minPollInterval: 5 * time.Minute,
			notifLastPollAt: now,
			now:             now.Add(5*time.Minute - time.Nanosecond),
			want:            true,
		},
		{
			name:            "exactly at the window boundary is not throttled",
			minPollInterval: 5 * time.Minute,
			notifLastPollAt: now,
			now:             now.Add(5 * time.Minute),
			want:            false,
		},
		{
			name:            "well after the window boundary is not throttled",
			minPollInterval: 5 * time.Minute,
			notifLastPollAt: now,
			now:             now.Add(time.Hour),
			want:            false,
		},
		{
			name:            "zero interval disables throttling even mid-window",
			minPollInterval: 0,
			notifLastPollAt: now,
			now:             now.Add(time.Second),
			want:            false,
		},
		{
			// Without the interval<=0 check running first and independently,
			// this case would fall through to the Before comparison alone:
			// now.Before(lastPollAt.Add(0)) is true whenever now is before
			// lastPollAt, which it is here. A mutant that removes the
			// interval<=0 guard (or narrows it, e.g. to interval < 0) turns
			// this case's want:false into an actual false — this is the case
			// that kills that mutant; none of the other cases above would.
			name:            "zero interval disables throttling even when now is before lastPollAt",
			minPollInterval: 0,
			notifLastPollAt: now.Add(time.Hour),
			now:             now,
			want:            false,
		},
		{
			name:            "negative interval disables throttling",
			minPollInterval: -5 * time.Second,
			notifLastPollAt: now,
			now:             now,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Adapter{
				notifMinPollInterval: tt.minPollInterval,
				notifLastPollAt:      tt.notifLastPollAt,
			}
			if got := a.notifThrottled(tt.now); got != tt.want {
				t.Errorf("notifThrottled(%v) = %v, want %v (minPollInterval=%v, notifLastPollAt=%v)",
					tt.now, got, tt.want, tt.minPollInterval, tt.notifLastPollAt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// list's self-throttle — integration-level tests against the real four-source
// fan-out, mirroring the composerFixture-based tests above list's other
// contracts in this package.
// ---------------------------------------------------------------------------

// TestAdapter_List_FirstCall_NeverThrottled pins the first-call case
// notifThrottled's doc comment argues by construction from time.Time's zero
// value: with no dedicated guard for it, a regression that broke that
// reasoning (e.g. comparing against a non-zero sentinel, or initializing
// notifLastPollAt to something other than the zero value) would silently
// throttle the very first List call against an interval nothing has opened
// yet, returning an empty, nil-error feed instead of ever running a real
// query. A full hour of throttle makes that failure mode unambiguous here.
func TestAdapter_List_FirstCall_NeverThrottled(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), time.Hour)

	rows, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("list() error = %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("list() rows = %v, want all 4 fixture rows — the very first call must never be throttled", identityIDs(rows))
	}
}

// TestAdapter_List_Throttled_ReturnsCachedResultWithoutRealQuery proves a
// call inside the throttle window returns the previous result unchanged
// rather than running a real query: the fixture is mutated to add a new PR
// between the first and second calls, and the second call — made inside the
// window — must not reflect it. A third call, made after the window has
// elapsed, must run a real query and pick up the mutation.
func TestAdapter_List_Throttled_ReturnsCachedResultWithoutRealQuery(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 5*time.Minute)

	rows1, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("first list() error = %v", err)
	}
	if len(rows1) != 4 {
		t.Fatalf("first list() len = %d, want 4: %v", len(rows1), identityIDs(rows1))
	}

	// A real second query would see this; a throttled one must not.
	f.prs = append(f.prs, PullRequest{
		ID:           999,
		Title:        "Second PR added after the window opened",
		Repository:   Repository{ID: "repo-1", Name: "myrepo"},
		CreationDate: now,
	})

	rows2, err := a.list(provider.NotifOpts{}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("throttled list() error = %v, want nil — a throttled return must never be mistaken for a failure", err)
	}
	if got, want := identityIDs(rows2), identityIDs(rows1); !equalStrings(got, want) {
		t.Errorf("throttled list() rows = %v, want exactly the previous result %v unchanged — a throttled call must not run a real query", got, want)
	}

	rows3, err := a.list(provider.NotifOpts{}, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("post-window list() error = %v", err)
	}
	found := false
	for _, row := range rows3 {
		if row.Identity.ID == "review/pr/999" {
			found = true
		}
	}
	if !found {
		t.Errorf("post-window list() rows = %v, want review/pr/999 present — once the window elapses the next call must run a real query and reflect the updated fixture", identityIDs(rows3))
	}
}

// TestAdapter_List_Throttled_ReturnsCopyNotAlias is the mutate-and-recheck
// test the self-throttle constraint explicitly calls for: comparing two
// aliases of one backing array is a tautology, so this mutates the first
// call's result in place and asserts the throttled second call is
// unaffected — the only way that can hold is if the throttled path returns a
// genuine copy (copyNotifications), not the cached slice itself.
func TestAdapter_List_Throttled_ReturnsCopyNotAlias(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 5*time.Minute)

	rows1, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("first list() error = %v", err)
	}
	if len(rows1) == 0 {
		t.Fatal("first list() returned no rows, cannot exercise the mutate-then-recheck contract")
	}
	original := rows1[0].Title
	rows1[0].Title = "MUTATED BY CALLER"

	rows2, err := a.list(provider.NotifOpts{}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("throttled list() error = %v", err)
	}
	if len(rows2) == 0 {
		t.Fatal("throttled list() returned no rows")
	}
	if rows2[0].Title == "MUTATED BY CALLER" {
		t.Fatal("throttled list()'s result aliases the first call's backing array — mutating rows1[0] leaked into rows2[0]; the cached slice must be returned by copy (see copyNotifications' doc comment)")
	}
	if rows2[0].Title != original {
		t.Errorf("rows2[0].Title = %q, want the original %q — this cache must not change content within the throttle window, independent of the aliasing bug above", rows2[0].Title, original)
	}
}

// TestAdapter_List_PartialFailure_StillEstablishesThrottleWindow pins that
// list's absorb branch (task 8: at least one source succeeded) is what
// advances the throttle anchor, even when the result it caches is itself
// degraded (missing the failed source's rows). The fixture is repaired
// between the first and second calls; if the partial failure had not opened
// the window, the second call would run a real query and pick the repair up.
func TestAdapter_List_PartialFailure_StillEstablishesThrottleWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	f.failBuilds = true // CIFailed alone fails; the other three sources succeed
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 5*time.Minute)

	rows1, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("first list() error = %v, want nil (a single failing source absorbs, per task 8)", err)
	}
	if len(rows1) != 3 {
		t.Fatalf("first list() len = %d, want 3: %v", len(rows1), identityIDs(rows1))
	}

	// Repair the fixture: if the partial-failure real query had not opened
	// the throttle window, this still-inside-the-window second call would
	// run a real query and pick cifail/run/7 back up.
	f.failBuilds = false

	rows2, err := a.list(provider.NotifOpts{}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("throttled list() error = %v", err)
	}
	if got, want := identityIDs(rows2), identityIDs(rows1); !equalStrings(got, want) {
		t.Errorf("throttled list() rows = %v, want unchanged from the partial-failure result %v — the absorb branch must still open a throttle window even when it degraded", got, want)
	}
}

// TestAdapter_List_FullFailure_DoesNotEstablishThrottleWindow pins the other
// half of the failed/partial-failure design decision: a real query where
// every source fails outright must neither cache the error nor open a
// throttle window. The Adapter's client is swapped to a healthy backend
// (white-box — this test lives in package azdevops) and called again at the
// exact same now used for the failing call; if the failure had wrongly
// advanced or cached anything, this identical-now second call would either
// throttle (returning nil, nil instead of real rows) or replay the cached
// error.
func TestAdapter_List_FullFailure_DoesNotEstablishThrottleWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	}))
	defer failingServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": failingServer})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 5*time.Minute)

	rows, err := a.list(provider.NotifOpts{}, now)
	if err == nil {
		t.Fatal("first list() error = nil, want an error — every source fails")
	}
	if rows != nil {
		t.Fatalf("first list() rows = %v, want nil alongside the error", rows)
	}
	if !a.notifLastPollAt.IsZero() {
		t.Errorf("notifLastPollAt = %v, want the zero value — a total-failure real query must not open a throttle window", a.notifLastPollAt)
	}

	f := newComposerFixture(now)
	healthyServer := newComposerServer(t, f)
	defer healthyServer.Close()
	a.mc = newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": healthyServer})
	setUserIDs(a.mc, composerTestUserID)

	rows2, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("second list() error = %v, want nil — the earlier total failure must not be cached as an error and replayed here", err)
	}
	if len(rows2) != 4 {
		t.Fatalf("second list() rows = %v, want all 4 fixture rows — the earlier total failure must not have opened/poisoned a throttle window", identityIDs(rows2))
	}
}

// TestAdapter_List_Throttled_ReflectsMarkReadMadeDuringWindow pins task 14
// review finding 1: a throttled call must not simply replay the previous
// result verbatim once a mark has landed in between. It re-reconciles the
// cached raw rows against the store's current state on every throttled
// call, so a MarkRead issued after the real query — but still inside the
// throttle window — must be visible on the very next throttled List, not
// just once the window closes and a real query finally runs.
func TestAdapter_List_Throttled_ReflectsMarkReadMadeDuringWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 5*time.Minute)

	rows1, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("first list() error = %v", err)
	}
	for _, row := range rows1 {
		if row.Identity.ID == "review/pr/42" && row.Read {
			t.Fatalf("review/pr/42 is already Read before any mark — fixture assumption broken")
		}
	}

	if err := a.MarkRead(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}

	// Still inside the throttle window: no real query, but the mark above
	// must be reflected because the throttled path re-reconciles against
	// the store's current state rather than replaying rows1 unchanged.
	rows2, err := a.list(provider.NotifOpts{}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("throttled list() error = %v", err)
	}
	if len(rows2) != len(rows1) {
		t.Fatalf("throttled list() len = %d, want %d — MarkRead must not remove a row (only MarkDone does)", len(rows2), len(rows1))
	}
	found := false
	for _, row := range rows2 {
		if row.Identity.ID == "review/pr/42" {
			found = true
			if !row.Read {
				t.Errorf("review/pr/42.Read = false on a throttled call made after MarkRead, want true — a throttled List must re-reconcile against the store's current state, not just replay notifLastResult verbatim")
			}
		}
	}
	if !found {
		t.Fatalf("throttled list() rows = %v, want review/pr/42 present", identityIDs(rows2))
	}
}

// TestAdapter_List_Throttled_ReflectsMarkDoneMadeDuringWindow is
// TestAdapter_List_Throttled_ReflectsMarkReadMadeDuringWindow's MarkDone
// counterpart: Reconcile drops a Done row from its output entirely (see
// notifications_reconcile.go), so the assertion here is the row's absence,
// not a flipped field.
func TestAdapter_List_Throttled_ReflectsMarkDoneMadeDuringWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 5*time.Minute)

	rows1, err := a.list(provider.NotifOpts{}, now)
	if err != nil {
		t.Fatalf("first list() error = %v", err)
	}
	if len(rows1) != 4 {
		t.Fatalf("first list() len = %d, want 4: %v", len(rows1), identityIDs(rows1))
	}

	if err := a.MarkDone(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}

	// Still inside the throttle window: no real query, but the mark above
	// must be reflected because the throttled path re-reconciles against
	// the store's current state rather than replaying rows1 unchanged.
	rows2, err := a.list(provider.NotifOpts{}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("throttled list() error = %v", err)
	}
	if len(rows2) != 3 {
		t.Fatalf("throttled list() len = %d, want 3 — MarkDone must drop review/pr/42 from a throttled call made after the mark, not just after the window closes: %v", len(rows2), identityIDs(rows2))
	}
	for _, row := range rows2 {
		if row.Identity.ID == "review/pr/42" {
			t.Errorf("throttled list() rows = %v, want review/pr/42 absent after MarkDone", identityIDs(rows2))
		}
	}
}

// ---------------------------------------------------------------------------
// Lock discipline — MarkRead/MarkDone must never block behind an in-flight
// List's network work. -race cannot build in the sandbox this was written
// in; this test proves the property by timing, not by race detection, and
// is meant to also be run with -race elsewhere.
// ---------------------------------------------------------------------------

// TestAdapter_MarkRead_NotBlockedByInFlightList proves ordering, not timing:
// the fixture server blocks on a channel the test controls, so a concurrent
// List call is guaranteed to still be holding notifThrottleMu (and doing
// network work under it, waiting on that channel) when MarkRead is called.
// MarkRead is required to complete and signal markDone *before* the test
// releases the server — if MarkRead acquired notifThrottleMu, the bug this
// test exists to catch, it would deadlock behind the in-flight List and
// never reach that signal, so the release would never happen and the test
// would time out instead of hanging forever (a real deadlock still fails
// fast here via the select's timeout, rather than the 10-minute suite
// timeout). serverEntered proves the handler is actually blocked mid-request
// before MarkRead is even attempted, so a passing test cannot be explained
// by MarkRead simply winning a race to run first.
func TestAdapter_MarkRead_NotBlockedByInFlightList(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	f := newComposerFixture(now)
	server := newComposerServer(t, f)
	defer server.Close()

	const testTimeout = 5 * time.Second
	serverEntered := make(chan struct{})
	releaseServer := make(chan struct{})
	orig := server.Config.Handler
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case serverEntered <- struct{}{}:
		default:
			// Only the first request signals; the composer fixture issues
			// several concurrent requests per real query, and only one
			// needs to prove the server side has been reached.
		}
		select {
		case <-releaseServer:
		case <-time.After(testTimeout):
			return
		}
		orig.ServeHTTP(w, r)
	})

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})
	setUserIDs(mc, composerTestUserID)
	store := newTestTriageStore(t)
	a := NewAdapterWithNotifications(mc, store, 14, DefaultNotificationSourceToggles(), 0)

	listDone := make(chan struct{})
	go func() {
		defer close(listDone)
		a.list(provider.NotifOpts{}, now)
	}()

	select {
	case <-serverEntered:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the in-flight List call to reach the server — cannot exercise the concurrent-with-List case")
	}

	markDone := make(chan struct{})
	go func() {
		defer close(markDone)
		if err := a.MarkRead(provider.Identity{Kind: provider.KindAzure, Scope: "alpha", ID: "review/pr/42"}); err != nil {
			t.Errorf("MarkRead() error = %v", err)
		}
	}()

	select {
	case <-markDone:
	case <-time.After(testTimeout):
		t.Fatal("MarkRead did not return while a List call was in flight — MarkRead must not block behind List's network work; see the Adapter struct's lock-order comment")
	}

	close(releaseServer)

	select {
	case <-listDone:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the in-flight List call to finish after releasing the server")
	}
}
