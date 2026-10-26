// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts/runtimeartifactstest"
)

// testManagerContext returns a context carrying a Manager backed by the real
// embedded resource catalog, for tests that exercise code reading the shared
// Manager from context.
func testManagerContext(t *testing.T) context.Context {
	t.Helper()

	return runtimeartifactstest.NewContext(t)
}

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

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
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

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
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

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
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
				assertHostRuntimePlatform(t, selected, localruntime.HostPlatformLinux)
			},
		},
		{
			name: "linux arm64",
			goos: localLinuxOS, goarch: localLinuxARM64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				assertHostRuntimePlatform(t, selected, localruntime.HostPlatformLinux)
			},
		},
		{
			name: "windows amd64",
			goos: localWindowsOS, goarch: localWindowsAMD64,
			assert: func(t *testing.T, selected localruntime.Runtime) {
				t.Helper()
				assertHostRuntimePlatform(t, selected, localruntime.HostPlatformWindows)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selected, err := newLocalRuntimeForPlatform(
				context.Background(), deployment, nil, nil, test.goos, test.goarch,
				detectEngine(localinstall.EnginePodman),
			)
			if err != nil {
				t.Fatalf("expected runtime selection to succeed, got %v", err)
			}
			test.assert(t, selected)
		})
	}
}

func TestNewLocalRuntimeForPlatform_RejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	_, err := newLocalRuntimeForPlatform(
		context.Background(), config.NewDeploymentDir(t.TempDir()), nil, nil, "freebsd", "amd64",
		detectEngine(localinstall.EnginePodman),
	)
	if !errors.Is(err, errUnsupportedLocalPlatform) {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

func TestNewLocalRuntimeForPlatform_SelectsConfiguredContainerEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured string
		detected   localinstall.ContainerEngine
		expected   localinstall.ContainerEngine
	}{
		{
			name: "unset detects docker", detected: localinstall.EngineDocker,
			expected: localinstall.EngineDocker,
		},
		{
			name: "unset detects podman", detected: localinstall.EnginePodman,
			expected: localinstall.EnginePodman,
		},
		{
			name: "auto detects docker", configured: "auto", detected: localinstall.EngineDocker,
			expected: localinstall.EngineDocker,
		},
		{
			name: "explicit podman ignores detection", configured: "podman",
			detected: localinstall.EngineDocker, expected: localinstall.EnginePodman,
		},
		{
			name: "explicit docker ignores detection", configured: "docker",
			detected: localinstall.EnginePodman, expected: localinstall.EngineDocker,
		},
	}
	for _, platform := range []struct{ goos, goarch string }{
		{localLinuxOS, localLinuxAMD64}, {localWindowsOS, localWindowsAMD64},
	} {
		for _, test := range tests {
			t.Run(platform.goos+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				manifest := &presets.InfrastructureManifest{
					Local: &presets.InfrastructureLocal{ContainerEngine: test.configured},
				}

				selected, err := newLocalRuntimeForPlatform(
					context.Background(), config.NewDeploymentDir(t.TempDir()), manifest, nil,
					platform.goos, platform.goarch, detectEngine(test.detected),
				)
				if err != nil {
					t.Fatalf("expected runtime selection to succeed, got %v", err)
				}
				assertHostRuntimeEngine(t, selected, test.expected)
			})
		}
	}
}

func TestNewLocalRuntimeForPlatform_DetectsAutoInPodmanFirstOrder(t *testing.T) {
	t.Parallel()
	expected := []localinstall.ContainerEngine{
		localinstall.EnginePodman, localinstall.EngineDocker,
	}

	for _, platform := range []struct{ goos, goarch string }{
		{localLinuxOS, localLinuxAMD64}, {localWindowsOS, localWindowsAMD64},
	} {
		t.Run(platform.goos, func(t *testing.T) {
			t.Parallel()
			var preference []localinstall.ContainerEngine
			detect := func(
				_ context.Context, engines []localinstall.ContainerEngine,
			) localinstall.ContainerEngine {
				preference = engines

				return engines[0]
			}

			_, err := newLocalRuntimeForPlatform(
				context.Background(), config.NewDeploymentDir(t.TempDir()),
				&presets.InfrastructureManifest{Local: &presets.InfrastructureLocal{}}, nil,
				platform.goos, platform.goarch, detect,
			)
			if err != nil {
				t.Fatalf("expected runtime selection to succeed, got %v", err)
			}
			if !slices.Equal(preference, expected) {
				t.Fatalf("expected preference %v, got %v", expected, preference)
			}
		})
	}
}

