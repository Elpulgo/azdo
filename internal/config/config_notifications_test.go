package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every fixture in this file goes through LoadFrom(filepath.Join(t.TempDir(),
// "config.yaml")) when it needs Load-time behavior (defaults, warnings,
// mixed-case keys). Pure Validate() cases use a bare Config{} struct literal
// — that is safe because Validate() never touches disk; only Save() does
// (convention 17). None of the tests in this file call Save().

func TestLoad_NotificationsDefaults_WhenBlockAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if n.OnlyConfiguredRepos {
		t.Error("OnlyConfiguredRepos = true by default; want false")
	}
	if len(n.ExcludeRepos) != 0 {
		t.Errorf("ExcludeRepos = %v, want empty", n.ExcludeRepos)
	}
	if len(n.IncludeRepos) != 0 {
		t.Errorf("IncludeRepos = %v, want empty", n.IncludeRepos)
	}
	if len(n.ExcludeReasons) != 0 {
		t.Errorf("ExcludeReasons = %v, want empty", n.ExcludeReasons)
	}
	if n.UnreadOnly {
		t.Error("UnreadOnly = true by default; want false")
	}
	if n.ParticipatingOnly {
		t.Error("ParticipatingOnly = true by default; want false")
	}
	if n.SinceDays != 0 {
		t.Errorf("SinceDays = %d, want 0", n.SinceDays)
	}
	if n.MaxItems != 0 {
		t.Errorf("MaxItems = %d, want 0", n.MaxItems)
	}
	if n.PollInterval != 0 {
		t.Errorf("PollInterval = %d, want 0", n.PollInterval)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for a clean config", cfg.Warnings)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with absent notifications block = %v, want nil", err)
	}
}

func TestLoad_NotificationsDefaults_WhenBlockPresentButEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications: {}
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if n.OnlyConfiguredRepos || n.UnreadOnly || n.ParticipatingOnly {
		t.Errorf("expected all bools false for empty block, got %+v", n)
	}
	if len(n.ExcludeRepos) != 0 || len(n.IncludeRepos) != 0 || len(n.ExcludeReasons) != 0 {
		t.Errorf("expected all lists empty for empty block, got %+v", n)
	}
	if n.SinceDays != 0 || n.MaxItems != 0 || n.PollInterval != 0 {
		t.Errorf("expected all ints 0 for empty block, got %+v", n)
	}
}

func TestLoad_NotificationsBlock_AllKeysParsed(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  only_configured_repos: true
  exclude_repos:
    - "spammy/*"
    - "owner/noisy-repo"
  include_repos:
    - "owner/repo"
  exclude_reasons:
    - subscribed
    - ci_activity
  unread_only: true
  participating_only: true
  since_days: 7
  max_items: 50
  poll_interval: 120
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	n := cfg.Notifications
	if !n.OnlyConfiguredRepos {
		t.Error("OnlyConfiguredRepos = false, want true")
	}
	if len(n.ExcludeRepos) != 2 || n.ExcludeRepos[0] != "spammy/*" || n.ExcludeRepos[1] != "owner/noisy-repo" {
		t.Errorf("ExcludeRepos = %v, want [spammy/* owner/noisy-repo]", n.ExcludeRepos)
	}
	if len(n.IncludeRepos) != 1 || n.IncludeRepos[0] != "owner/repo" {
		t.Errorf("IncludeRepos = %v, want [owner/repo]", n.IncludeRepos)
	}
	if len(n.ExcludeReasons) != 2 || n.ExcludeReasons[0] != "subscribed" || n.ExcludeReasons[1] != "ci_activity" {
		t.Errorf("ExcludeReasons = %v, want [subscribed ci_activity]", n.ExcludeReasons)
	}
	if !n.UnreadOnly {
		t.Error("UnreadOnly = false, want true")
	}
	if !n.ParticipatingOnly {
		t.Error("ParticipatingOnly = false, want true")
	}
	if n.SinceDays != 7 {
		t.Errorf("SinceDays = %d, want 7", n.SinceDays)
	}
	if n.MaxItems != 50 {
		t.Errorf("MaxItems = %d, want 50", n.MaxItems)
	}
	if n.PollInterval != 120 {
		t.Errorf("PollInterval = %d, want 120", n.PollInterval)
	}
	// This fixture sets both only_configured_repos: true and a non-empty
	// include_repos, which is flagged with exactly one warning -- include_repos
	// is overridden, not intersected, and the user is told so rather than left
	// to wonder why it had no effect. The exclude_reasons entries above are both
	// valid, so this is the only warning expected.
	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry (only_configured_repos + include_repos both set)", cfg.Warnings)
	}
	// Assert the content too, not just the count: a count-only check passes
	// if the intended warning disappears while some unrelated warning
	// appears in its place.
	if !strings.Contains(cfg.Warnings[0], "include_repos") || !strings.Contains(cfg.Warnings[0], "ignored") {
		t.Errorf("warning should say include_repos is ignored, got: %s", cfg.Warnings[0])
	}
}

