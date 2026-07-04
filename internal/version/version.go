package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL      = "https://api.github.com/repos/Elpulgo/azdo/releases/latest"
	defaultReleasesURL = "https://api.github.com/repos/Elpulgo/azdo/releases?per_page=100"
	httpTimeout        = 5 * time.Second
)

// UpdateInfo contains the result of a version check.
type UpdateInfo struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseURL      string
}

// Checker checks GitHub for newer releases.
type Checker struct {
	currentVersion string
	apiURL         string
	releasesURL    string
	httpClient     *http.Client
}

// githubRelease is the subset of the GitHub release API response we need.
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// NewChecker creates a version checker for the given current version.
func NewChecker(currentVersion string) *Checker {
	return &Checker{
		currentVersion: currentVersion,
		apiURL:         defaultAPIURL,
		releasesURL:    defaultReleasesURL,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// CheckForUpdate checks if a newer version is available on GitHub.
// Returns UpdateInfo with the result. Returns an error on network/API failures.
func (c *Checker) CheckForUpdate() (*UpdateInfo, error) {
	info := &UpdateInfo{
		CurrentVersion: c.currentVersion,
	}

	// Don't check for dev builds
	if c.currentVersion == "" || c.currentVersion == "dev" {
		return info, nil
	}

	req, err := http.NewRequest("GET", c.apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release response: %w", err)
	}

	info.LatestVersion = release.TagName
	info.ReleaseURL = release.HTMLURL
	info.UpdateAvailable = isNewer(c.currentVersion, release.TagName)

	return info, nil
}

// isNewer returns true if latest is a newer semver than current.
func isNewer(current, latest string) bool {
	curParts := parseSemver(current)
	latParts := parseSemver(latest)

	if curParts == nil || latParts == nil {
		return false
	}

	return compareSemver(latParts, curParts) > 0
}

// compareSemver compares two parsed major.minor.patch versions part by part.
// Returns 1 if a > b, -1 if a < b, and 0 when they are equal.
func compareSemver(a, b []int) int {
	for i := 0; i < 3; i++ {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		}
	}
	return 0
}

// parseSemver extracts major.minor.patch from a version string.
// Returns nil if parsing fails.
func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}

	result := make([]int, 3)
	for i, p := range parts {
		// Strip any pre-release suffix (e.g., "1-beta")
		p = strings.SplitN(p, "-", 2)[0]
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result[i] = n
	}

	return result
}
