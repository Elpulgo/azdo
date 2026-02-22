package config

import (
	"os"
	"path/filepath"
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
