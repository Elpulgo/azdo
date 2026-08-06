package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestConfigSave tests saving theme changes to config
func TestConfigSave(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config
	initialConfig := `organization: testorg
projects:
  - testproject
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load config
	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Theme != "dark" {
		t.Errorf("Expected theme 'dark', got '%s'", cfg.Theme)
	}

	// Change theme
	cfg.Theme = "gruvbox"

	// Save config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Reload config to verify changes were saved
	reloadedCfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if reloadedCfg.Theme != "gruvbox" {
		t.Errorf("Expected saved theme 'gruvbox', got '%s'", reloadedCfg.Theme)
	}

	// Verify other fields are preserved
	if reloadedCfg.Organization != "testorg" {
		t.Errorf("Expected organization 'testorg', got '%s'", reloadedCfg.Organization)
	}
	if len(reloadedCfg.Projects) != 1 || reloadedCfg.Projects[0] != "testproject" {
		t.Errorf("Expected projects ['testproject'], got %v", reloadedCfg.Projects)
	}
	if reloadedCfg.PollingInterval != 60 {
		t.Errorf("Expected polling_interval 60, got %d", reloadedCfg.PollingInterval)
	}
}

// TestConfigUpdateTheme tests the UpdateTheme convenience method
func TestConfigUpdateTheme(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config
	initialConfig := `organization: testorg
projects:
  - testproject
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load config
	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Update theme
	if err := cfg.UpdateTheme("nord"); err != nil {
		t.Fatalf("Failed to update theme: %v", err)
	}

	// Reload to verify
	reloadedCfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if reloadedCfg.Theme != "nord" {
		t.Errorf("Expected updated theme 'nord', got '%s'", reloadedCfg.Theme)
	}
}

// TestConfigSaveValidation tests that validation happens before save
func TestConfigSaveValidation(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config
	initialConfig := `organization: testorg
projects:
  - testproject
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load config
	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set invalid polling interval
	cfg.PollingInterval = -1

	// Save should fail validation
	err = cfg.Save()
	if err == nil {
		t.Error("Expected save to fail with invalid polling_interval")
	}

	// Set invalid theme
	cfg.PollingInterval = 60
	cfg.Theme = ""

	// Save should fail validation
	err = cfg.Save()
	if err == nil {
		t.Error("Expected save to fail with empty theme")
	}
}

// TestConfigUpdateThemeValidation tests UpdateTheme with empty theme name
func TestConfigUpdateThemeValidation(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config
	initialConfig := `organization: testorg
projects:
  - testproject
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load config
	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Update with empty theme should fail
	err = cfg.UpdateTheme("")
	if err == nil {
		t.Error("Expected UpdateTheme to fail with empty theme name")
	}

	// Theme should not have changed
	if cfg.Theme != "dark" {
		t.Errorf("Expected theme to remain 'dark', got '%s'", cfg.Theme)
	}
}

