package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setTestHome sets the appropriate home directory environment variable for the current OS
func setTestHome(t *testing.T, dir string) func() {
	var oldValue string
	var envVar string

	if runtime.GOOS == "windows" {
		envVar = "USERPROFILE"
		oldValue = os.Getenv(envVar)
		os.Setenv(envVar, dir)
	} else {
		envVar = "HOME"
		oldValue = os.Getenv(envVar)
		os.Setenv(envVar, dir)
	}

	return func() {
		if oldValue != "" {
			os.Setenv(envVar, oldValue)
		} else {
			os.Unsetenv(envVar)
		}
	}
}

func TestLoad_ConfigFileNotFound(t *testing.T) {
	// Create a temporary config directory
	tempDir := t.TempDir()

	// Set HOME to temp directory for testing
	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	// Load config (no config file exists, should return error)
	cfg, err := Load()
	if err == nil {
		t.Fatal("Load() should fail when config file is not found")
	}

	if cfg != nil {
		t.Error("Expected cfg to be nil when config file is not found")
	}

	// Verify error message is not empty and contains useful information
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Expected error message to contain information about missing config")
	}

	// The error should mention "config.yaml"
	if !strings.Contains(errMsg, "config.yaml") {
		t.Errorf("Expected error message to mention 'config.yaml', got: %s", errMsg)
	}
}

