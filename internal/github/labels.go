package github

import (
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
)

// LabelConvention configures the label prefixes used to derive ItemType and
// Priority from GitHub issue labels. Prefixes are matched case-insensitively.
//
// Example: a label "type:bug" matches TypePrefix "type:" and yields
// provider.ItemTypeBug. A label "priority:p1" matches PriorityPrefix
// "priority:" and yields priority 1.
//
// Phase 4 will wire these from user config; Phase 3 always uses
// DefaultLabelConvention.
type LabelConvention struct {
	// TypePrefix is the label prefix used to derive ItemType.
	// Default: "type:"
	TypePrefix string
	// PriorityPrefix is the label prefix used to derive Priority.
	// Default: "priority:"
	PriorityPrefix string
}

// DefaultLabelConvention returns a LabelConvention with the conventional
// defaults: TypePrefix "type:" and PriorityPrefix "priority:".
func DefaultLabelConvention() LabelConvention {
	return LabelConvention{
		TypePrefix:     "type:",
		PriorityPrefix: "priority:",
	}
}

// Parse inspects a slice of GitHub labels and derives:
//
//   - itemType: the provider.ItemType for the first label matching TypePrefix
//     (case-insensitive). Unrecognised type values and the absence of any
//     type: label both default to provider.ItemTypeIssue — a GitHub issue is
//     natively an issue when the convention is not applied.
//
//   - priority: the numeric priority from the first label matching
//     PriorityPrefix (case-insensitive). Accepts both the "p1"–"p4" form and
//     bare "1"–"4". Values outside 1–4 or unparseable values yield 0 (unset);
//     the work-item view renders 0 as "-".
//
//   - tags: all labels that match neither prefix, joined with "; " in their
//     original order. The "; " separator matches the separator consumed by the
//     work-item view's tagList / applyTagFilter functions.
//
// When multiple labels share the same prefix the first match wins; subsequent
// labels with that prefix are treated as unmatched and become tags.
func (c LabelConvention) Parse(labels []Label) (itemType provider.ItemType, priority int, tags string) {
	typePfx := strings.ToLower(c.TypePrefix)
	priPfx := strings.ToLower(c.PriorityPrefix)

	typeMatched := false
	priMatched := false

	var tagParts []string

	for _, lbl := range labels {
		lower := strings.ToLower(lbl.Name)

		if !typeMatched && strings.HasPrefix(lower, typePfx) {
			value := strings.TrimSpace(lbl.Name[len(c.TypePrefix):])
			itemType = mapItemType(strings.ToLower(value))
			typeMatched = true
			continue
		}

		if !priMatched && strings.HasPrefix(lower, priPfx) {
			value := strings.TrimSpace(lbl.Name[len(c.PriorityPrefix):])
			priority = parsePriority(strings.ToLower(value))
			priMatched = true
			continue
		}

		tagParts = append(tagParts, lbl.Name)
	}

	// Default ItemType for GitHub is ItemTypeIssue — a GitHub issue is
	// natively an issue when no type: label is present or when the value is
	// unrecognised.
	if !typeMatched {
		itemType = provider.ItemTypeIssue
	}

	tags = strings.Join(tagParts, "; ")
	return itemType, priority, tags
}

// mapItemType converts a lower-cased label value (after stripping the type:
// prefix) to a provider.ItemType. Unrecognised values return ItemTypeIssue.
func mapItemType(value string) provider.ItemType {
	switch value {
	case "bug":
		return provider.ItemTypeBug
	case "task":
		return provider.ItemTypeTask
	case "story", "user story", "userstory":
		return provider.ItemTypeUserStory
	case "feature":
		return provider.ItemTypeFeature
	case "epic":
		return provider.ItemTypeEpic
	case "issue":
		return provider.ItemTypeIssue
	default:
		// Unrecognised type: value → fall back to the GitHub native default.
		return provider.ItemTypeIssue
	}
}

// parsePriority converts a lower-cased priority label value (after stripping
// the priority: prefix) to an integer in the range 1–4. Accepts "p1"–"p4"
// and bare "1"–"4". Out-of-range or unparseable values return 0 (unset).
func parsePriority(value string) int {
	// Strip optional leading "p".
	s := value
	if strings.HasPrefix(s, "p") {
		s = s[1:]
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 4 {
		return 0
	}
	return n
}
