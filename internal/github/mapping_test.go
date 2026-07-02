package github_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

const (
	testScope        = "octo/repo"
	testScopeDisplay = "Octo Repo"
)

// assertIdentityInvariant checks that all required Identity fields are non-zero.
func assertIdentityInvariant(t *testing.T, name string, id provider.Identity) {
	t.Helper()
	if id.Kind == 0 {
		t.Errorf("%s: Identity.Kind is zero", name)
	}
	if id.Scope == "" {
		t.Errorf("%s: Identity.Scope is empty", name)
	}
	if id.ID == "" {
		t.Errorf("%s: Identity.ID is empty", name)
	}
}

// --- MapWorkItem: fully-populated issue ---

func TestMapWorkItem_FullyPopulated(t *testing.T) {
	// A closed issue with assignee, milestone, type:bug + priority:p2 + plain label.
	const raw = `{
		"number": 101,
		"title": "Segfault on login",
		"body": "Reproducible steps here.",
		"state": "closed",
		"state_reason": "completed",
		"user": {"login": "reporter", "id": 1},
		"assignee": {"login": "alice", "id": 2},
		"labels": [
			{"id": 10, "name": "type:bug",      "color": "d73a4a", "description": ""},
			{"id": 11, "name": "priority:p2",   "color": "fbca04", "description": ""},
			{"id": 12, "name": "needs-repro",   "color": "0075ca", "description": ""}
		],
		"milestone": {"title": "v2.0", "number": 4},
		"created_at": "2026-01-10T08:00:00Z",
		"updated_at": "2026-01-15T12:00:00Z",
		"closed_at":  "2026-01-15T11:30:00Z",
		"html_url":   "https://github.com/octo/repo/issues/101"
	}`

	var issue github.Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapWorkItem(issue, github.DefaultLabelConvention(), testScope, testScopeDisplay)

	// Identity invariant
	assertIdentityInvariant(t, "WorkItem(full)", got.Identity)
	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Identity.Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Identity.Scope != testScope {
		t.Errorf("Identity.Scope = %q, want %q", got.Identity.Scope, testScope)
	}
	if got.Identity.ScopeDisplay != testScopeDisplay {
		t.Errorf("Identity.ScopeDisplay = %q, want %q", got.Identity.ScopeDisplay, testScopeDisplay)
	}
	if got.Identity.ID != "101" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "101")
	}

	// Core fields
	if got.Title != "Segfault on login" {
		t.Errorf("Title = %q, want %q", got.Title, "Segfault on login")
	}
	if got.State != "closed" {
		t.Errorf("State = %q, want %q", got.State, "closed")
	}
	if got.Description != "Reproducible steps here." {
		t.Errorf("Description = %q, want %q", got.Description, "Reproducible steps here.")
	}
	if got.URL != "https://github.com/octo/repo/issues/101" {
		t.Errorf("URL = %q", got.URL)
	}

	// StateCategory: closed + completed → StateCategoryClosedDone
	if got.StateCategory != provider.StateCategoryClosedDone {
		t.Errorf("StateCategory = %v, want StateCategoryClosedDone", got.StateCategory)
	}

	// Labels → ItemKind, Priority, Tags
	if got.ItemKind != provider.ItemTypeBug {
		t.Errorf("ItemKind = %v, want ItemTypeBug", got.ItemKind)
	}
	if got.Priority != 2 {
		t.Errorf("Priority = %d, want 2", got.Priority)
	}
	if got.Tags != "needs-repro" {
		t.Errorf("Tags = %q, want %q", got.Tags, "needs-repro")
	}

	// WorkItemType derived from ItemKind
	if got.WorkItemType != "Bug" {
		t.Errorf("WorkItemType = %q, want %q", got.WorkItemType, "Bug")
	}

	// AssignedToName from Assignee.Login
	if got.AssignedToName != "alice" {
		t.Errorf("AssignedToName = %q, want %q", got.AssignedToName, "alice")
	}

	// IterationPath from Milestone.Title
	if got.IterationPath != "v2.0" {
		t.Errorf("IterationPath = %q, want %q", got.IterationPath, "v2.0")
	}

	// Dates
	wantCreated := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	if !got.CreatedDate.Equal(wantCreated) {
		t.Errorf("CreatedDate = %v, want %v", got.CreatedDate, wantCreated)
	}
	wantChanged := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	if !got.ChangedDate.Equal(wantChanged) {
		t.Errorf("ChangedDate = %v, want %v", got.ChangedDate, wantChanged)
	}
	wantClosed := time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC)
	if !got.ClosedDate.Equal(wantClosed) {
		t.Errorf("ClosedDate = %v, want %v", got.ClosedDate, wantClosed)
	}
	// StateChangeDate approximated from ClosedAt when closed
	if !got.StateChangeDate.Equal(wantClosed) {
		t.Errorf("StateChangeDate = %v, want %v (same as ClosedAt)", got.StateChangeDate, wantClosed)
	}

	// Zero-value fields (no GitHub equivalent)
	if got.ActivatedDate != (time.Time{}) {
		t.Errorf("ActivatedDate = %v, want zero", got.ActivatedDate)
	}
	if got.ReproSteps != "" {
		t.Errorf("ReproSteps = %q, want empty", got.ReproSteps)
	}
	if got.StoryPoints != 0 {
		t.Errorf("StoryPoints = %v, want 0", got.StoryPoints)
	}
}

