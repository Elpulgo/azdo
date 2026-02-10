package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/polling"
)

func TestStatusBar_New(t *testing.T) {
	sb := NewStatusBar()

	if sb == nil {
		t.Fatal("expected non-nil StatusBar")
	}
	if sb.state != polling.StateConnecting {
		t.Errorf("expected initial state to be Connecting, got %v", sb.state)
	}
}

func TestStatusBar_SetOrganization(t *testing.T) {
	sb := NewStatusBar()
	sb.SetOrganization("myorg")

	if sb.organization != "myorg" {
		t.Errorf("expected organization 'myorg', got '%s'", sb.organization)
	}
}

func TestStatusBar_SetProject(t *testing.T) {
	sb := NewStatusBar()
	sb.SetProject("myproject")

	if sb.project != "myproject" {
		t.Errorf("expected project 'myproject', got '%s'", sb.project)
	}
}

func TestStatusBar_SetState(t *testing.T) {
	sb := NewStatusBar()
	sb.SetState(polling.StateConnected)

	if sb.state != polling.StateConnected {
		t.Errorf("expected state StateConnected, got %v", sb.state)
	}
}

func TestStatusBar_SetWidth(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(100)

	if sb.width != 100 {
		t.Errorf("expected width 100, got %d", sb.width)
	}
}

func TestStatusBar_View_ContainsOrganization(t *testing.T) {
	sb := NewStatusBar()
	sb.SetOrganization("testorg")
	sb.SetProject("testproject")
	sb.SetWidth(80)

	view := sb.View()

	if !strings.Contains(view, "testorg") {
		t.Error("view should contain organization name")
	}
}

func TestStatusBar_View_ContainsProject(t *testing.T) {
	sb := NewStatusBar()
	sb.SetOrganization("testorg")
	sb.SetProject("testproject")
	sb.SetWidth(80)

	view := sb.View()

	if !strings.Contains(view, "testproject") {
		t.Error("view should contain project name")
	}
}

func TestStatusBar_View_Connected_ShowsConnected(t *testing.T) {
	sb := NewStatusBar()
	sb.SetState(polling.StateConnected)
	sb.SetWidth(80)

	view := sb.View()

	// Should show connected indicator
	if !strings.Contains(strings.ToLower(view), "connected") {
		t.Error("view should indicate connected state")
	}
}

func TestStatusBar_View_Error_ShowsError(t *testing.T) {
	sb := NewStatusBar()
	sb.SetState(polling.StateError)
	sb.SetWidth(80)

	view := sb.View()

	// Should show error indicator
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Error("view should indicate error state")
	}
}

func TestStatusBar_View_ContainsHelpHint(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(80)

	view := sb.View()

	// Should show help hint with question mark
	if !strings.Contains(view, "?") || !strings.Contains(strings.ToLower(view), "help") {
		t.Error("view should contain help hint")
	}
}

func TestStatusBar_SetHelpText(t *testing.T) {
	sb := NewStatusBar()
	sb.SetHelpText("Press r to refresh")
	sb.SetWidth(80)

	view := sb.View()

	if !strings.Contains(view, "Press r to refresh") {
		t.Errorf("view should contain custom help text, got: %s", view)
	}
}

func TestStatusBar_StateIcons(t *testing.T) {
	tests := []struct {
		state       polling.ConnectionState
		expectColor bool // We just verify it doesn't panic
	}{
		{polling.StateConnected, true},
		{polling.StateConnecting, true},
		{polling.StateDisconnected, true},
		{polling.StateError, true},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			sb := NewStatusBar()
			sb.SetState(tt.state)
			sb.SetWidth(80)

			// Should not panic
			view := sb.View()
			if len(view) == 0 {
				t.Error("view should not be empty")
			}
		})
	}
}

func TestStatusBar_View_MinimumWidth(t *testing.T) {
	sb := NewStatusBar()
	sb.SetOrganization("org")
	sb.SetProject("project")
	sb.SetState(polling.StateConnected)
	sb.SetWidth(20) // Very narrow

	// Should not panic with minimal width
	view := sb.View()
	if view == "" {
		t.Error("view should not be empty even with minimal width")
	}
}

func TestStatusBar_Update_ReturnsModel(t *testing.T) {
	sb := NewStatusBar()

	// StatusBar doesn't handle messages, but Update should return the model unchanged
	model, cmd := sb.Update(nil)
	if model != sb {
		t.Error("Update should return the same model")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
}

func TestStatusBar_Init_ReturnsNil(t *testing.T) {
	sb := NewStatusBar()
	cmd := sb.Init()

	if cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestStatusBar_OrgProjectSeparator(t *testing.T) {
	sb := NewStatusBar()
	sb.SetOrganization("myorg")
	sb.SetProject("myproject")
	sb.SetWidth(80)

	view := sb.View()

	// Should have org/project or org · project format
	if !strings.Contains(view, "/") && !strings.Contains(view, "·") {
		t.Error("view should contain org/project separator")
	}
}
