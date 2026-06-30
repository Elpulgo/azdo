package github

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

func labels(names ...string) []Label {
	out := make([]Label, len(names))
	for i, n := range names {
		out[i] = Label{Name: n}
	}
	return out
}

func TestLabelConventionParse(t *testing.T) {
	def := DefaultLabelConvention()

	tests := []struct {
		name         string
		labels       []Label
		wantType     provider.ItemType
		wantPriority int
		wantTags     string
	}{
		// ── no-match defaults ──────────────────────────────────────────────────
		{
			name:         "empty labels → ItemTypeIssue, 0, empty tags",
			labels:       nil,
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "no matching labels → ItemTypeIssue, 0, all labels as tags",
			labels:       labels("enhancement", "good first issue"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "enhancement; good first issue",
		},

		// ── type: values ───────────────────────────────────────────────────────
		{
			name:         "type:bug → ItemTypeBug",
			labels:       labels("type:bug"),
			wantType:     provider.ItemTypeBug,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:task → ItemTypeTask",
			labels:       labels("type:task"),
			wantType:     provider.ItemTypeTask,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:story → ItemTypeUserStory",
			labels:       labels("type:story"),
			wantType:     provider.ItemTypeUserStory,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:user story → ItemTypeUserStory",
			labels:       labels("type:user story"),
			wantType:     provider.ItemTypeUserStory,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:userstory → ItemTypeUserStory",
			labels:       labels("type:userstory"),
			wantType:     provider.ItemTypeUserStory,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:feature → ItemTypeFeature",
			labels:       labels("type:feature"),
			wantType:     provider.ItemTypeFeature,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:epic → ItemTypeEpic",
			labels:       labels("type:epic"),
			wantType:     provider.ItemTypeEpic,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "type:issue → ItemTypeIssue",
			labels:       labels("type:issue"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "unrecognised type value → ItemTypeIssue, kept as tag",
			labels:       labels("type:unknown-future-kind"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "type:unknown-future-kind",
		},

		// ── case-insensitivity ────────────────────────────────────────────────
		{
			name:         "Type:BUG (mixed case prefix+value) → ItemTypeBug",
			labels:       labels("Type:BUG"),
			wantType:     provider.ItemTypeBug,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "TYPE:Task → ItemTypeTask",
			labels:       labels("TYPE:Task"),
			wantType:     provider.ItemTypeTask,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "TYPE:EPIC → ItemTypeEpic",
			labels:       labels("TYPE:EPIC"),
			wantType:     provider.ItemTypeEpic,
			wantPriority: 0,
			wantTags:     "",
		},

		// ── priority: p-form ──────────────────────────────────────────────────
		{
			name:         "priority:p1 → 1",
			labels:       labels("priority:p1"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 1,
			wantTags:     "",
		},
		{
			name:         "priority:p2 → 2",
			labels:       labels("priority:p2"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 2,
			wantTags:     "",
		},
		{
			name:         "priority:p3 → 3",
			labels:       labels("priority:p3"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 3,
			wantTags:     "",
		},
		{
			name:         "priority:p4 → 4",
			labels:       labels("priority:p4"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 4,
			wantTags:     "",
		},

		// ── priority: bare-number form ────────────────────────────────────────
		{
			name:         "priority:1 → 1",
			labels:       labels("priority:1"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 1,
			wantTags:     "",
		},
		{
			name:         "priority:4 → 4",
			labels:       labels("priority:4"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 4,
			wantTags:     "",
		},

		// ── priority: case-insensitivity ──────────────────────────────────────
		{
			name:         "Priority:P1 → 1",
			labels:       labels("Priority:P1"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 1,
			wantTags:     "",
		},
		{
			name:         "PRIORITY:P4 → 4",
			labels:       labels("PRIORITY:P4"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 4,
			wantTags:     "",
		},

		// ── garbage / out-of-range priority → 0 ──────────────────────────────
		{
			name:         "priority:p0 → 0 (out of range), kept as tag",
			labels:       labels("priority:p0"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "priority:p0",
		},
		{
			name:         "priority:p5 → 0 (out of range), kept as tag",
			labels:       labels("priority:p5"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "priority:p5",
		},
		{
			name:         "priority:high → 0 (garbage), kept as tag",
			labels:       labels("priority:high"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "priority:high",
		},
		{
			name:         "priority:0 → 0 (out of range), kept as tag",
			labels:       labels("priority:0"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "priority:0",
		},

		// ── unmatched labels → tags, order preserved ──────────────────────────
		{
			name:         "two unmatched labels → tags in order",
			labels:       labels("enhancement", "good first issue"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "enhancement; good first issue",
		},

		// ── mixed bag: type + priority + plain labels ─────────────────────────
		{
			name:         "type:bug + priority:p2 + two plain labels",
			labels:       labels("type:bug", "priority:p2", "backend", "needs-review"),
			wantType:     provider.ItemTypeBug,
			wantPriority: 2,
			wantTags:     "backend; needs-review",
		},

		// ── recognised prefix, bad value → kept as tag; later valid one wins ──
		{
			name:         "type:chore (unrecognised) then type:bug → bug wins, chore tagged",
			labels:       labels("type:chore", "type:bug"),
			wantType:     provider.ItemTypeBug,
			wantPriority: 0,
			wantTags:     "type:chore",
		},
		{
			name:         "priority:high (garbage) then priority:p2 → 2, high tagged",
			labels:       labels("priority:high", "priority:p2"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 2,
			wantTags:     "priority:high",
		},

		// ── bare prefix (empty value) → no panic, kept as tag ─────────────────
		{
			name:         "bare type: → ItemTypeIssue, kept as tag",
			labels:       labels("type:"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "type:",
		},
		{
			name:         "bare priority: → 0, kept as tag",
			labels:       labels("priority:"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "priority:",
		},

		// ── first-match-wins on duplicates ────────────────────────────────────
		{
			name:         "two type: labels → first wins, second becomes tag",
			labels:       labels("type:bug", "type:task"),
			wantType:     provider.ItemTypeBug,
			wantPriority: 0,
			wantTags:     "type:task",
		},
		{
			name:         "two priority: labels → first wins, second becomes tag",
			labels:       labels("priority:p1", "priority:p3"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 1,
			wantTags:     "priority:p3",
		},

		// ── whitespace trimming around value ─────────────────────────────────
		{
			name:         "type: bug (space around value) → ItemTypeBug",
			labels:       labels("type: bug"),
			wantType:     provider.ItemTypeBug,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "priority: p2 (space around value) → 2",
			labels:       labels("priority: p2"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 2,
			wantTags:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotPriority, gotTags := def.Parse(tc.labels)
			if gotType != tc.wantType {
				t.Errorf("itemType: got %v, want %v", gotType, tc.wantType)
			}
			if gotPriority != tc.wantPriority {
				t.Errorf("priority: got %d, want %d", gotPriority, tc.wantPriority)
			}
			if gotTags != tc.wantTags {
				t.Errorf("tags: got %q, want %q", gotTags, tc.wantTags)
			}
		})
	}
}

// TestLabelConventionInjectablePrefixes verifies that a custom LabelConvention
// (non-default prefixes) routes correctly, proving the struct is injectable.
func TestLabelConventionInjectablePrefixes(t *testing.T) {
	custom := LabelConvention{
		TypePrefix:     "kind:",
		PriorityPrefix: "sev:",
	}

	lbls := labels("kind:bug", "sev:p1", "backlog")
	gotType, gotPri, gotTags := custom.Parse(lbls)

	if gotType != provider.ItemTypeBug {
		t.Errorf("itemType: got %v, want ItemTypeBug", gotType)
	}
	if gotPri != 1 {
		t.Errorf("priority: got %d, want 1", gotPri)
	}
	if gotTags != "backlog" {
		t.Errorf("tags: got %q, want %q", gotTags, "backlog")
	}
}

// TestLabelConventionEmptyPrefixMatchesNothing verifies that a zero-value
// LabelConvention (empty prefixes — e.g. a Phase 4 config that leaves a prefix
// blank) does NOT greedily consume the first labels as type/priority. Every
// label must route to tags, and type/priority stay at their unset defaults.
func TestLabelConventionEmptyPrefixMatchesNothing(t *testing.T) {
	var zero LabelConvention // TypePrefix == "" && PriorityPrefix == ""

	lbls := labels("type:bug", "priority:p1", "backend")
	gotType, gotPri, gotTags := zero.Parse(lbls)

	if gotType != provider.ItemTypeIssue {
		t.Errorf("itemType: got %v, want ItemTypeIssue (no prefix should match)", gotType)
	}
	if gotPri != 0 {
		t.Errorf("priority: got %d, want 0 (no prefix should match)", gotPri)
	}
	if gotTags != "type:bug; priority:p1; backend" {
		t.Errorf("tags: got %q, want all labels routed to tags", gotTags)
	}
}

// TestLabelConventionEmptyTypePrefixOnly verifies the guard is per-prefix: a
// blank TypePrefix routes type-like labels to tags while a present
// PriorityPrefix still resolves.
func TestLabelConventionEmptyTypePrefixOnly(t *testing.T) {
	c := LabelConvention{TypePrefix: "", PriorityPrefix: "priority:"}

	lbls := labels("type:bug", "priority:p3")
	gotType, gotPri, gotTags := c.Parse(lbls)

	if gotType != provider.ItemTypeIssue {
		t.Errorf("itemType: got %v, want ItemTypeIssue (blank TypePrefix)", gotType)
	}
	if gotPri != 3 {
		t.Errorf("priority: got %d, want 3", gotPri)
	}
	if gotTags != "type:bug" {
		t.Errorf("tags: got %q, want %q", gotTags, "type:bug")
	}
}

// TestDefaultLabelConvention checks that DefaultLabelConvention returns the
// documented prefix values.
func TestDefaultLabelConvention(t *testing.T) {
	c := DefaultLabelConvention()
	if c.TypePrefix != "type:" {
		t.Errorf("TypePrefix: got %q, want %q", c.TypePrefix, "type:")
	}
	if c.PriorityPrefix != "priority:" {
		t.Errorf("PriorityPrefix: got %q, want %q", c.PriorityPrefix, "priority:")
	}
}
