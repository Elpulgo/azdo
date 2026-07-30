package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/spf13/viper"
)

// ErrConfigNotFound is returned when the config file does not exist.
var ErrConfigNotFound = errors.New("config file not found")

// GitHubConfig holds the GitHub-specific configuration.
// TypePrefix and PriorityPrefix are left empty by default so that task 4 can
// fall back to github.DefaultLabelConvention() — a zero-value LabelConvention
// routes all labels to tags, which is the safe default before a user configures
// custom prefixes. Do NOT set viper defaults for these fields here.
type GitHubConfig struct {
	Repos          []string `mapstructure:"repos"`           // "owner/repo" slugs
	TypePrefix     string   `mapstructure:"type_prefix"`     // label prefix for item type; empty → use DefaultLabelConvention
	PriorityPrefix string   `mapstructure:"priority_prefix"` // label prefix for priority; empty → use DefaultLabelConvention
}

// Config holds the application configuration
type Config struct {
	Organization    string              `mapstructure:"organization"`
	Project         string              `mapstructure:"project"` // deprecated: use Projects
	Projects        []string            `mapstructure:"projects"`
	DisplayNames    map[string]string   `mapstructure:"-"`     // API name → display name
	Terms           map[string]string   `mapstructure:"terms"` // tab/term key → user-facing label
	PollingInterval int                 `mapstructure:"polling_interval"`
	Theme           string              `mapstructure:"theme"`
	DisabledPanes   []string            `mapstructure:"-"` // parsed from comma-separated "disabled_panes"
	Metrics         MetricsConfig       `mapstructure:"metrics"`
	GitHub          GitHubConfig        `mapstructure:"github"`
	Notifications   NotificationsConfig `mapstructure:"notifications"`
	// Warnings collects non-fatal load-time diagnostics — currently just
	// decision 26's unrecognised-exclude_reasons entries. Populated by
	// LoadFrom, never persisted (mapstructure:"-"), and never printed by this
	// package (decision 46): a TUI has no safe place to write a line before
	// or after Bubble Tea's alt-screen switch, so task 13 renders these in
	// the notifications pane instead.
	Warnings   []string `mapstructure:"-"`
	configPath string   // internal field to store config path for saving
}

// HasAzure reports whether Azure DevOps is fully configured (org AND projects
// are both present). Used by Validate and main.go to decide whether to build
// an Azure backend.
func (c *Config) HasAzure() bool {
	return c.Organization != "" && len(c.Projects) > 0
}

