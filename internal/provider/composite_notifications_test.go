package provider_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// ---------------------------------------------------------------------------
// Notification-capable stub backends.
//
// fakeNotifyBackend embeds *fakeBackend (composite_test.go) to reuse its
// Provider implementation, and additionally implements NotificationSource so
// fan-out tests can exercise the capability-filtered path against a mix of
// capable and incapable backends.
//
// partialNotifyBackend embeds *fakeBackend and defines List with the exact
// NotificationSource.List signature but deliberately omits MarkRead and
// MarkDone, so it does NOT satisfy provider.NotificationSource as a whole.
// It exists to catch a capability-filter bug that checks for List alone (a
// narrower ad hoc interface) instead of asserting the full three-method
// NotificationSource: its call counter proves List was never invoked
// despite existing on the type.
// ---------------------------------------------------------------------------

type fakeNotifyBackend struct {
	*fakeBackend

	notifs      []provider.Notification
	listErr     error
	markReadErr error
	markDoneErr error

	listCalls     int
	markReadCalls []provider.Identity
	markDoneCalls []provider.Identity
}

func newFakeNotifyBackend(kind provider.Kind, scopes []string) *fakeNotifyBackend {
	return &fakeNotifyBackend{fakeBackend: &fakeBackend{kind: kind, scopes: scopes}}
}

func (f *fakeNotifyBackend) List(_ provider.NotifOpts) ([]provider.Notification, error) {
	f.listCalls++
	return f.notifs, f.listErr
}

func (f *fakeNotifyBackend) MarkRead(id provider.Identity) error {
	f.markReadCalls = append(f.markReadCalls, id)
	return f.markReadErr
}

func (f *fakeNotifyBackend) MarkDone(id provider.Identity) error {
	f.markDoneCalls = append(f.markDoneCalls, id)
	return f.markDoneErr
}

// compile-time assertions: fakeNotifyBackend satisfies both Provider and
// NotificationSource.
var _ provider.Provider = (*fakeNotifyBackend)(nil)
var _ provider.NotificationSource = (*fakeNotifyBackend)(nil)

type partialNotifyBackend struct {
	*fakeBackend
	listCalls int
}

func (f *partialNotifyBackend) List(_ provider.NotifOpts) ([]provider.Notification, error) {
	f.listCalls++
	return nil, nil
}

// compile-time assertion: partialNotifyBackend still satisfies Provider.
var _ provider.Provider = (*partialNotifyBackend)(nil)

// TestPartialNotifyBackend_DoesNotSatisfyNotificationSource pins the
// fixture's own shape: it must NOT satisfy NotificationSource (it lacks
// MarkRead/MarkDone), otherwise the "skips incapable backends entirely"
// test below would not actually exercise the capability filter.
func TestPartialNotifyBackend_DoesNotSatisfyNotificationSource(t *testing.T) {
	var p provider.Provider = &partialNotifyBackend{fakeBackend: &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}}
	if _, ok := p.(provider.NotificationSource); ok {
		t.Fatal("partialNotifyBackend must not satisfy NotificationSource (it deliberately lacks MarkRead/MarkDone)")
	}
}

// hintingNotifyBackend embeds *fakeNotifyBackend and additionally implements
// PollIntervalHinter, so NotificationsPollInterval tests can exercise a mix
// of hinting and non-hinting capable backends. fakeNotifyBackend itself
// deliberately does NOT implement PollIntervalHinter (a capable backend need
// not hint), which is what proves the "falls back" half of the contract below.
type hintingNotifyBackend struct {
	*fakeNotifyBackend
	interval time.Duration
}

func newHintingNotifyBackend(kind provider.Kind, scopes []string, interval time.Duration) *hintingNotifyBackend {
	return &hintingNotifyBackend{fakeNotifyBackend: newFakeNotifyBackend(kind, scopes), interval: interval}
}

func (f *hintingNotifyBackend) PollInterval() time.Duration {
	return f.interval
}

// compile-time assertions: hintingNotifyBackend satisfies both
// NotificationSource and PollIntervalHinter.
var _ provider.NotificationSource = (*hintingNotifyBackend)(nil)
var _ provider.PollIntervalHinter = (*hintingNotifyBackend)(nil)

// TestFakeNotifyBackend_DoesNotImplementPollIntervalHinter pins the fixture's
// own shape: it must NOT satisfy PollIntervalHinter, otherwise the "falls
// back to 0 when no capable backend hints" test below would not actually
// exercise the fallback path.
func TestFakeNotifyBackend_DoesNotImplementPollIntervalHinter(t *testing.T) {
	var src provider.NotificationSource = newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	if _, ok := src.(provider.PollIntervalHinter); ok {
		t.Fatal("fakeNotifyBackend must not implement PollIntervalHinter — see hintingNotifyBackend")
	}
}

