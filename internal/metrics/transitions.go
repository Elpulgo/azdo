package metrics

import (
	"sort"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// GapAction tells the snapshot writer what to do for a given prev→curr state
// transition. "Write" = today's observation is enough. "NeedsFallback" =
// intermediate snapshot rows have to be synthesized from /updates because the
// state jumped more than one step or moved backwards.
type GapAction int

const (
	GapWrite         GapAction = iota // normal single-step transition (or first observation)
	GapNeedsFallback                  // multi-step forward jump or backward transition
)

// ClassifyTransition decides whether the writer can append today's observation
// directly, or whether the gap-fallback path has to fire /updates to recover
// the missing intermediate states.
//
// Rules:
//   - first observation (prev empty)              → write
//   - same state                                  → write
//   - legal single forward step in states.Order() → write
//   - everything else (multi-step jump, backward,
//     unknown state)                              → fallback
//
// `states` carries the configured workflow so teams with custom state names
// (e.g. Active / RFT / Done) get the same gap-detection behavior.
func ClassifyTransition(prev, curr string, states StateConfig) GapAction {
	if prev == "" {
		return GapWrite
	}
	if strings.EqualFold(strings.TrimSpace(prev), strings.TrimSpace(curr)) {
		return GapWrite
	}
	pi, pok := states.Index(prev)
	ci, cok := states.Index(curr)
	if !pok || !cok {
		return GapNeedsFallback
	}
	if ci == pi+1 {
		return GapWrite
	}
	return GapNeedsFallback
}

// StateTransition is a single state change at a point in time, extracted from
// the /updates revision history. Used by both the gap-fallback (PR 2) and the
// first-launch backfill (PR 3 — same struct, same fold helper).
type StateTransition struct {
	State string
	At    time.Time
}

// FromAzDevTransitions converts the azdevops layer's transition shape into the
// metrics layer's shape so the pure fold helpers in this package don't have to
// depend on azdevops types at the call site.
func FromAzDevTransitions(in []azdevops.WorkItemStateTransition) []StateTransition {
	out := make([]StateTransition, len(in))
	for i, t := range in {
		out[i] = StateTransition{State: t.State, At: t.At}
	}
	return out
}

// SynthesizeGapRows produces one daily snapshot row per calendar day strictly
// between `since` (exclusive) and `until` (exclusive), filling in the state
// the item was in on each of those days according to `transitions`. The row
// for `until` itself is NOT emitted — the caller already has today's
// observation and will write it separately.
//
// The template carries the static per-item fields (ID, Project, Tags, Points,
// AssignedTo, Iteration); only TS, State, StateSince and Source vary per row.
// All emitted rows are marked Source="updates".
//
// Pure helper — reused verbatim by the PR 3 backfill, which calls it with
// `since` = 90 days ago and `until` = today.
func SynthesizeGapRows(transitions []StateTransition, since, until time.Time, template Snapshot) []Snapshot {
	if len(transitions) == 0 {
		return nil
	}

	sorted := append([]StateTransition(nil), transitions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })

	// Walk one calendar day at a time across the (since, until) range.
	// For each day, find the latest transition whose `At` is on or before
	// that day, and emit a row stamped with that transition's state.
	startDay := stripTime(since).AddDate(0, 0, 1) // exclusive on `since`
	endDay := stripTime(until)                    // exclusive on `until`

	var rows []Snapshot
	for d := startDay; d.Before(endDay); d = d.AddDate(0, 0, 1) {
		state, stateSince, ok := stateOn(sorted, d)
		if !ok {
			continue
		}
		row := template
		row.TS = d.Format("2006-01-02")
		row.State = state
		row.StateSince = stateSince.UTC().Format(time.RFC3339)
		row.Source = SourceUpdates
		rows = append(rows, row)
	}
	return rows
}

// stateOn returns the state the item was in at the end of day `day`, plus the
// transition timestamp that put it there. Returns ok=false if the item had not
// yet been created on that day (i.e. all transitions are strictly after it).
func stateOn(sorted []StateTransition, day time.Time) (state string, since time.Time, ok bool) {
	// Latest transition whose timestamp is on or before the END of `day`.
	endOfDay := day.Add(24*time.Hour - time.Second)
	for i := len(sorted) - 1; i >= 0; i-- {
		if !sorted[i].At.After(endOfDay) {
			return sorted[i].State, sorted[i].At, true
		}
	}
	return "", time.Time{}, false
}

func stripTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
