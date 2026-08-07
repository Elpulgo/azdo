package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every fixture in this file goes through LoadFrom(filepath.Join(t.TempDir(),
// "config.yaml")) when it needs Load-time behavior (defaults, warnings,
// mixed-case keys). Pure Validate() cases use a bare Config{} struct literal
// — that is safe because Validate() never touches disk; only Save() does
// (convention 17). None of the tests in this file call Save().

func TestLoad_NotificationsDefaults_WhenBlockAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if n.GitHub.OnlyConfiguredRepos {
		t.Error("GitHub.OnlyConfiguredRepos = true by default; want false")
	}
	if len(n.ExcludeRepos) != 0 {
		t.Errorf("ExcludeRepos = %v, want empty", n.ExcludeRepos)
	}
	if len(n.IncludeRepos) != 0 {
		t.Errorf("IncludeRepos = %v, want empty", n.IncludeRepos)
	}
	if len(n.ExcludeReasons) != 0 {
		t.Errorf("ExcludeReasons = %v, want empty", n.ExcludeReasons)
	}
	if n.UnreadOnly {
		t.Error("UnreadOnly = true by default; want false")
	}
	if n.GitHub.ParticipatingOnly {
		t.Error("GitHub.ParticipatingOnly = true by default; want false")
	}
	if n.GitHub.SinceDays != 0 {
		t.Errorf("GitHub.SinceDays = %d, want 0", n.GitHub.SinceDays)
	}
	if n.MaxItems != 0 {
		t.Errorf("MaxItems = %d, want 0", n.MaxItems)
	}
	if n.PollInterval != 0 {
		t.Errorf("PollInterval = %d, want 0", n.PollInterval)
	}
	// Unlike the shared/GitHub fields above, Azure's two numeric fields do
	// NOT default to the Go zero value — LoadFrom's v.SetDefault registers
	// 14 and 300 (decision 13's YAML), and all four source toggles default
	// to true.
	if n.Azure.LookbackDays != DefaultAzureLookbackDays {
		t.Errorf("Azure.LookbackDays = %d, want %d (default)", n.Azure.LookbackDays, DefaultAzureLookbackDays)
	}
	if n.Azure.MinPollInterval != DefaultAzureMinPollInterval {
		t.Errorf("Azure.MinPollInterval = %d, want %d (default)", n.Azure.MinPollInterval, DefaultAzureMinPollInterval)
	}
	if !n.Azure.Sources.ReviewRequested || !n.Azure.Sources.Mentioned || !n.Azure.Sources.Assigned || !n.Azure.Sources.CIFailed {
		t.Errorf("Azure.Sources = %+v, want all four true by default", n.Azure.Sources)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for a clean config", cfg.Warnings)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with absent notifications block = %v, want nil", err)
	}
}

func TestLoad_NotificationsDefaults_WhenBlockPresentButEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications: {}
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if n.GitHub.OnlyConfiguredRepos || n.UnreadOnly || n.GitHub.ParticipatingOnly {
		t.Errorf("expected all bools false for empty block, got %+v", n)
	}
	if len(n.ExcludeRepos) != 0 || len(n.IncludeRepos) != 0 || len(n.ExcludeReasons) != 0 {
		t.Errorf("expected all lists empty for empty block, got %+v", n)
	}
	if n.GitHub.SinceDays != 0 || n.MaxItems != 0 || n.PollInterval != 0 {
		t.Errorf("expected all ints 0 for empty block, got %+v", n)
	}
}

