package styles

import (
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