func TestLoad_NotificationsBlock_MixedCaseKeys_Convention9(t *testing.T) {
	// Convention 9: viper lowercases all config keys on load. Mixed-case keys
	// in the YAML must still resolve — this pins that for the notifications
	// block specifically.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  UNREAD_ONLY: true
  Exclude_Reasons:
    - subscribed
  Only_Configured_Repos: true
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if !cfg.Notifications.UnreadOnly {
		t.Error("UnreadOnly = false, want true (mixed-case key UNREAD_ONLY should still resolve)")
	}
	if !cfg.Notifications.OnlyConfiguredRepos {
		t.Error("OnlyConfiguredRepos = false, want true (mixed-case key Only_Configured_Repos should still resolve)")
	}
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed] (mixed-case key Exclude_Reasons should still resolve)", cfg.Notifications.ExcludeReasons)
	}
}

func TestConfig_Validate_NotificationsRejectsNegative(t *testing.T) {
	base := func() *Config {
		return &Config{
			Organization:    "org",
			Projects:        []string{"p"},
			PollingInterval: 60,
			Theme:           "dark",
		}
	}

	// All three rows use -1 on purpose. MaxItems truncates only when it is >
	// 0, so -1 is precisely the value that would read as "unlimited" if a
	// guard were ever loosened from < 0 to, say, < -1 or == 0 — it is the
	// boundary worth pinning, not an arbitrarily large negative.
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "since_days negative",
			mutate:  func(c *Config) { c.Notifications.SinceDays = -1 },
			wantErr: "since_days",
		},
		{
			name:    "max_items negative",
			mutate:  func(c *Config) { c.Notifications.MaxItems = -1 },
			wantErr: "max_items",
		},
		{
			name:    "poll_interval negative",
			mutate:  func(c *Config) { c.Notifications.PollInterval = -1 },
			wantErr: "poll_interval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_NotificationsAcceptsZeroBounds(t *testing.T) {
	// Zero is the documented "no bound" default for all three — must not be
	// rejected by a >= 0 guard (only negatives are invalid).
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		Notifications: NotificationsConfig{
			SinceDays:    0,
			MaxItems:     0,
			PollInterval: 0,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with zero-value notification bounds = %v, want nil", err)
	}
}

func TestConfig_Validate_NotificationsRejectsEmptyGlobEntries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name:   "exclude_repos empty entry",
			mutate: func(c *Config) { c.Notifications.ExcludeRepos = []string{"owner/repo", ""} },
		},
		{
			name:   "include_repos empty entry",
			mutate: func(c *Config) { c.Notifications.IncludeRepos = []string{""} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Organization:    "org",
				Projects:        []string{"p"},
				PollingInterval: 60,
				Theme:           "dark",
			}
			tt.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("Validate() = nil, want error for empty glob entry")
			}
		})
	}
}

