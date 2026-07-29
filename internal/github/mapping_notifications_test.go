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
		// Invented, never-issued-by-GitHub string.
		{"some_future_reason_nobody_has_seen_yet", provider.NotificationReasonOther},
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

// TestMapNotificationReason_NeverEmitsUnknown widens the sweep beyond
// Decision 18's list: every declared enum value plus every case above,
// scanned again, must never collapse to the zero value from any wire input.
// This is a second, independent pass over the same claim from
// TestMapNotificationReason_Decision18Table's per-case check, guarding
// against a future edit to one test loop silently dropping the assertion
// from the other.
func TestMapNotificationReason_NeverEmitsUnknown(t *testing.T) {
	inputs := []string{
		"review_requested", "mention", "team_mention", "assign", "author",
		"comment", "state_change", "ci_activity", "security_alert",
		"security_advisory_credit", "approval_requested", "subscribed",
		"manual", "invitation", "member_feature_requested",
		"unknown", "totally_bogus", "",
	}
	for _, in := range inputs {
		if got := github.MapNotificationReason(in); got == provider.NotificationReasonUnknown {
			t.Errorf("MapNotificationReason(%q) = Unknown, want anything else", in)
		}
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

// TestMapNotification_NearlyEmptyThread_NoPanic covers the wire type's only
// pointer field (LastReadAt) being nil, plus every other field left at its
// zero value — MapNotification must not panic and must not populate Unknown.
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
			name: "release",
			subject: github.NotificationSubject{
				Type: "Release",
				URL:  "https://api.github.com/repos/octo/repo/releases/99",
			},
			want: "https://github.com/octo/repo/releases/99",
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

func TestNotificationWebURL_FallsBackWhenSubjectURLEmpty(t *testing.T) {
	// GitHub sends a null subject.url for some subject types (e.g. check-suite
	// rows). Type is still "PullRequest"-shaped here to prove the empty URL,
	// not the type, is what triggers the fallback.
	thread := github.NotificationThread{
		Subject: github.NotificationSubject{
			Type: "PullRequest",
			URL:  "",
		},
		Repository: github.NotificationRepository{
			FullName: "octo/repo",
			HTMLURL:  "https://github.com/octo/repo",
		},
	}

	got := github.NotificationWebURL(thread)
	want := "https://github.com/octo/repo"
	if got != want {
		t.Errorf("NotificationWebURL() = %q, want %q (repo fallback on empty subject.url)", got, want)
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

func TestNotificationWebURL_EmptyWhenRepositoryAbsent(t *testing.T) {
	// A pathological all-absent case: no subject.url, no repository fields at
	// all. Must not panic and must not produce a malformed link.
	var thread github.NotificationThread

	got := github.NotificationWebURL(thread)
	if got != "" {
		t.Errorf("NotificationWebURL() = %q, want empty string when repository is entirely absent", got)
	}
}