func TestLoadFrom_MissingFile_ReturnsConfigNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	missingPath := filepath.Join(tmpDir, "config.yaml")

	_, err := LoadFrom(missingPath)
	if err == nil {
		t.Fatal("LoadFrom() should fail when config file is missing")
	}

	// Should be a sentinel ErrConfigNotFound
	if !errors.Is(err, ErrConfigNotFound) {
		t.Errorf("expected ErrConfigNotFound, got: %v", err)
	}

	errMsg := err.Error()

	// Should mention the expected file path
	if !strings.Contains(errMsg, missingPath) {
		t.Errorf("error should contain config path %q, got: %s", missingPath, errMsg)
	}

	// Should NOT contain the raw "no such file or directory" OS error
	if strings.Contains(strings.ToLower(errMsg), "no such file or directory") {
		t.Errorf("error should not expose raw OS error, got: %s", errMsg)
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Create a temporary config directory
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config", "azdo-tui")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Write a test config file with projects list
	configFile := filepath.Join(configDir, "config.yaml")
	configContent := `organization: test-org
projects:
  - project-alpha
  - project-beta
polling_interval: 120
theme: dark
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set HOME to temp directory for testing
	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	// Load config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Test loaded values
	if cfg.Organization != "test-org" {
		t.Errorf("Expected Organization to be 'test-org', got %s", cfg.Organization)
	}

	if len(cfg.Projects) != 2 {
		t.Fatalf("Expected 2 projects, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0] != "project-alpha" {
		t.Errorf("Expected Projects[0] to be 'project-alpha', got %s", cfg.Projects[0])
	}
	if cfg.Projects[1] != "project-beta" {
		t.Errorf("Expected Projects[1] to be 'project-beta', got %s", cfg.Projects[1])
	}

	if cfg.PollingInterval != 120 {
		t.Errorf("Expected PollingInterval to be 120, got %d", cfg.PollingInterval)
	}

	if cfg.Theme != "dark" {
		t.Errorf("Expected Theme to be 'dark', got %s", cfg.Theme)
	}
}

func TestLoad_BackwardCompatSingleProject(t *testing.T) {
	// Old config format with single "project:" field should still work
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config", "azdo-tui")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configFile := filepath.Join(configDir, "config.yaml")
	configContent := `organization: test-org
project: legacy-project
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(cfg.Projects) != 1 {
		t.Fatalf("Expected 1 project from backward compat, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0] != "legacy-project" {
		t.Errorf("Expected Projects[0] to be 'legacy-project', got %s", cfg.Projects[0])
	}
}

func TestConfig_IsMultiProject(t *testing.T) {
	tests := []struct {
		name     string
		projects []string
		want     bool
	}{
		{"single project", []string{"alpha"}, false},
		{"multiple projects", []string{"alpha", "beta"}, true},
		{"three projects", []string{"a", "b", "c"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Projects: tt.projects, PollingInterval: 60, Theme: "dark"}
			if got := cfg.IsMultiProject(); got != tt.want {
				t.Errorf("IsMultiProject() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPath_ReturnsExpectedPath(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Set HOME to temp directory for testing
	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	path, err := GetPath()
	if err != nil {
		t.Fatalf("GetPath() failed: %v", err)
	}

	expectedPath := filepath.Join(tempDir, ".config", "azdo-tui", "config.yaml")
	if path != expectedPath {
		t.Errorf("GetPath() = %s, want %s", path, expectedPath)
	}
}

func TestParseProjects_StringList(t *testing.T) {
	// Simple string list: display names should equal API names
	raw := []interface{}{"proj-a", "proj-b"}
	projects, displayNames := parseProjects(raw)

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0] != "proj-a" || projects[1] != "proj-b" {
		t.Errorf("projects = %v, want [proj-a proj-b]", projects)
	}
	if len(displayNames) != 0 {
		t.Errorf("expected empty displayNames for plain strings, got %v", displayNames)
	}
}

func TestParseProjects_ObjectList(t *testing.T) {
	// Object entries with name + display_name
	raw := []interface{}{
		map[interface{}]interface{}{
			"name":         "ugly-api-name",
			"display_name": "My Project",
		},
		map[interface{}]interface{}{
			"name":         "another-api",
			"display_name": "Another",
		},
	}
	projects, displayNames := parseProjects(raw)

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0] != "ugly-api-name" || projects[1] != "another-api" {
		t.Errorf("projects = %v", projects)
	}
	if displayNames["ugly-api-name"] != "My Project" {
		t.Errorf("displayNames[ugly-api-name] = %q, want %q", displayNames["ugly-api-name"], "My Project")
	}
	if displayNames["another-api"] != "Another" {
		t.Errorf("displayNames[another-api] = %q, want %q", displayNames["another-api"], "Another")
	}
}

func TestParseProjects_MixedList(t *testing.T) {
	// Mix of strings and objects
	raw := []interface{}{
		"simple-project",
		map[interface{}]interface{}{
			"name":         "ugly-api-name",
			"display_name": "Friendly Name",
		},
	}
	projects, displayNames := parseProjects(raw)

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0] != "simple-project" || projects[1] != "ugly-api-name" {
		t.Errorf("projects = %v", projects)
	}
	// Only the object entry should have a display name
	if len(displayNames) != 1 {
		t.Errorf("expected 1 displayName entry, got %d", len(displayNames))
	}
	if displayNames["ugly-api-name"] != "Friendly Name" {
		t.Errorf("displayNames[ugly-api-name] = %q", displayNames["ugly-api-name"])
	}
}

func TestParseProjects_ObjectWithoutDisplayName(t *testing.T) {
	// Object entry with only name (no display_name) — display name defaults to API name
	raw := []interface{}{
		map[interface{}]interface{}{
			"name": "just-a-name",
		},
	}
	projects, displayNames := parseProjects(raw)

	if len(projects) != 1 || projects[0] != "just-a-name" {
		t.Errorf("projects = %v, want [just-a-name]", projects)
	}
	if len(displayNames) != 0 {
		t.Errorf("expected empty displayNames when display_name not set, got %v", displayNames)
	}
}

func TestParseProjects_StringMapKeys(t *testing.T) {
	// Viper sometimes returns map[string]interface{} instead of map[interface{}]interface{}
	raw := []interface{}{
		map[string]interface{}{
			"name":         "api-name",
			"display_name": "Display",
		},
	}
	projects, displayNames := parseProjects(raw)

	if len(projects) != 1 || projects[0] != "api-name" {
		t.Errorf("projects = %v, want [api-name]", projects)
	}
	if displayNames["api-name"] != "Display" {
		t.Errorf("displayNames[api-name] = %q, want %q", displayNames["api-name"], "Display")
	}
}

func TestConfig_DisplayNameFor(t *testing.T) {
	cfg := Config{
		Projects:     []string{"ugly-api", "simple"},
		DisplayNames: map[string]string{"ugly-api": "Friendly"},
	}

	// Project with display name
	if got := cfg.DisplayNameFor("ugly-api"); got != "Friendly" {
		t.Errorf("DisplayNameFor(ugly-api) = %q, want %q", got, "Friendly")
	}

	// Project without display name — returns API name
	if got := cfg.DisplayNameFor("simple"); got != "simple" {
		t.Errorf("DisplayNameFor(simple) = %q, want %q", got, "simple")
	}

	// Unknown project — returns as-is
	if got := cfg.DisplayNameFor("unknown"); got != "unknown" {
		t.Errorf("DisplayNameFor(unknown) = %q, want %q", got, "unknown")
	}
}

func TestLoad_WithDisplayNames(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `organization: test-org
projects:
  - name: ugly-api-project
    display_name: My Project
  - simple-project
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if len(cfg.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0] != "ugly-api-project" {
		t.Errorf("Projects[0] = %q, want %q", cfg.Projects[0], "ugly-api-project")
	}
	if cfg.Projects[1] != "simple-project" {
		t.Errorf("Projects[1] = %q, want %q", cfg.Projects[1], "simple-project")
	}
	if cfg.DisplayNameFor("ugly-api-project") != "My Project" {
		t.Errorf("DisplayNameFor(ugly-api-project) = %q, want %q", cfg.DisplayNameFor("ugly-api-project"), "My Project")
	}
	if cfg.DisplayNameFor("simple-project") != "simple-project" {
		t.Errorf("DisplayNameFor(simple-project) = %q, want %q", cfg.DisplayNameFor("simple-project"), "simple-project")
	}
}

func TestLoad_PlainStringListStillWorks(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `organization: test-org
projects:
  - alpha
  - beta
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if len(cfg.Projects) != 2 || cfg.Projects[0] != "alpha" || cfg.Projects[1] != "beta" {
		t.Errorf("Projects = %v, want [alpha beta]", cfg.Projects)
	}
	// No display names set
	if cfg.DisplayNameFor("alpha") != "alpha" {
		t.Errorf("DisplayNameFor(alpha) = %q, want %q", cfg.DisplayNameFor("alpha"), "alpha")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{"test-project"},
				PollingInterval: 60,
				Theme:           "light",
			},
			wantErr: false,
		},
		{
			name: "valid config multiple projects",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{"alpha", "beta"},
				PollingInterval: 60,
				Theme:           "light",
			},
			wantErr: false,
		},
		{
			name: "empty projects list",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{},
				PollingInterval: 60,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "nil projects list",
			config: Config{
				Organization:    "test-org",
				Projects:        nil,
				PollingInterval: 60,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "project with empty name",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{"alpha", ""},
				PollingInterval: 60,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "invalid polling interval - zero",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{"test-project"},
				PollingInterval: 0,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "invalid polling interval - negative",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{"test-project"},
				PollingInterval: -10,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "empty theme",
			config: Config{
				Organization:    "test-org",
				Projects:        []string{"test-project"},
				PollingInterval: 60,
				Theme:           "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_NoProjects_ErrorContainsSetupGuidance(t *testing.T) {
	cfg := Config{
		Organization:    "test-org",
		Projects:        []string{},
		PollingInterval: 60,
		Theme:           "dark",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty projects")
	}

	errMsg := err.Error()

	if !strings.Contains(errMsg, "projects") {
		t.Error("error should mention 'projects' field")
	}
	if !strings.Contains(errMsg, "config.yaml") {
		t.Error("error should reference config.yaml")
	}
	if !strings.Contains(errMsg, "github.com/Elpulgo/azdo") {
		t.Error("error should contain a link to the GitHub configuration docs")
	}
}

func TestConfig_Validate_NoOrganization_ErrorContainsSetupGuidance(t *testing.T) {
	cfg := Config{
		Organization:    "",
		Projects:        []string{"my-project"},
		PollingInterval: 60,
		Theme:           "dark",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty organization")
	}

	errMsg := err.Error()

	if !strings.Contains(errMsg, "organization") {
		t.Error("error should mention 'organization' field")
	}
	if !strings.Contains(errMsg, "config.yaml") {
		t.Error("error should reference config.yaml")
	}
}

func TestConfig_LoadFrom_MissingProjectsShowsExample(t *testing.T) {
	// Create a config file with organization but no projects
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := "organization: my-org\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadFrom(configPath)
	if err == nil {
		t.Fatal("expected error for missing projects")
	}

	errMsg := err.Error()

	if !strings.Contains(errMsg, "projects") {
		t.Error("error should mention 'projects'")
	}
	if !strings.Contains(errMsg, "github.com/Elpulgo/azdo") {
		t.Error("error should contain GitHub configuration link")
	}
}

func TestConfig_LoadFrom_MissingOrgShowsGuidance(t *testing.T) {
	// Create a config file with projects but no organization
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := "projects:\n  - my-project\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadFrom(configPath)
	if err == nil {
		t.Fatal("expected error for missing organization")
	}

	errMsg := err.Error()

	if !strings.Contains(errMsg, "organization") {
		t.Error("error should mention 'organization'")
	}
}

func TestConfig_IsPaneEnabled(t *testing.T) {
	tests := []struct {
		name          string
		disabledPanes []string
		pane          string
		want          bool
	}{
		{"no disabled panes - pipelines enabled", nil, "pipelines", true},
		{"no disabled panes - workitems enabled", nil, "workitems", true},
		{"no disabled panes - pullrequests enabled", nil, "pullrequests", true},
		{"pipelines disabled", []string{"pipelines"}, "pipelines", false},
		{"pipelines disabled - workitems still enabled", []string{"pipelines"}, "workitems", true},
		{"workitems disabled", []string{"workitems"}, "workitems", false},
		{"workitems disabled - pipelines still enabled", []string{"workitems"}, "pipelines", true},
		{"both disabled", []string{"pipelines", "workitems"}, "pipelines", false},
		{"both disabled - workitems", []string{"pipelines", "workitems"}, "workitems", false},
		{"both disabled - pullrequests always enabled", []string{"pipelines", "workitems"}, "pullrequests", true},
		{"unknown pane name", []string{"unknown"}, "pipelines", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Organization:    "test-org",
				Projects:        []string{"test-project"},
				PollingInterval: 60,
				Theme:           "dark",
				DisabledPanes:   tt.disabledPanes,
			}
			if got := cfg.IsPaneEnabled(tt.pane); got != tt.want {
				t.Errorf("IsPaneEnabled(%q) = %v, want %v", tt.pane, got, tt.want)
			}
		})
	}
}

func TestConfig_Validate_InvalidDisabledPane(t *testing.T) {
	tests := []struct {
		name          string
		disabledPanes []string
		wantErr       bool
	}{
		{"valid - no disabled panes", nil, false},
		{"valid - pipelines disabled", []string{"pipelines"}, false},
		{"valid - workitems disabled", []string{"workitems"}, false},
		{"valid - both disabled", []string{"pipelines", "workitems"}, false},
		{"invalid - unknown pane", []string{"unknown"}, true},
		{"invalid - pullrequests cannot be disabled", []string{"pullrequests"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Organization:    "test-org",
				Projects:        []string{"test-project"},
				PollingInterval: 60,
				Theme:           "dark",
				DisabledPanes:   tt.disabledPanes,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad_WithDisabledPanes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `organization: test-org
projects:
  - test-project
polling_interval: 60
theme: dark
disabled_panes: pipelines,workitems
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if cfg.IsPaneEnabled("pipelines") {
		t.Error("expected pipelines to be disabled")
	}
	if cfg.IsPaneEnabled("workitems") {
		t.Error("expected workitems to be disabled")
	}
	if !cfg.IsPaneEnabled("pullrequests") {
		t.Error("expected pullrequests to always be enabled")
	}
}

func TestLoad_WithDisabledPanes_SinglePane(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `organization: test-org
projects:
  - test-project
polling_interval: 60
theme: dark
disabled_panes: pipelines
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if cfg.IsPaneEnabled("pipelines") {
		t.Error("expected pipelines to be disabled")
	}
	if !cfg.IsPaneEnabled("workitems") {
		t.Error("expected workitems to be enabled")
	}
}

func TestSave_PreservesDisabledPanes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `organization: test-org
projects:
  - test-project
polling_interval: 60
theme: dark
disabled_panes: pipelines,workitems
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	// Save and reload
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	reloaded, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() after save failed: %v", err)
	}

	if reloaded.IsPaneEnabled("pipelines") {
		t.Error("expected pipelines to still be disabled after save")
	}
	if reloaded.IsPaneEnabled("workitems") {
		t.Error("expected workitems to still be disabled after save")
	}
}

func TestSave_PreservesMetricsSection(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// A config with a fully-populated metrics section plus an unmanaged
	// top-level key. Save() must round-trip ALL of this, not just the handful
	// of keys it explicitly manages — otherwise changing the theme silently
	// wipes the user's metrics configuration from disk.
	configContent := `organization: test-org
projects:
  - test-project
polling_interval: 60
theme: dark
metrics:
  enabled: true
  interval_days: 21
  wip_limit: 4
  run_one_shot_backfill: true
  states:
    active: Doing
    ready_for_test: QA
    closed: Done
some_future_key: keep-me
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	// Simulate a theme change — the exact action that triggered the bug.
	if err := cfg.UpdateTheme("nord"); err != nil {
		t.Fatalf("UpdateTheme() failed: %v", err)
	}

	reloaded, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() after save failed: %v", err)
	}

	if reloaded.Theme != "nord" {
		t.Errorf("Theme = %q, want nord", reloaded.Theme)
	}
	if !reloaded.Metrics.Enabled {
		t.Error("metrics.enabled was lost on save")
	}
	if reloaded.Metrics.IntervalDays != 21 {
		t.Errorf("metrics.interval_days = %d, want 21 (lost on save)", reloaded.Metrics.IntervalDays)
	}
	if reloaded.Metrics.WIPLimit != 4 {
		t.Errorf("metrics.wip_limit = %d, want 4 (lost on save)", reloaded.Metrics.WIPLimit)
	}
	if !reloaded.Metrics.RunOneShotBackfill {
		t.Error("metrics.run_one_shot_backfill was lost on save")
	}
	if reloaded.Metrics.States.Active != "Doing" || reloaded.Metrics.States.ReadyForTest != "QA" || reloaded.Metrics.States.Closed != "Done" {
		t.Errorf("metrics.states was lost on save: %+v", reloaded.Metrics.States)
	}

	// Unmanaged keys must survive too (read the raw file).
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read raw config: %v", err)
	}
	if !strings.Contains(string(raw), "some_future_key") {
		t.Errorf("unmanaged key was dropped on save; file:\n%s", raw)
	}
}

func TestNewWithPath_CreatesValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := NewWithPath("my-org", []string{"proj-a", "proj-b"}, 90, "nord", configPath)

	if cfg.Organization != "my-org" {
		t.Errorf("Organization = %q, want %q", cfg.Organization, "my-org")
	}
	if len(cfg.Projects) != 2 || cfg.Projects[0] != "proj-a" || cfg.Projects[1] != "proj-b" {
		t.Errorf("Projects = %v, want [proj-a proj-b]", cfg.Projects)
	}
	if cfg.PollingInterval != 90 {
		t.Errorf("PollingInterval = %d, want 90", cfg.PollingInterval)
	}
	if cfg.Theme != "nord" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "nord")
	}

	// Should be saveable
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify saved file can be loaded back
	loaded, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() failed after save: %v", err)
	}
	if loaded.Organization != "my-org" {
		t.Errorf("loaded Organization = %q, want %q", loaded.Organization, "my-org")
	}
}

