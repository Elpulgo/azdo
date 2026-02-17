package styles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// themeRegistry holds all built-in themes
var themeRegistry = map[string]Theme{
	"dark":       darkTheme,
	"gruvbox":    gruvboxTheme,
	"nord":       nordTheme,
	"dracula":    draculaTheme,
	"catppuccin": catppuccinTheme,
	"github":     githubTheme,
}

// GetThemeByName returns a theme by name.
// Returns ErrThemeNotFound if the theme doesn't exist.
func GetThemeByName(name string) (Theme, error) {
	if name == "" {
		return Theme{}, ErrThemeNotFound
	}
	theme, ok := themeRegistry[name]
	if !ok {
		return Theme{}, ErrThemeNotFound
	}
	return theme, nil
}

// GetThemeByNameWithFallback returns a theme by name, falling back to the default
// theme if the requested theme doesn't exist.
func GetThemeByNameWithFallback(name string) Theme {
	theme, err := GetThemeByName(name)
	if err != nil {
		return GetDefaultTheme()
	}
	return theme
}

// GetDefaultTheme returns the default gruvbox theme.
func GetDefaultTheme() Theme {
	return gruvboxTheme
}

// ListAvailableThemes returns a sorted list of all available theme names.
func ListAvailableThemes() []string {
	names := make([]string, 0, len(themeRegistry))
	for name := range themeRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetThemesDirectoryPath returns the path to the custom themes directory.
// Custom themes should be placed in ~/.config/azdo-tui/themes/
func GetThemesDirectoryPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "azdo-tui", "themes"), nil
}

// themeJSON represents the JSON structure for custom themes.
// Uses snake_case for JSON field names to match common conventions.
type themeJSON struct {
	Name                  string `json:"name"`
	Primary               string `json:"primary"`
	Secondary             string `json:"secondary"`
	Accent                string `json:"accent"`
	Success               string `json:"success"`
	Warning               string `json:"warning"`
	Error                 string `json:"error"`
	Info                  string `json:"info"`
	Background            string `json:"background"`
	BackgroundAlt         string `json:"background_alt"`
	BackgroundSelect      string `json:"background_select"`
	Foreground            string `json:"foreground"`
	ForegroundMuted       string `json:"foreground_muted"`
	ForegroundBold        string `json:"foreground_bold"`
	SelectForeground      string `json:"select_foreground"`
	SelectBackground      string `json:"select_background"`
	Border                string `json:"border"`
	Link                  string `json:"link"`
	Spinner               string `json:"spinner"`
	TabActiveForeground   string `json:"tab_active_foreground"`
	TabActiveBackground   string `json:"tab_active_background"`
	TabInactiveForeground string `json:"tab_inactive_foreground"`
	TabInactiveBackground string `json:"tab_inactive_background"`
}

// LoadThemeFromJSON loads a theme from JSON bytes.
// Returns an error if the JSON is invalid or the theme fails validation.
func LoadThemeFromJSON(data []byte) (Theme, error) {
	var tj themeJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return Theme{}, fmt.Errorf("failed to parse theme JSON: %w", err)
	}

	// Convert JSON structure to Theme
	theme := Theme{
		Name:                  tj.Name,
		Primary:               lipgloss.Color(tj.Primary),
		Secondary:             lipgloss.Color(tj.Secondary),
		Accent:                lipgloss.Color(tj.Accent),
		Success:               lipgloss.Color(tj.Success),
		Warning:               lipgloss.Color(tj.Warning),
		Error:                 lipgloss.Color(tj.Error),
		Info:                  lipgloss.Color(tj.Info),
		Background:            lipgloss.Color(tj.Background),
		BackgroundAlt:         lipgloss.Color(tj.BackgroundAlt),
		BackgroundSelect:      lipgloss.Color(tj.BackgroundSelect),
		Foreground:            lipgloss.Color(tj.Foreground),
		ForegroundMuted:       lipgloss.Color(tj.ForegroundMuted),
		ForegroundBold:        lipgloss.Color(tj.ForegroundBold),
		SelectForeground:      lipgloss.Color(tj.SelectForeground),
		SelectBackground:      lipgloss.Color(tj.SelectBackground),
		Border:                lipgloss.Color(tj.Border),
		Link:                  lipgloss.Color(tj.Link),
		Spinner:               lipgloss.Color(tj.Spinner),
		TabActiveForeground:   lipgloss.Color(tj.TabActiveForeground),
		TabActiveBackground:   lipgloss.Color(tj.TabActiveBackground),
		TabInactiveForeground: lipgloss.Color(tj.TabInactiveForeground),
		TabInactiveBackground: lipgloss.Color(tj.TabInactiveBackground),
	}

	// Validate the theme
	if err := theme.Validate(); err != nil {
		return Theme{}, fmt.Errorf("invalid theme: %w", err)
	}

	return theme, nil
}

