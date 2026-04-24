// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
)

func TestWriteLocalArtifacts_WritesDeploymentInfoAndSecrets(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	if err := writeInitializedStateWithoutVersionChecks(
		deployment,
		"0.0.0",
		"local-deployment-id",
		"cluster-identity",
	); err != nil {
		t.Fatalf("failed to write initial state: %v", err)
	}

	runtime := localruntime.New(deploymentDir)
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
	}); err != nil {
		t.Fatalf("failed to save local runtime state: %v", err)
	}

	// When
	err := writeLocalArtifacts(deployment, runtime, localClusterStateRunning, StatusRunning)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		t.Fatalf("failed to read deployment info: %v", err)
	}
	if info.Backend != config.DeploymentBackendLocal {
		t.Fatalf("expected backend %q, got %q", config.DeploymentBackendLocal, info.Backend)
	}
	if info.Runtime == nil || info.Runtime.DBPort != 8563 || info.Runtime.UIPort != 8443 {
		t.Fatalf("unexpected local runtime details: %#v", info.Runtime)
	}

	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		t.Fatalf("failed to read local secrets: %v", err)
	}
	if secrets.DbPassword != localDefaultDatabasePassword {
		t.Fatalf(
			"expected db password %q, got %q",
			localDefaultDatabasePassword,
			secrets.DbPassword,
		)
	}
	if secrets.AdminUiPassword != localDefaultAdminUIPassword {
		t.Fatalf(
			"expected admin UI password %q, got %q",
			localDefaultAdminUIPassword,
			secrets.AdminUiPassword,
		)
	}
}

func TestWaitForLocalRuntimeStarted_FailsWhenRunnerIsInactive(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	if err := writeInitializedStateWithoutVersionChecks(
		deployment,
		"0.0.0",
		"local-deployment-id",
		"cluster-identity",
	); err != nil {
		t.Fatalf("failed to write initial state: %v", err)
	}

	runtime := localruntime.New(deploymentDir)
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
	}); err != nil {
		t.Fatalf("failed to save local runtime state: %v", err)
	}
	if err := writeLocalArtifacts(
		deployment,
		runtime,
		localClusterStateStarting,
		StatusOperationInProgress,
	); err != nil {
		t.Fatalf("failed to write local artifacts: %v", err)
	}

	// When
	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := waitForLocalRuntimeStarted(waitCtx, deployment, runtime)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), runtime.Layout().RunnerLogFile()) {
		t.Fatalf("expected runner log hint, got %v", err)
	}
}

func TestWaitForLocalRuntimeStarted_SucceedsWhenDatabaseProbePasses(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	if err := writeInitializedStateWithoutVersionChecks(
		deployment,
		"0.0.0",
		"local-deployment-id",
		"cluster-identity",
	); err != nil {
		t.Fatalf("failed to write initial state: %v", err)
	}

	runtime := localruntime.New(deploymentDir)
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
	}); err != nil {
		t.Fatalf("failed to save local runtime state: %v", err)
	}
	if err := writeLocalArtifacts(
		deployment,
		runtime,
		localClusterStateStarting,
		StatusOperationInProgress,
	); err != nil {
		t.Fatalf("failed to write local artifacts: %v", err)
	}

	originalVerifyDatabaseConnection := verifyDatabaseConnectionFn
	verifyDatabaseConnectionFn = func(context.Context, config.DeploymentDir) error {
		return nil
	}
	t.Cleanup(func() {
		verifyDatabaseConnectionFn = originalVerifyDatabaseConnection
	})

	if err := waitForLocalRuntimeStarted(context.Background(), deployment, runtime); err != nil {
		t.Fatalf("expected database probe success to finish the wait, got %v", err)
	}
}

