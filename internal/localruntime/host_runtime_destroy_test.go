// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/localinstall"
)

// fakeDestroyDockerScript answers the commands a Docker destroy runs. The
// cleanup container restores write access to the directory it was given, the
// way a container running as root can delete what the launcher user cannot.
const fakeDestroyDockerScript = `#!/bin/sh
scenario_dir=$1
shift
printf '%s\n' "$*" >> "$scenario_dir/commands"
case "$2" in
  run)
    if [ -f "$scenario_dir/fail-cleanup" ]; then
      printf 'cannot pull the cleanup image\n' >&2
      exit 1
    fi
    for argument in "$@"; do
      case "$argument" in
        *:/data*) chmod -R u+w "${argument%%:/data*}" ;;
      esac
    done
    ;;
esac
exit 0
`

// givenRootOwnedQuarantine stands in for data a rootful engine left behind in
// a directory recovery set aside, which the launcher user cannot delete.
func givenRootOwnedQuarantine(t *testing.T, paths runtimePaths) string {
	t.Helper()
	quarantine := filepath.Join(paths.WorkDir, nanoDataDirName+".failed-20261028000000")
	protected := filepath.Join(quarantine, "data")
	writeLinuxRuntimeTestFile(t, filepath.Join(protected, "db-file"), "owned by root")
	//nolint:gosec // directory mode under test
	if err := os.Chmod(protected, 0o555); err != nil {
		t.Fatalf("failed to protect quarantined data: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(protected, 0o750) }) //nolint:gosec // restores the fixture

	return quarantine
}

func newDockerDestroyRuntime(t *testing.T) (*HostRuntime, string) {
	t.Helper()
	skipPOSIXPermissionTest(t)
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the directory permission this test relies on")
	}
	deployment := newLinuxRuntimeStatusDeployment(t)
	scenarioDir := t.TempDir()
	scriptPath := filepath.Join(t.TempDir(), "docker.sh")
	writeLinuxRuntimeTestFile(t, scriptPath, fakeDestroyDockerScript)
	runtime := newHostRuntime(
		deployment, nil, stubContainerHostPreparer{running: true}, localinstall.EngineDocker,
	)
	runtime.runtimeExec = []string{"/bin/sh", scriptPath, scenarioDir}
	if err := os.MkdirAll(runtime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create the runtime directory: %v", err)
	}
	if err := recordContainerEngine(runtime.paths, localinstall.EngineDocker); err != nil {
		t.Fatalf("failed to record the engine: %v", err)
	}
	writeLinuxRuntimeTestFile(
		t, filepath.Join(runtime.paths.WorkDir, nanoDataDirName, "exasol.conf"), "data",
	)

	return runtime, scenarioDir
}

// Regression: destroy cleared only the live data directory, so data recovery
// set aside kept a rootful Docker deployment from being destroyed.
func TestHostDestroy_ClearsQuarantinedDataThroughTheEngine(t *testing.T) {
	t.Parallel()
	runtime, scenarioDir := newDockerDestroyRuntime(t)
	quarantine := givenRootOwnedQuarantine(t, runtime.paths)

	err := runtime.Destroy(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("expected destroy to succeed, got %v", err)
	}
	if _, statErr := os.Stat(runtime.paths.Root); !os.IsNotExist(statErr) {
		t.Fatalf("expected the runtime directory to be removed, got %v", statErr)
	}
	commands, readErr := os.ReadFile(filepath.Join(scenarioDir, "commands"))
	if readErr != nil || !strings.Contains(string(commands), quarantine+":/data") {
		t.Fatalf("expected the cleanup container to clear %s, got %q, %v",
			quarantine, commands, readErr)
	}
}

// Regression: a failed cleanup could delete the engine record first, and the
// retry then treated the deployment as a Podman deployment.
func TestHostDestroy_KeepsTheEngineRecordWhenCleanupFails(t *testing.T) {
	t.Parallel()
	runtime, scenarioDir := newDockerDestroyRuntime(t)
	givenRootOwnedQuarantine(t, runtime.paths)
	writeLinuxRuntimeTestFile(t, filepath.Join(scenarioDir, "fail-cleanup"), "")

	err := runtime.Destroy(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected destroy to fail while the cleanup container cannot run")
	}
	engine, recorded, recordErr := RecordedContainerEngine(runtime.deployment)
	if recordErr != nil || !recorded || engine != localinstall.EngineDocker {
		t.Fatalf("expected the Docker record to survive, got %q recorded=%t err=%v",
			engine, recorded, recordErr)
	}
}

func TestRemoveRuntimeFiles_RemovesTheEngineRecordLast(t *testing.T) {
	t.Parallel()
	paths := newRuntimePaths(newLinuxRuntimeStatusDeployment(t))
	writeLinuxRuntimeTestFile(t, filepath.Join(paths.WorkDir, "bootstrap", "file"), "x")
	writeLinuxRuntimeTestFile(t, filepath.Join(paths.Root, "other", "file"), "x")
	writeLinuxRuntimeTestFile(t, containerEngineMarkerPath(paths), `{"engine":"docker"}`)

	if err := removeAllExcept(paths.Root, containerEngineMarkerPath(paths)); err != nil {
		t.Fatalf("expected removal to succeed, got %v", err)
	}
	entries, err := os.ReadDir(paths.WorkDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != containerEngineMarkerFileName {
		t.Fatalf("expected only the engine record to remain, got %v, %v", entries, err)
	}
	if err := removeRuntimeFiles(paths); err != nil {
		t.Fatalf("expected the runtime directory to be removed, got %v", err)
	}
	if _, err := os.Stat(paths.Root); !os.IsNotExist(err) {
		t.Fatalf("expected no runtime directory, got %v", err)
	}
}
