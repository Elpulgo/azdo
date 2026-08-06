package azdevops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// assignedServerFixture configures a per-project httptest server backing
// SourceAssigned's single WIQL + GetWorkItems round trip.
//
// ids is the WIQL response's id list. By default (now left zero) it is
// returned as-is — the caller is asserting on ids the fixture already
// decided should "win" and does not need window filtering. When now is
// non-zero, matchingIDs additionally filters ids to those whose item has
// ChangedDate >= now.AddDate(0, 0, -lookbackDays), modelling Azure's own
// server-side `[System.ChangedDate] >= @Today-N` filtering, since this
// repo's httptest fixtures cannot evaluate the macro themselves (matching
// newMentionServer's identical limitation for @RecentMentions). This lets a
// fixture declare an id that is present in the candidate set but excluded
// by the window, so a test can prove the window bound is load-bearing
// rather than merely absent from the fixture (see
// TestSourceAssigned_FreshInstall_WindowBoundsFlood).
//
// matchingIDs also excludes any item whose Fields.State is "Closed" or
// "Removed" — but only if the WIQL request this fixture actually captured
// contains both state-exclusion clauses. Unlike the window filter, this is
// not driven by a fixture field: it is driven by the literal query text the
// client under test sent, so a fixture cannot opt out of it, and removing
// the clause from ListRecentlyAssignedWorkItems's production query changes
// what this fixture returns, not just what a query-content test asserts on
// (see TestSourceAssigned_ClosedItem_DoesNotResurrect).
//
// items is keyed by id for the GetWorkItems batch response, deliberately
// returned in an order unrelated to ids (reversed) so a caller trusting
// either the WIQL order or the batch endpoint's own order — rather than
// re-sorting by Fields.ChangedDate — would be caught (see
// Client.ListRecentlyAssignedWorkItems's doc comment).
type assignedServerFixture struct {
	ids          []int
	items        map[int]WorkItem
	now          time.Time
	lookbackDays int

	mu            sync.Mutex
	capturedQuery string
}

// matchingIDs applies the window and state-clause filters described in the
// fixture's doc comment. now.IsZero() means "no window filtering
// configured", preserving every existing fixture's behaviour of trusting
// ids verbatim on that axis.
func (f *assignedServerFixture) matchingIDs() []int {
	ids := f.ids

	if !f.now.IsZero() {
		cutoff := f.now.AddDate(0, 0, -f.lookbackDays)
		filtered := make([]int, 0, len(ids))
		for _, id := range ids {
			wi, ok := f.items[id]
			if !ok || wi.Fields.ChangedDate.Before(cutoff) {
				continue
			}
			filtered = append(filtered, id)
		}
		ids = filtered
	}

	f.mu.Lock()
	rawBody := f.capturedQuery
	f.mu.Unlock()
	// capturedQuery is the raw JSON request body, which escapes `<`/`>` —
	// decode it back to the literal query text before matching on it.
	var decoded struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal([]byte(rawBody), &decoded)
	excludesClosedAndRemoved := strings.Contains(decoded.Query, "<> 'Closed'") && strings.Contains(decoded.Query, "<> 'Removed'")
	if excludesClosedAndRemoved {
		filtered := make([]int, 0, len(ids))
		for _, id := range ids {
			wi, ok := f.items[id]
			if ok && (wi.Fields.State == "Closed" || wi.Fields.State == "Removed") {
				continue
			}
			filtered = append(filtered, id)
		}
		ids = filtered
	}

	return ids
}

func newAssignedServer(t *testing.T, f *assignedServerFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "POST" {
			bodyBytes, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.capturedQuery = string(bodyBytes)
			f.mu.Unlock()

			ids := f.matchingIDs()
			refs := make([]WorkItemReference, len(ids))
			for i, id := range ids {
				refs[i] = WorkItemReference{ID: id}
			}
			resp := struct {
				WorkItems []WorkItemReference `json:"workItems"`
			}{WorkItems: refs}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// GetWorkItems batch: return items in reverse order relative to
		// the (window-filtered) ids, modelling the endpoint's own
		// independent ordering (see newMentionServer's identical comment).
		ids := f.matchingIDs()
		value := make([]WorkItem, 0, len(ids))
		for i := len(ids) - 1; i >= 0; i-- {
			if wi, ok := f.items[ids[i]]; ok {
				value = append(value, wi)
			}
		}
		resp := struct {
			Value []WorkItem `json:"value"`
		}{Value: value}
		json.NewEncoder(w).Encode(resp)
	}))
}

