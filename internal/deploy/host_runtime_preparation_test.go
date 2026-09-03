// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

// These tests pin the reason preparation is hoisted ahead of the
// OperationInProgress write: a host that cannot be prepared must leave the
// deployment in its previous, retryable state rather than stranded
// mid-operation. Emptying PATH removes Podman, which is the cheapest way to
// make Linux host preparation fail for real.

func assertWorkflowState[T any](t *testing.T, deployment config.DeploymentDir) {
	t.Helper()
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read workflow state: %v", err)
	}
	workflowState, err := state.GetWorkflowState()
	if err != nil {
		t.Fatalf("failed to decode workflow state: %v", err)
	}
	if _, ok := workflowState.(T); !ok {
		var want T
		t.Fatalf("expected workflow state %T, got %T", want, workflowState)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH for host preparation.
func TestDeployPreparationFailurePreservesInitializedWorkflowState(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment := newLocalTestDeployment(t)
	t.Setenv("PATH", t.TempDir())

	err := deployLocked(testResolverContext(t), deployment, false, DeployOptions{})

	if err == nil || !strings.Contains(err.Error(), "'podman' is required") {
		t.Fatalf("expected a Podman preparation failure, got %v", err)
	}
	assertWorkflowState[*config.WorkflowStateInitialized](t, deployment)
}

//nolint:paralleltest // The test replaces process-wide PATH for host preparation.
func TestStartPreparationFailurePreservesStoppedWorkflowState(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment := newLocalTestDeployment(t)
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read workflow state: %v", err)
	}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateStopped{}, deployment,
	); err != nil {
		t.Fatalf("failed to write stopped workflow state: %v", err)
	}
	t.Setenv("PATH", t.TempDir())

	err = startLocked(testResolverContext(t), deployment, false, StartOptions{})

	if err == nil || !strings.Contains(err.Error(), "'podman' is required") {
		t.Fatalf("expected a Podman preparation failure, got %v", err)
	}
	assertWorkflowState[*config.WorkflowStateStopped](t, deployment)
}
