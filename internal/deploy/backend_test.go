// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
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

func TestNewLocalRuntimeForPlatform_SelectsRuntime(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	tests := []struct {
		name   string
		goos   string
		goarch string
		assert func(*testing.T, localruntime.Runtime)
	}{
		{
			name: "mac vm",
			goos: localMacOS, goarch: localMacArch,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				if _, ok := selected.(*localruntime.MacVMRuntime); !ok {
					t.Fatalf("expected MacVMRuntime, got %T", selected)
				}
			},
		},
		{
			name: "linux amd64",
			goos: localLinuxOS, goarch: localLinuxAMD64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				if _, ok := selected.(*localruntime.LinuxHostRuntime); !ok {
					t.Fatalf("expected LinuxHostRuntime, got %T", selected)
				}
			},
		},
		{
			name: "linux arm64",
			goos: localLinuxOS, goarch: localLinuxARM64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				if _, ok := selected.(*localruntime.LinuxHostRuntime); !ok {
					t.Fatalf("expected LinuxHostRuntime, got %T", selected)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selected, err := newLocalRuntimeForPlatform(
				deployment, nil, test.goos, test.goarch,
			)
			if err != nil {
				t.Fatalf("expected runtime selection to succeed, got %v", err)
			}
			test.assert(t, selected)
		})
	}
}

func TestNewLocalRuntimeForPlatform_RejectsWindows(t *testing.T) {
	t.Parallel()

	_, err := newLocalRuntimeForPlatform(
		config.NewDeploymentDir(t.TempDir()), nil, "windows", "amd64",
	)
	if !errors.Is(err, errUnsupportedLocalPlatform) {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}