func TestConfig_Validate_DisabledPanes_NotificationsAccepted(t *testing.T) {
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"notifications"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with disabled_panes: notifications = %v, want nil", err)
	}
	if cfg.IsPaneEnabled("notifications") {
		t.Error("IsPaneEnabled(notifications) = true, want false")
	}
}

func TestConfig_Validate_InvalidDisabledPane_MentionsNotifications(t *testing.T) {
	cfg := &Config{
		Organization:    "org",
		Projects:        []string{"p"},
		PollingInterval: 60,
		Theme:           "dark",
		DisabledPanes:   []string{"bogus"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for invalid disabled pane")
	}
	if !strings.Contains(err.Error(), "notifications") {
		t.Errorf("error should mention 'notifications' among the valid pane names, got: %s", err.Error())
	}
}

// TestConfig_Validate_NotificationsOnly_Passes: a config where only
// notifications is enabled (the other three panes disabled) must pass
// Validate().
func TestConfig_Validate_NotificationsOnly_Passes(t *testing.T) {
	cfg := &Config{
		PollingInterval: 60,
		Theme:           "dark",
		GitHub:          GitHubConfig{Repos: []string{"owner/repo"}},
		DisabledPanes:   []string{"pullrequests", "workitems", "pipelines"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() for notifications-only config = %v, want nil", err)
	}
}

// --- Notifications counts as a remaining pane only when HasGitHub() is true.
// This coupling will need revisiting when a second notification-capable
// backend arrives. ---

// TestConfig_Validate_PaneGuard_NotificationsRequiresGitHub walks the three
// fixtures that together pin both conjuncts of `IsPaneEnabled("notifications")
// && HasGitHub()`:
//
//   - GitHub configured, other three panes disabled → valid (notifications is
//     the remaining pane);
//   - Azure-only, other three panes disabled → still rejected, because the
//     notifications tab hides on capability, and the message must explain the
//     GitHub coupling rather than repeat the old three-pane text verbatim;
//   - GitHub configured, ALL FOUR panes disabled → rejected.
//
// The third row is the one that distinguishes the conjunction from a bare
// HasGitHub(): the first two vary HasGitHub() while notifications stays
// enabled, so dropping the IsPaneEnabled("notifications") conjunct leaves
// them both green. Without it a GitHub config that explicitly turns off every
// pane would validate and the app would start with zero navigable tabs.
func TestConfig_Validate_PaneGuard_NotificationsRequiresGitHub(t *testing.T) {
	// oldThreePaneText is the message from before the GitHub coupling. Row 2
	// must not be it: repeating it verbatim explains nothing about why
	// notifications does not rescue an Azure-only config.
	const oldThreePaneText = "cannot disable all panes: at least one of 'pullrequests', 'workitems' or 'pipelines' must remain enabled"

	tests := []struct {
		name    string
		content string
		wantErr bool
		// wantErrContains are substrings every rejection message must carry.
		wantErrContains []string
		// forbidExactErr, when non-empty, must not be the whole message.
		forbidExactErr string
	}{
		{
			name: "GitHub configured, other three panes disabled",
			content: `polling_interval: 60
theme: dark
github:
  repos:
    - owner/repo
disabled_panes: pullrequests,workitems,pipelines
`,
			wantErr: false,
		},
		{
			name: "Azure-only, other three panes disabled",
			content: `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
disabled_panes: pullrequests,workitems,pipelines
`,
			wantErr:         true,
			wantErrContains: []string{"notifications", "GitHub"},
			forbidExactErr:  oldThreePaneText,
		},
		{
			name: "GitHub configured, all four panes disabled",
			content: `polling_interval: 60
theme: dark
github:
  repos:
    - owner/repo
disabled_panes: pullrequests,workitems,pipelines,notifications
`,
			wantErr:         true,
			wantErrContains: []string{"cannot disable all panes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := LoadFrom(configPath)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("LoadFrom() = %v, want nil — notifications is the only remaining pane and GitHub is configured", err)
				}
				if !cfg.HasGitHub() {
					t.Fatal("HasGitHub() = false, want true (test fixture invalid)")
				}
				return
			}

			if err == nil {
				t.Fatal("LoadFrom() = nil, want an error — this config leaves zero navigable tabs")
			}
			errMsg := err.Error()
			for _, want := range tt.wantErrContains {
				if !strings.Contains(errMsg, want) {
					t.Errorf("error should mention %q, got: %s", want, errMsg)
				}
			}
			if tt.forbidExactErr != "" && errMsg == tt.forbidExactErr {
				t.Errorf("error message is just the old three-pane text; must explain the GitHub/notifications coupling")
			}
		})
	}
}

