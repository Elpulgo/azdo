package config

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestLoad_WithDefaultValues(t *testing.T) {
	// Create a temporary config directory
	tempDir := t.TempDir()

	// Set HOME to temp directory for testing
	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	// Load config (no config file exists, should use defaults)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Test default polling interval
	if cfg.PollingInterval == 0 {
		t.Error("Expected default PollingInterval to be set, got 0")
	}

	// Test default theme
	if cfg.Theme == "" {
		t.Error("Expected default Theme to be set, got empty string")
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Create a temporary config directory
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config", "azdo-tui")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Write a test config file
	configFile := filepath.Join(configDir, "config.yaml")
	configContent := `organization: test-org
project: test-project
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

	if cfg.Project != "test-project" {
		t.Errorf("Expected Project to be 'test-project', got %s", cfg.Project)
	}

	if cfg.PollingInterval != 120 {
		t.Errorf("Expected PollingInterval to be 120, got %d", cfg.PollingInterval)
	}

	if cfg.Theme != "dark" {
		t.Errorf("Expected Theme to be 'dark', got %s", cfg.Theme)
	}
}

func TestLoad_MissingConfigDirectory(t *testing.T) {
	// Create a temporary directory without .config
	tempDir := t.TempDir()

	// Set HOME to temp directory for testing
	cleanup := setTestHome(t, tempDir)
	defer cleanup()

	// Load config (should create directory and use defaults)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not fail with missing config directory: %v", err)
	}

	// Verify defaults are set
	if cfg.PollingInterval == 0 {
		t.Error("Expected default PollingInterval to be set")
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
				Project:         "test-project",
				PollingInterval: 60,
				Theme:           "light",
			},
			wantErr: false,
		},
		{
			name: "invalid polling interval - zero",
			config: Config{
				Organization:    "test-org",
				Project:         "test-project",
				PollingInterval: 0,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "invalid polling interval - negative",
			config: Config{
				Organization:    "test-org",
				Project:         "test-project",
				PollingInterval: -10,
				Theme:           "light",
			},
			wantErr: true,
		},
		{
			name: "empty theme",
			config: Config{
				Organization:    "test-org",
				Project:         "test-project",
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