// HasGitHub reports whether GitHub is configured (at least one repo slug).
// Used by Validate and main.go to decide whether to build a GitHub backend.
func (c *Config) HasGitHub() bool {
	return len(c.GitHub.Repos) > 0
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

// NotificationsConfig holds the filters and cadence override for the
// Notifications tab's merged inbox (spec 20260729-notif-p1-github.md, task
// 9). Every field's zero value is the widest possible behaviour — whole
// inbox, nothing filtered, pane on — per the spec's Config shape section.
// There is deliberately no `enabled` key here: the pane disables the same
// way every other default-on pane does, via `disabled_panes: notifications`
// (decision 16).
type NotificationsConfig struct {
	// OnlyConfiguredRepos restricts the merged feed to repos listed under
	// github.repos. False (default) shows the whole inbox, including rows
	// from repos this config never built a client for (decisions 2, 3).
	OnlyConfiguredRepos bool `mapstructure:"only_configured_repos"`
	// ExcludeRepos is a glob list ("owner/repo" pattern, path.Match syntax —
	// validated where it is used, task 10) of repos to hide from the feed.
	// Empty (default) excludes nothing.
	ExcludeRepos []string `mapstructure:"exclude_repos"`
	// IncludeRepos is a glob list narrowing the feed to matching repos.
	// Empty (default) narrows nothing.
	IncludeRepos []string `mapstructure:"include_repos"`
	// ExcludeReasons lists the neutral NotificationReason string names
	// (decision 19, e.g. "subscribed", "ci_activity") to trim from the
	// merged feed client-side (task 10). Empty (default) filters no reason.
	//
	// This slice is sanitized during LoadFrom per decision 26: any entry
	// provider.ParseNotificationReason does not recognise (a typo, or the
	// reserved "unknown") is removed here and reported via Config.Warnings
	// instead of being kept around for task 10 to reinterpret. That keeps
	// task 10's contract simple — every string surviving in this slice is
	// guaranteed parseable — while still leaving "an unrecognised reason is
	// only filtered when `other` is listed explicitly" satisfiable, since a
	// literal "other" entry is recognised and is never removed.
	ExcludeReasons []string `mapstructure:"exclude_reasons"`
	// UnreadOnly filters the merged feed to unread rows only, client-side.
	// The fetch itself always requests the full inbox regardless of this
	// setting (decision 12); false (default) shows both read and unread.
	UnreadOnly bool `mapstructure:"unread_only"`
	// ParticipatingOnly narrows the server-side fetch to GitHub's
	// "participating" bundle — roughly everything except `subscribed` — and
	// composes with, rather than replaces, ExcludeReasons (decision 10).
	// False (default) fetches the whole inbox.
	ParticipatingOnly bool `mapstructure:"participating_only"`
	// SinceDays bounds the feed to notifications updated within the last N
	// days. Zero (default) means no bound.
	SinceDays int `mapstructure:"since_days"`
	// MaxItems caps the number of notifications returned across all fetched
	// pages, applied after the newest-first sort (decision 44). Zero
	// (default) means no cap.
	MaxItems int `mapstructure:"max_items"`
	// PollInterval overrides the global polling_interval for the
	// notifications poller only. Zero (default) falls back to the backend's
	// X-Poll-Interval hint when present, else the global polling_interval
	// (decision 8).
	PollInterval int `mapstructure:"poll_interval"`
}

// acceptedNotificationReasons returns the eleven configurable
// NotificationReason string names, in enum declaration order, for use in
// decision 26's unrecognised-exclude_reasons warning message. Generated from
// provider.NotificationReason.String() rather than a second hardcoded table
// — provider is the single source of truth for the enum (decision 19), and
// a duplicated string list here would silently drift from it.
//
// The range starts at NotificationReasonReviewRequested to skip the
// NotificationReasonUnknown zero value: "unknown" is reserved and
// ParseNotificationReason rejects it (decision 26), so including it here would
// make the warning self-contradicting — it would advertise as accepted a
// string the parser refuses.
//
// The upper bound comes from provider.NotificationReasonCount(), the sentinel
// that exists for exactly this purpose, rather than from a hardcoded last
// member: a value inserted after NotificationReasonOther is then listed
// automatically instead of being silently omitted from the warning. The
// capacity is derived from the same sentinel, less the excluded Unknown.
func acceptedNotificationReasons() []string {
	names := make([]string, 0, provider.NotificationReasonCount()-1)
	for r := provider.NotificationReasonReviewRequested; r < provider.NotificationReason(provider.NotificationReasonCount()); r++ {
		names = append(names, r.String())
	}
	return names
}

// sanitizeRepoGlobs drops any pattern in patterns that path.Match rejects as
// ErrBadPattern, appending a warning to *warnings naming the offending
// pattern and the config key it came from (decision 51). This mirrors the
// exclude_reasons sanitizer above: task 10's filter contract is "every
// pattern it receives compiles", so a malformed glob is warned about and
// removed here rather than left for match time, where a naive treatment of
// the error could make the pattern match everything instead of nothing.
//
// The probe scope "owner/repo" is representative but arbitrary --
// ErrBadPattern is a property of the pattern syntax alone (e.g. an
// unterminated "["), not of what it is matched against.
func sanitizeRepoGlobs(warnings *[]string, key string, patterns []string) []string {
	if len(patterns) == 0 {
		return patterns
	}
	sanitized := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if _, err := path.Match(p, "owner/repo"); errors.Is(err, path.ErrBadPattern) {
			*warnings = append(*warnings, fmt.Sprintf(
				"%s: dropping malformed pattern %q: %v", key, p, err))
			continue
		}
		sanitized = append(sanitized, p)
	}
	return sanitized
}