func TestLoad_NotificationsBlock_AllKeysParsed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_repos:
    - "spammy/*"
    - "owner/noisy-repo"
  include_repos:
    - "owner/repo"
  exclude_reasons:
    - subscribed
    - ci_activity
  unread_only: true
  max_items: 50
  poll_interval: 120
  github:
    only_configured_repos: true
    participating_only: true
    since_days: 7
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if !n.GitHub.OnlyConfiguredRepos {
		t.Error("GitHub.OnlyConfiguredRepos = false, want true")
	}
	if len(n.ExcludeRepos) != 2 || n.ExcludeRepos[0] != "spammy/*" || n.ExcludeRepos[1] != "owner/noisy-repo" {
		t.Errorf("ExcludeRepos = %v, want [spammy/* owner/noisy-repo]", n.ExcludeRepos)
	}
	if len(n.IncludeRepos) != 1 || n.IncludeRepos[0] != "owner/repo" {
		t.Errorf("IncludeRepos = %v, want [owner/repo]", n.IncludeRepos)
	}
	if len(n.ExcludeReasons) != 2 || n.ExcludeReasons[0] != "subscribed" || n.ExcludeReasons[1] != "ci_activity" {
		t.Errorf("ExcludeReasons = %v, want [subscribed ci_activity]", n.ExcludeReasons)
	}
	if !n.UnreadOnly {
		t.Error("UnreadOnly = false, want true")
	}
	if !n.GitHub.ParticipatingOnly {
		t.Error("GitHub.ParticipatingOnly = false, want true")
	}
	if n.GitHub.SinceDays != 7 {
		t.Errorf("GitHub.SinceDays = %d, want 7", n.GitHub.SinceDays)
	}
	if n.MaxItems != 50 {
		t.Errorf("MaxItems = %d, want 50", n.MaxItems)
	}
	if n.PollInterval != 120 {
		t.Errorf("PollInterval = %d, want 120", n.PollInterval)
	}
	// This fixture sets both only_configured_repos: true and a non-empty
	// include_repos, which is flagged with exactly one warning -- include_repos
	// is overridden, not intersected, and the user is told so rather than left
	// to wonder why it had no effect. The exclude_reasons entries above are both
	// valid, so this is the only warning expected.
	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry (only_configured_repos + include_repos both set)", cfg.Warnings)
	}
	// Assert the content too, not just the count: a count-only check passes
	// if the intended warning disappears while some unrelated warning
	// appears in its place.
	if !strings.Contains(cfg.Warnings[0], "include_repos") || !strings.Contains(cfg.Warnings[0], "ignored") {
		t.Errorf("warning should say include_repos is ignored, got: %s", cfg.Warnings[0])
	}
}

func TestLoad_NotificationsBlock_MixedCaseKeys_Convention9(t *testing.T) {
	// Convention 9: viper lowercases all config keys on load. Mixed-case keys
	// in the YAML must still resolve — this pins that for the notifications
	// block's own (root-nested) keys.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  UNREAD_ONLY: true
  Exclude_Reasons:
    - subscribed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if !cfg.Notifications.UnreadOnly {
		t.Error("UnreadOnly = false, want true (mixed-case key UNREAD_ONLY should still resolve)")
	}
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed] (mixed-case key Exclude_Reasons should still resolve)", cfg.Notifications.ExcludeReasons)
	}
}

// TestLoad_NotificationsGitHubBlock_MixedCaseKeys_Convention9 is the untested
// half convention 9 calls out: mixed-case keys one level below the root
// notifications map, inside notifications.github. A resolver that only
// lowercases the top-level notifications map (and relies on mapstructure's
// own case folding for everything nested inside a struct field) would still
// pass the root-level test above while silently failing to bind a mixed-case
// key here if that assumption were ever wrong.
func TestLoad_NotificationsGitHubBlock_MixedCaseKeys_Convention9(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  github:
    Only_Configured_Repos: true
    PARTICIPATING_ONLY: true
    Since_Days: 5
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if !cfg.Notifications.GitHub.OnlyConfiguredRepos {
		t.Error("GitHub.OnlyConfiguredRepos = false, want true (mixed-case key Only_Configured_Repos should still resolve)")
	}
	if !cfg.Notifications.GitHub.ParticipatingOnly {
		t.Error("GitHub.ParticipatingOnly = false, want true (mixed-case key PARTICIPATING_ONLY should still resolve)")
	}
	if cfg.Notifications.GitHub.SinceDays != 5 {
		t.Errorf("GitHub.SinceDays = %d, want 5 (mixed-case key Since_Days should still resolve)", cfg.Notifications.GitHub.SinceDays)
	}
}

