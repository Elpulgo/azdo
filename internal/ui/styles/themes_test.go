package styles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestGetThemeByName tests loading built-in themes by name
func TestGetThemeByName(t *testing.T) {
	tests := []struct {
		name      string
		themeName string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "load dark theme",
			themeName: "dark",
			wantName:  "dark",
			wantErr:   false,
		},
		{
			name:      "load gruvbox theme",
			themeName: "gruvbox",
			wantName:  "gruvbox",
			wantErr:   false,
		},
		{
			name:      "load nord theme",
			themeName: "nord",
			wantName:  "nord",
			wantErr:   false,
		},
		{
			name:      "load dracula theme",
			themeName: "dracula",
			wantName:  "dracula",
			wantErr:   false,
		},
		{
			name:      "load catppuccin theme",
			themeName: "catppuccin",
			wantName:  "catppuccin",
			wantErr:   false,
		},
		{
			name:      "load github theme",
			themeName: "github",
			wantName:  "github",
			wantErr:   false,
		},
		{
			name:      "invalid theme returns error",
			themeName: "nonexistent",
			wantName:  "",
			wantErr:   true,
		},
		{
			name:      "empty theme name returns error",
			themeName: "",
			wantName:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme, err := GetThemeByName(tt.themeName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetThemeByName(%q) expected error, got nil", tt.themeName)
				}
				return
			}
			if err != nil {
				t.Errorf("GetThemeByName(%q) unexpected error: %v", tt.themeName, err)
				return
			}
			if theme.Name != tt.wantName {
				t.Errorf("GetThemeByName(%q) got name %q, want %q", tt.themeName, theme.Name, tt.wantName)
			}
		})
	}
}

// TestGetThemeByNameWithFallback tests fallback to default theme on invalid name
func TestGetThemeByNameWithFallback(t *testing.T) {
	tests := []struct {
		name      string
		themeName string
		wantName  string
	}{
		{
			name:      "valid theme returns requested theme",
			themeName: "nord",
			wantName:  "nord",
		},
		{
			name:      "invalid theme falls back to dark",
			themeName: "nonexistent",
			wantName:  "dark",
		},
		{
			name:      "empty theme falls back to dark",
			themeName: "",
			wantName:  "dark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := GetThemeByNameWithFallback(tt.themeName)
			if theme.Name != tt.wantName {
				t.Errorf("GetThemeByNameWithFallback(%q) got name %q, want %q", tt.themeName, theme.Name, tt.wantName)
			}
		})
	}
}

// TestThemeValidation tests that all built-in themes pass validation
func TestThemeValidation(t *testing.T) {
	themeNames := []string{"dark", "gruvbox", "nord", "dracula", "catppuccin", "github"}

	for _, themeName := range themeNames {
		t.Run(themeName, func(t *testing.T) {
			theme, err := GetThemeByName(themeName)
			if err != nil {
				t.Fatalf("GetThemeByName(%q) failed: %v", themeName, err)
			}

			if err := theme.Validate(); err != nil {
				t.Errorf("Theme %q failed validation: %v", themeName, err)
			}
		})
	}
}

