// Package notifications implements the notifications-tab UI: the merged
// inbox pane and its config-driven filter.
package notifications

import (
	"path"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/provider"
)

// FilterNotifications applies the notifications config's filter knobs to rows.
//
// Selection is an override, not an intersection:
//   - If cfg.Notifications.GitHub.OnlyConfiguredRepos is true, keep only
//     GitHub rows (Identity.Kind == provider.KindGitHub) whose scope is one
//     of cfg.GitHub.Repos. include_repos is then ignored for every row, not
//     just the GitHub ones — this is a switch, so taking this branch skips
//     the include_repos branch altogether (a load-time warning already told
//     the user this). This knob is a
//     GitHub-only concept (decision 13 of the phase-2 spec) — it never has
//     an opinion about a row from any other backend, so a row whose Kind is
//     not GitHub always survives this branch regardless of its Scope.
//   - Otherwise, if include_repos holds at least one compilable glob, narrow
//     per backend: a backend is filtered only when at least one pattern
//     actually addresses it (see includeSelection), and rows from a backend
//     no pattern addressed pass through untouched. Rows from a narrowed
//     backend must match at least one of the globs, tested against every form
//     of their scope (see scopeCandidates).
//   - Otherwise keep everything. That includes an include_repos list whose
//     every entry is uncompilable, which mirrors what the load-time
//     sanitizer would have handed us (see compilableGlobs).
//
// Subtraction is applied next: exclude_repos, exclude_reasons, unread_only.
// All five knobs are independent per-row predicates, so the stages commute --
// every permutation is observationally identical. The sequence below is
// therefore presentational: from most to least specific, so the code reads
// like the documented precedence chain. Nothing depends on it and no test can
// pin it.
//
// It deliberately does not read participating_only, since_days, or max_items:
// participating_only and since_days are fetch-time (provider.NotifOpts) knobs,
// and max_items is applied by the composite after the merge sort. Re-applying
// any of them here would double-filter.
//
// Every return path allocates a fresh slice -- never rows itself, never
// rows[:0] -- and no element of rows is mutated. That deliberately includes
// the nil-cfg path and the zero-knob path, so the contract is unconditional.
// Input order is preserved. This matters because the pane keeps the unfiltered
// feed to re-apply its interactive `f` reason filter without refetching; an
// aliasing or in-place filter would corrupt that feed the first time a row was
// dropped.
//
// A nil or empty rows returns nil. A nil cfg means "no filtering configured"
// and returns a copy of every row. Neither case panics.
func FilterNotifications(rows []provider.Notification, cfg *config.Config) []provider.Notification {
	if len(rows) == 0 {
		return nil
	}

	out := make([]provider.Notification, 0, len(rows))
	if cfg == nil {
		return append(out, rows...)
	}

	nc := cfg.Notifications

	// --- Selection (override, not intersection) ---
	includeRepos := compilableGlobs(nc.IncludeRepos)
	switch {
	case nc.GitHub.OnlyConfiguredRepos:
		configured := make(map[string]bool, len(cfg.GitHub.Repos))
		for _, r := range cfg.GitHub.Repos {
			// TrimSpace because Validate only rejects an entry that is
			// empty or carries extra slashes: "acme/repo " loads clean,
			// and an untrimmed key would match no row at all and silently
			// empty the feed -- the same failure the glob path avoids by
			// trimming there.
			configured[strings.ToLower(strings.TrimSpace(r))] = true
		}
		for _, row := range rows {
			// only_configured_repos is a GitHub-only knob (decision 13):
			// it narrows the GitHub portion of the merged feed and has no
			// opinion about any other backend's rows. A non-GitHub row
			// never carries an "owner/repo"-shaped Scope the github.repos
			// list could match, so testing Scope against it would always
			// fail and silently delete that backend's entire share of the
			// feed -- which is what this branch did before this fix, for
			// every Kind other than GitHub. Passing every non-GitHub row
			// through unconditionally is future-proof: a third backend
			// added later is unaffected by this knob without this branch
			// needing to learn its Kind by name.
			if row.Identity.Kind != provider.KindGitHub {
				out = append(out, row)
				continue
			}
			if configured[strings.ToLower(row.Identity.Scope)] {
				out = append(out, row)
			}
		}
	case len(includeRepos) > 0:
		narrowAzure, narrowOther := includeSelection(includeRepos, cfg)
		for _, row := range rows {
			narrowed := narrowOther
			if row.Identity.Kind == provider.KindAzure {
				narrowed = narrowAzure
			}
			if !narrowed || matchesAnyScope(includeRepos, scopeCandidates(row, cfg.Organization)) {
				out = append(out, row)
			}
		}
	default:
		out = append(out, rows...)
	}

	// --- Subtraction: exclude_repos -> exclude_reasons -> unread_only ---
	if len(nc.ExcludeRepos) > 0 {
		// No per-backend narrowing rule here, unlike include_repos: a
		// subtractive list only ever removes, so a pattern that addresses
		// neither backend is already an inert no-op rather than something that
		// empties a feed. It gets the same org-qualified candidates so that
		// one spelling ("myorg/project1") means the same thing in both keys.
		out = dropWhere(out, func(row provider.Notification) bool {
			return matchesAnyScope(nc.ExcludeRepos, scopeCandidates(row, cfg.Organization))
		})
	}

	if len(nc.ExcludeReasons) > 0 {
		excluded := make(map[provider.NotificationReason]bool, len(nc.ExcludeReasons))
		for _, raw := range nc.ExcludeReasons {
			// The load-time sanitizer already dropped anything that doesn't
			// parse, but the bool is still honoured here as defence in depth:
			// an unrecognised entry must be skipped, never let degrade to
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

// NotifOptsFromConfig derives the fetch-time provider.NotifOpts from the
// notifications config's participating_only, since_days and max_items knobs.
// These three are server-side query shaping, not the post-fetch predicates
// FilterNotifications applies, so they are read here and nowhere else.
//
// A nil cfg returns the zero-value NotifOpts (no participating-only
// narrowing, no since bound, no cap) rather than panicking, matching
// FilterNotifications' own nil-cfg contract.
//
// since_days <= 0 leaves Since at its zero value (no lower bound), never a
// negative or zero-length window; only a positive count of days produces a
// Since cutoff.
func NotifOptsFromConfig(cfg *config.Config) provider.NotifOpts {
	if cfg == nil {
		return provider.NotifOpts{}
	}

	nc := cfg.Notifications
	opts := provider.NotifOpts{
		ParticipatingOnly: nc.GitHub.ParticipatingOnly,
		Max:               nc.MaxItems,
	}
	if nc.GitHub.SinceDays > 0 {
		since := time.Now().AddDate(0, 0, -nc.GitHub.SinceDays)
		// Truncate to the start of the day so repeated calls within the same
		// day (NotifOpts is derived once per fetch, not frozen at poller
		// construction) produce an identical Since value, which keeps the
		// GitHub request path (buildPath) stable instead of changing on every
		// single poll tick.
		opts.Since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
	}
	return opts
}

// matchesAnyGlob reports whether scope matches at least one pattern in
// patterns, using path.Match semantics with both sides lower-cased first.
// path is used rather than filepath because filepath's separator is
// OS-dependent and repo scopes always use "/".
//
// A pattern path.Match rejects with ErrBadPattern is skipped: it neither
// matches nor prevents the remaining patterns from matching. The two ways of
// getting that wrong are not symmetric, and neither is safe on its own:
// treating a bad pattern as match-all wipes the feed when it sits in
// exclude_repos, while treating it as no-match empties the feed when it is the
// only entry in include_repos. Skipping is the whole answer for a subtractive
// list -- an inert pattern excludes nothing, which is exactly what the
// load-time drop would have produced -- but not for a selection list, so
// callers run include_repos through compilableGlobs and fall back to "select
// everything" when nothing compilable survives. The load-time drop in
// config.sanitizeRepoGlobs and that mirror rule together are what deliver the
// fail-open guarantee, on either construction path.
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

// scopeCandidates returns every string a row's scope may be addressed by.
//
// A GitHub scope is already "owner/repo", so a glob addresses it directly. An
// Azure scope is a bare project name -- the four sources all set Scope from
// PullRequest.ProjectName or its work-item equivalent -- which lives in a
// different namespace: path.Match's "*" does not cross "/", so "elpulgo/*"
// cannot match "project1" and "*" cannot match "elpulgo/azdo". One shared
// include_repos list therefore had no spelling that addressed both backends.
// Adding the "<org>/<project>" form gives Azure a qualified name in the same
// shape GitHub already has, so "myorg/*" selects Azure projects the way
// "elpulgo/*" selects GitHub repos, while a bare project name keeps working
// for configs written before this existed.
//
// org is the config's `organization`. An empty org yields the bare scope
// alone, never a "/project" candidate that a pattern could match by accident.
func scopeCandidates(row provider.Notification, org string) []string {
	if row.Identity.Kind != provider.KindAzure || strings.TrimSpace(org) == "" {
		return []string{row.Identity.Scope}
	}
	return []string{row.Identity.Scope, strings.TrimSpace(org) + "/" + row.Identity.Scope}
}

// matchesAnyScope reports whether any of scopes matches any of patterns. It is
// matchesAnyGlob widened over the candidate spellings scopeCandidates
// produces, and inherits its case-insensitivity and its skip-on-ErrBadPattern
// rule unchanged.
func matchesAnyScope(patterns, scopes []string) bool {
	for _, scope := range scopes {
		if matchesAnyGlob(patterns, scope) {
			return true
		}
	}
	return false
}

// includeSelection decides, per backend, whether include_repos narrows it at
// all. Every row is still matched against the *whole* pattern list; only the
// "is this backend filtered in the first place" question is partitioned.
//
// include_repos is one shared list, but a pattern written for one backend must
// not silently empty the other. Before this rule, "include_repos: [elpulgo/*]"
// -- an ordinary GitHub selection -- deleted every Azure row from the merged
// feed, with nothing in the pane to say why. So a backend is narrowed only
// when at least one pattern actually addresses it, and a backend no pattern
// addressed keeps everything. That is what makes include_repos mean "a subset
// of what's configured" rather than "the only backend I remembered to name".
//
// A pattern addresses Azure when it matches a configured project, bare or
// org-qualified. That is decidable from the config alone because `projects` is
// a closed list. GitHub has no equivalent list to test against -- the GitHub
// inbox spans every repo the user watches, not just github.repos -- so the
// GitHub side is defined as the complement: a pattern matching no configured
// project is taken as addressing some other backend. Defining it the other way
// round (a pattern that matches a project addresses both) would re-create the
// original trap mirrored, since "myorg/project1" would then narrow GitHub to
// nothing.
//
// The consequence worth knowing: this fails OPEN. A mistyped Azure pattern
// ("myorg/projct1") matches no project, is read as a GitHub pattern, and
// leaves Azure unnarrowed -- too many rows rather than none. config.LoadFrom
// warns on exactly that shape (a first segment equal to the organization that
// matches no project), because failing open is quiet by nature and a stray
// extra row is a far cheaper mistake than a silently missing one.
func includeSelection(patterns []string, cfg *config.Config) (narrowAzure, narrowOther bool) {
	for _, p := range patterns {
		if globAddressesAzure(p, cfg.Organization, cfg.Projects) {
			narrowAzure = true
			continue
		}
		narrowOther = true
	}
	return narrowAzure, narrowOther
}

// globAddressesAzure reports whether pattern matches at least one configured
// Azure project, in either the bare or the "<org>/<project>" spelling.
//
// It tests cfg.Projects -- the API project names -- and not DisplayNames,
// because Identity.Scope is set from the API name too. Matching the display
// name here would classify a pattern as Azure-addressing that no row could
// ever match, narrowing Azure to nothing.
func globAddressesAzure(pattern, org string, projects []string) bool {
	for _, p := range projects {
		project := strings.TrimSpace(p)
		if project == "" {
			continue
		}
		candidates := []string{project}
		if trimmedOrg := strings.TrimSpace(org); trimmedOrg != "" {
			candidates = append(candidates, trimmedOrg+"/"+project)
		}
		if matchesAnyScope([]string{pattern}, candidates) {
			return true
		}
	}
	return false
}

// compilableGlobs returns the patterns path.Match can compile, dropping the
// ones it rejects as ErrBadPattern. It reproduces config.sanitizeRepoGlobs at
// match time so that FilterNotifications behaves the same whether its config
// came through LoadFrom -- where the sanitizer already removed those patterns
// and left a shorter (possibly empty) list -- or from a &config.Config{}
// literal, which bypasses the sanitizer entirely.
//
// The caller must treat an empty result from a non-empty input as "no
// selection configured" rather than "select nothing": that is what a list of
// only-bad patterns reduces to at load time, and it is the fail-open half of
// the guarantee.
//
// The probe scope "owner/repo" is arbitrary -- ErrBadPattern is a property of
// the pattern's syntax alone. It probes the lower-cased pattern because that
// is the exact string matchesAnyGlob will later compile; lower-casing cannot
// create or remove a syntax error, but probing a different string than the
// one used for matching could drift.
func compilableGlobs(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if _, err := path.Match(strings.ToLower(p), "owner/repo"); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

// dropWhere returns a new slice containing every row in rows for which drop
// returns false. Always allocates a fresh backing array -- callers must
// never reuse rows' own backing array (rows[:0]), since the caller's
// original, unfiltered slice must remain untouched.
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
