package metrics

import (
	"sort"
	"strings"
	"time"
)

// SprintWindow is the time range during which a sprint tag was active in the
// snapshot file. End is inclusive — Trends counts a day fully if any item
// carried the tag on that day.
type SprintWindow struct {
	Tag   string
	Start time.Time
	End   time.Time
}

// TrendRow is one row of the sprint-on-sprint comparison: a user and one
// TrendCell per sprint window passed to TrendAggregate.
type TrendRow struct {
	User  string
	Cells []TrendCell
}

// TrendCell carries the four numbers rendered in each sprint's column for one
// user, plus the overload flag.
type TrendCell struct {
	Points           float64       // story points on items closed inside the window
	AvgWIP           float64       // average daily in-flight count (Active+RFT)
	StuckCount       int           // distinct items that exceeded a stale threshold on at least one day in the window
	CycleTime        time.Duration // average (first-Active → first-Closed) for items closed in the window
	OverloadedAnyDay bool          // peak daily in-flight > WIPLimit at any point in the window
}

// DeriveSprintWindow returns the time range for `tag` derived purely from
// snapshot rows.
//   - Start = earliest TS the tag appears in any state.
//   - End:
//   - If there are open (non-Closed) observations later than the latest
//     Closed observation, the sprint is still in flight → End = now.
//   - Otherwise the sprint has wrapped → End = latest non-Closed TS (the
//     last day the sprint had items in flight). If every observation is
//     Closed, fall back to the latest TS seen.
//
// ok=false if the tag is never seen in the snapshot.
func DeriveSprintWindow(snaps []Snapshot, tag string, now time.Time) (SprintWindow, bool) {
	var earliest, latestOpen, latestClosed, latestAny string
	for _, s := range snaps {
		if !hasTag(s, tag) {
			continue
		}
		if earliest == "" || s.TS < earliest {
			earliest = s.TS
		}
		if latestAny == "" || s.TS > latestAny {
			latestAny = s.TS
		}
		if strings.EqualFold(s.State, "Closed") {
			if latestClosed == "" || s.TS > latestClosed {
				latestClosed = s.TS
			}
		} else {
			if latestOpen == "" || s.TS > latestOpen {
				latestOpen = s.TS
			}
		}
	}
	if earliest == "" {
		return SprintWindow{}, false
	}

	start, _ := time.Parse("2006-01-02", earliest)
	var end time.Time
	switch {
	case latestOpen != "" && latestOpen > latestClosed:
		// Sprint still in flight (open observations newer than any Closed).
		end = now
	case latestOpen != "":
		end, _ = time.Parse("2006-01-02", latestOpen)
	default:
		// All observations Closed — pin to the last seen row.
		end, _ = time.Parse("2006-01-02", latestAny)
	}
	return SprintWindow{Tag: tag, Start: start, End: end}, true
}