func (f *assignedServerFixture) query() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.capturedQuery
}

// --- SourceAssigned: mapping ---

func TestSourceAssigned_MapsWorkItemToNotification(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	changedDate := now.Add(-time.Hour)
	fixture := &assignedServerFixture{
		ids: []int{42},
		items: map[int]WorkItem{
			42: {ID: 42, Fields: WorkItemFields{Title: "Fix the widget", ChangedDate: changedDate}},
		},
	}
	server := newAssignedServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	rows, err := SourceAssigned(mc, 14, now)
	if err != nil {
		t.Fatalf("SourceAssigned failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	// Pins that mapAssigned actually uses assignedActivityStamp's return
	// value rather than leaving UpdatedAt at its zero value. This mattered:
	// before TestSourceAssigned_ChangedDateAdvance_ResurrectsDismissedRow
	// existed, a zero UpdatedAt survived the whole suite silently, because
	// Reconcile treats a zero stamp as "no activity information" and skips
	// the stamp comparison entirely (see Reconcile's doc comment), which
	// makes a resurrection mutant look idempotent by construction. Both
	// that test and this assertion now fail on such a mutant.
	if !row.UpdatedAt.Equal(changedDate) {
		t.Errorf("UpdatedAt = %v, want the work item's ChangedDate %v", row.UpdatedAt, changedDate)
	}
	if row.Identity.Kind != provider.KindAzure {
		t.Errorf("Identity.Kind = %v, want KindAzure", row.Identity.Kind)
	}
	if row.Identity.Scope != "alpha" {
		t.Errorf("Identity.Scope = %q, want %q", row.Identity.Scope, "alpha")
	}
	if row.Identity.ID != "assigned/wi/42" {
		t.Errorf("Identity.ID = %q, want %q", row.Identity.ID, "assigned/wi/42")
	}
	if row.Title != "Fix the widget" {
		t.Errorf("Title = %q, want %q", row.Title, "Fix the widget")
	}
	if row.Reason != provider.NotificationReasonAssigned {
		t.Errorf("Reason = %v, want NotificationReasonAssigned", row.Reason)
	}
}

func TestSourceAssigned_NegativeID_ProducesEmptyIdentityID(t *testing.T) {
	// NotifKey guards <= 0 ids (convention 11): a malformed negative id must
	// not silently produce a non-empty, malformed key.
	fixture := &assignedServerFixture{
		ids:   []int{-1},
		items: map[int]WorkItem{-1: {ID: -1, Fields: WorkItemFields{Title: "Broken"}}},
	}
	server := newAssignedServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	rows, err := SourceAssigned(mc, 14, time.Now())
	if err != nil {
		t.Fatalf("SourceAssigned failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Identity.ID != "" {
		t.Errorf("Identity.ID = %q, want empty for a non-positive work item id", rows[0].Identity.ID)
	}
}

func TestSourceAssigned_NilMultiClient_ReturnsError(t *testing.T) {
	_, err := SourceAssigned(nil, 14, time.Now())
	if err == nil {
		t.Fatal("expected error for nil MultiClient")
	}
}

func TestSourceAssigned_PropagatesListError(t *testing.T) {
	errServer := newErrorServer(t)
	defer errServer.Close()

	okFixture := &assignedServerFixture{
		ids:   []int{1},
		items: map[int]WorkItem{1: {ID: 1, Fields: WorkItemFields{Title: "Surviving WI"}}},
	}
	okServer := newAssignedServer(t, okFixture)
	defer okServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": errServer,
		"beta":  okServer,
	})

	rows, err := SourceAssigned(mc, 14, time.Now())
	if err == nil {
		t.Fatal("expected error to propagate from ListRecentlyAssignedWorkItems")
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

// --- Cross-poll resurrection (mapAssigned's UpdatedAt end-to-end) ---

// TestSourceAssigned_ChangedDateAdvance_ResurrectsDismissedRow pins that
// mapAssigned's UpdatedAt (assignedActivityStamp's return value) is what
// Reconcile actually uses to decide resurrection, mirroring task 4's
// TestSourceReviewRequested_NewPushResurrectsDismissedRow: poll, mark the
// row done, re-poll unchanged data (stays dropped), then re-poll with an
// advanced ChangedDate (resurrects unread). Without UpdatedAt reaching
// Reconcile, this test cannot pass — the mapping-level assertion in
// TestSourceAssigned_MapsWorkItemToNotification checks the field directly,
// this one checks that the field does something.
func TestSourceAssigned_ChangedDateAdvance_ResurrectsDismissedRow(t *testing.T) {
	firstChanged := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fixture := &assignedServerFixture{
		ids: []int{42},
		items: map[int]WorkItem{
			42: {ID: 42, Fields: WorkItemFields{Title: "Fix the widget", ChangedDate: firstChanged}},
		},
	}
	server := newAssignedServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	now := firstChanged.Add(time.Hour)
	rows, err := SourceAssigned(mc, 14, now)
	if err != nil {
		t.Fatalf("SourceAssigned (poll 1) failed: %v", err)
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

	// Re-poll unchanged data: the row must stay dropped (done).
	rows2, err := SourceAssigned(mc, 14, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("SourceAssigned (poll 2, unchanged) failed: %v", err)
	}
	rows2, state2 := Reconcile(rows2, state, now.Add(time.Minute))
	if len(rows2) != 0 {
		t.Fatalf("expected the done row to stay dropped on an unchanged poll, got %d rows", len(rows2))
	}

	// Third poll: the item is edited again (still assigned, not closed) —
	// ChangedDate advances, served from a fresh server round-trip.
	secondChanged := firstChanged.Add(24 * time.Hour)
	fixture2 := &assignedServerFixture{
		ids: []int{42},
		items: map[int]WorkItem{
			42: {ID: 42, Fields: WorkItemFields{Title: "Fix the widget", ChangedDate: secondChanged}},
		},
	}
	server2 := newAssignedServer(t, fixture2)
	defer server2.Close()
	mc2 := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server2})

	pollNow := secondChanged.Add(time.Minute)
	rows3, err := SourceAssigned(mc2, 14, pollNow)
	if err != nil {
		t.Fatalf("SourceAssigned (poll 3, advanced ChangedDate) failed: %v", err)
	}
	rows3, _ = Reconcile(rows3, state2, pollNow)
	if len(rows3) != 1 {
		t.Fatalf("expected the item to resurrect after ChangedDate advanced, got %d rows", len(rows3))
	}
	if rows3[0].Done {
		t.Error("resurrected row must not still be Done")
	}
	if rows3[0].Read {
		t.Error("resurrected row must not still be Read")
	}
}

// TestSourceAssigned_ClosedItem_DoesNotResurrect pins the state clause end
// to end. Closing a work item is itself a revision (ChangedDate advances);
// the second poll's fixture carries that advanced ChangedDate *and* a
// State of "Closed" on the same item id, and assignedServerFixture.
// matchingIDs excludes it only because the WIQL text
// ListRecentlyAssignedWorkItems actually sent contains the
// `<> 'Closed'`/`<> 'Removed'` clauses (see that method's doc comment) — the
// fixture does not special-case this test. So this test is load-bearing
// against a regression at the production query, not just against Reconcile:
// dropping the clause from ListRecentlyAssignedWorkItems makes the fixture
// return the closed item with its advanced stamp, and this test fails (a
// closed item resurrecting as unread is exactly the defect the clause
// fixes; see workitems.go's ListRecentlyAssignedWorkItems doc comment).
func TestSourceAssigned_ClosedItem_DoesNotResurrect(t *testing.T) {
	firstChanged := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fixture := &assignedServerFixture{
		ids: []int{42},
		items: map[int]WorkItem{
			42: {ID: 42, Fields: WorkItemFields{Title: "Fix the widget", State: "Active", ChangedDate: firstChanged}},
		},
	}
	server := newAssignedServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	now := firstChanged.Add(time.Hour)
	rows, err := SourceAssigned(mc, 14, now)
	if err != nil {
		t.Fatalf("SourceAssigned (poll 1) failed: %v", err)
	}
	rows, state := Reconcile(rows, TriageState{}, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first reconcile, got %d", len(rows))
	}

	// The item is closed on day 3: State moves to "Closed" and, since
	// closing is a revision, ChangedDate advances too.
	closedNow := now.Add(3 * 24 * time.Hour)
	secondChanged := closedNow.Add(-time.Minute)
	closedFixture := &assignedServerFixture{
		ids: []int{42},
		items: map[int]WorkItem{
			42: {ID: 42, Fields: WorkItemFields{Title: "Fix the widget", State: "Closed", ChangedDate: secondChanged}},
		},
	}
	closedServer := newAssignedServer(t, closedFixture)
	defer closedServer.Close()
	closedMC := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": closedServer})

	rows2, err := SourceAssigned(closedMC, 14, closedNow)
	if err != nil {
		t.Fatalf("SourceAssigned (poll 2, after close) failed: %v", err)
	}
	if len(rows2) != 0 {
		t.Fatalf("expected the closed item to no longer be returned by the source, got %d rows", len(rows2))
	}

	rows2, _ = Reconcile(rows2, state, closedNow)
	if len(rows2) != 0 {
		t.Fatalf("expected 0 rows once the closed item stops matching the query, got %d", len(rows2))
	}
}

// --- Idempotence (decision 6: the lookback window replaces the snapshot) ---

// TestSourceAssigned_Idempotence_SamePollTwiceYieldsIdenticalRowsAndState
// pins decision 6's exact justification for a stateless bounded-lookback
// design over a snapshot/delta one: "the same poll twice yields the same
// rows". It drives two full poll runs (SourceAssigned + Reconcile) against
// the same, unchanged fixture data and the same now, and asserts both the
// raw rows and Reconcile's output rows and state are identical across the
// two runs — the property that replaces the snapshot. Two projects are used
// so the run exercises the multi-project merge, not just a single item
// trivially equal to itself; their ChangedDate values are distinct, which
// fully determines sort order on its own and so does not exercise
// sort.Slice's stability (a pre-existing, out-of-scope trait of every
// MultiClient fan-out — see multiclient.go:414-416).
func TestSourceAssigned_Idempotence_SamePollTwiceYieldsIdenticalRowsAndState(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)

	alphaFixture := &assignedServerFixture{
		ids:   []int{1},
		items: map[int]WorkItem{1: {ID: 1, Fields: WorkItemFields{Title: "Alpha item", ChangedDate: older}}},
	}
	betaFixture := &assignedServerFixture{
		ids:   []int{2},
		items: map[int]WorkItem{2: {ID: 2, Fields: WorkItemFields{Title: "Beta item", ChangedDate: newer}}},
	}
	alphaServer := newAssignedServer(t, alphaFixture)
	defer alphaServer.Close()
	betaServer := newAssignedServer(t, betaFixture)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	const lookbackDays = 14

	rows1, err := SourceAssigned(mc, lookbackDays, now)
	if err != nil {
		t.Fatalf("SourceAssigned (poll 1) failed: %v", err)
	}
	result1, state1 := Reconcile(rows1, TriageState{}, now)

	// Second poll run: same fixture data, same now — nothing changed
	// between polls.
	rows2, err := SourceAssigned(mc, lookbackDays, now)
	if err != nil {
		t.Fatalf("SourceAssigned (poll 2) failed: %v", err)
	}
	result2, state2 := Reconcile(rows2, state1, now)

	if !reflect.DeepEqual(rows1, rows2) {
		t.Fatalf("SourceAssigned's raw rows differ between two identical polls:\npoll1=%+v\npoll2=%+v", rows1, rows2)
	}
	if !reflect.DeepEqual(result1, result2) {
		t.Fatalf("Reconcile's output rows differ between two identical polls:\npoll1=%+v\npoll2=%+v", result1, result2)
	}
	if !reflect.DeepEqual(state1, state2) {
		t.Fatalf("Reconcile's state differs between two identical polls:\npoll1=%+v\npoll2=%+v", state1, state2)
	}
}

// --- Window bound (decision 6: a fresh install must not flood) ---

// TestSourceAssigned_FreshInstall_WindowBoundsFlood pins decision 6's other
// justification: "a fresh install with an empty state file surfaces at most
// the window's worth of items". Both insideWindow and outsideWindow are
// placed in the fixture's ids (the WIQL candidate list) *and* its items map,
// and the fixture's own ChangedDate filter (matchingIDs, standing in for
// Azure's server-side `[System.ChangedDate] >= @Today-N`) is what excludes
// outsideWindow — not an absence from the fixture's wiring. That makes the
// window bound itself the thing under test: widening the fixture's window
// to 30 days or beyond surfaces outsideWindow and fails this test — the
// cutoff comparison is Before(cutoff), so equality already qualifies (verified
// manually while fixing this test; not re-asserted here since it would
// require duplicating the fixture's cutoff arithmetic in the test body). It
// also asserts the literal @Today-14 text reached the WIQL request, so the
// bound is not merely coincidental to this fixture.
func TestSourceAssigned_FreshInstall_WindowBoundsFlood(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	insideWindow := WorkItem{ID: 100, Fields: WorkItemFields{Title: "Recently assigned", ChangedDate: now.Add(-2 * 24 * time.Hour)}}
	outsideWindow := WorkItem{ID: 200, Fields: WorkItemFields{Title: "Assigned long ago", ChangedDate: now.Add(-30 * 24 * time.Hour)}}

	const lookbackDays = 14
	fixture := &assignedServerFixture{
		ids: []int{insideWindow.ID, outsideWindow.ID},
		items: map[int]WorkItem{
			insideWindow.ID:  insideWindow,
			outsideWindow.ID: outsideWindow,
		},
		now:          now,
		lookbackDays: lookbackDays,
	}
	server := newAssignedServer(t, fixture)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	rows, err := SourceAssigned(mc, lookbackDays, now)
	if err != nil {
		t.Fatalf("SourceAssigned failed: %v", err)
	}

	// A fresh install has no local state at all: reconcile against empty
	// state to prove the surfaced feed itself, not just the raw query
	// result, is bounded by the window.
	result, _ := Reconcile(rows, TriageState{}, now)
	if len(result) != 1 {
		t.Fatalf("expected at most the window's worth of items (1), got %d", len(result))
	}
	if result[0].Identity.ID != NotifKey("assigned", "wi", insideWindow.ID) {
		t.Errorf("Identity.ID = %q, want the in-window item %q", result[0].Identity.ID, NotifKey("assigned", "wi", insideWindow.ID))
	}

	if q := fixture.query(); !strings.Contains(q, fmt.Sprintf("@Today-%d", lookbackDays)) {
		t.Errorf("WIQL query must bound the lookback window via @Today-%d, got query: %s", lookbackDays, q)
	}
}

// --- assignedActivityStamp ---

func TestAssignedActivityStamp_FallsBackToCreatedDate(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := created.Add(time.Hour)
	wi := WorkItem{Fields: WorkItemFields{CreatedDate: created}}

	got := assignedActivityStamp(wi, now)
	if !got.Equal(created) {
		t.Errorf("assignedActivityStamp() = %v, want CreatedDate fallback %v", got, created)
	}
}

// TestAssignedActivityStamp_NonZeroChangedDate_UsesChangedDate pins that the
// CreatedDate fallback only applies to a zero ChangedDate — the normal case
// must use ChangedDate, the same field the WIQL query filters and orders by.
func TestAssignedActivityStamp_NonZeroChangedDate_UsesChangedDate(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	changed := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	now := changed.Add(time.Hour)
	wi := WorkItem{Fields: WorkItemFields{CreatedDate: created, ChangedDate: changed}}

	got := assignedActivityStamp(wi, now)
	if !got.Equal(changed) {
		t.Errorf("assignedActivityStamp() = %v, want ChangedDate %v (not CreatedDate %v)", got, changed, created)
	}
}

func TestAssignedActivityStamp_ClampsFutureChangedDate(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	wi := WorkItem{Fields: WorkItemFields{ChangedDate: future}}

	got := assignedActivityStamp(wi, now)
	if !got.Equal(now) {
		t.Errorf("assignedActivityStamp() = %v, want it clamped to now (%v), not the future ChangedDate %v", got, now, future)
	}
}

func TestAssignedActivityStamp_ClampsFutureCreatedDateFallback(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	wi := WorkItem{Fields: WorkItemFields{CreatedDate: future}}

	got := assignedActivityStamp(wi, now)
	if !got.Equal(now) {
		t.Errorf("assignedActivityStamp() = %v, want it clamped to now (%v), not the future CreatedDate fallback %v", got, now, future)
	}
}

// --- mapAssigned ---

func TestMapAssigned_WebURLIsLegalEmptyString(t *testing.T) {
	server := newPRServer(t, nil)
	defer server.Close()
	mc := newMultiClientWithServers(t, map[string]*httptest.Server{"alpha": server})

	wi := WorkItem{ID: 42, ProjectName: "does-not-exist", Fields: WorkItemFields{Title: "Widget"}}
	row := mapAssigned(mc, wi, time.Now())
	if row.WebURL != "" {
		t.Errorf("WebURL = %q, want empty string (legal degraded result)", row.WebURL)
	}
}

// --- assignedWebURL degradation ladder ---

func TestAssignedWebURL_DegradationLadder(t *testing.T) {
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
			got := assignedWebURL(mc, tt.wi)
			if got != tt.want {
				t.Errorf("assignedWebURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssignedWebURL_NilMultiClient(t *testing.T) {
	got := assignedWebURL(nil, WorkItem{ID: 42, ProjectName: "alpha"})
	if got != "" {
		t.Errorf("assignedWebURL(nil, ...) = %q, want empty", got)
	}
}