// TestLoad_NotificationsAzureSourcesBlock_MixedCaseKeys_Convention9 goes one
// level deeper still: notifications.azure.sources, three levels below the
// config root. This is the deepest nested map decision 13's shape has, and
// it is the case most likely to have been missed if the root's lowercasing
// were hand-rolled per level instead of applying uniformly.
func TestLoad_NotificationsAzureSourcesBlock_MixedCaseKeys_Convention9(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  azure:
    Lookback_Days: 21
    MIN_POLL_INTERVAL: 600
    sources:
      Review_Requested: true
      MENTIONED: false
      Assigned: true
      Ci_Failed: false
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	a := cfg.Notifications.Azure
	if a.LookbackDays != 21 {
		t.Errorf("Azure.LookbackDays = %d, want 21 (mixed-case key Lookback_Days should still resolve)", a.LookbackDays)
	}
	if a.MinPollInterval != 600 {
		t.Errorf("Azure.MinPollInterval = %d, want 600 (mixed-case key MIN_POLL_INTERVAL should still resolve)", a.MinPollInterval)
	}
	// Two of the four sources are explicitly false here, on purpose: task 11
	// defaults every source to true, so a fixture that sets all four to true
	// would pass even if the mixed-case keys below never resolved at all
	// (the default would produce the same "true" by coincidence). Setting
	// MENTIONED/Ci_Failed to false is the only way this test can tell "the
	// mixed-case key resolved to an explicit false" apart from "the key
	// never resolved and the field fell back to its default".
	if !a.Sources.ReviewRequested {
		t.Error("Azure.Sources.ReviewRequested = false, want true (mixed-case key Review_Requested should still resolve)")
	}
	if a.Sources.Mentioned {
		t.Error("Azure.Sources.Mentioned = true, want false (mixed-case key MENTIONED should resolve to its explicit false, not the true default)")
	}
	if !a.Sources.Assigned {
		t.Error("Azure.Sources.Assigned = false, want true (mixed-case key Assigned should still resolve)")
	}
	if a.Sources.CIFailed {
		t.Error("Azure.Sources.CIFailed = true, want false (mixed-case key Ci_Failed should resolve to its explicit false, not the true default)")
	}
}

// TestLoad_AzureSourceToggle_ExplicitFalse_OverridesDefaultTrue is the whole
// point of task 11's defaults-on design. All four source toggles default to
// true against bool's false zero value, so "the user explicitly disabled
// this source" and "the user never mentioned it" are indistinguishable in a
// plain bool unless SetDefault genuinely resolves through v.Unmarshal. A
// test that only checks the *defaults* (all true, nothing set) would pass
// against a broken implementation that hardcodes true and ignores the file
// entirely — this fixture sets exactly one source to false and leaves the
// other three unset, so it can only pass if the explicit false is honoured
// AND the unset three still fall back to their default.
func TestLoad_AzureSourceToggle_ExplicitFalse_OverridesDefaultTrue(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  azure:
    sources:
      review_requested: false
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	s := cfg.Notifications.Azure.Sources
	if s.ReviewRequested {
		t.Error("Sources.ReviewRequested = true, want false (explicit review_requested: false must override the true default)")
	}
	if !s.Mentioned {
		t.Error("Sources.Mentioned = false, want true (unset key must still fall back to the default)")
	}
	if !s.Assigned {
		t.Error("Sources.Assigned = false, want true (unset key must still fall back to the default)")
	}
	if !s.CIFailed {
		t.Error("Sources.CIFailed = false, want true (unset key must still fall back to the default)")
	}
}

// TestLoad_AzureSources_AllDisabled_IsLegal pins that turning off every
// source is a legal, empty-feed configuration, not a load or validation
// error — Validate() has no "at least one Azure source must be enabled"
// guard, and NotificationsAzureSourcesConfig's own doc comment says so.
func TestLoad_AzureSources_AllDisabled_IsLegal(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  azure:
    sources:
      review_requested: false
      mentioned: false
      assigned: false
      ci_failed: false
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() with every Azure source disabled = %v, want nil", err)
	}

	s := cfg.Notifications.Azure.Sources
	if s.ReviewRequested || s.Mentioned || s.Assigned || s.CIFailed {
		t.Errorf("Sources = %+v, want all four false", s)
	}
}

// TestLoad_AzureLookbackDays_Zero_FallsBackToDefault pins decision 13's
// closing note: zero is NOT "unbounded" for lookback_days the way it is for
// the shared/GitHub numeric keys. An explicit `lookback_days: 0` must land
// on DefaultAzureLookbackDays, the same value an absent key gets, rather
// than surviving as a literal 0 that Azure's sources could read as "every
// work item ever assigned".
func TestLoad_AzureLookbackDays_Zero_FallsBackToDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  azure:
    lookback_days: 0
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Notifications.Azure.LookbackDays != DefaultAzureLookbackDays {
		t.Errorf("Azure.LookbackDays = %d, want %d (explicit zero must fall back to the default, not be treated as unbounded)",
			cfg.Notifications.Azure.LookbackDays, DefaultAzureLookbackDays)
	}
}

