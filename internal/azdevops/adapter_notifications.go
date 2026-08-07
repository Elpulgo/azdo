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
// SourceAssigned's and SourceCIFailed's query windows; a non-positive value
// falls back to DefaultNotificationLookbackDays rather than reaching Azure
// with an unbounded or negative window. toggles controls which of the four
// sources List fans out to.
//
// This is a separate constructor from NewAdapter, not a new parameter on it,
// mirroring github.NewAdapterWithNotifications's precedent
// (internal/github/adapter.go): every existing NewAdapter call site
// constructs an Adapter with no notifications intent at all, and giving it a
// non-nil store implicitly would make "notifications capability" an
// accidental side effect of a signature change rather than an opt-in. A nil
// store is still safe to pass here — List, MarkRead and MarkDone all guard
// it the same way every other Adapter method guards a nil mc.
func NewAdapterWithNotifications(mc *MultiClient, store *TriageStore, lookbackDays int, toggles NotificationSourceToggles) *Adapter {
	if lookbackDays <= 0 {
		lookbackDays = DefaultNotificationLookbackDays
	}
	return &Adapter{
		mc:                mc,
		notifStore:        store,
		notifLookbackDays: lookbackDays,
		notifSources:      toggles,
	}
}

// --------------------------------------------------------------------------
// provider.NotificationSource
// --------------------------------------------------------------------------

// List implements provider.NotificationSource. It is a thin wrapper around
// list: time.Now() is read exactly once, here, and threaded through as a
// parameter to every source, to Reconcile and to MarkRead/MarkDone's callers
// indirectly via the state list writes back. Nothing below this method reads
// the wall clock directly, which is what keeps list fully testable against
// an injected now.
//
// opts.ParticipatingOnly and opts.Since are accepted for interface
// compliance but not used: all four sources already narrow server-side by
// their own means (reviewerId, WIQL, requestedFor) rather than a generic
// "participating" or "since" hint Azure's REST surface has no equivalent
// for. opts.Max truncates the returned slice after Reconcile and sorting,
// mirroring github.Adapter.List's own per-adapter truncation (task 12 will
// later move this to the composite for both adapters at once; doing it here
// today, the same way github already does, keeps that a one-place removal
// rather than a bespoke azdevops carve-out).
func (a *Adapter) List(opts provider.NotifOpts) ([]provider.Notification, error) {
	return a.list(opts, time.Now())
}

