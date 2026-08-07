package azdevops

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// DefaultNotificationLookbackDays is the window SourceAssigned and
// SourceCIFailed query within when NewAdapterWithNotifications is given a
// non-positive lookbackDays. Task 11 wires notifications.azure.lookback_days
// onto this constructor; this is the fallback a caller not yet wiring config
// (or an explicit "reset to default") gets by construction.
const DefaultNotificationLookbackDays = 14

// reviewRequestedQueryTop and ciFailedQueryTop bound SourceReviewRequested's
// and SourceCIFailed's own per-project query size, matching the $top
// convention assignedQueryTop and mentionCandidateQueryTop already use for
// their sources. The approved phase-2 config YAML carries no per-source
// "top" knob (only lookback_days, min_poll_interval and the four
// sources.* toggles), so these stay private constants here, not config.
const (
	reviewRequestedQueryTop = 50
	ciFailedQueryTop        = 50
)

// NotificationSourceToggles controls which of the four phase-2 notification
// sources List fans out to. All four default to true (see
// DefaultNotificationSourceToggles). This struct is deliberately the whole
// additive surface task 11 targets: wiring config's sources.review_requested
// / sources.mentioned / sources.assigned / sources.ci_failed booleans means
// building one of these and passing it to NewAdapterWithNotifications —
// neither this type's shape nor the constructor's signature needs to change
// for that.
type NotificationSourceToggles struct {
	ReviewRequested bool
	Mentioned       bool
	Assigned        bool
	CIFailed        bool
}

// DefaultNotificationSourceToggles returns every source enabled — the
// behaviour any caller not yet wiring task 11's config gets by construction.
func DefaultNotificationSourceToggles() NotificationSourceToggles {
	return NotificationSourceToggles{
		ReviewRequested: true,
		Mentioned:       true,
		Assigned:        true,
		CIFailed:        true,
	}
}

// NewAdapterWithNotifications creates an Adapter that additionally satisfies
// provider.NotificationSource, wrapping mc for the existing provider.Provider
// surface and store for local triage-state persistence
// (notifications_store.go, notifications_reconcile.go). lookbackDays bounds
// SourceAssigned's and SourceCIFailed's query windows (the other two
// sources, SourceReviewRequested and SourceMentioned, do not take a
// lookback at all); a non-positive value falls back to
// DefaultNotificationLookbackDays rather than reaching Azure with an
// unbounded or negative window, and a value above
// MaxNotificationLookbackDays is clamped down to it — orphanTTL's own doc
// comment explains why exceeding that bound reopens the resurfaces-as-unread
// gap the clamp exists to close. This clamp runs here regardless of caller:
// internal/config.LoadFrom applies the same bound before this constructor is
// ever reached, but that is a second, earlier clamp on top of this one, not
// a substitute for it — any other caller passing e.g. 90 directly is caught
// here too. toggles controls which of the four sources List fans out to.
//
// This is a separate constructor from NewAdapter, not a new parameter on it,
// mirroring github.NewAdapterWithNotifications's precedent
// (internal/github/adapter.go): every existing NewAdapter call site
// constructs an Adapter with no notifications intent at all, and giving it a
// non-nil store implicitly would make "notifications capability" an
// accidental side effect of a signature change rather than an opt-in. A nil
// store is still safe to pass here — List, MarkRead and MarkDone all guard
// it the same way every other Adapter method guards a nil mc.
// minPollInterval is task 14's wiring of notifications.azure.min_poll_interval
// (decision 10): List self-throttles to it, returning its previous result
// unchanged rather than issuing a real query, whenever called sooner than
// minPollInterval after its last real query. A non-positive value disables
// throttling outright — every List call runs a real query — which is what
// every call site in this package's own tests wants unless it is
// specifically exercising the throttle.
//
// A real *Config reaching cmd/azdo-tui's runTUI comes from exactly two
// places: internal/config.LoadFrom (an existing config file) or the setup
// wizard's internal/ui/setupwizard.Model.GetConfig, which builds its
// *Config via internal/config.NewWithPath. Both now populate
// Notifications.Azure.MinPollInterval to DefaultAzureMinPollInterval (300)
// whenever the value is not otherwise set: LoadFrom via its own `== 0`
// fallback (task-11 review decision B), NewWithPath via
// internal/config.defaultNotificationsConfig, which registers the exact
// same default through the same viper mechanism LoadFrom uses (see
// notificationsDefaults in internal/config/config.go). So neither
// production path reaches this constructor with a literal zero. That was
// not true of NewWithPath before task 14's review (🔴 finding 2): it left
// Notifications at its Go zero value entirely, which is what let a
// setup-wizard-produced config's first TUI session run with every Azure
// source toggle off and this constructor called with a literal zero here —
// disabling throttling for that entire session, not just leaving it at a
// wrong-but-still-positive default.
func NewAdapterWithNotifications(mc *MultiClient, store *TriageStore, lookbackDays int, toggles NotificationSourceToggles, minPollInterval time.Duration) *Adapter {
	if lookbackDays <= 0 {
		lookbackDays = DefaultNotificationLookbackDays
	}
	if lookbackDays > MaxNotificationLookbackDays {
		lookbackDays = MaxNotificationLookbackDays
	}
	return &Adapter{
		mc:                   mc,
		notifStore:           store,
		notifLookbackDays:    lookbackDays,
		notifSources:         toggles,
		notifMinPollInterval: minPollInterval,
	}
}