func TestLoad_MetricsDefaults_WhenBlockAbsent(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configFile)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = true by default; want false (opt-in)")
	}
	if cfg.Metrics.IntervalDays != DefaultMetricsIntervalDays {
		t.Errorf("Metrics.IntervalDays = %d, want %d", cfg.Metrics.IntervalDays, DefaultMetricsIntervalDays)
	}
	if cfg.Metrics.ActiveStaleDays != DefaultMetricsActiveStaleDays {
		t.Errorf("Metrics.ActiveStaleDays = %d, want %d", cfg.Metrics.ActiveStaleDays, DefaultMetricsActiveStaleDays)
	}
	if cfg.Metrics.RFTStaleDays != DefaultMetricsRFTStaleDays {
		t.Errorf("Metrics.RFTStaleDays = %d, want %d", cfg.Metrics.RFTStaleDays, DefaultMetricsRFTStaleDays)
	}
	if cfg.Metrics.WIPLimit != DefaultMetricsWIPLimit {
		t.Errorf("Metrics.WIPLimit = %d, want %d", cfg.Metrics.WIPLimit, DefaultMetricsWIPLimit)
	}
	if cfg.Metrics.RunOneShotBackfill {
		t.Error("Metrics.RunOneShotBackfill = true by default; want false (opt-in)")
	}
}

