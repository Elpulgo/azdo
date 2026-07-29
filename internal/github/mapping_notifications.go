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
func MapNotification(thread NotificationThread, scopeDisplay string) provider.Notification {
	scope := thread.Repository.FullName

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
// github.com browser URL that `o` (open in browser) opens (spec Decision 3;
// see the "Constraints" and "## Unknowns" sections of
// 20260729-notif-p1-github.md).
//
// Resolution is keyed on NotificationSubject.Type — GitHub's own
// classification of what the notification is about — not on parsing
// subject.url's path segments for a type hint; only the trailing identifier
// (issue/PR number, commit SHA, or release id) is extracted from the URL:
//
//	Type "PullRequest" → .../repos/{o}/{r}/pulls/{n}   → https://github.com/{o}/{r}/pull/{n}    (singular "pull")
//	Type "Issue"       → .../repos/{o}/{r}/issues/{n}  → https://github.com/{o}/{r}/issues/{n}  (unchanged)
//	Type "Commit"      → .../repos/{o}/{r}/commits/{s} → https://github.com/{o}/{r}/commit/{s}  (singular "commit")
//	Type "Release"     → .../repos/{o}/{r}/releases/{n} → https://github.com/{o}/{r}/releases/{n}
//
// Every other subject type — Discussion, CheckSuite,
// RepositoryVulnerabilityAlert, and any type GitHub adds later — falls back
// to the notification's repository web URL, as does any of the four types
// above when Subject.URL is empty. GitHub sends a null (empty, once decoded)
// subject.url for several subject types, notably Discussion and check-suite
// rows; that is exactly what the fallback exists for (see repositoryWebURL).
// The fallback is the correctness guarantee this function offers, not the
// per-type mapping above — the spec's "## Unknowns" section is explicit that
// none of these shapes have been confirmed against a live token in this
// environment (no token is available here).
//
// Decision on GitHub Enterprise / SetBaseURL: this function deliberately does
// NOT honour NotificationsClient.baseURL (or Client.baseURL) and always
// builds against defaultWebBaseURL ("https://github.com"), matching the
// existing precedent in weburl.go (WorkItemURL/PRURL/PipelineURL), whose doc
// comment already states that a GHE web host is a future config concern, not
// handled yet. This mapper is a pure function with no client receiver at all
// (a notification's repo varies row-by-row, unlike the per-repo Client
// methods in weburl.go), so there is no natural place to thread a host
// override through without adding a parameter every call site would have to
// thread for a scenario (GHE) that the rest of the package does not support
// today either. Widening only this one mapper would be inconsistent with its
// siblings and is not worth the complexity for a case with no test coverage
// possible in this environment.
func NotificationWebURL(thread NotificationThread) string {
	if id := lastPathSegment(thread.Subject.URL); id != "" {
		switch thread.Subject.Type {
		case "PullRequest":
			return fmt.Sprintf("%s/%s/pull/%s", defaultWebBaseURL, thread.Repository.FullName, id)
		case "Issue":
			return fmt.Sprintf("%s/%s/issues/%s", defaultWebBaseURL, thread.Repository.FullName, id)
		case "Commit":
			return fmt.Sprintf("%s/%s/commit/%s", defaultWebBaseURL, thread.Repository.FullName, id)
		case "Release":
			return fmt.Sprintf("%s/%s/releases/%s", defaultWebBaseURL, thread.Repository.FullName, id)
		}
	}
	return repositoryWebURL(thread.Repository)
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
// "" for an empty input. path.Base("") is "." (not ""), so that case is
// special-cased to "" rather than leaking a literal dot into a constructed
// URL; every subject.url shape observed in GitHub's documentation is a plain
// https:// URL with no query string, so a path-based split is sufficient and
// cannot panic on malformed input.
func lastPathSegment(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	base := path.Base(rawURL)
	if base == "." || base == "/" {
		return ""
	}
	return base
}
