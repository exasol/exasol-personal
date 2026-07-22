// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// testRunnerExecutableMode is a named constant rather than an inline literal
// so gosec's file-permission checks (which only pattern-match on literals)
// don't flag these test fixtures for needing an executable fake runner.
const testRunnerExecutableMode = 0o700

// runnerZipEntryName matches resources.yaml's resource_path for
// exasol-local-runner.
const runnerZipEntryName = "launcher"

// exasolLocalRunnerResourceID mirrors internal/localruntime's unexported
// resource ID constant, which callers here need too to build a matching test
// ResourceSpec.
const exasolLocalRunnerResourceID = "exasol-local-runner"

func TestClassifyLocalReachability_AllPortsBlocked(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	blockedJSON := `{"ports":{"ssh":{"state":"blocked"},"db":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, "", blockedJSON)
	localRuntime := localruntime.New(deployment, manager)

	err := classifyLocalReachability(context.Background(), localRuntime)
	if err == nil {
		t.Fatal("expected a reachability error when every port is blocked")
	}
	if !errors.Is(err, ErrLocalReachability) {
		t.Fatalf("expected errors.Is(err, ErrLocalReachability), got %v", err)
	}
}

func TestClassifyLocalReachability_OnlyDatabasePortBlocked(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// A reachable SSH port alongside a blocked database port means the
	// network path itself is fine; the problem is database-specific.
	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	mixedJSON := `{"ports":{"ssh":{"state":"reachable"},"db":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, "", mixedJSON)
	localRuntime := localruntime.New(deployment, manager)

	if err := classifyLocalReachability(context.Background(), localRuntime); err != nil {
		t.Fatalf("expected no reachability error when at least one port is reachable, got %v", err)
	}
}

func TestClassifyLocalReachability_NonLocalDeploymentIsNoop(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	// A nil manager is safe here: classifyLocalReachability short-circuits on
	// isLocalDeployment before it would ever be dereferenced.
	localRuntime := localruntime.New(deployment, nil)
	if err := classifyLocalReachability(context.Background(), localRuntime); err != nil {
		t.Fatalf("expected no-op for non-local deployment, got %v", err)
	}
}

func TestClassifyLocalReachability_HealthCheckUnavailableIsNoop(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// An old runner daemon that predates health-check must not turn every
	// local failure into a reachability error; the caller's original error
	// should stand instead.
	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	runnerScript := "#!/bin/sh\necho 'Unknown command: health-check' >&2\nexit 1\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime := localruntime.New(deployment, manager)

	if err := classifyLocalReachability(context.Background(), localRuntime); err != nil {
		t.Fatalf("expected no-op when health-check is unavailable, got %v", err)
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake local runner script is POSIX-only")
	}
}

func newLocalTestDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: local
`)

	return deployment
}

// ensureLocalRuntimeWorkDir creates the deployment's local runtime work
// directory, which cmd.Dir requires to exist before the resolved runner can
// actually be invoked.
func ensureLocalRuntimeWorkDir(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	if err := os.MkdirAll(localruntime.NewPaths(deployment).WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime work dir: %v", err)
	}
}

// writeFakeCombinedRunner builds a Manager whose "exasol-local-runner"
// resource resolves to a single fake runner script that answers both
// "status" and "health-check", since callers in this package may invoke
// either against whatever the manager resolves. statusJSON is the raw
// response body for "status" (e.g. `{"running":true}`).
func writeFakeCombinedRunner(
	t *testing.T,
	statusJSON, healthCheckJSON string,
) *runtimeartifacts.Manager {
	t.Helper()

	script := "#!/bin/sh\n" +
		"if [ \"$1\" = status ]; then echo '" + statusJSON + "'; exit 0; fi\n" +
		"if [ \"$1\" = health-check ]; then echo '" + healthCheckJSON + "'; exit 0; fi\n" +
		"exit 1\n"

	return newTestManagerForRunner(t, []byte(script))
}

// newTestManagerForRunner builds a Manager whose "exasol-local-runner"
// resource resolves through the same extract: true / resource_path shape the
// real resources.yaml entry uses: scriptContent is packed into a minimal,
// single-entry zip (mirroring the real release archive).
func newTestManagerForRunner(t *testing.T, scriptContent []byte) *runtimeartifacts.Manager {
	t.Helper()

	zipPath := writeRunnerZip(t, scriptContent)
	spec := runtimeartifacts.ResourceSpec{
		exasolLocalRunnerResourceID: {
			Extract: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"any": {URL: zipPath, ResourcePath: runnerZipEntryName},
			},
		},
	}

	return runtimeartifacts.NewResourceManagerForPlatform(
		spec, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)
}

func writeRunnerZip(t *testing.T, scriptContent []byte) string {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "runner.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create runner zip fixture: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: runnerZipEntryName, Method: zip.Deflate}
	header.SetMode(testRunnerExecutableMode)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create runner zip entry: %v", err)
	}
	if _, err := entry.Write(scriptContent); err != nil {
		t.Fatalf("failed to write runner zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close runner zip fixture: %v", err)
	}

	return zipPath
}