// stubBackendError is a distinguishable error type used to prove errors.As
// can recover a specific error through both the all-failed (errors.Join)
// and partial (PartialError.Unwrap) paths.
type stubBackendError struct{ msg string }

func (e *stubBackendError) Error() string { return e.msg }

func mkNotif(kind provider.Kind, scope, id string, ts time.Time) provider.Notification {
	return provider.Notification{
		Identity:  provider.Identity{Kind: kind, Scope: scope, ID: id},
		UpdatedAt: ts,
	}
}

// ---------------------------------------------------------------------------
// HasNotifications
// ---------------------------------------------------------------------------

// TestCompositeProvider_HasNotifications covers the whole capability matrix:
// no capable backend, one capable backend, and — the row that matters, since
// it is the only one that fails if the scan gives up at the first incapable
// backend instead of continuing — an incapable backend registered ahead of a
// capable one.
func TestCompositeProvider_HasNotifications(t *testing.T) {
	tests := []struct {
		name     string
		backends func() []provider.Provider
		want     bool
	}{
		{
			name: "zero capable backends",
			backends: func() []provider.Provider {
				return []provider.Provider{&fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}}
			},
			want: false,
		},
		{
			name: "one capable backend",
			backends: func() []provider.Provider {
				return []provider.Provider{newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})}
			},
			want: true,
		},
		{
			name: "incapable backend registered before a capable one",
			backends: func() []provider.Provider {
				return []provider.Provider{
					&fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}},
					newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"}),
				}
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := provider.NewCompositeProvider(tt.backends()...)
			if got := cp.HasNotifications(); got != tt.want {
				t.Fatalf("HasNotifications() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NotificationsPollInterval
// ---------------------------------------------------------------------------

// TestCompositeProvider_NotificationsPollInterval_ZeroWhenNoCapableBackends
// pins the "no hint available" floor: a composite with zero capable
// backends must report 0, not panic or reach for a nonexistent hinter.
func TestCompositeProvider_NotificationsPollInterval_ZeroWhenNoCapableBackends(t *testing.T) {
	a := &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}
	cp := provider.NewCompositeProvider(a)

	if got := cp.NotificationsPollInterval(); got != 0 {
		t.Fatalf("NotificationsPollInterval() = %v, want 0 with zero capable backends", got)
	}
}

// TestCompositeProvider_NotificationsPollInterval_FallsBackToZero_WhenCapableButNotHinting
// pins the fallback: a capable backend that does not implement
// PollIntervalHinter must not be mistaken for one hinting 0 — the caller's
// poller must fall back to the configured interval, and this fixture proves
// the composite does not panic or misbehave when the only capable backend
// lacks the hinter.
func TestCompositeProvider_NotificationsPollInterval_FallsBackToZero_WhenCapableButNotHinting(t *testing.T) {
	nonHinting := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(nonHinting)

	if got := cp.NotificationsPollInterval(); got != 0 {
		t.Fatalf("NotificationsPollInterval() = %v, want 0 when the sole capable backend does not implement PollIntervalHinter", got)
	}
}

// TestCompositeProvider_NotificationsPollInterval_ReturnsHint_WhenOneBackendHints
// pins the positive path: a single capable, hinting backend's value is
// returned verbatim.
func TestCompositeProvider_NotificationsPollInterval_ReturnsHint_WhenOneBackendHints(t *testing.T) {
	hinting := newHintingNotifyBackend(provider.KindGitHub, []string{"o/r"}, 90*time.Second)
	cp := provider.NewCompositeProvider(hinting)

	if got, want := cp.NotificationsPollInterval(), 90*time.Second; got != want {
		t.Fatalf("NotificationsPollInterval() = %v, want %v", got, want)
	}
}

// TestCompositeProvider_NotificationsPollInterval_MaxAcrossHintingBackends
// pins the aggregation rule: the composite reports the LARGEST hint among
// hinting backends, never the first or an average — the poller treats the
// hint as a floor to raise the configured interval to, so under-reporting it
// would let one backend's rate limit get exceeded.
func TestCompositeProvider_NotificationsPollInterval_MaxAcrossHintingBackends(t *testing.T) {
	small := newHintingNotifyBackend(provider.KindGitHub, []string{"o/r1"}, 30*time.Second)
	large := newHintingNotifyBackend(provider.KindGitHub, []string{"o/r2"}, 120*time.Second)
	cp := provider.NewCompositeProvider(small, large)

	if got, want := cp.NotificationsPollInterval(), 120*time.Second; got != want {
		t.Fatalf("NotificationsPollInterval() = %v, want max hint %v", got, want)
	}
}

// TestCompositeProvider_NotificationsPollInterval_IgnoresNonHintingCapableBackend
// pins that a capable-but-non-hinting backend does not drag the aggregate
// toward 0 when mixed with a hinting one — the non-hinting backend simply
// does not participate, it does not count as "hints 0".
func TestCompositeProvider_NotificationsPollInterval_IgnoresNonHintingCapableBackend(t *testing.T) {
	nonHinting := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r1"})
	hinting := newHintingNotifyBackend(provider.KindGitHub, []string{"o/r2"}, 45*time.Second)
	cp := provider.NewCompositeProvider(nonHinting, hinting)

	if got, want := cp.NotificationsPollInterval(), 45*time.Second; got != want {
		t.Fatalf("NotificationsPollInterval() = %v, want %v (non-hinting backend must not drag the max toward 0)", got, want)
	}
}

// ---------------------------------------------------------------------------
// Fan-out skips incapable backends
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_SkipsIncapableBackendEntirely verifies
// that List fans out only to capable backends, using a call counter (not
// merely absent rows) to prove the incapable backend's List was never
// invoked — even though partialNotifyBackend defines a method named List
// with the matching signature.
func TestCompositeProvider_Notifications_SkipsIncapableBackendEntirely(t *testing.T) {
	incapable := &partialNotifyBackend{fakeBackend: &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}}
	capable := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	capable.notifs = []provider.Notification{mkNotif(provider.KindGitHub, "o/r", "1", t1)}

	cp := provider.NewCompositeProvider(incapable, capable)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 notification from the capable backend, got %d", len(got))
	}
	if incapable.listCalls != 0 {
		t.Fatalf("want the incapable backend's List never called, got %d calls", incapable.listCalls)
	}
}

