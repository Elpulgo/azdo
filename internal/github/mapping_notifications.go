package github

import (
	"fmt"
	"path"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapNotification maps a GitHub wire NotificationThread to a provider.Notification.
//
// Unlike the PR/work-item/pipeline mappers, Scope is NOT supplied by the
// caller: a notification's repository varies row-by-row (Decision 3 keeps
// rows from unconfigured repos in the feed, so no single MultiClient scope
// covers every row), so Scope is derived here from the wire thread itself
// (Repository.FullName, GitHub's "owner/repo"). scopeDisplay is still an
// externally-resolved parameter — only the caller (task 7's Adapter, via
// MultiClient.DisplayNameFor) knows whether that scope has a configured
// display name, and only after learning the scope from this same thread.
//
// Identity.ID is the wire thread id string verbatim (GitHub's notification
// thread ids are opaque strings on the wire, not numbers task 5 parses).
//
// Read is the negation of the wire Unread flag. Done is always false: GitHub
// exposes no "done" bit on a listed thread — marking a thread done removes it
// from the inbox via DELETE (task 6), so any thread this mapper ever sees is
// by definition not done (Decision 15).
//
// NotificationThread.LastReadAt is not read by this mapper: Read is derived
// solely from Unread above, and phase 1 keeps no local read/done state
// (Decision 15) for LastReadAt to feed into.
func MapNotification(thread NotificationThread, scopeDisplay string) provider.Notification {
	scope := thread.Repository.FullName

	// provider.Notification documents ScopeDisplay falling back to Scope "at
	// the adapter boundary" — this mapper is that boundary for notifications,
	// since it derives scope from the thread itself and no caller can
	// precompute the fallback before calling (Decision 35).
	if scopeDisplay == "" {
		scopeDisplay = scope
	}

	return provider.Notification{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           thread.ID,
		},
		Title:     thread.Subject.Title,
		Reason:    MapNotificationReason(thread.Reason),
		Read:      !thread.Unread,
		Done:      false,
		UpdatedAt: thread.UpdatedAt,
		WebURL:    NotificationWebURL(thread),
	}
}

// MapNotificationReason maps a GitHub wire notification reason string to the
// neutral provider.NotificationReason enum, exactly per Decision 18
// (spec 20260729-notif-p1-github.md). Every reason string Decision 18 lists is
// handled explicitly; "manual", "invitation", and "member_feature_requested"
// are collapsed into NotificationReasonOther deliberately (nobody triages
// them differently), and the default case — covering any reason string this
// switch does not recognise, including "" and any future GitHub reason —
// also returns NotificationReasonOther rather than dropping the row.
//
// This function must never return provider.NotificationReasonUnknown: Unknown
// exists only as that enum's zero value before this mapping runs, and this is
// the one place that populates the field (Decision 18).
func MapNotificationReason(reason string) provider.NotificationReason {
	switch reason {
	case "review_requested":
		return provider.NotificationReasonReviewRequested
	case "mention", "team_mention":
		return provider.NotificationReasonMentioned
	case "assign":
		return provider.NotificationReasonAssigned
	case "author":
		return provider.NotificationReasonAuthored
	case "comment":
		return provider.NotificationReasonCommented
	case "state_change":
		return provider.NotificationReasonStateChanged
	case "ci_activity":
		return provider.NotificationReasonCIActivity
	case "security_alert", "security_advisory_credit":
		return provider.NotificationReasonSecurityAlert
	case "approval_requested":
		return provider.NotificationReasonApprovalRequested
	case "subscribed":
		return provider.NotificationReasonSubscribed
	case "manual", "invitation", "member_feature_requested":
		return provider.NotificationReasonOther
	default:
		// Any unrecognised or future GitHub reason string, and the empty
		// string (an absent/zero-value Reason field), collapse to Other
		// rather than being dropped from the feed (Decision 18).
		return provider.NotificationReasonOther
	}
}

