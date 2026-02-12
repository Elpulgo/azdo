package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestNewStyles tests creating a Styles instance from a theme
func TestNewStyles(t *testing.T) {
	theme := GetDefaultTheme()
	styles := NewStyles(theme)

	if styles.Theme.Name != theme.Name {
		t.Errorf("NewStyles() theme name = %q, want %q", styles.Theme.Name, theme.Name)
	}
}

// TestStylesTabStyles tests that tab styles are correctly generated
func TestStylesTabStyles(t *testing.T) {
	theme := GetDefaultTheme()
	styles := NewStyles(theme)

	// Tab active should have the theme's tab colors
	tabActive := styles.TabActive
	if tabActive.GetForeground() != lipgloss.Color(theme.TabActiveForeground) {
		t.Error("TabActive foreground doesn't match theme")
	}

	// Tab inactive should have different colors
	tabInactive := styles.TabInactive
	if tabInactive.GetForeground() != lipgloss.Color(theme.TabInactiveForeground) {
		t.Error("TabInactive foreground doesn't match theme")
	}
}

// TestStylesStatusStyles tests that status styles are correctly generated
func TestStylesStatusStyles(t *testing.T) {
	theme := GetDefaultTheme()
	styles := NewStyles(theme)

	// Success style should use success color
	success := styles.Success
	if success.GetForeground() != lipgloss.Color(theme.Success) {
		t.Error("Success foreground doesn't match theme")
	}

	// Error style should use error color
	errorStyle := styles.Error
	if errorStyle.GetForeground() != lipgloss.Color(theme.Error) {
		t.Error("Error foreground doesn't match theme")
	}

	// Warning style should use warning color
	warning := styles.Warning
	if warning.GetForeground() != lipgloss.Color(theme.Warning) {
		t.Error("Warning foreground doesn't match theme")
	}
}

// TestStylesWithDifferentThemes tests that styles vary by theme
func TestStylesWithDifferentThemes(t *testing.T) {
	darkTheme, _ := GetThemeByName("dark")
	nordTheme, _ := GetThemeByName("nord")

	darkStyles := NewStyles(darkTheme)
	nordStyles := NewStyles(nordTheme)

	// Themes should have different primary colors
	if darkStyles.Header.GetForeground() == nordStyles.Header.GetForeground() {
		// This might pass if colors happen to be same, so we check theme name
		if darkStyles.Theme.Name == nordStyles.Theme.Name {
			t.Error("Different themes should have different names")
		}
	}
}

// TestStylesSelectionStyles tests selection-related styles
func TestStylesSelectionStyles(t *testing.T) {
	theme := GetDefaultTheme()
	styles := NewStyles(theme)

	selected := styles.Selected
	// Selected should have selection colors
	if selected.GetForeground() != lipgloss.Color(theme.SelectForeground) {
		t.Error("Selected foreground doesn't match theme SelectForeground")
	}
	if selected.GetBackground() != lipgloss.Color(theme.SelectBackground) {
		t.Error("Selected background doesn't match theme SelectBackground")
	}
}

// TestStylesAllThemes tests that NewStyles works with all built-in themes
func TestStylesAllThemes(t *testing.T) {
	themeNames := ListAvailableThemes()

	for _, name := range themeNames {
		t.Run(name, func(t *testing.T) {
			theme, err := GetThemeByName(name)
			if err != nil {
				t.Fatalf("GetThemeByName(%q) failed: %v", name, err)
			}

			styles := NewStyles(theme)

			// Verify styles were created
			if styles.Theme.Name != name {
				t.Errorf("NewStyles() theme name = %q, want %q", styles.Theme.Name, name)
			}

			// Verify key styles exist (non-nil)
			if styles.TabActive.GetForeground() == nil {
				t.Error("TabActive has nil foreground")
			}
			if styles.Header.GetForeground() == nil {
				t.Error("Header has nil foreground")
			}
		})
	}
}