// RegisterTheme registers a custom theme in the theme registry.
// Returns an error if the theme fails validation.
func RegisterTheme(theme Theme) error {
	if err := theme.Validate(); err != nil {
		return fmt.Errorf("invalid theme: %w", err)
	}
	themeRegistry[theme.Name] = theme
	return nil
}

// LoadCustomThemesFromDirectory loads all custom theme files from a directory.
// Theme files must have a .json extension and contain valid theme JSON.
// Returns the number of themes successfully loaded and any critical errors.
// Missing directory or individual invalid theme files are not considered errors.
func LoadCustomThemesFromDirectory(dir string) (int, error) {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Directory doesn't exist - not an error, just no custom themes
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("failed to access themes directory: %w", err)
	}

	// Read all files in the directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read themes directory: %w", err)
	}

	loadedCount := 0
	for _, entry := range entries {
		// Skip directories and non-JSON files
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Read the theme file
		themePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(themePath)
		if err != nil {
			// Skip files that can't be read
			continue
		}

		// Try to load the theme
		theme, err := LoadThemeFromJSON(data)
		if err != nil {
			// Skip invalid theme files
			continue
		}

		// Register the theme
		if err := RegisterTheme(theme); err != nil {
			// Skip themes that fail registration
			continue
		}

		loadedCount++
	}

	return loadedCount, nil
}

// Dark theme - the default theme matching current application colors
var darkTheme = Theme{
	Name: "dark",

	// Primary colors
	Primary:   lipgloss.Color("33"),  // Blue - headers, active status
	Secondary: lipgloss.Color("39"),  // Cyan - org/project info
	Accent:    lipgloss.Color("212"), // Magenta - keybindings, section titles

	// Status colors
	Success: lipgloss.Color("42"),  // Green
	Warning: lipgloss.Color("214"), // Orange
	Error:   lipgloss.Color("196"), // Red
	Info:    lipgloss.Color("33"),  // Blue

	// Background colors
	Background:       lipgloss.Color("236"), // Dark gray
	BackgroundAlt:    lipgloss.Color("235"), // Slightly darker
	BackgroundSelect: lipgloss.Color("57"),  // Dark blue for selection

	// Foreground colors
	Foreground:      lipgloss.Color("252"), // Light gray - main text
	ForegroundMuted: lipgloss.Color("243"), // Gray - disabled/metadata
	ForegroundBold:  lipgloss.Color("255"), // White - emphasized

	// Selection colors
	SelectForeground: lipgloss.Color("229"), // Yellow
	SelectBackground: lipgloss.Color("57"),  // Dark blue

	// UI elements
	Border:  lipgloss.Color("240"), // Gray border
	Link:    lipgloss.Color("81"),  // Blue - hyperlinks
	Spinner: lipgloss.Color("205"), // Magenta

	// Tab colors
	TabActiveForeground:   lipgloss.Color("229"), // Yellow
	TabActiveBackground:   lipgloss.Color("57"),  // Dark blue
	TabInactiveForeground: lipgloss.Color("252"), // Light gray
	TabInactiveBackground: lipgloss.Color("236"), // Dark gray
}

