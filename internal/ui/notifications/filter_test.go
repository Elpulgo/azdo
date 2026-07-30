package notifications

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/provider"
)

// row is a small builder to keep the table fixtures below readable.
func row(id, scope string, reason provider.NotificationReason, read bool) provider.Notification {
	return provider.Notification{
		Identity: provider.Identity{Kind: provider.KindGitHub, Scope: scope, ID: id},
		Title:    "title-" + id,
		Reason:   reason,
		Read:     read,
	}
}

func idsOf(rows []provider.Notification) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Identity.ID
	}
	return ids
}

func assertIDs(t *testing.T, got []provider.Notification, want ...string) {
	t.Helper()
	gotIDs := idsOf(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("got %d rows %v, want %d rows %v", len(gotIDs), gotIDs, len(want), want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("row %d = %q, want %q (got order %v, want order %v)", i, gotIDs[i], want[i], gotIDs, want)
		}
	}
}

// --- Nil safety ---

func TestFilterNotifications_NilConfig_ReturnsRowsUnchanged(t *testing.T) {
	rows := []provider.Notification{row("1", "owner/repo", provider.NotificationReasonOther, false)}
	got := FilterNotifications(rows, nil)
	assertIDs(t, got, "1")
}

func TestFilterNotifications_NilRows_ReturnsNil(t *testing.T) {
	cfg := &config.Config{}
	got := FilterNotifications(nil, cfg)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFilterNotifications_EmptyRows_ReturnsNil(t *testing.T) {
	cfg := &config.Config{}
	got := FilterNotifications([]provider.Notification{}, cfg)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// --- Zero-value config: widest behaviour ---

func TestFilterNotifications_ZeroValueConfig_PassesEverythingThrough(t *testing.T) {
	rows := []provider.Notification{
		row("1", "owner/a", provider.NotificationReasonReviewRequested, false),
		row("2", "owner/b", provider.NotificationReasonSubscribed, true),
		row("3", "other/c", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1", "2", "3")
}

// --- Each knob alone ---

func TestFilterNotifications_OnlyConfiguredRepos_Alone(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/configured", provider.NotificationReasonOther, false),
		row("2", "acme/unconfigured", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		GitHub:        config.GitHubConfig{Repos: []string{"acme/configured"}},
		Notifications: config.NotificationsConfig{OnlyConfiguredRepos: true},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1")
}

func TestFilterNotifications_IncludeRepos_Alone(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/repo", provider.NotificationReasonOther, false),
		row("2", "other/repo", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{IncludeRepos: []string{"acme/*"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1")
}

func TestFilterNotifications_ExcludeRepos_Alone(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/keep", provider.NotificationReasonOther, false),
		row("2", "acme/noisy", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{ExcludeRepos: []string{"acme/noisy"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1")
}

func TestFilterNotifications_ExcludeReasons_Alone(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/repo", provider.NotificationReasonReviewRequested, false),
		row("2", "acme/repo", provider.NotificationReasonSubscribed, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{ExcludeReasons: []string{"subscribed"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1")
}

func TestFilterNotifications_UnreadOnly_Alone(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/repo", provider.NotificationReasonOther, false),
		row("2", "acme/repo", provider.NotificationReasonOther, true),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{UnreadOnly: true},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1")
}

// --- Full precedence chain: one fixture, one row removed per stage ---

func TestFilterNotifications_FullPrecedenceChain(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			IncludeRepos:   []string{"acme/*"},
			ExcludeRepos:   []string{"acme/blocked"},
			ExcludeReasons: []string{"subscribed"},
			UnreadOnly:     true,
		},
	}

	rows := []provider.Notification{
		// Fails selection: scope doesn't match include_repos.
		row("not-included", "other/repo", provider.NotificationReasonReviewRequested, false),
		// Passes selection, fails exclude_repos.
		row("excluded-repo", "acme/blocked", provider.NotificationReasonReviewRequested, false),
		// Passes selection + exclude_repos, fails exclude_reasons.
		row("excluded-reason", "acme/keep", provider.NotificationReasonSubscribed, false),
		// Passes selection + exclude_repos + exclude_reasons, fails unread_only.
		row("already-read", "acme/keep2", provider.NotificationReasonReviewRequested, true),
		// Survives every stage.
		row("survivor", "acme/keep3", provider.NotificationReasonReviewRequested, false),
	}

	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "survivor")
}

// --- Decision 50: only_configured_repos overrides include_repos, not intersects ---

func TestFilterNotifications_OnlyConfiguredRepos_OverridesIncludeRepos(t *testing.T) {
	// The configured repo does NOT match include_repos's pattern. Under
	// override semantics (decision 50) the row survives because
	// include_repos is ignored entirely. Under (wrong) intersection
	// semantics it would be dropped for failing the include_repos glob --
	// this fixture is built specifically so the two behaviours diverge.
	cfg := &config.Config{
		GitHub: config.GitHubConfig{Repos: []string{"acme/configured"}},
		Notifications: config.NotificationsConfig{
			OnlyConfiguredRepos: true,
			IncludeRepos:        []string{"other/*"},
		},
	}
	rows := []provider.Notification{
		row("configured-not-in-include", "acme/configured", provider.NotificationReasonOther, false),
		row("neither", "unrelated/repo", provider.NotificationReasonOther, false),
	}

	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "configured-not-in-include")
}

// --- exclude_reasons: "other" only filters Other rows when listed explicitly ---

func TestFilterNotifications_ExcludeReasons_OtherOnlyWhenListedExplicitly(t *testing.T) {
	rows := []provider.Notification{
		row("other-row", "acme/repo", provider.NotificationReasonOther, false),
		row("review-row", "acme/repo", provider.NotificationReasonReviewRequested, false),
	}

	t.Run("other listed explicitly trims Other rows", func(t *testing.T) {
		cfg := &config.Config{
			Notifications: config.NotificationsConfig{ExcludeReasons: []string{"other"}},
		}
		got := FilterNotifications(rows, cfg)
		assertIDs(t, got, "review-row")
	})

	t.Run("other not listed leaves Other rows in place", func(t *testing.T) {
		cfg := &config.Config{
			Notifications: config.NotificationsConfig{ExcludeReasons: []string{"assigned"}},
		}
		got := FilterNotifications(rows, cfg)
		assertIDs(t, got, "other-row", "review-row")
	})
}

// --- Decision 10 compose case: participating_only is fetch-time, not read here ---

func TestFilterNotifications_ParticipatingOnly_NotReadByFilter(t *testing.T) {
	rows := []provider.Notification{
		row("review-row", "acme/repo", provider.NotificationReasonReviewRequested, false),
		row("subscribed-row", "acme/repo", provider.NotificationReasonSubscribed, false),
	}

	withoutParticipating := &config.Config{
		Notifications: config.NotificationsConfig{ExcludeReasons: []string{"subscribed"}},
	}
	withParticipating := &config.Config{
		Notifications: config.NotificationsConfig{
			ExcludeReasons:    []string{"subscribed"},
			ParticipatingOnly: true,
		},
	}

	got1 := FilterNotifications(rows, withoutParticipating)
	got2 := FilterNotifications(rows, withParticipating)

	assertIDs(t, got1, "review-row")
	assertIDs(t, got2, "review-row")
}

// --- Task 9 guarantee: every entry the filter receives via LoadFrom parses ---

func TestFilterNotifications_ExcludeReasons_EveryLoadedEntryParses(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `github:
  repos:
    - acme/repo
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscribed
    - typo-value
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	for _, raw := range cfg.Notifications.ExcludeReasons {
		if _, ok := provider.ParseNotificationReason(raw); !ok {
			t.Errorf("cfg.Notifications.ExcludeReasons contains unparseable entry %q; task 9 should have dropped it at load", raw)
		}
	}
	// The typo was dropped, the valid entry survived.
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Fatalf("ExcludeReasons = %v, want [subscribed]", cfg.Notifications.ExcludeReasons)
	}

	rows := []provider.Notification{
		row("subscribed-row", "acme/repo", provider.NotificationReasonSubscribed, false),
		row("other-row", "acme/repo", provider.NotificationReasonOther, false),
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "other-row")
}

// --- Defence in depth: an unrecognised exclude_reasons entry, even if it
// bypasses task 9's load-time sanitizer, must be skipped rather than
// silently degrading to Other (decision 26/52). ---

func TestFilterNotifications_ExcludeReasons_UnrecognisedEntry_SkippedNotOther(t *testing.T) {
	rows := []provider.Notification{
		row("other-row", "acme/repo", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{ExcludeReasons: []string{"typo-value"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "other-row")
}

// --- Purity ---

func TestFilterNotifications_Purity_InputUnchangedAndNotAliased(t *testing.T) {
	original := []provider.Notification{
		row("keep", "acme/keep", provider.NotificationReasonOther, false),
		row("drop", "acme/drop", provider.NotificationReasonOther, false),
	}
	originalCopy := make([]provider.Notification, len(original))
	copy(originalCopy, original)

	cfg := &config.Config{
		Notifications: config.NotificationsConfig{ExcludeRepos: []string{"acme/drop"}},
	}
	got := FilterNotifications(original, cfg)

	// Length, order, and contents of the caller's slice are unchanged.
	if len(original) != len(originalCopy) {
		t.Fatalf("caller's slice length changed: got %d, want %d", len(original), len(originalCopy))
	}
	for i := range original {
		if original[i] != originalCopy[i] {
			t.Fatalf("caller's slice[%d] changed: got %+v, want %+v", i, original[i], originalCopy[i])
		}
	}

	if len(got) == 0 {
		t.Fatal("expected at least one surviving row")
	}
	if len(got) >= len(original) {
		t.Fatalf("expected fewer rows in result (%d) than input (%d)", len(got), len(original))
	}

	// Mutate the result and verify it does not alias the input's backing
	// array -- rows[:0] in place would make this mutation visible in
	// `original` too.
	got[0].Title = "MUTATED"
	if original[0].Title == "MUTATED" {
		t.Fatal("result aliases the input's backing array (rows[:0] in place?)")
	}
}

// TestFilterNotifications_Purity_SelectionStageAlone_NotAliased pins purity
// through the *selection* stage specifically, with no subtractive knob set.
// The subtraction stages (dropWhere) always allocate a fresh slice, so a test
// that also exercises a subtraction knob cannot distinguish a pure selection
// stage from one that reused rows[:0] internally -- the later dropWhere call
// would mask it by reallocating anyway. This fixture uses include_repos
// alone, which drops a row without ever calling dropWhere, so it is the only
// case that actually exercises the selection stage's own allocation.
func TestFilterNotifications_Purity_SelectionStageAlone_NotAliased(t *testing.T) {
	original := []provider.Notification{
		row("keep", "acme/keep", provider.NotificationReasonOther, false),
		row("drop", "other/drop", provider.NotificationReasonOther, false),
	}
	originalCopy := make([]provider.Notification, len(original))
	copy(originalCopy, original)

	cfg := &config.Config{
		Notifications: config.NotificationsConfig{IncludeRepos: []string{"acme/*"}},
	}
	got := FilterNotifications(original, cfg)

	if len(original) != len(originalCopy) {
		t.Fatalf("caller's slice length changed: got %d, want %d", len(original), len(originalCopy))
	}
	for i := range original {
		if original[i] != originalCopy[i] {
			t.Fatalf("caller's slice[%d] changed: got %+v, want %+v", i, original[i], originalCopy[i])
		}
	}
	if len(got) == 0 || len(got) >= len(original) {
		t.Fatalf("expected a strict subset (got %d of %d)", len(got), len(original))
	}

	got[0].Title = "MUTATED"
	if original[0].Title == "MUTATED" {
		t.Fatal("result aliases the input's backing array through the selection stage (rows[:0] in place?)")
	}
}

// --- Glob edges ---

func TestMatchesAnyGlob_Edges(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		scope    string
		want     bool
	}{
		{"owner/* matches owner/repo", []string{"owner/*"}, "owner/repo", true},
		{"* alone does not cross slash", []string{"*"}, "owner/repo", false},
		{"*/* matches owner/repo", []string{"*/*"}, "owner/repo", true},
		{"pattern uppercase, scope lowercase", []string{"Acme/Repo"}, "acme/repo", true},
		{"pattern lowercase, scope uppercase", []string{"acme/repo"}, "Acme/Repo", true},
		{"no match", []string{"other/*"}, "acme/repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAnyGlob(tt.patterns, tt.scope)
			if got != tt.want {
				t.Errorf("matchesAnyGlob(%v, %q) = %v, want %v", tt.patterns, tt.scope, got, tt.want)
			}
		})
	}
}

func TestMatchesAnyGlob_BadPattern_NeverMatchesAll(t *testing.T) {
	// path.Match("[bad", ...) returns ErrBadPattern. The filter must treat
	// that as no match, never as match-all -- a malformed exclude_repos
	// entry matching everything would empty the feed.
	got := matchesAnyGlob([]string{"[bad"}, "acme/repo")
	if got {
		t.Error("matchesAnyGlob with a malformed pattern = true, want false (must fail open, not match-all)")
	}
}
