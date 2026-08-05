package notifications

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Elpulgo/azdo/internal/browser"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openURL is a package-level seam so tests can intercept browser launches,
// mirroring internal/ui/metrics/list.go and pullrequests/detail.go.
var openURL = browser.Open

// SetOpenURLForTesting substitutes the openURL seam from outside this
// package, mirroring polling.NotificationsPoller's own SetEveryForTesting.
// Needed because internal/app's tests drive a real browser-launch failure
// through the full app.Model to exercise the status-bar visibility and
// footer-resize behaviour end to end, and openURL is unexported — a
// same-package test (list_test.go's withOpenURLSpy) has no such need since
// it can reassign the var directly. The caller must invoke the returned
// restore func, typically via t.Cleanup.
func SetOpenURLForTesting(fn func(string) error) (restore func()) {
	prev := openURL
	openURL = fn
	return func() { openURL = prev }
}

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
	// Task 16's UnreadCount() applies these same overrides (via applyOverrides)
	// over m.feed, so a successful `u` clears the row on screen and decrements
	// the footer badge in the same tick — the count and the rows never
	// disagree, per decision 68.
	overrides map[identityKey]override

	// now lets tests replace time.Now for deterministic debounce-window
	// assertions, mirroring internal/ui/metrics/list.go's own now field.
	// Always set by NewModelWithStyles; use the clock() accessor rather than
	// calling m.now directly so a zero-value Model (Decision 61's documented
	// latent hazard — never reachable through production code today) cannot
	// nil-deref here even if some future caller reaches this path.
	now func() time.Time

	// statusMessage carries the outcome of the last `o` (open in browser)
	// attempt, mirroring internal/ui/metrics/list.go's own statusMessage
	// field (metrics has the identical dead-rendering-path bug this field's
	// own consumer, app.syncNotificationsActionMessage, exists to work
	// around for this pane — out of scope to also fix for metrics). Phase 1
	// has no detail view (EnterDetail is a no-op stub), so unlike
	// pullrequests.DetailModel this is the pane's only status-message
	// surface — GetStatusMessage returns it in preference to
	// m.list.GetStatusMessage(), which always reports "" in list mode.
	//
	// Only ever non-empty for `o`'s two failure outcomes (empty WebURL, a
	// failed browser launch), a failed `u`/`d` (task 19, task 14 reviewer
	// finding 6), and the disable action's outcome (task 19, decision 17) —
	// success sets it back to "" for `o` and for a mark (see handleMarkResult),
	// a deliberately silent outcome for `o` (the browser window appearing is
	// the feedback) that doubles as one of two clearing triggers. The other is
	// HandleFetchResult, which resets it unconditionally on every fetch, so a
	// stale failure message never survives "the user navigated away and a
	// poll refreshed the feed while they were gone."
	statusMessage string

	// cfg is the config the disable action (task 19, decision 17, Part B)
	// mutates and saves. It is the same *config.Config the rest of app.Model
	// holds — NewModelWithStyles never copies it — so a successful disable is
	// visible to the rest of the app immediately, even though (per disablePane's
	// doc comment) it only changes what the *next* restart's tab list looks
	// like. May be nil (NewModel's zero-config path, and any pane built before
	// a config existed); disablePane reports failure rather than panicking.
	cfg *config.Config

	// disableConfirmPending is the confirm gate for the `x` disable action
	// (task 19, decision 17, Part B): true only in the window between the
	// first (arming) press and the second (confirming) press, so a single
	// keypress can never write the user's config. Reset by any key other than
	// a second x, and unconditionally by HandleFetchResult, so an arm can
	// never survive past the error occurrence that raised it into some later,
	// unrelated error.
	disableConfirmPending bool
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

// openURLResultMsg reports the outcome of an `o` (open in browser) attempt
// (decision 3). Unlike MarkResultMsg it needs no cross-tab routing: a
// browser launch is a near-instant OS call, not an API round trip the user
// is likely to have switched tabs during, so app forwards it like any other
// message — only while the notifications tab is active.
type openURLResultMsg struct {
	err error
}

