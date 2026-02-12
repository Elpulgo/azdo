package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/polling"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestStatusBar_New(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())

	if sb == nil {
		t.Fatal("expected non-nil StatusBar")
	}
	if sb.state != polling.StateConnecting {
		t.Errorf("expected initial state to be Connecting, got %v", sb.state)
	}
}

func TestStatusBar_SetOrganization(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetOrganization("myorg")

	if sb.organization != "myorg" {
		t.Errorf("expected organization 'myorg', got '%s'", sb.organization)
	}
}

func TestStatusBar_SetProject(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetProject("myproject")

	if sb.project != "myproject" {
		t.Errorf("expected project 'myproject', got '%s'", sb.project)
	}
}

func TestStatusBar_SetState(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetState(polling.StateConnected)

	if sb.state != polling.StateConnected {
		t.Errorf("expected state StateConnected, got %v", sb.state)
	}
}

func TestStatusBar_SetWidth(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetWidth(100)

	if sb.width != 100 {
		t.Errorf("expected width 100, got %d", sb.width)
	}
}

func TestStatusBar_View_ContainsOrganization(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetOrganization("testorg")
	sb.SetProject("testproject")
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(view, "testorg") {
		t.Error("view should contain organization name")
	}
}

func TestStatusBar_View_ContainsProject(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetOrganization("testorg")
	sb.SetProject("testproject")
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(view, "testproject") {
		t.Error("view should contain project name")
	}
}

func TestStatusBar_View_Connected_ShowsConnected(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetState(polling.StateConnected)
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(strings.ToLower(view), "connected") {
		t.Error("view should indicate connected state")
	}
}

func TestStatusBar_View_Error_ShowsError(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetState(polling.StateError)
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(strings.ToLower(view), "error") {
		t.Error("view should indicate error state")
	}
}

func TestStatusBar_View_ContainsDefaultKeybindings(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetWidth(120)

	view := sb.View()

	// Should contain default keybindings
	if !strings.Contains(view, "refresh") {
		t.Error("view should contain 'refresh' keybinding")
	}
	if !strings.Contains(view, "quit") {
		t.Error("view should contain 'quit' keybinding")
	}
	if !strings.Contains(view, "help") {
		t.Error("view should contain 'help' keybinding")
	}
}

func TestStatusBar_SetKeybindings(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetKeybindings("custom keybindings")
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(view, "custom keybindings") {
		t.Error("view should contain custom keybindings")
	}
}

func TestStatusBar_StateIcons(t *testing.T) {
	tests := []struct {
		state       polling.ConnectionState
		expectColor bool
	}{
		{polling.StateConnected, true},
		{polling.StateConnecting, true},
		{polling.StateDisconnected, true},
		{polling.StateError, true},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			sb := NewStatusBar(styles.DefaultStyles())
			sb.SetState(tt.state)
			sb.SetWidth(120)

			view := sb.View()
			if len(view) == 0 {
				t.Error("view should not be empty")
			}
		})
	}
}

func TestStatusBar_View_MinimumWidth(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetOrganization("org")
	sb.SetProject("project")
	sb.SetState(polling.StateConnected)
	sb.SetWidth(20)

	view := sb.View()
	if view == "" {
		t.Error("view should not be empty even with minimal width")
	}
}

func TestStatusBar_Update_ReturnsModel(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())

	model, cmd := sb.Update(nil)
	if model != sb {
		t.Error("Update should return the same model")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
}

func TestStatusBar_Init_ReturnsNil(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	cmd := sb.Init()

	if cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestStatusBar_OrgProjectSeparator(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetOrganization("myorg")
	sb.SetProject("myproject")
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(view, "/") {
		t.Error("view should contain org/project separator")
	}
}

func TestStatusBar_View_HasBackground(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetWidth(80)

	view := sb.View()

	// View should have ANSI codes for background color (236)
	// Just verify it's not empty and has some styling
	if len(view) < 20 {
		t.Error("view should have content with styling")
	}
}

func TestStatusBar_Update_WithKeyMsg(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())

	model, cmd := sb.Update(tea.KeyMsg{})
	if model != sb {
		t.Error("Update should return the same model for key messages")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd for key messages")
	}
}

func TestStatusBar_SetConfigPath(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetConfigPath("/home/user/.config/azdo-tui/config.yaml")

	if sb.configPath != "/home/user/.config/azdo-tui/config.yaml" {
		t.Errorf("expected configPath to be set, got '%s'", sb.configPath)
	}
}

func TestStatusBar_View_ContainsConfigPath(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetConfigPath("/home/user/.config/azdo-tui/config.yaml")
	sb.SetWidth(200)

	view := sb.View()

	if !strings.Contains(view, "config.yaml") {
		t.Error("view should contain config path")
	}
}

func TestStatusBar_SetScrollPercent(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetScrollPercent(45.5)

	if sb.scrollPercent != 45.5 {
		t.Errorf("expected scrollPercent 45.5, got %f", sb.scrollPercent)
	}
}

func TestStatusBar_ShowScrollPercent(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.ShowScrollPercent(true)

	if !sb.showScroll {
		t.Error("expected showScroll to be true")
	}

	sb.ShowScrollPercent(false)
	if sb.showScroll {
		t.Error("expected showScroll to be false")
	}
}

func TestStatusBar_View_ContainsScrollPercent(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetScrollPercent(75)
	sb.ShowScrollPercent(true)
	sb.SetWidth(120)

	view := sb.View()

	if !strings.Contains(view, "75%") {
		t.Error("view should contain scroll percentage when enabled")
	}
}

func TestStatusBar_View_NoScrollPercentWhenDisabled(t *testing.T) {
	sb := NewStatusBar(styles.DefaultStyles())
	sb.SetScrollPercent(75)
	sb.ShowScrollPercent(false)
	sb.SetWidth(120)

	view := sb.View()

	if strings.Contains(view, "75%") {
		t.Error("view should NOT contain scroll percentage when disabled")
	}
}
