package azdevops_test

import (
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
