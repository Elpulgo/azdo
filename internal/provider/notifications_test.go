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
