package polling

import (
	"errors"
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// MaxRecoverableErrors is the threshold after which errors are considered non-recoverable.
const MaxRecoverableErrors = 5

// ErrorHandler manages error state for the polling system.
// It provides graceful degradation by returning last known good data
// when errors occur.
type ErrorHandler struct {
	currentError      error
	consecutiveErrors int
	lastErrorTime     time.Time
	lastKnownGoodData []azdevops.PipelineRun
	partialWarning    string
	mu                sync.RWMutex
}

// NewErrorHandler creates a new ErrorHandler.
func NewErrorHandler() *ErrorHandler {
	return &ErrorHandler{}
}

// SetError sets the current error and increments the consecutive error count.
func (h *ErrorHandler) SetError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.currentError = err
	h.consecutiveErrors++
	h.lastErrorTime = time.Now()
}

// ClearError clears the current error and resets the consecutive error count.
func (h *ErrorHandler) ClearError() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.currentError = nil
	h.consecutiveErrors = 0
}

// HasError returns true if there is a current error.
func (h *ErrorHandler) HasError() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.currentError != nil
}

// GetError returns the current error.
func (h *ErrorHandler) GetError() error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.currentError
}

// ConsecutiveErrors returns the number of consecutive errors.
func (h *ErrorHandler) ConsecutiveErrors() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.consecutiveErrors
}

// SetLastKnownGoodData stores the last successful data fetch.
func (h *ErrorHandler) SetLastKnownGoodData(runs []azdevops.PipelineRun) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Make a copy to avoid mutation
	h.lastKnownGoodData = make([]azdevops.PipelineRun, len(runs))
	copy(h.lastKnownGoodData, runs)
}

// GetLastKnownGoodData returns the last successful data fetch.
func (h *ErrorHandler) GetLastKnownGoodData() []azdevops.PipelineRun {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.lastKnownGoodData == nil {
		return nil
	}

	// Return a copy to avoid mutation
	result := make([]azdevops.PipelineRun, len(h.lastKnownGoodData))
	copy(result, h.lastKnownGoodData)
	return result
}

// ProcessUpdate processes a PipelineRunsUpdated message.
// On success, it stores the data and clears errors.
// On partial error (PartialError), it treats data as valid but stores a warning.
// On full error, it sets the error and returns last known good data.
// Returns the data to display and whether there was a full error.
func (h *ErrorHandler) ProcessUpdate(msg PipelineRunsUpdated) ([]azdevops.PipelineRun, bool) {
	if msg.Err != nil {
		// Check if this is a partial error (some projects succeeded)
		var partialErr *azdevops.PartialError
		if errors.As(msg.Err, &partialErr) {
			// Partial success: treat data as valid, set warning
			h.SetLastKnownGoodData(msg.Runs)
			h.ClearError()
			h.setPartialWarning(partialErr.Error())
			return msg.Runs, false
		}

		h.SetError(msg.Err)
		return h.GetLastKnownGoodData(), true
	}

	// Full success - store the data and clear errors and warnings
	h.SetLastKnownGoodData(msg.Runs)
	h.ClearError()
	h.clearPartialWarning()
	return msg.Runs, false
}

// PartialWarning returns the partial load warning message, if any.
func (h *ErrorHandler) PartialWarning() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.partialWarning
}

func (h *ErrorHandler) setPartialWarning(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.partialWarning = msg
}

func (h *ErrorHandler) clearPartialWarning() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.partialWarning = ""
}

// ErrorMessage returns the error message if there is an error.
func (h *ErrorHandler) ErrorMessage() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.currentError == nil {
		return ""
	}
	return h.currentError.Error()
}

// LastErrorTime returns the time of the last error.
func (h *ErrorHandler) LastErrorTime() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.lastErrorTime
}

// ShouldShowError returns true if the error should be displayed to the user.
func (h *ErrorHandler) ShouldShowError() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.currentError != nil
}

// IsRecoverable returns true if the error is likely recoverable (transient).
func (h *ErrorHandler) IsRecoverable() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.consecutiveErrors <= MaxRecoverableErrors
}

// RecoveryMessage returns a user-friendly message about the error state.
func (h *ErrorHandler) RecoveryMessage() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.currentError == nil {
		return ""
	}

	if h.consecutiveErrors <= MaxRecoverableErrors {
		return "Connection issue. Retrying..."
	}
	return "Connection failed. Check your network and press 'r' to retry."
}
