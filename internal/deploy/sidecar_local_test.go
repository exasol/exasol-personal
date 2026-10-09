// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func TestSidecarCleanupFailureStillStopsDatabase(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	require.NoError(t, writeLocalDeploymentArtifacts(operations.deployment,
		&localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}))
	if _, err := operations.enable(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	selected.manager.definitions["second"] = sidecar.Container{Name: "second", Image: "caddy:2"}
	selected.manager.removeError = errors.New("cleanup failed")
	// When
	err := stopLocalRuntime(context.Background(), selected, nil, nil)
	sidecarErr, databaseErr := splitSidecarFailure(err)
	// Then
	if sidecarErr == nil || databaseErr != nil || selected.running || selected.stopCalls != 1 ||
		len(selected.manager.removeCalls) != 2 {
		t.Fatalf("stop result: sidecars=%v database=%v stopped=%d cleanup=%v",
			sidecarErr, databaseErr, selected.stopCalls, selected.manager.removeCalls)
	}
}

func TestLocalRuntimeArtifactsKeepCurrentCredentials(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	endpoint := &localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}
	require.NoError(t, writeLocalDeploymentArtifacts(operations.deployment, endpoint))
	if err := config.WriteSecrets(operations.deployment.Root(), &config.Secrets{
		DbPassword: "disposable-rotated-password",
	}); err != nil {
		t.Fatal(err)
	}
	// When
	err := writeLocalDeploymentArtifacts(operations.deployment, endpoint)
	sources, readErr := config.SidecarSources(operations.deployment)
	// Then
	if err != nil || readErr != nil ||
		sources[sidecar.DatabaseSource]["password"] != "disposable-rotated-password" {
		t.Fatalf("credential refresh: %v, %v", err, readErr)
	}
}

func TestSidecarStartupRunsBeforeDatabaseWait(t *testing.T) {
	// Given
	t.Setenv(localSkipDatabaseWaitEnv, "1")
	operations, selected := newSidecarOperationsFixture(t)
	materializeTestSidecar(t, operations.deployment, operations.catalog)
	selected.manager.startError = errors.New("sidecar example: image unavailable")
	endpoint := &localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}
	// When
	err := writeLocalRuntimeArtifactsAndWait(context.Background(), selected, endpoint, 0, nil, nil)
	sidecarErr, databaseErr := splitSidecarFailure(err)
	// Then
	if selected.manager.ensureCalls != 1 || sidecarErr == nil || databaseErr != nil {
		t.Fatalf("startup without SQL wait: %d, %v, %v",
			selected.manager.ensureCalls, sidecarErr, databaseErr)
	}
	if selected.manager.sources[sidecar.DatabaseSource]["password"] != localDBPassword {
		t.Fatal("connection sources were unavailable at container startup")
	}
}

func TestSidecarDestroyRemovesObservedState(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	// When
	err := destroyLocalRuntime(context.Background(), selected, nil, nil)
	_, stateErr := os.Stat(operations.deployment.SidecarStatePath())
	document, readErr := config.ReadSidecars(operations.deployment)
	// Then
	if err != nil || len(selected.manager.definitions) != 0 ||
		!errors.Is(stateErr, os.ErrNotExist) {
		t.Fatalf("destroyed sidecar state: %v, %v", err, stateErr)
	}
	if readErr != nil || len(document.Containers) != 1 {
		t.Fatalf("desired configuration after destroy: %+v, %v", document, readErr)
	}
}
