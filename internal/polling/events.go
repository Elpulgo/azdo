// Package polling provides background polling functionality for Azure DevOps data
// with tea.Msg types for Bubble Tea integration.
package polling

import "github.com/Elpulgo/azdo/internal/azdevops"

// PipelineRunsUpdated is a tea.Msg sent when pipeline runs are fetched.
// It contains either the updated runs or an error.
type PipelineRunsUpdated struct {
	Runs []azdevops.PipelineRun
	Err  error
}

// TickMsg is a tea.Msg sent on each polling interval tick.
// It signals that it's time to fetch updated data.
type TickMsg struct{}

// ConnectionState represents the current state of the API connection.
type ConnectionState int

const (
	// StateConnected indicates successful API communication
	StateConnected ConnectionState = iota
	// StateConnecting indicates an initial connection attempt
	StateConnecting
	// StateDisconnected indicates no active connection
	StateDisconnected
	// StateError indicates a connection error occurred
	StateError
)

// String returns a human-readable string for the connection state.
func (s ConnectionState) String() string {
	switch s {
	case StateConnected:
		return "connected"
	case StateConnecting:
		return "connecting"
	case StateDisconnected:
		return "disconnected"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// ConnectionStateChanged is a tea.Msg sent when the connection state changes.
type ConnectionStateChanged struct {
	State ConnectionState
	Err   error
}
