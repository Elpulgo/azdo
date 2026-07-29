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

// TestNotificationSourceInterfaceExists passes trivially once the file
// compiles. The real gate is the compile-time var _ assertion above.
func TestNotificationSourceInterfaceExists(t *testing.T) {
	t.Log("provider.NotificationSource interface compiles and stubNotificationSource satisfies it")
}

// TestNotificationReasonEnum_MatchesDecision18 pins the exact value set from
// spec Decision 18. It is written so that a future rename or omission fails
// loudly at compile time (unknown identifier) or at test time (missing from
// the set / wrong count).
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

	const wantCount = 12
	if len(want) != wantCount {
		t.Fatalf("test setup error: want slice has %d entries, expected %d", len(want), wantCount)
	}

	seen := make(map[provider.NotificationReason]bool, len(want))
	for i, r := range want {
		if seen[r] {
			t.Fatalf("duplicate NotificationReason value at index %d: %v (two decision-18 names share an ordinal)", i, r)
		}
		seen[r] = true
	}
	if len(seen) != wantCount {
		t.Fatalf("expected exactly %d distinct NotificationReason values, got %d", wantCount, len(seen))
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
}

// TestNotification_ReadAndDoneFieldsExist pins Decision 15: both Read and
// Done exist on the neutral type in phase 1, even though phase 1 keeps no
// local state. This test fails to compile if either field is removed.
func TestNotification_ReadAndDoneFieldsExist(t *testing.T) {
	n := provider.Notification{Read: true, Done: false}
	if !n.Read {
		t.Fatalf("expected Read to be true")
	}
	if n.Done {
		t.Fatalf("expected Done to be false")
	}
}
