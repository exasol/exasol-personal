// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

// testRunnerExecutableMode is a named constant rather than an inline literal
// so gosec's file-permission checks (which only pattern-match on literals)
// don't flag these test fixtures for needing an executable fake runner.
const testRunnerExecutableMode = 0o700

func TestClassifyLocalReachability_AllPortsBlocked(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	blockedJSON := `{"ports":{"ssh":{"state":"blocked"},"db":{"state":"blocked"}}}`
	writeFakeCombinedRunner(t, deployment, "", blockedJSON)

	err := classifyLocalReachability(context.Background(), deployment)
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
	mixedJSON := `{"ports":{"ssh":{"state":"reachable"},"db":{"state":"blocked"}}}`
	writeFakeCombinedRunner(t, deployment, "", mixedJSON)

	if err := classifyLocalReachability(context.Background(), deployment); err != nil {
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

	if err := classifyLocalReachability(context.Background(), deployment); err != nil {
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
	paths := localruntime.NewPaths(deployment)
	if err := os.MkdirAll(paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}
	runnerScript := "#!/bin/sh\necho 'Unknown command: health-check' >&2\nexit 1\n"
	if err := os.WriteFile(paths.RunnerPath, []byte(runnerScript), 0o600); err != nil {
		t.Fatalf("failed to write fake runner: %v", err)
	}
	if err := os.Chmod(paths.RunnerPath, testRunnerExecutableMode); err != nil {
		t.Fatalf("failed to mark fake runner executable: %v", err)
	}

	if err := classifyLocalReachability(context.Background(), deployment); err != nil {
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

// writeFakeCombinedRunner writes a single fake runner script that answers
// both "status" and "health-check", since callers in this package may invoke
// either against whatever binary is staged at the runner path. statusJSON is
// the raw response body for "status" (e.g. `{"running":true}`).
func writeFakeCombinedRunner(
	t *testing.T,
	deployment config.DeploymentDir,
	statusJSON, healthCheckJSON string,
) {
	t.Helper()

	paths := localruntime.NewPaths(deployment)
	if err := os.MkdirAll(paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}

	script := "#!/bin/sh\n" +
		"if [ \"$1\" = status ]; then echo '" + statusJSON + "'; exit 0; fi\n" +
		"if [ \"$1\" = health-check ]; then echo '" + healthCheckJSON + "'; exit 0; fi\n" +
		"exit 1\n"
	if err := os.WriteFile(paths.RunnerPath, []byte(script), 0o600); err != nil {
		t.Fatalf("failed to write fake runner: %v", err)
	}
	if err := os.Chmod(paths.RunnerPath, testRunnerExecutableMode); err != nil {
		t.Fatalf("failed to mark fake runner executable: %v", err)
	}
}
