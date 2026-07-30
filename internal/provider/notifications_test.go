package provider_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// stubNotificationSource is a compile-time conformance probe that claims to
// implement provider.NotificationSource. If the interface does not yet
// exist, or any method signature changes, this file fails to compile — that
// is the intentional TDD gate (mirrors stubProvider in provider_test.go).
type stubNotificationSource struct{}

func (s stubNotificationSource) List(opts provider.NotifOpts) ([]provider.Notification, error) {
	return nil, nil
}

func (s stubNotificationSource) MarkRead(id provider.Identity) error { return nil }

func (s stubNotificationSource) MarkDone(id provider.Identity) error { return nil }

// compile-time assertion: stubNotificationSource must satisfy
// provider.NotificationSource.
var _ provider.NotificationSource = stubNotificationSource{}

// TestNotificationReasonEnum_MatchesDecision18 pins the exact value set from
// spec Decision 18 in both directions: naming every value catches a rename or
// removal at compile time, and comparing the count against the enum's own
// sentinel catches an *addition* — the drift the enum's doc comment forbids
// first, and the one a name-only list cannot see.
func TestNotificationReasonEnum_MatchesDecision18(t *testing.T) {
	want := []provider.NotificationReason{
		provider.NotificationReasonUnknown,
		provider.NotificationReasonReviewRequested,
		provider.NotificationReasonMentioned,
		provider.NotificationReasonAssigned,
		provider.NotificationReasonAuthored,
		provider.NotificationReasonCommented,
		provider.NotificationReasonStateChanged,
		provider.NotificationReasonCIActivity,
		provider.NotificationReasonSecurityAlert,
		provider.NotificationReasonApprovalRequested,
		provider.NotificationReasonSubscribed,
		provider.NotificationReasonOther,
	}

	// Decision 18 lists exactly twelve reasons. Asserting against the enum's
	// own sentinel is what catches an added value: a thirteenth constant makes
	// NotificationReasonCount() 13 while this list stays at 12.
	if got := provider.NotificationReasonCount(); got != len(want) {
		t.Fatalf("enum has %d values but Decision 18 lists %d — a value was added or removed without updating the decision", got, len(want))
	}

	// NotificationReasonUnknown must remain the zero value so an unset field
	// is distinguishable from a deliberately-mapped reason.
	if provider.NotificationReasonUnknown != 0 {
		t.Fatalf("NotificationReasonUnknown must be the zero value, got %v", provider.NotificationReasonUnknown)
	}
}

// TestNotification_IdentityIsProviderQualified asserts Decision 14: identity
// is kind + scope + native id, never a bare id. Two notifications with the
// same native id but a different Kind or Scope must not collide.
func TestNotification_IdentityIsProviderQualified(t *testing.T) {
	githubRepoA := provider.Notification{
		Identity: provider.Identity{Kind: provider.KindGitHub, Scope: "acme/repo-a", ID: "1"},
	}
	githubRepoB := provider.Notification{
		Identity: provider.Identity{Kind: provider.KindGitHub, Scope: "acme/repo-b", ID: "1"},
	}
	azureSameScope := provider.Notification{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "acme/repo-a", ID: "1"},
	}
	githubRepoADuplicate := provider.Notification{
		Identity: provider.Identity{Kind: provider.KindGitHub, Scope: "acme/repo-a", ID: "1"},
	}

	if githubRepoA.Identity.SameItem(githubRepoB.Identity) {
		t.Fatalf("same native id, different Scope must not collide: %+v vs %+v", githubRepoA.Identity, githubRepoB.Identity)
	}
	if githubRepoA.Identity.SameItem(azureSameScope.Identity) {
		t.Fatalf("same native id, different Kind must not collide: %+v vs %+v", githubRepoA.Identity, azureSameScope.Identity)
	}
	if !githubRepoA.Identity.SameItem(githubRepoADuplicate.Identity) {
		t.Fatalf("identical Kind+Scope+ID must be treated as the same item: %+v vs %+v", githubRepoA.Identity, githubRepoADuplicate.Identity)
	}

	// ScopeDisplay must stay out of the comparison. It is a human-facing label
	// that can change (a repo rename, a fallback display name), and phase 2
	// keys its local read/done store on this identity — if display text leaked
	// in, a changed label would silently resurrect everything the user
	// dismissed. This is the only test of SameItem in the repo.
	displayA := provider.Identity{Kind: provider.KindGitHub, Scope: "acme/repo-a", ScopeDisplay: "Repo A", ID: "1"}
	displayB := provider.Identity{Kind: provider.KindGitHub, Scope: "acme/repo-a", ScopeDisplay: "acme/repo-a", ID: "1"}
	if !displayA.SameItem(displayB) {
		t.Fatalf("identities differing only in ScopeDisplay must be the same item: %+v vs %+v", displayA, displayB)
	}
}

