package notifications

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the notifications pane: a listview.Model[provider.Notification]
// plus the interactive `f` reason-filter cycle (Decisions 53, 54).
//
// The pane holds two layers over the merged feed:
//   - feed: the config-filtered rows (task 10's FilterNotifications output),
//     set wholesale by the caller via SetFeed whenever a fetch/poll lands.
//   - the `f` cycle, which narrows feed further to a single NotificationReason
//     (or "all", the default/reset position).
//
// Per decision 54, the narrowed result is always handed to listview via
// SetItems — never a second, independently-narrower slice fed to ToRows
// alone — so the column/row-cell invariant convention 7 relies on never
// diverges (convention 8: a divergence surfaces as a table.renderRow panic,
// not a failed assertion).
type Model struct {
	list listview.Model[provider.Notification]

	// feed is the config-filtered inbox, set by SetFeed. It is never itself
	// narrowed by the `f` cycle — reasonFiltered() derives a fresh slice from
	// it on every cycle step and every SetFeed call, so re-cycling never
	// needs a refetch.
	feed []provider.Notification

	// reasonFilterActive and reasonFilter together encode the `f` cycle's
	// current position. reasonFilterActive == false means "all reasons" —
	// the reset/default position reachable by cycling alone (decision 53).
	reasonFilterActive bool
	reasonFilter       provider.NotificationReason

	// capabilityUnsupported puts View() into decision 63's fourth render
	// state (see capabilityUnsupportedBody's doc comment for why it is
	// unreachable through the tab in phase 1). Set only by
	// SetCapabilityUnsupported; nothing in production ever calls it today.
	capabilityUnsupported bool

	// marker issues the mark-read/mark-done API calls behind `u`/`d` (task
	// 14, decisions 13, 25, 43). It is exactly provider.NotificationSource —
	// the composite already routes MarkRead/MarkDone by Identity.Kind over
	// capable backends and reports a descriptive per-kind error when none
	// match, so the pane needs no routing logic of its own, only the two
	// mutating methods.
	//
	// Nil-safe by construction: NewModelWithStyles is called unconditionally
	// from app.NewModel even for a capability-absent or nil provider
	// (Decision 61), so a nil marker is a reachable state, not a defensive
	// fallback. markRead/markDone both guard it explicitly before issuing a
	// tea.Cmd.
	marker provider.NotificationSource

	// overrides holds the pane's local optimistic intent for in-flight or
	// already-confirmed `u`/`d` actions, applied on top of the feed by
	// visibleItems until the debounce window expires (the spec's Unknowns
	// section: GitHub's read state can be eventually consistent, so a poll
	// landing right after a PATCH can still report unread).
	//
	// An entry outliving its own API call is the point, not an oversight: once
	// the call succeeds commitOverride writes the mark into feed and the entry
	// stays only to outweigh a lagging poll, which replaces feed wholesale.
	// Nothing removes a successful entry at the time it settles, so SetFeed
	// sweeps expired ones (prunedOverrides) to stop the map growing for the
	// life of the session. Rollback on API failure is dropping the entry here —
	// never deleting/re-inserting a row — so decision 45's merge-sort total
	// order is never reproduced by hand and can never be gotten wrong.
	//
	// Keyed by identityKey (Kind+Scope+ID) rather than provider.Identity
	// itself, matching Identity.SameItem's own comparison: ScopeDisplay is a
	// presentation detail that must never split one logical row's override
	// in two.
	//
	// FORWARD: task 16 — the unread-count footer badge must decide whether it
	// counts m.feed's raw Read field or this pane's override-adjusted view.
	// Nothing here exposes an "effective unread count" accessor; visibleItems
	// is unexported and is the only place the two are currently combined.
	// Without deciding this, a successful `u` would optimistically clear the
	// row on screen while the badge still counted it, which reads as the
	// count and the rows disagreeing.
	overrides map[identityKey]override

	// now lets tests replace time.Now for deterministic debounce-window
	// assertions, mirroring internal/ui/metrics/list.go's own now field.
	// Always set by NewModelWithStyles; use the clock() accessor rather than
	// calling m.now directly so a zero-value Model (Decision 61's documented
	// latent hazard — never reachable through production code today) cannot
	// nil-deref here even if some future caller reaches this path.
	now func() time.Time
}

// markDebounceWindow bounds how long a local u/d override outweighs a poll
// that has not yet caught up. It is finite on purpose: Decision 5 keeps
// phase 1 free of persisted local read/done state, so this is a short-lived
// debounce buffer, not a second source of truth — once it elapses, a poll
// that still disagrees with the local action is trusted again rather than
// held back forever.
const markDebounceWindow = 30 * time.Second

// overrideKind distinguishes the two `u`/`d` optimistic intents held in
// Model.overrides.
type overrideKind int

const (
	// overrideRead forces the row's effective Read to true — u's optimistic
	// mark-read. One-way per Decision 13: there is no corresponding
	// "unread" kind, since GitHub exposes no mark-unread endpoint.
	overrideRead overrideKind = iota
	// overrideHidden removes the row from the visible feed entirely — d's
	// optimistic mark-done.
	overrideHidden
)

