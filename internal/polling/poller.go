package polling

import (
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	tea "github.com/charmbracelet/bubbletea"
)

// Constants for polling configuration
const (
	// DefaultInterval is the default polling interval
	DefaultInterval = 30 * time.Second
	// MinInterval is the minimum allowed polling interval
	MinInterval = 5 * time.Second
	// DefaultRunCount is the default number of pipeline runs to fetch
	DefaultRunCount = 30
)

// PipelineClient defines the interface for fetching pipeline data.
// This allows for easy testing with mock clients.
type PipelineClient interface {
	ListPipelineRuns(top int) ([]azdevops.PipelineRun, error)
}

// Poller manages background polling of Azure DevOps pipeline data.
type Poller struct {
	client   PipelineClient
	interval time.Duration
	runCount int
	stopped  bool
	mu       sync.RWMutex
}

// NewPoller creates a new Poller with the given client and interval.
// If interval is 0 or less than MinInterval, DefaultInterval or MinInterval is used.
func NewPoller(client PipelineClient, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = DefaultInterval
	} else if interval < MinInterval {
		interval = MinInterval
	}

	return &Poller{
		client:   client,
		interval: interval,
		runCount: DefaultRunCount,
		stopped:  false,
	}
}

// SetInterval updates the polling interval.
// If interval is less than MinInterval, MinInterval is used.
func (p *Poller) SetInterval(interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if interval < MinInterval {
		interval = MinInterval
	}
	p.interval = interval
}

// SetRunCount sets the number of pipeline runs to fetch.
func (p *Poller) SetRunCount(count int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if count < 1 {
		count = DefaultRunCount
	}
	p.runCount = count
}

// Stop stops the poller from making further API calls.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
}

// IsStopped returns true if the poller has been stopped.
func (p *Poller) IsStopped() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stopped
}

// FetchPipelineRuns returns a tea.Cmd that fetches pipeline runs from the API.
// Returns nil if the poller has been stopped.
func (p *Poller) FetchPipelineRuns() tea.Cmd {
	if p.IsStopped() {
		return nil
	}

	p.mu.RLock()
	runCount := p.runCount
	p.mu.RUnlock()

	return func() tea.Msg {
		runs, err := p.client.ListPipelineRuns(runCount)
		return PipelineRunsUpdated{
			Runs: runs,
			Err:  err,
		}
	}
}

// StartPolling returns a tea.Cmd that starts the polling timer.
// It will send a TickMsg after the configured interval.
func (p *Poller) StartPolling() tea.Cmd {
	if p.IsStopped() {
		return nil
	}

	p.mu.RLock()
	interval := p.interval
	p.mu.RUnlock()

	return tea.Every(interval, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// OnTick handles a tick event by fetching data and scheduling the next tick.
// Returns a batch command that fetches pipeline runs and schedules the next poll.
func (p *Poller) OnTick() tea.Cmd {
	if p.IsStopped() {
		return nil
	}

	return tea.Batch(
		p.FetchPipelineRuns(),
		p.StartPolling(),
	)
}
