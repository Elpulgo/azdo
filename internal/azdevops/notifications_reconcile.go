package azdevops

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// orphanTTL is how long a TriageEntry survives in local state after its
// subject last appeared in a poll's rows before Reconcile prunes it.
// Decision 6 bounds every source to a lookback window (14 days by default),
// so a subject that has genuinely stopped matching any source's query will
// not reappear; orphanTTL only exists so the state file does not grow
// without bound (decision 7). It is generous relative to the default
// lookback only while the lookbackDays a caller is using stays
// <= MaxNotificationLookbackDays: beyond that, "inside the query window" and
// "returned by a given poll" are different sets (a source like
// SourceAssigned caps each query at assignedQueryTop, so an
// old-but-still-in-window subject can be crowded out of every poll's
// results, its LastSeen frozen, and pruned before the window says it
// should be). NewAdapterWithNotifications clamps any lookbackDays it is
// given to MaxNotificationLookbackDays, which closes that gap for the
// Adapter.List call path — internal/config.AzureLookbackDaysMax adds a
// second, earlier clamp of its own in LoadFrom (duplicating this same
// number in days, since config deliberately does not import this package
// for one constant), before Validate() ever runs. Neither clamp reaches
// SourceAssigned, SourceCIFailed or Reconcile directly: all three are
// exported, take lookbackDays (the two sources) or a TriageState (Reconcile)
// as plain parameters with no bound enforced inside them, so a caller
// composing this package's pieces without going through
// NewAdapterWithNotifications can still reopen the gap. If this constant's
// value ever changes, MaxNotificationLookbackDays (below) updates itself —
// it is derived from orphanTTL, not restated — but
// internal/config.AzureLookbackDaysMax does not and must be changed by hand;
// that constant's own comment points back here for the same reason.
const orphanTTL = 30 * 24 * time.Hour

// MaxNotificationLookbackDays is orphanTTL expressed in days rather than a
// time.Duration: the upper bound NewAdapterWithNotifications clamps
// lookbackDays to. Derived from orphanTTL, not restated, so the two cannot
// disagree within this package. internal/config.AzureLookbackDaysMax mirrors
// this exact number for its own, earlier clamp in LoadFrom; a one-line
// equality test in cmd/azdo-tui (which already imports both packages) pins
// the two staying equal — see
// TestAzureLookbackDaysMax_ConfigAndAdapterAgree.
const MaxNotificationLookbackDays = int(orphanTTL / (24 * time.Hour))

// NotifKey builds the stable identity key for a locally-tracked synthetic
// notification subject (decision 2): "<source>/<entity>/<entity_id>". This
// is the value stored in provider.Identity.ID — it names the subject, never
// the activity that made it interesting (decision 2's rejected
// stamp-in-key alternative).
//
// id is guarded per convention 11: a non-positive id (<= 0, not just == 0)
// is rejected and yields "", since a negative id would otherwise produce a
// malformed-but-non-empty key.
//
// The empty string is not a valid key and must never be used as a map key
// into local triage state: distinct malformed subjects would otherwise
// collapse into a single shared entry, each clobbering the other's
// Read/Done/LastActivity. Every caller of NotifKey — tasks 4-7's sources —
// shares this one contract; Reconcile enforces it on the read side by
// guarding key == "" itself, so a malformed row can never reach the state
// map regardless of which source produced it. Reconcile goes further than
// just guarding the map: it drops such a row from its returned slice
// entirely, rather than passing it through into the feed. A row with no
// usable key can never be marked read or done (MarkRead/MarkDone reject an
// empty Identity.ID outright), so rendering it would create a permanently
// un-triageable row and an error banner on every attempt to clear it.
func NotifKey(source, entity string, id int) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%d", source, entity, id)
}