// --- Unrecognised exclude_reasons entries warn and are dropped. ---

// wantElevenAcceptedReasonNames is a hardcoded, independent copy of the
// eleven accepted values — deliberately NOT generated from production code
// (never by calling acceptedNotificationReasons(), which would be
// tautological), so these tests cannot pass merely because the implementation
// and the test share the same (possibly wrong) source.
//
// The order is the enum declaration order, which is also the order
// acceptedNotificationReasons() emits. "unknown" is absent on purpose: it is
// the enum's twelfth value, reserved and rejected by ParseNotificationReason,
// so advertising it as accepted would tell the user to write a string the
// parser refuses.
var wantElevenAcceptedReasonNames = []string{
	"review_requested", "mentioned", "assigned", "authored", "commented",
	"state_changed", "ci_activity", "security_alert", "approval_requested",
	"subscribed", "other",
}

// acceptedValuesPrefix is the literal separator in the unrecognised-reason
// warning after which the accepted-values list begins.
const acceptedValuesPrefix = "accepted values: "

// TestLoad_ExcludeReasons_WarningListsExactlyTheElevenAcceptedValues pins the
// accepted-values segment of the warning to the eleven names, exactly, in enum
// order. A per-name strings.Contains check (as the test below does) cannot
// pin the exclusion of "unknown": in the unknown-value test the string
// "unknown" is already in the message as the *offending* value, so a
// containment assertion passes whether or not the accepted list also
// advertises it. Only an exact comparison of the segment kills that mutant.
func TestLoad_ExcludeReasons_WarningListsExactlyTheElevenAcceptedValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscibed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}

	msg := cfg.Warnings[0]
	parts := strings.SplitN(msg, acceptedValuesPrefix, 2)
	if len(parts) != 2 {
		t.Fatalf("warning %q does not contain %q", msg, acceptedValuesPrefix)
	}
	segment := parts[1]

	want := strings.Join(wantElevenAcceptedReasonNames, ", ")
	if segment != want {
		t.Errorf("accepted-values segment = %q, want %q", segment, want)
	}
	if got := len(strings.Split(segment, ", ")); got != 11 {
		t.Errorf("accepted-values segment lists %d values, want exactly 11", got)
	}
	if strings.Contains(segment, "unknown") {
		t.Errorf("accepted-values segment must not advertise the reserved 'unknown' value, got: %s", segment)
	}
}

func TestLoad_ExcludeReasons_UnrecognisedValue_WarnsAndDropped(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscribed
    - subscibed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on an unrecognised exclude_reasons entry: %v", err)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "subscibed") {
		t.Errorf("warning should name the offending value %q, got: %s", "subscibed", msg)
	}
	for _, name := range wantElevenAcceptedReasonNames {
		if !strings.Contains(msg, name) {
			t.Errorf("warning should list accepted value %q, got: %s", name, msg)
		}
	}

	// The valid entry is kept; the bad one is dropped so it can never act as "other".
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed] (bad entry dropped, good entry kept)", cfg.Notifications.ExcludeReasons)
	}
}

