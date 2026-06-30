package azdevops_test

import (
	"sort"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
)

// TestAdapterKind asserts that the Adapter reports KindAzure.
func TestAdapterKind(t *testing.T) {
	// We cannot call NewMultiClient without a real server, so we test via the
	// exported constructor that accepts a pre-built MultiClient.
	// Since NewMultiClient requires network access, use a nil pointer — Kind()
	// must not dereference the underlying client.
	a := azdevops.NewAdapter(nil)
	if got := a.Kind(); got != provider.KindAzure {
		t.Errorf("Adapter.Kind() = %v, want %v", got, provider.KindAzure)
	}
}

// TestAdapterIsMultiProject_nil asserts that IsMultiProject returns false
// when the underlying MultiClient is nil (zero projects).
func TestAdapterIsMultiProject_nil(t *testing.T) {
	a := azdevops.NewAdapter(nil)
	if a.IsMultiProject() {
		t.Error("expected IsMultiProject() = false for nil MultiClient")
	}
}

// TestAdapterScopes table-tests Scopes() for the Azure DevOps adapter.
func TestAdapterScopes(t *testing.T) {
	tests := []struct {
		name     string
		projects []string // nil signals: use a nil MultiClient
		want     []string // sorted expected scopes; nil means nil return
	}{
		{
			name:     "nil MultiClient returns nil",
			projects: nil,
			want:     nil,
		},
		{
			name:     "single project",
			projects: []string{"alpha"},
			want:     []string{"alpha"},
		},
		{
			name:     "multiple projects",
			projects: []string{"alpha", "beta", "gamma"},
			want:     []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a *azdevops.Adapter
			if tt.projects == nil {
				a = azdevops.NewAdapter(nil)
			} else {
				mc, err := azdevops.NewMultiClient("org", tt.projects, "pat", nil)
				if err != nil {
					t.Fatalf("NewMultiClient: %v", err)
				}
				a = azdevops.NewAdapter(mc)
			}

			got := a.Scopes()

			if tt.want == nil {
				if got != nil {
					t.Errorf("Scopes() = %v, want nil", got)
				}
				return
			}

			sort.Strings(got)
			if len(got) != len(tt.want) {
				t.Fatalf("Scopes() = %v, want %v", got, tt.want)
			}
			for i, s := range tt.want {
				if got[i] != s {
					t.Errorf("Scopes()[%d] = %q, want %q", i, got[i], s)
				}
			}
		})
	}
}
