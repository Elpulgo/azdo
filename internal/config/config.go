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
// TypePrefix and PriorityPrefix are left empty by default so the label mapping
// falls back to github.DefaultLabelConvention() — a zero-value LabelConvention
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
	// Warnings collects non-fatal load-time diagnostics — currently an
	// unrecognised notifications.exclude_reasons entry, a malformed
	// notifications.exclude_repos/include_repos glob,
	// notifications.github.only_configured_repos silently overriding a
	// non-empty notifications.include_repos, and
	// notifications.azure.lookback_days being clamped down to
	// AzureLookbackDaysMax. Populated by LoadFrom, never persisted
	// (mapstructure:"-"), and never printed by this package: a TUI has no
	// safe place to write a line before or after Bubble Tea's alt-screen
	// switch, so the notifications pane renders these instead.
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
// Notifications tab's merged inbox. Every field's zero value is the widest
// possible behaviour — whole inbox, nothing filtered, pane on. There is
// deliberately no `enabled` key here: the pane disables the same way every
// other default-on pane does, via `disabled_panes: notifications`.
//
// Only the fields below apply to the merged feed regardless of backend
// (decision 13 of the phase-2 spec). Provider-specific knobs live in the
// nested GitHub and Azure blocks: `participating_only`, `only_configured_repos`
// and `since_days` moved to notifications.github because none of the three
// has a meaningful Azure equivalent — see NotificationsGitHubConfig's doc
// comment. That does NOT mean the github block is invisible to non-GitHub
// rows: FilterNotifications runs on the already-merged feed, so a knob under
// notifications.github must itself check Identity.Kind to stay GitHub-only —
// only_configured_repos does exactly that. There is no migration for the old
// flat keys: a bare `notifications.participating_only` (etc.) is simply an
// unrecognised key and is silently ignored, not honoured and not warned
// about, on load (decision 14). That is only true of the read path — Save()
// round-trips whatever the file already contains, so an orphaned flat key
// survives in the file forever rather than disappearing on the next save
// (pinned by TestConfigSave_PreservesKeysOutsideTheConfigStruct).
type NotificationsConfig struct {
	// ExcludeReasons lists the neutral NotificationReason string names (e.g.
	// "subscribed", "ci_activity") to trim from the merged feed client-side.
	// Empty (default) filters no reason.
	//
	// This slice is sanitized during LoadFrom: any entry
	// provider.ParseNotificationReason does not recognise (a typo, or the
	// reserved "unknown") is removed here and reported via Config.Warnings
	// rather than kept around to reinterpret at filter time. Every string
	// surviving here is guaranteed parseable, while a literal "other" entry is
	// recognised and never removed.
	ExcludeReasons []string `mapstructure:"exclude_reasons"`
	// UnreadOnly filters the merged feed to unread rows only, client-side.
	// The fetch itself always requests the full inbox regardless of this
	// setting; false (default) shows both read and unread.
	UnreadOnly bool `mapstructure:"unread_only"`
	// ExcludeRepos is a glob list of scopes to hide from the feed
	// (path.Match syntax). On GitHub a scope is "owner/repo"; on Azure it is
	// a project name. Empty (default) excludes nothing.
	ExcludeRepos []string `mapstructure:"exclude_repos"`
	// IncludeRepos is a glob list narrowing the feed to matching scopes, same
	// "owner/repo" (GitHub) / project name (Azure) shape as ExcludeRepos.
	// Empty (default) narrows nothing.
	IncludeRepos []string `mapstructure:"include_repos"`
	// MaxItems caps the number of notifications in the merged, sorted feed
	// across every backend, applied after the newest-first sort. Zero
	// (default) means no cap.
	MaxItems int `mapstructure:"max_items"`
	// PollInterval overrides the global polling_interval for the
	// notifications poller only. The effective cadence is not simply "this
	// value, or a fallback if zero" — internal/app.notificationsPollInterval
	// computes max(configured, backend hint) unconditionally, so a capable
	// backend's cadence hint (e.g. GitHub's X-Poll-Interval response header)
	// can still raise the interval above a positive PollInterval that is
	// lower than the hint; the hint never gets overridden just because this
	// field is set. "configured" itself falls back poll_interval (this
	// field) -> polling_interval (the global PollingInterval) ->
	// polling.DefaultInterval, whichever is the first positive value. The
	// final result is additionally floored at polling.MinInterval by
	// NewNotificationsPoller/SetInterval, so a misconfigured poll_interval: 1
	// cannot produce sub-minimum polling. Zero (default) means "no override";
	// see internal/app.notificationsConfiguredInterval and
	// notificationsPollInterval for the exact arithmetic.
	PollInterval int `mapstructure:"poll_interval"`

	// GitHub holds GitHub-only notifications knobs — the phase-1 keys that
	// have no Azure equivalent.
	GitHub NotificationsGitHubConfig `mapstructure:"github"`
	// Azure holds Azure-only notifications knobs, matching decision 13's
	// YAML exactly. Disabling every entry under Azure.Sources is legal and
	// yields an empty Azure share of the feed, not a config error — nothing
	// here gates whether the Azure backend implements
	// provider.NotificationSource, since that is a compile-time method-set
	// fact about *azdevops.Adapter, not a config-driven capability. An
	// Azure-only config with every source toggle off still shows the
	// Notifications tab; it is simply always empty.
	Azure NotificationsAzureConfig `mapstructure:"azure"`
}

