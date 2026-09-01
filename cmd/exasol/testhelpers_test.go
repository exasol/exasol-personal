// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/resource/resourcetest"
)

func testResolverContext(t *testing.T) context.Context {
	t.Helper()

	return resourcetest.NewContext(t)
}