// TestThemeHasAllRequiredColors tests that all themes have non-empty colors
func TestThemeHasAllRequiredColors(t *testing.T) {
	themeNames := []string{"dark", "gruvbox", "nord", "dracula", "catppuccin", "github"}

	for _, themeName := range themeNames {
		t.Run(themeName, func(t *testing.T) {
			theme, err := GetThemeByName(themeName)
			if err != nil {
				t.Fatalf("GetThemeByName(%q) failed: %v", themeName, err)
			}

			// Check all color fields are set (non-empty)
			colorChecks := []struct {
				name  string
				color lipgloss.Color
			}{
				{"Primary", theme.Primary},
				{"Secondary", theme.Secondary},
				{"Accent", theme.Accent},
				{"Success", theme.Success},
				{"Warning", theme.Warning},
				{"Error", theme.Error},
				{"Info", theme.Info},
				{"Background", theme.Background},
				{"BackgroundAlt", theme.BackgroundAlt},
				{"BackgroundSelect", theme.BackgroundSelect},
				{"Foreground", theme.Foreground},
				{"ForegroundMuted", theme.ForegroundMuted},
				{"ForegroundBold", theme.ForegroundBold},
				{"SelectForeground", theme.SelectForeground},
				{"SelectBackground", theme.SelectBackground},
				{"Border", theme.Border},
				{"Link", theme.Link},
				{"Spinner", theme.Spinner},
				{"TabActiveForeground", theme.TabActiveForeground},
				{"TabActiveBackground", theme.TabActiveBackground},
				{"TabInactiveForeground", theme.TabInactiveForeground},
				{"TabInactiveBackground", theme.TabInactiveBackground},
			}

			for _, check := range colorChecks {
				if string(check.color) == "" {
					t.Errorf("Theme %q has empty %s color", themeName, check.name)
				}
			}
		})
	}
}

// TestListAvailableThemes tests the theme registry listing
func TestListAvailableThemes(t *testing.T) {
	themes := ListAvailableThemes()

	expectedThemes := []string{"dark", "gruvbox", "nord", "dracula", "catppuccin", "github"}

	if len(themes) < len(expectedThemes) {
		t.Errorf("ListAvailableThemes() returned %d themes, want at least %d", len(themes), len(expectedThemes))
	}

	// Check all expected themes are present
	themeMap := make(map[string]bool)
	for _, name := range themes {
		themeMap[name] = true
	}

	for _, expected := range expectedThemes {
		if !themeMap[expected] {
			t.Errorf("ListAvailableThemes() missing expected theme %q", expected)
		}
	}
}

// TestDefaultTheme tests that GetDefaultTheme returns the dark theme
func TestDefaultTheme(t *testing.T) {
	theme := GetDefaultTheme()

	if theme.Name != "dark" {
		t.Errorf("GetDefaultTheme() returned theme %q, want %q", theme.Name, "dark")
	}

	if err := theme.Validate(); err != nil {
		t.Errorf("Default theme failed validation: %v", err)
	}
}

// TestThemeColorsInterface tests that Theme implements ThemeColors interface
func TestThemeColorsInterface(t *testing.T) {
	theme, _ := GetThemeByName("dark")

	// Verify Theme implements ThemeColors
	var _ ThemeColors = theme

	// Test all interface methods return expected values
	if theme.GetName() != "dark" {
		t.Errorf("GetName() = %q, want %q", theme.GetName(), "dark")
	}
	if theme.GetPrimary() != theme.Primary {
		t.Error("GetPrimary() doesn't match Primary field")
	}
	if theme.GetSecondary() != theme.Secondary {
		t.Error("GetSecondary() doesn't match Secondary field")
	}
	if theme.GetAccent() != theme.Accent {
		t.Error("GetAccent() doesn't match Accent field")
	}
	if theme.GetSuccess() != theme.Success {
		t.Error("GetSuccess() doesn't match Success field")
	}
	if theme.GetWarning() != theme.Warning {
		t.Error("GetWarning() doesn't match Warning field")
	}
	if theme.GetError() != theme.Error {
		t.Error("GetError() doesn't match Error field")
	}
	if theme.GetBackground() != theme.Background {
		t.Error("GetBackground() doesn't match Background field")
	}
	if theme.GetForeground() != theme.Foreground {
		t.Error("GetForeground() doesn't match Foreground field")
	}
}