// override is one entry in Model.overrides: which intent is being held, and
// until when it outweighs whatever the polled feed says.
type override struct {
	kind      overrideKind
	expiresAt time.Time
}

// identityKey is the map key used for Model.overrides: provider.Identity
// narrowed to exactly the fields Identity.SameItem compares (Kind, Scope,
// ID), deliberately dropping ScopeDisplay so a presentation-only difference
// between the row that was marked and a later poll's row can never be read
// as two different notifications.
type identityKey struct {
	kind  provider.Kind
	scope string
	id    string
}

// keyOf derives an identityKey from a full Identity.
func keyOf(id provider.Identity) identityKey {
	return identityKey{kind: id.Kind, scope: id.Scope, id: id.ID}
}

// MarkResultMsg reports the outcome of an in-flight MarkRead/MarkDone call
// issued by markCmd. err is carried through untouched — never flattened into a
// string — so a caller can still errors.As it apart (e.g. *github.APIError)
// rather than pattern-matching rendered text.
//
// Exported for one reason only: app must route this message to the pane
// *unconditionally*, not through its delegate-to-the-active-tab switch. A mark
// is issued from the notifications tab but its result lands one HTTP round trip
// later, by which point the user may well have pressed 2 — and a result handed
// to the pull-requests pane is silently discarded, leaving the override as the
// sole holder of the mark and re-opening decision 65's resurrection defect.
// `polling.PipelineRunsUpdated` is the existing precedent for a pane-bound
// message the top-level switch must handle regardless of active tab.
//
// Its fields stay unexported deliberately: app needs to *recognise* and forward
// this message, never construct or inspect one. The pane owns the payload.
type MarkResultMsg struct {
	key  identityKey
	kind overrideKind
	err  error
}

// baseColumns are the notifications list's per-row column specs, excluding
// the optional dynamic Repo column. Defined once and copied inside toColumns
// to avoid mutating the package-level slice (mirrors prBaseColumns).
var baseColumns = []listview.ColumnSpec{
	{Title: "Reason", WidthPct: 20, MinWidth: 10},
	{Title: "Title", WidthPct: 60, MinWidth: 20},
	{Title: "Updated", WidthPct: 20, MinWidth: 10},
}

// repoColumn is the dynamic column prepended when the item slice spans more
// than one distinct Identity.Scope (convention 7).
var repoColumn = listview.ColumnSpec{Title: "Repo", WidthPct: 20, MinWidth: 10}

// NewModel creates a new notifications pane model with default styles and no
// marker (u/d are no-ops until a real one is injected).
func NewModel() Model {
	return NewModelWithStyles(styles.DefaultStyles(), nil)
}

// NewModelWithStyles creates a new notifications pane model with custom
// styles and the marker used for `u`/`d` (task 14). marker may be nil — see
// Model.marker's doc comment; that is a reachable state, not a caller error.
// Polling wiring (the tab registration, the render states) is owned by later
// tasks (12, 13, 15) — this constructor builds a pane whose feed data enters
// exclusively through SetFeed/HandleFetchResult.
func NewModelWithStyles(s *styles.Styles, marker provider.NotificationSource) Model {
	cfg := listview.Config[provider.Notification]{
		LoadingMessage: "Loading notifications...",
		EntityName:     "notifications",
		MinWidth:       50,
		ToRows:         toRows,
		ToColumns:      toColumns,
		// Fetching is wired by a later task (12/15); this pane's data enters
		// through SetFeed, so Init's fetch is a deliberate no-op rather than
		// nil (listview.Init calls Fetch() unconditionally).
		Fetch: func() tea.Cmd { return nil },
		// No detail view in phase 1 (`o` opens the browser per decision 3;
		// there is nothing else to drill into). A harmless no-op stub avoids
		// a nil-func panic if enter is pressed, without building real
		// navigation (out of scope for this task).
		EnterDetail: func(item provider.Notification, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			return nil, nil
		},
	}

	return Model{
		list:   listview.New(cfg, s),
		marker: marker,
		now:    time.Now,
	}
}

// clock returns the pane's current time, via now when set (always true for
// a pane built through NewModelWithStyles) and falling back to time.Now
// otherwise, so a zero-value Model (Decision 61's documented latent hazard)
// cannot nil-deref here.
func (m Model) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.list.Init()
}