// --------------------------------------------------------------------------
// provider.NotificationSource
// --------------------------------------------------------------------------

// List implements provider.NotificationSource. It is a thin wrapper around
// list: time.Now() is read exactly once, here, and threaded through as a
// parameter to every source and to Reconcile. Nothing below this method
// reads the wall clock directly, which is what keeps list fully testable
// against an injected now. MarkRead and MarkDone are a separate boundary,
// not fed by this now: each reads time.Now() directly (see MarkRead's own
// doc comment for why that is safe), since neither is ever called from
// List's call path.
//
// opts.ParticipatingOnly, opts.Since and opts.Max are all accepted for
// interface compliance but not used here: all four sources already narrow
// server-side by their own means (reviewerId, WIQL, requestedFor) rather than
// a generic "participating" or "since" hint Azure's REST surface has no
// equivalent for, and opts.Max — since decision 15 / task 12 — is honoured
// exclusively by provider.CompositeProvider.List, after it merges every
// capable backend's rows and sorts them newest-first. Truncating here too
// (phase 1's original per-adapter behaviour, mirroring github.Adapter.List's
// own prior truncation) would double-apply the cap once a second backend
// exists: each adapter would independently keep its own top-Max, in this
// adapter's own sort order, before the composite ever sees the full picture —
// which can silently drop a row that belongs in the true global top-Max
// while still returning a plausible, correctly-sized slice. Since decision C
// (review of task 12), provider.CompositeProvider.List no longer forwards
// opts verbatim either: it zeroes Max before calling this method, so opts.Max
// arrives here as 0 on every call that reaches this adapter through the
// composite. This method's own indifference to the field does not depend on
// that — it ignores whatever value opts.Max carries — but it means the
// double-apply scenario above is now prevented at the call site too, not
// only by this adapter declining to act on it.
func (a *Adapter) List(opts provider.NotifOpts) ([]provider.Notification, error) {
	return a.list(opts, time.Now())
}

