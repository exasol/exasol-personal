// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestResolveBackendForManifest_UsesExplicitBackend(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	// When
	backend, err := resolveBackendForManifest(manifest)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(tofuBackend); !ok {
		t.Fatalf("expected tofuBackend, got %T", backend)
	}
}

func TestResolveBackendForManifest_FallsBackToTofuForLegacyManifest(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Tofu: &presets.InfrastructureTofu{},
	}

	// When
	backend, err := resolveBackendForManifest(manifest)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(tofuBackend); !ok {
		t.Fatalf("expected tofuBackend, got %T", backend)
	}
}

func TestResolveBackendForManifest_RejectsUnknownBackend(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{Backend: "unknown"}

	// When
	_, err := resolveBackendForManifest(manifest)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownDeploymentType) {
		t.Fatalf("expected ErrUnknownDeploymentType, got %v", err)
	}
}

func TestResolveBackendForManifest_UsesLocalBackend(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	// When
	backend, err := resolveBackendForManifest(manifest)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(localBackend); !ok {
		t.Fatalf("expected localBackend, got %T", backend)
	}
}