// Reconcile folds locally-tracked read/done state into a freshly computed
// set of notification rows and returns both the resulting rows and the
// updated state, without mutating either input (decision 2's "pure
// function over (rows, state) -> rows" — the whole point is that this can
// be table-tested exhaustively rather than only end-to-end).
//
// For each row:
//   - a row whose Identity.ID is "" is dropped from the returned slice and
//     never enters the state map at all — an empty id is not a valid
//     NotifKey result and must never be used as a shared key that collapses
//     unrelated subjects into one entry (see NotifKey's doc comment). A
//     non-positive entity id is a malformed API response with no usable
//     identity, not something the four sources are expected to filter
//     themselves — Reconcile is the one choke point every source's rows
//     funnel through, so dropping the row here, once, is what keeps four
//     parallel per-source guards from drifting apart. The alternative —
//     rendering the row anyway — makes it permanently un-triageable: the
//     pane shows it, the user presses u or d, and MarkRead/MarkDone reject
//     the empty id outright, so the row and the resulting error both persist
//     forever.
//   - a subject with no existing entry is unread; a fresh entry is created
//     stamped with the row's UpdatedAt and LastSeen set to now.
//   - a subject whose row UpdatedAt is the zero value carries no activity
//     information at all — the source could not populate a stamp for this
//     poll, which is not the same claim as "this subject regressed to the
//     beginning of time". The stamp comparison is skipped entirely: the
//     stored Read/Done/LastActivity apply unchanged, and only LastSeen
//     advances, exactly like the unchanged-stamp case below.
//   - a subject whose stored LastActivity equals the row's UpdatedAt has
//     had no new activity since it was last seen: the stored Read/Done
//     apply to the returned row and LastSeen advances to now — this is the
//     every-poll case for a subject whose activity hasn't changed, so
//     advancing LastSeen here is what keeps a still-matching, unchanging
//     subject from ever being pruned as orphaned.
//   - a subject whose row UpdatedAt is strictly newer than the stored
//     LastActivity has new activity: Read/Done are cleared (the subject
//     resurfaces as unread), and both LastActivity and LastSeen advance.
//   - a subject whose row UpdatedAt is older than the stored LastActivity
//     (clock skew, or a reordered poll) leaves the triage state — Read,
//     Done and LastActivity — untouched, but LastSeen still advances to
//     now. LastSeen is presence bookkeeping, not triage: a subject that
//     keeps appearing in every poll with a regressed stamp must not be
//     TTL-pruned by the loop below just because its triage state is
//     frozen, or a dismissed item resurrects with no triggering event
//     (amended 2026-08-06 after review; decision 2's prose only ever
//     discusses clearing read/done, not LastSeen).
//
// Done rows (per the entry that ends up applying, stored or fresh) are
// dropped from the returned slice entirely — they are still triaged, so
// they no longer belong in the feed.
//
// Once every row has been folded, entries whose LastSeen is older than
// orphanTTL relative to now are pruned from the returned state — their
// subject has stopped matching any source's query, and decision 7 makes
// that pruning the only explicit cleanup work left once the feed is
// recomputed from scratch each poll.
//
// Task-9 invariant: an entry written outside Reconcile — by MarkRead or
// MarkDone for an id Reconcile has never seen — must carry a LastActivity
// at least as new as the row it was marked from. A zero-value LastActivity
// looks older than any real row.UpdatedAt, so the very next poll would take
// the "strictly newer" branch above and clear the mark within one poll
// interval, before the user has even navigated away.
func Reconcile(rows []provider.Notification, stored TriageState, now time.Time) ([]provider.Notification, TriageState) {
	newState := make(TriageState, len(stored))
	for k, v := range stored {
		newState[k] = v
	}

	result := make([]provider.Notification, 0, len(rows))
	for _, row := range rows {
		key := row.Identity.ID
		if key == "" {
			// Malformed subject: a non-positive entity id (NotifKey's own
			// convention-11 guard) means the source that produced this row
			// got a malformed API response with no usable identity — this
			// is not a case that "cannot happen" in production, all four
			// sources emit whatever id the API gave them without
			// pre-filtering. Drop the row entirely rather than passing it
			// through: an earlier version of this function rendered it as
			// unread-and-untracked, which left it visible in the pane but
			// permanently un-markable (MarkRead/MarkDone reject an empty
			// Identity.ID), so the user could press u or d on it forever
			// and only ever get an error back. Never used as a state-map
			// key either way (see NotifKey's and this function's doc
			// comments) — distinct malformed rows must never collapse into
			// one shared "" entry.
			continue
		}

		entry, seen := newState[key]

		switch {
		case !seen:
			entry = TriageEntry{
				LastActivity: row.UpdatedAt,
				LastSeen:     now,
			}
			newState[key] = entry
		case row.UpdatedAt.IsZero():
			// No activity information for this poll: skip the stamp
			// comparison, keep the stored Read/Done/LastActivity as-is,
			// and only refresh LastSeen (see doc comment above).
			entry.LastSeen = now
			newState[key] = entry
		case row.UpdatedAt.After(entry.LastActivity):
			entry.Read = false
			entry.Done = false
			entry.LastActivity = row.UpdatedAt
			entry.LastSeen = now
			newState[key] = entry
		case row.UpdatedAt.Before(entry.LastActivity):
			// Older stamp than what's stored: clock skew or a reordered
			// poll. Triage state stays untouched, but LastSeen still
			// advances (see doc comment above).
			entry.LastSeen = now
			newState[key] = entry
		default:
			// Unchanged stamp: stored Read/Done apply as-is, but LastSeen
			// still advances (see doc comment above).
			entry.LastSeen = now
			newState[key] = entry
		}

		if entry.Done {
			continue
		}
		row.Read = entry.Read
		row.Done = entry.Done
		result = append(result, row)
	}

	for k, v := range newState {
		if now.Sub(v.LastSeen) > orphanTTL {
			delete(newState, k)
		}
	}

	return result, newState
}
