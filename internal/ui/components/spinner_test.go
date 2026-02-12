package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func TestLoadingIndicator_New(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())

	if li == nil {
		t.Fatal("expected non-nil LoadingIndicator")
	}
	if li.message != "Loading..." {
		t.Errorf("expected default message 'Loading...', got '%s'", li.message)
	}
}

func TestLoadingIndicator_SetMessage(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())
	li.SetMessage("Fetching pipelines...")

	if li.message != "Fetching pipelines..." {
		t.Errorf("expected message 'Fetching pipelines...', got '%s'", li.message)
	}
}

func TestLoadingIndicator_SetVisible(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())

	li.SetVisible(true)
	if !li.visible {
		t.Error("expected visible to be true")
	}

	li.SetVisible(false)
	if li.visible {
		t.Error("expected visible to be false")
	}
}

func TestLoadingIndicator_IsVisible(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())

	if li.IsVisible() {
		t.Error("expected IsVisible to be false by default")
	}

	li.SetVisible(true)
	if !li.IsVisible() {
		t.Error("expected IsVisible to be true after SetVisible(true)")
	}
}

func TestLoadingIndicator_View_WhenVisible(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())
	li.SetVisible(true)
	li.SetMessage("Loading data...")

	view := li.View()

	// Should contain the message
	if !strings.Contains(view, "Loading data...") {
		t.Error("view should contain the loading message when visible")
	}
}

func TestLoadingIndicator_View_WhenNotVisible(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())
	li.SetVisible(false)

	view := li.View()

	// Should be empty when not visible
	if view != "" {
		t.Errorf("view should be empty when not visible, got '%s'", view)
	}
}

func TestLoadingIndicator_Init_ReturnsSpinnerTick(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())
	cmd := li.Init()

	if cmd == nil {
		t.Fatal("Init should return a command for spinner tick")
	}
}

func TestLoadingIndicator_Update_HandlesSpinnerMsg(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())

	// Create a spinner tick message
	tickMsg := spinner.TickMsg{}

	model, cmd := li.Update(tickMsg)
	if model == nil {
		t.Error("Update should return the model")
	}
	// Spinner will return a cmd for the next tick
	_ = cmd // cmd may or may not be nil depending on spinner state
}

func TestLoadingIndicator_Update_IgnoresOtherMessages(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())
	li.SetMessage("Original")

	// Send a random message type
	model, cmd := li.Update(tea.KeyMsg{})

	if model == nil {
		t.Error("Update should return the model")
	}
	if li.message != "Original" {
		t.Error("message should not change for unrelated messages")
	}
	if cmd != nil {
		t.Error("should return nil cmd for unrelated messages")
	}
}

func TestLoadingIndicator_SpinnerStyle(t *testing.T) {
	// Test that spinner can be created with different styles
	li := NewLoadingIndicator(styles.DefaultStyles())

	// Should not panic
	li.SetVisible(true)
	view := li.View()
	if len(view) == 0 {
		t.Error("view should not be empty when visible")
	}
}

func TestLoadingIndicator_Toggle(t *testing.T) {
	li := NewLoadingIndicator(styles.DefaultStyles())

	// Initial state
	if li.IsVisible() {
		t.Error("should start not visible")
	}

	// Toggle on
	li.Toggle()
	if !li.IsVisible() {
		t.Error("should be visible after first toggle")
	}

	// Toggle off
	li.Toggle()
	if li.IsVisible() {
		t.Error("should not be visible after second toggle")
	}
}