// list is List's internal, fully-testable implementation.
//
// Absorb-or-propagate contract (task 8 of the phase-2 notifications spec,
// resolved 2026-08-07): the four sources run concurrently via
// runSourcesConcurrently; if at least one row survives across however many
// sources failed outright, this method returns (rows, nil) rather than
// discarding those rows — exactly the rule internal/provider/composite.go
// already enforces one layer up for backends (decision 20), restated here
// because a source is to this adapter what a backend is to the composite,
// and phase 1 lost a defect to exactly this gap. Only when every source
// fails — or when every enabled source legitimately returns nothing — does
// this method return either an error or an honestly empty slice.
//
// Local triage state (read/done) is folded into the returned rows' Read/Done
// fields here, at this boundary, via Reconcile — nothing above this method
// may learn that Azure's read state is tracked locally rather than on the
// server (phase 1's unread-semantics constraint).
func (a *Adapter) list(opts provider.NotifOpts, now time.Time) ([]provider.Notification, error) {
	if a.mc == nil {
		return nil, fmt.Errorf("azdevops: notifications: no client configured")
	}

	rows, errs := a.runSourcesConcurrently(now)

	stored := TriageState{}
	if a.notifStore != nil {
		stored = a.notifStore.State()
	}
	reconciled, newState := Reconcile(rows, stored, now)

	sortNotificationsDeterministically(reconciled)

	if a.notifStore != nil {
		a.notifStore.Replace(newState)
	}

	if opts.Max > 0 && len(reconciled) > opts.Max {
		// Full slice expression caps capacity as well as length, so the
		// truncated rows cannot be recovered or overwritten by a later
		// append into this slice — matching github.Adapter.List's own
		// truncation.
		reconciled = reconciled[:opts.Max:opts.Max]
	}

	if len(errs) == 0 {
		return reconciled, nil
	}
	if len(reconciled) > 0 {
		// Absorb: at least one row survived the sources that did succeed.
		// Degrading to a shorter feed beats emptying it — see this method's
		// doc comment.
		return reconciled, nil
	}
	return nil, errors.Join(errs...)
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
// ordering guarantee on top of this; this function only guarantees the
// fan-out itself is race-free and index-deterministic.
//
// now is threaded straight through to every source; this method never calls
// time.Now() itself.
func (a *Adapter) runSourcesConcurrently(now time.Time) ([]provider.Notification, []error) {
	var jobs []func() ([]provider.Notification, error)

	if a.notifSources.ReviewRequested {
		jobs = append(jobs, func() ([]provider.Notification, error) {
			return SourceReviewRequested(a.mc, reviewRequestedQueryTop, now)
		})
	}
	if a.notifSources.Mentioned {
		jobs = append(jobs, func() ([]provider.Notification, error) {
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
			res, err := SourceMentioned(a.mc, now)
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
			return SourceCIFailed(a.mc, a.notifLookbackDays, ciFailedQueryTop, now)
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
	return rows, errs
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
// Provisional (task 8): for an id Reconcile has never seen (no existing
// entry), this writes LastActivity as time.Now() — only an approximation of
// the row's own UpdatedAt, since MarkRead's fixed provider.Identity-only
// signature carries no such timestamp. Reconcile's own doc comment records
// the invariant this must eventually satisfy in full (an entry written
// outside Reconcile must carry a LastActivity at least as new as the row it
// was marked from, or the very next poll's "strictly newer" branch clears
// the mark within one interval) as "task 9's problem" — hardening this,
// which may require widening this method's signature, is explicitly out of
// scope for task 8.
func (a *Adapter) MarkRead(id provider.Identity) error {
	if a.notifStore == nil {
		return fmt.Errorf("azdevops: mark read: no notifications store configured")
	}
	if id.Kind != provider.KindAzure {
		return fmt.Errorf("azdevops: mark read: identity kind %q is not %q", id.Kind, provider.KindAzure)
	}
	if id.ID == "" {
		return fmt.Errorf("azdevops: mark read: empty identity id")
	}
	now := time.Now()
	a.notifStore.Apply(func(state TriageState) {
		entry := state[id.ID]
		entry.Read = true
		if entry.LastActivity.IsZero() {
			entry.LastActivity = now
		}
		if entry.LastSeen.IsZero() {
			entry.LastSeen = now
		}
		state[id.ID] = entry
	})
	return nil
}

// MarkDone marks id as done in local triage state. See MarkRead's doc
// comment: the nil-store guard, the Kind check and the provisional
// LastActivity approximation are all shared and not repeated here. A done
// entry is dropped from the very next List call by Reconcile
// (notifications_reconcile.go) — there is no server-side delete to issue for
// an Azure-sourced row.
func (a *Adapter) MarkDone(id provider.Identity) error {
	if a.notifStore == nil {
		return fmt.Errorf("azdevops: mark done: no notifications store configured")
	}
	if id.Kind != provider.KindAzure {
		return fmt.Errorf("azdevops: mark done: identity kind %q is not %q", id.Kind, provider.KindAzure)
	}
	if id.ID == "" {
		return fmt.Errorf("azdevops: mark done: empty identity id")
	}
	now := time.Now()
	a.notifStore.Apply(func(state TriageState) {
		entry := state[id.ID]
		entry.Done = true
		if entry.LastActivity.IsZero() {
			entry.LastActivity = now
		}
		if entry.LastSeen.IsZero() {
			entry.LastSeen = now
		}
		state[id.ID] = entry
	})
	return nil
}
