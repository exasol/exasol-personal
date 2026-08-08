// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
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

func TestStatusUnsafe_ReportsNotInitializedForMissingDirectory(t *testing.T) {
	t.Parallel()

	// Given: an uninitialized deployment directory.
	deployment := config.NewDeploymentDir(t.TempDir())

	// When: status is requested via the unsafe (unlocked) path.
	status, err := StatusUnsafe(context.Background(), deployment)
	// Then: the status object reports not initialized for the active deployment directory.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if status.Status != StatusNotInitialized {
		t.Fatalf("expected status %q, got %q", StatusNotInitialized, status.Status)
	}
	if status.DeploymentDir != deployment.Root() {
		t.Fatalf("expected deployment dir %q, got %q", deployment.Root(), status.DeploymentDir)
	}
}

// TestLocalVMStoppedStatus_UnresolvableRunnerReturnsNil exercises
// localVMStoppedStatus's real newResourceManager()/Status() path for a local
// deployment. The embedded exasol-local-runner artifact only exists for
// darwin/arm64 (see assets/resources/resources.yaml), so on any other
// platform Status() fails to resolve it and localVMStoppedStatus must
// degrade to nil rather than propagate that error, exactly as it does for a
// genuinely offline VM daemon.
func TestLocalVMStoppedStatus_UnresolvableRunnerReturnsNil(t *testing.T) {
	t.Parallel()

	// Given: a platform where the embedded local runner cannot be resolved.
	if localRunnerResolvesOnThisPlatform(t) {
		t.Skip("the embedded runner resolves on this platform; this covers the failure path")
	}

	deployment := newLocalTestDeployment(t)

	// Then: the stopped-status check degrades to nil instead of propagating
	// the resolution error.
	if got := localVMStoppedStatus(context.Background(), deployment); got != nil {
		t.Fatalf("expected nil when the local runner can't be resolved, got %+v", got)
	}
}

func TestLocalVMStoppedStatus_NonLocalDeploymentReturnsNil(t *testing.T) {
	t.Parallel()

	// Given: a deployment backed by a non-local infrastructure preset.
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	// Then: the stopped-status check returns nil because the deployment
	// is not local.
	if got := localVMStoppedStatus(context.Background(), deployment); got != nil {
		t.Fatalf("expected nil for a non-local deployment, got %+v", got)
	}
}

func TestStaleOperationInProgressMessage_DeployOperation(t *testing.T) {
	t.Parallel()

	// When: the stale in-progress message is built for a deploy operation.
	msg := staleOperationInProgressMessage(config.DeployOperation)
	// Then: it includes deploy-specific guidance.
	if !strings.Contains(msg, "previous deploy operation") {
		t.Fatalf("expected deploy-specific guidance, got %q", msg)
	}
}

func TestStaleOperationInProgressMessage_UnknownOperationFallsBackToGenericGuidance(t *testing.T) {
	t.Parallel()

	// When: the stale in-progress message is built for an unrecognized operation.
	msg := staleOperationInProgressMessage("start")
	// Then: it falls back to generic guidance that still names the operation.
	if !strings.Contains(msg, "previous start operation") {
		t.Fatalf("expected the operation name to be included, got %q", msg)
	}
}

func TestBuildInterruptMessage_DeployOperation(t *testing.T) {
	t.Parallel()

	// When: the interrupt message is built for a deploy operation.
	msg := buildInterruptMessage(config.DeployOperation)
	// Then: it includes deploy-specific guidance.
	if !strings.Contains(msg, "Please run `deploy`") {
		t.Fatalf("expected deploy-specific guidance, got %q", msg)
	}
}

func TestBuildInterruptMessage_DefaultOperation(t *testing.T) {
	t.Parallel()

	// When: the interrupt message is built for an unrecognized operation.
	msg := buildInterruptMessage("start")
	// Then: it falls back to generic start/stop guidance.
	if !strings.Contains(msg, "Please run `start` or `stop`") {
		t.Fatalf("expected generic start/stop guidance, got %q", msg)
	}
}

func TestStatusFromLockError_ContextCanceledPropagates(t *testing.T) {
	t.Parallel()

	// When: a lock error is translated for a canceled context.
	status, err := statusFromLockError(context.Canceled)
	// Then: no status is returned and the canceled error propagates.
	if status != nil {
		t.Fatalf("expected no status for a canceled context, got %+v", status)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the canceled error to propagate, got %v", err)
	}
}

func TestStatusFromLockError_UnknownErrorPropagates(t *testing.T) {
	t.Parallel()

	// Given: an error unrelated to context cancellation.
	unexpected := errors.New("boom")

	// When: a lock error is translated for an unexpected error.
	status, err := statusFromLockError(unexpected)
	// Then: no status is returned and the unexpected error propagates unchanged.
	if status != nil {
		t.Fatalf("expected no status for an unexpected error, got %+v", status)
	}
	if !errors.Is(err, unexpected) {
		t.Fatalf("expected the unexpected error to propagate, got %v", err)
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
