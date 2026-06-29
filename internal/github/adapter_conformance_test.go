//go:build adapter

package github_test

import (
	"github.com/Elpulgo/azdo/internal/github"
	"github.com/Elpulgo/azdo/internal/provider"
)

// Compile-time assertion: Adapter must satisfy provider.Provider.
// This file is excluded from the default build. Compile with -tags adapter to
// verify the conformance gate:
//
//	CGO_ENABLED=0 go build -tags adapter ./...
var _ provider.Provider = (*github.Adapter)(nil)
