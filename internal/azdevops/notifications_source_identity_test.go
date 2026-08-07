package azdevops

import "testing"

// TestGuardNonEmptyUserID pins finding 8 of task 7's review. The guard is
// unreachable through a real Client — GetCurrentUserID rejects an empty
// AuthenticatedUser.ID at client.go:234 before resolveAuthenticatedUserID
// ever sees one — which is exactly why it is a separate function: a branch
// that cannot be reached through the production path can still be exercised
// directly, and without this test, deleting the guard changes no test result
// at all.
//
// What the guard prevents is a fail-open, not a fail-closed: both call sites
// compare identities with ==, so an empty id matches every payload whose own
// identity field is absent rather than matching nothing.
func TestGuardNonEmptyUserID(t *testing.T) {
	if _, err := guardNonEmptyUserID(""); err == nil {
		t.Fatal("guardNonEmptyUserID(\"\") returned no error; an empty id must never reach an == identity comparison")
	}

	got, err := guardNonEmptyUserID("user-1")
	if err != nil {
		t.Fatalf("guardNonEmptyUserID(\"user-1\") error = %v, want nil", err)
	}
	if got != "user-1" {
		t.Errorf("guardNonEmptyUserID(\"user-1\") = %q, want %q", got, "user-1")
	}
}