// ---------------------------------------------------------------------------
// Merge / sort
// ---------------------------------------------------------------------------

// gotIDs returns the Identity.IDs of a merged feed in order, so ordering
// assertions can name the exact expected sequence.
func gotIDs(notifs []provider.Notification) []string {
	ids := make([]string, len(notifs))
	for i, n := range notifs {
		ids[i] = n.Identity.ID
	}
	return ids
}

func joinIDs(ids []string) string { return strings.Join(ids, ",") }

// TestCompositeProvider_Notifications_MergeSortedNewestFirst interleaves rows
// across two backends, including a tie, so the sort is genuinely exercised
// rather than validated against already-ordered input.
//
// The merged order is a total order, so every position is pinned exactly —
// including the two tied t3 rows, which the Identity.Kind tiebreaker orders
// Azure (1) before GitHub (2) no matter which goroutine drains first.
// Asserting the tie as an unordered set would leave that guarantee unpinned.
func TestCompositeProvider_Notifications_MergeSortedNewestFirst(t *testing.T) {
	a := newFakeNotifyBackend(provider.KindGitHub, []string{"acme/repo-a"})
	a.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "acme/repo-a", "1", t3),
		mkNotif(provider.KindGitHub, "acme/repo-a", "2", t1),
	}
	b := newFakeNotifyBackend(provider.KindAzure, []string{"acme/repo-b"})
	b.notifs = []provider.Notification{
		mkNotif(provider.KindAzure, "acme/repo-b", "3", t2),
		mkNotif(provider.KindAzure, "acme/repo-b", "4", t3), // ties backend a's row 1
	}

	cp := provider.NewCompositeProvider(a, b)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 notifications, got %d", len(got))
	}
	// "4" (Azure, t3) before "1" (GitHub, t3) by the Kind tiebreaker, then
	// "3" (t2), then "2" (t1).
	want := "4,1,3,2"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want merged order %q, got %q", want, joinIDs(gotIDs(got)))
	}
	wantTimes := []time.Time{t3, t3, t2, t1}
	for i, ts := range wantTimes {
		if !got[i].UpdatedAt.Equal(ts) {
			t.Errorf("position %d: want UpdatedAt %v, got %v", i, ts, got[i].UpdatedAt)
		}
	}
}