// list is List's internal, fully-testable implementation.
//
// Absorb-or-propagate contract (task 8 of the phase-2 notifications spec;
// corrected 2026-08-07, same day, from this method's own first review): the
// four sources run concurrently via runSourcesConcurrently, which reports
// how many of them ran (jobCount) alongside their rows and errors. This
// method returns (rows, nil) whenever at least one source *succeeded* —
// jobCount == 0 (every source disabled) trivially counts as every enabled
// source succeeding — and propagates an error only when every source that
// ran failed outright (len(errs) == jobCount, jobCount > 0). This is
// deliberately keyed on how many sources answered, not on how many rows came
// back: a poll where every source succeeds but the user has already triaged
// every subject away legitimately returns zero rows, and that is a feed, not
// an outage — collapsing that case to an error would be wrong exactly the
// way returning an error whenever len(rows) happens to be zero would be.
// This mirrors the rule internal/provider/composite.go already enforces one
// layer up for backends (decision 20): a source is to this adapter what a
// backend is to the composite.
//
// Local triage state (read/done) is folded into the returned rows' Read/Done
// fields here, at this boundary, via Reconcile — nothing above this method
// may learn that Azure's read state is tracked locally rather than on the
// server (phase 1's unread-semantics constraint). The read (State), compute
// (Reconcile) and write (the new state) are done inside one TriageStore.Swap
// call, not as three separate lock acquisitions, so a concurrent MarkRead or
// MarkDone landing mid-poll is never silently lost underneath this method's
// own write (task 8 review, 🟡 finding 5).
//
// Self-throttle (decision 10, task 14): before any of the above runs, this
// method checks notifThrottled(now) and, if the call falls inside the
// window opened by the last real query, skips runSourcesConcurrently and
// notifStore.Swap entirely — no network work, no store write, for that
// call. It does not, however, skip Reconcile: it re-runs Reconcile in memory
// against notifLastRawRows (the raw, pre-Reconcile rows the last real query
// produced) and a fresh read of the store's current triage state
// (notifStore.State()), discarding the state Reconcile returns rather than
// writing it back. This is what makes a mark (MarkRead/MarkDone) made after
// the last real query visible on the very next throttled call, instead of
// being masked until the throttle window closes and a real query finally
// runs (task 14 review, 🔴 finding 1) — reusing the cached, pre-Reconcile
// rows verbatim means this still costs no network work, and discarding the
// recomputed state means it still writes nothing to the store: both halves
// of the "throttled call" contract hold. Only a real query that lands in
// this method's absorb branch (at least one source succeeded) advances
// notifLastPollAt/notifLastRawRows/notifLastResult and so opens the next
// window; a real query that fails outright (the error-propagating branch
// below) leaves them exactly as they were, so a total outage is retried on
// the very next call rather than being remembered as a throttle anchor —
// see notifThrottled's own doc comment for the first-call case and
// copyNotifications' for why the cached slice itself cannot be corrupted by
// a caller mutating what they were handed.
func (a *Adapter) list(opts provider.NotifOpts, now time.Time) ([]provider.Notification, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("azdevops: notifications: no client configured")
	}
	if a.notifStore == nil {
		// Mirrors github.Adapter.List's nil-nc guard (internal/github/adapter.go),
		// which returns "github: notifications: no notifications client
		// configured" — worded here without the word "store" so that, per
		// MarkRead/MarkDone's own guard below, nothing above this boundary
		// can infer from an error string that Azure's read state happens to
		// be tracked locally rather than on the server.
		return nil, fmt.Errorf("azdevops: notifications: not configured")
	}

	// notifThrottleMu is held for the rest of this call, including the
	// network work below and the notifStore.Swap that follows it — see the
	// lock-order comment on the Adapter struct for why that is safe and
	// does not block MarkRead/MarkDone.
	a.notifThrottleMu.Lock()
	defer a.notifThrottleMu.Unlock()

	if a.notifThrottled(now) {
		// Re-reconcile rather than replay: notifLastRawRows is the raw,
		// pre-Reconcile data from the last real query, and a.notifStore.State()
		// is a fresh read of whatever triage state MarkRead/MarkDone have
		// written since then (State only takes notifStore's own mu, never
		// notifThrottleMu, so this cannot deadlock against them — see the
		// lock-order comment on the Adapter struct). The state Reconcile
		// returns is discarded: a throttled call must write nothing to the
		// store. No network work happens on this path.
		state := a.notifStore.State()
		reconciled, _ := Reconcile(a.notifLastRawRows, state, now)
		sortNotificationsDeterministically(reconciled)
		return copyNotifications(reconciled), nil
	}

	rows, errs, jobCount := a.runSourcesConcurrently(now)

	var reconciled []provider.Notification
	a.notifStore.Swap(func(stored TriageState) TriageState {
		var newState TriageState
		reconciled, newState = Reconcile(rows, stored, now)
		return newState
	})

	sortNotificationsDeterministically(reconciled)

	if jobCount == 0 || len(errs) < jobCount {
		// Absorb: every enabled source succeeded (possibly zero of them, a
		// legal all-toggles-off empty feed), or enough of them did that at
		// least one contributed. See this method's doc comment for why this
		// is keyed on source count, not on len(reconciled). This is also
		// the only branch that advances the throttle: a real query that
		// fails outright (the branch below) leaves notifLastPollAt and
		// notifLastResult untouched, so it never starts a throttle window
		// and never gets cached — see notifThrottled's doc comment.
		a.notifLastPollAt = now
		a.notifLastRawRows = rows
		a.notifLastResult = reconciled
		return copyNotifications(a.notifLastResult), nil
	}
	return nil, fmt.Errorf("azdevops: notifications: all %d sources failed: %w", len(errs), errors.Join(errs...))
}

