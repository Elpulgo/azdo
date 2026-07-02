package provider

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
