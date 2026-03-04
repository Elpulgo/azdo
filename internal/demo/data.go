package demo

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// Fictional team members used across all mock data.
var team = []azdevops.Identity{
	{ID: "a1b2c3d4-0001-4000-8000-000000000001", DisplayName: "Alex Chen", UniqueName: "alex.chen@contoso.com"},
	{ID: "a1b2c3d4-0002-4000-8000-000000000002", DisplayName: "Maria Santos", UniqueName: "maria.santos@contoso.com"},
	{ID: "a1b2c3d4-0003-4000-8000-000000000003", DisplayName: "James Wilson", UniqueName: "james.wilson@contoso.com"},
	{ID: "a1b2c3d4-0004-4000-8000-000000000004", DisplayName: "Priya Patel", UniqueName: "priya.patel@contoso.com"},
	{ID: "a1b2c3d4-0005-4000-8000-000000000005", DisplayName: "Liam O'Brien", UniqueName: "liam.obrien@contoso.com"},
	{ID: "a1b2c3d4-0006-4000-8000-000000000006", DisplayName: "Sofia Kim", UniqueName: "sofia.kim@contoso.com"},
}

// demoUserID is the ID returned by the mock connectionData endpoint.
const demoUserID = "a1b2c3d4-0001-4000-8000-000000000001" // Alex Chen

// Repository IDs used by both projects.
const (
	repoIDNexus   = "repo-nexus-0001"
	repoNameNexus = "nexus-platform"
	repoIDHorizon = "repo-horizon-0001"
)

// now anchors all timestamps so data looks recent regardless of when demo runs.
var now = time.Now()

func hoursAgo(h int) time.Time   { return now.Add(-time.Duration(h) * time.Hour) }
func minutesAgo(m int) time.Time { return now.Add(-time.Duration(m) * time.Minute) }

func ptr[T any](v T) *T { return &v }