// NotificationWebURL resolves a GitHub notification thread's subject.url — an
// API URL such as "https://api.github.com/repos/o/r/pulls/1" — to the
// github.com (or GHE) browser URL that `o` (open in browser) opens (spec
// Decision 3; see the "Constraints" and "## Unknowns" sections of
// 20260729-notif-p1-github.md).
//
// Every per-type URL is built on the repositoryWebURL(thread.Repository)
// prefix (Decision 34) rather than defaultWebBaseURL+FullName: it honours a
// GHE host via the wire Repository.HTMLURL, the same way mapping_pr.go and
// mapping_pipeline.go already populate WebURL from a wire HTMLURL, and it
// makes an empty FullName fall back cleanly instead of ever producing
// "https://github.com//pull/42". When the repository itself carries no usable
// URL at all, the prefix — and so this function — returns "".
//
// Resolution is keyed on NotificationSubject.Type — GitHub's own
// classification of what the notification is about — not on parsing
// subject.url's path segments for a type hint; only the trailing identifier
// (issue/PR number or commit SHA) is extracted from the URL, and it is
// shape-validated before being trusted (digits for PullRequest/Issue, hex for
// Commit — Decision 33 / convention 11's spirit applied to a string id).
// Anything that fails validation — no id segment, a trailing slash, or a
// non-matching id — falls back to the repository prefix rather than emitting
// a clickable 404:
//
//	Type "PullRequest" → .../repos/{o}/{r}/pulls/{n}   → {prefix}/pull/{n}    (singular "pull"; n must be all-digit)
//	Type "Issue"       → .../repos/{o}/{r}/issues/{n}  → {prefix}/issues/{n} (unchanged; n must be all-digit)
//	Type "Commit"      → .../repos/{o}/{r}/commits/{s} → {prefix}/commit/{s} (singular "commit"; s must be hex)
//
// Type "Release" is deliberately NOT resolved to a per-item route at all
// (Decision 33): the real github.com route for a specific release needs the
// tag name (verified live: "/cli/cli/releases/348300685" → 404,
// "/cli/cli/releases/tag/v2.96.0" → 200), and NotificationSubject carries no
// tag name — only Title, URL, LatestCommentURL and Type. Every Release row
// therefore resolves to the repo's "/releases" list page
// ("/cli/cli/releases" → 200), regardless of what subject.url contains.
//
// Every other subject type — Discussion, CheckSuite,
// RepositoryVulnerabilityAlert, and any type GitHub adds later — falls back
// to the notification's repository web URL, as does any of the three
// id-bearing types above when Subject.URL is empty or its id segment does not
// validate. GitHub sends a null (empty, once decoded) subject.url for several
// subject types, notably Discussion and check-suite rows; that is exactly
// what the fallback exists for (see repositoryWebURL). The fallback is the
// correctness guarantee this function offers, not the per-type mapping above
// — the spec's "## Unknowns" section is explicit that none of these shapes
// have been confirmed against a live token in this environment (no token is
// available here).
func NotificationWebURL(thread NotificationThread) string {
	prefix := repositoryWebURL(thread.Repository)
	if prefix == "" {
		return ""
	}

	// Release has no per-item route buildable from a notification payload
	// (see doc comment above) — always the list page.
	if thread.Subject.Type == "Release" {
		return prefix + "/releases"
	}

	if id := lastPathSegment(thread.Subject.URL); id != "" {
		switch thread.Subject.Type {
		case "PullRequest":
			if isDigits(id) {
				return fmt.Sprintf("%s/pull/%s", prefix, id)
			}
		case "Issue":
			if isDigits(id) {
				return fmt.Sprintf("%s/issues/%s", prefix, id)
			}
		case "Commit":
			if isHex(id) {
				return fmt.Sprintf("%s/commit/%s", prefix, id)
			}
		}
	}
	return prefix
}

// isDigits reports whether s is non-empty and consists entirely of ASCII
// digits — the shape of a GitHub PR or issue number.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isHex reports whether s is non-empty and consists entirely of ASCII
// hexadecimal digits — the shape of a (possibly abbreviated) commit SHA.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// repositoryWebURL returns the browser URL for a notification's repository —
// the fallback destination for any subject type NotificationWebURL does not
// resolve to a specific PR/issue/release/commit, and for the empty-url case
// GitHub sends for some subject types. It prefers the wire-supplied HTMLURL;
// when that is itself empty (a partial repository payload), it constructs
// "https://github.com/{full_name}" from FullName. Returns "" only when both
// HTMLURL and FullName are empty, so a row with no repository information at
// all never renders a bare, dangling "https://github.com".
func repositoryWebURL(repo NotificationRepository) string {
	if repo.HTMLURL != "" {
		return repo.HTMLURL
	}
	if repo.FullName != "" {
		return fmt.Sprintf("%s/%s", defaultWebBaseURL, repo.FullName)
	}
	return ""
}

// lastPathSegment returns the final "/"-separated segment of rawURL —
// typically the numeric id or commit SHA at the end of a GitHub API URL — or
// "" for an empty input. It only extracts the segment; it does not validate
// its shape (whether it looks like a real id) — that is NotificationWebURL's
// job via isDigits/isHex, so a bare "pulls" (no id segment at all) or "abc"
// (a non-numeric one) still comes back from here, deliberately, for the
// caller to reject. Every subject.url shape observed in GitHub's
// documentation is a plain https:// URL with no query string, so a
// path-based split is sufficient and cannot panic on malformed input.
func lastPathSegment(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	base := path.Base(rawURL)
	// path.Base returns "." only for an empty input, which the rawURL == ""
	// guard above already handles, so that arm would be dead code here. The
	// "/" case is live and real: path.Base("/") is "/", which would otherwise
	// leak a literal slash into a constructed URL.
	if base == "/" {
		return ""
	}
	return base
}
