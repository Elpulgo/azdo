//go:build integration

package github

import (
	"os"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Integration tests for the work-item Client methods. These tests hit the real
// GitHub API and require a GITHUB_TOKEN environment variable. They are excluded
// from the default test run by the //go:build integration build tag.
//
// Run manually with:
//
//	CGO_ENABLED=0 GOCACHE=$PWD/.gocache TMPDIR=$PWD/.gotmp \
//	  GITHUB_TOKEN=<token> \
//	  go test -tags integration ./internal/github/... -v -run TestIntegration

const (
	// integrationOwner and integrationRepo point at a known public repository
	// for integration testing. The public octocat/Hello-World repo is a safe
	// read-only target.
	integrationOwner = "octocat"
	integrationRepo  = "Hello-World"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set — skipping integration test")
	}
	return NewClient(integrationOwner, integrationRepo, token)
}

func TestIntegration_ListWorkItems(t *testing.T) {
	c := integrationClient(t)

	issues, err := c.ListWorkItems(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}

	// All returned items must be real issues (no PRs).
	for _, issue := range issues {
		if issue.PullRequest != nil {
			t.Errorf("ListWorkItems returned a PR (number %d)", issue.Number)
		}
	}
	t.Logf("ListWorkItems: got %d issues", len(issues))
}

func TestIntegration_ListMyWorkItems(t *testing.T) {
	c := integrationClient(t)

	issues, err := c.ListMyWorkItems(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListMyWorkItems() error = %v", err)
	}
	t.Logf("ListMyWorkItems: got %d issues", len(issues))
}

func TestIntegration_GetWorkItemTypeStates(t *testing.T) {
	c := integrationClient(t)

	states, err := c.GetWorkItemTypeStates("issue")
	if err != nil {
		t.Fatalf("GetWorkItemTypeStates() error = %v", err)
	}
	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d: %v", len(states), states)
	}
}

func TestIntegration_GetWorkItemComments(t *testing.T) {
	c := integrationClient(t)

	// Issue #1 on octocat/Hello-World is a well-known fixture that should have
	// at least some comments. The test only checks that the call succeeds.
	comments, err := c.GetWorkItemComments(1)
	if err != nil {
		t.Fatalf("GetWorkItemComments(1) error = %v", err)
	}
	t.Logf("GetWorkItemComments(1): got %d comments", len(comments))
}
