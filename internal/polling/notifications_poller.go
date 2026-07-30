package polling

import (
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	tea "github.com/charmbracelet/bubbletea"
)

// NotificationsClient defines the interface for fetching a notification
// inbox. It mirrors provider.NotificationSource's List method only (not
// MarkRead/MarkDone), the same narrowing PipelineClient applies to
// provider.Provider's list method, so mock clients in tests stay minimal.
type NotificationsClient interface {
	List(opts provider.NotifOpts) ([]provider.Notification, error)
}

// NotificationsPoller manages background polling of the notifications inbox.
// It is a distinct type from Poller (Decision 69), not a generalisation of
// it: app.go's single `case polling.TickMsg:` drives the pipeline Poller's
// OnTick unconditionally, so this poller emits its own NotificationsTickMsg
// rather than reusing TickMsg, and app.go dispatches it through its own
// top-level case.
//
// The interval this poller uses is driven entirely by its caller (app.go)
// via SetInterval — mirroring Poller, which has no notion of a
// server-suggested cadence hint either. Decision 8's max(configured, hint)
// computation lives in app.go, not here, so this type has no dependency on
// provider.PollIntervalHinter.
type NotificationsPoller struct {
	client   NotificationsClient
	interval time.Duration
	opts     provider.NotifOpts
	stopped  bool
	mu       sync.RWMutex
}

// NewNotificationsPoller creates a new NotificationsPoller with the given
// client, interval and fetch options. If interval is 0 or less than
// MinInterval, DefaultInterval or MinInterval is used — the same floor
// Poller applies, which is also how a misconfigured
// notifications.poll_interval: 1 (accepted by config validation) is kept
// from producing sub-MinInterval polling.
func NewNotificationsPoller(client NotificationsClient, interval time.Duration, opts provider.NotifOpts) *NotificationsPoller {
	if interval <= 0 {
		interval = DefaultInterval
	} else if interval < MinInterval {
		interval = MinInterval
	}

	return &NotificationsPoller{
		client:   client,
		interval: interval,
		opts:     opts,
		stopped:  false,
	}
}

// SetInterval updates the polling interval.
// If interval is less than MinInterval, MinInterval is used.
func (p *NotificationsPoller) SetInterval(interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if interval < MinInterval {
		interval = MinInterval
	}
	p.interval = interval
}

// Interval reports the interval the next poll will be scheduled at, after the
// MinInterval floor has been applied.
//
// It exists as a test seam. Applying GitHub's X-Poll-Interval hint is a single
// SetInterval call in app's NotificationsFetchedMsg handler, and that call is
// the whole of task 15's "cadence is max(X-Poll-Interval, configured)"
// criterion — delete it and the configured interval is used forever while the
// hint is computed and discarded. With no way to observe the poller's interval
// there is nothing to assert against, and the deletion is invisible: the pure
// interval-arithmetic functions still pass, because they are not what broke.
//
// Poller deliberately does not gain the same method. It has no equivalent
// hint-driven mutation to pin, and adding an accessor to it on grounds of
// symmetry alone would widen this task's blast radius into three other panes
// for no test.
func (p *NotificationsPoller) Interval() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.interval
}

// SetOpts updates the fetch options used on the next poll.
func (p *NotificationsPoller) SetOpts(opts provider.NotifOpts) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opts = opts
}

// Stop stops the poller from making further API calls.
func (p *NotificationsPoller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
}

// IsStopped returns true if the poller has been stopped.
func (p *NotificationsPoller) IsStopped() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stopped
}

// FetchNotifications returns a tea.Cmd that fetches the notification inbox.
// Returns nil if the poller has been stopped or if no client is configured
// (a defensive guard; callers always pass a capable composite).
func (p *NotificationsPoller) FetchNotifications() tea.Cmd {
	if p.client == nil {
		return nil
	}
	if p.IsStopped() {
		return nil
	}

	p.mu.RLock()
	opts := p.opts
	p.mu.RUnlock()

	return func() tea.Msg {
		items, err := p.client.List(opts)
		return NotificationsFetchedMsg{
			Items: items,
			Err:   err,
		}
	}
}

// StartPolling returns a tea.Cmd that starts the polling timer.
// It will send a NotificationsTickMsg after the configured interval.
func (p *NotificationsPoller) StartPolling() tea.Cmd {
	if p.IsStopped() {
		return nil
	}

	p.mu.RLock()
	interval := p.interval
	p.mu.RUnlock()

	return tea.Every(interval, func(t time.Time) tea.Msg {
		return NotificationsTickMsg{}
	})
}

// OnTick handles a tick event by fetching data and scheduling the next tick.
// Returns a batch command that fetches notifications and schedules the next
// poll. Returns nil when no client is configured (a defensive guard;
// callers always pass a capable composite).
func (p *NotificationsPoller) OnTick() tea.Cmd {
	if p.client == nil {
		return nil
	}
	if p.IsStopped() {
		return nil
	}

	return tea.Batch(
		p.FetchNotifications(),
		p.StartPolling(),
	)
}
