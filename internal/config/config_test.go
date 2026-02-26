package config

import (
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

func TestLoad_MissingConfigDirectory(t *testing.T) {
	// Create a temporary directory without .config
	tempDir := t.TempDir()

	// Set HOME to temp directory for testing
	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	// Load config (should return error since config file doesn't exist)
	cfg, err := Load()
	if err == nil {
		t.Fatal("Load() should fail when config file is not found")
	}

	if cfg != nil {
		t.Error("Expected cfg to be nil when config file is not found")
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
