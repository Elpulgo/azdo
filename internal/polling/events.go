// Package polling provides background polling of pipeline data (across all
// configured provider backends) with tea.Msg types for Bubble Tea integration.
package polling

import "github.com/Elpulgo/azdo/internal/provider"

// PipelineRunsUpdated is a tea.Msg sent when pipeline runs are fetched.
// It contains either the updated runs or an error.
type PipelineRunsUpdated struct {
	Runs []provider.PipelineRun
	Err  error
}

// TickMsg is a tea.Msg sent on each polling interval tick.
// It signals that it's time to fetch updated data.
type TickMsg struct{}

// NotificationsTickMsg is a tea.Msg sent on each notifications polling
// interval tick. It is a distinct type from TickMsg (Decision 69), not the
// same struct discriminated by a field: app.go's `case polling.TickMsg:`
// calls the pipeline poller's OnTick unconditionally, so reusing TickMsg
// here — even with an added field — would still match that case and drive
// the wrong poller.
type NotificationsTickMsg struct{}

// NotificationsFetchedMsg is a tea.Msg sent when the notifications poller's
// background fetch completes. It contains either the fetched notifications
// or an error. A nil Items with a nil Err means the inbox is genuinely
// empty, and callers MUST clear their list.
//
// An earlier version of this comment claimed the opposite — that nil/nil was
// a "nothing to update" result from a transparent 304 replay. That was wrong
// and cost a review cycle: the GitHub client answers a 304 by replaying its
// cached threads, never nil (see internal/github/notifications.go, "a 304
// must never be read as 'the inbox is now empty'"), an unsolicited 304 with
// no matching cache surfaces as an error (Decision 28), and a skipped fetch
// emits no message at all because FetchNotifications returns a nil tea.Cmd.
// No producer emits nil/nil to mean "unchanged".
type NotificationsFetchedMsg struct {
	Items []provider.Notification
	Err   error
}

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
