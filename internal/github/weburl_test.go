package github

import "testing"

// TestClient_WorkItemURL_Shapes table-tests the exact URL shapes WorkItemURL produces.
func TestClient_WorkItemURL_Shapes(t *testing.T) {
	c := NewClient("octocat", "Hello-World", "tok")

	tests := []struct {
		name string
		id   int
		want string
	}{
		{
			name: "standard issue URL",
			id:   42,
			want: "https://github.com/octocat/Hello-World/issues/42",
		},
		{
			name: "zero id returns empty",
			id:   0,
			want: "",
		},
		{
			name: "negative id returns empty",
			id:   -1,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.WorkItemURL(tt.id)
			if got != tt.want {
				t.Errorf("WorkItemURL(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestClient_PRURL_Shapes table-tests the exact URL shapes PRURL produces.
func TestClient_PRURL_Shapes(t *testing.T) {
	c := NewClient("octocat", "Hello-World", "tok")

	tests := []struct {
		name string
		prID int
		want string
	}{
		{
			name: "standard PR URL",
			prID: 7,
			want: "https://github.com/octocat/Hello-World/pull/7",
		},
		{
			name: "zero prID returns empty",
			prID: 0,
			want: "",
		},
		{
			name: "negative prID returns empty",
			prID: -5,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.PRURL(tt.prID)
			if got != tt.want {
				t.Errorf("PRURL(%d) = %q, want %q", tt.prID, got, tt.want)
			}
		})
	}
}

// TestClient_PRThreadWebURL_Shapes table-tests the exact URL shapes PRThreadWebURL produces.
func TestClient_PRThreadWebURL_Shapes(t *testing.T) {
	c := NewClient("octocat", "Hello-World", "tok")

	tests := []struct {
		name     string
		prID     int
		threadID int
		want     string
	}{
		{
			name:     "standard thread URL with discussion anchor",
			prID:     7,
			threadID: 123456,
			want:     "https://github.com/octocat/Hello-World/pull/7#discussion_r123456",
		},
		{
			name:     "zero threadID returns empty",
			prID:     7,
			threadID: 0,
			want:     "",
		},
		{
			name:     "zero prID returns empty",
			prID:     0,
			threadID: 123456,
			want:     "",
		},
		{
			name:     "negative prID returns empty",
			prID:     -1,
			threadID: 123456,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.PRThreadWebURL(tt.prID, tt.threadID)
			if got != tt.want {
				t.Errorf("PRThreadWebURL(%d, %d) = %q, want %q", tt.prID, tt.threadID, got, tt.want)
			}
		})
	}
}

// TestClient_PipelineURL_Shapes table-tests the exact URL shapes PipelineURL produces.
func TestClient_PipelineURL_Shapes(t *testing.T) {
	c := NewClient("octocat", "Hello-World", "tok")

	tests := []struct {
		name string
		id   int
		want string
	}{
		{
			name: "standard pipeline run URL",
			id:   99,
			want: "https://github.com/octocat/Hello-World/actions/runs/99",
		},
		{
			name: "zero id returns empty",
			id:   0,
			want: "",
		},
		{
			name: "negative id returns empty",
			id:   -3,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.PipelineURL(tt.id)
			if got != tt.want {
				t.Errorf("PipelineURL(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
