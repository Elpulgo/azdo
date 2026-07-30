package notifications

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestFilterNotifications_NilConfig_ReturnsCopyOfEveryRow(t *testing.T) {
	rows := []provider.Notification{
		row("1", "owner/repo", provider.NotificationReasonOther, false),
		row("2", "owner/other", provider.NotificationReasonSubscribed, true),
	}
	got := FilterNotifications(rows, nil)
	assertIDs(t, got, "1", "2")

	// Decision 52a: the nil-cfg path allocates like every other path, so the
	// caller's slice is not handed back. `return rows` would make this
	// mutation visible in the caller's feed, which the pane keeps unfiltered
	// so it can re-apply task 11's interactive `f` filter.
	got[0].Title = "MUTATED"
	if rows[0].Title == "MUTATED" {
		t.Fatal("nil-cfg result aliases the caller's slice (return rows instead of a copy?)")
	}
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

// TestFilterNotifications_ZeroValueConfig_NotAliased pins decision 52a on the
// zero-knob `default:` selection branch. That branch is the shipped default
// config -- the most-exercised path in the product -- and it is also the only
// one where no row is ever dropped, so no length or content assertion can
// distinguish `out = append(out, rows...)` from `out = rows`. Since no
// subtraction knob is set, dropWhere never runs to reallocate and mask it,
// making this the only case that pins the branch's own allocation.
func TestFilterNotifications_ZeroValueConfig_NotAliased(t *testing.T) {
	original := []provider.Notification{
		row("1", "owner/a", provider.NotificationReasonReviewRequested, false),
		row("2", "owner/b", provider.NotificationReasonSubscribed, true),
	}
	got := FilterNotifications(original, &config.Config{})
	assertIDs(t, got, "1", "2")

	got[0].Title = "MUTATED"
	if original[0].Title == "MUTATED" {
		t.Fatal("zero-knob result aliases the caller's slice (out = rows in the default branch?)")
	}
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

// TestFilterNotifications_OnlyConfiguredRepos_CaseInsensitiveBothDirections
// pins that BOTH sides of the github.repos lookup are lower-cased (decision
// 51's case rule, applied to the strict-mode selector). One direction per
// case, because either half alone passes one of them: with only the config
// key lowered, an uppercase Scope misses; with only the Scope lowered, an
// uppercase config entry misses. GitHub owner/repo names are
// case-insensitive while Scope carries the wire's canonical casing, so a user
// who types `Acme/Repo` must still see their rows.
func TestFilterNotifications_OnlyConfiguredRepos_CaseInsensitiveBothDirections(t *testing.T) {
	tests := []struct {
		name           string
		configuredRepo string
		rowScope       string
	}{
		{"config entry uppercase, row scope lowercase", "Acme/Repo", "acme/repo"},
		{"config entry lowercase, row scope uppercase", "acme/repo", "Acme/Repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := []provider.Notification{
				row("match", tt.rowScope, provider.NotificationReasonOther, false),
				row("other", "unrelated/repo", provider.NotificationReasonOther, false),
			}
			cfg := &config.Config{
				GitHub:        config.GitHubConfig{Repos: []string{tt.configuredRepo}},
				Notifications: config.NotificationsConfig{OnlyConfiguredRepos: true},
			}
			got := FilterNotifications(rows, cfg)
			assertIDs(t, got, "match")
		})
	}
}

