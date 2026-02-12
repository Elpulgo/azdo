package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Organization    string `mapstructure:"organization"`
	Project         string `mapstructure:"project"`
	PollingInterval int    `mapstructure:"polling_interval"`
	Theme           string `mapstructure:"theme"`
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

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create a new viper instance to avoid state pollution
	v := viper.New()

	// Configure viper
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	// Set defaults
	v.SetDefault("polling_interval", DefaultPollingInterval)
	v.SetDefault("theme", DefaultTheme)

	// Read config file - return error if not found
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil, fmt.Errorf("config file not found at: %s\nPlease create a config.yaml file with 'organization' and 'project' settings", configPath)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal config into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration values are valid
func (c *Config) Validate() error {
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
