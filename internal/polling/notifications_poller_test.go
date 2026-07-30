package polling

import (
	"errors"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	tea "github.com/charmbracelet/bubbletea"
)

// MockNotificationsClient implements a minimal interface for testing.
type MockNotificationsClient struct {
	Items        []provider.Notification
	Err          error
	CallCount    int
	RequestedOpt provider.NotifOpts
}

func (m *MockNotificationsClient) List(opts provider.NotifOpts) ([]provider.Notification, error) {
	m.CallCount++
	m.RequestedOpt = opts
	return m.Items, m.Err
}

// Compile-time check: the composite provider must satisfy NotificationsClient.
var _ NotificationsClient = (*provider.CompositeProvider)(nil)

func TestNotificationsPoller_DefaultInterval(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 0, provider.NotifOpts{}) // 0 should use default

	if p.interval != DefaultInterval {
		t.Errorf("expected default interval %v, got %v", DefaultInterval, p.interval)
	}
}

func TestNotificationsPoller_MinimumInterval(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 1*time.Second, provider.NotifOpts{}) // Too short

	if p.interval < MinInterval {
		t.Errorf("interval %v should not be less than MinInterval %v", p.interval, MinInterval)
	}
}

// TestNotificationsPoller_MinimumInterval_FloorsPollIntervalOne pins the
// sixth FORWARD item: notifications.poll_interval: 1 is accepted by config
// validation, and on GitHub Enterprise (where the server may send no
// X-Poll-Interval) that would otherwise mean 1-second polling. The floor
// lives here, at the same MinInterval the pipeline Poller already enforces,
// rather than in config validation or app.go — mirroring exactly how a
// misconfigured pipeline PollingInterval is already floored.
func TestNotificationsPoller_MinimumInterval_FloorsPollIntervalOne(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 1*time.Second, provider.NotifOpts{})

	if p.interval != MinInterval {
		t.Errorf("interval = %v, want floored to MinInterval %v for a configured 1s interval", p.interval, MinInterval)
	}
}

func TestNotificationsPoller_FetchNotifications_Success(t *testing.T) {
	expected := []provider.Notification{
		{Identity: provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"}},
		{Identity: provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "2"}},
	}
	client := &MockNotificationsClient{Items: expected}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{Max: 50})

	cmd := p.FetchNotifications()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	msg := cmd()

	fetchMsg, ok := msg.(NotificationsFetchedMsg)
	if !ok {
		t.Fatalf("expected NotificationsFetchedMsg, got %T", msg)
	}
	if fetchMsg.Err != nil {
		t.Errorf("expected no error, got %v", fetchMsg.Err)
	}
	if len(fetchMsg.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(fetchMsg.Items))
	}
	if client.CallCount != 1 {
		t.Errorf("expected 1 API call, got %d", client.CallCount)
	}
	if client.RequestedOpt.Max != 50 {
		t.Errorf("expected opts.Max=50 forwarded to the client, got %d", client.RequestedOpt.Max)
	}
}

func TestNotificationsPoller_FetchNotifications_Error(t *testing.T) {
	expectedErr := errors.New("network error")
	client := &MockNotificationsClient{Err: expectedErr}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})

	cmd := p.FetchNotifications()
	msg := cmd()

	fetchMsg, ok := msg.(NotificationsFetchedMsg)
	if !ok {
		t.Fatalf("expected NotificationsFetchedMsg, got %T", msg)
	}
	if fetchMsg.Err == nil {
		t.Error("expected error, got nil")
	}
	if fetchMsg.Err.Error() != "network error" {
		t.Errorf("expected 'network error', got '%s'", fetchMsg.Err.Error())
	}
	if fetchMsg.Items != nil {
		t.Error("expected nil items on error")
	}
}

func TestNotificationsPoller_StartPolling_RespectsStopState(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})

	cmd := p.StartPolling()
	if cmd == nil {
		t.Error("StartPolling should return a command when running")
	}

	p.Stop()

	cmd = p.StartPolling()
	if cmd != nil {
		t.Error("StartPolling should return nil when stopped")
	}
}

// TestNotificationsPoller_StartPolling_EmitsNotificationsTickMsg is the
// mutation-guarding test for Decision 69: if NotificationsTickMsg were
// changed back to (or aliased as) polling.TickMsg, app.go's existing
// `case polling.TickMsg:` would silently swallow notifications ticks into
// the pipeline poller's OnTick, and this assertion is what would catch it —
// a plain "cmd != nil" check (as StartPolling_RespectsStopState above
// already does) would not.
func TestNotificationsPoller_StartPolling_EmitsNotificationsTickMsg(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})
	// Bypass SetInterval's MinInterval floor (white-box, same package) so the
	// timer fires almost immediately instead of making this test wait out a
	// real MinInterval — cmd() blocks on the timer channel (see
	// bubbletea.Every), it does not merely return the message type.
	p.interval = time.Millisecond

	cmd := p.StartPolling()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	msg := cmd()
	if _, ok := msg.(NotificationsTickMsg); !ok {
		t.Fatalf("StartPolling produced %T, want NotificationsTickMsg — reusing polling.TickMsg here would let app.go's pipeline-poller case swallow this tick (Decision 69)", msg)
	}
	if _, ok := msg.(TickMsg); ok {
		t.Fatal("StartPolling must never produce polling.TickMsg (Decision 69): app.go's `case polling.TickMsg:` drives the pipeline poller unconditionally")
	}
}