func TestNewLocalRuntimeForPlatform_KeepsTheRecordedEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		record   string
		expected localinstall.ContainerEngine
	}{
		{
			name: "recorded docker", record: `{"engine":"docker"}`,
			expected: localinstall.EngineDocker,
		},
		{
			name: "recorded podman", record: `{"engine":"podman"}`,
			expected: localinstall.EnginePodman,
		},
		{name: "runtime directory from before records", expected: localinstall.EnginePodman},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deployment := config.NewDeploymentDir(t.TempDir())
			writeContainerEngineRecord(t, deployment, test.record)
			manifest := &presets.InfrastructureManifest{
				Local: &presets.InfrastructureLocal{ContainerEngine: "auto"},
			}
			detectsOther := localinstall.EngineDocker
			if test.expected == localinstall.EngineDocker {
				detectsOther = localinstall.EnginePodman
			}

			selected, err := newLocalRuntimeForPlatform(
				context.Background(), deployment, manifest, nil, localLinuxOS, localLinuxAMD64,
				detectEngine(detectsOther),
			)
			if err != nil {
				t.Fatalf("expected runtime selection to succeed, got %v", err)
			}
			assertHostRuntimeEngine(t, selected, test.expected)
		})
	}
}

func TestNewLocalRuntimeForPlatform_RejectsUnsupportedContainerEngine(t *testing.T) {
	t.Parallel()
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{ContainerEngine: "containerd"},
	}

	_, err := newLocalRuntimeForPlatform(
		context.Background(),
		config.NewDeploymentDir(t.TempDir()),
		manifest,
		nil,
		localLinuxOS,
		localLinuxAMD64,
		detectEngine(localinstall.EnginePodman),
	)
	if err == nil || !strings.Contains(err.Error(), "must be one of auto, podman, docker") {
		t.Fatalf("expected the configuration error for the stored engine, got %v", err)
	}
}

func TestNewLocalRuntime_ReadsContainerEngineFromDeployment(t *testing.T) {
	requireLinuxLocalPlatform(t)
	t.Parallel()
	deployment := newLocalTestDeployment(t)
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: local
local:
  containerEngine: docker
`)

	selected, err := newLocalRuntime(context.Background(), deployment, nil)
	if err != nil {
		t.Fatalf("expected runtime selection to succeed, got %v", err)
	}
	assertHostRuntimeEngine(t, selected, localinstall.EngineDocker)
}

func TestNewLocalRuntime_RequiresDeploymentManifest(t *testing.T) {
	t.Parallel()

	_, err := newLocalRuntime(context.Background(), config.NewDeploymentDir(t.TempDir()), nil)
	if err == nil {
		t.Fatal("expected runtime selection to fail without a deployment manifest")
	}
}

func detectEngine(engine localinstall.ContainerEngine) containerEngineDetector {
	return func(context.Context, []localinstall.ContainerEngine) localinstall.ContainerEngine {
		return engine
	}
}

// writeContainerEngineRecord creates the deployment's database data, with the
// given record when it is not empty.
func writeContainerEngineRecord(t *testing.T, deployment config.DeploymentDir, record string) {
	t.Helper()
	workDir := filepath.Join(deployment.Resolve("local"), "runtime")
	if err := os.MkdirAll(filepath.Join(workDir, "exa"), 0o750); err != nil {
		t.Fatalf("failed to create database data: %v", err)
	}
	if record == "" {
		return
	}
	writeTestFile(t, filepath.Join(workDir, "container-engine.json"), record)
}

func assertHostRuntimeEngine(
	t *testing.T,
	selected localruntime.Runtime,
	expected localinstall.ContainerEngine,
) {
	t.Helper()
	host, ok := selected.(*localruntime.HostRuntime)
	if !ok {
		t.Fatalf("expected HostRuntime, got %T", selected)
	}
	if host.ContainerEngine() != expected {
		t.Fatalf("expected container engine %q, got %q", expected, host.ContainerEngine())
	}
}

// assertHostRuntimePlatform checks the selector produced a direct-host
// runtime for the expected platform. Linux and Windows share one runtime
// type, so the platform is what distinguishes them.
func assertHostRuntimePlatform(
	t *testing.T,
	selected localruntime.Runtime,
	expected localruntime.HostPlatform,
) {
	t.Helper()
	hostRuntime, ok := selected.(*localruntime.HostRuntime)
	if !ok {
		t.Fatalf("expected HostRuntime, got %T", selected)
	}
	if hostRuntime.Platform() != expected {
		t.Fatalf("expected platform %q, got %q", expected, hostRuntime.Platform())
	}
}
