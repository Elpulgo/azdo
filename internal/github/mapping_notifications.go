package github

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapNotification maps a GitHub wire NotificationThread to a provider.Notification.
//
// Unlike the PR/work-item/pipeline mappers, Scope is NOT supplied by the
// caller: a notification's repository varies row-by-row (it can be any repo,
// configured or not, so no single MultiClient scope covers every row), so Scope
// is derived here from the wire thread itself (Repository.FullName, GitHub's
// "owner/repo"). scopeDisplay is still an externally-resolved parameter — only
// the caller (the Adapter, via MultiClient.DisplayNameFor) knows whether that
// scope has a configured display name, and only after learning the scope from
// this same thread.
//
// Identity.ID is the wire thread id string verbatim (GitHub's notification
// thread ids are opaque strings on the wire, not numbers).
//
// Read is the negation of the wire Unread flag. Done is always false: GitHub
// exposes no "done" bit on a listed thread, so this mapper has nothing to
// populate the field from. Note that "no done bit" does NOT mean a listed
// thread is never done — NotificationsClient.List fetches all=true, and that
// response keeps returning threads marked done via DELETE, indistinguishable
// from live ones (this mapper's original assumption to the contrary is what
// let every marked-done row resurrect on the next full fetch). Marked-done
// threads are instead dropped from the feed one layer up, by the Adapter's
// DoneStore.Filter (notifications_donestore.go), which tracks them locally
// because the wire cannot.
//
// NotificationThread.LastReadAt is not read by this mapper: Read is derived
// solely from Unread above, and there is currently no local read/done state
// for LastReadAt to feed into.
func MapNotification(thread NotificationThread, scopeDisplay string) provider.Notification {
	scope := thread.Repository.FullName

	// provider.Notification documents ScopeDisplay falling back to Scope "at
	// the adapter boundary" — this mapper is that boundary for notifications,
	// since it derives scope from the thread itself and no caller can
	// precompute the fallback before calling.
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
// neutral provider.NotificationReason enum. Every reason string GitHub
// documents is handled explicitly; "manual", "invitation", and
// "member_feature_requested" are collapsed into NotificationReasonOther
// deliberately (nobody triages them differently), and the default case —
// covering any reason string this switch does not recognise, including "" and
// any future GitHub reason — also returns NotificationReasonOther rather than
// dropping the row.
//
// This function must never return provider.NotificationReasonUnknown: Unknown
// exists only as that enum's zero value before this mapping runs, and this is
// the one place that populates the field.
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
		// rather than being dropped from the feed.
		return provider.NotificationReasonOther
	}
}