func TestLoad_MetricsStates_DefaultsWhenAbsent(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
metrics:
  enabled: true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadFrom(configFile)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Metrics.States.Active != "Active" {
		t.Errorf("States.Active = %q, want %q", cfg.Metrics.States.Active, "Active")
	}
	if cfg.Metrics.States.ReadyForTest != "Ready for Test" {
		t.Errorf("States.ReadyForTest = %q, want %q", cfg.Metrics.States.ReadyForTest, "Ready for Test")
	}
	if cfg.Metrics.States.Closed != "Closed" {
		t.Errorf("States.Closed = %q, want %q", cfg.Metrics.States.Closed, "Closed")
	}
}

func TestLoad_MetricsStates_CustomNames(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
metrics:
  enabled: true
  states:
    active: In Progress
    ready_for_test: RFT
    closed: Done
  state_labels:
    ready_for_test: rft
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadFrom(configFile)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Metrics.States.Active != "In Progress" {
		t.Errorf("States.Active = %q", cfg.Metrics.States.Active)
	}
	if cfg.Metrics.States.ReadyForTest != "RFT" {
		t.Errorf("States.ReadyForTest = %q", cfg.Metrics.States.ReadyForTest)
	}
	if cfg.Metrics.States.Closed != "Done" {
		t.Errorf("States.Closed = %q", cfg.Metrics.States.Closed)
	}
	if cfg.Metrics.StateLabels.ReadyForTest != "rft" {
		t.Errorf("StateLabels.ReadyForTest = %q", cfg.Metrics.StateLabels.ReadyForTest)
	}
}