// ---------------------------------------------------------------------------
// Deterministic tie order (total order, not merely stable)
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_TieBrokenByKind pins the second level of
// the total order. Both rows share an UpdatedAt, and their Scopes are chosen
// so scope order ("acme/aaa" first) disagrees with kind order (Azure first):
// dropping the Kind comparison flips these two rows.
func TestCompositeProvider_Notifications_TieBrokenByKind(t *testing.T) {
	gh := newFakeNotifyBackend(provider.KindGitHub, []string{"acme/aaa"})
	gh.notifs = []provider.Notification{mkNotif(provider.KindGitHub, "acme/aaa", "gh", t2)}
	az := newFakeNotifyBackend(provider.KindAzure, []string{"acme/zzz"})
	az.notifs = []provider.Notification{mkNotif(provider.KindAzure, "acme/zzz", "az", t2)}

	cp := provider.NewCompositeProvider(gh, az)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "az,gh"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want tied rows ordered by Identity.Kind (%q), got %q", want, joinIDs(gotIDs(got)))
	}
}

// TestCompositeProvider_Notifications_TieBrokenByScope pins the third level.
// One backend supplies both rows so the input order is fixed rather than
// goroutine-completion dependent. UpdatedAt and Kind tie, and the IDs are
// chosen so ID order ("1" first) disagrees with scope order ("acme/aaa"
// first): dropping the Scope comparison flips these two rows.
func TestCompositeProvider_Notifications_TieBrokenByScope(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"acme/aaa", "acme/bbb"})
	b.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "acme/bbb", "1", t2),
		mkNotif(provider.KindGitHub, "acme/aaa", "2", t2),
	}

	cp := provider.NewCompositeProvider(b)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2,1"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want tied rows ordered by Identity.Scope (%q), got %q", want, joinIDs(gotIDs(got)))
	}
}

// TestCompositeProvider_Notifications_TieBrokenByID pins the fourth and last
// level. UpdatedAt, Kind and Scope all tie, and the rows are fed in
// descending ID order, so without the ID comparison the stable sort would
// return them exactly as supplied ("2","1").
func TestCompositeProvider_Notifications_TieBrokenByID(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"acme/aaa"})
	b.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "acme/aaa", "2", t2),
		mkNotif(provider.KindGitHub, "acme/aaa", "1", t2),
	}

	cp := provider.NewCompositeProvider(b)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "1,2"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want tied rows ordered by Identity.ID (%q), got %q", want, joinIDs(gotIDs(got)))
	}
}

// TestCompositeProvider_Notifications_IdenticalKeysKeepInputOrder pins the
// residual guarantee of sort.SliceStable over sort.Slice. Once the comparator
// is a total order over (UpdatedAt, Kind, Scope, ID), stability is observable
// only for rows the comparator cannot distinguish at all — a duplicate thread
// from an overlapping page, identical in all four keys. Measured: Go's pdqsort
// only reorders such rows once there are several multi-member equivalence
// classes, so this fixture uses 10 duplicate pairs.
func TestCompositeProvider_Notifications_IdenticalKeysKeepInputOrder(t *testing.T) {
	const pairs = 10
	var rows []provider.Notification
	var want []string
	// Ascending timestamps, so the sort must genuinely reverse the input.
	for i := 0; i < pairs; i++ {
		ts := t1.Add(time.Duration(i) * time.Minute)
		id := fmt.Sprintf("dup-%d", i)
		first := mkNotif(provider.KindGitHub, "acme/aaa", id, ts)
		first.Title = fmt.Sprintf("%s-first", id)
		second := mkNotif(provider.KindGitHub, "acme/aaa", id, ts)
		second.Title = fmt.Sprintf("%s-second", id)
		rows = append(rows, first, second)
	}
	// Newest pair first in the expected output, each pair still first-then-second.
	for i := pairs - 1; i >= 0; i-- {
		want = append(want, fmt.Sprintf("dup-%d-first", i), fmt.Sprintf("dup-%d-second", i))
	}

	b := newFakeNotifyBackend(provider.KindGitHub, []string{"acme/aaa"})
	b.notifs = rows

	cp := provider.NewCompositeProvider(b)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	titles := make([]string, len(got))
	for i, n := range got {
		titles[i] = n.Title
	}
	if strings.Join(titles, ",") != strings.Join(want, ",") {
		t.Fatalf("want rows identical in all four sort keys to keep their input order\nwant %v\ngot  %v", want, titles)
	}
}

