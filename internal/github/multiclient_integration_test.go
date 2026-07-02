//go:build integration

package github

import (
	"os"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// Integration tests for the MultiClient and Adapter. These tests hit the real
// GitHub API and require a GITHUB_TOKEN environment variable. They are excluded
// from the default test run by the //go:build integration build tag.
//
// Run manually with:
//
//	CGO_ENABLED=0 GOCACHE=$PWD/.gocache TMPDIR=$PWD/.gotmp \
//	  GITHUB_TOKEN=<token> \
//	  go test -tags integration ./internal/github/... -v -run TestIntegration_Multi
//
// The tests target a well-known public repository (octocat/Hello-World) to
// avoid requiring write permissions or a private repo token.

func integrationMultiClient(t *testing.T) *MultiClient {
	t.Helper()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set — skipping integration test")
	}
	mc, err := NewMultiClient(
		[]string{"octocat/Hello-World"},
		token,
		DefaultLabelConvention(),
		map[string]string{"octocat/Hello-World": "Hello World (octocat)"},
	)
	if err != nil {
		t.Fatalf("NewMultiClient: %v", err)
	}
	return mc
}

func TestIntegration_MultiClient_ListWorkItems(t *testing.T) {
	mc := integrationMultiClient(t)

	items, err := mc.ListWorkItems(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListWorkItems() error = %v", err)
	}
	t.Logf("ListWorkItems: got %d items", len(items))
	for _, item := range items {
		if item.Identity.Scope != "octocat/Hello-World" {
			t.Errorf("item scope = %q, want %q", item.Identity.Scope, "octocat/Hello-World")
		}
		if item.Identity.Kind != provider.KindGitHub {
			t.Errorf("item kind = %v, want KindGitHub", item.Identity.Kind)
		}
		if item.Identity.ScopeDisplay != "Hello World (octocat)" {
			t.Errorf("item scopeDisplay = %q, want %q", item.Identity.ScopeDisplay, "Hello World (octocat)")
		}
	}
}

func TestIntegration_MultiClient_ListPullRequests(t *testing.T) {
	mc := integrationMultiClient(t)

	prs, err := mc.ListPullRequests(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	t.Logf("ListPullRequests: got %d PRs", len(prs))
	for _, pr := range prs {
		if pr.Identity.Kind != provider.KindGitHub {
			t.Errorf("PR kind = %v, want KindGitHub", pr.Identity.Kind)
		}
	}
}

func TestIntegration_MultiClient_ListPipelineRuns(t *testing.T) {
	mc := integrationMultiClient(t)

	runs, err := mc.ListPipelineRuns(10, provider.ListOpts{})
	if err != nil {
		t.Fatalf("ListPipelineRuns() error = %v", err)
	}
	t.Logf("ListPipelineRuns: got %d runs", len(runs))
}

func TestIntegration_Adapter_Kind(t *testing.T) {
	mc := integrationMultiClient(t)
	a := NewAdapter(mc)

	if a.Kind() != provider.KindGitHub {
		t.Errorf("Kind() = %v, want KindGitHub", a.Kind())
	}
}

func TestIntegration_Adapter_ListWorkItems(t *testing.T) {
	mc := integrationMultiClient(t)
	a := NewAdapter(mc)

	items, err := a.ListWorkItems(5, provider.ListOpts{})
	if err != nil {
		t.Fatalf("Adapter.ListWorkItems() error = %v", err)
	}
	t.Logf("Adapter.ListWorkItems: got %d items", len(items))
}

func TestIntegration_Adapter_GetPRIterations_Synthetic(t *testing.T) {
	mc := integrationMultiClient(t)
	a := NewAdapter(mc)

	// Use PR #1 on octocat/Hello-World (always exists on that repo).
	iters, err := a.GetPRIterations("octocat/Hello-World", "", 1)
	if err != nil {
		t.Fatalf("GetPRIterations() error = %v", err)
	}
	if len(iters) != 1 || iters[0].ID != 1 {
		t.Errorf("expected 1 synthetic iteration with ID=1, got %+v", iters)
	}
}

func TestIntegration_Adapter_WorkItemURL(t *testing.T) {
	mc := integrationMultiClient(t)
	a := NewAdapter(mc)

	got := a.WorkItemURL("octocat/Hello-World", 1)
	want := "https://github.com/octocat/Hello-World/issues/1"
	if got != want {
		t.Errorf("WorkItemURL = %q, want %q", got, want)
	}
}