// NotificationsGitHubConfig holds the notifications knobs that apply to the
// GitHub backend only. Each was a top-level notifications.* key in phase 1;
// decision 13 of the phase-2 spec moved them here because none generalises
// to Azure: ParticipatingOnly names a GitHub inbox concept with no Azure
// analogue; OnlyConfiguredRepos narrows only the GitHub rows of the merged
// feed — FilterNotifications checks Identity.Kind before applying it, since
// an Azure row's Scope is a project name, never an "owner/repo" the
// github.repos list could match, so treating a Kind mismatch as "excluded"
// would silently delete every Azure row; and SinceDays's zero value ("no
// bound") would be catastrophic applied to Azure's assigned-work-item query,
// which is why Azure gets its own lookback_days instead of sharing this
// field.
type NotificationsGitHubConfig struct {
	// ParticipatingOnly narrows the server-side fetch to GitHub's
	// "participating" bundle — roughly everything except `subscribed` — and
	// composes with, rather than replaces, ExcludeReasons.
	// False (default) fetches the whole inbox.
	ParticipatingOnly bool `mapstructure:"participating_only"`
	// OnlyConfiguredRepos restricts the GitHub share of the merged feed to
	// repos listed under github.repos; rows from any other backend are
	// unaffected regardless of their Scope (FilterNotifications gates this on
	// Identity.Kind == provider.KindGitHub). False (default) shows the whole
	// inbox, including GitHub rows from repos this config never built a
	// client for.
	OnlyConfiguredRepos bool `mapstructure:"only_configured_repos"`
	// SinceDays bounds the feed to notifications updated within the last N
	// days. Zero (default) means no bound.
	SinceDays int `mapstructure:"since_days"`
}

// NotificationsAzureConfig holds the notifications knobs that apply to the
// Azure DevOps backend only, shaped to match decision 13's YAML exactly.
// Both numeric fields have LoadFrom v.SetDefault registrations, so an absent
// notifications.azure block (or one that omits a given key) lands on a
// non-zero default, not the Go zero value — see each field's own comment for
// the exact number and what a user-supplied zero means.
type NotificationsAzureConfig struct {
	// LookbackDays bounds two of the four Azure notification sources --
	// SourceAssigned and SourceCIFailed -- to activity within the last N
	// days. SourceReviewRequested ("open PRs where I am a reviewer") is
	// unbounded by time, and SourceMentioned's stage-1 WIQL is
	// @RecentMentions with no date clause of its own; neither reads this
	// field. Defaults to DefaultAzureLookbackDays (14). Unlike
	// notifications.github.since_days, zero is NOT "unbounded" here: LoadFrom
	// treats an explicit `lookback_days: 0` the same as an absent key and
	// falls back to the default, because Azure's sources have no snapshot
	// state and an unbounded window means "every work item ever assigned to
	// me" (decision 13's closing note). LoadFrom also clamps a value above
	// AzureLookbackDaysMax (30, mirroring azdevops.orphanTTL) down to it and
	// records a Config.Warnings entry when that clamp actually fires.
	//
	// Both of those are LoadFrom post-conditions, not a guarantee this field
	// carries everywhere: by the time Validate() runs a value that reached
	// LoadFrom's body is negative (a config error Validate() rejects below)
	// or in [1, 30], but a *Config not built via LoadFrom -- NewWithPath's
	// setup-wizard constructor, or any bare struct literal -- has had none of
	// this normalization applied and may simply be the Go zero value, 0.
	LookbackDays int `mapstructure:"lookback_days"`
	// MinPollInterval is the shortest interval, in seconds, between two real
	// Azure notification queries; the adapter self-throttles to it (task 14
	// wires this value into the adapter — this struct only parses, defaults
	// and validates it). Defaults to DefaultAzureMinPollInterval (300).
	// Like LookbackDays, zero is NOT "no self-throttle" here: LoadFrom
	// treats an explicit `min_poll_interval: 0` the same as an absent key
	// and falls back to the default, because a literal zero would mean the
	// adapter never throttles at all (task-11 review decision B). This
	// field's own Validate() check only rejects a negative value — the
	// zero-fallback happens earlier, in LoadFrom.
	MinPollInterval int `mapstructure:"min_poll_interval"`
	// Sources holds the per-source enable toggles, all defaulting to true.
	Sources NotificationsAzureSourcesConfig `mapstructure:"sources"`
}

