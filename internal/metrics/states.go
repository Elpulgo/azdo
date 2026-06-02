package metrics

import "strings"

// StateConfig carries the configured canonical names for the three workflow
// states the metrics tab buckets on. All matching against snapshot or live
// state strings is case-insensitive — different ADO projects and historical
// data often spell the same state differently ("Ready for Test" vs
// "Ready for test").
type StateConfig struct {
	Active       string
	ReadyForTest string
	Closed       string
}

// DefaultStates returns the historical hardcoded names. Used by tests and as
// a safety net when the config layer hasn't populated a StateConfig.
func DefaultStates() StateConfig {
	return StateConfig{
		Active:       "Active",
		ReadyForTest: "Ready for Test",
		Closed:       "Closed",
	}
}

// IsActive reports whether s matches the configured Active state (case-
// insensitive, trimmed).
func (sc StateConfig) IsActive(s string) bool {
	return eqState(s, sc.Active)
}

// IsRFT reports whether s matches the configured Ready-for-Test state.
func (sc StateConfig) IsRFT(s string) bool {
	return eqState(s, sc.ReadyForTest)
}

// IsClosed reports whether s matches the configured Closed state.
func (sc StateConfig) IsClosed(s string) bool {
	return eqState(s, sc.Closed)
}

// Order returns the three configured states in workflow order:
// Active → Ready for Test → Closed. The gap-fallback classifier uses this
// to decide whether a transition between two observed states is a legal
// single forward step.
func (sc StateConfig) Order() []string {
	return []string{sc.Active, sc.ReadyForTest, sc.Closed}
}

// Index returns the position of `s` in Order() (case-insensitive). ok=false
// when `s` does not match any of the three configured states — typically a
// New/Removed observation, or a custom state we don't track.
func (sc StateConfig) Index(s string) (int, bool) {
	switch {
	case sc.IsActive(s):
		return 0, true
	case sc.IsRFT(s):
		return 1, true
	case sc.IsClosed(s):
		return 2, true
	}
	return -1, false
}

func eqState(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
