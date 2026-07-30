package github_test

import (
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// ── MapNotificationReason: every Decision-18 wire string, plus unknowns ─────

func TestMapNotificationReason_Decision18Table(t *testing.T) {
	cases := []struct {
		wire string
		want provider.NotificationReason
	}{
		{"review_requested", provider.NotificationReasonReviewRequested},
		{"mention", provider.NotificationReasonMentioned},
		{"team_mention", provider.NotificationReasonMentioned},
		{"assign", provider.NotificationReasonAssigned},
		{"author", provider.NotificationReasonAuthored},
		{"comment", provider.NotificationReasonCommented},
		{"state_change", provider.NotificationReasonStateChanged},
		{"ci_activity", provider.NotificationReasonCIActivity},
		{"security_alert", provider.NotificationReasonSecurityAlert},
		{"security_advisory_credit", provider.NotificationReasonSecurityAlert},
		{"approval_requested", provider.NotificationReasonApprovalRequested},
		{"subscribed", provider.NotificationReasonSubscribed},
		{"manual", provider.NotificationReasonOther},
		{"invitation", provider.NotificationReasonOther},
		{"member_feature_requested", provider.NotificationReasonOther},
		// Invented, never-issued-by-GitHub strings. "unknown" is called out
		// separately from the other bogus values because it is the one input
		// most likely to tempt an implementation into echoing the reserved
		// NotificationReasonUnknown back out — Decision 18 forbids that, and
		// the per-case check below is what pins it.
		{"some_future_reason_nobody_has_seen_yet", provider.NotificationReasonOther},
		{"unknown", provider.NotificationReasonOther},
		{"totally_bogus", provider.NotificationReasonOther},
		// Absent/zero-value reason.
		{"", provider.NotificationReasonOther},
	}

	for _, tc := range cases {
		t.Run(tc.wire, func(t *testing.T) {
			got := github.MapNotificationReason(tc.wire)
			if got != tc.want {
				t.Errorf("MapNotificationReason(%q) = %v, want %v", tc.wire, got, tc.want)
			}
			// Decision 18: the mapper must never emit Unknown.
			if got == provider.NotificationReasonUnknown {
				t.Errorf("MapNotificationReason(%q) = Unknown; the wire mapper must never emit Unknown (Decision 18)", tc.wire)
			}
		})
	}
}

// ── MapNotification: Identity population ────────────────────────────────────

func TestMapNotification_IdentityFullyPopulated(t *testing.T) {
	thread := github.NotificationThread{
		ID:        "12345",
		Unread:    true,
		Reason:    "review_requested",
		UpdatedAt: time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC),
		Subject: github.NotificationSubject{
			Title: "Fix the thing",
			URL:   "https://api.github.com/repos/octo/repo/pulls/42",
			Type:  "PullRequest",
		},
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
			HTMLURL:  "https://github.com/octo/repo",
		},
	}

	got := github.MapNotification(thread, "Octo Repo")

	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Identity.Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Identity.Scope != "octo/repo" {
		t.Errorf("Identity.Scope = %q, want %q", got.Identity.Scope, "octo/repo")
	}
	if got.Identity.ScopeDisplay != "Octo Repo" {
		t.Errorf("Identity.ScopeDisplay = %q, want %q", got.Identity.ScopeDisplay, "Octo Repo")
	}
	if got.Identity.ID != "12345" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "12345")
	}

	if got.Title != "Fix the thing" {
		t.Errorf("Title = %q, want %q", got.Title, "Fix the thing")
	}
	if got.Reason != provider.NotificationReasonReviewRequested {
		t.Errorf("Reason = %v, want NotificationReasonReviewRequested", got.Reason)
	}
	if got.Read {
		t.Errorf("Read = true, want false (Unread was true)")
	}
	if got.Done {
		t.Errorf("Done = true, want false — GitHub never surfaces a done bit on a listed thread")
	}
	wantUpdated := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	if !got.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, wantUpdated)
	}
	if got.WebURL != "https://github.com/octo/repo/pull/42" {
		t.Errorf("WebURL = %q, want %q", got.WebURL, "https://github.com/octo/repo/pull/42")
	}
}