// TestLoadThemeFromJSON tests loading a theme from JSON bytes
func TestLoadThemeFromJSON(t *testing.T) {
	jsonData := `{
		"name": "custom-test",
		"primary": "#ff0000",
		"secondary": "#00ff00",
		"accent": "#0000ff",
		"success": "#00ff00",
		"warning": "#ffff00",
		"error": "#ff0000",
		"info": "#00ffff",
		"background": "#000000",
		"background_alt": "#111111",
		"background_select": "#222222",
		"foreground": "#ffffff",
		"foreground_muted": "#888888",
		"foreground_bold": "#ffffff",
		"select_foreground": "#ffffff",
		"select_background": "#0000ff",
		"border": "#444444",
		"link": "#0088ff",
		"spinner": "#ff00ff",
		"tab_active_foreground": "#ffffff",
		"tab_active_background": "#0000ff",
		"tab_inactive_foreground": "#888888",
		"tab_inactive_background": "#333333"
	}`

	theme, err := LoadThemeFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("LoadThemeFromJSON() failed: %v", err)
	}

	if theme.Name != "custom-test" {
		t.Errorf("Name = %q, want %q", theme.Name, "custom-test")
	}
	if string(theme.Primary) != "#ff0000" {
		t.Errorf("Primary = %q, want %q", theme.Primary, "#ff0000")
	}
	if string(theme.Background) != "#000000" {
		t.Errorf("Background = %q, want %q", theme.Background, "#000000")
	}

	// Validate the loaded theme
	if err := theme.Validate(); err != nil {
		t.Errorf("Loaded theme failed validation: %v", err)
	}
}

