package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
)

// TestRunTUI_UsesGitHubAdapterWithNotifications pins the GitHub backend
// wiring in runTUI: it must construct the adapter via
// github.NewAdapterWithNotifications (which also wires a
// github.NotificationsClient) rather than the notifications-less
// github.NewAdapter. Without this, a.nc stays nil forever (see
// internal/github/adapter.go's NewAdapter doc comment), and every List call
// on the GitHub backend returns "no notifications client configured" even
// though the notifications tab shows up under the capability
// check — *Adapter satisfies provider.NotificationSource at compile time
// regardless of nc, so a type-assertion-only test cannot catch a regression
// back to NewAdapter here. Parsing the AST (rather than grepping source
// text) keeps this test immune to reformatting/reordering of runTUI while
// still failing the moment the wrong constructor is called.
func TestRunTUI_UsesGitHubAdapterWithNotifications(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path via runtime.Caller")
	}
	mainFile := filepath.Join(filepath.Dir(thisFile), "main.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, mainFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", mainFile, err)
	}

	var (
		sawNewAdapterWithNotifications bool
		sawBareNewAdapter              bool
		sawNewNotificationsClient      bool
		notifAdapterArgCount           int
		notifAdapterSecondArgIsNil     bool
	)

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "github" {
			return true
		}
		switch sel.Sel.Name {
		case "NewAdapterWithNotifications":
			sawNewAdapterWithNotifications = true
			notifAdapterArgCount = len(call.Args)
			if len(call.Args) == 2 {
				if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == "nil" {
					notifAdapterSecondArgIsNil = true
				}
			}
		case "NewAdapter":
			sawBareNewAdapter = true
		case "NewNotificationsClient":
			sawNewNotificationsClient = true
		}
		return true
	})

	if !sawNewAdapterWithNotifications {
		t.Fatal("expected runTUI to call github.NewAdapterWithNotifications to construct the GitHub backend, found no such call")
	}
	if sawBareNewAdapter {
		t.Error("runTUI must not call github.NewAdapter (leaves the notifications client nil); use github.NewAdapterWithNotifications instead")
	}

	// Pinning the callee is only half the mutation space. The
	// walk above keys on sel.Sel.Name, so deleting the
	// `ghNC := github.NewNotificationsClient(token)` line and passing nil as
	// the second argument satisfies every assertion so far while reproducing
	// exactly the state this test's doc comment says it prevents: a.nc stays
	// nil and every List call returns "no notifications client configured".
	//
	// Note this pins a property that is currently unreachable rather than
	// fixing a live bug — NewNotificationsClient always returns non-nil with no
	// error, GetGitHubToken() has already errored out upstream, and GitHubConfig
	// carries no base-URL/GHE field. The value is that it stays unreachable.
	if notifAdapterArgCount != 2 {
		t.Errorf("github.NewAdapterWithNotifications called with %d args, want 2 (MultiClient, NotificationsClient)", notifAdapterArgCount)
	}
	if notifAdapterSecondArgIsNil {
		t.Error("github.NewAdapterWithNotifications's second argument must not be nil — a nil NotificationsClient makes every notifications List call fail with \"no notifications client configured\" while the tab still shows up under the capability check")
	}
	if !sawNewNotificationsClient {
		t.Error("expected runTUI to construct the user-scoped client via github.NewNotificationsClient, found no such call")
	}
}