// TestFilterNotifications_OnlyConfiguredRepos_TrimsConfiguredRepoWhitespace
// pins that a stray space in github.repos does not silently empty the feed.
// `Validate()` accepts "acme/repo " today (its slug check only rejects extra
// slashes and empty entries), and an untrimmed lookup key then matches no row
// at all -- the strict-mode equivalent of the empty feed decision 51 avoids
// for globs by trimming them.
func TestFilterNotifications_OnlyConfiguredRepos_TrimsConfiguredRepoWhitespace(t *testing.T) {
	rows := []provider.Notification{
		row("match", "acme/repo", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		GitHub:        config.GitHubConfig{Repos: []string{"  acme/repo "}},
		Notifications: config.NotificationsConfig{OnlyConfiguredRepos: true},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "match")
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

// --- Decision 51a: the filter mirrors the load-time sanitizer, so a
// struct-literal config that bypassed it behaves identically ---

// TestFilterNotifications_IncludeRepos_AllPatternsBad_SelectsEverything pins
// the fail-open half of decision 51 in the filter itself. Treating an
// uncompilable pattern as "no match" is fail-OPEN for exclude_repos (nothing
// is excluded) but fail-CLOSED for include_repos: `["[bad"]` would select
// zero rows and the user's whole feed would vanish. `LoadFrom` drops the
// pattern before the filter sees it, leaving an empty list that selects
// everything -- so the filter reproduces that instead of selecting nothing.
func TestFilterNotifications_IncludeRepos_AllPatternsBad_SelectsEverything(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/repo", provider.NotificationReasonOther, false),
		row("2", "other/repo", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{IncludeRepos: []string{"[bad"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1", "2")
}

// TestFilterNotifications_IncludeRepos_OneBadOneGood_GoodStillSelects pins the
// other side of the mirror: one compilable pattern is enough to make the list
// meaningful, so the bad entry is skipped rather than widening the selection
// back to everything.
func TestFilterNotifications_IncludeRepos_OneBadOneGood_GoodStillSelects(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/repo", provider.NotificationReasonOther, false),
		row("2", "other/repo", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{IncludeRepos: []string{"[bad", "acme/*"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1")
}

// TestFilterNotifications_IncludeRepos_BadPattern_SameResultFromLoadFromAndLiteral
// is the property decision 51a actually buys: identical output whatever the
// construction path. The LoadFrom config has been through the sanitizer (the
// bad pattern is gone from the slice); the struct literal has not (it is still
// there). Tasks 11-13 build their fixtures the second way, so a divergence
// here would only show up as an empty pane in a hand-built fixture.
func TestFilterNotifications_IncludeRepos_BadPattern_SameResultFromLoadFromAndLiteral(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/repo", provider.NotificationReasonOther, false),
		row("2", "other/repo", provider.NotificationReasonOther, false),
	}

	tests := []struct {
		name     string
		patterns []string
		yaml     string
	}{
		{
			name:     "only a bad pattern",
			patterns: []string{"[bad"},
			yaml:     "    - \"[bad\"\n",
		},
		{
			name:     "a bad pattern and a good one",
			patterns: []string{"[bad", "acme/*"},
			yaml:     "    - \"[bad\"\n    - \"acme/*\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			content := `github:
  repos:
    - acme/repo
polling_interval: 60
theme: dark
notifications:
  include_repos:
` + tt.yaml
			if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			loaded, err := config.LoadFrom(configPath)
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			literal := &config.Config{
				Notifications: config.NotificationsConfig{IncludeRepos: tt.patterns},
			}
			// Sanity: the two configs really do differ in their raw lists,
			// otherwise the comparison below is vacuous.
			if len(loaded.Notifications.IncludeRepos) == len(literal.Notifications.IncludeRepos) {
				t.Fatalf("fixture is vacuous: LoadFrom kept %v, same length as the literal's %v",
					loaded.Notifications.IncludeRepos, literal.Notifications.IncludeRepos)
			}

			fromLoad := idsOf(FilterNotifications(rows, loaded))
			fromLiteral := idsOf(FilterNotifications(rows, literal))
			if len(fromLoad) != len(fromLiteral) {
				t.Fatalf("LoadFrom gave %v, struct literal gave %v", fromLoad, fromLiteral)
			}
			for i := range fromLoad {
				if fromLoad[i] != fromLiteral[i] {
					t.Fatalf("LoadFrom gave %v, struct literal gave %v", fromLoad, fromLiteral)
				}
			}
		})
	}
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

// TestFilterNotifications_ExcludeRepos_AllPatternsBad_ExcludesNothing is the
// exclude-side counterpart of the include mirror above. Skipping the pattern
// already reproduces what the sanitizer would have produced here -- an empty
// exclude list excludes nothing -- so this list needs no fallback rule, but
// the outcome is worth pinning: the failure mode to avoid is a bad pattern
// treated as match-all, which would wipe the feed.
func TestFilterNotifications_ExcludeRepos_AllPatternsBad_ExcludesNothing(t *testing.T) {
	rows := []provider.Notification{
		row("1", "acme/keep", provider.NotificationReasonOther, false),
		row("2", "other/keep", provider.NotificationReasonOther, false),
	}
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{ExcludeRepos: []string{"[bad"}},
	}
	got := FilterNotifications(rows, cfg)
	assertIDs(t, got, "1", "2")
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

// --- Decision 52: the three knobs the filter must NOT read ---

// TestFilterNotifications_NonFilterKnobs_NotReadByFilter pins decision 52's
// knob scope for all three out-of-scope knobs, not just participating_only.
// participating_only and since_days are fetch-time knobs (provider.NotifOpts)
// and max_items is applied by the composite after the merge sort (decisions
// 31, 44); honouring any of them here would double-filter -- max_items would
// truncate a feed the composite already capped, and since_days would re-cut a
// window the server already applied.
//
// The fixture is three rows old enough that any plausible since_days cutoff
// would drop them, and each case sets a value that would visibly shrink the
// result if the filter read it (MaxItems: 1 would leave one row, SinceDays: 1
// would leave none). All three rows must survive every case.
func TestFilterNotifications_NonFilterKnobs_NotReadByFilter(t *testing.T) {
	old := time.Now().AddDate(0, 0, -90)
	rows := []provider.Notification{
		row("review-row", "acme/repo", provider.NotificationReasonReviewRequested, false),
		row("subscribed-row", "acme/repo", provider.NotificationReasonSubscribed, false),
		row("mention-row", "acme/repo", provider.NotificationReasonMentioned, false),
	}
	for i := range rows {
		rows[i].UpdatedAt = old
	}

	tests := []struct {
		name string
		nc   config.NotificationsConfig
	}{
		{"no out-of-scope knob set (baseline)", config.NotificationsConfig{}},
		{"participating_only is fetch-time", config.NotificationsConfig{ParticipatingOnly: true}},
		{"max_items is the composite's job", config.NotificationsConfig{MaxItems: 1}},
		{"since_days is fetch-time", config.NotificationsConfig{SinceDays: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterNotifications(rows, &config.Config{Notifications: tt.nc})
			assertIDs(t, got, "review-row", "subscribed-row", "mention-row")
		})
	}
}

// TestFilterNotifications_NonFilterKnobs_ComposeWithAnInScopeKnob keeps the
// original decision 10 compose case: an out-of-scope knob set alongside an
// in-scope one changes nothing about what the in-scope knob does.
func TestFilterNotifications_NonFilterKnobs_ComposeWithAnInScopeKnob(t *testing.T) {
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
	// path.Match("[bad", ...) returns ErrBadPattern. matchesAnyGlob skips such
	// a pattern, so it never matches -- match-all would be the worse error
	// here, since a malformed exclude_repos entry matching everything wipes
	// the feed. Skipping is the whole story for a subtractive list; a
	// selection list additionally needs compilableGlobs (decision 51a), which
	// TestFilterNotifications_IncludeRepos_AllPatternsBad_SelectsEverything
	// pins.
	got := matchesAnyGlob([]string{"[bad"}, "acme/repo")
	if got {
		t.Error("matchesAnyGlob with a malformed pattern = true, want false (a bad pattern is inert, not match-all)")
	}
}

func TestCompilableGlobs(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{"nil stays empty", nil, nil},
		{"all compilable are kept in order", []string{"acme/*", "other/repo"}, []string{"acme/*", "other/repo"}},
		{"the bad one is dropped, the good one survives", []string{"[bad", "acme/*"}, []string{"acme/*"}},
		{"all bad collapses to empty, which callers read as no selection", []string{"[bad", "also[bad"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compilableGlobs(tt.patterns)
			if len(got) != len(tt.want) {
				t.Fatalf("compilableGlobs(%v) = %v, want %v", tt.patterns, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("compilableGlobs(%v) = %v, want %v", tt.patterns, got, tt.want)
				}
			}
		})
	}
}

// --- NotifOptsFromConfig (task 15, decision 52) ---

func TestNotifOptsFromConfig_NilConfig_ReturnsZeroValue(t *testing.T) {
	got := NotifOptsFromConfig(nil)
	want := provider.NotifOpts{}
	if got != want {
		t.Errorf("NotifOptsFromConfig(nil) = %+v, want zero value %+v", got, want)
	}
}

func TestNotifOptsFromConfig_ForwardsParticipatingOnlyAndMax(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			ParticipatingOnly: true,
			MaxItems:          42,
		},
	}

	got := NotifOptsFromConfig(cfg)

	if !got.ParticipatingOnly {
		t.Error("ParticipatingOnly = false, want true")
	}
	if got.Max != 42 {
		t.Errorf("Max = %d, want 42", got.Max)
	}
	if !got.Since.IsZero() {
		t.Errorf("Since = %v, want zero value when since_days is unset", got.Since)
	}
}

func TestNotifOptsFromConfig_SinceDays_ProducesPastCutoff(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{SinceDays: 7},
	}

	want := time.Now().AddDate(0, 0, -7)
	want = time.Date(want.Year(), want.Month(), want.Day(), 0, 0, 0, 0, want.Location())

	got := NotifOptsFromConfig(cfg)

	if !got.Since.Equal(want) {
		t.Errorf("Since = %v, want start-of-day 7 days ago %v", got.Since, want)
	}
}

// TestNotifOptsFromConfig_SinceDays_TruncatesToDay_StableAcrossSameDayCalls
// pins task 15's decision 75: NotifOpts is now re-derived on every fetch
// (not frozen at poller construction), so Since must be truncated to the day
// boundary — otherwise two calls a second apart would each compute a
// slightly different Since, changing the GitHub request path (buildPath) on
// every single poll tick.
func TestNotifOptsFromConfig_SinceDays_TruncatesToDay_StableAcrossSameDayCalls(t *testing.T) {
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{SinceDays: 3},
	}

	first := NotifOptsFromConfig(cfg)
	time.Sleep(time.Second)
	second := NotifOptsFromConfig(cfg)

	if !first.Since.Equal(second.Since) {
		t.Errorf("Since differed across same-day calls a second apart: first=%v second=%v, want identical truncated-to-day values", first.Since, second.Since)
	}
	if first.Since.Hour() != 0 || first.Since.Minute() != 0 || first.Since.Second() != 0 || first.Since.Nanosecond() != 0 {
		t.Errorf("Since = %v, want truncated to start of day (00:00:00)", first.Since)
	}
}

func TestNotifOptsFromConfig_SinceDaysZero_LeavesSinceZeroValue(t *testing.T) {
	cfg := &config.Config{Notifications: config.NotificationsConfig{SinceDays: 0}}

	got := NotifOptsFromConfig(cfg)
	if !got.Since.IsZero() {
		t.Errorf("Since = %v, want zero value when since_days is 0", got.Since)
	}
}

func TestNotifOptsFromConfig_DoesNotReadFilterOnlyKnobs(t *testing.T) {
	// exclude_repos/exclude_reasons/unread_only/include_repos/
	// only_configured_repos are FilterNotifications' concern (decision 52),
	// not NotifOptsFromConfig's -- this pins that populating them produces no
	// observable effect on the derived NotifOpts.
	cfg := &config.Config{
		Notifications: config.NotificationsConfig{
			ExcludeRepos:        []string{"acme/*"},
			ExcludeReasons:      []string{"subscribed"},
			UnreadOnly:          true,
			IncludeRepos:        []string{"acme/*"},
			OnlyConfiguredRepos: true,
		},
	}

	got := NotifOptsFromConfig(cfg)
	want := provider.NotifOpts{}
	if got != want {
		t.Errorf("NotifOptsFromConfig with only filter-stage knobs set = %+v, want zero value %+v", got, want)
	}
}