// TestLoad_AzureLookbackDays_ClampedToOrphanTTL pins the boundary shape
// convention 13 asks for: exactly at the clamp (30) is left unchanged, one
// past it (31) is silently clamped down. Beyond azureLookbackDaysMax an item
// can be pruned from the local triage store (azdevops.orphanTTL) while
// still inside the query window and resurface as unread with nothing having
// actually happened to it.
//
// The "29, unchanged" row exists specifically to pin the threshold's exact
// value (30), not just the shape of the clamp: a clamp whose comparison
// mistakenly fires one day early (e.g. `> 25` instead of `> 30`) still
// passes the 30/31 rows above — 30 and 31 both land on the clamp target
// (30) either way, since clamping 30 to 30 is a no-op regardless of which
// threshold triggered it. 29 is far enough inside the true window that any
// threshold lower than 30 clamps it down and gets caught, while any correct
// or higher threshold leaves it alone.
func TestLoad_AzureLookbackDays_ClampedToOrphanTTL(t *testing.T) {
	tests := []struct {
		name string
		days int
		want int
	}{
		{name: "one below the clamp is unaffected", days: 29, want: 29},
		{name: "exactly at the clamp is unchanged", days: 30, want: 30},
		{name: "one past the clamp is pulled down to it", days: 31, want: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			content := fmt.Sprintf(`organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  azure:
    lookback_days: %d
`, tt.days)
			if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := LoadFrom(configPath)
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}

			if cfg.Notifications.Azure.LookbackDays != tt.want {
				t.Errorf("Azure.LookbackDays = %d, want %d", cfg.Notifications.Azure.LookbackDays, tt.want)
			}
		})
	}
}

func TestConfig_Validate_NotificationsRejectsNegative(t *testing.T) {
	base := func() *Config {
		return &Config{
			Organization:    "org",
			Projects:        []string{"p"},
			PollingInterval: 60,
			Theme:           "dark",
		}
	}

	// All three rows use -1 on purpose. MaxItems truncates only when it is >
	// 0, so -1 is precisely the value that would read as "unlimited" if a
	// guard were ever loosened from < 0 to, say, < -1 or == 0 — it is the
	// boundary worth pinning, not an arbitrarily large negative.
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "since_days negative",
			mutate:  func(c *Config) { c.Notifications.GitHub.SinceDays = -1 },
			wantErr: "since_days",
		},
		{
			name:    "max_items negative",
			mutate:  func(c *Config) { c.Notifications.MaxItems = -1 },
			wantErr: "max_items",
		},
		{
			name:    "poll_interval negative",
			mutate:  func(c *Config) { c.Notifications.PollInterval = -1 },
			wantErr: "poll_interval",
		},
		{
			name:    "azure lookback_days negative",
			mutate:  func(c *Config) { c.Notifications.Azure.LookbackDays = -1 },
			wantErr: "lookback_days",
		},
		{
			name:    "azure min_poll_interval negative",
			mutate:  func(c *Config) { c.Notifications.Azure.MinPollInterval = -1 },
			wantErr: "min_poll_interval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_NotificationsAcceptsZeroBounds(t *testing.T) {
	// Zero is the documented "no bound" default for all three — must not be
	// rejected by a >= 0 guard (only negatives are invalid).
	//
	// Azure.LookbackDays/MinPollInterval are included at zero too: Validate()
	// itself only rejects negatives, the same >= 0 guard as the other three.
	// LoadFrom is what turns a zero LookbackDays into the 14-day default
	// before Validate() ever sees it (TestLoad_AzureLookbackDays_Zero_FallsBackToDefault
	// pins that pipeline); Validate() called directly on a struct literal
	// (as here) has no such normalization and must not treat either as an
	// error on its own.
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Notifications: NotificationsConfig{
			MaxItems:     0,
			PollInterval: 0,
			GitHub:       NotificationsGitHubConfig{SinceDays: 0},
			Azure:        NotificationsAzureConfig{LookbackDays: 0, MinPollInterval: 0},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with zero-value notification bounds = %v, want nil", err)
	}
}