// TestCompositeProvider_Notifications_EmptyResultIsNotAnError collects the
// shapes that legitimately produce an empty feed with a nil error, since both
// rows make the same two assertions:
//
//   - nil vs empty slice: a capable backend returning (nil, nil) and one
//     returning an empty non-nil slice merge to the same observable result;
//   - zero capable backends: the separate total == 0 guard, which must not
//     take the all-failed branch — len(errs) == total is satisfied vacuously
//     at 0 == 0.
func TestCompositeProvider_Notifications_EmptyResultIsNotAnError(t *testing.T) {
	tests := []struct {
		name     string
		backends func() []provider.Provider
	}{
		{
			name: "capable backends returning nil and empty slices",
			backends: func() []provider.Provider {
				a := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r1"})
				a.notifs = nil
				b := newFakeNotifyBackend(provider.KindAzure, []string{"o/r2"})
				b.notifs = []provider.Notification{}
				return []provider.Provider{a, b}
			},
		},
		{
			name: "zero capable backends",
			backends: func() []provider.Provider {
				return []provider.Provider{&fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := provider.NewCompositeProvider(tt.backends()...)

			got, err := cp.List(provider.NotifOpts{})
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			if len(got) != 0 {
				t.Fatalf("List() returned %d items, want an empty result", len(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Partial failure never empties the feed; total is the capable count.
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_PartialFailure verifies that one backend
// erroring keeps the other backend's rows and surfaces a *PartialError
// alongside them.
func TestCompositeProvider_Notifications_PartialFailure(t *testing.T) {
	healthy := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	healthy.notifs = []provider.Notification{mkNotif(provider.KindGitHub, "o/r", "1", t1)}
	broken := newFakeNotifyBackend(provider.KindAzure, []string{"P"})
	broken.listErr = errors.New("backend down")

	cp := provider.NewCompositeProvider(healthy, broken)

	got, err := cp.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe *provider.PartialError
	if !errors.As(err, &pe) {
		t.Fatalf("want *PartialError, got %T: %v", err, err)
	}
	if pe.Failed != 1 || pe.Total != 2 {
		t.Errorf("want Failed=1 Total=2, got Failed=%d Total=%d", pe.Failed, pe.Total)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 healthy notification returned, got %d", len(got))
	}
	if got[0].Identity.ID != "1" {
		t.Errorf("want the healthy backend's row, got %+v", got[0])
	}
}

// TestCompositeProvider_Notifications_TotalIsCapableCountNotAllBackends
// pins the scenario: one incapable Azure backend plus one capable GitHub
// backend that fails. total must be 1 (the capable count), not 2
// (len(cp.backends)) — otherwise len(errs)==1 != total==2 takes the partial
// branch and returns (nil, &PartialError{1,2}), an empty feed carrying a
// buried error.
//
// This is also the only test covering the all-failed path with exactly one
// capable backend, so len(errs) == 1 — and it therefore also pins the error
// chain through a *single-error* errors.Join. errors.Join wraps even one error
// in a *joinError, so the outer %w reaches it via Unwrap() []error; a %v there
// would leave the suite green (the error is still non-nil) while silently
// breaking scope-error recovery. The two-capable-backend chain is pinned
// separately below.
func TestCompositeProvider_Notifications_TotalIsCapableCountNotAllBackends(t *testing.T) {
	incapable := &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}
	stub := &stubBackendError{msg: "403 missing scope"}
	broken := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	broken.listErr = stub

	cp := provider.NewCompositeProvider(incapable, broken)

	got, err := cp.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("want an error when the only capable backend fails")
	}
	var pe *provider.PartialError
	if errors.As(err, &pe) {
		t.Fatalf("want a plain all-failed error (total=1 capable backend), got *PartialError with Total=%d — total must count capable backends only", pe.Total)
	}
	if got != nil {
		t.Errorf("want nil results on all-failed (1 of 1 capable backends), got %v", got)
	}

	var target *stubBackendError
	if !errors.As(err, &target) {
		t.Fatalf("want errors.As to recover *stubBackendError through the single-error errors.Join chain, got %v", err)
	}
	if target.msg != stub.msg {
		t.Errorf("want recovered error msg %q, got %q", stub.msg, target.msg)
	}
	if !errors.Is(err, stub) {
		t.Fatalf("want errors.Is to find the exact backend error through the single-error errors.Join chain, got %v", err)
	}
}

// TestCompositeProvider_Notifications_BackendRowsWithErrorAreDiscarded pins
// the discard contract: a backend that returns rows *and* an error
// contributes only its error, never its rows. Two capable backends are needed
// to observe it — with one, the all-failed path returns nil regardless. No
// shipped implementation reaches this (github.Adapter returns (nil, err) on
// every error path), but the choice is deliberate and otherwise untested:
// dropping the `continue` after errs = append(...) is invisible to every
// other fixture, because they all leave notifs nil whenever listErr is set.
func TestCompositeProvider_Notifications_BackendRowsWithErrorAreDiscarded(t *testing.T) {
	healthy := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	healthy.notifs = []provider.Notification{mkNotif(provider.KindGitHub, "o/r", "kept", t1)}
	broken := newFakeNotifyBackend(provider.KindAzure, []string{"P"})
	broken.notifs = []provider.Notification{mkNotif(provider.KindAzure, "P", "discarded", t3)}
	broken.listErr = errors.New("partial page then failed")

	cp := provider.NewCompositeProvider(healthy, broken)

	got, err := cp.List(provider.NotifOpts{})
	var pe *provider.PartialError
	if !errors.As(err, &pe) {
		t.Fatalf("want *PartialError, got %T: %v", err, err)
	}
	if len(got) != 1 {
		t.Fatalf("want only the healthy backend's row, got %d rows: %v", len(got), gotIDs(got))
	}
	if got[0].Identity.ID != "kept" {
		t.Fatalf("want the failing backend's rows discarded, got %v", gotIDs(got))
	}
}

// ---------------------------------------------------------------------------
// NotifOpts.Max is re-applied after the merge
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_MaxTruncatesToNewest verifies Max caps
// the merged feed and does so *after* the sort. The single backend supplies
// its rows oldest-first, so an implementation that truncates before sorting
// keeps {t1,t2} and returns "old,mid" — the exact rows the user does not want.
func TestCompositeProvider_Notifications_MaxTruncatesToNewest(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	b.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "o/r", "old", t1),
		mkNotif(provider.KindGitHub, "o/r", "mid", t2),
		mkNotif(provider.KindGitHub, "o/r", "new", t3),
	}

	cp := provider.NewCompositeProvider(b)

	got, err := cp.List(provider.NotifOpts{Max: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "new,mid"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want the newest %d rows (%q), got %q", 2, want, joinIDs(gotIDs(got)))
	}
	// Full slice expression: what Max removed must be unrecoverable, not
	// merely out of view (same reasoning as the adapter's own Max cap).
	if cap(got) != len(got) {
		t.Errorf("want cap == len on a truncated result so the dropped rows cannot be recovered via got[:cap(got)], got cap=%d len=%d", cap(got), len(got))
	}
}

// TestCompositeProvider_Notifications_MaxLargerThanFeedIsNoOp verifies a Max
// above the merged length neither truncates nor reorders.
func TestCompositeProvider_Notifications_MaxLargerThanFeedIsNoOp(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	b.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "o/r", "old", t1),
		mkNotif(provider.KindGitHub, "o/r", "new", t3),
	}

	cp := provider.NewCompositeProvider(b)

	got, err := cp.List(provider.NotifOpts{Max: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "new,old"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want all rows unchanged (%q), got %q", want, joinIDs(gotIDs(got)))
	}
}

// TestCompositeProvider_Notifications_MaxZeroIsUncapped verifies Max's zero
// value means "no cap" (NotifOpts's documented zero-value contract), so the
// guard must be Max > 0 and never Max >= 0.
func TestCompositeProvider_Notifications_MaxZeroIsUncapped(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	b.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "o/r", "old", t1),
		mkNotif(provider.KindGitHub, "o/r", "mid", t2),
		mkNotif(provider.KindGitHub, "o/r", "new", t3),
	}

	cp := provider.NewCompositeProvider(b)

	got, err := cp.List(provider.NotifOpts{Max: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want Max: 0 to leave the feed uncapped (3 rows), got %d: %v", len(got), gotIDs(got))
	}
}

// TestCompositeProvider_Notifications_MaxAppliedOnPartialPath verifies Max is
// re-applied on the partial-error return too, not only the clean one — the
// partial path is where a truncated cap matters most, since the feed the user
// sees is already incomplete.
func TestCompositeProvider_Notifications_MaxAppliedOnPartialPath(t *testing.T) {
	healthy := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	healthy.notifs = []provider.Notification{
		mkNotif(provider.KindGitHub, "o/r", "old", t1),
		mkNotif(provider.KindGitHub, "o/r", "mid", t2),
		mkNotif(provider.KindGitHub, "o/r", "new", t3),
	}
	broken := newFakeNotifyBackend(provider.KindAzure, []string{"P"})
	broken.listErr = errors.New("backend down")

	cp := provider.NewCompositeProvider(healthy, broken)

	got, err := cp.List(provider.NotifOpts{Max: 2})
	var pe *provider.PartialError
	if !errors.As(err, &pe) {
		t.Fatalf("want *PartialError, got %T: %v", err, err)
	}
	want := "new,mid"
	if joinIDs(gotIDs(got)) != want {
		t.Fatalf("want Max applied on the partial-error path too (%q), got %q", want, joinIDs(gotIDs(got)))
	}
	if cap(got) != len(got) {
		t.Errorf("want cap == len on the truncated partial result, got cap=%d len=%d", cap(got), len(got))
	}
}

// ---------------------------------------------------------------------------
// Error chain preservation
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_AllFail_ErrorsAsRecovery verifies that
// when every capable backend fails, errors.As can still recover a specific
// underlying error type through the errors.Join chain.
func TestCompositeProvider_Notifications_AllFail_ErrorsAsRecovery(t *testing.T) {
	a := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	a.listErr = &stubBackendError{msg: "err-a"}
	b := newFakeNotifyBackend(provider.KindAzure, []string{"P"})
	b.listErr = &stubBackendError{msg: "err-b"}

	cp := provider.NewCompositeProvider(a, b)

	got, err := cp.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe *provider.PartialError
	if errors.As(err, &pe) {
		t.Fatal("want a plain all-failed error, not *PartialError")
	}
	if got != nil {
		t.Errorf("want nil results, got %v", got)
	}

	var target *stubBackendError
	if !errors.As(err, &target) {
		t.Fatalf("want errors.As to recover *stubBackendError through the all-failed errors.Join chain, got %v", err)
	}
}

// TestCompositeProvider_Notifications_PartialFailure_ErrorsAsRecovery
// verifies errors.As recovery through the partial path, which depends on
// PartialError.Unwrap() []error.
func TestCompositeProvider_Notifications_PartialFailure_ErrorsAsRecovery(t *testing.T) {
	healthy := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	healthy.notifs = []provider.Notification{mkNotif(provider.KindGitHub, "o/r", "1", t1)}
	broken := newFakeNotifyBackend(provider.KindAzure, []string{"P"})
	broken.listErr = &stubBackendError{msg: "boom"}

	cp := provider.NewCompositeProvider(healthy, broken)

	_, err := cp.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	var target *stubBackendError
	if !errors.As(err, &target) {
		t.Fatalf("want errors.As to recover *stubBackendError through PartialError.Unwrap(), got %v", err)
	}
	if target.msg != "boom" {
		t.Errorf("want recovered error msg %q, got %q", "boom", target.msg)
	}
}

// TestPartialError_Unwrap directly pins the additive Unwrap() []error method.
func TestPartialError_Unwrap(t *testing.T) {
	e1 := errors.New("e1")
	e2 := errors.New("e2")
	pe := &provider.PartialError{Failed: 2, Total: 3, Errors: []error{e1, e2}}

	got := pe.Unwrap()
	if len(got) != 2 || got[0] != e1 || got[1] != e2 {
		t.Fatalf("want Unwrap() to return the wrapped Errors slice, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// MarkRead / MarkDone routing
// ---------------------------------------------------------------------------

// TestCompositeProvider_MarkRead_RoutesByKindNotScope pins routing by kind:
// a row whose Identity.Scope is absent from the backend's registered Scopes()
// must still route to that backend.
func TestCompositeProvider_MarkRead_RoutesByKindNotScope(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"configured/repo"})
	cp := provider.NewCompositeProvider(b)

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "unconfigured/other-repo", ID: "99"}
	if err := cp.MarkRead(id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b.markReadCalls) != 1 || b.markReadCalls[0] != id {
		t.Fatalf("want MarkRead to reach the backend for an unconfigured scope, got calls=%v", b.markReadCalls)
	}
}

// TestCompositeProvider_MarkDone_RoutesByKindNotScope mirrors the MarkRead
// case for MarkDone, and its distinct call-recording slice also catches a
// MarkDone-calls-MarkRead copy-paste swap.
func TestCompositeProvider_MarkDone_RoutesByKindNotScope(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"configured/repo"})
	cp := provider.NewCompositeProvider(b)

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "unconfigured/other-repo", ID: "99"}
	if err := cp.MarkDone(id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b.markDoneCalls) != 1 || b.markDoneCalls[0] != id {
		t.Fatalf("want MarkDone to reach the backend for an unconfigured scope, got calls=%v", b.markDoneCalls)
	}
	if len(b.markReadCalls) != 0 {
		t.Fatalf("want MarkDone to never call the backend's MarkRead, got %d MarkRead calls", len(b.markReadCalls))
	}
}

