// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resourcetest

import (
	"context"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/assets/resourcedata/embedded"
	"github.com/exasol/exasol-personal/internal/resource"
)

func NewResolverContext(t *testing.T, spec resource.ResourceSpec) context.Context {
	t.Helper()

	resolver, err := resource.New(resource.Options{
		Definitions: spec,
		CacheRoot:   t.TempDir(),
		Platform:    resource.Platform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
	})
	if err != nil {
		t.Fatalf("failed to build a resolver: %v", err)
	}

	return resource.NewContext(context.Background(), resolver)
}

func NewContext(t *testing.T) context.Context {
	t.Helper()

	resolver, err := resource.New(resource.Options{
		Spec:      embedded.ResolvedSpec,
		Blobs:     embedded.Blobs,
		CacheRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("failed to build a resolver: %v", err)
	}

	return resource.NewContext(context.Background(), resolver)
}
