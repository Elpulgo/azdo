package provider_test

import (
	"errors"
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

func TestCompositeProvider_HasNotifications_FalseWithZeroCapable(t *testing.T) {
	a := &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}
	cp := provider.NewCompositeProvider(a)

	if cp.HasNotifications() {
		t.Fatal("want false with zero capable backends")
	}
}

func TestCompositeProvider_HasNotifications_TrueWithOneCapable(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(b)

	if !cp.HasNotifications() {
		t.Fatal("want true with one capable backend")
	}
}

func TestCompositeProvider_HasNotifications_TrueWithMix(t *testing.T) {
	incapable := &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}
	capable := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(incapable, capable)

	if !cp.HasNotifications() {
		t.Fatal("want true with a mix of capable and incapable backends")
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

// TestCompositeProvider_Notifications_MergeSortedNewestFirst interleaves rows
// across two backends, including a tie, so the sort is genuinely exercised
// rather than validated against already-ordered input.
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
	// The two t3 rows tie; sort.Slice does not guarantee their relative
	// order, so only assert both land ahead of t2 and t1.
	if !got[0].UpdatedAt.Equal(t3) || !got[1].UpdatedAt.Equal(t3) {
		t.Errorf("want the first two entries at t3 (tie), got %v and %v", got[0].UpdatedAt, got[1].UpdatedAt)
	}
	if !got[2].UpdatedAt.Equal(t2) {
		t.Errorf("want the third entry at t2, got %v", got[2].UpdatedAt)
	}
	if !got[3].UpdatedAt.Equal(t1) {
		t.Errorf("want the fourth (oldest) entry at t1, got %v", got[3].UpdatedAt)
	}
}

// TestCompositeProvider_Notifications_NilVsEmptySliceConsistent verifies a
// capable backend returning (nil, nil) and one returning an empty non-nil
// slice merge to the same observable (empty) result.
func TestCompositeProvider_Notifications_NilVsEmptySliceConsistent(t *testing.T) {
	a := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r1"})
	a.notifs = nil
	b := newFakeNotifyBackend(provider.KindAzure, []string{"o/r2"})
	b.notifs = []provider.Notification{}

	cp := provider.NewCompositeProvider(a, b)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want an empty merged result regardless of nil-vs-empty sources, got %d items", len(got))
	}
}

// ---------------------------------------------------------------------------
// Decision 20 / 41: partial failure never empties the feed; total is the
// capable count.
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_PartialFailure verifies Decision 20:
// one backend erroring keeps the other backend's rows and surfaces a
// *PartialError alongside them.
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
// pins Decision 41's exact scenario: one incapable Azure backend plus one
// capable GitHub backend that fails. total must be 1 (the capable count),
// not 2 (len(cp.backends)) — otherwise len(errs)==1 != total==2 takes the
// partial branch and returns (nil, &PartialError{1,2}), an empty feed
// carrying a buried error.
func TestCompositeProvider_Notifications_TotalIsCapableCountNotAllBackends(t *testing.T) {
	incapable := &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}
	broken := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	broken.listErr = errors.New("403 missing scope")

	cp := provider.NewCompositeProvider(incapable, broken)

	got, err := cp.List(provider.NotifOpts{})
	if err == nil {
		t.Fatal("want an error when the only capable backend fails")
	}
	var pe *provider.PartialError
	if errors.As(err, &pe) {
		t.Fatalf("want a plain all-failed error (total=1 capable backend), got *PartialError with Total=%d — total must count capable backends only (Decision 41)", pe.Total)
	}
	if got != nil {
		t.Errorf("want nil results on all-failed (1 of 1 capable backends), got %v", got)
	}
}

// TestCompositeProvider_Notifications_ZeroCapable_ReturnsEmptyNilError
// verifies the separate zero-capable-backends guard: len(errs)==total is
// satisfied vacuously at 0==0, so this must not take the all-failed branch.
func TestCompositeProvider_Notifications_ZeroCapable_ReturnsEmptyNilError(t *testing.T) {
	a := &fakeBackend{kind: provider.KindAzure, scopes: []string{"P"}}
	cp := provider.NewCompositeProvider(a)

	got, err := cp.List(provider.NotifOpts{})
	if err != nil {
		t.Fatalf("want nil error with zero capable backends, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want an empty result, got %d items", len(got))
	}
}

// ---------------------------------------------------------------------------
// Decision 42: error chain preservation
// ---------------------------------------------------------------------------

// TestCompositeProvider_Notifications_AllFail_ErrorsAsRecovery verifies that
// when every capable backend fails, errors.As can still recover a specific
// underlying error type through the errors.Join chain (Decision 42).
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
// Decisions 25 / 43: MarkRead / MarkDone routing
// ---------------------------------------------------------------------------

// TestCompositeProvider_MarkRead_RoutesByKindNotScope is the test that
// matters most for Decision 25: a row whose Identity.Scope is absent from
// the backend's registered Scopes() must still route to that backend.
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

// TestCompositeProvider_MarkRead_FiltersCapableBeforeKind pins Decision 43's
// lookup order: an incapable backend sharing the same Kind as a later
// capable backend must not shadow it. Matching kind before asserting
// capability would let the incapable backend's kind match win and return a
// not-found error instead of continuing the search.
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

// TestCompositeProvider_MarkRead_ZeroIdentity verifies that a zero-value
// Identity (unset Kind) falls through to the not-found error rather than
// accidentally matching the first capable backend.
func TestCompositeProvider_MarkRead_ZeroIdentity(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(b)

	err := cp.MarkRead(provider.Identity{})
	if err == nil {
		t.Fatal("want error for a zero-value Identity, got nil")
	}
	if len(b.markReadCalls) != 0 {
		t.Errorf("want no backend call for a zero-value Identity, got %d calls", len(b.markReadCalls))
	}
}

// TestCompositeProvider_MarkDone_ZeroIdentity mirrors the MarkRead case for
// MarkDone.
func TestCompositeProvider_MarkDone_ZeroIdentity(t *testing.T) {
	b := newFakeNotifyBackend(provider.KindGitHub, []string{"o/r"})
	cp := provider.NewCompositeProvider(b)

	err := cp.MarkDone(provider.Identity{})
	if err == nil {
		t.Fatal("want error for a zero-value Identity, got nil")
	}
	if len(b.markDoneCalls) != 0 {
		t.Errorf("want no backend call for a zero-value Identity, got %d calls", len(b.markDoneCalls))
	}
}
