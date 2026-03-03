package polling

import (
	"testing"
)

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