// --- MapWorkItem: minimal open issue ---

func TestMapWorkItem_Minimal(t *testing.T) {
	// Open issue: no assignee, no milestone, null state_reason, no labels.
	const raw = `{
		"number": 7,
		"title": "Button misaligned",
		"body": "",
		"state": "open",
		"state_reason": null,
		"user": {"login": "user1", "id": 3},
		"assignee": null,
		"labels": [],
		"milestone": null,
		"created_at": "2026-03-01T00:00:00Z",
		"updated_at": "2026-03-02T00:00:00Z",
		"closed_at": null,
		"html_url": "https://github.com/octo/repo/issues/7"
	}`

	var issue github.Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Must not panic.
	got := github.MapWorkItem(issue, github.DefaultLabelConvention(), testScope, testScopeDisplay)

	assertIdentityInvariant(t, "WorkItem(minimal)", got.Identity)
	if got.Identity.ID != "7" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "7")
	}

	if got.AssignedToName != "" {
		t.Errorf("AssignedToName = %q, want empty (no assignee)", got.AssignedToName)
	}
	if got.IterationPath != "" {
		t.Errorf("IterationPath = %q, want empty (no milestone)", got.IterationPath)
	}
	// No type: label → default ItemTypeIssue
	if got.ItemKind != provider.ItemTypeIssue {
		t.Errorf("ItemKind = %v, want ItemTypeIssue", got.ItemKind)
	}
	if got.WorkItemType != "Issue" {
		t.Errorf("WorkItemType = %q, want %q", got.WorkItemType, "Issue")
	}
	if got.Priority != 0 {
		t.Errorf("Priority = %d, want 0", got.Priority)
	}
	if got.Tags != "" {
		t.Errorf("Tags = %q, want empty", got.Tags)
	}
	// open → StateCategoryActive
	if got.StateCategory != provider.StateCategoryActive {
		t.Errorf("StateCategory = %v, want StateCategoryActive", got.StateCategory)
	}
	// ClosedDate must be zero for an open issue
	if !got.ClosedDate.IsZero() {
		t.Errorf("ClosedDate = %v, want zero", got.ClosedDate)
	}
	// StateChangeDate must be zero for an open issue
	if !got.StateChangeDate.IsZero() {
		t.Errorf("StateChangeDate = %v, want zero", got.StateChangeDate)
	}
}

// --- MapWorkItem: not_planned state_reason → StateCategoryRemoved ---

func TestMapWorkItem_NotPlanned(t *testing.T) {
	const raw = `{
		"number": 99,
		"title": "Wontfix",
		"body": "",
		"state": "closed",
		"state_reason": "not_planned",
		"user": {"login": "triager", "id": 5},
		"assignee": null,
		"labels": [],
		"milestone": null,
		"created_at": "2026-02-01T00:00:00Z",
		"updated_at": "2026-02-02T00:00:00Z",
		"closed_at": "2026-02-02T00:00:00Z",
		"html_url": "https://github.com/octo/repo/issues/99"
	}`

	var issue github.Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapWorkItem(issue, github.DefaultLabelConvention(), testScope, testScopeDisplay)

	if got.StateCategory != provider.StateCategoryRemoved {
		t.Errorf("StateCategory = %v, want StateCategoryRemoved for not_planned", got.StateCategory)
	}
}

// --- MapWorkItemComment ---

func TestMapWorkItemComment(t *testing.T) {
	const raw = `{
		"id": 987654321,
		"body": "Looks good to me!",
		"user": {"login": "bob", "id": 42},
		"created_at": "2026-05-20T09:15:00Z",
		"updated_at": "2026-05-20T09:15:00Z",
		"html_url": "https://github.com/octo/repo/issues/101#issuecomment-987654321"
	}`

	var c github.IssueComment
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	got := github.MapWorkItemComment(c, testScope, testScopeDisplay)

	// Identity invariant
	assertIdentityInvariant(t, "WorkItemComment", got.Identity)
	if got.Identity.Kind != provider.KindGitHub {
		t.Errorf("Identity.Kind = %v, want KindGitHub", got.Identity.Kind)
	}
	if got.Identity.Scope != testScope {
		t.Errorf("Identity.Scope = %q, want %q", got.Identity.Scope, testScope)
	}
	if got.Identity.ScopeDisplay != testScopeDisplay {
		t.Errorf("Identity.ScopeDisplay = %q, want %q", got.Identity.ScopeDisplay, testScopeDisplay)
	}
	if got.Identity.ID != "987654321" {
		t.Errorf("Identity.ID = %q, want %q", got.Identity.ID, "987654321")
	}

	// Mapped fields
	if got.ID != 987654321 {
		t.Errorf("ID = %d, want 987654321", got.ID)
	}
	if got.Text != "Looks good to me!" {
		t.Errorf("Text = %q, want %q", got.Text, "Looks good to me!")
	}
	if got.AuthorName != "bob" {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, "bob")
	}
	wantCreated := time.Date(2026, 5, 20, 9, 15, 0, 0, time.UTC)
	if !got.CreatedDate.Equal(wantCreated) {
		t.Errorf("CreatedDate = %v, want %v", got.CreatedDate, wantCreated)
	}
}