// notifThrottled reports whether now falls inside the self-throttle window
// opened by the last real query that produced a cacheable result (list's
// absorb branch — see its own comment). Must be called with notifThrottleMu
// held.
//
// A non-positive notifMinPollInterval disables throttling unconditionally,
// checked first and independently of notifLastPollAt/now so that no clock
// relationship between the two — including one where now is earlier than
// notifLastPollAt — can make this return true when throttling is off.
//
// A zero notifLastPollAt (no real query has ever succeeded yet) needs no
// dedicated check: time.Time's zero value is year 1, so
// notifLastPollAt.Add(notifMinPollInterval) still lands far in the past
// relative to any real now, and now.Before(...) is false — the very first
// call is never throttled, without a second explicit guard whose effect the
// first one would already subsume.
func (a *Adapter) notifThrottled(now time.Time) bool {
	if a.notifMinPollInterval <= 0 {
		return false
	}
	return now.Before(a.notifLastPollAt.Add(a.notifMinPollInterval))
}

// copyNotifications returns a fresh slice holding the same elements as rows,
// so a caller that mutates the result cannot corrupt notifLastResult — the
// slice a real query's absorb branch caches and every subsequent call reads
// from until the next real query replaces it — by mutating what list handed
// back on some earlier call. Phase 1 lost a defect to exactly this aliasing
// in its conditional-request cache.
//
// A per-element copy into a new backing array is already a true value copy
// here: every field of provider.Notification (Identity's strings, Title,
// Reason, Read, Done, UpdatedAt, WebURL) is a value type with no field a
// caller can mutate through — even time.Time, whose only field is an
// unexported *time.Location, exposes no way to reach through a
// Notification and mutate the Location a cached row's UpdatedAt points at.
// TestCopyNotifications_NoMutableFieldAliasing guards this claim with
// reflection over provider.Notification's fields, so a future field added
// to Notification without updating this comment still gets checked.
func copyNotifications(rows []provider.Notification) []provider.Notification {
	if rows == nil {
		return nil
	}
	out := make([]provider.Notification, len(rows))
	copy(out, rows)
	return out
}

// sourceResult is one source's raw outcome, kept together so
// runSourcesConcurrently can hand each goroutine's slot a single write.
type sourceResult struct {
	rows []provider.Notification
	err  error
}