// TestCompositeProvider_MarkRead_FiltersCapableBeforeKind pins the lookup
// shape: an incapable backend sharing the same Kind as a later capable
// backend must not shadow it. What this catches is any implementation that
// *gives up* at the first kind-matching-but-incapable backend — e.g. turning
// the !ok branch into an early `return nil, notificationMarkRouteErr(kind)`
// instead of continuing the loop.
//
// It deliberately does not claim to catch a swap of the two checks: with both
// `continue`s kept, checking Kind before capability is a provably equivalent
// mutant (each iteration skips on the disjunction of the same two conditions),
// so no test can distinguish the orders.
func TestCompositeProvider_MarkRead_FiltersCapableBeforeKind(t *testing.T) {
	incapableGH := &fakeBackend{kind: provider.KindGitHub, scopes: []string{"o/r"}}
	capableGH := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(incapableGH, capableGH)

	id := provider.Identity{Kind: provider.KindGitHub, Scope: "o/r", ID: "1"}
	if err := cp.MarkRead(id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capableGH.markReadCalls) != 1 {
		t.Fatalf("want 1 MarkRead call reaching the capable backend, got %d", len(capableGH.markReadCalls))
	}
}

// TestCompositeProvider_MarkRead_UnroutableKind verifies that a Kind with no
// matching capable backend falls through to a descriptive not-found error
// naming the kind, and never reaches an unrelated backend.
func TestCompositeProvider_MarkRead_UnroutableKind(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(b)

	id := provider.Identity{Kind: provider.KindAzure, Scope: "o/r", ID: "1"}
	err := cp.MarkRead(id)
	if err == nil {
		t.Fatal("want error for a kind with no capable backend, got nil")
	}
	if !strings.Contains(err.Error(), "azure") {
		t.Errorf("want the error message to name the unroutable kind, got %q", err.Error())
	}
	if len(b.markReadCalls) != 0 {
		t.Errorf("want no backend call for an unroutable kind, got %d calls", len(b.markReadCalls))
	}
}