func TestConfig_Validate_NotificationsRejectsEmptyGlobEntries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name:   "exclude_repos empty entry",
			mutate: func(c *Config) { c.Notifications.ExcludeRepos = []string{"owner/repo", ""} },
		},
		{
			name:   "include_repos empty entry",
			mutate: func(c *Config) { c.Notifications.IncludeRepos = []string{""} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Organization:    "org",
				Projects:        []string{"p"},
				PollingInterval: 60,
				Theme:           "dark",
			}
			tt.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("Validate() = nil, want error for empty glob entry")
			}
		})
	}
}

func TestConfig_Validate_DisabledPanes_NotificationsAccepted(t *testing.T) {
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with disabled_panes: notifications = %v, want nil", err)
	}
	if cfg.IsPaneEnabled("notifications") {
		t.Error("IsPaneEnabled(notifications) = true, want false")
	}
}

func TestConfig_Validate_InvalidDisabledPane_MentionsNotifications(t *testing.T) {
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"bogus"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for invalid disabled pane")
	}
	if !strings.Contains(err.Error(), "notifications") {
		t.Errorf("error should mention 'notifications' among the valid pane names, got: %s", err.Error())
	}
}

// TestConfig_Validate_NotificationsOnly_Passes: a config where only
// notifications is enabled (the other three panes disabled) must pass
// Validate().
func TestConfig_Validate_NotificationsOnly_Passes(t *testing.T) {
	cfg := &Config{
		PollingInterval: 60,
		Theme:           "dark",
		GitHub:          GitHubConfig{Repos: []string{"owner/repo"}},
		DisabledPanes:   []string{"pullrequests", "workitems", "pipelines"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() for notifications-only config = %v, want nil", err)
	}
}

// --- Notifications counts as a remaining pane only when HasGitHub() is true.
// This coupling will need revisiting when a second notification-capable
// backend arrives. ---

// TestConfig_Validate_PaneGuard_NotificationsRequiresGitHub walks the three
// fixtures that together pin both conjuncts of `IsPaneEnabled("notifications")
// && HasGitHub()`:
//
//   - GitHub configured, other three panes disabled → valid (notifications is
//     the remaining pane);
//   - Azure-only, other three panes disabled → still rejected, because the
//     notifications tab hides on capability, and the message must explain the
//     GitHub coupling rather than repeat the old three-pane text verbatim;
//   - GitHub configured, ALL FOUR panes disabled → rejected.
//
// The third row is the one that distinguishes the conjunction from a bare
// HasGitHub(): the first two vary HasGitHub() while notifications stays
// enabled, so dropping the IsPaneEnabled("notifications") conjunct leaves
// them both green. Without it a GitHub config that explicitly turns off every
// pane would validate and the app would start with zero navigable tabs.
func TestConfig_Validate_PaneGuard_NotificationsRequiresGitHub(t *testing.T) {
	// oldThreePaneText is the message from before the GitHub coupling. Row 2
	// must not be it: repeating it verbatim explains nothing about why
	// notifications does not rescue an Azure-only config.
	const oldThreePaneText = "cannot disable all panes: at least one of 'pullrequests', 'workitems' or 'pipelines' must remain enabled"

	tests := []struct {
		name    string
		content string
		wantErr bool
		// wantErrContains are substrings every rejection message must carry.
		wantErrContains []string
		// forbidExactErr, when non-empty, must not be the whole message.
		forbidExactErr string
	}{
		{
			name: "GitHub configured, other three panes disabled",
			content: `polling_interval: 60
theme: dark
github:
  repos:
    - owner/repo
disabled_panes: pullrequests,workitems,pipelines
`,
			wantErr: false,
		},
		{
			name: "Azure-only, other three panes disabled",
			content: `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
disabled_panes: pullrequests,workitems,pipelines
`,
			wantErr:         true,
			wantErrContains: []string{"notifications", "GitHub"},
			forbidExactErr:  oldThreePaneText,
		},
		{
			name: "GitHub configured, all four panes disabled",
			content: `polling_interval: 60
theme: dark
github:
  repos:
    - owner/repo
disabled_panes: pullrequests,workitems,pipelines,notifications
`,
			wantErr:         true,
			wantErrContains: []string{"cannot disable all panes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := LoadFrom(configPath)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("LoadFrom() = %v, want nil — notifications is the only remaining pane and GitHub is configured", err)
				}
				if !cfg.HasGitHub() {
					t.Fatal("HasGitHub() = false, want true (test fixture invalid)")
				}
				return
			}

			if err == nil {
				t.Fatal("LoadFrom() = nil, want an error — this config leaves zero navigable tabs")
			}
			errMsg := err.Error()
			for _, want := range tt.wantErrContains {
				if !strings.Contains(errMsg, want) {
					t.Errorf("error should mention %q, got: %s", want, errMsg)
				}
			}
			if tt.forbidExactErr != "" && errMsg == tt.forbidExactErr {
				t.Errorf("error message is just the old three-pane text; must explain the GitHub/notifications coupling")
			}
		})
	}
}

