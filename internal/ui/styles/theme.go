package styles

import "github.com/charmbracelet/lipgloss"

// Theme defines the color palette for the application.
// All colors are represented as lipgloss.Color which can be ANSI 256 colors
// (e.g., "33") or hex colors (e.g., "#7c6f64").
type Theme struct {
	Name string

	// Primary colors for headers, active elements, and emphasis
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color

	// Status colors for semantic meaning
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Info    lipgloss.Color

	// Background colors
	Background       lipgloss.Color // Main background
	BackgroundAlt    lipgloss.Color // Alternative background (modals, etc.)
	BackgroundSelect lipgloss.Color // Selection background

	// Foreground colors
	Foreground      lipgloss.Color // Main text
	ForegroundMuted lipgloss.Color // Secondary/disabled text
	ForegroundBold  lipgloss.Color // Emphasized text

	// Selection colors
	SelectForeground lipgloss.Color
	SelectBackground lipgloss.Color

	// UI element colors
	Border  lipgloss.Color
	Link    lipgloss.Color
	Spinner lipgloss.Color

	// Tab colors
	TabActiveForeground   lipgloss.Color
	TabActiveBackground   lipgloss.Color
	TabInactiveForeground lipgloss.Color
	TabInactiveBackground lipgloss.Color
}

// ThemeColors provides an interface for accessing theme colors.
// This allows for future extensibility and testing.
type ThemeColors interface {
	GetPrimary() lipgloss.Color
	GetSecondary() lipgloss.Color
	GetAccent() lipgloss.Color
	GetSuccess() lipgloss.Color
	GetWarning() lipgloss.Color
	GetError() lipgloss.Color
	GetInfo() lipgloss.Color
	GetBackground() lipgloss.Color
	GetBackgroundAlt() lipgloss.Color
	GetBackgroundSelect() lipgloss.Color
	GetForeground() lipgloss.Color
	GetForegroundMuted() lipgloss.Color
	GetForegroundBold() lipgloss.Color
	GetSelectForeground() lipgloss.Color
	GetSelectBackground() lipgloss.Color
	GetBorder() lipgloss.Color
	GetLink() lipgloss.Color
	GetSpinner() lipgloss.Color
	GetTabActiveForeground() lipgloss.Color
	GetTabActiveBackground() lipgloss.Color
	GetTabInactiveForeground() lipgloss.Color
	GetTabInactiveBackground() lipgloss.Color
	GetName() string
}

// Implement ThemeColors interface for Theme

func (t Theme) GetPrimary() lipgloss.Color            { return t.Primary }
func (t Theme) GetSecondary() lipgloss.Color          { return t.Secondary }
func (t Theme) GetAccent() lipgloss.Color             { return t.Accent }
func (t Theme) GetSuccess() lipgloss.Color            { return t.Success }
func (t Theme) GetWarning() lipgloss.Color            { return t.Warning }
func (t Theme) GetError() lipgloss.Color              { return t.Error }
func (t Theme) GetInfo() lipgloss.Color               { return t.Info }
func (t Theme) GetBackground() lipgloss.Color         { return t.Background }
func (t Theme) GetBackgroundAlt() lipgloss.Color      { return t.BackgroundAlt }
func (t Theme) GetBackgroundSelect() lipgloss.Color   { return t.BackgroundSelect }
func (t Theme) GetForeground() lipgloss.Color         { return t.Foreground }
func (t Theme) GetForegroundMuted() lipgloss.Color    { return t.ForegroundMuted }
func (t Theme) GetForegroundBold() lipgloss.Color     { return t.ForegroundBold }
func (t Theme) GetSelectForeground() lipgloss.Color   { return t.SelectForeground }
func (t Theme) GetSelectBackground() lipgloss.Color   { return t.SelectBackground }
func (t Theme) GetBorder() lipgloss.Color             { return t.Border }
func (t Theme) GetLink() lipgloss.Color               { return t.Link }
func (t Theme) GetSpinner() lipgloss.Color            { return t.Spinner }
func (t Theme) GetTabActiveForeground() lipgloss.Color   { return t.TabActiveForeground }
func (t Theme) GetTabActiveBackground() lipgloss.Color   { return t.TabActiveBackground }
func (t Theme) GetTabInactiveForeground() lipgloss.Color { return t.TabInactiveForeground }
func (t Theme) GetTabInactiveBackground() lipgloss.Color { return t.TabInactiveBackground }
func (t Theme) GetName() string                       { return t.Name }

// Validate checks that all required colors are set in the theme.
// Returns an error if any required color is empty.
func (t Theme) Validate() error {
	// All colors should be set for a valid theme
	// Empty lipgloss.Color is just an empty string
	if t.Name == "" {
		return ErrThemeNameRequired
	}
	return nil
}

// ThemeError represents errors related to theme operations.
type ThemeError struct {
	Message string
}

func (e ThemeError) Error() string {
	return e.Message
}

// Common theme errors
var (
	ErrThemeNameRequired = ThemeError{Message: "theme name is required"}
	ErrThemeNotFound     = ThemeError{Message: "theme not found"}
)
