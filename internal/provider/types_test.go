package provider_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

// TestKindAzureConstant verifies the Kind type and KindAzure constant are defined
// and that the constant is non-zero (distinguishable from the zero value).
func TestKindAzureConstant(t *testing.T) {
	var k provider.Kind
	if k == provider.KindAzure {
		t.Fatal("zero value of Kind must not equal KindAzure")
	}
}

// TestIdentityFields verifies that Identity carries Kind, Scope, and ID.
func TestIdentityFields(t *testing.T) {
	id := provider.Identity{
		Kind:  provider.KindAzure,
		Scope: "my-project",
		ID:    "42",
	}
	if id.Kind != provider.KindAzure {
		t.Errorf("expected Kind=%v, got %v", provider.KindAzure, id.Kind)
	}
	if id.Scope != "my-project" {
		t.Errorf("expected Scope=%q, got %q", "my-project", id.Scope)
	}
	if id.ID != "42" {
		t.Errorf("expected ID=%q, got %q", "42", id.ID)
	}
}

// TestWorkItemHasIdentity verifies that WorkItem embeds or carries an Identity.
func TestWorkItemHasIdentity(t *testing.T) {
	wi := provider.WorkItem{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "1"},
	}
	if wi.Identity.Kind != provider.KindAzure {
		t.Errorf("WorkItem.Identity.Kind must be KindAzure, got %v", wi.Identity.Kind)
	}
}

// TestPullRequestHasIdentity verifies that PullRequest carries an Identity.
func TestPullRequestHasIdentity(t *testing.T) {
	pr := provider.PullRequest{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "10"},
	}
	if pr.Identity.ID != "10" {
		t.Errorf("PullRequest.Identity.ID expected %q, got %q", "10", pr.Identity.ID)
	}
}

// TestPipelineRunHasIdentity verifies that PipelineRun carries an Identity.
func TestPipelineRunHasIdentity(t *testing.T) {
	run := provider.PipelineRun{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "99"},
	}
	if run.Identity.Scope != "proj" {
		t.Errorf("PipelineRun.Identity.Scope expected %q, got %q", "proj", run.Identity.Scope)
	}
}

// TestThreadHasIdentity verifies that Thread carries an Identity.
func TestThreadHasIdentity(t *testing.T) {
	th := provider.Thread{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "5"},
	}
	if th.Identity.Kind == (provider.Kind)(0) {
		t.Error("Thread.Identity.Kind must not be zero")
	}
}

// TestThreadLineField verifies that Thread carries a Line field for inline diff placement.
// This field maps from the wire RightFileStart.Line so the adapter can reconstruct
// which diff line an inline comment belongs to.
func TestThreadLineField(t *testing.T) {
	th := provider.Thread{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "5"},
		FilePath: "internal/foo.go",
		Line:     42,
	}
	if th.Line != 42 {
		t.Errorf("Thread.Line expected 42, got %d", th.Line)
	}
	// A general (non-inline) thread should have Line == 0.
	general := provider.Thread{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "6"},
	}
	if general.Line != 0 {
		t.Errorf("Thread.Line for a general thread expected 0, got %d", general.Line)
	}
}

// TestCommentHasIdentity verifies that Comment carries an Identity.
func TestCommentHasIdentity(t *testing.T) {
	c := provider.Comment{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "7"},
	}
	if c.Identity.ID != "7" {
		t.Errorf("Comment.Identity.ID expected %q, got %q", "7", c.Identity.ID)
	}
}

// TestTimelineHasIdentity verifies that Timeline carries an Identity.
func TestTimelineHasIdentity(t *testing.T) {
	tl := provider.Timeline{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "build-1"},
	}
	if tl.Identity.ID != "build-1" {
		t.Errorf("Timeline.Identity.ID expected %q, got %q", "build-1", tl.Identity.ID)
	}
}

// TestBuildLogHasIdentity verifies that BuildLog carries an Identity.
func TestBuildLogHasIdentity(t *testing.T) {
	bl := provider.BuildLog{
		Identity: provider.Identity{Kind: provider.KindAzure, Scope: "proj", ID: "3"},
	}
	if bl.Identity.Scope != "proj" {
		t.Errorf("BuildLog.Identity.Scope expected %q, got %q", "proj", bl.Identity.Scope)
	}
}

// TestAllDomainTypesExist is a compile-time proof that all required domain types
// exist in the package; if any type is missing this file will not compile.
func TestAllDomainTypesExist(t *testing.T) {
	types := []interface{}{
		provider.WorkItem{},
		provider.PullRequest{},
		provider.PipelineRun{},
		provider.Thread{},
		provider.Comment{},
		provider.Timeline{},
		provider.BuildLog{},
		provider.Identity{},
	}
	if len(types) == 0 {
		t.Fatal("unreachable")
	}
}