// TestRunTUI_UsesAzureAdapterWithNotifications mirrors
// TestRunTUI_UsesGitHubAdapterWithNotifications above for the Azure DevOps
// backend: runTUI must construct the adapter via
// azdevops.NewAdapterWithNotifications (which also wires an
// azdevops.TriageStore) rather than the notifications-less
// azdevops.NewAdapter. Without this, the zero-value *Adapter still
// satisfies provider.NotificationSource by method set alone — Go's
// structural typing does not care that notifStore was never wired up — so
// the Notifications tab shows up for an Azure-only config while every List
// call fails closed with "azdevops: notifications: not configured" (task 8
// review, 🔴 finding 1). As with the GitHub test, parsing the AST rather
// than grepping source text keeps this immune to reformatting/reordering of
// runTUI while still failing the moment the wrong constructor is called.
func TestRunTUI_UsesAzureAdapterWithNotifications(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path via runtime.Caller")
	}
	mainFile := filepath.Join(filepath.Dir(thisFile), "main.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, mainFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", mainFile, err)
	}

	var (
		sawNewAdapterWithNotifications bool
		sawBareNewAdapter              bool
		sawNewTriageStore              bool
		sawAzureNotificationArgsCall   bool
		notifAdapterArgCount           int
		notifAdapterSecondArgIsNil     bool
		notifAdapterHasHardcodedArg    bool
	)

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Bare, unqualified call — looking for azureNotificationArgs(...),
		// which lives in this same package (main), not behind a package
		// selector.
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "azureNotificationArgs" {
			sawAzureNotificationArgsCall = true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "azdevops" {
			return true
		}
		switch sel.Sel.Name {
		case "NewAdapterWithNotifications":
			sawNewAdapterWithNotifications = true
			notifAdapterArgCount = len(call.Args)
			if len(call.Args) >= 2 {
				if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == "nil" {
					notifAdapterSecondArgIsNil = true
				}
			}
			// The third, fourth and fifth arguments (lookbackDays,
			// toggles, minPollInterval) must come from config, not from a
			// hardcoded literal 0 or azdevops.DefaultNotificationSourceToggles()
			// — task 11 parses, defaults, clamps and validates
			// notifications.azure's fields, and a hardcoded arg here is
			// exactly the gap task 14 exists to close (a user-set
			// lookback_days/sources/min_poll_interval accepted, validated,
			// then silently ignored). This does not pin *which* helper
			// supplies the value, only that none of these three positions
			// is a bare 0 literal or a call to
			// DefaultNotificationSourceToggles — see
			// TestAzureNotificationArgs_NonDefaultValuesReachTheAdapter for
			// the runtime proof that azureNotificationArgs itself carries a
			// non-default value through correctly.
			for i := 2; i < len(call.Args) && i <= 4; i++ {
				arg := call.Args[i]
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Value == "0" {
					notifAdapterHasHardcodedArg = true
				}
				if argCall, ok := arg.(*ast.CallExpr); ok {
					if argSel, ok := argCall.Fun.(*ast.SelectorExpr); ok && argSel.Sel.Name == "DefaultNotificationSourceToggles" {
						notifAdapterHasHardcodedArg = true
					}
				}
			}
		case "NewAdapter":
			sawBareNewAdapter = true
		case "NewTriageStore":
			sawNewTriageStore = true
		}
		return true
	})

	if !sawNewAdapterWithNotifications {
		t.Fatal("expected runTUI to call azdevops.NewAdapterWithNotifications to construct the Azure DevOps backend, found no such call")
	}
	if sawBareNewAdapter {
		t.Error("runTUI must not call azdevops.NewAdapter (leaves the notifications store nil, and the zero-value *Adapter still satisfies provider.NotificationSource by method set alone); use azdevops.NewAdapterWithNotifications instead")
	}

	if notifAdapterArgCount != 5 {
		t.Errorf("azdevops.NewAdapterWithNotifications called with %d args, want 5 (MultiClient, *TriageStore, lookbackDays, NotificationSourceToggles, minPollInterval)", notifAdapterArgCount)
	}
	if notifAdapterSecondArgIsNil {
		t.Error("azdevops.NewAdapterWithNotifications's second argument must not be nil — a nil TriageStore makes every notifications List call fail with \"azdevops: notifications: not configured\" while the tab still shows up under the capability check")
	}
	if !sawNewTriageStore {
		t.Error("expected runTUI to construct the local triage store via azdevops.NewTriageStore, found no such call")
	}
	if notifAdapterHasHardcodedArg {
		t.Error("azdevops.NewAdapterWithNotifications's lookbackDays/toggles/minPollInterval arguments must come from notifications.azure config, not a hardcoded 0 literal or azdevops.DefaultNotificationSourceToggles() — a user-set lookback_days, sources toggle or min_poll_interval would be silently ignored")
	}
	if !sawAzureNotificationArgsCall {
		t.Error("expected runTUI to derive azdevops.NewAdapterWithNotifications's config-sourced arguments via azureNotificationArgs, found no such call")
	}
}

// TestAzureNotificationArgs_NonDefaultValuesReachTheAdapter is the runtime
// counterpart to the AST checks above: it proves a non-default
// notifications.azure value actually reaches azdevops.NewAdapterWithNotifications's
// arguments, not merely that config.NotificationsAzureConfig parses one.
// Every field below is deliberately set away from its LoadFrom default
// (LookbackDays 14, MinPollInterval 300, every Sources bool true) so a
// regression back to a hardcoded default — the exact bug task 14 exists to
// fix — fails this test even if it happened to also satisfy the AST checks
// above.
func TestAzureNotificationArgs_NonDefaultValuesReachTheAdapter(t *testing.T) {
	azure := config.NotificationsAzureConfig{
		LookbackDays:    7,
		MinPollInterval: 42,
		Sources: config.NotificationsAzureSourcesConfig{
			ReviewRequested: false,
			Mentioned:       true,
			Assigned:        false,
			CIFailed:        true,
		},
	}

	lookbackDays, toggles, minPollInterval := azureNotificationArgs(azure)

	if lookbackDays != 7 {
		t.Errorf("lookbackDays = %d, want 7", lookbackDays)
	}
	wantToggles := azdevops.NotificationSourceToggles{
		ReviewRequested: false,
		Mentioned:       true,
		Assigned:        false,
		CIFailed:        true,
	}
	if toggles != wantToggles {
		t.Errorf("toggles = %+v, want %+v", toggles, wantToggles)
	}
	if minPollInterval != 42*time.Second {
		t.Errorf("minPollInterval = %v, want %v", minPollInterval, 42*time.Second)
	}
}
