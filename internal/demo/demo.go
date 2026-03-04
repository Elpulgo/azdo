package demo

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/Elpulgo/azdo/internal/app"
	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	demoOrg             = "contoso"
	projectNexus        = "nexus-platform"
	projectHorizon      = "horizon-app"
	displayNexus        = "Nexus Platform"
	displayHorizon      = "Horizon App"
	demoPollingInterval = 3600 // high interval so polling is effectively inert
)

// Run starts the TUI in demo mode with mock data.
func Run(version, commit string) error {
	// Start mock HTTP server
	srv := httptest.NewServer(newMockHandler())
	defer srv.Close()

	projects := []string{projectNexus, projectHorizon}
	displayNames := map[string]string{
		projectNexus:   displayNexus,
		projectHorizon: displayHorizon,
	}

	// Create multi-client with a dummy PAT (mock server ignores auth)
	client, err := azdevops.NewMultiClient(demoOrg, projects, "demo-pat", displayNames)
	if err != nil {
		return fmt.Errorf("failed to create demo client: %w", err)
	}

	// Override base URLs and user IDs to point at mock server
	for _, project := range projects {
		c := client.ClientFor(project)
		c.SetBaseURL(srv.URL)
		c.SetUserID(demoUserID)
	}

	// Create config with a temp path so theme changes don't touch the real config.
	// This allows demo mode to work without any prior setup.
	tmpDir, err := os.MkdirTemp("", "azdo-demo-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.NewWithPath(demoOrg, projects, demoPollingInterval, "dracula", filepath.Join(tmpDir, "config.yaml"))
	cfg.DisplayNames = displayNames

	model := app.NewModel(client, cfg, version+" (demo)", commit)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("demo TUI error: %w", err)
	}

	return nil
}