// NotificationsAzureSourcesConfig holds the four independent per-source
// enable toggles for the Azure notifications feed. All four default to true
// (LoadFrom registers a v.SetDefault for each), so an absent
// notifications.azure.sources block — or one that sets only some of the four
// keys — leaves every unset toggle on. There is no validation here: every
// bool value is legal, including all four false, which is not a config error
// and does not affect whether the Azure backend implements
// provider.NotificationSource (see NotificationsConfig.Azure's doc comment).
//
// CIFailed names a *source* ("my failed pipeline runs"), not a
// provider.NotificationReason: that source emits NotificationReasonCIActivity
// (decision 8 of the phase-2 spec), a member that already existed in the
// enum. The config key keeps the ci_failed spelling because it describes what
// the source queries; do not read the two spellings as one vocabulary.
//
// Toggling a source off for longer than AzureLookbackDaysMax (30 days) and
// back on silently discards its triage history. azdevops's Reconcile prunes
// a subject's local read/done state once its LastSeen falls outside
// orphanTTL, and LastSeen only advances for rows a source actually returns —
// a disabled source returns none. Every mention, review or failed run this
// source had surfaced, whether the user had read or dismissed it, comes back
// as unread the moment the toggle flips back on if that gap exceeded the
// prune window. This is a consequence of decision 7's "expiry is implicit in
// recomputation," not a bug, but it is exactly the kind of surprise the user
// making this toggle decision should know about up front.
type NotificationsAzureSourcesConfig struct {
	ReviewRequested bool `mapstructure:"review_requested"`
	Mentioned       bool `mapstructure:"mentioned"`
	Assigned        bool `mapstructure:"assigned"`
	CIFailed        bool `mapstructure:"ci_failed"`
}

// acceptedNotificationReasons returns the eleven configurable
// NotificationReason string names, in enum declaration order, for the
// unrecognised-exclude_reasons warning message. Generated from
// provider.NotificationReason.String() rather than a second hardcoded table —
// provider is the single source of truth for the enum, and a duplicated string
// list here would silently drift from it.
//
// The range starts at NotificationReasonReviewRequested to skip the
// NotificationReasonUnknown zero value: "unknown" is reserved and
// ParseNotificationReason rejects it, so including it here would make the
// warning self-contradicting — it would advertise as accepted a string the
// parser refuses.
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
// pattern, the config key it came from, and the effect the user will actually
// see. This mirrors the exclude_reasons sanitizer above: the filter contract
// is "every pattern it receives compiles", so a malformed glob is warned about
// and removed here rather than left for match time, where a naive treatment of
// the error could make the pattern match everything instead of nothing.
//
// The warnings are emitted after the loop rather than inside it because the
// consequence clause for include_repos depends on whether any compilable
// pattern survived the whole list, which is only known once it has been
// walked.
//
// The probe scope "owner/repo" is representative but arbitrary --
// ErrBadPattern is a property of the pattern syntax alone (e.g. an
// unterminated "["), not of what it is matched against.
func sanitizeRepoGlobs(warnings *[]string, key string, patterns []string) []string {
	if len(patterns) == 0 {
		return patterns
	}
	sanitized := make([]string, 0, len(patterns))
	var dropped []string
	var droppedErrs []error
	for _, p := range patterns {
		if _, err := path.Match(p, "owner/repo"); errors.Is(err, path.ErrBadPattern) {
			dropped = append(dropped, p)
			droppedErrs = append(droppedErrs, err)
			continue
		}
		sanitized = append(sanitized, p)
	}
	effect := repoGlobDropEffect(key, len(sanitized) == 0)
	for i, p := range dropped {
		*warnings = append(*warnings, fmt.Sprintf(
			"%s: dropping malformed pattern %q: %v — %s", key, p, droppedErrs[i], effect))
	}
	return sanitized
}

