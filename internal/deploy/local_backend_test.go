// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
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
	err := writeLocalArtifacts(deploymentDir, localClusterStateRunning, StatusRunning)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	info, err := config.ReadLocalDeploymentInfo(deploymentDir)
	if err != nil {
		t.Fatalf("failed to read local deployment info: %v", err)
	}
	if info.Backend != config.DeploymentBackendLocal {
		t.Fatalf("expected backend %q, got %q", config.DeploymentBackendLocal, info.Backend)
	}
	if info.Local == nil || info.Local.DBPort != 8563 || info.Local.UIPort != 8443 {
		t.Fatalf("unexpected local runtime details: %#v", info.Local)
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
		deploymentDir,
		localClusterStateStarting,
		StatusOperationInProgress,
	); err != nil {
		t.Fatalf("failed to write local artifacts: %v", err)
	}

	// When
	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := waitForLocalRuntimeStarted(waitCtx, deploymentDir, runtime)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), runtime.Layout().RunnerLogFile()) {
		t.Fatalf("expected runner log hint, got %v", err)
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
