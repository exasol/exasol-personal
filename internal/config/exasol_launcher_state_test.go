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

func expectPanic(t *testing.T, action func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid workflow state, got none")
		}
	}()

	action()
}

func mustChmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod dir failed: %v", err)
	}
}

// assertWorkflowStateRoundtrip writes state to a fresh deployment dir, reads
// it back, and checks that GetWorkflowState reports the same concrete type.
func assertWorkflowStateRoundtrip[T any](t *testing.T, state T) {
	t.Helper()
	deployment := NewDeploymentDir(t.TempDir())

	exasolState := &ExasolPersonalState{}
	//nolint:errcheck,gosec,revive // error checked in subsequent read
	exasolState.SetWorkflowStateAndWrite(state, deployment)

	newExasolState, err := ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read exasol personal state: %v", err)
	}

	workflowState, err := newExasolState.GetWorkflowState()
	expectNoErr(t, err)

	if _, ok := workflowState.(T); !ok {
		t.Fatalf("expected %T, got %T", state, workflowState)
	}
}

func TestWorkflowState(t *testing.T) {
	t.Parallel()

	t.Run("Invalid state panics", func(t *testing.T) {
		t.Parallel()
		exasolState := &ExasolPersonalState{}
		expectPanic(t, func() {
			//nolint:errcheck,gosec // intentionally testing panic behavior
			exasolState.SetWorkflowState(struct{ X int }{X: 1})
		})
	})

	t.Run("Write error (non-writable dir)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deployment := NewDeploymentDir(dir)
		//nolint:gosec // remove write bit to force writeConfig error
		mustChmod(t, dir, 0o600)
		//nolint:gosec
		defer os.Chmod(dir, 0o700)

		exasolState := &ExasolPersonalState{}
		expectErr(t, WriteExasolPersonalState(exasolState, deployment))
	})

	t.Run("Initialized", func(t *testing.T) {
		t.Parallel()
		assertWorkflowStateRoundtrip(t, &WorkflowStateInitialized{})
	})

	t.Run("OperationInProgress", func(t *testing.T) {
		t.Parallel()
		assertWorkflowStateRoundtrip(t, &WorkflowStateOperationInProgress{})
	})

	t.Run("Running", func(t *testing.T) {
		t.Parallel()
		assertWorkflowStateRoundtrip(t, &WorkflowStateRunning{})
	})

	t.Run("Stopped", func(t *testing.T) {
		t.Parallel()
		assertWorkflowStateRoundtrip(t, &WorkflowStateStopped{})
	})

	t.Run("Interrupted", func(t *testing.T) {
		t.Parallel()
		assertWorkflowStateRoundtrip(t, &WorkflowStateInterrupted{
			Error:                      "e",
			InterruptedDuringOperation: StopOperation,
		})
	})

	t.Run("DeploymentFailed", func(t *testing.T) {
		t.Parallel()
		assertWorkflowStateRoundtrip(t, &WorkflowStateDeploymentFailed{
			Error: "f",
		})
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
		if !errors.Is(err, ErrNoWorkflowStateSet) {
			t.Fatalf("expected ErrNoWorkflowStateSet, got: %v", err)
		}
	})
}