// notificationColumns are the notifications list's per-row column specs:
// [•] [Repo] [Reason] [Title] [Updated]. Defined once and copied inside
// toColumns to avoid mutating the package-level slice (mirrors
// prBaseColumns).
//
// Unlike every sibling pane, the Repo column here is NOT gated on the item
// slice spanning multiple scopes (display.MultiScope, as convention 7's
// example describes). Two reasons, both specific to this pane:
//
//   - The whole point of this tab is a merged cross-repo inbox, so "which
//     repo is this about" is primary context, not redundant detail.
//   - The gate made the column vanish exactly when a filter narrowed the
//     feed to one repo — so turning on only_configured_repos, or cycling the
//     `f` reason filter down to a single repo's rows, silently removed the
//     answer to the question the filter was asked in service of. A column
//     that disappears as a side effect of filtering reads as a bug.
//
// Keeping it unconditional also makes convention 7 trivially satisfied: with
// no predicate, toColumns and toRows cannot disagree about the column count.
//
// readColumn carries the unread marker. It has a blank header because the
// glyph is self-describing and a label would cost more width than the column
// itself uses.
// The Reason column is wider than the 20% it had before the marker column
// existed: its widest label, "◉ Approval requested", is 20 cells, and the
// marker's share came out of Reason's under the old split — truncating the
// most common label ("◐ Review requested") to "◐ Review reques…". The extra
// width is taken from Title, which has the most slack.
var notificationColumns = []listview.ColumnSpec{
	{Title: "", WidthPct: 4, MinWidth: 2},
	{Title: "Repo", WidthPct: 20, MinWidth: 10},
	{Title: "Reason", WidthPct: 24, MinWidth: 12},
	{Title: "Title", WidthPct: 52, MinWidth: 20},
	{Title: "Updated", WidthPct: 18, MinWidth: 10},
}

// unreadGlyph marks a row whose effective Read state is false. Read rows
// render a blank cell rather than a second glyph: an inbox is mostly-read in
// steady state, so marking the exception keeps the column quiet, and a
// "read" glyph would compete with the Reason column's own glyph for
// attention.
//
// Decision 56's caveat applies here exactly as it does to titleStyle: a
// rendered-bytes comparison cannot distinguish styled from unstyled output
// in a test binary, so the emphasis is asserted on the style object while
// the glyph itself — a plain rune, not an escape sequence — is asserted on
// the cell text.
const unreadGlyph = "●"

// Cell indices into the row layout built by toRows, matching
// notificationColumns position for position. Named so that a column added or
// reordered is a single edit here plus notificationColumns, rather than a
// hunt for magic numbers across the render path and its tests.
// TestColumnOrder_MatchesCellIndices pins the two in step.
const (
	cellRead = iota
	cellRepo
	cellReason
	cellTitle
	cellUpdated
	cellCount
)

// NewModel creates a new notifications pane model with default styles, no
// marker (u/d are no-ops until a real one is injected), and no config (the
// fetch derives NotifOpts from a nil config, which NotifOptsFromConfig
// treats as the zero value).
func NewModel() Model {
	return NewModelWithStyles(styles.DefaultStyles(), nil, nil)
}