func TestMapNotification_UnreadFalseWhenThreadRead(t *testing.T) {
	thread := github.NotificationThread{
		ID:     "1",
		Unread: false,
		Reason: "subscribed",
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
		},
	}
	got := github.MapNotification(thread, "octo/repo")
	if !got.Read {
		t.Errorf("Read = false, want true when Unread == false")
	}
}

// TestMapNotification_NearlyEmptyThread_NoPanic drives a fully zero-value wire
// thread through the mapper and pins what that must produce: Kind GitHub, an
// empty wire reason mapped to Other (never Unknown), WebURL "" (no subject url
// and no repository url to build one from) and ScopeDisplay "" (Scope is empty
// too, so the Decision 35 fallback has nothing to default to). The no-panic
// criterion holds by construction — the mapper reads no pointer field, in
// particular not LastReadAt — so the recover below is a tripwire, not the
// thing this test pins.
func TestMapNotification_NearlyEmptyThread_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MapNotification panicked on a nearly-empty thread: %v", r)
		}
	}()

	var thread github.NotificationThread // zero value; LastReadAt is nil

	got := github.MapNotification(thread, "")

	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Identity.Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Reason == provider.NotificationReasonUnknown {
		t.Errorf("Reason = Unknown; empty wire reason must map to Other, never Unknown")
	}
	if got.Reason != provider.NotificationReasonOther {
		t.Errorf("Reason = %v, want NotificationReasonOther for an empty wire reason", got.Reason)
	}
	if got.WebURL != "" {
		t.Errorf("WebURL = %q, want empty — no subject url and no repository url present", got.WebURL)
	}
	if got.Identity.ScopeDisplay != "" {
		t.Errorf("Identity.ScopeDisplay = %q, want empty — Scope is also empty on a zero-value thread, so the Decision 35 fallback has nothing to default to", got.Identity.ScopeDisplay)
	}
}

// TestMapNotification_ScopeDisplayDefaultsToScope pins Decision 35: the
// mapper is the adapter boundary for notifications (it derives scope from
// the thread itself, so no caller can precompute the fallback before
// calling), so an empty scopeDisplay argument must default to Scope here,
// not stay blank and leave the repo column empty in every list view.
func TestMapNotification_ScopeDisplayDefaultsToScope(t *testing.T) {
	thread := github.NotificationThread{
		ID:     "1",
		Reason: "subscribed",
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
		},
	}

	got := github.MapNotification(thread, "")

	if got.Identity.ScopeDisplay != "octo/repo" {
		t.Errorf("Identity.ScopeDisplay = %q, want %q (falls back to Scope per Decision 35)", got.Identity.ScopeDisplay, "octo/repo")
	}
}

// ── NotificationWebURL: subject.url resolution per type ─────────────────────

