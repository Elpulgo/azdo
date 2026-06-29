package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ErrConfigNotFound is returned when the config file does not exist.
var ErrConfigNotFound = errors.New("config file not found")

// Config holds the application configuration
type Config struct {
	Organization    string            `mapstructure:"organization"`
	Project         string            `mapstructure:"project"` // deprecated: use Projects
	Projects        []string          `mapstructure:"projects"`
	DisplayNames    map[string]string `mapstructure:"-"`        // API name → display name
	Terms           map[string]string `mapstructure:"terms"`    // tab/term key → user-facing label
	PollingInterval int               `mapstructure:"polling_interval"`
	Theme           string            `mapstructure:"theme"`
	DisabledPanes   []string          `mapstructure:"-"` // parsed from comma-separated "disabled_panes"
	Metrics         MetricsConfig     `mapstructure:"metrics"`
	configPath      string            // internal field to store config path for saving
}

// MetricsConfig holds opt-in settings for the metrics dashboard tab.
// The tab is hidden entirely unless Enabled is true.
type MetricsConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	IntervalDays       int           `mapstructure:"interval_days"`         // window for points-closed / velocity
	ActiveStaleDays    int           `mapstructure:"active_stale_days"`     // dwell in Active above this flags the item
	RFTStaleDays       int           `mapstructure:"rft_stale_days"`        // dwell in Ready for Test above this flags the item
	WIPLimit           int           `mapstructure:"wip_limit"`             // in-flight strictly above this marks a user overloaded
	RunOneShotBackfill bool          `mapstructure:"run_one_shot_backfill"` // PR 3: opt-in 90-day /updates seed
	States             MetricsStates `mapstructure:"states"`                // PR 4: configurable state names
	StateLabels        MetricsStates `mapstructure:"state_labels"`          // PR 4: column-header abbreviations (auto-derived when absent)
}

// MetricsStates holds the canonical name (or label override) for each of the
// three workflow states the metrics tab buckets on. Empty values fall back to
// defaults during config load.
type MetricsStates struct {
	Active       string `mapstructure:"active"`
	ReadyForTest string `mapstructure:"ready_for_test"`
	Closed       string `mapstructure:"closed"`
}

// validDisabledPanes lists the pane names that can be disabled.
var validDisabledPanes = map[string]bool{
	"pipelines": true,
	"workitems": true,
}

// IsPaneEnabled returns true if the given pane is not in the disabled list.
func (c *Config) IsPaneEnabled(pane string) bool {
	for _, p := range c.DisabledPanes {
		if p == pane {
			return false
		}
	}
	return true
}

// IsMultiProject returns true when more than one project is configured.
func (c *Config) IsMultiProject() bool {
	return len(c.Projects) > 1
}

// DisplayNameFor returns the display name for a project API name.
// If no display name is configured, returns the API name itself.
func (c *Config) DisplayNameFor(apiName string) string {
	if c.DisplayNames != nil {
		if dn, ok := c.DisplayNames[apiName]; ok {
			return dn
		}
	}
	return apiName
}

// TermFor returns the configured label for a term key, or the fallback
// when no override is configured. An empty-string override also falls
// through to the fallback so that a misconfigured entry doesn't erase labels.
func (c *Config) TermFor(key, fallback string) string {
	if c.Terms != nil {
		if t, ok := c.Terms[key]; ok && t != "" {
			return t
		}
	}
	return fallback
}

// parseProjects parses the raw "projects" value from YAML which can be:
//   - a list of strings: ["proj-a", "proj-b"]
//   - a list of objects: [{name: "api-name", display_name: "Friendly"}]
//   - a mixed list of both
//
// Returns the list of API names and a map of API name → display name
// (only for projects that have a different display name).
func parseProjects(raw []interface{}) ([]string, map[string]string) {
	projects := make([]string, 0, len(raw))
	displayNames := make(map[string]string)

	for _, item := range raw {
		switch v := item.(type) {
		case string:
			projects = append(projects, v)
		case map[interface{}]interface{}:
			name, _ := v["name"].(string)
			if name == "" {
				continue
			}
			projects = append(projects, name)
			if dn, ok := v["display_name"].(string); ok && dn != "" && dn != name {
				displayNames[name] = dn
			}
		case map[string]interface{}:
			name, _ := v["name"].(string)
			if name == "" {
				continue
			}
			projects = append(projects, name)
			if dn, ok := v["display_name"].(string); ok && dn != "" && dn != name {
				displayNames[name] = dn
			}
		}
	}

	if len(displayNames) == 0 {
		displayNames = nil
	}
	return projects, displayNames
}

// Default configuration values
const (
	DefaultPollingInterval = 60 // seconds
	DefaultTheme           = "dark"

	DefaultMetricsIntervalDays    = 14 // days
	DefaultMetricsActiveStaleDays = 3
	DefaultMetricsRFTStaleDays    = 2
	DefaultMetricsWIPLimit        = 4

	DefaultMetricsActiveState       = "Active"
	DefaultMetricsReadyForTestState = "Ready for Test"
	DefaultMetricsClosedState       = "Closed"
)