// TrendAggregate folds snapshot rows into per-user × per-sprint cells. Pure.
//
// For each window, only rows whose Tags include the window's Tag AND whose
// snapshot TS falls inside [Start, End] contribute. Rows belonging to other
// sprints are ignored.
func TrendAggregate(snaps []Snapshot, windows []SprintWindow, th Thresholds, now time.Time) []TrendRow {
	if len(windows) == 0 {
		return nil
	}

	// users[name] = true means we've seen at least one row for that user
	// in any window — so we render their row.
	users := make(map[string]bool)
	// per (user, windowIdx) accumulators
	type acc struct {
		closedPoints    map[int]float64                        // item → points, summed once at finalize
		dailyWIP        map[string]map[int]struct{}            // day → set of item IDs in-flight
		stateDays       map[int]map[string]map[string]struct{} // item → state → set of TS
		closedItemFirst map[int]time.Time                      // item → first-Active observation
		closedItemDone  map[int]time.Time                      // item → first-Closed observation
	}
	cells := make([][]*acc, len(windows))
	for i := range cells {
		cells[i] = nil
	}
	// index keyed by user name for fast row lookup per window
	type key struct {
		user string
		wIdx int
	}
	store := make(map[key]*acc)
	getAcc := func(user string, wIdx int) *acc {
		k := key{user, wIdx}
		a := store[k]
		if a == nil {
			a = &acc{
				closedPoints:    make(map[int]float64),
				dailyWIP:        make(map[string]map[int]struct{}),
				stateDays:       make(map[int]map[string]map[string]struct{}),
				closedItemFirst: make(map[int]time.Time),
				closedItemDone:  make(map[int]time.Time),
			}
			store[k] = a
		}
		return a
	}

	_ = th // thresholds applied at finalize, see stuck-count calc below

	for _, s := range snaps {
		// Skip unassigned rows — they're not a developer signal and would
		// otherwise produce a no-owner row in the grid.
		if s.AssignedTo == "" || s.AssignedTo == "-" {
			continue
		}
		d, err := time.Parse("2006-01-02", s.TS)
		if err != nil {
			continue
		}
		for wIdx, w := range windows {
			if !hasTag(s, w.Tag) {
				continue
			}
			if d.Before(stripTime(w.Start)) || d.After(stripTime(w.End)) {
				continue
			}
			users[s.AssignedTo] = true
			a := getAcc(s.AssignedTo, wIdx)

			state := strings.ToLower(s.State)

			// Track observed days per (item, state) so we can compute stuck
			// from observations alone (no reliance on StateSince).
			if state == "active" || state == "ready for test" {
				if a.stateDays[s.ID] == nil {
					a.stateDays[s.ID] = make(map[string]map[string]struct{})
				}
				if a.stateDays[s.ID][state] == nil {
					a.stateDays[s.ID][state] = make(map[string]struct{})
				}
				a.stateDays[s.ID][state][s.TS] = struct{}{}
			}

			switch state {
			case "active", "ready for test":
				if a.dailyWIP[s.TS] == nil {
					a.dailyWIP[s.TS] = make(map[int]struct{})
				}
				a.dailyWIP[s.TS][s.ID] = struct{}{}

				if state == "active" {
					if cur, ok := a.closedItemFirst[s.ID]; !ok || d.Before(cur) {
						a.closedItemFirst[s.ID] = d
					}
				}
			case "closed":
				if cur, ok := a.closedItemDone[s.ID]; !ok || d.Before(cur) {
					a.closedItemDone[s.ID] = d
				}
				// Record the item's points keyed by ID — multiple daily Closed
				// rows for the same item overwrite the same key, so the total
				// is summed once per distinct item at finalize.
				a.closedPoints[s.ID] = s.Points
			}
		}
	}

	// Build rows, one per user, in alphabetical order.
	names := make([]string, 0, len(users))
	for u := range users {
		names = append(names, u)
	}
	sort.Strings(names)

	rows := make([]TrendRow, len(names))
	for i, u := range names {
		row := TrendRow{User: u, Cells: make([]TrendCell, len(windows))}
		for wIdx, w := range windows {
			a := store[key{u, wIdx}]
			if a == nil {
				continue
			}

			// Average WIP across the window's days (count days w/ activity).
			total, days := 0, 0
			peak := 0
			for _, ids := range a.dailyWIP {
				n := len(ids)
				if n > peak {
					peak = n
				}
				total += n
				days++
			}
			avg := 0.0
			if days > 0 {
				avg = float64(total) / float64(days)
			}

			// Cycle time: average across items that have both a first-Active
			// and a first-Closed observation inside this window.
			var sumDur time.Duration
			closedN := 0
			for id, done := range a.closedItemDone {
				start, ok := a.closedItemFirst[id]
				if !ok {
					continue
				}
				dur := done.Sub(start)
				if dur < 0 {
					continue
				}
				sumDur += dur
				closedN++
			}
			cy := time.Duration(0)
			if closedN > 0 {
				cy = sumDur / time.Duration(closedN)
			}

			// Stuck count: distinct items observed in the same in-flight
			// state for MORE than the threshold's day count within this window.
			stuckSet := make(map[int]struct{})
			for id, byState := range a.stateDays {
				if th.ActiveStaleDays > 0 {
					if days := byState["active"]; len(days) > th.ActiveStaleDays {
						stuckSet[id] = struct{}{}
					}
				}
				if th.RFTStaleDays > 0 {
					if days := byState["ready for test"]; len(days) > th.RFTStaleDays {
						stuckSet[id] = struct{}{}
					}
				}
			}

			var points float64
			for _, p := range a.closedPoints {
				points += p
			}

			row.Cells[wIdx] = TrendCell{
				Points:           points,
				AvgWIP:           avg,
				StuckCount:       len(stuckSet),
				CycleTime:        cy,
				OverloadedAnyDay: th.WIPLimit > 0 && peak > th.WIPLimit,
			}
			_ = w // window is captured in row order, not stored per-cell
		}
		rows[i] = row
	}
	return rows
}

// hasTag reports whether the snapshot row carries the given tag.
func hasTag(s Snapshot, tag string) bool {
	for _, t := range s.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