func TestLoad_ExcludeReasons_UnknownValue_Warns(t *testing.T) {
	// "unknown" is the enum's twelfth value but is explicitly not accepted in
	// config — it must warn exactly like any other typo.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - unknown
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on 'unknown' in exclude_reasons: %v", err)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	if !strings.Contains(cfg.Warnings[0], "unknown") {
		t.Errorf("warning should name 'unknown' as the offending value, got: %s", cfg.Warnings[0])
	}
	if len(cfg.Notifications.ExcludeReasons) != 0 {
		t.Errorf("ExcludeReasons = %v, want empty ('unknown' dropped)", cfg.Notifications.ExcludeReasons)
	}
}

// TestLoad_ExcludeReasons_TwoUnrecognisedValues_WarnOnceEach pins that
// warnings accumulate rather than collapsing to the last one. Every other
// fixture in this file has at most one bad entry, so replacing the append with
// an assignment would go unnoticed here — and the notifications pane renders
// the whole slice, so a collapse-to-one regression would silently hide typos
// the user needs to see.
func TestLoad_ExcludeReasons_TwoUnrecognisedValues_WarnOnceEach(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscibed
    - ci_activty
    - subscribed
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on unrecognised exclude_reasons entries: %v", err)
	}

	if len(cfg.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want exactly 2 (one per unrecognised entry)", cfg.Warnings)
	}
	// Each warning names its own offending value and only its own — a single
	// warning listing both, or two copies of the same one, is a regression.
	if !strings.Contains(cfg.Warnings[0], "subscibed") || strings.Contains(cfg.Warnings[0], "ci_activty") {
		t.Errorf("Warnings[0] should name only \"subscibed\", got: %s", cfg.Warnings[0])
	}
	if !strings.Contains(cfg.Warnings[1], "ci_activty") || strings.Contains(cfg.Warnings[1], "subscibed") {
		t.Errorf("Warnings[1] should name only \"ci_activty\", got: %s", cfg.Warnings[1])
	}
	// The one good entry survives both drops.
	if len(cfg.Notifications.ExcludeReasons) != 1 || cfg.Notifications.ExcludeReasons[0] != "subscribed" {
		t.Errorf("ExcludeReasons = %v, want [subscribed]", cfg.Notifications.ExcludeReasons)
	}
}

func TestLoad_ExcludeReasons_AllValid_NoWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_reasons:
    - subscribed
    - other
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for all-valid exclude_reasons", cfg.Warnings)
	}
	if len(cfg.Notifications.ExcludeReasons) != 2 {
		t.Errorf("ExcludeReasons = %v, want both entries kept", cfg.Notifications.ExcludeReasons)
	}
}

// --- Malformed exclude_repos / include_repos globs are dropped at load with a
// warning naming the pattern and the key; a whitespace-only entry is a hard
// Validate() error, extending the existing empty-entry check rather than
// becoming a new warning class. ---

func TestLoad_ExcludeRepos_BadPattern_DroppedWithWarning_ValidEntriesSurvive(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  exclude_repos:
    - "owner/good"
    - "[bad"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on a malformed exclude_repos pattern: %v", err)
	}

	if len(cfg.Notifications.ExcludeRepos) != 1 || cfg.Notifications.ExcludeRepos[0] != "owner/good" {
		t.Fatalf("ExcludeRepos = %v, want [owner/good] ([bad dropped, owner/good kept)", cfg.Notifications.ExcludeRepos)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "[bad") {
		t.Errorf("warning should name the offending pattern %q, got: %s", "[bad", msg)
	}
	if !strings.Contains(msg, "notifications.exclude_repos") {
		t.Errorf("warning should name the key notifications.exclude_repos, got: %s", msg)
	}
	// The pattern and the error state the cause; the warning must also state
	// the consequence, otherwise the user is told a pattern was dropped but
	// not what their feed will now do.
	if !strings.Contains(msg, "nothing is excluded by it") {
		t.Errorf("warning should say nothing is excluded by the dropped pattern, got: %s", msg)
	}
}