// NewModelWithStyles creates a new notifications pane model with custom
// styles, the marker used for `u`/`d` (task 14), and the config used to
// derive fetch-time NotifOpts (task 15, decision 52). marker may be nil —
// see Model.marker's doc comment; that is a reachable state, not a caller
// error. cfg may also be nil (NotifOptsFromConfig's own nil contract).
//
// The constructed pane starts in listview's loading state (decision 64):
// Init() cannot mutate model state (tea.Model.Init has a value receiver), so
// the spinner has to be turned on here, at construction, to cover the gap
// between the model existing and Init()'s own fetch cmd resolving — without
// this, Loading() reads false and View()'s empty-inbox render fires for the
// whole of that window.
func NewModelWithStyles(s *styles.Styles, marker provider.NotificationSource, cfg *config.Config) Model {
	lvCfg := listview.Config[provider.Notification]{
		LoadingMessage: "Loading notifications...",
		EntityName:     "notifications",
		MinWidth:       50,
		ToRows:         toRows,
		ToColumns:      toColumns,
		Fetch:          fetchNotifications(marker, cfg),
		// No detail view in phase 1 (`o` opens the browser per decision 3;
		// there is nothing else to drill into). A harmless no-op stub avoids
		// a nil-func panic if enter is pressed, without building real
		// navigation (out of scope for this task).
		EnterDetail: func(item provider.Notification, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			return nil, nil
		},
	}

	return Model{
		list:   listview.New(lvCfg, s).SetLoading(true),
		marker: marker,
		now:    time.Now,
		cfg:    cfg,
	}
}

// notificationsFetchMsg is the private, pane-owned result of the pane's own
// Fetch closure — the result of Init()'s initial fetch and of a real `r`
// refresh (FORWARD item 1). It is deliberately unexported, mirroring
// internal/ui/pipelines/list.go's pipelineRunsMsg: the app-level poller's
// own fetch result is a separate, exported message
// (polling.NotificationsFetchedMsg) that reaches this pane through
// HandleFetchResult directly rather than through this type, so a stale tab
// switch cannot misroute one kind of result as the other.
type notificationsFetchMsg struct {
	items []provider.Notification
	err   error
}

// fetchNotifications returns the listview.Config.Fetch closure this pane's
// own Init()/`r` path uses: it calls marker.List with cfg-derived NotifOpts
// (decision 52) and applies task 10's config filter to a successful result,
// mirroring exactly what HandleFetchResult expects a caller to have already
// done for a poller-driven result. A nil marker (Decision 61's reachable
// capability-absent/disabled-pane state) fetches nothing and reports no
// error — there is no capability to report a failure about.
func fetchNotifications(marker provider.NotificationSource, cfg *config.Config) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			if marker == nil {
				return notificationsFetchMsg{}
			}
			items, err := marker.List(NotifOptsFromConfig(cfg))
			if err != nil {
				return notificationsFetchMsg{err: err}
			}
			return notificationsFetchMsg{items: FilterNotifications(items, cfg)}
		}
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
// API call issued by markCmd) and notificationsFetchMsg (the result of this
// pane's own Init()/`r`-triggered fetch) are both handled unconditionally,
// since either can land regardless of view mode or search state.
// openURLResultMsg (the result of an `o` browser-launch attempt) is likewise
// handled unconditionally here rather than in the key-guarded switch below,
// since it is not itself a tea.KeyMsg. Otherwise: the `f` key cycles the
// reason filter (decision 53), `u`/`d` mark read/done (task 14) and `o` opens
// the selected row's WebURL (decision 3) when canTriage allows it, `r` now
// reaches listview's own real refresh handling (decision 58's stopgap is
// gone — fetchNotifications is a real Fetch hook, not a stub), and every
// other message is forwarded to the underlying listview.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if res, ok := msg.(MarkResultMsg); ok {
		return m.handleMarkResult(res), nil
	}
	if res, ok := msg.(notificationsFetchMsg); ok {
		return m.HandleFetchResult(res.items, res.err), nil
	}
	if res, ok := msg.(openURLResultMsg); ok {
		if res.err != nil {
			m.statusMessage = "Failed to open browser: " + res.err.Error()
		} else {
			// Silent success (decision 3, reviewer finding): the browser
			// window appearing is the feedback. Setting statusMessage to ""
			// here also doubles as "the next successful action clears it" —
			// one of two clearing triggers, alongside HandleFetchResult's own
			// unconditional reset on every fetch below — so a prior failure
			// message never lingers past the next `o` that actually works.
			m.statusMessage = ""
		}
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && !m.list.IsSearching() && m.list.GetViewMode() == listview.ViewList {
		if m.disableConfirmPending && key.String() != disablePaneKey {
			// Any key other than a second disablePaneKey cancels the arm.
			// Falls through to whatever that key would otherwise do below —
			// harmless in practice, since u/d/o are already refused by
			// canTriage while the pane is errored, which is the only state
			// this arm is reachable from.
			m.disableConfirmPending = false
		}

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
		case "o":
			if !m.canTriage() {
				return m, nil
			}
			return m.openInBrowser()
		case disablePaneKey:
			return m.handleDisableKey(), nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// canTriage reports whether `u`/`d`/`o` make sense right now: there must be a
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

// openInBrowser opens the selected row's WebURL (decision 3). The row comes
// from selectedItem, which reads m.list.Items() — the filtered/overridden
// view the user is actually looking at (visibleItems' output), never the raw
// m.feed — so an active `f` reason filter can never make `o` open something
// other than the highlighted row. The !ok guard mirrors markRead/markDone's
// own defensive re-check even though the call site already gated on
// canTriage.
//
// An empty WebURL (types.go's documented "no usable repository URL at all"
// case, task 5's fallback chain exhausted) is treated as nothing to open,
// per that field's contract — not attempted, and openURL is never called for
// it. A status message tells the user why nothing happened rather than
// staying silent.
func (m Model) openInBrowser() (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		return m, nil
	}
	if item.WebURL == "" {
		m.statusMessage = "No URL for this notification"
		return m, nil
	}
	return m, func() tea.Msg {
		return openURLResultMsg{err: openURL(item.WebURL)}
	}
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

// handleMarkResult applies the outcome of a markCmd: failure rolls the
// optimistic change back by dropping the override, success commits it into the
// held feed via commitOverride while deliberately leaving the override entry
// in place.
//
// Part C of task 19 closes reviewer finding 6 here: a failed `u`/`d` used to
// roll back silently, so a user pressing `d` against a 403 saw the row vanish
// and silently reappear a round trip later, indistinguishable from a
// rendering glitch. Failure now sets m.statusMessage to a message naming the
// action and the underlying error; app.go's syncNotificationsActionMessage
// (which already implements decisions 81/82 correctly) mirrors it onto the
// status bar. A success clears statusMessage rather than leaving a stale
// failure message from an earlier action lingering after a later one
// succeeds.
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
		m.statusMessage = markFailureMessage(res.kind, res.err)
		return m.dropOverride(res.key)
	}
	// Deliberately also commits when no entry remains: a success means the
	// server accepted the change, so correcting the feed is right even if the
	// entry was pruned or the user has since acted elsewhere. Erring the other
	// way would resurrect a row the server has already dropped.
	m.statusMessage = ""
	return m.commitOverride(res.key, res.kind)
}

