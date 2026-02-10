package polling

import (
	"errors"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

func TestErrorHandler_New(t *testing.T) {
	eh := NewErrorHandler()

	if eh == nil {
		t.Fatal("expected non-nil ErrorHandler")
	}
	if eh.HasError() {
		t.Error("new error handler should not have an error")
	}
	if eh.consecutiveErrors != 0 {
		t.Error("consecutive errors should be 0 initially")
	}
}

func TestErrorHandler_SetError(t *testing.T) {
	eh := NewErrorHandler()
	err := errors.New("connection failed")

	eh.SetError(err)

	if !eh.HasError() {
		t.Error("should have error after SetError")
	}
	if eh.GetError() != err {
		t.Error("GetError should return the set error")
	}
}

func TestErrorHandler_ClearError(t *testing.T) {
	eh := NewErrorHandler()
	eh.SetError(errors.New("some error"))

	eh.ClearError()

	if eh.HasError() {
		t.Error("should not have error after ClearError")
	}
	if eh.GetError() != nil {
		t.Error("GetError should return nil after ClearError")
	}
}

func TestErrorHandler_ConsecutiveErrors(t *testing.T) {
	eh := NewErrorHandler()

	// Add multiple errors
	eh.SetError(errors.New("error 1"))
	if eh.ConsecutiveErrors() != 1 {
		t.Errorf("expected 1 consecutive error, got %d", eh.ConsecutiveErrors())
	}

	eh.SetError(errors.New("error 2"))
	if eh.ConsecutiveErrors() != 2 {
		t.Errorf("expected 2 consecutive errors, got %d", eh.ConsecutiveErrors())
	}

	// Clear errors resets count
	eh.ClearError()
	if eh.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 consecutive errors after clear, got %d", eh.ConsecutiveErrors())
	}
}

func TestErrorHandler_LastKnownGoodData(t *testing.T) {
	eh := NewErrorHandler()

	// Initially no data
	if data := eh.GetLastKnownGoodData(); data != nil {
		t.Error("should have no data initially")
	}

	// Set good data
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1"},
		{ID: 2, BuildNumber: "2024.2"},
	}
	eh.SetLastKnownGoodData(runs)

	retrieved := eh.GetLastKnownGoodData()
	if len(retrieved) != 2 {
		t.Errorf("expected 2 runs, got %d", len(retrieved))
	}
}

func TestErrorHandler_ProcessUpdate_Success(t *testing.T) {
	eh := NewErrorHandler()
	runs := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1"},
	}

	msg := PipelineRunsUpdated{Runs: runs, Err: nil}
	result, hasError := eh.ProcessUpdate(msg)

	if hasError {
		t.Error("should not have error on success")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 run, got %d", len(result))
	}
	if eh.HasError() {
		t.Error("should not have stored error on success")
	}
}

func TestErrorHandler_ProcessUpdate_Error_ReturnsLastGood(t *testing.T) {
	eh := NewErrorHandler()

	// First, set some good data
	goodRuns := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1"},
	}
	eh.SetLastKnownGoodData(goodRuns)

	// Now process an error
	msg := PipelineRunsUpdated{Runs: nil, Err: errors.New("API error")}
	result, hasError := eh.ProcessUpdate(msg)

	if !hasError {
		t.Error("should have error")
	}
	// Should return last known good data
	if len(result) != 1 {
		t.Errorf("should return last known good data, got %d runs", len(result))
	}
	if eh.ConsecutiveErrors() != 1 {
		t.Errorf("should have 1 consecutive error, got %d", eh.ConsecutiveErrors())
	}
}

func TestErrorHandler_ProcessUpdate_Error_NoLastGood(t *testing.T) {
	eh := NewErrorHandler()

	msg := PipelineRunsUpdated{Runs: nil, Err: errors.New("API error")}
	result, hasError := eh.ProcessUpdate(msg)

	if !hasError {
		t.Error("should have error")
	}
	if result != nil {
		t.Error("should return nil when no last known good data")
	}
}

func TestErrorHandler_ErrorMessage(t *testing.T) {
	eh := NewErrorHandler()

	// No error
	if msg := eh.ErrorMessage(); msg != "" {
		t.Errorf("expected empty message when no error, got '%s'", msg)
	}

	// With error
	eh.SetError(errors.New("connection timeout"))
	msg := eh.ErrorMessage()
	if msg != "connection timeout" {
		t.Errorf("expected 'connection timeout', got '%s'", msg)
	}
}

func TestErrorHandler_LastErrorTime(t *testing.T) {
	eh := NewErrorHandler()

	// No error, should return zero time
	if !eh.LastErrorTime().IsZero() {
		t.Error("should return zero time when no error")
	}

	// Set error
	before := time.Now()
	eh.SetError(errors.New("error"))
	after := time.Now()

	errorTime := eh.LastErrorTime()
	if errorTime.Before(before) || errorTime.After(after) {
		t.Error("error time should be between before and after")
	}
}

func TestErrorHandler_ShouldShowError(t *testing.T) {
	eh := NewErrorHandler()

	// No error - don't show
	if eh.ShouldShowError() {
		t.Error("should not show error when no error")
	}

	// Single error - show
	eh.SetError(errors.New("error"))
	if !eh.ShouldShowError() {
		t.Error("should show error after first error")
	}
}

func TestErrorHandler_IsRecoverable(t *testing.T) {
	eh := NewErrorHandler()

	// First few errors are recoverable
	eh.SetError(errors.New("error 1"))
	if !eh.IsRecoverable() {
		t.Error("single error should be recoverable")
	}

	eh.SetError(errors.New("error 2"))
	eh.SetError(errors.New("error 3"))
	if !eh.IsRecoverable() {
		t.Error("3 errors should still be recoverable")
	}

	// Many errors may not be recoverable
	for i := 0; i < 10; i++ {
		eh.SetError(errors.New("more errors"))
	}
	// After MaxRecoverableErrors, it's not recoverable
	if eh.ConsecutiveErrors() > MaxRecoverableErrors && eh.IsRecoverable() {
		t.Error("too many errors should not be recoverable")
	}
}

func TestErrorHandler_RecoveryMessage(t *testing.T) {
	eh := NewErrorHandler()
	eh.SetError(errors.New("error"))

	msg := eh.RecoveryMessage()
	if msg == "" {
		t.Error("should have a recovery message when error exists")
	}
}