func TestConfig_Validate_RejectsEmptyStateName(t *testing.T) {
	c := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Metrics: MetricsConfig{
			Enabled:         true,
			IntervalDays:    14,
			ActiveStaleDays: 3,
			RFTStaleDays:    2,
			WIPLimit:        4,
			States:          MetricsStates{Active: "", ReadyForTest: "RFT", Closed: "Done"},
		},
	}
	if err := c.Validate(); err == nil {
		t.Error("Validate accepted empty state name; want error")
	}
}

func TestConfig_Validate_RejectsSingleQuoteInStateName(t *testing.T) {
	c := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Metrics: MetricsConfig{
			Enabled:         true,
			IntervalDays:    14,
			ActiveStaleDays: 3,
			RFTStaleDays:    2,
			WIPLimit:        4,
			States:          MetricsStates{Active: "Active", ReadyForTest: "RF'T", Closed: "Done"},
		},
	}
	if err := c.Validate(); err == nil {
		t.Error("Validate accepted single-quote in state name; want error (WIQL injection guard)")
	}
}

func TestConfig_Validate_RejectsDuplicateStateNames(t *testing.T) {
	c := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Metrics: MetricsConfig{
			Enabled:         true,
			IntervalDays:    14,
			ActiveStaleDays: 3,
			RFTStaleDays:    2,
			WIPLimit:        4,
			States:          MetricsStates{Active: "Active", ReadyForTest: "active", Closed: "Done"},
		},
	}
	if err := c.Validate(); err == nil {
		t.Error("Validate accepted duplicate state names (case-insensitive); want error")
	}
}