// --- Unrecognised exclude_reasons entries warn and are dropped. ---

// wantElevenAcceptedReasonNames is a hardcoded, independent copy of the
// eleven accepted values — deliberately NOT generated from production code
// (never by calling acceptedNotificationReasons(), which would be
// tautological), so these tests cannot pass merely because the implementation
// and the test share the same (possibly wrong) source.
//
// The order is the enum declaration order, which is also the order
// acceptedNotificationReasons() emits. "unknown" is absent on purpose: it is
// the enum's twelfth value, reserved and rejected by ParseNotificationReason,
// so advertising it as accepted would tell the user to write a string the
// parser refuses.
var wantElevenAcceptedReasonNames = []string{
	"review_requested", "mentioned", "assigned", "authored", "commented",
	"state_changed", "ci_activity", "security_alert", "approval_requested",
	"subscribed", "other",
}

// acceptedValuesPrefix is the literal separator in the unrecognised-reason
// warning after which the accepted-values list begins.
const acceptedValuesPrefix = "accepted values: "

// TestLoad_ExcludeReasons_WarningListsExactlyTheElevenAcceptedValues pins the
// accepted-values segment of the warning to the eleven names, exactly, in enum
// order. A per-name strings.Contains check (as the test below does) cannot
// pin the exclusion of "unknown": in the unknown-value test the string
// "unknown" is already in the message as the *offending* value, so a
// containment assertion passes whether or not the accepted list also
// advertises it. Only an exact comparison of the segment kills that mutant.
func TestLoad_ExcludeReasons_WarningListsExactlyTheElevenAcceptedValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscibed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}

	msg := cfg.Warnings[0]
	parts := strings.SplitN(msg, acceptedValuesPrefix, 2)
	if len(parts) != 2 {
		t.Fatalf("warning %q does not contain %q", msg, acceptedValuesPrefix)
	}
	segment := parts[1]

	want := strings.Join(wantElevenAcceptedReasonNames, ", ")
	if segment != want {
		t.Errorf("accepted-values segment = %q, want %q", segment, want)
	}
	if got := len(strings.Split(segment, ", ")); got != 11 {
		t.Errorf("accepted-values segment lists %d values, want exactly 11", got)
	}
	if strings.Contains(segment, "unknown") {
		t.Errorf("accepted-values segment must not advertise the reserved 'unknown' value, got: %s", segment)
	}
}

func TestLoad_ExcludeReasons_UnrecognisedValue_WarnsAndDropped(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscribed
    - subscibed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on an unrecognised exclude_reasons entry: %v", err)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "subscibed") {
		t.Errorf("warning should name the offending value %q, got: %s", "subscibed", msg)
	}
	for _, name := range wantElevenAcceptedReasonNames {
		if !strings.Contains(msg, name) {
			t.Errorf("warning should list accepted value %q, got: %s", name, msg)
		}
	}

	// The valid entry is kept; the bad one is dropped so it can never act as "other".
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed] (bad entry dropped, good entry kept)", cfg.Notifications.ExcludeReasons)
	}
}

func TestLoad_ExcludeReasons_UnknownValue_Warns(t *testing.T) {
	// "unknown" is the enum's twelfth value but is explicitly not accepted in
	// config — it must warn exactly like any other typo.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - unknown
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on 'unknown' in exclude_reasons: %v", err)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	if !strings.Contains(cfg.Warnings[0], "unknown") {
		t.Errorf("warning should name 'unknown' as the offending value, got: %s", cfg.Warnings[0])
	}
	if len(cfg.Notifications.ExcludeReasons) != 0 {
		t.Errorf("ExcludeReasons = %v, want empty ('unknown' dropped)", cfg.Notifications.ExcludeReasons)
	}
}