// runSourcesConcurrently fans out to every enabled source at once and waits
// for all of them to finish before returning. Each goroutine writes to its
// own fixed index of a slice pre-sized to job count — no shared append, no
// lock beyond sync.WaitGroup.Wait — so which project or source happens to
// respond first can never corrupt another slot, and the concatenation order
// below is determined solely by job index (review, mentioned, assigned,
// ci_failed, skipping any disabled source), never by goroutine completion
// order. sortNotificationsDeterministically imposes the actual returned
// ordering guarantee on top of this. jobCount (== len(jobs)) is returned
// alongside rows and errs so list can key its absorb-or-propagate decision on
// how many sources actually ran, not on how many rows survived — see list's
// doc comment.
//
// now is threaded straight through to every source; this method never calls
// time.Now() itself.
//
// The authenticated user id is resolved exactly once, here, before any
// source's goroutine starts, and threaded into the three sources that need
// it (ReviewRequested, Mentioned, CIFailed — Assigned narrows server-side via
// a WIQL @Me macro instead and needs no id at all). This method is not
// race-free by construction alone; it is race-free because it removes the
// only shared mutable state the three concurrent sources would otherwise
// touch. Before this fix, all three called Client.GetCurrentUserID
// independently, and in the common single-project case that meant three
// goroutines racing the same *Client's unsynchronized userID cache field —
// one goroutine's unsynchronized write landing between another's
// unsynchronized read-check and its own write is a real data race, not a
// theoretical one (task 8 review, 🔴 finding 2). Resolving once here also
// drops two of the three redundant connectionData HTTP round-trips per poll
// this used to cost, and removes the resulting nondeterminism of which
// project's client happened to win the race to populate the cache first. A
// resolution failure here — mc has no reachable client, or the resolved id is
// empty — is reported once, tagged onto each of the three id-dependent jobs
// rather than aborting the whole poll, so a healthy Assigned source (which
// needs no id) still runs and contributes its rows.
//
// The resolve call itself is skipped entirely when none of the three
// id-dependent sources are enabled (e.g. a config running only Assigned, or
// every toggle off) — there is no point spending an HTTP round trip on an id
// nothing in this poll will use.
func (a *Adapter) runSourcesConcurrently(now time.Time) ([]provider.Notification, []error, int) {
	var userID string
	var userIDErr error
	if a.notifSources.ReviewRequested || a.notifSources.Mentioned || a.notifSources.CIFailed {
		userID, userIDErr = resolveAuthenticatedUserID(a.mc)
	}

	var jobs []func() ([]provider.Notification, error)

	if a.notifSources.ReviewRequested {
		jobs = append(jobs, func() ([]provider.Notification, error) {
			if userIDErr != nil {
				return nil, userIDErr
			}
			return SourceReviewRequested(a.mc, userID, reviewRequestedQueryTop, now)
		})
	}
	if a.notifSources.Mentioned {
		jobs = append(jobs, func() ([]provider.Notification, error) {
			if userIDErr != nil {
				return nil, userIDErr
			}
			// SourceMentionedResult also carries CandidatesDropped,
			// CandidateLimit and CommentFetchFailures — structured
			// diagnostics about how much of stage 1's candidate fan-out was
			// truncated and how many stage-2 comment fetches failed. Only
			// .Rows and the error propagate past this point: provider.
			// NotificationSource.List's return shape has no side channel to
			// carry them further, and this repo has no logging facility a
			// mid-poll write could safely reach (a write here would land on
			// top of Bubble Tea's alt-screen render). Dropping them is
			// deliberate, not an oversight — widening NotificationSource
			// with a warnings channel is recorded under the phase-2 spec's
			// "Unknowns" section for Oscar to decide, not invented here.
			res, err := SourceMentioned(a.mc, userID, now)
			return res.Rows, err
		})
	}
	if a.notifSources.Assigned {
		jobs = append(jobs, func() ([]provider.Notification, error) {
			return SourceAssigned(a.mc, a.notifLookbackDays, now)
		})
	}
	if a.notifSources.CIFailed {
		jobs = append(jobs, func() ([]provider.Notification, error) {
			if userIDErr != nil {
				return nil, userIDErr
			}
			return SourceCIFailed(a.mc, userID, a.notifLookbackDays, ciFailedQueryTop, now)
		})
	}

	results := make([]sourceResult, len(jobs))
	var wg sync.WaitGroup
	for i, run := range jobs {
		wg.Add(1)
		go func(idx int, run func() ([]provider.Notification, error)) {
			defer wg.Done()
			rows, err := run()
			results[idx] = sourceResult{rows: rows, err: err}
		}(i, run)
	}
	wg.Wait()

	var rows []provider.Notification
	var errs []error
	for _, r := range results {
		rows = append(rows, r.rows...)
		if r.err != nil {
			errs = append(errs, r.err)
		}
	}
	return rows, errs, len(jobs)
}

// sortNotificationsDeterministically sorts merged rows by UpdatedAt
// descending, then Identity.Scope, then Identity.ID ascending as tie-breaks
// — mirroring internal/provider/composite.go's own mergeNotifications
// tie-break (its leading Kind comparison is omitted here since every row
// this adapter produces always carries provider.KindAzure).
//
// This is this adapter's own ordering guarantee, asserted directly by this
// package's tests, not an assumption that the composite's later re-sort will
// paper over an unsorted return — a documented trap this package has
// already hit once (see this file's sibling sources' own doc comments on
// determinism).
func sortNotificationsDeterministically(rows []provider.Notification) {
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
		}
		if rows[i].Identity.Scope != rows[j].Identity.Scope {
			return rows[i].Identity.Scope < rows[j].Identity.Scope
		}
		return rows[i].Identity.ID < rows[j].Identity.ID
	})
}