func TestWaitForLocalRuntimeStarted_ReturnsProbeErrorWhileRunnerIsActive(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	if err := writeInitializedStateWithoutVersionChecks(
		deployment,
		"0.0.0",
		"local-deployment-id",
		"cluster-identity",
	); err != nil {
		t.Fatalf("failed to write initial state: %v", err)
	}

	runtime := localruntime.New(deploymentDir)
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
	}); err != nil {
		t.Fatalf("failed to save local runtime state: %v", err)
	}
	if err := writeLocalArtifacts(
		deployment,
		runtime,
		localClusterStateStarting,
		StatusOperationInProgress,
	); err != nil {
		t.Fatalf("failed to write local artifacts: %v", err)
	}
	if err := runtime.WriteRunnerPID(os.Getpid()); err != nil {
		t.Fatalf("failed to write active runner pid: %v", err)
	}

	probeErr := errors.New("database still starting")
	originalVerifyDatabaseConnection := verifyDatabaseConnectionFn
	verifyDatabaseConnectionFn = func(context.Context, config.DeploymentDir) error {
		return probeErr
	}
	t.Cleanup(func() {
		verifyDatabaseConnectionFn = originalVerifyDatabaseConnection
	})

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := waitForLocalRuntimeStarted(waitCtx, deployment, runtime)
	if !errors.Is(err, probeErr) {
		t.Fatalf("expected probe error %v, got %v", probeErr, err)
	}
}

func TestLocalBackendStop_CleansUpWhenRunnerIsAlreadyGone(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	if err := writeInitializedStateWithoutVersionChecks(
		deployment,
		"0.0.0",
		"local-deployment-id",
		"cluster-identity",
	); err != nil {
		t.Fatalf("failed to write initial state: %v", err)
	}
	runtime := localruntime.New(deploymentDir)
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
	}); err != nil {
		t.Fatalf("failed to save local runtime state: %v", err)
	}
	if err := runtime.WriteRunnerPID(999999); err != nil {
		t.Fatalf("failed to write fake PID: %v", err)
	}

	// When
	err := (localBackend{}).Stop(context.Background(), deployment, nil, nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, readErr := runtime.ReadRunnerPID(); !errors.Is(
		readErr,
		localruntime.ErrRuntimeNotRunning,
	) {
		t.Fatalf("expected PID file cleanup, got %v", readErr)
	}
}

func TestLocalBackendDestroy_FailsSafeWhenRuntimeStillLooksActive(t *testing.T) {
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	runtime := localruntime.New(deploymentDir)
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("failed to prepare runtime root: %v", err)
	}

	originalRuntimeActive := localRuntimeActiveFn
	originalWaitInactive := localRuntimeWaitInactiveFn
	localRuntimeActiveFn = func(*localruntime.Runtime) (bool, int, error) {
		return true, 0, nil
	}
	localRuntimeWaitInactiveFn = func(_ *localruntime.Runtime, ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() {
		localRuntimeActiveFn = originalRuntimeActive
		localRuntimeWaitInactiveFn = originalWaitInactive
	})

	destroyCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := (localBackend{}).Destroy(destroyCtx, deployment, nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected destroy to fail with deadline, got %v", err)
	}

	if _, statErr := os.Stat(runtime.Layout().RuntimeRoot()); statErr != nil {
		t.Fatalf("expected runtime root to remain after failed destroy, got %v", statErr)
	}
}

func TestStopLocalRuntime_DoesNotCleanupStateWhileRuntimeStillActive(t *testing.T) {
	deploymentDir := t.TempDir()
	runtime := localruntime.New(deploymentDir)
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("failed to prepare runtime root: %v", err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	if err := runtime.WriteRunnerPID(cmd.Process.Pid); err != nil {
		t.Fatalf("failed to write runner pid: %v", err)
	}

	originalRuntimeActive := localRuntimeActiveFn
	originalWaitInactive := localRuntimeWaitInactiveFn
	localRuntimeActiveFn = func(*localruntime.Runtime) (bool, int, error) {
		return true, cmd.Process.Pid, nil
	}
	localRuntimeWaitInactiveFn = func(_ *localruntime.Runtime, _ context.Context) error {
		return context.DeadlineExceeded
	}
	t.Cleanup(func() {
		localRuntimeActiveFn = originalRuntimeActive
		localRuntimeWaitInactiveFn = originalWaitInactive
	})

	err := stopLocalRuntime(context.Background(), runtime)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected forced stop timeout, got %v", err)
	}

	if _, readErr := runtime.ReadRunnerPID(); readErr != nil {
		t.Fatalf("expected PID file to remain after failed stop, got %v", readErr)
	}
}
