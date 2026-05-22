// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestResolveBackendKind_UsesExplicitBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	kind, err := resolveBackendKind(manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if kind != backendTypeTofu {
		t.Fatalf("expected kind %q, got %q", backendTypeTofu, kind)
	}
}

func TestResolveBackendKind_FallsBackToTofuForLegacyManifest(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{
		Tofu: &presets.InfrastructureTofu{},
	}

	kind, err := resolveBackendKind(manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if kind != backendTypeTofu {
		t.Fatalf("expected kind %q, got %q", backendTypeTofu, kind)
	}
}

func TestResolveBackendKind_RejectsUnknownBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: "unknown"}

	_, err := resolveBackendKind(manifest)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownDeploymentType) {
		t.Fatalf("expected ErrUnknownDeploymentType, got %v", err)
	}
}

func TestNewDeploymentBackend_ReturnsTofuBackendForTofuManifest(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Backend: backendTypeTofu,
		Tofu:    &presets.InfrastructureTofu{},
	}

	backend, err := newDeploymentBackend(deployment, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(*tofuBackend); !ok {
		t.Fatalf("expected *tofuBackend, got %T", backend)
	}
}

func TestNewDeploymentBackend_AcceptsTofuManifestWithoutTofuSectionAsNoop(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	backend, err := newDeploymentBackend(deployment, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestNewDeploymentBackend_ReturnsLocalBackendForLocalManifest(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	backend, err := newDeploymentBackend(deployment, manifest)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := backend.(*localBackend); !ok {
		t.Fatalf("expected *localBackend, got %T", backend)
	}
}
