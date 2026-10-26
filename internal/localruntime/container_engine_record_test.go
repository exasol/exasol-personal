// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestHostRuntimePrepare_RecordsTheEngineOnce(t *testing.T) {
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "docker", `printf 'linux 29.9.1\n'`)
	deployment := config.NewDeploymentDir(t.TempDir())

	runtime := NewHostLinuxRuntime(deployment, nil, localinstall.EngineDocker)
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("expected preparation to succeed, got %v", err)
	}
	assertRecordedEngine(t, deployment, localinstall.EngineDocker)

	writeLinuxRuntimeTestFile(t, containerEngineMarkerPath(newRuntimePaths(deployment)),
		`{"engine":"podman"}`)
	if err := runtime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("expected repeated preparation to succeed, got %v", err)
	}
	assertRecordedEngine(t, deployment, localinstall.EnginePodman)
}

func TestRecordedContainerEngine_TreatsPreRecordDatabaseDataAsPodman(t *testing.T) {
	t.Parallel()
	deployment := config.NewDeploymentDir(t.TempDir())

	if _, recorded, err := RecordedContainerEngine(deployment); err != nil || recorded {
		t.Fatalf("expected no record before the first start, got recorded=%t err=%v",
			recorded, err)
	}
	dataDir := filepath.Join(newRuntimePaths(deployment).WorkDir, nanoDataDirName)
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatalf("failed to create database data: %v", err)
	}
	assertRecordedEngine(t, deployment, localinstall.EnginePodman)
}

// A runtime directory left without data or a record, as after a partly
// completed destroy, does not turn the deployment into a Podman deployment.
func TestRecordedContainerEngine_IgnoresRuntimeFilesWithoutDatabaseData(t *testing.T) {
	t.Parallel()
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(newRuntimePaths(deployment).WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create runtime directory: %v", err)
	}

	if engine, recorded, err := RecordedContainerEngine(deployment); err != nil || recorded {
		t.Fatalf("expected no recorded engine, got %q recorded=%t err=%v", engine, recorded, err)
	}
}

func TestRecordedContainerEngine_RejectsACorruptRecord(t *testing.T) {
	t.Parallel()
	deployment := config.NewDeploymentDir(t.TempDir())
	writeLinuxRuntimeTestFile(t, containerEngineMarkerPath(newRuntimePaths(deployment)), "{")

	if _, _, err := RecordedContainerEngine(deployment); err == nil {
		t.Fatal("expected a corrupt record to be reported")
	}
}

//nolint:paralleltest // The tests replace process-wide PATH with fake binary shims.
func TestDetectContainerEngine_SelectsTheFirstInstalledEngine(t *testing.T) {
	const linuxDocker = `case "$1" in --version) printf 'Docker version 29.9.1\n' ;; ` +
		`*) printf 'linux 29.9.1\n' ;; esac`
	podmanFirst := []localinstall.ContainerEngine{
		localinstall.EnginePodman, localinstall.EngineDocker,
	}
	tests := []struct {
		name       string
		preference []localinstall.ContainerEngine
		podman     bool
		docker     string
		expected   localinstall.ContainerEngine
	}{
		{name: "nothing installed", preference: podmanFirst, expected: localinstall.EnginePodman},
		{
			name: "both installed", preference: podmanFirst, podman: true, docker: linuxDocker,
			expected: localinstall.EnginePodman,
		},
		{
			name: "both installed, docker preferred",
			preference: []localinstall.ContainerEngine{
				localinstall.EngineDocker, localinstall.EnginePodman,
			},
			podman: true, docker: linuxDocker, expected: localinstall.EngineDocker,
		},
		{
			name: "only docker", preference: podmanFirst, docker: linuxDocker,
			expected: localinstall.EngineDocker,
		},
		{
			name: "only podman's docker command", preference: podmanFirst,
			docker:   `printf 'podman version 5.4.0\n'`,
			expected: localinstall.EnginePodman,
		},
		{
			name: "only docker with windows containers", preference: podmanFirst,
			docker: `case "$1" in --version) printf 'Docker version 29.9.1\n' ;; ` +
				`*) printf 'windows\n' ;; esac`,
			expected: localinstall.EnginePodman,
		},
		{
			name: "only docker with an unreachable daemon", preference: podmanFirst,
			docker: `case "$1" in --version) printf 'Docker version 29.9.1\n' ;; ` +
				`*) exit 1 ;; esac`,
			expected: localinstall.EngineDocker,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := newIsolatedShimDir(t)
			if test.podman {
				dropShim(t, dir, "podman", `exit 0`)
			}
			if test.docker != "" {
				dropShim(t, dir, "docker", test.docker)
			}

			engine := DetectContainerEngine(context.Background(), test.preference)
			if engine != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, engine)
			}
		})
	}
}

func assertRecordedEngine(
	t *testing.T,
	deployment config.DeploymentDir,
	expected localinstall.ContainerEngine,
) {
	t.Helper()
	engine, recorded, err := RecordedContainerEngine(deployment)
	if err != nil || !recorded || engine != expected {
		t.Fatalf("expected recorded engine %q, got %q (recorded=%t, err=%v)",
			expected, engine, recorded, err)
	}
}
