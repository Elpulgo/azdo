// Package metrics holds pure aggregation logic for the metrics tab.
//
// It depends only on internal/azdevops types and has no I/O or UI, so the
// core can be exercised entirely by table-driven tests.
package metrics

import (
	"sort"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// Thresholds carries the configurable cut-offs that drive flagging
// and the overloaded marker on the per-user table.
type Thresholds struct {
	ActiveStaleDays int // dwell in Active longer than this -> active-stale flag
	RFTStaleDays    int // dwell in Ready for Test longer than this -> rft-stale flag
	WIPLimit        int // strictly more in-flight items than this -> Overloaded
}

// UserMetrics is one row of the per-developer table.
type UserMetrics struct {
	User         string
	InFlight     int           // Active + Ready for Test
	ActiveCount  int
	RFTCount     int
	PointsClosed float64       // story points on items closed inside the interval
	OldestActive time.Duration // worst current-state dwell among Active items
	OldestRFT    time.Duration // worst current-state dwell among RFT items
	Stalled      int           // count of this user's items that tripped a threshold
	Overloaded   bool          // InFlight > Thresholds.WIPLimit
}

// ItemFlag is one row of the worst-first stuck-items digest.
type ItemFlag struct {
	ID      int
	Title   string
	Project string
	User    string
	State   string
	Dwell   time.Duration
	Reason  string // "active-stale" or "rft-stale"
}

const (
	stateActive = "active"
	stateRFT    = "ready for test"
	stateClosed = "closed"

	reasonActiveStale = "active-stale"
	reasonRFTStale    = "rft-stale"
)

// Aggregate rolls a flat slice of work items into per-developer rows and a
// flat list of flagged items, applying `th` thresholds. Items in states other
// than Active / Ready for Test / Closed are ignored.
//
// Closed items contribute to PointsClosed only when their ClosedDate falls
// strictly after intervalStart.
func Aggregate(items []azdevops.WorkItem, intervalStart, now time.Time, th Thresholds) ([]UserMetrics, []ItemFlag) {
	byUser := make(map[string]*UserMetrics)
	get := func(u string) *UserMetrics {
		if byUser[u] == nil {
			byUser[u] = &UserMetrics{User: u}
		}
		return byUser[u]
	}

	activeStale := time.Duration(th.ActiveStaleDays) * 24 * time.Hour
	rftStale := time.Duration(th.RFTStaleDays) * 24 * time.Hour

	var flags []ItemFlag

	for i := range items {
		wi := items[i]
		user := wi.AssignedToName()
		dwell := wi.TimeInCurrentState(now)

		switch strings.ToLower(wi.Fields.State) {
		case stateActive:
			um := get(user)
			um.InFlight++
			um.ActiveCount++
			if dwell > um.OldestActive {
				um.OldestActive = dwell
			}
			if dwell > activeStale {
				um.Stalled++
				flags = append(flags, ItemFlag{
					ID:      wi.ID,
					Title:   wi.Fields.Title,
					Project: wi.ProjectDisplayName,
					User:    user,
					State:   wi.Fields.State,
					Dwell:   dwell,
					Reason:  reasonActiveStale,
				})
			}
		case stateRFT:
			um := get(user)
			um.InFlight++
			um.RFTCount++
			if dwell > um.OldestRFT {
				um.OldestRFT = dwell
			}
			if dwell > rftStale {
				um.Stalled++
				flags = append(flags, ItemFlag{
					ID:      wi.ID,
					Title:   wi.Fields.Title,
					Project: wi.ProjectDisplayName,
					User:    user,
					State:   wi.Fields.State,
					Dwell:   dwell,
					Reason:  reasonRFTStale,
				})
			}
		case stateClosed:
			if wi.IsCompletedSince(intervalStart) {
				um := get(user)
				um.PointsClosed += wi.EffectivePoints()
			}
		}
	}

	rows := make([]UserMetrics, 0, len(byUser))
	for _, um := range byUser {
		um.Overloaded = um.InFlight > th.WIPLimit
		rows = append(rows, *um)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Stalled != rows[j].Stalled {
			return rows[i].Stalled > rows[j].Stalled
		}
		return rows[i].InFlight > rows[j].InFlight
	})
	sort.Slice(flags, func(i, j int) bool { return flags[i].Dwell > flags[j].Dwell })

	return rows, flags
}
