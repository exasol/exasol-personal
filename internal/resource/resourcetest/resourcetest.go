// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package resourcetest provides shared test helpers for building a
// context.Context carrying a *resource.Manager, for tests in other
// packages that exercise code reading the shared Manager from context.
package resourcetest

import (
	"context"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/assets/resourcedata"
	"github.com/exasol/exasol-personal/internal/resource"
)

// NewManagerContext returns a context carrying a Manager built from spec,
// backed by a throwaway cache root.
func NewManagerContext(t *testing.T, spec resource.ResourceSpec) context.Context {
	t.Helper()

	manager := resource.NewResourceManagerForPlatform(
		spec, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)

	return resource.NewContext(context.Background(), manager)
}

// NewContext returns a context carrying a Manager backed by the real embedded
// resource catalog (so embedded presets resolve for real, since they are
// embed: always) and a throwaway cache root.
func NewContext(t *testing.T) context.Context {
	t.Helper()

	manager, err := resource.NewResourceManagerWithSpecForPlatform(
		resourcedata.ResourcesYAML, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)
	if err != nil {
		t.Fatalf("failed to parse resources.yaml: %v", err)
	}

	return resource.NewContext(context.Background(), manager)
}
