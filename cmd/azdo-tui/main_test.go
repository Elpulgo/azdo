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
		case "NewAdapter":
			sawBareNewAdapter = true
		}
		return true
	})

	if !sawNewAdapterWithNotifications {
		t.Error("expected runTUI to call github.NewAdapterWithNotifications to construct the GitHub backend, found no such call")
	}
	if sawBareNewAdapter {
		t.Error("runTUI must not call github.NewAdapter (leaves the notifications client nil); use github.NewAdapterWithNotifications instead")
	}
}
