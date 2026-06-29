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
			name:         "unrecognised type value → ItemTypeIssue",
			labels:       labels("type:unknown-future-kind"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
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
			name:         "priority:p0 → 0 (out of range)",
			labels:       labels("priority:p0"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "priority:p5 → 0 (out of range)",
			labels:       labels("priority:p5"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "priority:high → 0 (garbage)",
			labels:       labels("priority:high"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
		},
		{
			name:         "priority:0 → 0 (out of range)",
			labels:       labels("priority:0"),
			wantType:     provider.ItemTypeIssue,
			wantPriority: 0,
			wantTags:     "",
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
