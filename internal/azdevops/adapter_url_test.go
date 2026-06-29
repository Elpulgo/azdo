package azdevops_test

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// newTestMultiClient creates a MultiClient with a single project for URL shape tests.
// NewMultiClient + NewClient build the struct without any network call, so this
// is safe to use in unit tests.
func newTestMultiClient(t *testing.T, org, project string) *azdevops.MultiClient {
	t.Helper()
	mc, err := azdevops.NewMultiClient(org, []string{project}, "dummy-pat", nil)
	if err != nil {
		t.Fatalf("newTestMultiClient(%q, %q): %v", org, project, err)
	}
	return mc
}

// TestAdapter_WorkItemURL_Shapes table-tests the exact URL shapes that WorkItemURL
// must produce, matching the inline builder in workitems/detail.go.
func TestAdapter_WorkItemURL_Shapes(t *testing.T) {
	tests := []struct {
		name    string
		org     string
		project string
		scope   string
		id      int
		want    string
	}{
		{
			name:    "standard URL",
			org:     "myorg",
			project: "myproject",
			scope:   "myproject",
			id:      123,
			want:    "https://dev.azure.com/myorg/myproject/_workitems/edit/123",
		},
		{
			name:    "scope not in client returns empty",
			org:     "myorg",
			project: "myproject",
			scope:   "otherproject",
			id:      123,
			want:    "",
		},
		{
			name:    "zero id still builds URL",
			org:     "myorg",
			project: "myproject",
			scope:   "myproject",
			id:      0,
			want:    "https://dev.azure.com/myorg/myproject/_workitems/edit/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newTestMultiClient(t, tt.org, tt.project)
			a := azdevops.NewAdapter(mc)
			got := a.WorkItemURL(tt.scope, tt.id)
			if got != tt.want {
				t.Errorf("WorkItemURL(%q, %d) = %q, want %q", tt.scope, tt.id, got, tt.want)
			}
		})
	}
}

// TestAdapter_PRURL_Shapes table-tests the exact URL shapes that PRURL must
// produce, matching the inline builder in pullrequests/detail.go.
func TestAdapter_PRURL_Shapes(t *testing.T) {
	tests := []struct {
		name       string
		org        string
		project    string
		scope      string
		repoID     string
		prID       int
		want       string
	}{
		{
			name:    "standard PR overview URL",
			org:     "myorg",
			project: "myproject",
			scope:   "myproject",
			repoID:  "repo-guid-123",
			prID:    42,
			want:    "https://dev.azure.com/myorg/myproject/_git/repo-guid-123/pullrequest/42",
		},
		{
			name:    "scope not in client returns empty",
			org:     "myorg",
			project: "myproject",
			scope:   "otherproject",
			repoID:  "repo-guid-123",
			prID:    42,
			want:    "",
		},
		{
			name:    "empty repoID returns empty",
			org:     "myorg",
			project: "myproject",
			scope:   "myproject",
			repoID:  "",
			prID:    42,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newTestMultiClient(t, tt.org, tt.project)
			a := azdevops.NewAdapter(mc)
			got := a.PRURL(tt.scope, tt.repoID, tt.prID)
			if got != tt.want {
				t.Errorf("PRURL(%q, %q, %d) = %q, want %q", tt.scope, tt.repoID, tt.prID, got, tt.want)
			}
		})
	}
}

// TestAdapter_PRThreadWebURL_Shapes table-tests the exact URL shapes that
// PRThreadWebURL must produce, matching the inline buildPRThreadURL helper in
// pullrequests/detail.go which builds ?discussionId=<threadID>.
func TestAdapter_PRThreadWebURL_Shapes(t *testing.T) {
	tests := []struct {
		name     string
		org      string
		project  string
		scope    string
		repoID   string
		prID     int
		threadID int
		want     string
	}{
		{
			name:     "standard thread URL includes discussionId",
			org:      "myorg",
			project:  "myproject",
			scope:    "myproject",
			repoID:   "repo-guid-123",
			prID:     123,
			threadID: 456,
			want:     "https://dev.azure.com/myorg/myproject/_git/repo-guid-123/pullrequest/123?discussionId=456",
		},
		{
			name:     "scope not in client returns empty",
			org:      "myorg",
			project:  "myproject",
			scope:    "otherproject",
			repoID:   "repo-guid-123",
			prID:     123,
			threadID: 456,
			want:     "",
		},
		{
			name:     "zero threadID returns empty",
			org:      "myorg",
			project:  "myproject",
			scope:    "myproject",
			repoID:   "repo-guid-123",
			prID:     123,
			threadID: 0,
			want:     "",
		},
		{
			name:     "empty repoID returns empty",
			org:      "myorg",
			project:  "myproject",
			scope:    "myproject",
			repoID:   "",
			prID:     123,
			threadID: 456,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newTestMultiClient(t, tt.org, tt.project)
			a := azdevops.NewAdapter(mc)
			got := a.PRThreadWebURL(tt.scope, tt.repoID, tt.prID, tt.threadID)
			if got != tt.want {
				t.Errorf("PRThreadWebURL(%q, %q, %d, %d) = %q, want %q",
					tt.scope, tt.repoID, tt.prID, tt.threadID, got, tt.want)
			}
		})
	}
}

// TestAdapter_WorkItemURL_NilClient asserts that URL methods return "" when
// the underlying MultiClient is nil.
func TestAdapter_WorkItemURL_NilClient(t *testing.T) {
	a := azdevops.NewAdapter(nil)
	if got := a.WorkItemURL("anyproject", 1); got != "" {
		t.Errorf("WorkItemURL with nil client = %q, want %q", got, "")
	}
}

// TestAdapter_PRURL_NilClient asserts that URL methods return "" when
// the underlying MultiClient is nil.
func TestAdapter_PRURL_NilClient(t *testing.T) {
	a := azdevops.NewAdapter(nil)
	if got := a.PRURL("anyproject", "repo", 1); got != "" {
		t.Errorf("PRURL with nil client = %q, want %q", got, "")
	}
}

// TestAdapter_PRThreadWebURL_NilClient asserts that PRThreadWebURL returns ""
// when the underlying MultiClient is nil.
func TestAdapter_PRThreadWebURL_NilClient(t *testing.T) {
	a := azdevops.NewAdapter(nil)
	if got := a.PRThreadWebURL("anyproject", "repo", 1, 42); got != "" {
		t.Errorf("PRThreadWebURL with nil client = %q, want %q", got, "")
	}
}
