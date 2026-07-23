// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
)

func TestStatus_IncludesDeploymentDirInStatusObject(t *testing.T) {
	t.Parallel()

	// Given: an uninitialized deployment directory.
	deployment := config.NewDeploymentDir(t.TempDir())

	// When: status is requested.
	status, err := Status(context.Background(), deployment)
	// Then: the status object includes the active deployment directory and status.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if status.DeploymentDir != deployment.Root() {
		t.Fatalf("expected deployment dir %q, got %q", deployment.Root(), status.DeploymentDir)
	}
	if status.Status != StatusNotInitialized {
		t.Fatalf("expected status %q, got %q", StatusNotInitialized, status.Status)
	}
}

func TestStatus_ReportsNotInitializedForMissingDirectory(t *testing.T) {
	t.Parallel()

	// Given: a deployment directory path that does not exist.
	deployment := config.NewDeploymentDir(t.TempDir() + "/missing")

	// When: status is requested.
	status, err := Status(context.Background(), deployment)
	// Then: status reports not initialized instead of failing.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if status.Status != StatusNotInitialized {
		t.Fatalf("expected status %q, got %q", StatusNotInitialized, status.Status)
	}
}

func TestStatus_ReportsOperationInProgressWhenLockedBeforeStateFileExists(t *testing.T) {
	t.Parallel()

	// Given: an existing deployment directory locked exclusively before init writes state.
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		t.Fatalf("expected mutex creation to succeed, got: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("expected exclusive lock to succeed, got: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.ReleaseExclusive(context.Background())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// When: status is requested while the init lock is still held.
	status, err := Status(ctx, deployment)
	// Then: the deployment is reported as having an operation in progress.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if status.Status != StatusOperationInProgress {
		t.Fatalf("expected status %q, got %q", StatusOperationInProgress, status.Status)
	}
	if status.Message == "" {
		t.Fatal("expected operation-in-progress message, got empty message")
	}
}

func TestStatus_ReportsStaleDestroyOperationWithRecoveryGuidance(t *testing.T) {
	t.Parallel()

	// Given: a deployment directory whose previous destroy failed after setting
	// operation-in-progress, but no process currently holds the deployment lock.
	deployment := config.NewDeploymentDir(t.TempDir())
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateOperationInProgress{Operation: config.DestroyOperation},
		deployment,
	); err != nil {
		t.Fatalf("write workflow state failed: %v", err)
	}

	// When: status is requested.
	status, err := Status(context.Background(), deployment)
	// Then: the message points to retry destroy or local-only removal.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if status.Status != StatusOperationInProgress {
		t.Fatalf("expected status %q, got %q", StatusOperationInProgress, status.Status)
	}
	if !strings.Contains(status.Message, "run `destroy` again") {
		t.Fatalf("expected destroy retry guidance, got %q", status.Message)
	}
	if !strings.Contains(status.Message, "run `remove`") {
		t.Fatalf("expected remove guidance, got %q", status.Message)
	}
}