// GetPath returns the path to the config file
func GetPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "azdo-tui", "config.yaml"), nil
}

// Load reads the configuration from ~/.config/azdo-tui/config.yaml
// Returns an error if the file doesn't exist, showing the expected path
func Load() (*Config, error) {
	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Set config file location
	configDir := filepath.Join(homeDir, ".config", "azdo-tui")
	configPath := filepath.Join(configDir, "config.yaml")

	return LoadFrom(configPath)
}

// LoadFrom reads the configuration from a specific path
// This is useful for testing or custom config locations
func LoadFrom(configPath string) (*Config, error) {
	configDir := filepath.Dir(configPath)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create a new viper instance to avoid state pollution
	v := viper.New()

	// Configure viper
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Set defaults
	v.SetDefault("polling_interval", DefaultPollingInterval)
	v.SetDefault("theme", DefaultTheme)
	v.SetDefault("metrics.enabled", false)
	v.SetDefault("metrics.interval_days", DefaultMetricsIntervalDays)
	v.SetDefault("metrics.active_stale_days", DefaultMetricsActiveStaleDays)
	v.SetDefault("metrics.rft_stale_days", DefaultMetricsRFTStaleDays)
	v.SetDefault("metrics.wip_limit", DefaultMetricsWIPLimit)
	v.SetDefault("metrics.run_one_shot_backfill", false)
	v.SetDefault("metrics.states.active", DefaultMetricsActiveState)
	v.SetDefault("metrics.states.ready_for_test", DefaultMetricsReadyForTestState)
	v.SetDefault("metrics.states.closed", DefaultMetricsClosedState)

	// Read config file - return error if not found
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok || os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, configPath)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Check if "projects" contains object entries (with display_name).
	// We need to parse the raw value before mapstructure unmarshalling,
	// which only handles string lists.
	var parsedProjects []string
	var parsedDisplayNames map[string]string
	hasObjectEntries := false
	if rawProjects := v.Get("projects"); rawProjects != nil {
		if rawSlice, ok := rawProjects.([]interface{}); ok {
			parsedProjects, parsedDisplayNames = parseProjects(rawSlice)
			// Check if any entry was an object (non-string)
			for _, item := range rawSlice {
				if _, isStr := item.(string); !isStr {
					hasObjectEntries = true
					break
				}
			}
		}
	}

	// If projects contains object entries, clear it from viper before
	// unmarshalling so mapstructure doesn't choke on non-string entries.
	if hasObjectEntries {
		v.Set("projects", []string{})
	}

	// Unmarshal config into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Restore parsed projects (always, since we parsed them above)
	if len(parsedProjects) > 0 {
		cfg.Projects = parsedProjects
		cfg.DisplayNames = parsedDisplayNames
	}

	// Store the config path for saving
	cfg.configPath = configPath

	// Backward compatibility: migrate single "project" to "projects" list
	if len(cfg.Projects) == 0 && cfg.Project != "" {
		cfg.Projects = []string{cfg.Project}
	}
	cfg.Project = "" // clear deprecated field

	// Parse disabled_panes (comma-separated string)
	if raw := v.GetString("disabled_panes"); raw != "" {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.DisabledPanes = append(cfg.DisabledPanes, p)
			}
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// NewWithPath creates a Config with all fields set and the internal configPath
// populated so that Save() writes to the correct location.
func NewWithPath(org string, projects []string, pollingInterval int, theme string, configPath string) *Config {
	return &Config{
		Organization:    org,
		Projects:        projects,
		PollingInterval: pollingInterval,
		Theme:           theme,
		configPath:      configPath,
	}
}

// configurationGuideURL is the link to the GitHub configuration documentation.
const configurationGuideURL = "https://github.com/Elpulgo/azdo#configuration"