// validDisabledPanes lists the pane names that can be disabled.
var validDisabledPanes = map[string]bool{
	"pullrequests":  true,
	"pipelines":     true,
	"workitems":     true,
	"notifications": true,
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
	// These nine registrations are currently no-ops — every notifications
	// default is a Go zero value, which mapstructure leaves in place anyway —
	// and are kept deliberately (decision 48) so the first genuinely non-zero
	// default has a correct landing site instead of being bolted on ad hoc.
	// max_items and since_days stay unbounded on purpose: see decision 48.
	v.SetDefault("notifications.only_configured_repos", false)
	v.SetDefault("notifications.exclude_repos", []string{})
	v.SetDefault("notifications.include_repos", []string{})
	v.SetDefault("notifications.exclude_reasons", []string{})
	v.SetDefault("notifications.unread_only", false)
	v.SetDefault("notifications.participating_only", false)
	v.SetDefault("notifications.since_days", 0)
	v.SetDefault("notifications.max_items", 0)
	v.SetDefault("notifications.poll_interval", 0)

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

	// Decision 26: sanitize notifications.exclude_reasons. An entry
	// provider.ParseNotificationReason does not recognise (a typo, or the
	// reserved "unknown") must never silently act as "other" — it is
	// reported via cfg.Warnings and dropped from the slice here, so task 10
	// never has to reinterpret a bad value. This must never be a hard
	// config error: a typo cannot be allowed to stop the app from starting.
	if len(cfg.Notifications.ExcludeReasons) > 0 {
		sanitized := make([]string, 0, len(cfg.Notifications.ExcludeReasons))
		for _, raw := range cfg.Notifications.ExcludeReasons {
			if _, ok := provider.ParseNotificationReason(raw); ok {
				sanitized = append(sanitized, raw)
				continue
			}
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
				"notifications.exclude_reasons: unrecognised value %q — accepted values: %s",
				raw, strings.Join(acceptedNotificationReasons(), ", ")))
		}
		cfg.Notifications.ExcludeReasons = sanitized
	}

	// Decision 51: drop notifications.exclude_repos / include_repos entries
	// that path.Match rejects as ErrBadPattern, same warn-don't-die channel
	// as the exclude_reasons sanitizer above. An entry that is merely empty
	// after trimming (e.g. "   ") is a valid-but-useless pattern, not a bad
	// one -- Validate below rejects that case as a hard error instead.
	cfg.Notifications.ExcludeRepos = sanitizeRepoGlobs(&cfg.Warnings, "notifications.exclude_repos", cfg.Notifications.ExcludeRepos)
	cfg.Notifications.IncludeRepos = sanitizeRepoGlobs(&cfg.Warnings, "notifications.include_repos", cfg.Notifications.IncludeRepos)

	// Decision 50: only_configured_repos overrides include_repos rather than
	// intersecting with it, so a config setting both is not a conflict --
	// but silently ignoring include_repos would be its own trap, hence the
	// warning rather than staying quiet about it.
	if cfg.Notifications.OnlyConfiguredRepos && len(cfg.Notifications.IncludeRepos) > 0 {
		cfg.Warnings = append(cfg.Warnings,
			"notifications.only_configured_repos is true — notifications.include_repos is ignored")
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

// Validate checks if the configuration values are valid.
//
// Backend requirement (D5): at least one of Azure or GitHub must be configured.
// Azure is present when Organization AND Projects are both non-empty. GitHub is
// present when at least one repo slug is listed. Both may coexist.
//
// Half-configured Azure rule: if Organization or Projects is set but not both,
// that is a user error — both fields are required for a functioning Azure backend.
func (c *Config) Validate() error {
	// --- Backend presence check ---
	azureHasOrg := c.Organization != ""
	azureHasProjects := len(c.Projects) > 0
	azurePartial := azureHasOrg != azureHasProjects // XOR: one set, other not

	// Half-configured Azure: only one of org/projects is present.
	// This is only a fatal error when GitHub is NOT configured — per Decision D5,
	// a partial Azure stanza is silently skipped when GitHub carries the config
	// (HasAzure() returns false so the Azure backend won't be built).
	if azurePartial && !c.HasGitHub() {
		if !azureHasOrg {
			return fmt.Errorf(
				"'organization' is not set in config.yaml\n\n"+
					"Add your Azure DevOps organization name to the config file:\n\n"+
					"  organization: your-org-name\n\n"+
					"For more details, visit: %s", configurationGuideURL)
		}
		// org is set but projects is empty
		return fmt.Errorf(
			"no projects configured in config.yaml\n\n"+
				"Add at least one project to the config file:\n\n"+
				"  projects:\n"+
				"    - your-project-name\n\n"+
				"For more details, visit: %s", configurationGuideURL)
	}

	// Require at least one backend.
	if !c.HasAzure() && !c.HasGitHub() {
		return fmt.Errorf(
			"no backend configured in config.yaml\n\n"+
				"Configure at least one of:\n\n"+
				"  Azure DevOps:\n"+
				"    organization: your-org-name\n"+
				"    projects:\n"+
				"      - your-project-name\n\n"+
				"  GitHub:\n"+
				"    github:\n"+
				"      repos:\n"+
				"        - owner/repo\n\n"+
				"For more details, visit: %s", configurationGuideURL)
	}

	// Validate Azure project names when Azure is configured.
	for i, p := range c.Projects {
		if p == "" {
			return fmt.Errorf("project name at index %d cannot be empty", i)
		}
	}

	// Validate GitHub repo slugs when GitHub is configured.
	// Each slug must be "owner/repo" — exactly one slash, both halves non-empty.
	// Note: SplitN(r, "/", 2) allows "owner/repo/sub" (owner="owner", repo="repo/sub")
	// which is invalid for our purposes, so we check for exactly one slash.
	for _, r := range c.GitHub.Repos {
		parts := strings.SplitN(r, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[1], "/") {
			return fmt.Errorf("invalid github repo slug %q: must be in owner/repo format (no extra slashes)", r)
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
			return fmt.Errorf("invalid disabled pane %q: only 'pullrequests', 'pipelines', 'workitems' and 'notifications' can be disabled", p)
		}
	}

	// At least one pane must remain enabled — otherwise the app would start
	// with no navigable tabs.
	//
	// Notifications counts as a remaining pane only when a
	// notification-capable backend will exist — in phase 1 that means
	// HasGitHub() (decision 47). Decision 11 hides the notifications tab on
	// *capability*, not on config, so a naive four-way guard would let an
	// Azure-only config with the other three panes disabled pass here and
	// then start with zero navigable tabs, since no backend implements
	// NotificationSource to show the notifications tab either. Metrics
	// cannot rescue this: it is separately gated on metrics.enabled.
	//
	// PHASE-1-ONLY: this HasGitHub() coupling is specific to phase 1, where
	// GitHub is the only backend capable of notifications. Once phase 2
	// makes Azure capable too, this condition must widen to "any configured
	// backend" and the special case disappears.
	notificationsCounts := c.IsPaneEnabled("notifications") && c.HasGitHub()
	if !c.IsPaneEnabled("pullrequests") && !c.IsPaneEnabled("workitems") && !c.IsPaneEnabled("pipelines") && !notificationsCounts {
		return fmt.Errorf("cannot disable all panes: at least one of 'pullrequests', 'workitems' or 'pipelines' must remain enabled " +
			"(or leave 'notifications' enabled with a GitHub backend configured — github.repos — since notifications requires GitHub in phase 1)")
	}

	// Notifications validation. There is no `notifications.enabled` guard
	// (decision 16) — the block is always present, if only at its all-zero
	// default, so these checks must hold even when the user never wrote a
	// `notifications:` section at all. That default (0 for each of the
	// three numeric fields, no entries in the two glob lists) already
	// satisfies every check below, so an absent block is always valid.
	if c.Notifications.SinceDays < 0 {
		return fmt.Errorf("notifications.since_days must be >= 0, got %d", c.Notifications.SinceDays)
	}
	if c.Notifications.MaxItems < 0 {
		return fmt.Errorf("notifications.max_items must be >= 0, got %d", c.Notifications.MaxItems)
	}
	if c.Notifications.PollInterval < 0 {
		return fmt.Errorf("notifications.poll_interval must be >= 0, got %d", c.Notifications.PollInterval)
	}
	for _, r := range c.Notifications.ExcludeRepos {
		if strings.TrimSpace(r) == "" {
			return fmt.Errorf("notifications.exclude_repos entries must not be empty")
		}
	}
	for _, r := range c.Notifications.IncludeRepos {
		if strings.TrimSpace(r) == "" {
			return fmt.Errorf("notifications.include_repos entries must not be empty")
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

	// Only persist the github section when at least one repo is configured —
	// mirrors the pattern used for terms/disabled_panes and ensures Azure-only
	// configs don't gain an empty github: {} block on every Save.
	if c.HasGitHub() {
		ghMap := map[string]interface{}{
			"repos": c.GitHub.Repos,
		}
		if c.GitHub.TypePrefix != "" {
			ghMap["type_prefix"] = c.GitHub.TypePrefix
		}
		if c.GitHub.PriorityPrefix != "" {
			ghMap["priority_prefix"] = c.GitHub.PriorityPrefix
		}
		v.Set("github", ghMap)
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
