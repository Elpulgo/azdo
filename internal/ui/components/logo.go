package components

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

// LogoArt is the ASCII art representation of the "azdo" logo.
var LogoArt = []string{
	"╔═╗╔═╗╔╦╗╔═╗",
	"╠═╣╔═╝ ║║║ ║",
	"╩ ╩╚═╝═╩╝╚═╝",
}

// Logo renders the ASCII art logo for the application.
type Logo struct {
	styles *styles.Styles
}

// NewLogo creates a new Logo component.
func NewLogo(s *styles.Styles) *Logo {
	return &Logo{
		styles: s,
	}
}

// Height returns the number of lines the logo occupies.
func (l *Logo) Height() int {
	return len(LogoArt)
}

// View renders the styled ASCII art logo.
func (l *Logo) View() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(l.styles.Theme.Primary)).
		Bold(true)

	return style.Render(strings.Join(LogoArt, "\n"))
}