// markFailureMessage names the failed action (mark read vs. mark done) and
// folds in the underlying error, so the status bar tells a `u` failure apart
// from a `d` failure rather than a single generic "mark failed".
func markFailureMessage(kind overrideKind, err error) string {
	action := "Mark done"
	if kind == overrideRead {
		action = "Mark read"
	}
	return fmt.Sprintf("%s failed: %v", action, err)
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
//  2. error — a failed List (decisions 17, 63), differentiated per task 19
//     into a 403-missing-scope, a 401-expired-token and a generic render (see
//     errorBody) — or, while the `x` disable action's confirm is armed, task
//     19/decision 17's confirmation overlay instead (see disableConfirmBody).
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
		if m.disableConfirmPending {
			return disableConfirmBody()
		}
		return errorBody(err)
	}

	if !m.list.Loading() && len(m.list.Items()) == 0 {
		// len(m.feed) > 0, not len(m.list.Items()) == 0 alone: the discriminant
		// must be "does the underlying feed genuinely have rows the filter is
		// hiding", not "is the rendered table empty" — the latter is also true
		// for a genuinely empty inbox with a stale-but-active `f` position
		// (e.g. the previously-selected reason's last row was marked done),
		// which must still read as "you're clear", not "a filter is hiding
		// something" (decisions 57, 63).
		if m.reasonFilterActive && len(m.feed) > 0 {
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
//
// Also clears statusMessage unconditionally, success or failure alike
// (decision 3, reviewer finding): a fresh fetch is "the next fetch" clearing
// trigger for a stale `o` outcome — the scenario the reviewer named
// explicitly ("after ... a poll refreshes the feed"). Cleared here rather
// than only on the next successful `o` so the message does not survive
// indefinitely on an inbox the user has since navigated away from and back
// to, or one a poll has since moved on from entirely.
//
// Also resets disableConfirmPending unconditionally (task 19, decision 17,
// Part B): a fresh fetch means whichever error occurrence armed the confirm
// is over — success clears the error state outright, and even a repeat
// failure deserves its own fresh confirm rather than letting a stale arm from
// a *previous* failure silently confirm on the next `x` press against an
// unrelated one.
func (m Model) HandleFetchResult(items []provider.Notification, err error) Model {
	m.statusMessage = ""
	m.disableConfirmPending = false
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

// classicTokenSettingsURL is the only token page these error bodies ever
// name. GitHub's notifications endpoints "only support authentication using
// a personal access token (classic)" and require the `notifications` or
// `repo` scope — a fine-grained token cannot reach this API at all, and no
// account permission exists to grant it. So there is no fine-grained remedy
// to offer: every fixable failure here is fixed with a classic token.
const classicTokenSettingsURL = "https://github.com/settings/tokens"

// errorBody renders decision 63's fourth state: a failed List call. Task 19
// recovers *github.APIError via errors.As to tell three cases apart, each
// with its own distinct render so decision 63's mutual-distinguishability
// rule holds pairwise:
//
//   - A 403 that is not rate-limiting: scopeErrorBody — names the
//     "notifications" scope and how to add it.
//   - A 401: expiredTokenErrorBody — the token itself is rejected, not a
//     scope gap; re-adding a scope would not fix this.
//   - Everything else, including a non-*APIError (e.g. the adapter's
//     nil-client message, "github: notifications: no notifications client
//     configured", when no NotificationsClient was wired at construction):
//     genericErrorBody. This case must never show the scope banner — doing
//     so for, say, a network timeout or a missing client would be actively
//     misleading.
//
// All three end with disableHint, the shared "press x to disable this pane"
// pointer into task 19's Part B — a shared suffix does not erase each
// variant's distinct, unique-substring identifying content above it.
func errorBody(err error) string {
	var apiErr *github.APIError
	if errors.As(err, &apiErr) {
		switch {
		case !apiErr.RateLimited && apiErr.StatusCode == http.StatusForbidden:
			return scopeErrorBody(apiErr)
		case apiErr.StatusCode == http.StatusUnauthorized:
			return expiredTokenErrorBody(err)
		}
	}
	return genericErrorBody(err)
}

// scopeErrorBody renders a 403 that is not rate-limiting. GitHub sends
// X-Accepted-OAuth-Scopes/X-OAuth-Scopes only for classic PATs, so their
// absence is itself the diagnosis: the token is almost certainly
// fine-grained, and a fine-grained token can never reach this API — the
// notifications endpoints accept classic tokens only, and there is no
// account permission that enables them.
//
// The two branches therefore give genuinely different remedies rather than
// two shades of the same one: with headers, add a scope to the token you
// have; without them, the token flavor itself is wrong and must be replaced.
// An earlier version told the headers-absent user to "check the token's
// notifications permission" on the fine-grained settings page. No such
// permission exists, so that sent them hunting through a list that could
// not contain the answer.
func scopeErrorBody(apiErr *github.APIError) string {
	if apiErr.RequiredScopes == "" && apiErr.GrantedScopes == "" {
		return fmt.Sprintf(
			"Notifications unavailable: %v\n\n"+
				"GitHub token scope required: notifications\n\n"+
				"GitHub reported no token scopes with this response, which is what a "+
				"fine-grained personal access token looks like. GitHub's notifications "+
				"API accepts classic tokens only — no fine-grained permission enables "+
				"it. Create a classic token with the notifications scope at %s.\n\n%s",
			apiErr, classicTokenSettingsURL, disableHint(),
		)
	}
	return fmt.Sprintf(
		"Notifications unavailable: %v\n\n"+
			"GitHub token scope required: notifications\n\n"+
			"Granted scopes: %s\nRequired scopes: %s\n\n"+
			"Add the notifications scope to this classic token at %s.\n\n%s",
		apiErr, orNone(apiErr.GrantedScopes), orNone(apiErr.RequiredScopes),
		classicTokenSettingsURL, disableHint(),
	)
}

// expiredTokenErrorBody renders a 401: the token itself was rejected
// (expired, revoked, or malformed), not a scope gap. Deliberately does not
// mention scopes at all — telling the user to add a scope to a token GitHub
// no longer accepts at all would be wrong.
//
// Names only the classic settings page. This body is rendered inside the
// notifications pane, and a fine-grained replacement token would leave this
// pane broken however valid it is for the app's other tabs.
func expiredTokenErrorBody(err error) string {
	return fmt.Sprintf(
		"Notifications unavailable: %v\n\n"+
			"Your GitHub token appears to be expired or invalid. Generate a new "+
			"classic token with the notifications scope at %s and update your "+
			"config.\n\n%s",
		err, classicTokenSettingsURL, disableHint(),
	)
}

// genericErrorBody renders every other failure: non-*APIError errors (the
// nil-client message chief among them), rate-limited 403s, and any status
// code that is neither 403 nor 401. Never shows the scope banner — this is
// exactly the case task 19 stops attaching it to falsely.
func genericErrorBody(err error) string {
	return fmt.Sprintf("Notifications unavailable: %v\n\n%s", err, disableHint())
}

// disableHint is the shared closing line across all three error renders,
// pointing at task 19 Part B's in-view disable action. Takes effect at the
// next restart (decision 61: buildEnabledTabs computes the tab list once at
// NewModel construction), which this sentence says outright rather than
// implying the tab disappears immediately.
func disableHint() string {
	return fmt.Sprintf("Press %s to disable this pane (takes effect on next restart).", disablePaneKey)
}

// orNone renders an empty header value as "none" rather than a blank
// string, so scopeErrorBody's "Granted scopes: " line never looks like a
// rendering bug when GitHub reports an empty (but present) scope list.
func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// capabilityUnsupportedBody renders decision 63's third, unreachable-in-phase-1
// state. See SetCapabilityUnsupported's doc comment for why nothing in
// production ever reaches this.
func capabilityUnsupportedBody() string {
	return "Notifications are not supported by this configuration.\n\nNo configured backend implements the notifications capability."
}

// disablePaneKey is task 19 Part B's in-view disable action, reachable only
// from the error state (see errorBody's disableHint and View()'s
// disableConfirmPending branch). Chosen deliberately free of every key this
// pane and app.go's top-level switch already reserve: not d/u/o (triage),
// not f (reason filter), not r (decision 58's refresh stopgap), and not
// app.go's q/ctrl+c/?/t/1-5/left/right.
const disablePaneKey = "x"

// disableConfirmBody renders the one-press-armed, confirm-on-second-press
// overlay shown in place of errorBody while m.disableConfirmPending is true.
// No existing confirm pattern was found elsewhere in this codebase to model
// this on (setupwizard.go's stepConfirm is a full-screen step, not a
// single-pane keypress action), so this is a minimal invented two-press
// arm/confirm: pressing disablePaneKey again writes disabled_panes; any
// other key cancels (see Update()'s disableConfirmPending reset).
func disableConfirmBody() string {
	return fmt.Sprintf(
		"Disable the Notifications pane?\n\n"+
			"This writes disabled_panes: notifications to your config file. It "+
			"takes effect the next time azdo-tui starts — this session keeps the "+
			"tab as-is.\n\nPress %s again to confirm, or any other key to cancel.",
		disablePaneKey,
	)
}

// handleDisableKey implements the arm/confirm state machine driven by
// disablePaneKey. Only reachable while the pane is in an error state (the
// caller in Update() only routes here from that branch, but this guard
// keeps the method safe to call directly from a test without depending on
// that routing). The first press arms disableConfirmPending; the second
// press (this method being called again while already armed) commits the
// write via disablePane.
func (m Model) handleDisableKey() Model {
	if m.list.Err() == nil {
		return m
	}
	if !m.disableConfirmPending {
		m.disableConfirmPending = true
		return m
	}
	return m.disablePane()
}

// disablePane commits task 19 Part B's confirmed disable action: it
// idempotently appends "notifications" to m.cfg.DisabledPanes and calls
// Config.Save() — the same *config.Config pointer app.go holds (see the cfg
// field's doc comment), never a second write path, so task 18's round-trip
// preservation guarantees apply here too. Existing entries (e.g. a prior
// "metrics" disable) are preserved, not replaced.
//
// A nil m.cfg (never true in production, since app.go always passes its own
// config pointer, but possible in a hand-built test Model) and a Save()
// failure both surface visibly through m.statusMessage rather than looking
// like they worked — Part C's routing of statusMessage to the status bar via
// app.go's syncNotificationsActionMessage applies here identically.
func (m Model) disablePane() Model {
	m.disableConfirmPending = false

	if m.cfg == nil {
		m.statusMessage = "Cannot disable notifications: no config available."
		return m
	}
	if containsString(m.cfg.DisabledPanes, "notifications") {
		m.statusMessage = "Notifications pane is already disabled. Restart azdo-tui for it to take effect."
		return m
	}

	original := m.cfg.DisabledPanes
	next := make([]string, len(original), len(original)+1)
	copy(next, original)
	m.cfg.DisabledPanes = append(next, "notifications")

	if err := m.cfg.Save(); err != nil {
		m.cfg.DisabledPanes = original
		m.statusMessage = fmt.Sprintf("Failed to disable notifications pane: %v", err)
		return m
	}

	m.statusMessage = "Notifications pane disabled. Restart azdo-tui for this to take effect."
	return m
}

// containsString reports whether s is present in list. Small local helper —
// not worth pulling in a dependency for one linear scan over a handful of
// pane names.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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
// statusMessage (the outcome of the pane's own `o` handling) takes
// precedence over the underlying listview's, which always reports "" in
// list mode — phase 1's only view mode for this pane (see the statusMessage
// field's doc comment).
func (m Model) GetStatusMessage() string {
	if m.statusMessage != "" {
		return m.statusMessage
	}
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
// This guard is live, not merely theoretical: task 15 wires both this pane's
// own Init()/`r` fetch and the app-level poller's push through
// HandleFetchResult, either of which can now land a failure while a `u`/`d`
// override is settling. A poll failing mid-debounce, or a real 304 surfacing
// as an error (an unsolicited 304 — no matching cache — per
// internal/github/notifications.go), reaches here exactly as the reachability
// note above describes.
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
	return m.applyOverrides(m.reasonFiltered())
}

// applyOverrides folds Model.overrides on top of base: an overrideHidden
// entry drops the row, an overrideRead entry forces Read to true, and an
// expired entry (past its debounce window) is skipped entirely so the
// underlying data wins again. Factored out of visibleItems so UnreadCount
// (task 16, decision 68) can apply the exact same override semantics over
// m.feed directly, without going through reasonFiltered's `f`-cycle
// narrowing — reusing this logic rather than re-deriving it is what keeps
// the two call sites from silently drifting apart.
//
// Always a freshly allocated slice when any override is active, mirroring
// reasonFiltered: the pane's held feed must never be aliased or mutated by a
// later step.
func (m Model) applyOverrides(base []provider.Notification) []provider.Notification {
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

// UnreadCount returns the number of unread rows in m.feed — the
// config-filtered inbox (decision 21: filters from decision 19's
// exclude_reasons/exclude_repos/unread_only/only_configured_repos are
// already applied before the feed reaches the pane) — with any active `u`/`d`
// override folded on top via applyOverrides, so a row the user has just
// cleared with `u` (or removed with `d`) is reflected immediately, before the
// next poll lands.
//
// Deliberately over m.feed, never reasonFiltered(): the `f` cycle is an
// interactive, local view narrowing, not a config filter, and decision 68
// requires the badge to ignore it — otherwise the count would drop to a
// per-reason subtotal the moment the user pressed `f`, and since the badge
// is meant to be visible from every tab, that wrong number would follow the
// user to a tab where the `f` filter producing it is invisible.
func (m Model) UnreadCount() int {
	count := 0
	for _, n := range m.applyOverrides(m.feed) {
		if !n.Read {
			count++
		}
	}
	return count
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

// toColumns returns the notifications list's column specs:
// [•] [Repo] [Reason] [Title] [Updated].
//
// The items parameter is unused — the layout is static (see
// notificationColumns) — but is kept because listview's ToColumns hook is
// defined in terms of the item slice, and every sibling pane's
// implementation reads it.
func toColumns(_ []provider.Notification) []listview.ColumnSpec {
	cols := make([]listview.ColumnSpec, len(notificationColumns))
	copy(cols, notificationColumns)

	listview.NormalizeWidths(cols)
	return cols
}

// toRows converts notifications to table rows, mirroring toColumns's layout
// exactly: [•] [Repo] [Reason] [Title] [Updated]. Neither has a gating
// predicate, so the two cannot diverge (convention 7).
//
//   - The read cell carries unreadGlyph for an unread row and a blank cell
//     for a read one, styled through readStyle.
//   - The Repo cell falls back to "—" for an empty ScopeDisplay: decision 35
//     defaults it to Scope at the adapter boundary, but a thread whose
//     repository payload is absent leaves both empty, the one case the
//     mapper cannot fix.
//   - The Title cell is rendered through titleStyle's named style —
//     styles.Styles.Title (bold) for unread rows, an empty style for read
//     ones — so unread emphasis is a named style rather than an inline
//     lipgloss.NewStyle() (convention 6). An empty Title dashes to "—"
//     before styling (decision 58).
//   - The Updated cell renders "—" for a zero UpdatedAt (the mapper leaves it
//     zero when the wire omits updated_at), never a year-0001 date.
func toRows(items []provider.Notification, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, n := range items {
		reasonCell := display.NotificationReasonStyle(n.Reason, s).
			Render(display.NotificationReasonGlyph(n.Reason) + " " + display.NotificationReasonLabel(n.Reason))

		row := make(table.Row, cellCount)
		row[cellRead] = readCell(n, s)
		row[cellRepo] = dashIfEmpty(n.Identity.ScopeDisplay)
		row[cellReason] = reasonCell
		row[cellTitle] = titleCell(n, s)
		row[cellUpdated] = formatUpdatedAt(n.UpdatedAt)
		rows[i] = row
	}
	return rows
}

// readStyle returns the style the read-marker cell is rendered with. It
// mirrors titleStyle deliberately: the unread glyph and the unread title are
// one emphasis expressed in two cells, so they resolve to the same named
// style (styles.Styles.Title) and a theme change moves both together.
//
// The read branch returns an empty style for the same reason titleStyle's
// does — see its comment for why a named foreground style would be wrong on
// the selected row.
func readStyle(n provider.Notification, s *styles.Styles) lipgloss.Style {
	if !n.Read {
		return s.Title
	}
	return lipgloss.NewStyle()
}

// readCell returns the unread-marker cell text: unreadGlyph for an unread
// row, empty for a read one.
//
// This exists because unread state was previously conveyed *only* by the
// Title cell's boldness, which is close to invisible in themes with a low
// contrast between bold and regular weight — making `u` (mark read) look
// like it had done nothing at all, since the row correctly stays put when
// unread_only is false. A glyph appearing and disappearing is legible in
// every theme and at every terminal font weight.
func readCell(n provider.Notification, s *styles.Styles) string {
	if n.Read {
		return ""
	}
	return readStyle(n, s).Render(unreadGlyph)
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
