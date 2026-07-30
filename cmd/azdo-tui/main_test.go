package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRunTUI_UsesGitHubAdapterWithNotifications pins the GitHub backend
// wiring in runTUI: it must construct the adapter via
// github.NewAdapterWithNotifications (which also wires a
// github.NotificationsClient) rather than the notifications-less
// github.NewAdapter. Without this, a.nc stays nil forever (see
// internal/github/adapter.go's NewAdapter doc comment), and every List call
// on the GitHub backend returns "no notifications client configured" even
// though the notifications tab shows up under Decision 11's capability
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

	// Pinning the callee is only half the mutation space (Decision 62). The
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
		t.Error("github.NewAdapterWithNotifications's second argument must not be nil — a nil NotificationsClient makes every notifications List call fail with \"no notifications client configured\" while the tab still shows up under Decision 11's capability check")
	}
	if !sawNewNotificationsClient {
		t.Error("expected runTUI to construct the user-scoped client via github.NewNotificationsClient, found no such call")
	}
}