// TestCompositeProvider_Mark_ZeroIdentity verifies that a zero-value Identity
// (unset Kind) falls through to the not-found error rather than accidentally
// matching the first capable backend — for MarkRead and MarkDone alike, each
// row asserting on its own call-recording slice so a
// MarkDone-calls-MarkRead swap stays visible.
func TestCompositeProvider_Mark_ZeroIdentity(t *testing.T) {
	tests := []struct {
		name  string
		mark  func(*provider.CompositeProvider, provider.Identity) error
		calls func(*fakeNotifyBackend) []provider.Identity
	}{
		{
			name:  "MarkRead",
			mark:  func(cp *provider.CompositeProvider, id provider.Identity) error { return cp.MarkRead(id) },
			calls: func(b *fakeNotifyBackend) []provider.Identity { return b.markReadCalls },
		},
		{
			name:  "MarkDone",
			mark:  func(cp *provider.CompositeProvider, id provider.Identity) error { return cp.MarkDone(id) },
			calls: func(b *fakeNotifyBackend) []provider.Identity { return b.markDoneCalls },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
			cp := provider.NewCompositeProvider(b)

			if err := tt.mark(cp, provider.Identity{}); err == nil {
				t.Fatal("want error for a zero-value Identity, got nil")
			}
			if got := tt.calls(b); len(got) != 0 {
				t.Errorf("want no backend call for a zero-value Identity, got %d calls", len(got))
			}
		})
	}
}

