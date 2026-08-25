// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package runtimeartifactstest provides shared test helpers for building a
// context.Context carrying a *runtimeartifacts.Manager, for tests in other
// packages that exercise code reading the shared Manager from context.
package runtimeartifactstest

import (
	"context"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// NewManagerContext returns a context carrying a Manager built from spec,
// backed by a throwaway cache root.
func NewManagerContext(t *testing.T, spec runtimeartifacts.ResourceSpec) context.Context {
	t.Helper()

	manager := runtimeartifacts.NewResourceManagerForPlatform(
		spec, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)

	return runtimeartifacts.NewContext(context.Background(), manager)
}

// NewContext returns a context carrying a Manager backed by the real embedded
// resource catalog (so embedded presets resolve for real, since they are
// embed: always) and a throwaway cache root.
func NewContext(t *testing.T) context.Context {
	t.Helper()

	manager, err := runtimeartifacts.NewResourceManagerWithSpecForPlatform(
		resources.ResourcesYAML, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)
	if err != nil {
		t.Fatalf("failed to parse resources.yaml: %v", err)
	}

	return runtimeartifacts.NewContext(context.Background(), manager)
}