// TestLoad_ExcludeReasons_TwoUnrecognisedValues_WarnOnceEach pins that
// warnings accumulate rather than collapsing to the last one. Every other
// fixture in this file has at most one bad entry, so replacing the append with
// an assignment would go unnoticed here — and the notifications pane renders
// the whole slice, so a collapse-to-one regression would silently hide typos
// the user needs to see.
func TestLoad_ExcludeReasons_TwoUnrecognisedValues_WarnOnceEach(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscibed
    - ci_activty
    - subscribed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on unrecognised exclude_reasons entries: %v", err)
	}

	if len(cfg.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want exactly 2 (one per unrecognised entry)", cfg.Warnings)
	}
	// Each warning names its own offending value and only its own — a single
	// warning listing both, or two copies of the same one, is a regression.
	if !strings.Contains(cfg.Warnings[0], "subscibed") || strings.Contains(cfg.Warnings[0], "ci_activty") {
		t.Errorf("Warnings[0] should name only \"subscibed\", got: %s", cfg.Warnings[0])
	}
	if !strings.Contains(cfg.Warnings[1], "ci_activty") || strings.Contains(cfg.Warnings[1], "subscibed") {
		t.Errorf("Warnings[1] should name only \"ci_activty\", got: %s", cfg.Warnings[1])
	}
	// The one good entry survives both drops.
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed]", cfg.Notifications.ExcludeReasons)
	}
}

func TestLoad_ExcludeReasons_AllValid_NoWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscribed
    - other
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for all-valid exclude_reasons", cfg.Warnings)
	}
	if len(cfg.Notifications.ExcludeReasons) != 2 {
		t.Errorf("ExcludeReasons = %v, want both entries kept", cfg.Notifications.ExcludeReasons)
	}
}

// --- Malformed exclude_repos / include_repos globs are dropped at load with a
// warning naming the pattern and the key; a whitespace-only entry is a hard
// Validate() error, extending the existing empty-entry check rather than
// becoming a new warning class. ---

func TestLoad_ExcludeRepos_BadPattern_DroppedWithWarning_ValidEntriesSurvive(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_repos:
    - "owner/good"
    - "[bad"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on a malformed exclude_repos pattern: %v", err)
	}

	if len(cfg.Notifications.ExcludeRepos) != 1 || cfg.Notifications.ExcludeRepos[0] != "owner/good" {
		t.Fatalf("ExcludeRepos = %v, want [owner/good] ([bad dropped, owner/good kept)", cfg.Notifications.ExcludeRepos)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "[bad") {
		t.Errorf("warning should name the offending pattern %q, got: %s", "[bad", msg)
	}
	if !strings.Contains(msg, "notifications.exclude_repos") {
		t.Errorf("warning should name the key notifications.exclude_repos, got: %s", msg)
	}
	// The pattern and the error state the cause; the warning must also state
	// the consequence, otherwise the user is told a pattern was dropped but
	// not what their feed will now do.
	if !strings.Contains(msg, "nothing is excluded by it") {
		t.Errorf("warning should say nothing is excluded by the dropped pattern, got: %s", msg)
	}
}

func TestLoad_IncludeRepos_BadPattern_DroppedWithWarning_ValidEntriesSurvive(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  include_repos:
    - "[bad"
    - "owner/good"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on a malformed include_repos pattern: %v", err)
	}

	if len(cfg.Notifications.IncludeRepos) != 1 || cfg.Notifications.IncludeRepos[0] != "owner/good" {
		t.Fatalf("IncludeRepos = %v, want [owner/good] ([bad dropped, owner/good kept)", cfg.Notifications.IncludeRepos)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "[bad") {
		t.Errorf("warning should name the offending pattern %q, got: %s", "[bad", msg)
	}
	if !strings.Contains(msg, "notifications.include_repos") {
		t.Errorf("warning should name the key notifications.include_repos, got: %s", msg)
	}
	// A compilable pattern survived, so the selection still narrows the feed:
	// the consequence clause must say that, not that the whole inbox is shown.
	if !strings.Contains(msg, "the remaining include patterns still apply") {
		t.Errorf("warning should say the surviving include patterns still apply, got: %s", msg)
	}
}

