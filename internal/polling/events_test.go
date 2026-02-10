package polling

import (
	"errors"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

func TestPipelineRunsUpdated_WithRuns(t *testing.T) {
	// Given: a list of pipeline runs
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "20240101.1"},
		{ID: 2, BuildNumber: "20240101.2"},
	}

	// When: creating a PipelineRunsUpdated message
	msg := PipelineRunsUpdated{Runs: runs, Err: nil}

	// Then: the message should contain the runs
	if len(msg.Runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(msg.Runs))
	}
	if msg.Runs[0].ID != 1 {
		t.Errorf("expected first run ID to be 1, got %d", msg.Runs[0].ID)
	}
	if msg.Err != nil {
		t.Error("expected Err to be nil")
	}
}

func TestPipelineRunsUpdated_WithError(t *testing.T) {
	// Given: an error
	expectedErr := errors.New("API connection failed")

	// When: creating a PipelineRunsUpdated message with an error
	msg := PipelineRunsUpdated{Runs: nil, Err: expectedErr}

	// Then: the message should contain the error
	if msg.Err == nil {
		t.Error("expected Err to be set")
	}
	if msg.Err.Error() != "API connection failed" {
		t.Errorf("expected error message 'API connection failed', got '%s'", msg.Err.Error())
	}
	if msg.Runs != nil {
		t.Error("expected Runs to be nil when error is present")
	}
}

func TestPipelineRunsUpdated_IsTeaMsg(t *testing.T) {
	// PipelineRunsUpdated should be usable as a tea.Msg (any type)
	// This test verifies the type can be used in a type switch
	msg := PipelineRunsUpdated{}

	// Type assertion test - should compile and work
	var teaMsg interface{} = msg
	switch m := teaMsg.(type) {
	case PipelineRunsUpdated:
		// Success - the type assertion works
		_ = m
	default:
		t.Error("PipelineRunsUpdated should be usable as tea.Msg")
	}
}

func TestTickMsg(t *testing.T) {
	// Given: a TickMsg
	msg := TickMsg{}

	// Then: it should be usable as a tea.Msg
	var teaMsg interface{} = msg
	switch m := teaMsg.(type) {
	case TickMsg:
		_ = m
	default:
		t.Error("TickMsg should be usable as tea.Msg")
	}
}

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateConnected, "connected"},
		{StateConnecting, "connecting"},
		{StateDisconnected, "disconnected"},
		{StateError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("ConnectionState.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestConnectionStateChanged(t *testing.T) {
	// Given: a connection state change
	msg := ConnectionStateChanged{State: StateConnected}

	// Then: it should contain the new state
	if msg.State != StateConnected {
		t.Errorf("expected StateConnected, got %v", msg.State)
	}

	// And it should be usable as tea.Msg
	var teaMsg interface{} = msg
	switch m := teaMsg.(type) {
	case ConnectionStateChanged:
		if m.State != StateConnected {
			t.Error("state should be preserved through type assertion")
		}
	default:
		t.Error("ConnectionStateChanged should be usable as tea.Msg")
	}
}

func TestConnectionStateChanged_WithError(t *testing.T) {
	// Given: an error state
	err := errors.New("network timeout")
	msg := ConnectionStateChanged{State: StateError, Err: err}

	// Then: it should contain both state and error
	if msg.State != StateError {
		t.Errorf("expected StateError, got %v", msg.State)
	}
	if msg.Err == nil || msg.Err.Error() != "network timeout" {
		t.Error("expected error to be preserved")
	}
}
