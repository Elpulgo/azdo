package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// ReleaseNote is a single published release's tag name and body, as shown in
// the help modal's Release Notes section.
type ReleaseNote struct {
	TagName string
	Body    string
}

// githubReleaseNote is the subset of the GitHub releases API response we need
// to build a ReleaseNote.
type githubReleaseNote struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// LatestReleaseNotes fetches up to count published releases at or below the
// checker's current version, newest first. Drafts, pre-releases and releases
// with unparseable tags are skipped, as is anything newer than the current
// version — the user never sees notes for a version they are not yet on.
//
// When the current version is not valid semver (e.g. a "dev" build) the
// upper-bound filter cannot be applied, so the newest published releases are
// returned as-is.
func (c *Checker) LatestReleaseNotes(count int) ([]ReleaseNote, error) {
	req, err := http.NewRequest("GET", c.releasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []githubReleaseNote
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases response: %w", err)
	}

	current := parseSemver(c.currentVersion)

	type verNote struct {
		ver  []int
		note ReleaseNote
	}
	var valid []verNote

	for _, r := range releases {
		if r.Draft || r.Prerelease || isPrereleaseTag(r.TagName) {
			continue
		}
		ver := parseSemver(r.TagName)
		if ver == nil {
			continue // unparseable tag
		}
		if current != nil && compareSemver(ver, current) > 0 {
			continue // newer than the user's version — exclude
		}
		valid = append(valid, verNote{ver, ReleaseNote{TagName: r.TagName, Body: r.Body}})
	}

	sort.Slice(valid, func(i, j int) bool {
		return compareSemver(valid[i].ver, valid[j].ver) > 0
	})

	if len(valid) > count {
		valid = valid[:count]
	}

	notes := make([]ReleaseNote, len(valid))
	for i, v := range valid {
		notes[i] = v.note
	}
	return notes, nil
}

// isPrereleaseTag reports whether a version tag carries a pre-release suffix
// such as -beta or -rc1. parseSemver strips these suffixes, so pre-releases
// must be detected on the raw tag before parsing.
func isPrereleaseTag(tag string) bool {
	return strings.Contains(strings.TrimPrefix(tag, "v"), "-")
}