// Gruvbox dark theme
var gruvboxTheme = Theme{
	Name: "gruvbox",

	// Gruvbox palette
	Primary:   lipgloss.Color("#458588"), // Blue
	Secondary: lipgloss.Color("#689d6a"), // Aqua
	Accent:    lipgloss.Color("#d3869b"), // Purple

	// Status colors
	Success: lipgloss.Color("#b8bb26"), // Green
	Warning: lipgloss.Color("#fabd2f"), // Yellow
	Error:   lipgloss.Color("#fb4934"), // Red
	Info:    lipgloss.Color("#83a598"), // Light blue

	// Background colors
	Background:       lipgloss.Color("#282828"), // bg0
	BackgroundAlt:    lipgloss.Color("#1d2021"), // bg0_h
	BackgroundSelect: lipgloss.Color("#3c3836"), // bg1

	// Foreground colors
	Foreground:      lipgloss.Color("#ebdbb2"), // fg
	ForegroundMuted: lipgloss.Color("#928374"), // gray
	ForegroundBold:  lipgloss.Color("#fbf1c7"), // fg0

	// Selection colors
	SelectForeground: lipgloss.Color("#fabd2f"), // Yellow
	SelectBackground: lipgloss.Color("#504945"), // bg2

	// UI elements
	Border:  lipgloss.Color("#504945"), // bg2
	Link:    lipgloss.Color("#83a598"), // Light blue
	Spinner: lipgloss.Color("#d3869b"), // Purple

	// Tab colors
	TabActiveForeground:   lipgloss.Color("#fabd2f"), // Yellow
	TabActiveBackground:   lipgloss.Color("#504945"), // bg2
	TabInactiveForeground: lipgloss.Color("#a89984"), // fg4
	TabInactiveBackground: lipgloss.Color("#282828"), // bg0
}

// Nord theme
var nordTheme = Theme{
	Name: "nord",

	// Nord palette
	Primary:   lipgloss.Color("#81a1c1"), // Nord9 - blue
	Secondary: lipgloss.Color("#88c0d0"), // Nord8 - cyan
	Accent:    lipgloss.Color("#b48ead"), // Nord15 - purple

	// Status colors
	Success: lipgloss.Color("#a3be8c"), // Nord14 - green
	Warning: lipgloss.Color("#ebcb8b"), // Nord13 - yellow
	Error:   lipgloss.Color("#bf616a"), // Nord11 - red
	Info:    lipgloss.Color("#5e81ac"), // Nord10 - dark blue

	// Background colors
	Background:       lipgloss.Color("#2e3440"), // Nord0
	BackgroundAlt:    lipgloss.Color("#3b4252"), // Nord1
	BackgroundSelect: lipgloss.Color("#434c5e"), // Nord2

	// Foreground colors
	Foreground:      lipgloss.Color("#eceff4"), // Nord6
	ForegroundMuted: lipgloss.Color("#4c566a"), // Nord3
	ForegroundBold:  lipgloss.Color("#eceff4"), // Nord6

	// Selection colors
	SelectForeground: lipgloss.Color("#eceff4"), // Nord6
	SelectBackground: lipgloss.Color("#434c5e"), // Nord2

	// UI elements
	Border:  lipgloss.Color("#4c566a"), // Nord3
	Link:    lipgloss.Color("#88c0d0"), // Nord8
	Spinner: lipgloss.Color("#b48ead"), // Nord15

	// Tab colors
	TabActiveForeground:   lipgloss.Color("#eceff4"), // Nord6
	TabActiveBackground:   lipgloss.Color("#5e81ac"), // Nord10
	TabInactiveForeground: lipgloss.Color("#d8dee9"), // Nord4
	TabInactiveBackground: lipgloss.Color("#3b4252"), // Nord1
}

// Dracula theme
var draculaTheme = Theme{
	Name: "dracula",

	// Dracula palette
	Primary:   lipgloss.Color("#bd93f9"), // Purple
	Secondary: lipgloss.Color("#8be9fd"), // Cyan
	Accent:    lipgloss.Color("#ff79c6"), // Pink

	// Status colors
	Success: lipgloss.Color("#50fa7b"), // Green
	Warning: lipgloss.Color("#f1fa8c"), // Yellow
	Error:   lipgloss.Color("#ff5555"), // Red
	Info:    lipgloss.Color("#8be9fd"), // Cyan

	// Background colors
	Background:       lipgloss.Color("#282a36"), // Background
	BackgroundAlt:    lipgloss.Color("#21222c"), // Darker background
	BackgroundSelect: lipgloss.Color("#44475a"), // Current line

	// Foreground colors
	Foreground:      lipgloss.Color("#f8f8f2"), // Foreground
	ForegroundMuted: lipgloss.Color("#6272a4"), // Comment
	ForegroundBold:  lipgloss.Color("#f8f8f2"), // Foreground

	// Selection colors
	SelectForeground: lipgloss.Color("#f8f8f2"), // Foreground
	SelectBackground: lipgloss.Color("#44475a"), // Current line

	// UI elements
	Border:  lipgloss.Color("#6272a4"), // Comment
	Link:    lipgloss.Color("#8be9fd"), // Cyan
	Spinner: lipgloss.Color("#ff79c6"), // Pink

	// Tab colors
	TabActiveForeground:   lipgloss.Color("#f8f8f2"), // Foreground
	TabActiveBackground:   lipgloss.Color("#bd93f9"), // Purple
	TabInactiveForeground: lipgloss.Color("#f8f8f2"), // Foreground
	TabInactiveBackground: lipgloss.Color("#44475a"), // Current line
}

