// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/util"
)

func TestWithDeploymentExclusiveLockReturnsOperationInProgressMessage(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
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
		_ = mutex.ReleaseExclusive(context.Background())
	})

	// When
	err = withDeploymentExclusiveLock(context.Background(), deploymentDir, func(string) error {
		return nil
	})

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
	mutex, err := directorymutex.New(deploymentDir)
	if err != nil {
		t.Fatalf("new mutex: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("acquire exclusive: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.ReleaseExclusive(context.Background())
	})

	// When
	err = withDeploymentSharedLock(context.Background(), deploymentDir, func(string) error {
		return nil
	})

	// Then
	if err == nil {
		t.Fatal("expected lock error, got nil")
	}
	if err.Error() != lockUnavailableMessage {
		t.Fatalf("expected %q, got %q", lockUnavailableMessage, err.Error())
	}
}

func TestWithDeploymentSharedLockPreservesContextCancellation(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	err := withDeploymentSharedLock(ctx, deploymentDir, func(string) error {
		return nil
	})

	// Then
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

// nolint:paralleltest
func TestWithDeploymentExclusiveLockReleasesOnSignal(t *testing.T) {
	// Given
	util.StopSignalHandler()
	t.Cleanup(func() {
		util.StopSignalHandler()
	})

	signalChannel := make(chan os.Signal, 1)
	util.StartSignalHandlerWithChannel(signalChannel, func(os.Signal) {})

	deploymentDir := t.TempDir()
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
			deploymentDir,
			func(string) error {
				close(acquired)
				<-releaseCallback

				return nil
			},
		)
	}()

	<-acquired

	// When
	signalChannel <- syscall.SIGINT

	// Then
	waitForUnlocked(t, mutex, 2*time.Second)

	close(releaseCallback)
	if err := <-lockDone; err != nil {
		t.Fatalf("withDeploymentExclusiveLock failed: %v", err)
	}
}

func writeWorkflowState(t *testing.T, deploymentDir string, workflowState any) {
	t.Helper()

	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowStateAndWrite(workflowState, deploymentDir); err != nil {
		t.Fatalf("write workflow state: %v", err)
	}
}

func waitForUnlocked(t *testing.T, mutex *directorymutex.DirectoryMutex, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := mutex.Status()
		if err != nil {
			t.Fatalf("status failed: %v", err)
		}
		if !status.Locked {
			return
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("expected lock to be released after signal")
}