// TestConfigSave_PreservesMetricsAndNotifications is a regression test.
// Save() only ever calls v.Set() for organization, projects,
// polling_interval, theme, disabled_panes, terms and github (config.go:718) --
// it never sets "metrics" or "notifications". Those two sections survive a
// Save() only because of the v.ReadInConfig() round-trip at config.go:744-748,
// which re-reads the existing file into the same viper instance before the
// explicit Set() calls overlay the fields Save() does manage. Delete that
// round-trip and the very next theme change silently wipes both sections.
//
// Per convention 17, the fixture is written to disk and loaded through
// LoadFrom(filepath.Join(t.TempDir(), "config.yaml")) -- never a bare Config{}
// literal, since a literal has no serialised form to round-trip and so cannot
// detect this failure mode at all. cfg.configPath is set by LoadFrom to a path
// inside t.TempDir(), and Save() only falls back to GetPath() (the real
// ~/.config/azdo-tui/config.yaml) when configPath is empty -- since every
// fixture here is loaded, not constructed, that fallback is unreachable and
// this test cannot touch the developer's real config.
func TestConfigSave_PreservesMetricsAndNotifications(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	initialConfig := `organization: testorg
projects:
  - testproject
polling_interval: 60
theme: dark
metrics:
  enabled: true
  interval_days: 21
  active_stale_days: 5
  rft_stale_days: 3
  wip_limit: 4
  run_one_shot_backfill: true
  states:
    active: "In Progress"
    ready_for_test: "In Test"
    closed: "Done"
  state_labels:
    active: "IP"
    ready_for_test: "RFT"
    closed: "CL"
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
  since_days: 14
  max_items: 25
  poll_interval: 90
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load through LoadFrom -- never a struct literal (convention 17). This
	// also sets cfg.configPath to the temp path, so Save() below cannot fall
	// through to GetPath() and reach a real user config.
	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	wantMetrics := MetricsConfig{
		Enabled:            true,
		IntervalDays:       21,
		ActiveStaleDays:    5,
		RFTStaleDays:       3,
		WIPLimit:           4,
		RunOneShotBackfill: true,
		States: MetricsStates{
			Active:       "In Progress",
			ReadyForTest: "In Test",
			Closed:       "Done",
		},
		StateLabels: MetricsStates{
			Active:       "IP",
			ReadyForTest: "RFT",
			Closed:       "CL",
		},
	}
	wantNotifications := NotificationsConfig{
		OnlyConfiguredRepos: true,
		ExcludeRepos:        []string{"spammy/*", "owner/noisy-repo"},
		IncludeRepos:        []string{"owner/repo"},
		ExcludeReasons:      []string{"subscribed", "ci_activity"},
		UnreadOnly:          true,
		ParticipatingOnly:   true,
		SinceDays:           14,
		MaxItems:            25,
		PollInterval:        90,
	}

	// Sanity-check the fixture actually parsed as seeded, before mutating
	// anything. Without this, a typo'd fixture key that loads as the Go zero
	// value would make the post-Save "survives" assertions below vacuously
	// true (zero compared to zero).
	if !reflect.DeepEqual(cfg.Metrics, wantMetrics) {
		t.Fatalf("fixture invalid: cfg.Metrics after initial load = %+v, want %+v", cfg.Metrics, wantMetrics)
	}
	if !reflect.DeepEqual(cfg.Notifications, wantNotifications) {
		t.Fatalf("fixture invalid: cfg.Notifications after initial load = %+v, want %+v", cfg.Notifications, wantNotifications)
	}

	// Change only the theme -- Save() must not touch metrics or notifications
	// at all, since it never Sets() either section explicitly.
	cfg.Theme = "gruvbox"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	reloaded, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	// The mutation must actually take effect -- otherwise this test could
	// pass by Save() being a no-op that leaves the whole file untouched.
	if reloaded.Theme != "gruvbox" {
		t.Fatalf("Theme after Save() = %q, want %q", reloaded.Theme, "gruvbox")
	}

	// Every seeded metrics value must survive with its exact value, not just
	// the block being present/non-empty.
	if !reflect.DeepEqual(reloaded.Metrics, wantMetrics) {
		t.Errorf("Metrics after theme-only Save() = %+v, want unchanged %+v", reloaded.Metrics, wantMetrics)
	}

	// Every seeded notifications value must survive with its exact value.
	if !reflect.DeepEqual(reloaded.Notifications, wantNotifications) {
		t.Errorf("Notifications after theme-only Save() = %+v, want unchanged %+v", reloaded.Notifications, wantNotifications)
	}
}

// TestConfigSave_PreservesKeysOutsideTheConfigStruct is the general form of
// what TestConfigSave_PreservesMetricsAndNotifications pins for two specific
// sections: Save() must not delete a key just because the Config struct has no
// field for it. Save()'s own doc comment promises this for "navigation state,
// future additions" as well as metrics.
//
// It exists as a separate test because it is the only assertion here that can
// see such a key at all. Every other check reads the reloaded *Config*, so a
// section absent from the struct is invisible to it — it would survive or be
// destroyed with the suite equally green. This one re-reads the raw YAML.
//
// Practical stakes: this is what makes it safe to hand-add a key the running
// binary predates, and what protects a config written by a newer azdo-tui from
// an older one. Only comments are expected to be lost (viper rewrites the file
// rather than patching it), which is the one loss the user accepted explicitly.
func TestConfigSave_PreservesKeysOutsideTheConfigStruct(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	// some_future_section has no counterpart anywhere in Config. Scalar and
	// nested values are both seeded: viper flattens on read, so a round-trip
	// that mishandled nesting would drop the child keys while keeping the
	// parent, and a top-level-only fixture would not catch it.
	initial := `organization: testorg
projects:
  - testproject
theme: dark
some_future_section:
  nested_key: keepme
  count: 7
`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	cfg.Theme = "gruvbox"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to re-read config: %v", err)
	}
	got := string(raw)

	// The mutation must have landed, or an untouched file would pass trivially.
	if !strings.Contains(got, "gruvbox") {
		t.Fatalf("theme change did not reach the file; contents:\n%s", got)
	}

	for _, want := range []string{"some_future_section", "nested_key", "keepme", "count"} {
		if !strings.Contains(got, want) {
			t.Errorf("Save() dropped %q, which the Config struct does not model; contents:\n%s", want, got)
		}
	}
}