func TestNotificationWebURL_ResolvesPerSubjectType(t *testing.T) {
	repo := github.NotificationRepository{
		FullName: "octo/repo",
		HTMLURL:  "https://github.com/octo/repo",
	}

	cases := []struct {
		name    string
		subject github.NotificationSubject
		want    string
	}{
		{
			name: "pull request",
			subject: github.NotificationSubject{
				Type: "PullRequest",
				URL:  "https://api.github.com/repos/octo/repo/pulls/42",
			},
			want: "https://github.com/octo/repo/pull/42",
		},
		{
			name: "issue",
			subject: github.NotificationSubject{
				Type: "Issue",
				URL:  "https://api.github.com/repos/octo/repo/issues/7",
			},
			want: "https://github.com/octo/repo/issues/7",
		},
		{
			// Decision 33, verified against live github.com:
			// "/cli/cli/releases/348300685" → 404 (per-release-id is not a
			// real route; the real one needs a tag name the wire payload
			// does not carry), "/cli/cli/releases" → 200. So every Release
			// row resolves to the repo's /releases list page, never a
			// per-id route, regardless of what subject.url's trailing
			// segment happens to be.
			name: "release",
			subject: github.NotificationSubject{
				Type: "Release",
				URL:  "https://api.github.com/repos/octo/repo/releases/99",
			},
			want: "https://github.com/octo/repo/releases",
		},
		{
			name: "commit",
			subject: github.NotificationSubject{
				Type: "Commit",
				URL:  "https://api.github.com/repos/octo/repo/commits/abc123def456",
			},
			want: "https://github.com/octo/repo/commit/abc123def456",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thread := github.NotificationThread{Subject: tc.subject, Repository: repo}
			got := github.NotificationWebURL(thread)
			if got != tc.want {
				t.Errorf("NotificationWebURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNotificationWebURL_AcceptsValidIDShapes is the positive counterpart to
// TestNotificationWebURL_FallsBackWhenIDSegmentInvalid: Decision 36 tightened
// the numeric guard, so pin that the shapes GitHub really issues still resolve
// to a per-item URL — the smallest legal item number, an ordinary one, a full
// 40-char SHA, an abbreviated 7-char one, and hex in upper and mixed case
// (isHex is case-insensitive and, unlike the numeric guard, allows a leading
// zero).
func TestNotificationWebURL_AcceptsValidIDShapes(t *testing.T) {
	repo := github.NotificationRepository{
		FullName: "octo/repo",
		HTMLURL:  "https://github.com/octo/repo",
	}

	cases := []struct {
		name    string
		subject github.NotificationSubject
		want    string
	}{
		{
			name:    "smallest legal pr number",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/octo/repo/pulls/1"},
			want:    "https://github.com/octo/repo/pull/1",
		},
		{
			name:    "ordinary pr number",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/octo/repo/pulls/42"},
			want:    "https://github.com/octo/repo/pull/42",
		},
		{
			name:    "smallest legal issue number",
			subject: github.NotificationSubject{Type: "Issue", URL: "https://api.github.com/repos/octo/repo/issues/1"},
			want:    "https://github.com/octo/repo/issues/1",
		},
		{
			name:    "full 40-char sha",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://api.github.com/repos/octo/repo/commits/0a1b2c3d4e5f60718293a4b5c6d7e8f901234567"},
			want:    "https://github.com/octo/repo/commit/0a1b2c3d4e5f60718293a4b5c6d7e8f901234567",
		},
		{
			name:    "abbreviated 7-char sha",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://api.github.com/repos/octo/repo/commits/0abc123"},
			want:    "https://github.com/octo/repo/commit/0abc123",
		},
		{
			name:    "uppercase hex sha",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://api.github.com/repos/octo/repo/commits/ABCDEF1"},
			want:    "https://github.com/octo/repo/commit/ABCDEF1",
		},
		{
			name:    "mixed-case hex sha",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://api.github.com/repos/octo/repo/commits/AbC123dEf"},
			want:    "https://github.com/octo/repo/commit/AbC123dEf",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thread := github.NotificationThread{Subject: tc.subject, Repository: repo}
			got := github.NotificationWebURL(thread)
			if got != tc.want {
				t.Errorf("NotificationWebURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── NotificationWebURL: fallback to the repository URL ──────────────────────

func TestNotificationWebURL_FallsBackForUnrecognisedSubjectType(t *testing.T) {
	thread := github.NotificationThread{
		Subject: github.NotificationSubject{
			Type: "Discussion",
			URL:  "https://api.github.com/repos/octo/repo/discussions/3",
		},
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
			HTMLURL:  "https://github.com/octo/repo",
		},
	}

	got := github.NotificationWebURL(thread)
	want := "https://github.com/octo/repo"
	if got != want {
		t.Errorf("NotificationWebURL() = %q, want %q (repo fallback)", got, want)
	}
}

// TestNotificationWebURL_FallsBackWhenIDSegmentInvalid covers every measured
// 404-producing id shape: an absent subject.url, no id segment at all, a
// trailing-slash URL, a non-URL string, a non-numeric id where a number is
// required, the implausible numbers Decision 36 rejects ("0", "007", a
// 26-digit id), and — on the Commit side, where the guard is isHex rather
// than isItemNumber — a missing and a non-hex segment. Each must fall back to
// the repository URL rather than emit a clickable 404.
func TestNotificationWebURL_FallsBackWhenIDSegmentInvalid(t *testing.T) {
	repo := github.NotificationRepository{
		FullName: "octo/repo",
		HTMLURL:  "https://github.com/octo/repo",
	}
	want := "https://github.com/octo/repo"

	cases := []struct {
		name    string
		subject github.NotificationSubject
	}{
		{
			// GitHub sends a null subject.url for some subject types (e.g.
			// check-suite rows). Type is still "PullRequest"-shaped here to
			// prove the empty URL, not the type, triggers the fallback.
			name:    "empty subject url",
			subject: github.NotificationSubject{Type: "PullRequest", URL: ""},
		},
		{
			name:    "no id segment at all",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/o/r/pulls"},
		},
		{
			name:    "trailing slash",
			subject: github.NotificationSubject{Type: "Issue", URL: "https://api.github.com/repos/o/r/issues/"},
		},
		{
			name:    "not a url",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "notaurl"},
		},
		{
			name:    "non-numeric id",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/o/r/pulls/abc"},
		},
		{
			// Decision 36: GitHub PR/issue numbers start at 1, so "0" is not a
			// real item — "/pull/0" is a measured dead page.
			name:    "zero pr number",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/o/r/pulls/0"},
		},
		{
			name:    "zero issue number",
			subject: github.NotificationSubject{Type: "Issue", URL: "https://api.github.com/repos/o/r/issues/0"},
		},
		{
			// Leading zeros: "/pull/007" is likewise a dead page, and GitHub
			// never issues a zero-padded number.
			name:    "leading-zero pr number",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/o/r/pulls/007"},
		},
		{
			name:    "leading-zero issue number",
			subject: github.NotificationSubject{Type: "Issue", URL: "https://api.github.com/repos/o/r/issues/007"},
		},
		{
			// 26 digits — far past int64. Must be rejected via the parse's
			// range error, not wrapped to a negative and not a panic.
			name:    "absurdly long numeric id",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://api.github.com/repos/o/r/pulls/12345678901234567890123456"},
		},
		{
			// Commit's guard is isHex, not isItemNumber: prove the hex
			// rejection direction, not just the digit one.
			name:    "commit with no sha segment",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://api.github.com/repos/o/r/commits"},
		},
		{
			name:    "commit with non-hex sha",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://api.github.com/repos/o/r/commits/zzz"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thread := github.NotificationThread{Subject: tc.subject, Repository: repo}
			got := github.NotificationWebURL(thread)
			if got != want {
				t.Errorf("NotificationWebURL() = %q, want %q (repo fallback on unvalidated id)", got, want)
			}
		})
	}
}

// TestNotificationWebURL_EmptyFullNameUsesHTMLURL pins the fix for
// "https://github.com//pull/42": with FullName empty and HTMLURL present, the
// per-type URL must be built on HTMLURL, never on the empty FullName.
func TestNotificationWebURL_EmptyFullNameUsesHTMLURL(t *testing.T) {
	thread := github.NotificationThread{
		Subject: github.NotificationSubject{
			Type: "PullRequest",
			URL:  "https://api.github.com/repos/octo/repo/pulls/42",
		},
		Repository: github.NotificationRepository{
			FullName: "",
			HTMLURL:  "https://github.com/octo/repo",
		},
	}

	got := github.NotificationWebURL(thread)
	want := "https://github.com/octo/repo/pull/42"
	if got != want {
		t.Errorf("NotificationWebURL() = %q, want %q", got, want)
	}
}

// TestNotificationWebURL_HonoursGHEHost pins Decision 34: every per-type URL
// is built on the Repository.HTMLURL prefix, so a GHE thread gets the GHE
// host on the four handled subject types, not just on the fallback path.
func TestNotificationWebURL_HonoursGHEHost(t *testing.T) {
	repo := github.NotificationRepository{
		FullName: "o/r",
		HTMLURL:  "https://ghe.corp.example/o/r",
	}

	cases := []struct {
		name    string
		subject github.NotificationSubject
		want    string
	}{
		{
			name:    "pull request",
			subject: github.NotificationSubject{Type: "PullRequest", URL: "https://ghe.corp.example/api/v3/repos/o/r/pulls/42"},
			want:    "https://ghe.corp.example/o/r/pull/42",
		},
		{
			name:    "issue",
			subject: github.NotificationSubject{Type: "Issue", URL: "https://ghe.corp.example/api/v3/repos/o/r/issues/7"},
			want:    "https://ghe.corp.example/o/r/issues/7",
		},
		{
			name:    "commit",
			subject: github.NotificationSubject{Type: "Commit", URL: "https://ghe.corp.example/api/v3/repos/o/r/commits/abc123"},
			want:    "https://ghe.corp.example/o/r/commit/abc123",
		},
		{
			name:    "release",
			subject: github.NotificationSubject{Type: "Release", URL: "https://ghe.corp.example/api/v3/repos/o/r/releases/9"},
			want:    "https://ghe.corp.example/o/r/releases",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thread := github.NotificationThread{Subject: tc.subject, Repository: repo}
			got := github.NotificationWebURL(thread)
			if got != tc.want {
				t.Errorf("NotificationWebURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNotificationWebURL_FallsBackToConstructedRepoURLWhenHTMLURLEmpty(t *testing.T) {
	thread := github.NotificationThread{
		Subject: github.NotificationSubject{
			Type: "CheckSuite",
		},
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
			// HTMLURL deliberately empty.
		},
	}

	got := github.NotificationWebURL(thread)
	want := "https://github.com/octo/repo"
	if got != want {
		t.Errorf("NotificationWebURL() = %q, want %q (constructed from FullName)", got, want)
	}
}

// TestNotificationWebURL_EmptyWhenRepositoryAbsent pins the empty-prefix early
// return. The zero-thread row alone does not: its Subject.Type is "", so it
// never reaches a per-type branch and would return "" even without the guard.
// The other two rows pair an absent repository with a subject type that DOES
// resolve, so without the guard they return the relative strings "/releases"
// and "/pull/42" — bare paths handed to the OS opener, strictly worse than "",
// which the caller can at least detect.
func TestNotificationWebURL_EmptyWhenRepositoryAbsent(t *testing.T) {
	cases := []struct {
		name   string
		thread github.NotificationThread
	}{
		{
			// A pathological all-absent case: no subject.url, no repository
			// fields at all. Must not panic and must not produce a malformed
			// link.
			name:   "zero thread",
			thread: github.NotificationThread{},
		},
		{
			name: "release with absent repository",
			thread: github.NotificationThread{
				Subject:    github.NotificationSubject{Type: "Release"},
				Repository: github.NotificationRepository{},
			},
		},
		{
			name: "pull request with absent repository",
			thread: github.NotificationThread{
				Subject: github.NotificationSubject{
					Type: "PullRequest",
					URL:  "https://api.github.com/repos/o/r/pulls/42",
				},
				Repository: github.NotificationRepository{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := github.NotificationWebURL(tc.thread)
			if got != "" {
				t.Errorf("NotificationWebURL() = %q, want empty string when the repository payload is absent", got)
			}
		})
	}
}

// TestNotificationWebURL_TrailingSlashHTMLURLYieldsSingleSlash pins the
// TrimSuffix in repositoryWebURL. GitHub's own html_url never carries a
// trailing slash, but the prefix is wire-controlled, and without the trim this
// payload emits "https://github.com/octo/repo//pull/42".
func TestNotificationWebURL_TrailingSlashHTMLURLYieldsSingleSlash(t *testing.T) {
	thread := github.NotificationThread{
		Subject: github.NotificationSubject{
			Type: "PullRequest",
			URL:  "https://api.github.com/repos/octo/repo/pulls/42",
		},
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
			HTMLURL:  "https://github.com/octo/repo/",
		},
	}

	got := github.NotificationWebURL(thread)
	want := "https://github.com/octo/repo/pull/42"
	if got != want {
		t.Errorf("NotificationWebURL() = %q, want %q (trailing slash on the wire HTMLURL must not double up)", got, want)
	}
}
