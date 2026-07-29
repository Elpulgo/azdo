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
// (repo globs, exclude_reasons, unread_only) is a separate pure function
// (task 10) and is deliberately not part of this struct.
//
// Zero value is always valid: all fields at zero mean "fetch the whole
// inbox, no bound".
type NotifOpts struct {
	// UnreadOnly is a generic fetch-time hint for backends that can safely
	// filter unread server-side. GitHub's adapter does not honor it: per
	// spec Decision 12, GitHub always fetches with all=true regardless of
	// this field, because the default (unread-only) response would make a
	// row disappear the moment it's marked read. unread_only is applied
	// client-side instead, over the full fetched feed (task 10).
	UnreadOnly bool

	// ParticipatingOnly restricts results to notifications where the
	// authenticated user is directly participating (assigned, mentioned,
	// author, review-requested, etc.) rather than merely subscribed. Maps to
	// GitHub's participating=true query parameter (Decision 10) — a coarser,
	// cheaper server-side bundle that composes with, and does not replace,
	// the client-side exclude_reasons filter.
	ParticipatingOnly bool

	// Since restricts results to notifications updated at or after this
	// time. Zero value means no lower bound. Maps to GitHub's since query
	// parameter.
	Since time.Time

	// Max caps the number of notifications returned across all fetched
	// pages. Zero means no cap.
	Max int
}