// TestCompositeProvider_MarkRead_ZeroKindCapableBackend pins the explicit
// kind == 0 guard rather than leaving it implied by KindAzure = iota + 1.
// TestCompositeProvider_MarkRead_ZeroIdentity above cannot: its fixture
// reports KindGitHub, so it pins the kind-*mismatch* path. Here the sole
// capable backend reports Kind() == 0 — the shape CompositeProvider.Kind()
// itself returns on an empty backend list — so without the guard a fully
// zero-valued Identity matches it and MarkRead returns nil, an optimistic row
// update whose rollback never fires for a mark that targeted nothing.
func TestCompositeProvider_MarkRead_ZeroKindCapableBackend(t *testing.T) {
	b := newFakeNotifyBackend(provider.Kind(0), []string{"o/r"})
	cp := provider.NewCompositeProvider(b)

	if b.Kind() != 0 {
		t.Fatalf("fixture must report Kind() == 0 for this test to pin the guard, got %d", b.Kind())
	}

	err := cp.MarkRead(provider.Identity{})
	if err == nil {
		t.Fatal("want error routing a zero Kind even when a capable backend reports Kind() == 0, got nil")
	}
	if len(b.markReadCalls) != 0 {
		t.Errorf("want no backend call for a zero Kind, got %d calls: %v", len(b.markReadCalls), b.markReadCalls)
	}
}
