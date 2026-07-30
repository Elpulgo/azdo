package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Task 9: notifications config block ---
//
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
	if n.OnlyConfiguredRepos {
		t.Error("OnlyConfiguredRepos = true by default; want false")
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
	if n.ParticipatingOnly {
		t.Error("ParticipatingOnly = true by default; want false")
	}
	if n.SinceDays != 0 {
		t.Errorf("SinceDays = %d, want 0", n.SinceDays)
	}
	if n.MaxItems != 0 {
		t.Errorf("MaxItems = %d, want 0", n.MaxItems)
	}
	if n.PollInterval != 0 {
		t.Errorf("PollInterval = %d, want 0", n.PollInterval)
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
	if n.OnlyConfiguredRepos || n.UnreadOnly || n.ParticipatingOnly {
		t.Errorf("expected all bools false for empty block, got %+v", n)
	}
	if len(n.ExcludeRepos) != 0 || len(n.IncludeRepos) != 0 || len(n.ExcludeReasons) != 0 {
		t.Errorf("expected all lists empty for empty block, got %+v", n)
	}
	if n.SinceDays != 0 || n.MaxItems != 0 || n.PollInterval != 0 {
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
  only_configured_repos: true
  exclude_repos:
    - "spammy/*"
    - "owner/noisy-repo"
  include_repos:
    - "owner/repo"
  exclude_reasons:
    - subscribed
    - ci_activity
  unread_only: true
  participating_only: true
  since_days: 7
  max_items: 50
  poll_interval: 120
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if !n.OnlyConfiguredRepos {
		t.Error("OnlyConfiguredRepos = false, want true")
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
	if !n.ParticipatingOnly {
		t.Error("ParticipatingOnly = false, want true")
	}
	if n.SinceDays != 7 {
		t.Errorf("SinceDays = %d, want 7", n.SinceDays)
	}
	if n.MaxItems != 50 {
		t.Errorf("MaxItems = %d, want 50", n.MaxItems)
	}
	if n.PollInterval != 120 {
		t.Errorf("PollInterval = %d, want 120", n.PollInterval)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty (both exclude_reasons entries valid)", cfg.Warnings)
	}
}

func TestLoad_NotificationsBlock_MixedCaseKeys_Convention9(t *testing.T) {
	// Convention 9: viper lowercases all config keys on load. Mixed-case keys
	// in the YAML must still resolve — this pins that for the notifications
	// block specifically, since it is new in this task.
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
  Only_Configured_Repos: true
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
	if !cfg.Notifications.OnlyConfiguredRepos {
		t.Error("OnlyConfiguredRepos = false, want true (mixed-case key Only_Configured_Repos should still resolve)")
	}
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed] (mixed-case key Exclude_Reasons should still resolve)", cfg.Notifications.ExcludeReasons)
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

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "since_days negative",
			mutate:  func(c *Config) { c.Notifications.SinceDays = -1 },
			wantErr: "since_days",
		},
		{
			name:    "max_items negative",
			mutate:  func(c *Config) { c.Notifications.MaxItems = -5 },
			wantErr: "max_items",
		},
		{
			name:    "poll_interval negative",
			mutate:  func(c *Config) { c.Notifications.PollInterval = -30 },
			wantErr: "poll_interval",
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
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Notifications: NotificationsConfig{
			SinceDays:    0,
			MaxItems:     0,
			PollInterval: 0,
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

// TestConfig_Validate_NotificationsOnly_Passes covers decision 22 directly:
// a config where only notifications is enabled (the other three panes
// disabled) must pass Validate().
func TestConfig_Validate_NotificationsOnly_Passes(t *testing.T) {
	cfg := &Config{
		PollingInterval: 60,
		Theme:           "dark",
		GitHub:          GitHubConfig{Repos: []string{"owner/repo"}},
		DisabledPanes:   []string{"pullrequests", "workitems", "pipelines"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() for notifications-only config = %v, want nil (decision 22)", err)
	}
}

// --- Decision 47: notifications counts as a remaining pane only when
// HasGitHub() is true (phase-1-only coupling). ---

func TestConfig_Validate_PaneGuard_Decision47_GitHubConfigured_OthersDisabled_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `polling_interval: 60
theme: dark
github:
  repos:
    - owner/repo
disabled_panes: pullrequests,workitems,pipelines
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should succeed: GitHub configured + notifications is the only remaining pane, got: %v", err)
	}
	if !cfg.HasGitHub() {
		t.Fatal("HasGitHub() = false, want true (test fixture invalid)")
	}
}

func TestConfig_Validate_PaneGuard_Decision47_AzureOnly_OthersDisabled_StillRejected(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
disabled_panes: pullrequests,workitems,pipelines
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadFrom(configPath)
	if err == nil {
		t.Fatal("LoadFrom() should fail: Azure-only config with the other three panes disabled leaves zero navigable tabs (notifications tab hides on capability, decision 11)")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "notifications") {
		t.Errorf("error should explain the notifications/GitHub coupling, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "GitHub") {
		t.Errorf("error should name GitHub as the missing requirement, got: %s", errMsg)
	}
	// Must not be merely the old three-pane text repeated verbatim with no
	// explanation of why notifications doesn't rescue this config.
	if errMsg == "cannot disable all panes: at least one of 'pullrequests', 'workitems' or 'pipelines' must remain enabled" {
		t.Error("error message is just the old three-pane text; must explain the GitHub/notifications coupling")
	}
}

// --- Decision 26: unrecognised exclude_reasons entries warn and are dropped. ---

// wantElevenAcceptedReasonNames is a hardcoded, independent copy of the
// eleven accepted values from the spec's Config shape section — deliberately
// NOT generated from production code, so this test cannot pass merely
// because the implementation and the test share the same (possibly wrong)
// source.
var wantElevenAcceptedReasonNames = []string{
	"review_requested", "mentioned", "assigned", "authored", "commented",
	"state_changed", "ci_activity", "security_alert", "subscribed",
	"approval_requested", "other",
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
		t.Fatalf("LoadFrom() should not fail on an unrecognised exclude_reasons entry (decision 26): %v", err)
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
	// config (decision 26) — it must warn exactly like any other typo.
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
