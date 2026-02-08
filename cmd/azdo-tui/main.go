package main

import (
	"fmt"
	"os"

	"github.com/Elpulgo/azdo/internal/app"
	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Verify organization and project are configured
	if cfg.Organization == "" {
		return fmt.Errorf("organization not configured\nHint: Set 'organization' in ~/.config/azdo-tui/config.yaml")
	}
	if cfg.Project == "" {
		return fmt.Errorf("project not configured\nHint: Set 'project' in ~/.config/azdo-tui/config.yaml")
	}

	// Get PAT from keyring
	store := config.NewKeyringStore()
	pat, err := store.GetPAT()
	if err != nil {
		return fmt.Errorf("failed to get PAT: %w\nHint: Set PAT in keyring or use environment variable", err)
	}

	// Create Azure DevOps client
	client, err := azdevops.NewClient(cfg.Organization, cfg.Project, pat)
	if err != nil {
		return fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	// Create and run the TUI application
	model := app.NewModel(client)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI application error: %w", err)
	}

	return nil
}