// TestLoadThemeFromJSON_InvalidJSON tests error handling for invalid JSON
func TestLoadThemeFromJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantErr  bool
	}{
		{
			name:     "invalid JSON syntax",
			jsonData: `{invalid json}`,
			wantErr:  true,
		},
		{
			name:     "missing name field",
			jsonData: `{"primary": "#ff0000"}`,
			wantErr:  true,
		},
		{
			name:     "empty name field",
			jsonData: `{"name": "", "primary": "#ff0000"}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadThemeFromJSON([]byte(tt.jsonData))
			if tt.wantErr && err == nil {
				t.Error("LoadThemeFromJSON() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("LoadThemeFromJSON() unexpected error: %v", err)
			}
		})
	}
}

// TestGetThemesDirectoryPath tests the themes directory path function
func TestGetThemesDirectoryPath(t *testing.T) {
	path, err := GetThemesDirectoryPath()
	if err != nil {
		t.Fatalf("GetThemesDirectoryPath() failed: %v", err)
	}

	if path == "" {
		t.Error("GetThemesDirectoryPath() returned empty path")
	}

	// Path should contain .config/azdo-tui/themes
	if !contains(path, "azdo-tui") || !contains(path, "themes") {
		t.Errorf("GetThemesDirectoryPath() = %q, should contain 'azdo-tui' and 'themes'", path)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (hasPrefix(s, substr) || hasSuffix(s, substr) || hasInfix(s, substr)))
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func hasInfix(s, infix string) bool {
	for i := 0; i <= len(s)-len(infix); i++ {
		if s[i:i+len(infix)] == infix {
			return true
		}
	}
	return false
}

// TestLoadCustomThemesFromDirectory tests loading custom themes from a directory
func TestLoadCustomThemesFromDirectory(t *testing.T) {
	// Create a temporary directory for test themes
	tmpDir := t.TempDir()

	// Create a valid custom theme file
	customTheme := `{
		"name": "custom-blue",
		"primary": "#0066ff",
		"secondary": "#00aaff",
		"accent": "#0044aa",
		"success": "#00ff00",
		"warning": "#ffff00",
		"error": "#ff0000",
		"info": "#00ffff",
		"background": "#000033",
		"background_alt": "#000055",
		"background_select": "#000077",
		"foreground": "#ffffff",
		"foreground_muted": "#888888",
		"foreground_bold": "#ffffff",
		"select_foreground": "#ffffff",
		"select_background": "#0066ff",
		"border": "#444444",
		"link": "#0088ff",
		"spinner": "#ff00ff",
		"tab_active_foreground": "#ffffff",
		"tab_active_background": "#0066ff",
		"tab_inactive_foreground": "#888888",
		"tab_inactive_background": "#333333"
	}`

	// Write the theme file
	themeFile := filepath.Join(tmpDir, "custom-blue.json")
	if err := os.WriteFile(themeFile, []byte(customTheme), 0644); err != nil {
		t.Fatalf("Failed to write test theme file: %v", err)
	}

	// Create an invalid theme file (should be skipped)
	invalidFile := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(invalidFile, []byte("{invalid}"), 0644); err != nil {
		t.Fatalf("Failed to write invalid theme file: %v", err)
	}

	// Create a non-JSON file (should be ignored)
	txtFile := filepath.Join(tmpDir, "readme.txt")
	if err := os.WriteFile(txtFile, []byte("This is not a theme"), 0644); err != nil {
		t.Fatalf("Failed to write txt file: %v", err)
	}

	// Load custom themes from the directory
	count, err := LoadCustomThemesFromDirectory(tmpDir)
	if err != nil {
		t.Fatalf("LoadCustomThemesFromDirectory() failed: %v", err)
	}

	if count != 1 {
		t.Errorf("LoadCustomThemesFromDirectory() loaded %d themes, want 1", count)
	}

	// Verify the custom theme is now available
	theme, err := GetThemeByName("custom-blue")
	if err != nil {
		t.Fatalf("GetThemeByName('custom-blue') failed after loading: %v", err)
	}

	if theme.Name != "custom-blue" {
		t.Errorf("Theme name = %q, want %q", theme.Name, "custom-blue")
	}
	if string(theme.Primary) != "#0066ff" {
		t.Errorf("Theme Primary = %q, want %q", theme.Primary, "#0066ff")
	}
}

// TestLoadCustomThemesFromDirectory_NonexistentDirectory tests handling of missing directory
func TestLoadCustomThemesFromDirectory_NonexistentDirectory(t *testing.T) {
	// Should not error, just return 0 themes loaded
	count, err := LoadCustomThemesFromDirectory("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Errorf("LoadCustomThemesFromDirectory() with nonexistent dir should not error, got: %v", err)
	}
	if count != 0 {
		t.Errorf("LoadCustomThemesFromDirectory() with nonexistent dir should return 0, got %d", count)
	}
}

// TestRegisterTheme tests registering a custom theme
func TestRegisterTheme(t *testing.T) {
	customTheme := Theme{
		Name:                  "test-register",
		Primary:               "#ff0000",
		Secondary:             "#00ff00",
		Accent:                "#0000ff",
		Success:               "#00ff00",
		Warning:               "#ffff00",
		Error:                 "#ff0000",
		Info:                  "#00ffff",
		Background:            "#000000",
		BackgroundAlt:         "#111111",
		BackgroundSelect:      "#222222",
		Foreground:            "#ffffff",
		ForegroundMuted:       "#888888",
		ForegroundBold:        "#ffffff",
		SelectForeground:      "#ffffff",
		SelectBackground:      "#0000ff",
		Border:                "#444444",
		Link:                  "#0088ff",
		Spinner:               "#ff00ff",
		TabActiveForeground:   "#ffffff",
		TabActiveBackground:   "#0000ff",
		TabInactiveForeground: "#888888",
		TabInactiveBackground: "#333333",
	}

	// Register the theme
	if err := RegisterTheme(customTheme); err != nil {
		t.Fatalf("RegisterTheme() failed: %v", err)
	}

	// Verify it's now available
	theme, err := GetThemeByName("test-register")
	if err != nil {
		t.Fatalf("GetThemeByName('test-register') failed after registration: %v", err)
	}

	if theme.Name != "test-register" {
		t.Errorf("Theme name = %q, want %q", theme.Name, "test-register")
	}
}

// TestRegisterTheme_InvalidTheme tests error handling for invalid themes
func TestRegisterTheme_InvalidTheme(t *testing.T) {
	invalidTheme := Theme{
		Name: "", // Empty name should fail validation
	}

	err := RegisterTheme(invalidTheme)
	if err == nil {
		t.Error("RegisterTheme() with invalid theme should return error, got nil")
	}
}