// Update handles messages. MarkResultMsg (the result of a `u`/`d`
// API call issued by markCmd) is handled unconditionally, since it can land
// regardless of view mode or search state. Otherwise: the `f` key cycles the
// reason filter (decision 53), `u`/`d` mark read/done (task 14) when
// canTriage allows it, `r` is swallowed while the fetch hook is a stub
// (decision 58), and every other message is forwarded to the underlying
// listview.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if res, ok := msg.(MarkResultMsg); ok {
		return m.handleMarkResult(res), nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && !m.list.IsSearching() && m.list.GetViewMode() == listview.ViewList {
		switch key.String() {
		case "f":
			return m.cycleReasonFilter(), nil
		case "u":
			if !m.canTriage() {
				return m, nil
			}
			return m.markRead()
		case "d":
			if !m.canTriage() {
				return m, nil
			}
			return m.markDone()
		case "r":
			// STOPGAP — removed by task 15, which wires the real fetch.
			// listview.updateList sets loading = true and shows the spinner
			// before batching config.Fetch(), and this pane's Fetch is a stub
			// returning nil, so nothing would ever call HandleFetchResult and
			// the spinner would never clear: a permanent one-keypress dead
			// end (decision 58). Swallowing `r` keeps the rows on screen. Task
			// 15 deletes this case together with the Fetch stub in
			// NewModelWithStyles.
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// canTriage reports whether `u`/`d` make sense right now: there must be a
// visible row under the cursor, and the pane must not be showing one of
// View()'s error/capability render states (task 13, decisions 17, 63) — both
// hide the table entirely, so a keypress reaching m.list in either state
// would act on a row the user cannot even see. listview.HandleFetchResult's
// error path leaves stale items in place (see listview.Err's doc comment),
// so the error check is needed in addition to the emptiness check, not
// implied by it.
func (m Model) canTriage() bool {
	if m.capabilityUnsupported || m.list.Err() != nil {
		return false
	}
	return len(m.list.Items()) > 0
}

// markRead issues one MarkRead call for the row under the cursor and marks
// it read optimistically (task 14). One-way per Decision 13: an already-Read
// row — whether the feed itself says so, or a still-active overrideRead
// already does — is left alone: no second MarkRead call, and no toggle back
// to unread. item.Read here is the row's *effective* state (visibleItems
// already applied any active override), so this single check covers both
// cases without consulting m.overrides directly.
func (m Model) markRead() (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || m.marker == nil {
		return m, nil
	}
	if item.Read {
		return m, nil
	}

	id := item.Identity
	m = m.withOverride(keyOf(id), overrideRead)
	return m, m.markCmd(id, overrideRead)
}

// markDone issues one MarkDone call for the row under the cursor and removes
// it from view optimistically (task 14). Restoring it on failure is dropping
// the override — never re-inserting into m.feed — so Decision 45's merge-sort
// total order is never reproduced by hand.
func (m Model) markDone() (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || m.marker == nil {
		return m, nil
	}

	key := keyOf(item.Identity)
	if ov, exists := m.overrides[key]; exists && ov.kind == overrideHidden && m.clock().Before(ov.expiresAt) {
		// Already marked done and still hidden by its own override: nothing
		// to re-trigger. Not reachable through the UI today — the row
		// disappears from Items() the moment this happens, so the cursor can
		// never land back on it while the override is live — guarded anyway
		// so a future caller driving markDone directly cannot double-fire.
		return m, nil
	}

	id := item.Identity
	m = m.withOverride(key, overrideHidden)
	return m, m.markCmd(id, overrideHidden)
}

// withOverride returns a copy of m with an override recorded for key,
// expiring markDebounceWindow after the current clock reading, and
// immediately re-applies it to the visible list (the "optimistic" half of
// task 14 — the row updates before the API call resolves).
//
// Always allocates a new map rather than mutating m.overrides in place: every
// other mutating method on this pane (reasonFiltered, visibleItems) hands
// back a fresh slice for the same reason — an aliased map would let two
// Model value copies silently share mutations, which would be observable the
// moment a rollback on one copy also changed a snapshot taken earlier.
func (m Model) withOverride(key identityKey, kind overrideKind) Model {
	next := make(map[identityKey]override, len(m.overrides)+1)
	for k, v := range m.overrides {
		next[k] = v
	}
	next[key] = override{kind: kind, expiresAt: m.clock().Add(markDebounceWindow)}
	m.overrides = next
	return m.refreshItems()
}

// dropOverride returns a copy of m with key's override removed — the
// rollback path for a failed MarkRead/MarkDone (task 14). This is
// deliberately NOT a no-op: dropping the override is what un-hides a failed
// `d` and un-marks-read a failed `u`. It is a complete rollback because only
// commitOverride ever writes a mark into m.feed and it runs solely on the
// success path, so on failure the override is still the only thing holding the
// optimistic change. A no-op rollback would leave that change on screen even
// though the API call failed.
func (m Model) dropOverride(key identityKey) Model {
	if _, ok := m.overrides[key]; !ok {
		return m
	}
	next := make(map[identityKey]override, len(m.overrides))
	for k, v := range m.overrides {
		if k != key {
			next[k] = v
		}
	}
	m.overrides = next
	return m.refreshItems()
}

// markCmd returns the tea.Cmd that performs the actual API call for kind
// (overrideRead -> MarkRead, overrideHidden -> MarkDone) against id, and
// reports the outcome via MarkResultMsg. m.marker is captured by
// value into the closure at call time, not m itself, so the goroutine
// bubbletea runs this in can never race a later Update on the model.
func (m Model) markCmd(id provider.Identity, kind overrideKind) tea.Cmd {
	marker := m.marker
	key := keyOf(id)
	return func() tea.Msg {
		var err error
		if kind == overrideRead {
			err = marker.MarkRead(id)
		} else {
			err = marker.MarkDone(id)
		}
		return MarkResultMsg{key: key, kind: kind, err: err}
	}
}

// FORWARD: task 19 — a failed `u`/`d` is currently silent. res.err is carried
// here intact and then discarded, so a user pressing `d` against a 403 sees the
// row vanish and silently reappear a round trip later, indistinguishable from a
// rendering glitch. Task 19 owns 403/401 differentiation for the *List* path;
// the mark path has no owner in the spec and needs at least a status-bar
// message on rollback.
//
// handleMarkResult applies the outcome of a markCmd: failure rolls the
// optimistic change back by dropping the override, success commits it into the
// held feed via commitOverride while deliberately leaving the override entry
// in place.
//
// Success must not be a no-op. The override alone is a *finite* debounce
// buffer, so leaving the confirmed mark resting on it means the row reappears
// the moment the window elapses — visibleItems re-derives from m.feed, which
// still carries the pre-mark row, and no poll needs to be involved: any purely
// local re-render past the window (an `f` cycle, a resize) resurrects a row the
// server already accepted as read or done. Committing to the feed is what makes
// a confirmed mark durable for as long as the pane holds that feed.
func (m Model) handleMarkResult(res MarkResultMsg) Model {
	if ov, ok := m.overrides[res.key]; ok && ov.kind != res.kind {
		// A newer action on this same row has already replaced the entry this
		// result owned — reachable as `u` then `d` on one row, since the row is
		// still in Items() with Read=true after the `u` and markDone's
		// already-hidden guard only rejects a live overrideHidden.
		//
		// Neither outcome may be applied on the newer action's behalf. A
		// rollback would drop the entry the `d` owns and un-hide a row whose
		// DELETE is still in flight; a commit would fold the older intent into
		// the feed. The newer action's own result is still coming and owns the
		// entry, so dropping this one loses nothing.
		return m
	}
	if res.err != nil {
		return m.dropOverride(res.key)
	}
	// Deliberately also commits when no entry remains: a success means the
	// server accepted the change, so correcting the feed is right even if the
	// entry was pruned or the user has since acted elsewhere. Erring the other
	// way would resurrect a row the server has already dropped.
	return m.commitOverride(res.key, res.kind)
}

// commitOverride folds a server-confirmed `u`/`d` into m.feed itself:
// overrideHidden drops the row, overrideRead sets its Read field. It keeps the
// override entry, which is not redundant — the two layers answer two different
// questions:
//
//   - the feed carries the mark for as long as this feed is held, so the
//     override expiring can never resurrect the row (see handleMarkResult);
//   - the override still outweighs a *poll* landing inside the debounce window
//     with stale `unread`, because SetFeed replaces m.feed wholesale and would
//     otherwise reintroduce the row GitHub has not caught up on yet.
//
// Once the window elapses a disagreeing poll is trusted again, exactly as
// Decision 5 intends: phase 1 keeps no persisted local read/done state, so the
// server is the only long-term source of truth.
//
// A key that matches no row in the feed leaves the model untouched (a poll may
// already have dropped it); the override stays live so a later poll that still
// reports the row does not flicker it back.
func (m Model) commitOverride(key identityKey, kind overrideKind) Model {
	next := make([]provider.Notification, 0, len(m.feed))
	found := false
	for _, n := range m.feed {
		if keyOf(n.Identity) != key {
			next = append(next, n)
			continue
		}
		found = true
		if kind == overrideHidden {
			continue
		}
		n.Read = true
		next = append(next, n)
	}
	if !found {
		return m
	}

	m.feed = next
	// Idempotent with respect to the visible row set while the override is still
	// live (visibleItems already applied the same change) — but not with respect
	// to listview's err/loading flags, which SetItems resets, hence refreshItems
	// rather than a direct call. Kept so the rendered list derives from the
	// committed feed instead of relying on the override to keep reproducing it.
	return m.refreshItems()
}

// View renders task 13's four render states, in priority order, per decision
// 63:
//
//  1. capability-unsupported (SetCapabilityUnsupported) — unreachable through
//     the tab in phase 1; see capabilityUnsupportedBody.
//  2. error — a failed List (decisions 17, 63); see errorBody.
//  3. filter-empty — the feed has rows but the active `f` filter matches
//     none of them (the bug decision 57 measured); see filterEmptyBody.
//  4. empty inbox — the feed itself has no rows; see emptyInboxBody.
//
// Otherwise it delegates to listview's table render, appending the
// `Filter: <reason>` indicator whenever the `f` cycle is off its "all
// reasons" position (decision 57): without it, a feed containing no rows of
// the selected reason would render as the plain empty-inbox text while a
// user-set filter is what is hiding everything — which is exactly what state
// 3 above exists to prevent for the *zero-rows* case; the indicator here
// covers the *non-zero* case, where the table itself is still the right
// content but needs the same "a filter is active" disclosure.
func (m Model) View() string {
	if m.capabilityUnsupported {
		return capabilityUnsupportedBody()
	}

	if err := m.list.Err(); err != nil {
		return errorBody(err)
	}

	if !m.list.Loading() && len(m.list.Items()) == 0 {
		if m.reasonFilterActive {
			return filterEmptyBody(m.reasonFilter)
		}
		return emptyInboxBody()
	}

	view := m.list.View()
	if !m.reasonFilterActive {
		return view
	}
	return view + "\n" + m.filterIndicator()
}

// filterIndicator renders the active reason filter, mirroring the
// `Filter: …` shape internal/ui/metrics/list.go already uses for its own
// flag filter. Deliberately unstyled: per convention 6, a state whose text
// needs no emphasis uses the empty lipgloss.NewStyle() rather than reaching
// for a foreground color that could collide with a theme's selection
// background (the Matrix theme sets Foreground == SelectBackground).
func (m Model) filterIndicator() string {
	return "Filter: " + display.NotificationReasonLabel(m.reasonFilter)
}

// SetCapabilityUnsupported puts the pane into decision 63's fourth render
// state.
//
// UNREACHABLE THROUGH THE TAB in phase 1 — nothing in production ever calls
// this. CompositeProvider.HasNotifications gates the tab itself on
// capability (decision 11), and the only backend this pane is ever wired to
// in phase 1 (*github.Adapter, via NewAdapterWithNotifications) satisfies
// provider.NotificationSource unconditionally once constructed — a nil
// NotificationsClient is the *error* arm's nil-client message
// ("github: notifications: no notifications client configured"), not this
// one (decision 63). This method and capabilityUnsupportedBody exist purely
// so the state task 13 names is implemented and testable at the pane level,
// instead of an app-level test that could never fail for the right reason —
// the same trap decision 59 documents for a nil-provider capability fixture.
func (m Model) SetCapabilityUnsupported() Model {
	m.capabilityUnsupported = true
	return m
}

// HandleFetchResult forwards a fetch outcome to the underlying listview.
//
// On success it behaves like SetFeed (task 10's config-filtered feed,
// re-applying whatever `f` position is active per decision 54) so a future
// caller — task 15's poller — gets the same cursor-preserving behavior
// whichever entry point it uses. On failure it puts the pane into task 13's
// error render state without touching the held feed, so a transient failure
// does not discard rows a later successful poll could otherwise have
// resumed showing.
func (m Model) HandleFetchResult(items []provider.Notification, err error) Model {
	if err != nil {
		m.list = m.list.HandleFetchResult(nil, err)
		return m
	}
	return m.SetFeed(items)
}

// emptyInboxBody renders decision 63's first state: the config-filtered feed
// itself has no rows — distinct from filterEmptyBody, where rows exist but
// the active `f` filter hides all of them. Reads as "you're clear", never as
// an error, and per decision 58's `r` stopgap must not tell the user to
// press a key this pane currently swallows.
//
// Keeps listview's original "No notifications found." headline on purpose:
// internal/app/app_test.go's notificationsPaneMarker constant pins that
// exact string as the discriminator between this pane rendering and a
// sibling pane's fall-through (decision 60) — only the misleading "Press r
// to refresh" instruction is removed.
func emptyInboxBody() string {
	return "No notifications found.\n\nYou're all caught up."
}

// filterEmptyBody renders decision 63's second, genuinely reachable state:
// the feed has rows, but the active `f` reason filter matches none of them.
// Rendering emptyInboxBody here is the bug decision 57 measured — it tells
// the user "you're clear" while a filter they set is what is hiding every
// row. Names the active filter and how to clear it; `f` is never swallowed
// (decision 58's stopgap is `r`-only), so telling the user to press it is
// honest.
func filterEmptyBody(reason provider.NotificationReason) string {
	return fmt.Sprintf(
		"No notifications match Filter: %s.\n\nPress f to cycle back to all reasons.",
		display.NotificationReasonLabel(reason),
	)
}

// errorBody renders decision 63's fourth state: a failed List call. It
// carries decision 17's token-scope skeleton — a static line naming the
// GitHub scope every List call needs — and folds the underlying error
// (including the adapter's nil-client message,
// "github: notifications: no notifications client configured", when no
// NotificationsClient was wired at construction) into the body verbatim.
//
// FORWARD: task 19 replaces this flat skeleton with real differentiation —
// recovering *github.APIError via errors.As to tell a 403 missing-scope
// response apart from a 401-expired token and a generic failure, plus the
// in-view "disable this pane" action that writes disabled_panes via
// Config.Save(). Do not build that branching here.
func errorBody(err error) string {
	return fmt.Sprintf(
		"Notifications unavailable: %v\n\nGitHub token scope required: notifications",
		err,
	)
}

// capabilityUnsupportedBody renders decision 63's third, unreachable-in-phase-1
// state. See SetCapabilityUnsupported's doc comment for why nothing in
// production ever reaches this.
func capabilityUnsupportedBody() string {
	return "Notifications are not supported by this configuration.\n\nNo configured backend implements the notifications capability."
}

// ReasonFilter reports the `f` cycle's current position: the selected
// NotificationReason and whether a reason filter is active at all. A false
// second return is the "all reasons" position (decision 57) — exported
// because tasks 13 and 16 cannot tell an empty inbox from a filter that hides
// every row without it.
func (m Model) ReasonFilter() (provider.NotificationReason, bool) {
	return m.reasonFilter, m.reasonFilterActive
}

// GetContextItems returns context bar items for the current view. Phase 1
// has no detail view (EnterDetail is a no-op stub), so this forwards straight
// to the underlying listview with no branching, unlike pullrequests.Model's
// diff-view special case.
func (m Model) GetContextItems() []components.ContextItem {
	return m.list.GetContextItems()
}

// GetScrollPercent returns the scroll percentage for the current view.
func (m Model) GetScrollPercent() float64 {
	return m.list.GetScrollPercent()
}

// GetStatusMessage returns the status message for the current view.
func (m Model) GetStatusMessage() string {
	return m.list.GetStatusMessage()
}

// HasContextBar returns true if the current view should show a context bar.
func (m Model) HasContextBar() bool {
	return m.list.HasContextBar()
}

// IsSearching returns true if the view has an active text input that should
// suppress global keyboard shortcuts. Decision 57 sets no FilterFunc in
// phase 1, so this always forwards false — kept for symmetry with every
// other pane's app.isActiveViewCapturingInput wiring.
func (m Model) IsSearching() bool {
	return m.list.IsSearching()
}

// SetFeed sets the config-filtered feed (task 10's FilterNotifications
// output) and re-applies whatever `f` reason filter is currently active on
// top of it, routing the result through listview.SetItems per decision 54.
func (m Model) SetFeed(feed []provider.Notification) Model {
	m.feed = feed
	m.overrides = prunedOverrides(m.overrides, m.clock())
	return m.setItemsPreservingSelection()
}

// prunedOverrides drops entries whose debounce window has already elapsed.
// visibleItems and markDone both already skip an expired entry, so this changes
// no behaviour — it exists so the map does not grow monotonically for the life
// of the session, since the success path keeps its entry and only a failure
// removes one. SetFeed is the hook because it already rebuilds everything.
//
// Returns nil for an empty result rather than an empty map, so visibleItems'
// len(m.overrides) == 0 fast path (which aliases m.feed instead of copying it)
// comes back once every mark has settled.
func prunedOverrides(overrides map[identityKey]override, now time.Time) map[identityKey]override {
	if len(overrides) == 0 {
		return overrides
	}
	var next map[identityKey]override
	for k, v := range overrides {
		if !now.Before(v.expiresAt) {
			continue
		}
		if next == nil {
			next = make(map[identityKey]override, len(overrides))
		}
		next[k] = v
	}
	return next
}

// cycleReasonFilter advances the `f` cycle by one step and re-applies it.
// The cycle visits reasons present in m.feed, in enum order (decision 53) —
// never feed order (which reshuffles every poll per decision 45) and never
// the full enum (which would offer Unknown, a value no mapped row can
// carry — decision 18). The "all reasons" position is the implicit reset:
// cycling past the last present reason returns to it, and an empty feed
// (or a feed with no reasons at all) also resets to it defensively.
func (m Model) cycleReasonFilter() Model {
	present := presentReasons(m.feed)

	switch {
	case len(present) == 0:
		m.reasonFilterActive = false
	case !m.reasonFilterActive:
		m.reasonFilterActive = true
		m.reasonFilter = present[0]
	default:
		idx := indexOfReason(present, m.reasonFilter)
		if idx < 0 || idx == len(present)-1 {
			m.reasonFilterActive = false
		} else {
			m.reasonFilter = present[idx+1]
		}
	}

	return m.setItemsPreservingSelection()
}

// refreshItems re-derives the visible rows from feed+overrides, unless the pane
// is currently showing listview's error state.
//
// listview.SetItems assigns m.err = nil and m.loading = false as a side effect
// (internal/ui/components/listview/listview.go:379-392), so an override-driven
// re-derivation landing while a fetch has failed silently replaces the error
// render with "No notifications found. / You're all caught up." — telling the
// user their inbox is clear when the fetch actually failed. That is precisely
// the lie decisions 57 and 63 exist to prevent, and it is worse than either,
// because the error it hides is the one explaining why there is no data.
//
// Reachable as: press `d`, a poll fails, then the DELETE result lands. Only the
// mark-result paths need this — canTriage already refuses `u`/`d` while
// m.list.Err() != nil, so withOverride cannot be entered in the error state;
// it routes through here anyway so no future caller has to rediscover the rule.
//
// SetFeed deliberately does NOT route through here: a successful fetch clearing
// a previous error is the recovery path, and it is asserted by
// TestView_SuccessfulFeedAfterError_ClearsErrorState.
//
// FORWARD: task 15 — nothing calls HandleFetchResult with an error yet, so this
// is unreachable today and becomes live the moment the real poller lands.
func (m Model) refreshItems() Model {
	if m.list.Err() != nil {
		return m
	}
	return m.setItemsPreservingSelection()
}

// setItemsPreservingSelection hands the current override-applied,
// `f`-filtered view to listview.SetItems (decision 54) and restores the
// cursor onto the *same item* it was on before, by Identity.SameItem
// (decision 55).
//
// listview.setColumnsAndRows restores the cursor purely positionally — it
// saves table.Cursor() and re-applies it clamped to the new row count — so
// narrowing the feed silently moves the selection to a different row.
// Decision 45 canonicalised the merge's sort order to protect exactly this
// index-held cursor; an in-pane filter reintroduces the hazard from the other
// direction, and task 14's `d` would then mark the wrong row done. This same
// call is what applies a fresh `u`/`d` override immediately (the "optimistic"
// half of task 14) and what re-applies one after a poll (SetFeed) or a
// rollback (dropOverride) — a single call site for every path that changes
// the visible row set, per convention 7.
//
// When the previously selected item did not survive the filter there is
// nothing to restore to, and listview's clamp is the correct behaviour — so
// this deliberately leaves it alone in that case. TestMarkDone_CursorLandsOnA
// ConcreteRow pins the resulting positions; the restore itself is pinned by
// TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives.
func (m Model) setItemsPreservingSelection() Model {
	prev, hadSelection := m.selectedIdentity()

	m.list = m.list.SetItems(m.visibleItems())

	if hadSelection {
		if idx := m.list.FindIndex(func(n provider.Notification) bool {
			return n.Identity.SameItem(prev)
		}); idx >= 0 {
			m.list.SetCursor(idx)
		}
	}
	return m
}

// selectedItem returns the notification currently under the cursor and
// whether there was one (an empty list, or a cursor listview has clamped to
// -1, yields false).
func (m Model) selectedItem() (provider.Notification, bool) {
	items := m.list.Items()
	idx := m.list.SelectedIndex()
	if idx < 0 || idx >= len(items) {
		return provider.Notification{}, false
	}
	return items[idx], true
}

// selectedIdentity returns the Identity of the row currently under the cursor
// and whether there was one.
func (m Model) selectedIdentity() (provider.Identity, bool) {
	item, ok := m.selectedItem()
	if !ok {
		return provider.Identity{}, false
	}
	return item.Identity, true
}

// visibleItems returns the rows the table should render: the `f`-filtered
// feed (reasonFiltered) with any active `u`/`d` override applied on top —
// overrideHidden rows dropped, overrideRead rows shown with Read forced true.
// An override past its debounce window is treated as expired and skipped, so
// the polled feed's own data wins again (Decision 5: this is a short-lived
// buffer, not persisted local state) — this is exactly what makes "the
// debounce window" a real, finite window rather than a permanent override.
//
// Expiry is safe for a mark whose result has been *applied*: commitOverride
// folded it into m.feed, so the entry left here is redundant with the feed and
// letting it lapse changes nothing. The entry is deliberately kept until then
// (see commitOverride) — it is what outweighs a poll replacing the feed with
// stale `unread`.
//
// Expiry is NOT safe for a mark whose result never arrived. Such an override is
// the sole holder of the mark, so lapsing it re-derives the pre-mark row from an
// unchanged m.feed with no poll involved. For an *in-flight* mark that is
// correct — an unconfirmed action reverting after 30s is the intended contract
// (Decision 5: no persisted local state). It is a defect only if a confirmed
// result was dropped in transit, which is why app must route MarkResultMsg
// unconditionally rather than through its active-tab delegate switch.
//
// Always a freshly allocated slice when any override is active, for the same
// reason reasonFiltered is: the pane's held feed must never be aliased or
// mutated by a later step.
func (m Model) visibleItems() []provider.Notification {
	base := m.reasonFiltered()
	if len(m.overrides) == 0 {
		return base
	}

	now := m.clock()
	out := make([]provider.Notification, 0, len(base))
	for _, n := range base {
		ov, ok := m.overrides[keyOf(n.Identity)]
		if !ok || !now.Before(ov.expiresAt) {
			out = append(out, n)
			continue
		}
		if ov.kind == overrideHidden {
			continue
		}
		n.Read = true
		out = append(out, n)
	}
	return out
}

// reasonFiltered returns the rows visible under the current `f` position: the
// whole feed when no reason filter is active, otherwise only rows whose
// Reason matches. Always a freshly allocated slice when narrowing, so the
// pane's held feed is never aliased or mutated by a subsequent filter step.
func (m Model) reasonFiltered() []provider.Notification {
	if !m.reasonFilterActive {
		return m.feed
	}
	out := make([]provider.Notification, 0, len(m.feed))
	for _, n := range m.feed {
		if n.Reason == m.reasonFilter {
			out = append(out, n)
		}
	}
	return out
}

// presentReasons returns the NotificationReason values that appear in feed,
// in enum declaration order. NotificationReasonUnknown (the zero value) is
// always excluded: the wire mapper never emits it (Decision 18), and
// including it would offer a cycle stop that can never match a real row.
// Iterating provider.NotificationReasonCount() rather than a hardcoded slice
// means the cycle stays in sync automatically if the enum ever grows.
func presentReasons(feed []provider.Notification) []provider.NotificationReason {
	seen := make(map[provider.NotificationReason]bool, len(feed))
	for _, n := range feed {
		seen[n.Reason] = true
	}

	out := make([]provider.NotificationReason, 0, provider.NotificationReasonCount())
	for i := 1; i < provider.NotificationReasonCount(); i++ { // start at 1: skip Unknown (0)
		r := provider.NotificationReason(i)
		if seen[r] {
			out = append(out, r)
		}
	}
	return out
}

// indexOfReason returns the index of target in reasons, or -1 if absent.
func indexOfReason(reasons []provider.NotificationReason, target provider.NotificationReason) int {
	for i, r := range reasons {
		if r == target {
			return i
		}
	}
	return -1
}

// multiRepo reports whether items span more than one distinct
// Identity.Scope, via display.MultiScope. Both toRows and toColumns compute
// it from the exact same slice they're each called with, so the dynamic
// Repo column's presence and the per-row Repo cell can never diverge
// (convention 7).
func multiRepo(items []provider.Notification) bool {
	scopes := make([]string, len(items))
	for i, n := range items {
		scopes[i] = n.Identity.Scope
	}
	return display.MultiScope(scopes)
}

// toColumns derives the notifications list's column specs from the current
// items: [Repo?] [Reason] [Title] [Updated].
func toColumns(items []provider.Notification) []listview.ColumnSpec {
	cols := make([]listview.ColumnSpec, len(baseColumns))
	copy(cols, baseColumns)

	if multiRepo(items) {
		cols = append([]listview.ColumnSpec{repoColumn}, cols...)
	}

	listview.NormalizeWidths(cols)
	return cols
}

// toRows converts notifications to table rows, mirroring toColumns's layout
// and gating predicate exactly: [Repo?] [Reason] [Title] [Updated].
//
//   - The Repo cell (when present) falls back to "—" for an empty
//     ScopeDisplay: decision 35 defaults it to Scope at the adapter
//     boundary, but a thread whose repository payload is absent leaves both
//     empty, the one case the mapper cannot fix.
//   - The Title cell is rendered through titleStyle's named style —
//     styles.Styles.Title (bold) for unread rows, styles.Styles.Value for
//     read ones — so unread emphasis is a named style rather than an inline
//     lipgloss.NewStyle() (convention 6). An empty Title dashes to "—"
//     before styling (decision 58).
//   - The Updated cell renders "—" for a zero UpdatedAt (the mapper leaves it
//     zero when the wire omits updated_at), never a year-0001 date.
func toRows(items []provider.Notification, s *styles.Styles) []table.Row {
	multi := multiRepo(items)

	rows := make([]table.Row, len(items))
	for i, n := range items {
		reasonCell := display.NotificationReasonStyle(n.Reason, s).
			Render(display.NotificationReasonGlyph(n.Reason) + " " + display.NotificationReasonLabel(n.Reason))

		cells := table.Row{
			reasonCell,
			titleCell(n, s),
			formatUpdatedAt(n.UpdatedAt),
		}
		if multi {
			cells = append(table.Row{dashIfEmpty(n.Identity.ScopeDisplay)}, cells...)
		}
		rows[i] = cells
	}
	return rows
}

// titleStyle returns the style the Title cell is rendered with: unread rows get
// the named styles.Styles.Title (Primary + Bold — the unread emphasis), read
// rows get an empty style, whose Render is the identity function and emits no
// escape sequence at all.
//
// The read branch must stay empty rather than resolve to a named foreground
// style. Every sibling pane emits its title cell bare (pullrequests/list.go:531)
// and lets the table style it, and that is not merely convention: table.renderRow
// wraps each cell in styles.Cell, inheriting styles.Selected on the cursor row,
// so an inner foreground SGR overrides the selection's own foreground for that
// one cell. styles.Value is Foreground(theme.Foreground), which is invisible on
// the selected row of any theme where Foreground equals SelectBackground — the
// Matrix theme sets both to #00ff41 (themes.go:467, :474). Unread rows accept
// that trade deliberately, because persisting the emphasis is the point; read
// rows have nothing to gain from it.
//
// This is split out from titleCell on purpose (decision 56). lipgloss resolves
// the Ascii profile in a test binary, so Render is the identity function there
// and a rendered-bytes comparison of styled vs. unstyled output is vacuous —
// deleting the unread branch entirely left the suite green. Returning the
// lipgloss.Style itself makes convention 6 assertable on the *style object*
// (GetBold/GetForeground against styles.Styles' own fields, plus that the two
// branches resolve to different styles), which no color profile can flatten.
func titleStyle(n provider.Notification, s *styles.Styles) lipgloss.Style {
	if !n.Read {
		return s.Title
	}
	return lipgloss.NewStyle()
}

// titleCell returns the Title column's cell text for a notification, rendered
// through titleStyle's named style.
//
// An empty Title falls back to "—" (decision 58), dashed *before* the style is
// applied so an untitled unread row still shows the dash: Title comes verbatim
// from thread.Subject.Title with no wire-level fallback, and a blank cell in
// the 60%-width column reads as a rendering bug rather than as missing data.
func titleCell(n provider.Notification, s *styles.Styles) string {
	return titleStyle(n, s).Render(dashIfEmpty(n.Title))
}

// dashIfEmpty returns "—" for an empty string, val otherwise.
func dashIfEmpty(val string) string {
	if val == "" {
		return "—"
	}
	return val
}

// formatUpdatedAt renders a notification's UpdatedAt for display. A zero
// time (the mapper's sentinel for "the wire omitted updated_at") renders as
// "—", never as a year-0001 date.
func formatUpdatedAt(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}
