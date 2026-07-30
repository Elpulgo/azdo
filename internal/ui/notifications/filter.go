// Package notifications implements the notifications-tab UI: the merged
// inbox pane (task 11) and its config-driven filter (task 10).
package notifications

import (
	"path"
	"strings"

	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/provider"
)

// FilterNotifications applies the notifications config's filter knobs to
// rows, per decisions 49-52 of 20260729-notif-p1-github.md.
//
// Selection is an override, not an intersection (decision 50):
//   - If cfg.Notifications.OnlyConfiguredRepos is true, keep only rows whose
//     scope is one of cfg.GitHub.Repos, and include_repos is ignored
//     entirely (a load-time warning already told the user this).
//   - Otherwise, if include_repos is non-empty, keep only rows matching at
//     least one of its globs.
//   - Otherwise keep everything.
//
// Subtraction is applied next, in this fixed order (decision 50):
// exclude_repos -> exclude_reasons -> unread_only. Each stage only removes
// rows, so the order cannot change the result of any single knob, but it is
// still pinned so a table test with one fixture per stage stays meaningful.
//
// It deliberately does not read participating_only, since_days, or
// max_items (decision 52): participating_only and since_days are fetch-time
// (provider.NotifOpts) knobs, and max_items is applied by the composite
// after the merge sort. Re-applying any of them here would double-filter.
//
// The result is always a newly allocated slice -- never rows[:0] -- and no
// element of rows is mutated. Input order is preserved; decision 45's total
// order is established upstream in the merge and this function must not
// fight it. This matters because the pane keeps the unfiltered feed to
// re-apply task 11's interactive `f` reason filter without refetching; an
// in-place filter would corrupt that feed the first time a row was dropped.
//
// A nil cfg returns rows unchanged (treated as "no filtering configured").
// A nil or empty rows returns nil. Neither case panics.
func FilterNotifications(rows []provider.Notification, cfg *config.Config) []provider.Notification {
	if cfg == nil {
		return rows
	}
	if len(rows) == 0 {
		return nil
	}

	nc := cfg.Notifications

	// --- Selection (override, not intersection; decision 50) ---
	out := make([]provider.Notification, 0, len(rows))
	switch {
	case nc.OnlyConfiguredRepos:
		configured := make(map[string]bool, len(cfg.GitHub.Repos))
		for _, r := range cfg.GitHub.Repos {
			configured[strings.ToLower(r)] = true
		}
		for _, row := range rows {
			if configured[strings.ToLower(row.Identity.Scope)] {
				out = append(out, row)
			}
		}
	case len(nc.IncludeRepos) > 0:
		for _, row := range rows {
			if matchesAnyGlob(nc.IncludeRepos, row.Identity.Scope) {
				out = append(out, row)
			}
		}
	default:
		out = append(out, rows...)
	}

	// --- Subtraction: exclude_repos -> exclude_reasons -> unread_only ---
	if len(nc.ExcludeRepos) > 0 {
		out = dropWhere(out, func(row provider.Notification) bool {
			return matchesAnyGlob(nc.ExcludeRepos, row.Identity.Scope)
		})
	}

	if len(nc.ExcludeReasons) > 0 {
		excluded := make(map[provider.NotificationReason]bool, len(nc.ExcludeReasons))
		for _, raw := range nc.ExcludeReasons {
			// Task 9 guarantees every surviving entry parses (decision 26's
			// load-time sanitizer already dropped anything that doesn't),
			// but the bool is still honoured here as defence in depth: an
			// unrecognised entry must be skipped, never let degrade to
			// Other and silently trim rows nobody asked to hide.
			reason, ok := provider.ParseNotificationReason(raw)
			if !ok {
				continue
			}
			excluded[reason] = true
		}
		if len(excluded) > 0 {
			out = dropWhere(out, func(row provider.Notification) bool {
				return excluded[row.Reason]
			})
		}
	}

	if nc.UnreadOnly {
		out = dropWhere(out, func(row provider.Notification) bool {
			return row.Read
		})
	}

	return out
}

// matchesAnyGlob reports whether scope matches at least one pattern in
// patterns, using path.Match semantics with both sides lower-cased first
// (decision 51). path is used rather than filepath because filepath's
// separator is OS-dependent and repo scopes always use "/".
//
// A pattern path.Match rejects with ErrBadPattern is treated as no match --
// never as match-all. This should not occur in practice (decision 51 drops
// such patterns at load time) but is handled defensively: a malformed
// pattern matching everything would empty the feed on include_repos or wipe
// it on exclude_repos, which is worse than the pattern being inert.
func matchesAnyGlob(patterns []string, scope string) bool {
	lowerScope := strings.ToLower(scope)
	for _, p := range patterns {
		ok, err := path.Match(strings.ToLower(p), lowerScope)
		if err != nil {
			continue
		}
		if ok {
			return true
		}
	}
	return false
}

// dropWhere returns a new slice containing every row in rows for which drop
// returns false. Always allocates a fresh backing array -- callers must
// never reuse rows' own backing array (rows[:0]), since the caller's
// original, unfiltered slice must remain untouched (decision 52).
func dropWhere(rows []provider.Notification, drop func(provider.Notification) bool) []provider.Notification {
	out := make([]provider.Notification, 0, len(rows))
	for _, row := range rows {
		if drop(row) {
			continue
		}
		out = append(out, row)
	}
	return out
}