func TestLoad_IncludeRepos_BadPattern_DroppedWithWarning_ValidEntriesSurvive(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  include_repos:
    - "[bad"
    - "owner/good"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on a malformed include_repos pattern: %v", err)
	}

	if len(cfg.Notifications.IncludeRepos) != 1 || cfg.Notifications.IncludeRepos[0] != "owner/good" {
		t.Fatalf("IncludeRepos = %v, want [owner/good] ([bad dropped, owner/good kept)", cfg.Notifications.IncludeRepos)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "[bad") {
		t.Errorf("warning should name the offending pattern %q, got: %s", "[bad", msg)
	}
	if !strings.Contains(msg, "notifications.include_repos") {
		t.Errorf("warning should name the key notifications.include_repos, got: %s", msg)
	}
	// A compilable pattern survived, so the selection still narrows the feed:
	// the consequence clause must say that, not that the whole inbox is shown.
	if !strings.Contains(msg, "the remaining include patterns still apply") {
		t.Errorf("warning should say the surviving include patterns still apply, got: %s", msg)
	}
}

// TestLoad_IncludeRepos_AllPatternsBad_WarningSaysWholeInboxIsShown pins the
// other consequence clause: with no compilable include pattern left the list
// is empty, and an empty include_repos selects everything, so the user must be
// told their filter has stopped narrowing anything rather than being left to
// guess.
func TestLoad_IncludeRepos_AllPatternsBad_WarningSaysWholeInboxIsShown(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  include_repos:
    - "[bad"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() should not fail on a malformed include_repos pattern: %v", err)
	}
	if len(cfg.Notifications.IncludeRepos) != 0 {
		t.Fatalf("IncludeRepos = %v, want empty (the only pattern was malformed)", cfg.Notifications.IncludeRepos)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	if !strings.Contains(cfg.Warnings[0], "the whole inbox is shown") {
		t.Errorf("warning should say the whole inbox is shown, got: %s", cfg.Warnings[0])
	}
}

func TestLoad_OnlyConfiguredRepos_WithIncludeRepos_Warns(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
notifications:
  only_configured_repos: true
  include_repos:
    - "owner/repo"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if len(cfg.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 entry", cfg.Warnings)
	}
	msg := cfg.Warnings[0]
	if !strings.Contains(msg, "only_configured_repos") || !strings.Contains(msg, "include_repos") {
		t.Errorf("warning should name both only_configured_repos and include_repos, got: %s", msg)
	}
	// include_repos itself is not mutated by the warning -- only ignored at
	// filter time, so it should still be present in the config.
	if len(cfg.Notifications.IncludeRepos) != 1 || cfg.Notifications.IncludeRepos[0] != "owner/repo" {
		t.Errorf("IncludeRepos = %v, want [owner/repo] (warned about, not dropped)", cfg.Notifications.IncludeRepos)
	}
}

func TestLoad_WhitespaceOnlyGlobEntry_IsLoadError(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"exclude_repos", "exclude_repos"},
		{"include_repos", "include_repos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			content := "organization: test-org\n" +
				"projects:\n  - alpha\n" +
				"polling_interval: 60\n" +
				"theme: dark\n" +
				"notifications:\n" +
				"  " + tt.key + ":\n" +
				"    - \"   \"\n"
			if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := LoadFrom(configPath)
			if err == nil {
				t.Fatalf("LoadFrom() = nil error, want error for whitespace-only notifications.%s entry", tt.key)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error should mention %q, got: %s", tt.key, err.Error())
			}
		})
	}
}

func TestLoad_Warnings_EmptyForCleanConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `organization: test-org
projects:
  - alpha
polling_interval: 60
theme: dark
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Warnings != nil && len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty for a clean config", cfg.Warnings)
	}
}