// Catppuccin Mocha theme
var catppuccinTheme = Theme{
	Name: "catppuccin",

	// Catppuccin Mocha palette
	Primary:   lipgloss.Color("#89b4fa"), // Blue
	Secondary: lipgloss.Color("#94e2d5"), // Teal
	Accent:    lipgloss.Color("#cba6f7"), // Mauve

	// Status colors
	Success: lipgloss.Color("#a6e3a1"), // Green
	Warning: lipgloss.Color("#f9e2af"), // Yellow
	Error:   lipgloss.Color("#f38ba8"), // Red
	Info:    lipgloss.Color("#89dceb"), // Sky

	// Background colors
	Background:       lipgloss.Color("#1e1e2e"), // Base
	BackgroundAlt:    lipgloss.Color("#181825"), // Mantle
	BackgroundSelect: lipgloss.Color("#313244"), // Surface0

	// Foreground colors
	Foreground:      lipgloss.Color("#cdd6f4"), // Text
	ForegroundMuted: lipgloss.Color("#6c7086"), // Overlay0
	ForegroundBold:  lipgloss.Color("#cdd6f4"), // Text

	// Selection colors
	SelectForeground: lipgloss.Color("#cdd6f4"), // Text
	SelectBackground: lipgloss.Color("#45475a"), // Surface1

	// UI elements
	Border:  lipgloss.Color("#585b70"), // Surface2
	Link:    lipgloss.Color("#89dceb"), // Sky
	Spinner: lipgloss.Color("#f5c2e7"), // Pink

	// Tab colors
	TabActiveForeground:   lipgloss.Color("#1e1e2e"), // Base
	TabActiveBackground:   lipgloss.Color("#89b4fa"), // Blue
	TabInactiveForeground: lipgloss.Color("#bac2de"), // Subtext1
	TabInactiveBackground: lipgloss.Color("#313244"), // Surface0
}

// GitHub Dark theme
var githubTheme = Theme{
	Name: "github",

	// GitHub Dark palette
	Primary:   lipgloss.Color("#58a6ff"), // Blue
	Secondary: lipgloss.Color("#56d4dd"), // Cyan
	Accent:    lipgloss.Color("#bc8cff"), // Purple

	// Status colors
	Success: lipgloss.Color("#3fb950"), // Green
	Warning: lipgloss.Color("#d29922"), // Yellow/Orange
	Error:   lipgloss.Color("#f85149"), // Red
	Info:    lipgloss.Color("#58a6ff"), // Blue

	// Background colors
	Background:       lipgloss.Color("#0d1117"), // Default background
	BackgroundAlt:    lipgloss.Color("#161b22"), // Darker background
	BackgroundSelect: lipgloss.Color("#21262d"), // Selection

	// Foreground colors
	Foreground:      lipgloss.Color("#c9d1d9"), // Default text
	ForegroundMuted: lipgloss.Color("#8b949e"), // Muted text
	ForegroundBold:  lipgloss.Color("#f0f6fc"), // Bright text

	// Selection colors
	SelectForeground: lipgloss.Color("#f0f6fc"), // Bright text
	SelectBackground: lipgloss.Color("#264f78"), // Selection blue

	// UI elements
	Border:  lipgloss.Color("#30363d"), // Border
	Link:    lipgloss.Color("#58a6ff"), // Blue
	Spinner: lipgloss.Color("#bc8cff"), // Purple

	// Tab colors
	TabActiveForeground:   lipgloss.Color("#f0f6fc"), // Bright text
	TabActiveBackground:   lipgloss.Color("#58a6ff"), // Blue
	TabInactiveForeground: lipgloss.Color("#8b949e"), // Muted text
	TabInactiveBackground: lipgloss.Color("#21262d"), // Selection
}
