// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/resource/resourcetest"
)

// testManagerContext returns a context carrying a Manager backed by the real
// embedded resource catalog, for tests that exercise commands reading the
// shared Manager from context.
func testManagerContext(t *testing.T) context.Context {
	t.Helper()

	return resourcetest.NewContext(t)
}
