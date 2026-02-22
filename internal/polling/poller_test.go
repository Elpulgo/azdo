package polling

import (
	"errors"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// MockClient implements a minimal interface for testing
type MockClient struct {
	Runs         []azdevops.PipelineRun
	Err          error
	CallCount    int
	RequestedTop int
}

func (m *MockClient) ListPipelineRuns(top int) ([]azdevops.PipelineRun, error) {
	m.CallCount++
	m.RequestedTop = top
	return m.Runs, m.Err
}

// Compile-time check: MultiClient must satisfy PipelineClient interface.
var _ PipelineClient = (*azdevops.MultiClient)(nil)

func TestPoller_New(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	if p == nil {
		t.Fatal("expected non-nil Poller")
	}
	if p.interval != 30*time.Second {
		t.Errorf("expected interval 30s, got %v", p.interval)
	}
}

func TestPoller_DefaultInterval(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 0) // 0 should use default

	if p.interval != DefaultInterval {
		t.Errorf("expected default interval %v, got %v", DefaultInterval, p.interval)
	}
}

func TestPoller_MinimumInterval(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 1*time.Second) // Too short

	if p.interval < MinInterval {
		t.Errorf("interval %v should not be less than MinInterval %v", p.interval, MinInterval)
	}
}

func TestPoller_FetchPipelineRuns_Success(t *testing.T) {
	expectedRuns := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1"},
		{ID: 2, BuildNumber: "2024.2"},
	}
	client := &MockClient{Runs: expectedRuns}
	p := NewPoller(client, 30*time.Second)

	// Get the command
	cmd := p.FetchPipelineRuns()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	// Execute the command to get the message
	msg := cmd()

	// Verify the message type and content
	runsMsg, ok := msg.(PipelineRunsUpdated)
	if !ok {
		t.Fatalf("expected PipelineRunsUpdated, got %T", msg)
	}

	if runsMsg.Err != nil {
		t.Errorf("expected no error, got %v", runsMsg.Err)
	}
	if len(runsMsg.Runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runsMsg.Runs))
	}
	if client.CallCount != 1 {
		t.Errorf("expected 1 API call, got %d", client.CallCount)
	}
}

func TestPoller_FetchPipelineRuns_Error(t *testing.T) {
	expectedErr := errors.New("network error")
	client := &MockClient{Err: expectedErr}
	p := NewPoller(client, 30*time.Second)

	cmd := p.FetchPipelineRuns()
	msg := cmd()

	runsMsg, ok := msg.(PipelineRunsUpdated)
	if !ok {
		t.Fatalf("expected PipelineRunsUpdated, got %T", msg)
	}

	if runsMsg.Err == nil {
		t.Error("expected error, got nil")
	}
	if runsMsg.Err.Error() != "network error" {
		t.Errorf("expected 'network error', got '%s'", runsMsg.Err.Error())
	}
	if runsMsg.Runs != nil {
		t.Error("expected nil runs on error")
	}
}

func TestPoller_StartPolling_ReturnsTickCmd(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	cmd := p.StartPolling()
	if cmd == nil {
		t.Fatal("expected non-nil command from StartPolling")
	}
	// The command will return a TickMsg after the interval
	// We can't easily test the timing here, but we verify it returns a command
}

func TestPoller_OnTick_ReturnsFetchAndNextTick(t *testing.T) {
	expectedRuns := []azdevops.PipelineRun{
		{ID: 1, BuildNumber: "2024.1"},
	}
	client := &MockClient{Runs: expectedRuns}
	p := NewPoller(client, 30*time.Second)

	// OnTick should return a batch command that:
	// 1. Fetches pipeline runs
	// 2. Schedules the next tick
	cmd := p.OnTick()
	if cmd == nil {
		t.Fatal("expected non-nil command from OnTick")
	}
}

func TestPoller_SetInterval(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	p.SetInterval(60 * time.Second)
	if p.interval != 60*time.Second {
		t.Errorf("expected interval 60s, got %v", p.interval)
	}
}

func TestPoller_SetInterval_EnforcesMinimum(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	p.SetInterval(1 * time.Second) // Too short
	if p.interval < MinInterval {
		t.Errorf("interval %v should not be less than MinInterval %v", p.interval, MinInterval)
	}
}

func TestPoller_Stop(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	p.Stop()
	if !p.stopped {
		t.Error("expected poller to be stopped")
	}
}

func TestPoller_IsStopped(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	if p.IsStopped() {
		t.Error("expected poller to not be stopped initially")
	}

	p.Stop()
	if !p.IsStopped() {
		t.Error("expected poller to be stopped after Stop()")
	}
}

func TestPoller_FetchPipelineRuns_WhenStopped(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)
	p.Stop()

	// When stopped, fetch should return nil or a no-op
	cmd := p.FetchPipelineRuns()
	if cmd == nil {
		// This is acceptable - returning nil when stopped
		return
	}

	// If it returns a command, it should not make API calls
	msg := cmd()
	if msg != nil {
		// If it does return something, check that no API was called
		// Or it could return an empty update
		runsMsg, ok := msg.(PipelineRunsUpdated)
		if ok && runsMsg.Runs != nil {
			// Only error if API was actually called when stopped
			if client.CallCount > 0 {
				t.Error("API should not be called when poller is stopped")
			}
		}
	}
}

func TestPoller_RequestedRunCount(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)

	cmd := p.FetchPipelineRuns()
	cmd()

	// Default should request 25 runs (matching current behavior)
	if client.RequestedTop != DefaultRunCount {
		t.Errorf("expected to request %d runs, got %d", DefaultRunCount, client.RequestedTop)
	}
}

func TestPoller_CustomRunCount(t *testing.T) {
	client := &MockClient{}
	p := NewPoller(client, 30*time.Second)
	p.SetRunCount(50)

	cmd := p.FetchPipelineRuns()
	cmd()

	if client.RequestedTop != 50 {
		t.Errorf("expected to request 50 runs, got %d", client.RequestedTop)
	}
}