func mockPullRequests() []azdevops.PullRequest {
	return []azdevops.PullRequest{
		{
			ID: 1042, Title: "Refactor authentication middleware to support OAuth2",
			Description: "## Summary\nReplaces the legacy token-based auth with a proper OAuth2 flow.\n\n## Changes\n- Added OAuth2 provider configuration\n- Refactored middleware chain\n- Updated integration tests\n\n## Testing\n- All existing auth tests pass\n- Added 12 new integration tests",
			Status: "active", CreationDate: hoursAgo(4),
			SourceRefName: "refs/heads/feature/auth-refactor", TargetRefName: "refs/heads/main",
			CreatedBy:  team[0],
			Repository: azdevops.Repository{ID: repoIDNexus, Name: repoNameNexus},
			Reviewers: []azdevops.Reviewer{
				{ID: team[1].ID, DisplayName: team[1].DisplayName, Vote: azdevops.VoteApprove},
				{ID: team[3].ID, DisplayName: team[3].DisplayName, Vote: azdevops.VoteApproveWithSuggestions},
			},
		},
		{
			ID: 1039, Title: "Fix memory leak in WebSocket connection pool",
			Description: "Fixes a memory leak caused by connections not being properly cleaned up when clients disconnect unexpectedly.\n\nRoot cause: the cleanup goroutine was blocked on a channel send that nobody was reading after timeout.",
			Status: "active", CreationDate: hoursAgo(12),
			SourceRefName: "refs/heads/fix/memory-leak", TargetRefName: "refs/heads/main",
			CreatedBy:  team[2],
			Repository: azdevops.Repository{ID: repoIDNexus, Name: repoNameNexus},
			Reviewers: []azdevops.Reviewer{
				{ID: team[0].ID, DisplayName: team[0].DisplayName, Vote: azdevops.VoteNoVote},
				{ID: team[4].ID, DisplayName: team[4].DisplayName, Vote: azdevops.VoteWaitForAuthor},
			},
		},
		{
			ID: 1037, Title: "Add rate limiting to public API endpoints",
			Description: "Implements token bucket rate limiting for all public API endpoints.\n\nConfigurable per-route limits via config file.",
			Status: "active", CreationDate: hoursAgo(28),
			SourceRefName: "refs/heads/feature/rate-limiting", TargetRefName: "refs/heads/main",
			CreatedBy:  team[3],
			Repository: azdevops.Repository{ID: repoIDNexus, Name: repoNameNexus},
			Reviewers: []azdevops.Reviewer{
				{ID: team[0].ID, DisplayName: team[0].DisplayName, Vote: azdevops.VoteApprove},
				{ID: team[1].ID, DisplayName: team[1].DisplayName, Vote: azdevops.VoteApprove},
			},
		},
		{
			ID: 1035, Title: "Upgrade database driver to v5 with connection pooling",
			Description: "Major version upgrade of the database driver. Includes connection pooling improvements and prepared statement caching.",
			Status: "active", CreationDate: hoursAgo(48), IsDraft: true,
			SourceRefName: "refs/heads/chore/db-driver-v5", TargetRefName: "refs/heads/main",
			CreatedBy:  team[4],
			Repository: azdevops.Repository{ID: repoIDNexus, Name: repoNameNexus},
			Reviewers:  []azdevops.Reviewer{},
		},
		{
			ID: 1041, Title: "Implement dark mode toggle in settings panel",
			Description: "Adds a dark mode toggle to the user settings panel with system preference detection.",
			Status: "active", CreationDate: hoursAgo(6),
			SourceRefName: "refs/heads/feature/dark-mode", TargetRefName: "refs/heads/main",
			CreatedBy:  team[5],
			Repository: azdevops.Repository{ID: repoIDHorizon, Name: "horizon-app"},
			Reviewers: []azdevops.Reviewer{
				{ID: team[3].ID, DisplayName: team[3].DisplayName, Vote: azdevops.VoteApprove},
			},
		},
		{
			ID: 1040, Title: "Fix table sorting not persisting across page navigation",
			Description: "Sort state was being reset when navigating between pages. Now stores sort preferences in session storage.",
			Status: "active", CreationDate: hoursAgo(8),
			SourceRefName: "refs/heads/fix/table-sort-persist", TargetRefName: "refs/heads/develop",
			CreatedBy:  team[1],
			Repository: azdevops.Repository{ID: repoIDHorizon, Name: "horizon-app"},
			Reviewers: []azdevops.Reviewer{
				{ID: team[5].ID, DisplayName: team[5].DisplayName, Vote: azdevops.VoteNoVote},
				{ID: team[0].ID, DisplayName: team[0].DisplayName, Vote: azdevops.VoteReject},
			},
		},
		{
			ID: 1038, Title: "Add E2E tests for checkout flow",
			Description: "Adds comprehensive end-to-end tests for the checkout flow using Playwright.",
			Status: "active", CreationDate: hoursAgo(24),
			SourceRefName: "refs/heads/test/checkout-e2e", TargetRefName: "refs/heads/main",
			CreatedBy:  team[0],
			Repository: azdevops.Repository{ID: repoIDHorizon, Name: "horizon-app"},
			Reviewers: []azdevops.Reviewer{
				{ID: team[2].ID, DisplayName: team[2].DisplayName, Vote: azdevops.VoteApproveWithSuggestions},
			},
		},
		{
			ID: 1036, Title: "Migrate component library to new design tokens",
			Description: "Replaces hardcoded color values with design tokens from the new design system.",
			Status: "active", CreationDate: hoursAgo(36), IsDraft: true,
			SourceRefName: "refs/heads/chore/design-tokens", TargetRefName: "refs/heads/main",
			CreatedBy:  team[3],
			Repository: azdevops.Repository{ID: repoIDHorizon, Name: "horizon-app"},
			Reviewers:  []azdevops.Reviewer{},
		},
	}
}

