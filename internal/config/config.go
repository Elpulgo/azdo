package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Organization    string            `mapstructure:"organization"`
	Project         string            `mapstructure:"project"` // deprecated: use Projects
	Projects        []string          `mapstructure:"projects"`
	DisplayNames    map[string]string `mapstructure:"-"` // API name → display name
	PollingInterval int               `mapstructure:"polling_interval"`
	Theme           string            `mapstructure:"theme"`
	configPath      string            // internal field to store config path for saving
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

	// Read config file - return error if not found
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok || os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"config file not found at: %s\n\n"+
					"To get started, create a config file with your Azure DevOps settings:\n\n"+
					"Then set up your Personal Access Token:\n\n"+
					"  azdo auth\n\n"+
					"For more details, visit: https://github.com/Elpulgo/azdo#configuration",
				configPath,
			)
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

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration values are valid
func (c *Config) Validate() error {
	if len(c.Projects) == 0 {
		return fmt.Errorf("at least one project must be configured")
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
