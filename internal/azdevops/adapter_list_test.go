package azdevops_test

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
)

// TestBuildWIQLFilters verifies that BuildWIQLFilters translates ListOpts
// intent into WIQL WHERE clause fragments correctly. Each test checks that
// all expected clause snippets are present in the output.
func TestBuildWIQLFilters(t *testing.T) {
	tests := []struct {
		name            string
		opts            provider.ListOpts
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "zero value — no extra filters",
			opts:            provider.ListOpts{},
			wantContains:    []string{},
			wantNotContains: []string{"@Me", "System.Title", "System.State"},
		},
		{
			name:         "Mine=true adds @Me clause",
			opts:         provider.ListOpts{Mine: true},
			wantContains: []string{"[System.AssignedTo] = @Me"},
		},
		{
			name: "States maps StateCategory to WIQL state strings",
			opts: provider.ListOpts{
				States: []provider.StateCategory{
					provider.StateCategoryNew,
					provider.StateCategoryActive,
				},
			},
			wantContains: []string{
				"[System.State]",
				"'New'",
				"'Active'",
			},
		},
		{
			name: "States with ClosedDone includes Closed",
			opts: provider.ListOpts{
				States: []provider.StateCategory{provider.StateCategoryClosedDone},
			},
			wantContains: []string{"'Closed'"},
		},
		{
			name: "States with Resolved includes Resolved",
			opts: provider.ListOpts{
				States: []provider.StateCategory{provider.StateCategoryResolved},
			},
			wantContains: []string{"'Resolved'"},
		},
		{
			name: "States with ReadyForTest includes ready-like pattern",
			opts: provider.ListOpts{
				States: []provider.StateCategory{provider.StateCategoryReadyForTest},
			},
			wantContains: []string{"ready"},
		},
		{
			name: "States with Removed includes Removed",
			opts: provider.ListOpts{
				States: []provider.StateCategory{provider.StateCategoryRemoved},
			},
			wantContains: []string{"'Removed'"},
		},
		{
			name: "Search adds title contains clause",
			opts: provider.ListOpts{Search: "login bug"},
			wantContains: []string{
				"[System.Title]",
				"login bug",
			},
		},
		{
			name: "Top is not a WHERE filter — not present in output",
			opts: provider.ListOpts{Top: 10},
			// Top controls $top query param, not a WIQL WHERE clause
			wantContains:    []string{},
			wantNotContains: []string{"[System.Title]", "@Me", "[System.State]"},
		},
		{
			name: "Mine+States+Search combined",
			opts: provider.ListOpts{
				Mine:   true,
				States: []provider.StateCategory{provider.StateCategoryNew, provider.StateCategoryActive},
				Search: "auth",
			},
			wantContains: []string{
				"[System.AssignedTo] = @Me",
				"[System.State]",
				"'New'",
				"'Active'",
				"[System.Title]",
				"auth",
			},
		},
		{
			name: "Search with single-quote is escaped to prevent WIQL injection",
			opts: provider.ListOpts{Search: "O'Brien's bug"},
			wantContains: []string{
				"[System.Title]",
				// Single quotes must be doubled: O'Brien's → O''Brien''s
				"O''Brien''s",
			},
			wantNotContains: []string{
				// The raw unescaped form must not appear in the output
				"O'Brien's",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azdevops.BuildWIQLFilters(tt.opts)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("BuildWIQLFilters(%+v):\ngot:  %q\nmissing: %q", tt.opts, got, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("BuildWIQLFilters(%+v):\ngot:  %q\nshould not contain: %q", tt.opts, got, notWant)
				}
			}
		})
	}
}

// TestStateCategoryToWIQLStates verifies that each StateCategory maps to the
// expected set of WIQL state string literals.
func TestStateCategoryToWIQLStates(t *testing.T) {
	tests := []struct {
		name      string
		category  provider.StateCategory
		wantAll   []string // every element must appear in the output slice
		wantNone  []string // none of these may appear
	}{
		{
			name:     "New maps to New",
			category: provider.StateCategoryNew,
			wantAll:  []string{"New"},
		},
		{
			name:     "Active maps to Active",
			category: provider.StateCategoryActive,
			wantAll:  []string{"Active"},
		},
		{
			name:     "Resolved maps to Resolved",
			category: provider.StateCategoryResolved,
			wantAll:  []string{"Resolved"},
		},
		{
			name:     "ClosedDone maps to Closed",
			category: provider.StateCategoryClosedDone,
			wantAll:  []string{"Closed"},
		},
		{
			name:     "Removed maps to Removed",
			category: provider.StateCategoryRemoved,
			wantAll:  []string{"Removed"},
		},
		{
			name:     "Unknown maps to empty",
			category: provider.StateCategoryUnknown,
			wantAll:  []string{},
		},
		{
			name:     "ReadyForTest produces at least one entry",
			category: provider.StateCategoryReadyForTest,
			wantAll:  []string{}, // non-empty checked separately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azdevops.StateCategoryToWIQLStates(tt.category)
			gotSet := make(map[string]bool, len(got))
			for _, s := range got {
				gotSet[s] = true
			}
			for _, want := range tt.wantAll {
				if !gotSet[want] {
					t.Errorf("StateCategoryToWIQLStates(%v) = %v, missing %q", tt.category, got, want)
				}
			}
			for _, nope := range tt.wantNone {
				if gotSet[nope] {
					t.Errorf("StateCategoryToWIQLStates(%v) = %v, must not contain %q", tt.category, got, nope)
				}
			}
			// ReadyForTest must produce at least one entry
			if tt.category == provider.StateCategoryReadyForTest && len(got) == 0 {
				t.Errorf("StateCategoryToWIQLStates(ReadyForTest) returned empty slice, want at least one entry")
			}
		})
	}
}
