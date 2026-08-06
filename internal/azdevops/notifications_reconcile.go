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
// without bound (decision 7). It is deliberately generous relative to the
// default lookback so a longer-than-default lookback_days configuration
// does not prune entries a still-running source would otherwise resurrect.
const orphanTTL = 30 * 24 * time.Hour

// NotifKey builds the stable identity key for a locally-tracked synthetic
// notification subject (decision 2): "<source>/<entity>/<entity_id>". This
// is the value stored in provider.Identity.ID — it names the subject, never
// the activity that made it interesting (decision 2's rejected
// stamp-in-key alternative).
//
// id is guarded per convention 11: a non-positive id (<= 0, not just == 0)
// is rejected and yields "", since a negative id would otherwise produce a
// malformed-but-non-empty key.
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
//   - a subject with no existing entry is unread; a fresh entry is created
//     stamped with the row's UpdatedAt and LastSeen set to now.
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
//     (clock skew, or a reordered poll) is left completely untouched: the
//     stored entry — Read, Done, LastActivity, and LastSeen alike — is
//     neither cleared nor advanced. The stored Read/Done still apply to
//     the returned row.
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
func Reconcile(rows []provider.Notification, state map[string]TriageEntry, now time.Time) ([]provider.Notification, map[string]TriageEntry) {
	newState := make(map[string]TriageEntry, len(state))
	for k, v := range state {
		newState[k] = v
	}

	result := make([]provider.Notification, 0, len(rows))
	for _, row := range rows {
		key := row.Identity.ID
		entry, seen := newState[key]

		switch {
		case !seen:
			entry = TriageEntry{
				LastActivity: row.UpdatedAt,
				LastSeen:     now,
			}
			newState[key] = entry
		case row.UpdatedAt.After(entry.LastActivity):
			entry.Read = false
			entry.Done = false
			entry.LastActivity = row.UpdatedAt
			entry.LastSeen = now
			newState[key] = entry
		case row.UpdatedAt.Before(entry.LastActivity):
			// Older stamp than what's stored: clock skew or a reordered
			// poll. Left completely untouched — do not write it back.
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