func TestNotificationsPoller_OnTick_RespectsStopState(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})

	cmd := p.OnTick()
	if cmd == nil {
		t.Error("OnTick should return a command when running")
	}

	p.Stop()

	cmd = p.OnTick()
	if cmd != nil {
		t.Error("OnTick should return nil when stopped")
	}
}

func TestNotificationsPoller_SetInterval_EnforcesMinimum(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})

	p.SetInterval(1 * time.Second) // Too short
	if p.interval < MinInterval {
		t.Errorf("interval %v should not be less than MinInterval %v", p.interval, MinInterval)
	}
}

func TestNotificationsPoller_IsStopped(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})

	if p.IsStopped() {
		t.Error("expected poller to not be stopped initially")
	}

	p.Stop()
	if !p.IsStopped() {
		t.Error("expected poller to be stopped after Stop()")
	}
}

func TestNotificationsPoller_FetchNotifications_WhenStopped(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{})
	p.Stop()

	cmd := p.FetchNotifications()
	if cmd != nil {
		t.Error("FetchNotifications should return nil when stopped")
	}
	if client.CallCount != 0 {
		t.Error("API should not be called when poller is stopped")
	}
}

func TestNotificationsPoller_SetOpts_ForwardsToNextFetch(t *testing.T) {
	client := &MockNotificationsClient{}
	p := NewNotificationsPoller(client, 30*time.Second, provider.NotifOpts{Max: 10})

	p.SetOpts(provider.NotifOpts{Max: 99, ParticipatingOnly: true})

	cmd := p.FetchNotifications()
	cmd()

	if client.RequestedOpt.Max != 99 {
		t.Errorf("expected opts.Max=99 forwarded after SetOpts, got %d", client.RequestedOpt.Max)
	}
	if !client.RequestedOpt.ParticipatingOnly {
		t.Error("expected opts.ParticipatingOnly=true forwarded after SetOpts")
	}
}

// TestNotificationsPoller_NilClient_* guards against the typed-nil boxed-interface
// panic that occurs when a provider has no notifications-capable backend. The
// poller must no-op (return nil / skip fetch) rather than panic when client == nil.

func TestNotificationsPoller_NilClient_FetchNotifications_ReturnsNil(t *testing.T) {
	var nc NotificationsClient // untyped nil — interface value is genuinely nil
	p := NewNotificationsPoller(nc, 30*time.Second, provider.NotifOpts{})

	cmd := p.FetchNotifications()
	if cmd != nil {
		t.Error("FetchNotifications should return nil when client is nil")
	}
}

func TestNotificationsPoller_NilClient_OnTick_ReturnsNil(t *testing.T) {
	var nc NotificationsClient
	p := NewNotificationsPoller(nc, 30*time.Second, provider.NotifOpts{})

	cmd := p.OnTick()
	if cmd != nil {
		t.Error("OnTick should return nil when client is nil")
	}
}

// TestNotificationsPoller_NilClient_StartPolling_ReturnsNil pins the review
// fix: a nil client means the provider has no notifications-capable backend
// at all (app.go now constructs the poller with a nil client whenever
// hasNotificationCapability(p) is false), and arming a timer that will only
// ever tick into a no-op OnTick is a permanent, pointless poll loop. Previous
// behaviour (arming the timer regardless of client) is intentionally
// reversed here.
func TestNotificationsPoller_NilClient_StartPolling_ReturnsNil(t *testing.T) {
	var nc NotificationsClient
	p := NewNotificationsPoller(nc, 30*time.Second, provider.NotifOpts{})

	cmd := p.StartPolling()
	if cmd != nil {
		t.Error("StartPolling should return nil when client is nil")
	}
}

// TestNotificationsPoller_StartPolling_UsesInjectedEverySeam is the
// mutation-guarding test for the interval-arithmetic mutant that survived
// review: `interval := p.interval` silently replaced by `interval :=
// DefaultInterval` inside StartPolling. Without an injectable seam, the
// interval tea.Every is actually called with is unobservable short of
// waiting out a real timer, so this substitutes a fast stand-in and asserts
// on the duration it was invoked with.
func TestNotificationsPoller_StartPolling_UsesInjectedEverySeam(t *testing.T) {
	client := &MockNotificationsClient{}
	// Deliberately distinct from DefaultInterval (30s): if the mutation this
	// test guards against (interval := p.interval silently replaced by
	// interval := DefaultInterval) were present, using 30s here would not
	// distinguish the two and the mutation would survive undetected.
	const configured = 90 * time.Second
	p := NewNotificationsPoller(client, configured, provider.NotifOpts{})

	var gotDuration time.Duration
	p.SetEveryForTesting(func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		gotDuration = d
		return func() tea.Msg { return fn(time.Now()) }
	})

	cmd := p.StartPolling()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	cmd()

	if gotDuration != configured {
		t.Errorf("StartPolling armed every() with %v, want the poller's configured interval %v (not DefaultInterval)", gotDuration, configured)
	}
}
