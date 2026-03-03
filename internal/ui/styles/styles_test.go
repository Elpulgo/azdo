package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

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

// TestBorderedStylesHaveBorders tests that styles expected to have borders have them on all 4 sides
func TestBorderedStylesHaveBorders(t *testing.T) {
	theme := GetDefaultTheme()
	s := NewStyles(theme)

	assertBordered := func(t *testing.T, style lipgloss.Style) {
		t.Helper()
		if style.GetBorderStyle() == (lipgloss.Border{}) {
			t.Error("should have a border style set")
		}
		if style.GetBorderTopSize() != 1 {
			t.Error("should have a top border")
		}
		if style.GetBorderBottomSize() != 1 {
			t.Error("should have a bottom border")
		}
		if style.GetBorderLeftSize() != 1 {
			t.Error("should have a left border")
		}
		if style.GetBorderRightSize() != 1 {
			t.Error("should have a right border")
		}
	}

	t.Run("ContentBox", func(t *testing.T) { assertBordered(t, s.ContentBox) })
	t.Run("TabBar", func(t *testing.T) { assertBordered(t, s.TabBar) })
}

// TestStylesTabInactiveHasNoBackground tests that inactive tabs have no background color
func TestStylesTabInactiveHasNoBackground(t *testing.T) {
	theme := GetDefaultTheme()
	styles := NewStyles(theme)

	// TabInactive should NOT have a background color set
	bg := styles.TabInactive.GetBackground()
	if _, isNoColor := bg.(lipgloss.NoColor); !isNoColor {
		t.Errorf("TabInactive should have no background color, got %v", bg)
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
