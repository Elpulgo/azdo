package components

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ThemeSelectedMsg is sent when a theme is selected
type ThemeSelectedMsg struct {
	ThemeName string
}

// ThemePicker is a modal component for selecting themes
type ThemePicker struct {
	styles          *styles.Styles
	visible         bool
	width           int
	height          int
	availableThemes []string
	currentTheme    string
	cursor          int
}

// NewThemePicker creates a new theme picker
func NewThemePicker(appStyles *styles.Styles, availableThemes []string, currentTheme string) ThemePicker {
	// Find the index of the current theme to set initial cursor position
	cursor := 0
	for i, theme := range availableThemes {
		if theme == currentTheme {
			cursor = i
			break
		}
	}

	return ThemePicker{
		styles:          appStyles,
		visible:         false,
		availableThemes: availableThemes,
		currentTheme:    currentTheme,
		cursor:          cursor,
	}
}

// Show makes the theme picker visible
func (t *ThemePicker) Show() {
	t.visible = true
}

// Hide makes the theme picker invisible
func (t *ThemePicker) Hide() {
	t.visible = false
}

// IsVisible returns whether the theme picker is visible
func (t ThemePicker) IsVisible() bool {
	return t.visible
}

// SetSize sets the dimensions for centering
func (t *ThemePicker) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// GetCursor returns the current cursor position
func (t ThemePicker) GetCursor() int {
	return t.cursor
}

// Update handles messages
func (t ThemePicker) Update(msg tea.Msg) (ThemePicker, tea.Cmd) {
	if !t.visible {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			t.visible = false
			return t, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if t.cursor > 0 {
				t.cursor--
			}
			return t, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if t.cursor < len(t.availableThemes)-1 {
				t.cursor++
			}
			return t, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			selectedTheme := t.availableThemes[t.cursor]
			t.visible = false
			return t, func() tea.Msg {
				return ThemeSelectedMsg{ThemeName: selectedTheme}
			}
		}
	}

	return t, nil
}

// View renders the theme picker
func (t ThemePicker) View() string {
	if !t.visible {
		return ""
	}

	// Build the theme list
	var themeList string
	for i, themeName := range t.availableThemes {
		cursor := " "
		if i == t.cursor {
			cursor = ">"
		}

		isCurrent := ""
		if themeName == t.currentTheme {
			isCurrent = " (current)"
		}

		line := fmt.Sprintf("%s %s%s", cursor, themeName, isCurrent)

		if i == t.cursor {
			// Highlight selected item
			line = lipgloss.NewStyle().
				Foreground(t.styles.Theme.GetSelectForeground()).
				Background(t.styles.Theme.GetSelectBackground()).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(t.styles.Theme.GetForeground()).
				Render(line)
		}

		themeList += line + "\n"
	}

	// Build the modal content
	title := lipgloss.NewStyle().
		Foreground(t.styles.Theme.GetPrimary()).
		Bold(true).
		Render("Select Theme")

	helpText := lipgloss.NewStyle().
		Foreground(t.styles.Theme.GetForegroundMuted()).
		Render("↑/↓: navigate • enter: select • esc/q: cancel")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		themeList,
		helpText,
	)

	// Create modal box
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.styles.Theme.GetBorder()).
		Padding(1, 2).
		Background(t.styles.Theme.GetBackground())

	modal := modalStyle.Render(content)

	// Center the modal
	if t.width > 0 && t.height > 0 {
		modal = lipgloss.Place(
			t.width,
			t.height,
			lipgloss.Center,
			lipgloss.Center,
			modal,
		)
	}

	return modal
}