// MarkRead marks id as read in local triage state (notifications_store.go).
// Unlike GitHub, Azure DevOps has no server-side notifications inbox to
// call: phase 1's unread-semantics constraint folds all read/done state into
// Notification.Read/Done at the List boundary, so marking read here means
// writing the local TriageStore, not issuing an HTTP call.
//
// id.ID is the key the store writes through (decision 2). An empty id is
// rejected outright rather than written: a "" key would be shared by every
// malformed subject Reconcile drops from the feed before it ever reaches
// local state (see NotifKey's and Reconcile's own doc comments), so
// accepting it here would create a single entry every such row collapses
// into rather than failing visibly.
//
// id the store has never seen creates the entry rather than erroring — the
// composite routes MarkRead/MarkDone by Identity.Kind alone
// (CompositeProvider), so it cannot know ahead of time which ids a given
// backend's store already has, and a create-on-mark is what keeps that
// routing correct without a round trip through List first. The backfill
// below (LastActivity/LastSeen set to now when zero) is not actually scoped
// to "an id the store has never seen": it fires on any entry whose
// LastActivity is still zero, which also covers an entry Reconcile itself
// created earlier from a row that carried no usable activity stamp
// (Reconcile's "zero UpdatedAt" branch leaves LastActivity at zero). Either
// way, the value written here is an approximation of the row's own
// UpdatedAt, since MarkRead's fixed provider.Identity-only signature carries
// no such timestamp.
//
// That approximation holds against the row that triggered the mark: every
// source clamps its own activity stamp to the now it was given (see
// prActivityStamp, mentionActivityStamp, assignedActivityStamp,
// ciFailedActivityStamp), and MarkRead can only ever be called by a UI that
// is reacting to a row rendered from some earlier List(now) call — so
// MarkRead's own time.Now() is always later than the now that bounded that
// particular row's stamp, and the LastActivity written here is >= the
// UpdatedAt of the row the mark was issued from. That is a narrower claim
// than ">= whatever row Reconcile compares against on the next poll": each
// *ActivityStamp helper's forward clamp caps at *that poll's own* now, so a
// subject whose raw stamp runs ahead of the client clock can yield a clamped
// stamp that keeps advancing every poll with nothing having happened to the
// subject — which Reconcile then reads as new activity and clears the very
// mark this just wrote. Whether and how to change that behaviour is an open
// question this code does not handle; see the phase-2 notifications spec's
// `## Unknowns`, "What should happen when an activity stamp is
// forward-clamped?".
//
// Marking an id that is already read is a no-op: it does not touch
// LastActivity or LastSeen, and — via TriageStore.ApplyIfChanged — does not
// mark the store dirty or re-arm its debounce timer. Without this check,
// every repeated MarkRead on an already-read row (e.g. re-selecting it while
// browsing) would churn the debounced write for a value that is not
// changing.
func (a *Adapter) MarkRead(id provider.Identity) error {
	if a.notifStore == nil {
		return fmt.Errorf("azdevops: mark read: notifications not configured")
	}
	if id.Kind != provider.KindAzure {
		return fmt.Errorf("azdevops: mark read: identity kind %q is not %q", id.Kind, provider.KindAzure)
	}
	if id.ID == "" {
		return fmt.Errorf("azdevops: mark read: empty identity id")
	}
	now := time.Now()
	a.notifStore.ApplyIfChanged(func(state TriageState) bool {
		entry := state[id.ID]
		if entry.Read {
			return false
		}
		entry.Read = true
		if entry.LastActivity.IsZero() {
			entry.LastActivity = now
		}
		if entry.LastSeen.IsZero() {
			entry.LastSeen = now
		}
		state[id.ID] = entry
		return true
	})
	return nil
}

// MarkDone marks id as done in local triage state. See MarkRead's doc
// comment: the nil-store guard, the Kind check, the empty-id guard, the
// create-on-mark behaviour for an id the store has never seen, and the
// LastActivity approximation (and why it is safe) are all shared and not
// repeated here. A done entry is dropped from the very next List call by
// Reconcile (notifications_reconcile.go) — there is no server-side delete to
// issue for an Azure-sourced row.
//
// Marking an id that is already done is a no-op, the same way and for the
// same reason MarkRead's is: it does not re-touch LastActivity/LastSeen and
// does not re-dirty the debounced store (TriageStore.ApplyIfChanged).
func (a *Adapter) MarkDone(id provider.Identity) error {
	if a.notifStore == nil {
		return fmt.Errorf("azdevops: mark done: notifications not configured")
	}
	if id.Kind != provider.KindAzure {
		return fmt.Errorf("azdevops: mark done: identity kind %q is not %q", id.Kind, provider.KindAzure)
	}
	if id.ID == "" {
		return fmt.Errorf("azdevops: mark done: empty identity id")
	}
	now := time.Now()
	a.notifStore.ApplyIfChanged(func(state TriageState) bool {
		entry := state[id.ID]
		if entry.Done {
			return false
		}
		entry.Done = true
		if entry.LastActivity.IsZero() {
			entry.LastActivity = now
		}
		if entry.LastSeen.IsZero() {
			entry.LastSeen = now
		}
		state[id.ID] = entry
		return true
	})
	return nil
}