// Validate checks if the configuration values are valid
func (c *Config) Validate() error {
	if c.Organization == "" {
		return fmt.Errorf(
			"'organization' is not set in config.yaml\n\n"+
				"Add your Azure DevOps organization name to the config file:\n\n"+
				"  organization: your-org-name\n\n"+
				"For more details, visit: %s", configurationGuideURL)
	}

	if len(c.Projects) == 0 {
		return fmt.Errorf(
			"no projects configured in config.yaml\n\n"+
				"Add at least one project to the config file:\n\n"+
				"  projects:\n"+
				"    - your-project-name\n\n"+
				"For more details, visit: %s", configurationGuideURL)
	}

	for i, p := range c.Projects {
		if p == "" {
			return fmt.Errorf("project name at index %d cannot be empty", i)
		}
	}

	if c.PollingInterval <= 0 {
		return fmt.Errorf("polling_interval must be greater than 0, got %d", c.PollingInterval)
	}

	if c.Theme == "" {
		return fmt.Errorf("theme cannot be empty")
	}

	for _, p := range c.DisabledPanes {
		if !validDisabledPanes[p] {
			return fmt.Errorf("invalid disabled pane %q: only 'pipelines' and 'workitems' can be disabled", p)
		}
	}

	if c.Metrics.Enabled {
		if c.Metrics.IntervalDays <= 0 {
			return fmt.Errorf("metrics.interval_days must be > 0, got %d", c.Metrics.IntervalDays)
		}
		if c.Metrics.ActiveStaleDays < 0 {
			return fmt.Errorf("metrics.active_stale_days must be >= 0, got %d", c.Metrics.ActiveStaleDays)
		}
		if c.Metrics.RFTStaleDays < 0 {
			return fmt.Errorf("metrics.rft_stale_days must be >= 0, got %d", c.Metrics.RFTStaleDays)
		}
		if c.Metrics.WIPLimit <= 0 {
			return fmt.Errorf("metrics.wip_limit must be > 0, got %d", c.Metrics.WIPLimit)
		}
		if err := validateStateName("metrics.states.active", c.Metrics.States.Active); err != nil {
			return err
		}
		if err := validateStateName("metrics.states.ready_for_test", c.Metrics.States.ReadyForTest); err != nil {
			return err
		}
		if err := validateStateName("metrics.states.closed", c.Metrics.States.Closed); err != nil {
			return err
		}
		names := []string{
			strings.ToLower(strings.TrimSpace(c.Metrics.States.Active)),
			strings.ToLower(strings.TrimSpace(c.Metrics.States.ReadyForTest)),
			strings.ToLower(strings.TrimSpace(c.Metrics.States.Closed)),
		}
		for i := range names {
			for j := i + 1; j < len(names); j++ {
				if names[i] == names[j] {
					return fmt.Errorf("metrics.states: duplicate state name %q — Active / Ready for Test / Closed must each be distinct", names[i])
				}
			}
		}
	}

	return nil
}

// validateStateName guards the configured names against empty values and
// single quotes (which would break the WIQL `IN ('...','...')` literal).
func validateStateName(key, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%s must not be empty", key)
	}
	if strings.ContainsRune(trimmed, '\'') {
		return fmt.Errorf("%s contains a single quote (%q) — not allowed (WIQL safety)", key, name)
	}
	return nil
}

// GetTheme returns the configured theme name.
// Returns the default theme if the theme is empty.
func (c *Config) GetTheme() string {
	if c.Theme == "" {
		return DefaultTheme
	}
	return c.Theme
}

// Save writes the current configuration to the config file
func (c *Config) Save() error {
	// Validate before saving
	if err := c.Validate(); err != nil {
		return fmt.Errorf("cannot save invalid config: %w", err)
	}

	// Get config path - use stored path if available, otherwise get default
	configPath := c.configPath
	if configPath == "" {
		var err error
		configPath, err = GetPath()
		if err != nil {
			return fmt.Errorf("failed to get config path: %w", err)
		}
	}

	// Create a new viper instance for writing
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Round-trip any existing file first so keys we don't explicitly manage
	// here — the entire metrics section, navigation state, future additions —
	// are preserved. Without this, every Save (e.g. a theme change) rewrites
	// the file from scratch and silently deletes the user's metrics config.
	// A missing file is fine: we fall through and create one.
	if _, statErr := os.Stat(configPath); statErr == nil {
		if readErr := v.ReadInConfig(); readErr != nil {
			return fmt.Errorf("failed to read existing config before save: %w", readErr)
		}
	}

	// Set all config values
	v.Set("organization", c.Organization)

	// Persist projects in new format when display names are configured
	if len(c.DisplayNames) > 0 {
		projectEntries := make([]interface{}, len(c.Projects))
		for i, p := range c.Projects {
			if dn, ok := c.DisplayNames[p]; ok {
				projectEntries[i] = map[string]string{
					"name":         p,
					"display_name": dn,
				}
			} else {
				projectEntries[i] = p
			}
		}
		v.Set("projects", projectEntries)
	} else {
		v.Set("projects", c.Projects)
	}

	v.Set("polling_interval", c.PollingInterval)
	v.Set("theme", c.Theme)

	if len(c.DisabledPanes) > 0 {
		v.Set("disabled_panes", strings.Join(c.DisabledPanes, ","))
	}

	if len(c.Terms) > 0 {
		v.Set("terms", c.Terms)
	}

	// Write config file
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// UpdateTheme updates the theme in the config and saves it
func (c *Config) UpdateTheme(themeName string) error {
	if themeName == "" {
		return fmt.Errorf("theme name cannot be empty")
	}

	c.Theme = themeName

	if err := c.Save(); err != nil {
		return fmt.Errorf("failed to save theme update: %w", err)
	}

	return nil
}
