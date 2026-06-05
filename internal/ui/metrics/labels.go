package metrics

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/config"
)

// stateLabels resolves the three column-header labels from the model's
// config. Order of precedence per state: explicit override in
// `metrics.state_labels`, then auto-derived from the configured state name.
//
// Returns (active, readyForTest, closed) labels.
func (m Model) stateLabels() (string, string, string) {
	sc := m.stateConfig()
	var lbl config.MetricsStates
	if m.config != nil {
		lbl = m.config.Metrics.StateLabels
	}
	return labelFor(sc.Active, lbl.Active),
		labelFor(sc.ReadyForTest, lbl.ReadyForTest),
		labelFor(sc.Closed, lbl.Closed)
}

// labelFor returns the override if non-empty, else an auto-derived
// abbreviation from `name`. The abbreviation rule:
//   - multi-word names → initials, lowercase ("Ready for Test" → "rft",
//     "In Progress" → "ip")
//   - single-word names → lowercase as-is ("Active" → "active",
//     "Done" → "done")
func labelFor(name, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	words := strings.Fields(strings.TrimSpace(name))
	if len(words) == 0 {
		return ""
	}
	if len(words) == 1 {
		return strings.ToLower(words[0])
	}
	var b strings.Builder
	for _, w := range words {
		b.WriteByte(strings.ToLower(w)[0])
	}
	return b.String()
}

// labelTitle returns the same label as labelFor but with the first letter
// upper-cased so column headers feel like proper titles ("Active",
// "RFT" → "Rft"; the override survives intact if the user wrote "RFT").
func labelTitle(name, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	return titleCase(labelFor(name, ""))
}

// titleCase upper-cases the first byte of an ASCII string. Safe for the
// short labels we deal with here (state names, abbreviations).
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

