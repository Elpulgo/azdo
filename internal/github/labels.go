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
//   - itemType: the provider.ItemType for the first label whose value matches
//     TypePrefix (case-insensitive) AND maps to a known type. Unrecognised type
//     values and the absence of any type: label both default to
//     provider.ItemTypeIssue — a GitHub issue is natively an issue when the
//     convention is not applied.
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
// A label that matches a prefix is consumed (removed from tags) only when it
// yields a usable value — a recognised type, or a priority in 1–4. A prefixed
// label whose value does NOT map (e.g. "priority:high", "type:chore") is kept
// visible as a tag rather than silently dropped, and does not satisfy the
// match — a later well-formed label with the same prefix can still win.
//
// An empty prefix never matches: a zero-value LabelConvention routes every
// label to tags rather than greedily consuming the first ones.
func (c LabelConvention) Parse(labels []Label) (itemType provider.ItemType, priority int, tags string) {
	typePfx := strings.ToLower(c.TypePrefix)
	priPfx := strings.ToLower(c.PriorityPrefix)

	typeMatched := false
	priMatched := false

	var tagParts []string

	for _, lbl := range labels {
		lower := strings.ToLower(lbl.Name)

		if !typeMatched && typePfx != "" && strings.HasPrefix(lower, typePfx) {
			value := strings.TrimSpace(lbl.Name[len(c.TypePrefix):])
			if mapped, ok := mapItemType(strings.ToLower(value)); ok {
				itemType = mapped
				typeMatched = true
				continue
			}
			// Recognised prefix, unrecognised value: surface it as a tag.
			tagParts = append(tagParts, lbl.Name)
			continue
		}

		if !priMatched && priPfx != "" && strings.HasPrefix(lower, priPfx) {
			value := strings.TrimSpace(lbl.Name[len(c.PriorityPrefix):])
			if p := parsePriority(strings.ToLower(value)); p != 0 {
				priority = p
				priMatched = true
				continue
			}
			// Recognised prefix, unparseable/out-of-range value: keep as a tag.
			tagParts = append(tagParts, lbl.Name)
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
// prefix) to a provider.ItemType. The bool reports whether the value was a
// recognised type; an unrecognised value returns (ItemTypeIssue, false) so the
// caller can keep the original label as a tag instead of dropping it.
func mapItemType(value string) (provider.ItemType, bool) {
	switch value {
	case "bug":
		return provider.ItemTypeBug, true
	case "task":
		return provider.ItemTypeTask, true
	case "story", "user story", "userstory":
		return provider.ItemTypeUserStory, true
	case "feature":
		return provider.ItemTypeFeature, true
	case "epic":
		return provider.ItemTypeEpic, true
	case "issue":
		return provider.ItemTypeIssue, true
	default:
		// Unrecognised type: value → not a match; caller keeps it as a tag.
		return provider.ItemTypeIssue, false
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
