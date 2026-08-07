package provider

import "time"

// ListOpts carries neutral filter intent for list methods. The adapter is
// responsible for translating these fields into backend-specific query
// parameters (e.g. WIQL clauses, REST query params).
//
// Zero value is always valid: all fields at their zero value mean "no extra
// filtering" and reproduce the current default behavior exactly.
type ListOpts struct {
	// Mine, when true, restricts results to items belonging to the
	// authenticated user. For work items this translates to an
	// [System.AssignedTo] = @Me WIQL clause; for pull requests it maps to
	// the creatorId / reviewerId REST search criteria.
	Mine bool

	// States restricts work-item results to the given neutral state
	// categories. An empty slice means no state filter (all states).
	// The adapter maps each StateCategory to one or more backend state
	// strings and emits an IN clause.
	States []StateCategory

	// Statuses restricts pipeline-run results to the given neutral status
	// values. An empty slice means no status filter (all statuses).
	// The adapter maps each RunStatus to the appropriate status/result
	// REST parameter or post-filter.
	Statuses []RunStatus

	// Search restricts results whose title contains the given substring
	// (case-insensitive). An empty string means no title filter.
	Search string

	// Top overrides the default result-count limit when non-zero. A zero
	// value means use the caller-supplied top argument (backwards compatible).
	Top int
}

// NotifOpts carries neutral fetch intent for NotificationSource.List — the
// things the fetch itself needs (server-side query shaping and pagination
// hints). Config-driven filtering over the already-fetched merged feed
// (repo globs, exclude_reasons, unread_only) is a separate pure function and
// is deliberately not part of this struct.
//
// Zero value is always valid: all fields at zero mean "fetch the whole inbox,
// no bound".
// There is deliberately no UnreadOnly field. Fetching unread-only is
// forbidden — GitHub's default response would make a row vanish the moment it
// is marked read — so unread_only is a client-side filter over the full feed
// and has exactly one landing site. A fetch-time hint that its only
// implementer is required to ignore is a trap, not an option.
type NotifOpts struct {
	// ParticipatingOnly restricts results to notifications where the
	// authenticated user is directly participating (assigned, mentioned,
	// author, review-requested, etc.) rather than merely subscribed. Maps to
	// GitHub's participating=true query parameter — a coarser, cheaper
	// server-side bundle that composes with, and does not replace, the
	// client-side exclude_reasons filter.
	ParticipatingOnly bool

	// Since restricts results to notifications updated at or after this
	// time. Zero value means no lower bound. Maps to GitHub's since query
	// parameter.
	Since time.Time

	// Max caps the number of notifications returned across all backends.
	// Zero or negative means no cap. This struct is forwarded verbatim to
	// every capable backend's List, but Max is honoured exclusively by
	// CompositeProvider.List, applied once after it merges every backend's
	// rows and sorts them newest-first (decision 15 / task 12 of the phase-2
	// notifications spec) — never by an individual backend. A backend that
	// truncated on Max itself (phase 1's original per-adapter behaviour)
	// would double-apply the cap the moment a second capable backend exists,
	// each backend keeping only its own top-Max in its own order before the
	// composite ever sees the full picture. Backend implementations must
	// therefore ignore this field.
	Max int
}
