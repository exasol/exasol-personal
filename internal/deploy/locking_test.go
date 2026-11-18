// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/util"
)

func TestWithDeploymentExclusiveLockReturnsOperationInProgressMessage(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	writeWorkflowState(t, deploymentDir, &config.WorkflowStateOperationInProgress{
		Operation: config.DeployOperation,
	})

	mutex, err := directorymutex.New(deploymentDir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("acquire exclusive: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.Release()
	})

	// When
	err = withDeploymentExclusiveLock(
		context.Background(),
		deployment,
		func(config.DeploymentDir) error {
			return nil
		},
	)

	// Then
	if err == nil {
		t.Fatal("expected lock error, got nil")
	}
	if !strings.Contains(err.Error(), "Operation 'deploy' is currently in progress") {
		t.Fatalf("expected operation-in-progress message, got: %v", err)
	}
}

func TestWithDeploymentSharedLockReturnsGenericMessageWhenStatusNotInProgress(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	mutex, err := directorymutex.New(deploymentDir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("acquire exclusive: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.Release()
	})

	// When
	err = withDeploymentSharedLock(
		context.Background(),
		deployment,
		func(config.DeploymentDir) error {
			return nil
		},
	)

	// Then
	if err == nil {
		t.Fatal("expected lock error, got nil")
	}
	if err.Error() != lockUnavailableMessage {
		t.Fatalf("expected %q, got %q", lockUnavailableMessage, err.Error())
	}
}

func TestWithDeploymentSharedLockNamesAnOlderLauncherMarker(t *testing.T) {
	t.Parallel()

	// Given a marker file left by an older launcher version
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	marker := filepath.Join(deploymentDir, ".dirmutex.exclusive")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// When
	err := withDeploymentSharedLock(
		context.Background(),
		deployment,
		func(config.DeploymentDir) error {
			return nil
		},
	)

	// Then the error names the marker and how to clear it
	if err == nil {
		t.Fatal("expected lock error, got nil")
	}
	for _, want := range []string{"older exasol launcher version", marker, "exasol diag unlock"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %q", want, err.Error())
		}
	}
}

func TestWithDeploymentSharedLockPreservesContextCancellation(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	err := withDeploymentSharedLock(ctx, deployment, func(config.DeploymentDir) error {
		return nil
	})

	// Then
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

//nolint:paralleltest // The signal handler is process-global.
func TestWithDeploymentExclusiveLockHoldsTheLockThroughASignalUntilReturn(t *testing.T) {
	// Given an operation holding the exclusive lock
	util.StopSignalHandler()
	t.Cleanup(util.StopSignalHandler)
	signalChannel := make(chan os.Signal, 1)
	signalHandled := make(chan struct{})
	util.StartSignalHandlerWithChannel(signalChannel, func(os.Signal) { close(signalHandled) })

	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	mutex, err := directorymutex.New(deploymentDir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}

	acquired := make(chan struct{})
	releaseCallback := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- withDeploymentExclusiveLock(
			context.Background(),
			deployment,
			func(config.DeploymentDir) error {
				close(acquired)
				<-releaseCallback

				return nil
			},
		)
	}()
	<-acquired

	// When the process is interrupted
	signalChannel <- syscall.SIGINT
	<-signalHandled

	// Then the operation keeps the lock until it returns
	requireLockState(t, mutex, true)
	close(releaseCallback)
	if err := <-lockDone; err != nil {
		t.Fatalf("withDeploymentExclusiveLock failed: %v", err)
	}
	requireLockState(t, mutex, false)
}

func writeWorkflowState(t *testing.T, deploymentDir string, workflowState any) {
	t.Helper()

	exasolState := &config.ExasolPersonalState{}
	deployment := config.NewDeploymentDir(deploymentDir)
	if err := exasolState.SetWorkflowStateAndWrite(workflowState, deployment); err != nil {
		t.Fatalf("write workflow state: %v", err)
	}
}

func requireLockState(t *testing.T, mutex *directorymutex.DirectoryMutex, locked bool) {
	t.Helper()

	status, err := mutex.Status()
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Locked != locked {
		t.Fatalf("expected locked=%t, got %+v", locked, status)
	}
}