// TestLoad_IncludeRepos_AllPatternsBad_WarningSaysWholeInboxIsShown pins the
// other consequence clause: with no compilable include pattern left the list
// is empty, and an empty include_repos selects everything, so the user must be
// told their filter has stopped narrowing anything rather than being left to
// guess.
func TestLoad_IncludeRepos_AllPatternsBad_WarningSaysWholeInboxIsShown(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  include_repos:
    - "[bad"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on a malformed include_repos pattern: %v", err)
	}
	if len(cfg.Notifications.IncludeRepos) != 0 {
		t.Fatalf("IncludeRepos = %v, want empty (the only pattern was malformed)", cfg.Notifications.IncludeRepos)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	if !strings.Contains(cfg.Warnings[0], "the whole inbox is shown") {
		t.Errorf("warning should say the whole inbox is shown, got: %s", cfg.Warnings[0])
	}
}

func TestLoad_OnlyConfiguredRepos_WithIncludeRepos_Warns(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  include_repos:
    - "owner/repo"
  github:
    only_configured_repos: true
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	// The qualified spelling is the point of the change: a message that only
	// contained "only_configured_repos" would pass just as well against the
	// pre-restructure flat key (notifications.only_configured_repos), which
	// this test is meant to distinguish from the nested
	// notifications.github.only_configured_repos. include_repos stays
	// unqualified in the assertion because it genuinely is still top-level
	// (decision 13's second note) -- it is not meant to gain a "github."
	// prefix, so it is not asserted as qualified.
	if !strings.Contains(msg, "notifications.github.only_configured_repos") || !strings.Contains(msg, "include_repos") {
		t.Errorf("warning should name both the qualified notifications.github.only_configured_repos and include_repos, got: %s", msg)
	}
	// include_repos itself is not mutated by the warning -- only ignored at
	// filter time, so it should still be present in the config.
	if len(cfg.Notifications.IncludeRepos) != 1 || cfg.Notifications.IncludeRepos[0] != "owner/repo" {
		t.Errorf("IncludeRepos = %v, want [owner/repo] (warned about, not dropped)", cfg.Notifications.IncludeRepos)
	}
}

func TestLoad_WhitespaceOnlyGlobEntry_IsLoadError(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"exclude_repos", "exclude_repos"},
		{"include_repos", "include_repos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			content := "organization: test-org\n" +
				"projects:\n  - alpha\n" +
				"polling_interval: 60\n" +
				"theme: dark\n" +
				"notifications:\n" +
				"  " + tt.key + ":\n" +
				"    - \"   \"\n"
			if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := LoadFrom(configPath)
			if err == nil {
				t.Fatalf("LoadFrom() = nil error, want error for whitespace-only notifications.%s entry", tt.key)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error should mention %q, got: %s", tt.key, err.Error())
			}
		})
	}
}

func TestLoad_Warnings_EmptyForCleanConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Warnings != nil && len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for a clean config", cfg.Warnings)
	}
}

// TestLoad_FlatMovedGitHubKeys_AreNotHonoured pins decision 14 of the
// phase-2 spec: there is no migration shim and no deprecation warning for
// the three keys decision 13 moved from notifications.* to
// notifications.github.*. A flat notifications.participating_only (etc.) is
// simply an unrecognised key to the current struct shape -- mapstructure
// silently drops it during Unmarshal because NotificationsConfig carries no
// field tagged "participating_only" any more, only NotificationsGitHubConfig
// does, under "github.participating_only". This test would fail loudly (the
// booleans would read true, the int would read 14) if a shim were ever added
// back that reads the flat form into the nested struct.
func TestLoad_FlatMovedGitHubKeys_AreNotHonoured(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  participating_only: true
  only_configured_repos: true
  since_days: 14
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Notifications.GitHub.ParticipatingOnly {
		t.Error("GitHub.ParticipatingOnly = true, want false -- the flat notifications.participating_only key must not be honoured")
	}
	if cfg.Notifications.GitHub.OnlyConfiguredRepos {
		t.Error("GitHub.OnlyConfiguredRepos = true, want false -- the flat notifications.only_configured_repos key must not be honoured")
	}
	if cfg.Notifications.GitHub.SinceDays != 0 {
		t.Errorf("GitHub.SinceDays = %d, want 0 -- the flat notifications.since_days key must not be honoured", cfg.Notifications.GitHub.SinceDays)
	}
	// Decision 14 also rules out a deprecation warning, not just a shim: the
	// flat keys must be silently ignored, never flagged.
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty -- decision 14 forbids a deprecation warning for the moved keys", cfg.Warnings)
	}
}
