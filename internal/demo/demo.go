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

	// Redirect metrics persistence (snapshots, sprint selection, backfill
	// marker) into the temp dir so seeding and live snapshotting never touch
	// the user's real ~/.config/azdo-tui history.
	if err := os.Setenv("AZDO_CONFIG_DIR", filepath.Join(tmpDir, "azdo-tui")); err != nil {
		return fmt.Errorf("failed to set demo config dir: %w", err)
	}

	cfg := config.NewWithPath(demoOrg, projects, demoPollingInterval, "dracula", filepath.Join(tmpDir, "config.yaml"))
	cfg.DisplayNames = displayNames
	enableDemoMetrics(cfg)

	// Seed synthetic sprint history so the metrics tab's Trends view has data.
	if err := seedMetricsHistory(); err != nil {
		return fmt.Errorf("failed to seed demo metrics: %w", err)
	}

	model := app.NewModel(client, cfg, version+" (demo)", commit)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("demo TUI error: %w", err)
	}

	return nil
}