func mockWorkItems() []azdevops.WorkItem {
	return []azdevops.WorkItem{
		{
			ID: 5001, Rev: 3,
			Fields: azdevops.WorkItemFields{
				Title: "Login page crashes on mobile Safari", State: "Active", WorkItemType: "Bug",
				AssignedTo: &team[0], Priority: 1, ChangedDate: hoursAgo(2),
				IterationPath: "Nexus Platform\\Sprint 24",
				ReproSteps:    "<ol><li>Open login page on iOS Safari 17</li><li>Enter credentials</li><li>Tap Sign In</li><li>Page crashes with white screen</li></ol>",
				Tags:          "mobile; critical; safari",
			},
		},
		{
			ID: 5002, Rev: 5,
			Fields: azdevops.WorkItemFields{
				Title: "Implement user profile avatar upload", State: "Active", WorkItemType: "User Story",
				AssignedTo: &team[1], Priority: 2, ChangedDate: hoursAgo(5),
				IterationPath: "Nexus Platform\\Sprint 24",
				Description:   "As a user, I want to upload a profile avatar so that other team members can identify me visually.\n\n## Acceptance Criteria\n- Support JPEG, PNG, WebP formats\n- Max file size: 5MB\n- Auto-crop to square\n- Generate thumbnails at 32px, 64px, 128px",
				Tags:          "frontend; ux",
			},
		},
		{
			ID: 5003, Rev: 2,
			Fields: azdevops.WorkItemFields{
				Title: "Set up CI pipeline for integration tests", State: "New", WorkItemType: "Task",
				AssignedTo: &team[2], Priority: 2, ChangedDate: hoursAgo(8),
				IterationPath: "Nexus Platform\\Sprint 24",
				Description:   "Configure the CI pipeline to run integration tests against the staging database after unit tests pass.",
			},
		},
		{
			ID: 5004, Rev: 7,
			Fields: azdevops.WorkItemFields{
				Title: "API returns 500 when filtering by date range", State: "Resolved", WorkItemType: "Bug",
				AssignedTo: &team[3], Priority: 2, ChangedDate: hoursAgo(1),
				IterationPath: "Nexus Platform\\Sprint 24",
				ReproSteps:    "<ol><li>Call GET /api/v1/events?from=2024-01-01&to=2024-12-31</li><li>Returns HTTP 500</li></ol><p>Cause: date parsing fails for timezone-aware timestamps.</p>",
				Tags:          "api; backend",
			},
		},
		{
			ID: 5005, Rev: 1,
			Fields: azdevops.WorkItemFields{
				Title: "Migrate search from ElasticSearch to Meilisearch", State: "New", WorkItemType: "User Story",
				AssignedTo: nil, Priority: 3, ChangedDate: hoursAgo(72),
				IterationPath: "Nexus Platform\\Backlog",
				Description:   "Evaluate and migrate our full-text search from ElasticSearch to Meilisearch for simpler operations and lower resource usage.",
				Tags:          "infrastructure; search",
			},
		},
		{
			ID: 6001, Rev: 4,
			Fields: azdevops.WorkItemFields{
				Title: "Dashboard charts not rendering in Firefox", State: "Active", WorkItemType: "Bug",
				AssignedTo: &team[5], Priority: 1, ChangedDate: hoursAgo(3),
				IterationPath: "Horizon App\\Sprint 12",
				ReproSteps:    "<ol><li>Open dashboard in Firefox 121+</li><li>Charts show empty containers</li><li>Console shows: 'ResizeObserver loop completed with undelivered notifications'</li></ol>",
				Tags:          "firefox; charts; critical",
			},
		},
		{
			ID: 6002, Rev: 2,
			Fields: azdevops.WorkItemFields{
				Title: "Add keyboard navigation to data grid", State: "Active", WorkItemType: "User Story",
				AssignedTo: &team[0], Priority: 2, ChangedDate: hoursAgo(6),
				IterationPath: "Horizon App\\Sprint 12",
				Description:   "As a power user, I want to navigate the data grid using keyboard shortcuts for faster data entry.\n\n## Shortcuts\n- Arrow keys: move between cells\n- Enter: edit cell\n- Escape: cancel edit\n- Tab: move to next cell",
				Tags:          "accessibility; ux",
			},
		},
		{
			ID: 6003, Rev: 1,
			Fields: azdevops.WorkItemFields{
				Title: "Write unit tests for notification service", State: "New", WorkItemType: "Task",
				AssignedTo: &team[4], Priority: 3, ChangedDate: hoursAgo(24),
				IterationPath: "Horizon App\\Sprint 12",
				Description:   "Add unit tests for the notification service. Target: 80% code coverage.",
			},
		},
		{
			ID: 6004, Rev: 6,
			Fields: azdevops.WorkItemFields{
				Title: "Optimize bundle size by code splitting routes", State: "Active", WorkItemType: "Task",
				AssignedTo: &team[3], Priority: 2, ChangedDate: hoursAgo(4),
				IterationPath: "Horizon App\\Sprint 12",
				Description:   "Current bundle is 2.3MB. Split routes using dynamic imports to reduce initial load to under 500KB.",
				Tags:          "performance",
			},
		},
		{
			ID: 6005, Rev: 3,
			Fields: azdevops.WorkItemFields{
				Title: "Implement real-time collaboration cursors", State: "New", WorkItemType: "User Story",
				AssignedTo: nil, Priority: 4, ChangedDate: hoursAgo(96),
				IterationPath: "Horizon App\\Backlog",
				Description:   "As a team member, I want to see other users' cursors in real-time when editing the same document.",
				Tags:          "collaboration; websocket",
			},
		},
		{
			ID: 5006, Rev: 2,
			Fields: azdevops.WorkItemFields{
				Title: "Add OpenTelemetry tracing to API gateway", State: "Active", WorkItemType: "Task",
				AssignedTo: &team[2], Priority: 2, ChangedDate: hoursAgo(10),
				IterationPath: "Nexus Platform\\Sprint 24",
				Description:   "Instrument the API gateway with OpenTelemetry for distributed tracing. Export to Jaeger.",
				Tags:          "observability; infrastructure",
			},
		},
		{
			ID: 5007, Rev: 1,
			Fields: azdevops.WorkItemFields{
				Title: "Security audit: review dependency vulnerabilities", State: "New", WorkItemType: "Task",
				AssignedTo: &team[1], Priority: 1, ChangedDate: hoursAgo(16),
				IterationPath: "Nexus Platform\\Sprint 24",
				Description:   "Run a full dependency audit and address any critical or high severity CVEs.",
				Tags:          "security",
			},
		},
	}
}

