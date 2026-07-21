// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT
package config

import (
	"errors"
	"os"
	"testing"
)

func expectErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func expectNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowState(t *testing.T) {
	t.Parallel()

	t.Run("Invalid state panics", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for invalid workflow state, got none")
			}
		}()

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // intentionally testing panic behavior
		exasolState.SetWorkflowState(struct{ X int }{X: 1})
	})

	t.Run("Write error (non-writable dir)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)
		//nolint:gosec // remove write bit to force writeConfig error
		if err := os.Chmod(dir, 0o600); err != nil {
			t.Fatalf("chmod dir failed: %v", err)
		}
		//nolint:gosec
		defer os.Chmod(dir, 0o700)

		exasolState := &ExasolPersonalState{}
		expectErr(t, WriteExasolPersonalState(exasolState, deployment))
	})

	t.Run("Initialized", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		exasolState.SetWorkflowStateAndWrite(&WorkflowStateInitialized{}, NewDeploymentDir(dir))

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		workflowState, err := newExasolState.GetWorkflowState()

		expectNoErr(t, err)
		if _, ok := workflowState.(*WorkflowStateInitialized); !ok {
			t.Fatalf("expected Initialized, got %T", workflowState)
		}
	})

	t.Run("OperationInProgress", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		exasolState.SetWorkflowStateAndWrite(
			&WorkflowStateOperationInProgress{},
			deployment,
		)

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		workflowState, err := newExasolState.GetWorkflowState()

		expectNoErr(t, err)
		if _, ok := workflowState.(*WorkflowStateOperationInProgress); !ok {
			t.Fatalf("expected OperationInProgress, got %T", workflowState)
		}
	})

	t.Run("Running", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		exasolState.SetWorkflowStateAndWrite(
			&WorkflowStateRunning{},
			deployment,
		)

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		workflowState, err := newExasolState.GetWorkflowState()

		expectNoErr(t, err)
		if _, ok := workflowState.(*WorkflowStateRunning); !ok {
			t.Fatalf("expected Running, got %T", workflowState)
		}
	})

	t.Run("Stopped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		exasolState.SetWorkflowStateAndWrite(&WorkflowStateStopped{}, NewDeploymentDir(dir))

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		workflowState, err := newExasolState.GetWorkflowState()

		expectNoErr(t, err)
		if _, ok := workflowState.(*WorkflowStateStopped); !ok {
			t.Fatalf("expected Stopped, got %T", workflowState)
		}
	})

	t.Run("Interrupted", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		exasolState.SetWorkflowStateAndWrite(&WorkflowStateInterrupted{
			Error:                      "e",
			InterruptedDuringOperation: StopOperation,
		}, NewDeploymentDir(dir))

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		workflowState, err := newExasolState.GetWorkflowState()

		expectNoErr(t, err)
		if _, ok := workflowState.(*WorkflowStateInterrupted); !ok {
			t.Fatalf("expected Interrupted, got %T", workflowState)
		}
	})

	t.Run("DeploymentFailed", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		exasolState.SetWorkflowStateAndWrite(&WorkflowStateDeploymentFailed{
			Error: "f",
		}, NewDeploymentDir(dir))

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		workflowState, err := newExasolState.GetWorkflowState()

		expectNoErr(t, err)
		if _, ok := workflowState.(*WorkflowStateDeploymentFailed); !ok {
			t.Fatalf("expected DeploymentFailed, got %T", workflowState)
		}
	})

	t.Run("Missing file returns error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		_, err := ReadExasolPersonalState(NewDeploymentDir(dir))
		expectErr(t, err)
	})

	t.Run("No field set returns ErrNoWorkflowStateSet", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)

		exasolState := &ExasolPersonalState{}
		//nolint:errcheck,gosec // error checked in subsequent read
		WriteExasolPersonalState(exasolState, deployment)

		newExasolState, err := ReadExasolPersonalState(deployment)
		if err != nil {
			t.Fatalf("failed to read exasol personal state: %v", err)
		}

		_, err = newExasolState.GetWorkflowState()
		if err == nil {
			t.Fatal("expected ErrNoWorkflowStateSet, got nil")
		}
		if !errors.Is(err, ErrNoWorkflowStateSet) {
			t.Fatalf("expected ErrNoWorkflowStateSet, got: %v", err)
		}
	})
}
