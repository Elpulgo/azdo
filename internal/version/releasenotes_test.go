package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func releaseNotesServer(t *testing.T, releases []githubReleaseNote) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
}

func TestLatestReleaseNotes(t *testing.T) {
	t.Run("returns newest-first, excludes newer than current", func(t *testing.T) {
		server := releaseNotesServer(t, []githubReleaseNote{
			{TagName: "v0.7.0", Body: "seven"},   // newer than current — excluded
			{TagName: "v0.4.0", Body: "four"},     // out of order on purpose
			{TagName: "v0.6.0", Body: "six"},      // == current — included
			{TagName: "v0.5.0", Body: "five"},
			{TagName: "v0.3.0", Body: "three"},
			{TagName: "v0.2.0", Body: "two"},
			{TagName: "v0.1.0", Body: "one"},      // trimmed by count
		})
		defer server.Close()

		c := NewChecker("0.6.0")
		c.releasesURL = server.URL

		notes, err := c.LatestReleaseNotes(5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantTags := []string{"v0.6.0", "v0.5.0", "v0.4.0", "v0.3.0", "v0.2.0"}
		if len(notes) != len(wantTags) {
			t.Fatalf("expected %d notes, got %d: %+v", len(wantTags), len(notes), notes)
		}
		for i, want := range wantTags {
			if notes[i].TagName != want {
				t.Errorf("note[%d].TagName = %q, want %q", i, notes[i].TagName, want)
			}
		}
		if notes[0].Body != "six" {
			t.Errorf("note[0].Body = %q, want %q", notes[0].Body, "six")
		}
	})

	t.Run("skips drafts and pre-releases", func(t *testing.T) {
		server := releaseNotesServer(t, []githubReleaseNote{
			{TagName: "v0.6.0", Body: "stable"},
			{TagName: "v0.5.0", Body: "draft", Draft: true},
			{TagName: "v0.4.0", Body: "prerelease-flag", Prerelease: true},
			{TagName: "v0.3.0-rc1", Body: "prerelease-tag"},
			{TagName: "not-a-version", Body: "junk"},
			{TagName: "v0.2.0", Body: "stable-old"},
		})
		defer server.Close()

		c := NewChecker("0.6.0")
		c.releasesURL = server.URL

		notes, err := c.LatestReleaseNotes(5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantTags := []string{"v0.6.0", "v0.2.0"}
		if len(notes) != len(wantTags) {
			t.Fatalf("expected %d notes, got %d: %+v", len(wantTags), len(notes), notes)
		}
		for i, want := range wantTags {
			if notes[i].TagName != want {
				t.Errorf("note[%d].TagName = %q, want %q", i, notes[i].TagName, want)
			}
		}
	})

	t.Run("unparseable current version skips upper-bound filter", func(t *testing.T) {
		server := releaseNotesServer(t, []githubReleaseNote{
			{TagName: "v0.7.0", Body: "seven"},
			{TagName: "v0.6.0", Body: "six"},
		})
		defer server.Close()

		c := NewChecker("dev")
		c.releasesURL = server.URL

		notes, err := c.LatestReleaseNotes(5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(notes) != 2 || notes[0].TagName != "v0.7.0" {
			t.Fatalf("expected both releases newest-first, got %+v", notes)
		}
	})

	t.Run("server error returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		c := NewChecker("0.6.0")
		c.releasesURL = server.URL

		if _, err := c.LatestReleaseNotes(5); err == nil {
			t.Error("expected error for server error response")
		}
	})
}
