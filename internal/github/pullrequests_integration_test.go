//go:build integration

package github

import (
	"os"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Integration tests for the PR Client methods. These tests hit the real
// GitHub API and require a GITHUB_TOKEN environment variable. They are excluded
// from the default test run by the //go:build integration build tag.
//
// Run manually with:
//
//	CGO_ENABLED=0 GOCACHE=$PWD/.gocache TMPDIR=$PWD/.gotmp \
//	  GITHUB_TOKEN=<token> \
//	  go test -tags integration ./internal/github/... -v -run TestIntegrationPR

func integrationPRClient(t *testing.T) *Client {
	t.Helper()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set — skipping integration test")
	}
	// torvalds/linux is a public repo with many open PRs suitable for read-only testing.
	return NewClient("torvalds", "linux", token)
}

func TestIntegrationPR_ListPullRequests(t *testing.T) {
	c := integrationPRClient(t)

	prs, err := c.ListPullRequests(5, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	t.Logf("ListPullRequests: got %d PRs", len(prs))
	for _, pr := range prs {
		if pr.Number == 0 {
			t.Error("PR has zero Number")
		}
	}
}

func TestIntegrationPR_ListMyPullRequests(t *testing.T) {
	c := integrationPRClient(t)

	prs, err := c.ListMyPullRequests(5, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListMyPullRequests() error = %v", err)
	}
	t.Logf("ListMyPullRequests: got %d PRs", len(prs))
	// Fidelity check: search items must not have Head.Ref populated.
	for _, pr := range prs {
		if pr.Head.Ref != "" {
			t.Logf("NOTE: Head.Ref = %q (unexpected for search items — fidelity regression?)", pr.Head.Ref)
		}
	}
}

func TestIntegrationPR_ListPullRequestsAsReviewer(t *testing.T) {
	c := integrationPRClient(t)

	prs, err := c.ListPullRequestsAsReviewer(5, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPullRequestsAsReviewer() error = %v", err)
	}
	t.Logf("ListPullRequestsAsReviewer: got %d PRs", len(prs))
}

func TestIntegrationPR_GetPRFiles(t *testing.T) {
	// Use octocat/Hello-World PR #1 as a stable read-only fixture.
	c := NewClient("octocat", "Hello-World", os.Getenv("GITHUB_TOKEN"))
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set — skipping integration test")
	}

	// PR #1 is a well-known fixture on octocat/Hello-World.
	files, err := c.GetPRFiles(1)
	if err != nil {
		// The PR may have been closed or the repo updated; log and skip.
		t.Logf("GetPRFiles(1) error = %v (PR may not exist — skipping)", err)
		t.Skip("skipping: PR #1 unavailable")
	}
	t.Logf("GetPRFiles(1): got %d files", len(files))
}

func TestIntegrationPR_GetFileContent(t *testing.T) {
	c := NewClient("octocat", "Hello-World", os.Getenv("GITHUB_TOKEN"))
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set — skipping integration test")
	}

	content, err := c.GetFileContent("README", "master")
	if err != nil {
		t.Fatalf("GetFileContent(README, master) error = %v", err)
	}
	if content == "" {
		t.Error("GetFileContent returned empty string for README")
	}
	t.Logf("GetFileContent(README): %d bytes", len(content))
}