func TestLoad_MetricsRunOneShotBackfill_Parsed(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
metrics:
  enabled: true
  run_one_shot_backfill: true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configFile)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if !cfg.Metrics.RunOneShotBackfill {
		t.Error("Metrics.RunOneShotBackfill = false, want true")
	}
}

func TestLoad_MetricsBlockParsed(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
metrics:
  enabled: true
  interval_days: 21
  active_stale_days: 5
  rft_stale_days: 1
  wip_limit: 6
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configFile)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.IntervalDays != 21 {
		t.Errorf("Metrics.IntervalDays = %d, want 21", cfg.Metrics.IntervalDays)
	}
	if cfg.Metrics.ActiveStaleDays != 5 {
		t.Errorf("Metrics.ActiveStaleDays = %d, want 5", cfg.Metrics.ActiveStaleDays)
	}
	if cfg.Metrics.RFTStaleDays != 1 {
		t.Errorf("Metrics.RFTStaleDays = %d, want 1", cfg.Metrics.RFTStaleDays)
	}
	if cfg.Metrics.WIPLimit != 6 {
		t.Errorf("Metrics.WIPLimit = %d, want 6", cfg.Metrics.WIPLimit)
	}
}

func TestConfig_Validate_MetricsRejectsNonPositive(t *testing.T) {
	base := func() *Config {
		return &Config{
			Organization:    "org",
			Projects:        []string{"p"},
			PollingInterval: 60,
			Theme:           "dark",
			Metrics: MetricsConfig{
				Enabled:         true,
				IntervalDays:    14,
				ActiveStaleDays: 3,
				RFTStaleDays:    2,
				WIPLimit:        4,
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "interval_days zero",
			mutate:  func(c *Config) { c.Metrics.IntervalDays = 0 },
			wantErr: "interval_days",
		},
		{
			name:    "active_stale_days negative",
			mutate:  func(c *Config) { c.Metrics.ActiveStaleDays = -1 },
			wantErr: "active_stale_days",
		},
		{
			name:    "rft_stale_days negative",
			mutate:  func(c *Config) { c.Metrics.RFTStaleDays = -1 },
			wantErr: "rft_stale_days",
		},
		{
			name:    "wip_limit zero",
			mutate:  func(c *Config) { c.Metrics.WIPLimit = 0 },
			wantErr: "wip_limit",
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

func TestConfig_Validate_MetricsDisabledSkipsThresholdChecks(t *testing.T) {
	// When disabled the block is inert, so invalid values shouldn't break Load.
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Metrics: MetricsConfig{
			Enabled:      false,
			IntervalDays: 0, // would be invalid if enabled
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with disabled metrics = %v, want nil", err)
	}
}

func TestLoad_WithTerms(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `organization: test-org
projects:
  - test-project
polling_interval: 60
theme: dark
terms:
  work_items: Tasks
  pipelines: Builds
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if cfg.Terms["work_items"] != "Tasks" {
		t.Errorf("Terms[work_items] = %q, want %q", cfg.Terms["work_items"], "Tasks")
	}
	if cfg.Terms["pipelines"] != "Builds" {
		t.Errorf("Terms[pipelines] = %q, want %q", cfg.Terms["pipelines"], "Builds")
	}
}

func TestConfig_TermFor(t *testing.T) {
	tests := []struct {
		name     string
		terms    map[string]string
		key      string
		fallback string
		want     string
	}{
		{"override present", map[string]string{"work_items": "Tasks"}, "work_items", "Work Items", "Tasks"},
		{"key absent", map[string]string{"pipelines": "Builds"}, "work_items", "Work Items", "Work Items"},
		{"nil terms", nil, "work_items", "Work Items", "Work Items"},
		{"empty override falls through to fallback", map[string]string{"work_items": ""}, "work_items", "Work Items", "Work Items"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Terms: tt.terms}
			if got := cfg.TermFor(tt.key, tt.fallback); got != tt.want {
				t.Errorf("TermFor(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestSave_RoundTripsTerms(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write initial config with a pre-existing key (theme) we want to verify survives.
	initialContent := `organization: test-org
projects:
  - test-project
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	cfg.Terms = map[string]string{
		"work_items": "Tasks",
		"pipelines":  "Builds",
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	reloaded, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() after save failed: %v", err)
	}

	if reloaded.Terms["work_items"] != "Tasks" {
		t.Errorf("Terms[work_items] = %q, want %q after round-trip", reloaded.Terms["work_items"], "Tasks")
	}
	if reloaded.Terms["pipelines"] != "Builds" {
		t.Errorf("Terms[pipelines] = %q, want %q after round-trip", reloaded.Terms["pipelines"], "Builds")
	}

	// Unrelated key must survive the save.
	if reloaded.Theme != "dark" {
		t.Errorf("Theme = %q, want dark (lost on save)", reloaded.Theme)
	}
}