func mockPipelineRuns() []azdevops.PipelineRun {
	startTime1 := hoursAgo(2)
	finishTime1 := startTime1.Add(4 * time.Minute)
	startTime2 := hoursAgo(3)
	finishTime2 := startTime2.Add(12 * time.Minute)
	startTime3 := hoursAgo(5)
	finishTime3 := startTime3.Add(6 * time.Minute)
	startTime4 := hoursAgo(1)
	startTime5 := hoursAgo(8)
	finishTime5 := startTime5.Add(3 * time.Minute)
	startTime6 := hoursAgo(10)
	finishTime6 := startTime6.Add(8 * time.Minute)
	startTime7 := hoursAgo(12)
	finishTime7 := startTime7.Add(5 * time.Minute)
	startTime8 := hoursAgo(6)
	finishTime8 := startTime8.Add(15 * time.Minute)
	startTime9 := hoursAgo(14)
	finishTime9 := startTime9.Add(7 * time.Minute)
	startTime10 := hoursAgo(16)
	finishTime10 := startTime10.Add(2 * time.Minute)

	return []azdevops.PipelineRun{
		{
			ID: 8001, BuildNumber: "20240315.1", Status: "completed", Result: "succeeded",
			SourceBranch: "refs/heads/main", SourceVersion: "a3f1c2e",
			QueueTime: hoursAgo(2), StartTime: &startTime1, FinishTime: &finishTime1,
			Definition: azdevops.PipelineDefinition{ID: 1, Name: "CI Build"},
			Project:    azdevops.Project{ID: "proj-nexus-001", Name: "nexus-platform"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/nexus-platform/_build/results?buildId=8001"}},
		},
		{
			ID: 8002, BuildNumber: "20240315.2", Status: "completed", Result: "failed",
			SourceBranch: "refs/heads/feature/auth-refactor", SourceVersion: "b7d4e5f",
			QueueTime: hoursAgo(3), StartTime: &startTime2, FinishTime: &finishTime2,
			Definition: azdevops.PipelineDefinition{ID: 2, Name: "Integration Tests"},
			Project:    azdevops.Project{ID: "proj-nexus-001", Name: "nexus-platform"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/nexus-platform/_build/results?buildId=8002"}},
		},
		{
			ID: 8003, BuildNumber: "20240315.3", Status: "completed", Result: "succeeded",
			SourceBranch: "refs/heads/main", SourceVersion: "c8f9a0b",
			QueueTime: hoursAgo(5), StartTime: &startTime3, FinishTime: &finishTime3,
			Definition: azdevops.PipelineDefinition{ID: 3, Name: "Deploy Staging"},
			Project:    azdevops.Project{ID: "proj-nexus-001", Name: "nexus-platform"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/nexus-platform/_build/results?buildId=8003"}},
		},
		{
			ID: 8004, BuildNumber: "20240315.4", Status: "inProgress", Result: "",
			SourceBranch: "refs/heads/feature/rate-limiting", SourceVersion: "d1e2f3a",
			QueueTime: hoursAgo(1), StartTime: &startTime4, FinishTime: nil,
			Definition: azdevops.PipelineDefinition{ID: 1, Name: "CI Build"},
			Project:    azdevops.Project{ID: "proj-nexus-001", Name: "nexus-platform"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/nexus-platform/_build/results?buildId=8004"}},
		},
		{
			ID: 8005, BuildNumber: "20240315.1", Status: "completed", Result: "succeeded",
			SourceBranch: "refs/heads/main", SourceVersion: "e4f5a6b",
			QueueTime: hoursAgo(8), StartTime: &startTime5, FinishTime: &finishTime5,
			Definition: azdevops.PipelineDefinition{ID: 10, Name: "CI Build"},
			Project:    azdevops.Project{ID: "proj-horizon-001", Name: "horizon-app"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/horizon-app/_build/results?buildId=8005"}},
		},
		{
			ID: 8006, BuildNumber: "20240315.2", Status: "completed", Result: "partiallySucceeded",
			SourceBranch: "refs/heads/feature/dark-mode", SourceVersion: "f7a8b9c",
			QueueTime: hoursAgo(10), StartTime: &startTime6, FinishTime: &finishTime6,
			Definition: azdevops.PipelineDefinition{ID: 11, Name: "E2E Tests"},
			Project:    azdevops.Project{ID: "proj-horizon-001", Name: "horizon-app"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/horizon-app/_build/results?buildId=8006"}},
		},
		{
			ID: 8007, BuildNumber: "20240315.3", Status: "completed", Result: "succeeded",
			SourceBranch: "refs/heads/main", SourceVersion: "a0b1c2d",
			QueueTime: hoursAgo(12), StartTime: &startTime7, FinishTime: &finishTime7,
			Definition: azdevops.PipelineDefinition{ID: 12, Name: "Deploy Preview"},
			Project:    azdevops.Project{ID: "proj-horizon-001", Name: "horizon-app"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/horizon-app/_build/results?buildId=8007"}},
		},
		{
			ID: 8008, BuildNumber: "20240314.5", Status: "completed", Result: "failed",
			SourceBranch: "refs/heads/fix/table-sort-persist", SourceVersion: "b3c4d5e",
			QueueTime: hoursAgo(6), StartTime: &startTime8, FinishTime: &finishTime8,
			Definition: azdevops.PipelineDefinition{ID: 11, Name: "E2E Tests"},
			Project:    azdevops.Project{ID: "proj-horizon-001", Name: "horizon-app"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/horizon-app/_build/results?buildId=8008"}},
		},
		{
			ID: 8009, BuildNumber: "20240314.4", Status: "completed", Result: "succeeded",
			SourceBranch: "refs/heads/main", SourceVersion: "c5d6e7f",
			QueueTime: hoursAgo(14), StartTime: &startTime9, FinishTime: &finishTime9,
			Definition: azdevops.PipelineDefinition{ID: 13, Name: "Security Scan"},
			Project:    azdevops.Project{ID: "proj-nexus-001", Name: "nexus-platform"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/nexus-platform/_build/results?buildId=8009"}},
		},
		{
			ID: 8010, BuildNumber: "20240314.3", Status: "completed", Result: "succeeded",
			SourceBranch: "refs/heads/main", SourceVersion: "d7e8f9a",
			QueueTime: hoursAgo(16), StartTime: &startTime10, FinishTime: &finishTime10,
			Definition: azdevops.PipelineDefinition{ID: 10, Name: "CI Build"},
			Project:    azdevops.Project{ID: "proj-horizon-001", Name: "horizon-app"},
			Links:      azdevops.Links{Web: azdevops.Link{Href: "https://dev.azure.com/contoso/horizon-app/_build/results?buildId=8010"}},
		},
	}
}

func mockThreads() []azdevops.Thread {
	return []azdevops.Thread{
		{
			ID: 1, PublishedDate: hoursAgo(3), LastUpdatedDate: hoursAgo(2),
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/middleware/auth.go",
				RightFileStart: &azdevops.FilePosition{Line: 42, Offset: 1},
				RightFileEnd:   &azdevops.FilePosition{Line: 42, Offset: 1},
			},
			Comments: []azdevops.Comment{
				{
					ID: 1, Content: "This error handling could be more specific. Consider wrapping with a custom error type so callers can distinguish between auth failures and network errors.",
					CommentType: "text", PublishedDate: hoursAgo(3), LastUpdatedDate: hoursAgo(3),
					Author: team[1],
				},
				{
					ID: 2, ParentCommentID: 1,
					Content:     "Good point, I'll create an `AuthError` type with an error code field.",
					CommentType: "text", PublishedDate: hoursAgo(2), LastUpdatedDate: hoursAgo(2),
					Author: team[0],
				},
			},
		},
		{
			ID: 2, PublishedDate: hoursAgo(3), LastUpdatedDate: hoursAgo(3),
			Status: "fixed",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/config/oauth.go",
				RightFileStart: &azdevops.FilePosition{Line: 15, Offset: 1},
				RightFileEnd:   &azdevops.FilePosition{Line: 15, Offset: 1},
			},
			Comments: []azdevops.Comment{
				{
					ID: 1, Content: "Should we validate the redirect URI against a whitelist here?",
					CommentType: "text", PublishedDate: hoursAgo(3), LastUpdatedDate: hoursAgo(3),
					Author: team[3],
				},
				{
					ID: 2, ParentCommentID: 1,
					Content:     "Yes, added validation in the latest push. Only registered URIs are accepted now.",
					CommentType: "text", PublishedDate: hoursAgo(2), LastUpdatedDate: hoursAgo(2),
					Author: team[0],
				},
			},
		},
		{
			ID: 3, PublishedDate: hoursAgo(2), LastUpdatedDate: hoursAgo(2),
			Status: "active",
			Comments: []azdevops.Comment{
				{
					ID: 1, Content: "Overall looks great! Just the two inline comments to address. The OAuth2 flow is well implemented.",
					CommentType: "text", PublishedDate: hoursAgo(2), LastUpdatedDate: hoursAgo(2),
					Author: team[1],
				},
			},
		},
		{
			ID: 4, PublishedDate: hoursAgo(1), LastUpdatedDate: hoursAgo(1),
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/handlers/token.go",
				RightFileStart: &azdevops.FilePosition{Line: 78, Offset: 1},
				RightFileEnd:   &azdevops.FilePosition{Line: 78, Offset: 1},
			},
			Comments: []azdevops.Comment{
				{
					ID: 1, Content: "The token refresh logic should handle concurrent requests. Consider using `singleflight` to deduplicate refresh calls.",
					CommentType: "text", PublishedDate: hoursAgo(1), LastUpdatedDate: hoursAgo(1),
					Author: team[3],
				},
			},
		},
	}
}

func mockTimeline() azdevops.Timeline {
	stageID := "stage-build-001"
	jobBuildID := "job-build-001"
	jobTestID := "job-test-001"
	stageDeployID := "stage-deploy-001"
	jobDeployID := "job-deploy-001"

	buildStart := hoursAgo(2)
	buildFinish := buildStart.Add(2 * time.Minute)
	testStart := buildFinish
	testFinish := testStart.Add(5 * time.Minute)
	deployStart := testFinish
	deployFinish := deployStart.Add(3 * time.Minute)

	return azdevops.Timeline{
		ID:       "timeline-001",
		ChangeID: 1,
		Records: []azdevops.TimelineRecord{
			// Stage: Build
			{
				ID: stageID, ParentID: nil, Type: "Stage", Name: "Build",
				State: "completed", Result: "succeeded", Order: 1,
				StartTime: &buildStart, FinishTime: &testFinish,
			},
			// Job: Compile & Package
			{
				ID: jobBuildID, ParentID: &stageID, Type: "Job", Name: "Compile & Package",
				State: "completed", Result: "succeeded", Order: 1,
				StartTime: &buildStart, FinishTime: &buildFinish,
			},
			// Tasks under Compile & Package
			{
				ID: "task-restore-001", ParentID: &jobBuildID, Type: "Task", Name: "Restore dependencies",
				State: "completed", Result: "succeeded", Order: 1,
				StartTime: &buildStart, FinishTime: ptr(buildStart.Add(30 * time.Second)),
				Log: &azdevops.LogReference{ID: 1, Type: "Container"},
			},
			{
				ID: "task-build-001", ParentID: &jobBuildID, Type: "Task", Name: "Build solution",
				State: "completed", Result: "succeeded", Order: 2,
				StartTime: ptr(buildStart.Add(30 * time.Second)), FinishTime: ptr(buildStart.Add(90 * time.Second)),
				Log: &azdevops.LogReference{ID: 2, Type: "Container"},
			},
			{
				ID: "task-publish-001", ParentID: &jobBuildID, Type: "Task", Name: "Publish artifacts",
				State: "completed", Result: "succeeded", Order: 3,
				StartTime: ptr(buildStart.Add(90 * time.Second)), FinishTime: &buildFinish,
				Log: &azdevops.LogReference{ID: 3, Type: "Container"},
			},
			// Job: Run Tests
			{
				ID: jobTestID, ParentID: &stageID, Type: "Job", Name: "Run Tests",
				State: "completed", Result: "succeeded", Order: 2,
				StartTime: &testStart, FinishTime: &testFinish,
			},
			{
				ID: "task-unit-001", ParentID: &jobTestID, Type: "Task", Name: "Unit tests",
				State: "completed", Result: "succeeded", Order: 1,
				StartTime: &testStart, FinishTime: ptr(testStart.Add(2 * time.Minute)),
				Log: &azdevops.LogReference{ID: 4, Type: "Container"},
			},
			{
				ID: "task-integ-001", ParentID: &jobTestID, Type: "Task", Name: "Integration tests",
				State: "completed", Result: "succeededWithIssues", Order: 2,
				StartTime: ptr(testStart.Add(2 * time.Minute)), FinishTime: &testFinish,
				Log:    &azdevops.LogReference{ID: 5, Type: "Container"},
				Issues: []azdevops.Issue{{Type: "warning", Message: "3 tests skipped due to flaky network dependency"}},
			},
			// Stage: Deploy
			{
				ID: stageDeployID, ParentID: nil, Type: "Stage", Name: "Deploy to Staging",
				State: "completed", Result: "succeeded", Order: 2,
				StartTime: &deployStart, FinishTime: &deployFinish,
			},
			{
				ID: jobDeployID, ParentID: &stageDeployID, Type: "Job", Name: "Deploy",
				State: "completed", Result: "succeeded", Order: 1,
				StartTime: &deployStart, FinishTime: &deployFinish,
			},
			{
				ID: "task-deploy-001", ParentID: &jobDeployID, Type: "Task", Name: "Deploy to Azure App Service",
				State: "completed", Result: "succeeded", Order: 1,
				StartTime: &deployStart, FinishTime: ptr(deployStart.Add(2 * time.Minute)),
				Log: &azdevops.LogReference{ID: 6, Type: "Container"},
			},
			{
				ID: "task-smoke-001", ParentID: &jobDeployID, Type: "Task", Name: "Smoke tests",
				State: "completed", Result: "succeeded", Order: 2,
				StartTime: ptr(deployStart.Add(2 * time.Minute)), FinishTime: &deployFinish,
				Log: &azdevops.LogReference{ID: 7, Type: "Container"},
			},
		},
	}
}

func mockWorkItemTypeStates(workItemType string) []azdevops.WorkItemTypeState {
	switch workItemType {
	case "Bug":
		return []azdevops.WorkItemTypeState{
			{Name: "New", Color: "b2b2b2", Category: "Proposed"},
			{Name: "Active", Color: "007acc", Category: "InProgress"},
			{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
			{Name: "Closed", Color: "339933", Category: "Completed"},
		}
	case "User Story":
		return []azdevops.WorkItemTypeState{
			{Name: "New", Color: "b2b2b2", Category: "Proposed"},
			{Name: "Active", Color: "007acc", Category: "InProgress"},
			{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
			{Name: "Closed", Color: "339933", Category: "Completed"},
		}
	case "Task":
		return []azdevops.WorkItemTypeState{
			{Name: "New", Color: "b2b2b2", Category: "Proposed"},
			{Name: "Active", Color: "007acc", Category: "InProgress"},
			{Name: "Closed", Color: "339933", Category: "Completed"},
		}
	default:
		return []azdevops.WorkItemTypeState{
			{Name: "New", Color: "b2b2b2", Category: "Proposed"},
			{Name: "Active", Color: "007acc", Category: "InProgress"},
			{Name: "Closed", Color: "339933", Category: "Completed"},
		}
	}
}

func mockPRIterations() []azdevops.Iteration {
	return []azdevops.Iteration{
		{ID: 1, Description: "Initial implementation"},
		{ID: 2, Description: "Address review feedback"},
		{ID: 3, Description: "Fix CI failures"},
	}
}

func mockIterationChanges() []azdevops.IterationChange {
	return []azdevops.IterationChange{
		{
			ChangeID: 1, ChangeType: "edit",
			Item: azdevops.ChangeItem{ObjectID: "obj-001", Path: "/src/middleware/auth.go", GitObjectType: "blob"},
		},
		{
			ChangeID: 2, ChangeType: "add",
			Item: azdevops.ChangeItem{ObjectID: "obj-002", Path: "/src/config/oauth.go", GitObjectType: "blob"},
		},
		{
			ChangeID: 3, ChangeType: "edit",
			Item: azdevops.ChangeItem{ObjectID: "obj-003", Path: "/src/handlers/token.go", GitObjectType: "blob"},
		},
		{
			ChangeID: 4, ChangeType: "add",
			Item: azdevops.ChangeItem{ObjectID: "obj-004", Path: "/src/middleware/auth_test.go", GitObjectType: "blob"},
		},
		{
			ChangeID: 5, ChangeType: "delete",
			Item: azdevops.ChangeItem{ObjectID: "obj-005", Path: "/src/middleware/legacy_auth.go", GitObjectType: "blob"},
		},
	}
}

func mockBuildLogs() []azdevops.BuildLog {
	return []azdevops.BuildLog{
		{ID: 1, Type: "Container", LineCount: 15},
		{ID: 2, Type: "Container", LineCount: 25},
		{ID: 3, Type: "Container", LineCount: 10},
		{ID: 4, Type: "Container", LineCount: 45},
		{ID: 5, Type: "Container", LineCount: 30},
		{ID: 6, Type: "Container", LineCount: 20},
		{ID: 7, Type: "Container", LineCount: 12},
	}
}

func mockBuildLogContent(logID int) string {
	logs := map[int]string{
		1: "Restoring packages...\nRestored 142 packages in 8.2s\nAll packages restored successfully.",
		2: "Building solution...\nCompiling 24 projects...\n  nexus-core -> bin/Release/net8.0/nexus-core.dll\n  nexus-api -> bin/Release/net8.0/nexus-api.dll\nBuild succeeded.\n    0 Warning(s)\n    0 Error(s)\nTime Elapsed 00:00:58.12",
		3: "Publishing artifacts...\nArtifact 'drop' published with 3 files.\nUpload completed successfully.",
		4: fmt.Sprintf("Running unit tests...\nTest run for nexus-core.Tests.dll\nPassed!  - Failed: 0, Passed: 247, Skipped: 0, Total: 247\nDuration: 1m 42s\nResults File: TestResults.trx"),
		5: "Running integration tests...\nTest run for nexus-api.IntegrationTests.dll\nWarning: 3 tests skipped (flaky network dependency)\nPassed!  - Failed: 0, Passed: 89, Skipped: 3, Total: 92\nDuration: 4m 58s",
		6: "Deploying to Azure App Service...\nPackage validated successfully.\nDeploying to slot: staging\nDeployment successful.\nApp URL: https://nexus-staging.azurewebsites.net",
		7: "Running smoke tests...\nGET /health -> 200 OK (42ms)\nGET /api/v1/status -> 200 OK (65ms)\nPOST /api/v1/auth/token -> 200 OK (128ms)\nAll smoke tests passed (3/3).",
	}
	if content, ok := logs[logID]; ok {
		return content
	}
	return "Log content not available."
}

// mockFileContent returns file content for a given path and branch.
// For "edit" files, the target branch (old) and source branch (new) differ
// so the diff view shows both additions and removals.
func mockFileContent(filePath, branch string) string {
	key := filePath + ":" + branch

	contents := map[string]string{
		// auth.go — target (main) version: old token-based auth
		"/src/middleware/auth.go:main": `package auth

import (
	"context"
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware that validates tokens.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			// Legacy: validate with shared secret
			if !validateToken(token, secret) {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

type contextKey string

const claimsKey contextKey = "claims"

// validateToken checks the token against a shared secret.
func validateToken(token, secret string) bool {
	return token == secret
}
`,
		// auth.go — source (feature) version: new OAuth2 flow
		"/src/middleware/auth.go:feature/auth-refactor": `package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware that validates OAuth2 tokens.
func Middleware(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			claims, err := validator.Validate(r.Context(), token)
			if err != nil {
				slog.Warn("token validation failed",
					"error", err,
					"remote_addr", r.RemoteAddr,
				)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

type contextKey string

const claimsKey contextKey = "claims"

// TokenValidator validates OAuth2 tokens.
type TokenValidator interface {
	Validate(ctx context.Context, token string) (*Claims, error)
}

// Claims represents the validated token claims.
type Claims struct {
	UserID    string
	Email     string
	Roles     []string
}
`,
		// token.go — target (main) version
		"/src/handlers/token.go:main": `package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

// TokenHandler handles token refresh requests.
type TokenHandler struct {
	secret   string
	lifetime time.Duration
}

// NewTokenHandler creates a new token handler.
func NewTokenHandler(secret string, lifetime time.Duration) *TokenHandler {
	return &TokenHandler{secret: secret, lifetime: lifetime}
}

// HandleRefresh handles POST /auth/refresh requests.
func (h *TokenHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RefreshToken string ` + "`" + `json:"refresh_token"` + "`" + `
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Generate new token
	token := generateToken(h.secret, h.lifetime)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": token,
		"token_type":   "Bearer",
	})
}

func generateToken(secret string, lifetime time.Duration) string {
	return secret + ":" + time.Now().Add(lifetime).Format(time.RFC3339)
}
`,
		// token.go — source (feature) version: uses OAuth2 provider
		"/src/handlers/token.go:feature/auth-refactor": `package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"
)

// TokenHandler handles token refresh requests.
type TokenHandler struct {
	provider TokenProvider
	lifetime time.Duration
	group    singleflight.Group
}

// TokenProvider abstracts the OAuth2 token provider.
type TokenProvider interface {
	RefreshToken(refreshToken string) (string, error)
}

// NewTokenHandler creates a new token handler.
func NewTokenHandler(provider TokenProvider, lifetime time.Duration) *TokenHandler {
	return &TokenHandler{provider: provider, lifetime: lifetime}
}

// HandleRefresh handles POST /auth/refresh requests.
// Uses singleflight to deduplicate concurrent refresh requests.
func (h *TokenHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RefreshToken string ` + "`" + `json:"refresh_token"` + "`" + `
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Deduplicate concurrent refreshes for the same token
	result, err, _ := h.group.Do(req.RefreshToken, func() (interface{}, error) {
		return h.provider.RefreshToken(req.RefreshToken)
	})
	if err != nil {
		http.Error(w, "token refresh failed", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": result.(string),
		"token_type":   "Bearer",
		"expires_in":   h.lifetime.String(),
	})
}
`,
	}

	if content, ok := contents[key]; ok {
		return content
	}

	// Fallback: return a generic file for add/delete cases or unknown paths
	return `package placeholder

// This file is part of the demo.
func init() {}
`
}