// TestNotificationReason_String pins Decision 19's config-facing contract:
// lowercase snake_case, exactly the strings task 3's design notes settle on.
// Tasks 9 and 10 parse user config through ParseNotificationReason, so these
// strings are effectively public API — a rename here is a breaking change.
func TestNotificationReason_String(t *testing.T) {
	tests := []struct {
		name   string
		reason provider.NotificationReason
		want   string
	}{
		// Out-of-range must fall back to "other", matching the glyph, label and
		// style fallbacks in ui/display (which pins the same NotificationReason(99)
		// sentinel). Before Unknown got its own case this branch was covered
		// incidentally; it needs its own row now.
		{"out_of_range", provider.NotificationReason(99), "other"},
		{"unknown", provider.NotificationReasonUnknown, "unknown"},
		{"review_requested", provider.NotificationReasonReviewRequested, "review_requested"},
		{"mentioned", provider.NotificationReasonMentioned, "mentioned"},
		{"assigned", provider.NotificationReasonAssigned, "assigned"},
		{"authored", provider.NotificationReasonAuthored, "authored"},
		{"commented", provider.NotificationReasonCommented, "commented"},
		{"state_changed", provider.NotificationReasonStateChanged, "state_changed"},
		{"ci_activity", provider.NotificationReasonCIActivity, "ci_activity"},
		{"security_alert", provider.NotificationReasonSecurityAlert, "security_alert"},
		{"approval_requested", provider.NotificationReasonApprovalRequested, "approval_requested"},
		{"subscribed", provider.NotificationReasonSubscribed, "subscribed"},
		{"other", provider.NotificationReasonOther, "other"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.reason.String(); got != tc.want {
				t.Errorf("%d.String() = %q, want %q", tc.reason, got, tc.want)
			}
		})
	}
}

// TestNotificationReason_StringRoundTripsThroughParse asserts the round-trip
// contract for all 12 declared values, per Decision 26's asymmetry:
//   - The 11 non-Unknown values survive String() -> ParseNotificationReason()
//     unchanged, with the recognised bool true. This is what guarantees
//     exclude_reasons config values map back to the exact reason the user
//     intended.
//   - NotificationReasonUnknown is the deliberate exception: its String() is
//     "unknown", but that string is reserved (the wire mapper never emits
//     Unknown) and does not parse back to itself — ParseNotificationReason
//     degrades it to (Other, false), same as any other unrecognised string.
func TestNotificationReason_StringRoundTripsThroughParse(t *testing.T) {
	all := []provider.NotificationReason{
		provider.NotificationReasonUnknown,
		provider.NotificationReasonReviewRequested,
		provider.NotificationReasonMentioned,
		provider.NotificationReasonAssigned,
		provider.NotificationReasonAuthored,
		provider.NotificationReasonCommented,
		provider.NotificationReasonStateChanged,
		provider.NotificationReasonCIActivity,
		provider.NotificationReasonSecurityAlert,
		provider.NotificationReasonApprovalRequested,
		provider.NotificationReasonSubscribed,
		provider.NotificationReasonOther,
	}
	if len(all) != provider.NotificationReasonCount() {
		t.Fatalf("test lists %d values but the enum declares %d — update this list alongside Decision 18", len(all), provider.NotificationReasonCount())
	}
	for _, r := range all {
		t.Run(r.String(), func(t *testing.T) {
			got, ok := provider.ParseNotificationReason(r.String())

			if r == provider.NotificationReasonUnknown {
				if got != provider.NotificationReasonOther || ok {
					t.Errorf("ParseNotificationReason(%q) = (%v, %v), want (Other, false) — unknown is reserved and must not round-trip to itself (Decision 26)", r.String(), got, ok)
				}
				return
			}

			if !ok {
				t.Errorf("ParseNotificationReason(%q) ok = false, want true", r.String())
			}
			if got != r {
				t.Errorf("ParseNotificationReason(%q) = %v, want %v", r.String(), got, r)
			}
		})
	}
}

// TestParseNotificationReason_UnparseableYieldsOther asserts Decision 26's
// hard rule end to end: a config value the parser does not recognise must
// resolve to (NotificationReasonOther, false) — never an error that would
// cause a filter to drop the row, never the zero value (which would misfile
// it as "unset" rather than "recognised but uncategorised"), and never a bool
// that claims recognition it did not have — task 9's warning and task 10's
// skip-unrecognised behaviour both depend on the bool being accurate.
func TestParseNotificationReason_UnparseableYieldsOther(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"garbage word", "nonsense"},
		{"empty string", ""},
		// Case-sensitivity: String() only ever emits lowercase, and nothing
		// upstream lowercases these strings — `exclude_reasons` entries are
		// list *values*, and viper lowercases config *keys* only, so they
		// arrive verbatim. A mixed-case match is deliberately NOT folded: it
		// is treated the same as any other unrecognised string, surfacing the
		// typo instead of silently reinterpreting it (Decision 26).
		{"mixed case", "Review_Requested"},
		{"upper case", "REVIEW_REQUESTED"},
		// Decision 26: "unknown" is reserved. The wire mapper never emits
		// NotificationReasonUnknown, so this string must be reported as
		// unrecognised even though it is what Unknown.String() emits.
		{"reserved unknown", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := provider.ParseNotificationReason(tc.input)
			if got != provider.NotificationReasonOther {
				t.Errorf("ParseNotificationReason(%q) = %v, want NotificationReasonOther", tc.input, got)
			}
			if ok {
				t.Errorf("ParseNotificationReason(%q) ok = true, want false", tc.input)
			}
		})
	}
}