// NotificationWebURL resolves a GitHub notification thread's subject.url — an
// API URL such as "https://api.github.com/repos/o/r/pulls/1" — to the
// github.com (or GHE) browser URL that `o` (open in browser) opens.
//
// Every per-type URL is built on the repositoryWebURL(thread.Repository)
// prefix rather than defaultWebBaseURL+FullName: it honours a GHE host via the
// wire Repository.HTMLURL, the same way mapping_pr.go and mapping_pipeline.go
// already populate WebURL from a wire HTMLURL, and it makes an empty FullName
// fall back cleanly instead of ever producing "https://github.com//pull/42".
// When the repository itself carries no usable URL at all, the prefix — and so
// this function — returns "".
//
// Resolution is keyed on NotificationSubject.Type — GitHub's own
// classification of what the notification is about — not on parsing
// subject.url's path segments for a type hint; only the trailing identifier
// (issue/PR number or commit SHA) is extracted from the URL, and it is
// shape-validated before being trusted (a plausible item number for
// PullRequest/Issue, hex for Commit — convention 11 applied to a string id).
// Anything that fails validation — no id segment, a trailing slash, a
// non-matching id, or a number GitHub can never have issued ("0", "007") —
// falls back to the repository prefix rather than emitting a clickable 404:
//
//	Type "PullRequest" → .../repos/{o}/{r}/pulls/{n}   → {prefix}/pull/{n}    (singular "pull"; n must satisfy isItemNumber)
//	Type "Issue"       → .../repos/{o}/{r}/issues/{n}  → {prefix}/issues/{n} (unchanged; n must satisfy isItemNumber)
//	Type "Commit"      → .../repos/{o}/{r}/commits/{s} → {prefix}/commit/{s} (singular "commit"; s must be hex)
//
// Type "Release" is deliberately NOT resolved to a per-item route at all: the
// real github.com route for a specific release needs the tag name (verified
// live: "/cli/cli/releases/348300685" → 404, "/cli/cli/releases/tag/v2.96.0" →
// 200), and NotificationSubject carries no tag name — only Title, URL,
// LatestCommentURL and Type. Every Release row therefore resolves to the repo's
// "/releases" list page ("/cli/cli/releases" → 200), regardless of what
// subject.url contains.
//
// Every other subject type — Discussion, CheckSuite,
// RepositoryVulnerabilityAlert, and any type GitHub adds later — falls back
// to the notification's repository web URL, as does any of the three
// id-bearing types above when Subject.URL is empty or its id segment does not
// validate. GitHub sends a null (empty, once decoded) subject.url for several
// subject types, notably Discussion and check-suite rows; that is exactly
// what the fallback exists for (see repositoryWebURL). The fallback is the
// correctness guarantee this function offers, not the per-type mapping above
// — none of these shapes have been confirmed against a live token.
func NotificationWebURL(thread NotificationThread) string {
	prefix := repositoryWebURL(thread.Repository)
	if prefix == "" {
		// Load-bearing: without this every branch below would still run and
		// return a relative path ("/releases", "/pull/42"). A bare path handed
		// to the OS opener is strictly worse than "", which the caller can at
		// least detect.
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
			if isItemNumber(id) {
				return fmt.Sprintf("%s/pull/%s", prefix, id)
			}
		case "Issue":
			if isItemNumber(id) {
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

// isItemNumber reports whether s has the shape of a GitHub PR or issue number:
// non-empty, all ASCII digits, no leading "0", and > 0 once parsed
// (convention 11's "guard positive identifiers with <= 0, never == 0"). The
// same rule guards thread ids elsewhere in this package, so the two guards
// stay consistent.
//
// Digits-only alone is not enough: ".../pulls/0" would render
// "github.com/o/r/pull/0" and ".../pulls/007" would render ".../pull/007",
// both dead pages — the exact class of clickable 404 this guard exists to
// eliminate, since GitHub PR and issue numbers start at 1.
//
// A digit string too long for an int (e.g. a 26-digit id) fails the parse with
// a range error and is rejected here, rather than wrapping to a negative or
// panicking downstream.
//
// isHex is deliberately exempt from both the leading-zero and the magnitude
// rule: a commit SHA legitimately begins with "0" and has no ordering.
func isItemNumber(s string) bool {
	if s == "" {
		return false
	}
	// Rejects "0" itself and every leading-zero form ("007").
	if s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		// Out of int range: too long to be a real item number.
		return false
	}
	// Unreachable today: the leading-zero check above already rejects "0",
	// and the digit-only loop already rejects '-', so every s that reaches
	// this line is a non-empty run of '1'-'9'/'0' digits not starting with
	// '0' — which parses to n >= 1 unconditionally. This line is kept anyway
	// as defence-in-depth: it is the actual carrier of convention 11's "<= 0"
	// guarantee in this function's contract, so if a future change relaxes
	// either guard above (e.g. permits a leading "0"), this still stops a
	// non-positive id from silently being accepted rather than depending on
	// whichever guard happens to get relaxed to also remember the rule.
	return n > 0
}

// isHex reports whether s is non-empty and consists entirely of ASCII
// hexadecimal digits — the shape of a (possibly abbreviated) commit SHA.
// Unlike isItemNumber it applies no leading-zero or magnitude rule (see
// isItemNumber's note).
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
//
// A trailing "/" is trimmed off the wire HTMLURL because every caller appends
// its own "/..." suffix: GitHub's own html_url never carries one, but the
// prefix is wire-controlled, and without the trim an "https://github.com/o/r/"
// payload would emit "https://github.com/o/r//pull/42".
func repositoryWebURL(repo NotificationRepository) string {
	if prefix := strings.TrimSuffix(repo.HTMLURL, "/"); prefix != "" {
		return prefix
	}
	if repo.FullName != "" {
		return fmt.Sprintf("%s/%s", defaultWebBaseURL, repo.FullName)
	}
	return ""
}

// lastPathSegment returns the final "/"-separated segment of rawURL —
// typically the numeric id or commit SHA at the end of a GitHub API URL — or
// "" for an empty input. It only extracts the segment; it does NOT validate
// its shape (whether it looks like a real id) — that is NotificationWebURL's
// job via isItemNumber/isHex, so a bare "pulls" (no id segment at all) or
// "abc" (a non-numeric one) still comes back from here, deliberately, for the
// caller to reject. Every subject.url shape observed in GitHub's
// documentation is a plain https:// URL with no query string, so a
// path-based split is sufficient and cannot panic on malformed input.
//
// The invariant that keeps this safe is on the caller's side:
// NotificationWebURL never interpolates a segment it has not shape-validated.
// Do NOT read the two guards below as sanitisation — they are not. path.Base
// can return "." from non-empty input (path.Base(".") and path.Base("./") are
// both "."), and it can return any other junk a malformed URL ends with. Those
// values are harmless only because every consumer rejects them. Anyone adding
// a fifth subject type must add its own shape check too.
func lastPathSegment(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	base := path.Base(rawURL)
	// path.Base("/") is "/" — returning it would leak a literal slash into a
	// constructed URL if a future caller ever skipped validation. Redundant
	// today (all three consumers reject "/"), kept as a cheap belt.
	if base == "/" {
		return ""
	}
	return base
}
