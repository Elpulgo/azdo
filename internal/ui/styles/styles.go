package styles

import "github.com/charmbracelet/lipgloss"

// Styles holds all the lipgloss styles for the application UI.
// These are generated from a Theme and provide consistent styling
// across all components.
type Styles struct {
	// Theme is the source theme for these styles
	Theme Theme

	// Tab styles
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	TabBar      lipgloss.Style

	// Box and container styles
	Box         lipgloss.Style
	BoxRounded  lipgloss.Style
	ModalBox    lipgloss.Style

	// Text styles
	Header       lipgloss.Style
	Title        lipgloss.Style
	Subtitle     lipgloss.Style
	Label        lipgloss.Style
	Value        lipgloss.Style
	Muted        lipgloss.Style
	Bold         lipgloss.Style

	// Status styles
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style

	// Selection styles
	Selected lipgloss.Style

	// Interactive element styles
	Key         lipgloss.Style
	Description lipgloss.Style
	Link        lipgloss.Style

	// UI element styles
	Border      lipgloss.Style
	Spinner     lipgloss.Style
	ScrollInfo  lipgloss.Style

	// Connection state styles
	Connected    lipgloss.Style
	Connecting   lipgloss.Style
	Disconnected lipgloss.Style
	ConnError    lipgloss.Style

	// Table styles
	TableHeader   lipgloss.Style
	TableSelected lipgloss.Style
}

// NewStyles creates a new Styles instance from the given theme.
// All lipgloss styles are pre-computed for efficient rendering.
func NewStyles(theme Theme) *Styles {
	s := &Styles{
		Theme: theme,
	}

	// Tab styles
	s.TabActive = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.TabActiveForeground)).
		Background(lipgloss.Color(theme.TabActiveBackground)).
		Padding(0, 2).
		Bold(true)

	s.TabInactive = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.TabInactiveForeground)).
		Background(lipgloss.Color(theme.TabInactiveBackground)).
		Padding(0, 2)

	s.TabBar = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.Background))

	// Box styles
	s.Box = lipgloss.NewStyle().
		BorderForeground(lipgloss.Color(theme.Border)).
		Background(lipgloss.Color(theme.Background))

	s.BoxRounded = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Accent)).
		Background(lipgloss.Color(theme.Background)).
		Padding(0, 1)

	s.ModalBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Accent)).
		Background(lipgloss.Color(theme.BackgroundAlt)).
		Padding(1, 2)

	// Text styles
	s.Header = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Primary)).
		Bold(true)

	s.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Primary)).
		Bold(true)

	s.Subtitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Secondary))

	s.Label = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Warning)).
		Bold(true)

	s.Value = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground))

	s.Muted = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ForegroundMuted))

	s.Bold = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ForegroundBold)).
		Bold(true)

	// Status styles
	s.Success = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Success))

	s.Warning = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Warning))

	s.Error = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Error))

	s.Info = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Info))

	// Selection styles
	s.Selected = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.SelectForeground)).
		Background(lipgloss.Color(theme.SelectBackground)).
		Bold(false)

	// Interactive element styles
	s.Key = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Accent)).
		Background(lipgloss.Color(theme.Background)).
		Bold(true)

	s.Description = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Background(lipgloss.Color(theme.Background))

	s.Link = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Link)).
		Underline(true)

	// UI element styles
	s.Border = lipgloss.NewStyle().
		BorderForeground(lipgloss.Color(theme.Border))

	s.Spinner = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Spinner))

	s.ScrollInfo = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Secondary))

	// Connection state styles
	s.Connected = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Success))

	s.Connecting = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Warning))

	s.Disconnected = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ForegroundMuted))

	s.ConnError = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Error))

	// Table styles
	s.TableHeader = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.Border)).
		BorderBottom(true).
		Bold(false)

	s.TableSelected = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.SelectForeground)).
		Background(lipgloss.Color(theme.SelectBackground)).
		Bold(false)

	return s
}

// DefaultStyles returns styles using the default dark theme.
func DefaultStyles() *Styles {
	return NewStyles(GetDefaultTheme())
}
