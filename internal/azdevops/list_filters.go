package azdevops

import (
	"fmt"
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
)

// BuildWIQLFilters translates a provider.ListOpts into a string of WIQL WHERE
// clause fragments (each starting with AND) that can be appended verbatim to a
// WIQL query. The output is always safe to append after an existing WHERE clause.
//
// Zero-value opts produces an empty string, reproducing the current default
// query behaviour exactly.
func BuildWIQLFilters(opts provider.ListOpts) string {
	var parts []string

	if opts.Mine {
		parts = append(parts, "  AND [System.AssignedTo] = @Me")
	}

	if len(opts.States) > 0 {
		var stateLiterals []string
		for _, cat := range opts.States {
			for _, s := range StateCategoryToWIQLStates(cat) {
				stateLiterals = append(stateLiterals, fmt.Sprintf("'%s'", s))
			}
		}
		// ReadyForTest uses a CONTAINS pattern instead of exact match.
		hasReady := false
		for _, cat := range opts.States {
			if cat == provider.StateCategoryReadyForTest {
				hasReady = true
				break
			}
		}

		// Build the filter: if only ReadyForTest requested, use CONTAINS;
		// if mixed, use an IN clause for exact states plus OR CONTAINS for ready.
		var exactLiterals []string
		for _, cat := range opts.States {
			if cat == provider.StateCategoryReadyForTest {
				continue
			}
			for _, s := range StateCategoryToWIQLStates(cat) {
				exactLiterals = append(exactLiterals, fmt.Sprintf("'%s'", s))
			}
		}

		switch {
		case hasReady && len(exactLiterals) == 0:
			// Only ReadyForTest — use CONTAINS on state name
			parts = append(parts, "  AND [System.State] CONTAINS 'ready'")
		case hasReady && len(exactLiterals) > 0:
			// Mixed: IN clause OR CONTAINS
			parts = append(parts, fmt.Sprintf(
				"  AND ([System.State] IN (%s) OR [System.State] CONTAINS 'ready')",
				strings.Join(exactLiterals, ", "),
			))
		default:
			// No ReadyForTest: plain IN clause
			_ = stateLiterals // already built above; use exactLiterals here
			parts = append(parts, fmt.Sprintf(
				"  AND [System.State] IN (%s)",
				strings.Join(exactLiterals, ", "),
			))
		}
	}

	if opts.Search != "" {
		parts = append(parts, fmt.Sprintf("  AND [System.Title] CONTAINS '%s'", opts.Search))
	}

	return strings.Join(parts, "\n")
}

// StateCategoryToWIQLStates maps a neutral StateCategory to the set of Azure
// DevOps wire state strings used in WIQL IN clauses.
//
// ReadyForTest is the exception: because "ready" states are custom strings
// that vary across projects (e.g. "Ready for Test", "Ready for Review"),
// the returned slice contains the lowercase token "ready" as a signal to
// BuildWIQLFilters to use a CONTAINS clause instead of an exact IN clause.
func StateCategoryToWIQLStates(cat provider.StateCategory) []string {
	switch cat {
	case provider.StateCategoryNew:
		return []string{"New"}
	case provider.StateCategoryActive:
		return []string{"Active"}
	case provider.StateCategoryResolved:
		return []string{"Resolved"}
	case provider.StateCategoryReadyForTest:
		// Signal to caller to use CONTAINS 'ready' — not an exact state name.
		return []string{"ready"}
	case provider.StateCategoryClosedDone:
		return []string{"Closed"}
	case provider.StateCategoryRemoved:
		return []string{"Removed"}
	default:
		return nil
	}
}