// repoGlobDropEffect returns the one-clause description of what the user will
// observe after a malformed glob was dropped from key. The pattern and the
// error name the cause; without this the warning never says what changed, and
// the notifications pane renders these lines verbatim, so it stays a clause
// rather than a second sentence.
//
// The two keys differ because the lists are not symmetric: an exclude list
// with a pattern removed simply stops excluding by it, while an include list
// that loses its last pattern stops selecting at all and the filter falls back
// to showing the whole inbox.
func repoGlobDropEffect(key string, noneLeft bool) string {
	if strings.HasSuffix(key, "include_repos") {
		if noneLeft {
			return "no include pattern is left, so the whole inbox is shown"
		}
		return "the remaining include patterns still apply"
	}
	return "nothing is excluded by it"
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

	// DefaultAzureLookbackDays is notifications.azure.lookback_days' default
	// (decision 13's YAML). It intentionally matches
	// azdevops.DefaultNotificationLookbackDays; the two are declared in
	// separate packages (see AzureLookbackDaysMax's comment for why config
	// does not import azdevops) and must be changed together. A one-line
	// equality assertion in cmd/azdo-tui (which already imports both
	// packages) catches the two drifting apart; see
	// TestDefaultAzureLookbackDays_ConfigAndAdapterAgree.
	DefaultAzureLookbackDays = 14
	// DefaultAzureMinPollInterval is notifications.azure.min_poll_interval's
	// default, in seconds (decision 13's YAML).
	DefaultAzureMinPollInterval = 300

	// AzureLookbackDaysMax duplicates azdevops.MaxNotificationLookbackDays
	// (itself derived from azdevops.orphanTTL, 30 days) expressed in days
	// rather than a time.Duration, and is the upper bound
	// notifications.azure.lookback_days is clamped to. config deliberately
	// does not import internal/azdevops to read the real constant -- that
	// would make a leaf config package depend on one backend's internals for
	// a single number, and internal/azdevops does not import internal/config
	// either, so nothing here forces the duplication beyond the choice to
	// keep config free of a backend dependency. Comments alone cannot stop
	// the two drifting apart -- nothing in the build or the type system
	// relates them -- so a one-line equality test in cmd/azdo-tui (which
	// already imports both packages) asserts
	// AzureLookbackDaysMax == azdevops.MaxNotificationLookbackDays; see
	// TestAzureLookbackDaysMax_ConfigAndAdapterAgree. azdevops.orphanTTL's
	// own doc comment points back at this constant, and this comment points
	// back at that one, for a human reading either file in isolation.
	AzureLookbackDaysMax = 30
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
	// The six shared/GitHub registrations below are currently no-ops — every
	// one of those defaults is a Go zero value, which mapstructure leaves in
	// place anyway — and are kept deliberately so a future genuinely
	// non-zero default has a correct landing site instead of being bolted on
	// ad hoc. max_items and notifications.github.since_days stay unbounded
	// on purpose.
	//
	// notifications.azure.* is different: two of its six registrations below
	// are genuinely non-zero (lookback_days, min_poll_interval), and all four
	// source toggles default to true against bool's false zero value — a
	// plain "leave it at the Go zero value" default cannot express
	// "defaults on" at all, only "defaults off". SetDefault is verified to
	// resolve correctly through v.Unmarshal at every depth this block
	// touches, including a config file that sets only one of the four
	// sources.* keys and leaves the other three to their defaults (see
	// TestLoad_NotificationsAzureSourcesConfig_* below).
	//
	// lookback_days and min_poll_interval's registrations just below are
	// redundant-on-purpose, not load-bearing: each has its own `== 0`
	// fallback later in this function (LookbackDays via decision A,
	// MinPollInterval via decision B of the task-11 review) that restores
	// the exact same default whether or not SetDefault ever ran, since an
	// absent key leaves the Go zero value 0 for viper's Unmarshal to
	// materialize and the `== 0` branch catches it just as it catches an
	// explicit `: 0` in the file. Deleting either registration therefore
	// changes no observable behaviour and fails no test; both are kept
	// anyway for symmetry with the always-registered shared/GitHub keys
	// above. The four sources.* registrations are the genuinely load-bearing
	// ones in this block: a bool has no `== false` fallback that could tell
	// "unset" apart from "explicitly disabled", so SetDefault is the only
	// mechanism that makes an absent toggle default to true.
	v.SetDefault("notifications.exclude_repos", []string{})
	v.SetDefault("notifications.include_repos", []string{})
	v.SetDefault("notifications.exclude_reasons", []string{})
	v.SetDefault("notifications.unread_only", false)
	v.SetDefault("notifications.max_items", 0)
	v.SetDefault("notifications.poll_interval", 0)
	v.SetDefault("notifications.github.only_configured_repos", false)
	v.SetDefault("notifications.github.participating_only", false)
	v.SetDefault("notifications.github.since_days", 0)
	v.SetDefault("notifications.azure.lookback_days", DefaultAzureLookbackDays)
	v.SetDefault("notifications.azure.min_poll_interval", DefaultAzureMinPollInterval)
	v.SetDefault("notifications.azure.sources.review_requested", true)
	v.SetDefault("notifications.azure.sources.mentioned", true)
	v.SetDefault("notifications.azure.sources.assigned", true)
	v.SetDefault("notifications.azure.sources.ci_failed", true)

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

	// Sanitize notifications.exclude_reasons. An entry
	// provider.ParseNotificationReason does not recognise (a typo, or the
	// reserved "unknown") must never silently act as "other" — it is reported
	// via cfg.Warnings and dropped from the slice here, so the filter never has
	// to reinterpret a bad value. This must never be a hard config error: a
	// typo cannot be allowed to stop the app from starting.
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

	// Drop notifications.exclude_repos / include_repos entries that
	// path.Match rejects as ErrBadPattern, same warn-don't-die channel as the
	// exclude_reasons sanitizer above. An entry that is merely empty after
	// trimming (e.g. "   ") is a valid-but-useless pattern, not a bad one --
	// Validate below rejects that case as a hard error instead.
	cfg.Notifications.ExcludeRepos = sanitizeRepoGlobs(&cfg.Warnings, "notifications.exclude_repos", cfg.Notifications.ExcludeRepos)
	cfg.Notifications.IncludeRepos = sanitizeRepoGlobs(&cfg.Warnings, "notifications.include_repos", cfg.Notifications.IncludeRepos)

	// only_configured_repos overrides include_repos rather than intersecting
	// with it, so a config setting both is not a conflict -- but silently
	// ignoring include_repos would be its own trap, hence the warning rather
	// than staying quiet about it.
	if cfg.Notifications.GitHub.OnlyConfiguredRepos && len(cfg.Notifications.IncludeRepos) > 0 {
		cfg.Warnings = append(cfg.Warnings,
			"notifications.github.only_configured_repos is true — notifications.include_repos is ignored")
	}

	// notifications.azure.lookback_days: zero means "use the default", never
	// "unbounded" (decision 13's closing note). The shared-key convention
	// elsewhere in this file is that zero is the widest possible behaviour;
	// that convention is exactly what would be dangerous here, since Azure's
	// sources have no snapshot state and a truly unbounded window means
	// "every work item ever assigned to me". A config that never sets this
	// key already lands on DefaultAzureLookbackDays via v.SetDefault above,
	// so this branch only fires when the file explicitly writes
	// `lookback_days: 0`. This normalization stays silent (no Warnings
	// entry) on purpose: zero reads as "unset", not as an expressed intent,
	// so there is nothing surprising to flag (task-11 review decision A).
	if cfg.Notifications.Azure.LookbackDays == 0 {
		cfg.Notifications.Azure.LookbackDays = DefaultAzureLookbackDays
	}
	// Clamp to AzureLookbackDaysMax (mirrors azdevops.orphanTTL, 30 days —
	// see that constant's own doc comment). Beyond this bound a subject can
	// be pruned from the local triage store while still inside the query
	// window and resurface as unread with nothing having actually changed,
	// because "inside the window" and "returned by a given poll" are
	// different sets once a source's own per-poll query cap starts crowding
	// out older-but-still-in-window rows. Unlike the zero-fallback above,
	// this branch DOES warn: the user expressed an intent (a specific,
	// larger number) that this silently overrides, which is exactly what
	// Config.Warnings exists for — the same channel this function already
	// uses for an unrecognised exclude_reasons entry, a malformed repo glob,
	// and only_configured_repos swallowing include_repos, all above
	// (task-11 review decision A). This is a normalization, not a
	// validation error — the same treatment azdevops's own WIQL builders
	// already give a negative lookbackDays.
	if cfg.Notifications.Azure.LookbackDays > AzureLookbackDaysMax {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"notifications.azure.lookback_days: %d exceeds the %d-day maximum — using %d",
			cfg.Notifications.Azure.LookbackDays, AzureLookbackDaysMax, AzureLookbackDaysMax))
		cfg.Notifications.Azure.LookbackDays = AzureLookbackDaysMax
	}

	// notifications.azure.min_poll_interval: zero also means "use the
	// default", not "no self-throttle" (decision B of the task-11 review).
	// Under decision 10 the adapter self-throttles to this value; a literal
	// zero would mean it never throttles at all, pricing every shared poll
	// tick at Azure's 4+-queries-per-project cost — exactly the outcome
	// decision 10 exists to bound. Symmetric with lookback_days above,
	// including staying silent: zero reads as "unset", not as a deliberate
	// request for unthrottled polling.
	if cfg.Notifications.Azure.MinPollInterval == 0 {
		cfg.Notifications.Azure.MinPollInterval = DefaultAzureMinPollInterval
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
// Backend requirement: at least one of Azure or GitHub must be configured.
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
	// This is only a fatal error when GitHub is NOT configured — a partial
	// Azure stanza is silently skipped when GitHub carries the config
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
	// notification-capable backend will exist — today that means HasGitHub().
	// The notifications tab hides on *capability*, not on config, so a naive
	// four-way guard would let an Azure-only config with the other three panes
	// disabled pass here and then start with zero navigable tabs, since no
	// backend implements NotificationSource to show the notifications tab
	// either. Metrics cannot rescue this: it is separately gated on
	// metrics.enabled.
	//
	// This HasGitHub() coupling will need revisiting when a second
	// notification-capable backend arrives: the condition must then widen to
	// "any configured backend" and the special case disappears.
	notificationsCounts := c.IsPaneEnabled("notifications") && c.HasGitHub()
	if !c.IsPaneEnabled("pullrequests") && !c.IsPaneEnabled("workitems") && !c.IsPaneEnabled("pipelines") && !notificationsCounts {
		return fmt.Errorf("cannot disable all panes: at least one of 'pullrequests', 'workitems' or 'pipelines' must remain enabled " +
			"(or leave 'notifications' enabled with at least one repo under github.repos — the Notifications tab needs a GitHub backend)")
	}

	// Notifications validation. There is no `notifications.enabled` guard —
	// the block is always present, if only at its defaults, so these checks
	// must hold even when the user never wrote a `notifications:` section at
	// all. Three of the five numeric fields checked below default to the Go
	// zero value (MaxItems, PollInterval, GitHub.SinceDays; no entries in the
	// two glob lists), which trivially satisfies a ">= 0" check. The other
	// two, Azure.LookbackDays and Azure.MinPollInterval, do NOT default to
	// zero: LoadFrom's v.SetDefault calls populate them at 14 and 300
	// (decision 13's YAML) before Unmarshal runs, and the zero-means-default
	// normalization above (LoadFrom, right before this call) backstops an
	// explicit `lookback_days: 0` and `min_poll_interval: 0` the same way.
	// Both non-zero defaults still satisfy ">= 0" below, so an absent or
	// all-default block remains valid — what actually fails these checks is
	// a user-supplied negative, not the absence of the block. That
	// zero-fallback happens in LoadFrom, before Validate() is ever called —
	// a *Config built any other way (a struct literal, NewWithPath) reaches
	// this method with no such normalization applied.
	if c.Notifications.GitHub.SinceDays < 0 {
		return fmt.Errorf("notifications.github.since_days must be >= 0, got %d", c.Notifications.GitHub.SinceDays)
	}
	if c.Notifications.MaxItems < 0 {
		return fmt.Errorf("notifications.max_items must be >= 0, got %d", c.Notifications.MaxItems)
	}
	if c.Notifications.PollInterval < 0 {
		return fmt.Errorf("notifications.poll_interval must be >= 0, got %d", c.Notifications.PollInterval)
	}
	if c.Notifications.Azure.LookbackDays < 0 {
		return fmt.Errorf("notifications.azure.lookback_days must be >= 0, got %d", c.Notifications.Azure.LookbackDays)
	}
	if c.Notifications.Azure.MinPollInterval < 0 {
		return fmt.Errorf("notifications.azure.min_poll_interval must be >= 0, got %d", c.Notifications.Azure.MinPollInterval)
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
