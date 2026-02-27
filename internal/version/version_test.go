package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"same version", "1.0.0", "1.0.0", false},
		{"older patch", "1.0.0", "1.0.1", true},
		{"older minor", "1.0.0", "1.1.0", true},
		{"older major", "1.0.0", "2.0.0", true},
		{"newer than latest", "2.0.0", "1.0.0", false},
		{"with v prefix current", "v1.0.0", "1.0.1", true},
		{"with v prefix latest", "1.0.0", "v1.0.1", true},
		{"both v prefix", "v1.0.0", "v1.0.1", true},
		{"dev version", "dev", "1.0.0", false},
		{"empty current", "", "1.0.0", false},
		{"empty latest", "1.0.0", "", false},
		{"invalid current", "abc", "1.0.0", false},
		{"invalid latest", "1.0.0", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewer(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestCheckerCheckForUpdate(t *testing.T) {
	t.Run("update available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := githubRelease{
				TagName: "v2.0.0",
				HTMLURL: "https://github.com/Elpulgo/azdo/releases/tag/v2.0.0",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		c := NewChecker("1.0.0")
		c.apiURL = server.URL

		info, err := c.CheckForUpdate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !info.UpdateAvailable {
			t.Error("expected UpdateAvailable to be true")
		}
		if info.LatestVersion != "v2.0.0" {
			t.Errorf("expected LatestVersion = %q, got %q", "v2.0.0", info.LatestVersion)
		}
		if info.ReleaseURL != "https://github.com/Elpulgo/azdo/releases/tag/v2.0.0" {
			t.Errorf("unexpected ReleaseURL: %s", info.ReleaseURL)
		}
	})

	t.Run("no update available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := githubRelease{
				TagName: "v1.0.0",
				HTMLURL: "https://github.com/Elpulgo/azdo/releases/tag/v1.0.0",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		c := NewChecker("1.0.0")
		c.apiURL = server.URL

		info, err := c.CheckForUpdate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.UpdateAvailable {
			t.Error("expected UpdateAvailable to be false")
		}
	})

	t.Run("server error returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		c := NewChecker("1.0.0")
		c.apiURL = server.URL

		_, err := c.CheckForUpdate()
		if err == nil {
			t.Error("expected error for server error response")
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer server.Close()

		c := NewChecker("1.0.0")
		c.apiURL = server.URL

		_, err := c.CheckForUpdate()
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("network error returns error", func(t *testing.T) {
		c := NewChecker("1.0.0")
		c.apiURL = "http://localhost:1" // nothing listening

		_, err := c.CheckForUpdate()
		if err == nil {
			t.Error("expected error for network failure")
		}
	})

	t.Run("dev version skips check", func(t *testing.T) {
		c := NewChecker("dev")
		// No server needed — should return early

		info, err := c.CheckForUpdate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.UpdateAvailable {
			t.Error("dev version should never show update available")
		}
	})
}
