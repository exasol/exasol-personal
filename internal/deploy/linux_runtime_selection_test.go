// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

//nolint:paralleltest // The fake Podman executable requires a process-wide PATH override.
func TestLocalStatusUsesLinuxPodmanRuntime(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment, state := newLinuxLocalWorkflowTestDeployment(t)
	installFakePodmanStatus(t, fakePodmanContainerMissing)
	ctx := testManagerContext(t)

	status := localVMStoppedStatus(ctx, deployment)
	if status == nil || status.Status != StatusStopped {
		t.Fatalf("expected stopped local status, got %#v", status)
	}

	if err := reconcileLocalVMState(ctx, state, deployment); err != nil {
		t.Fatalf("expected Linux reconciliation to succeed, got %v", err)
	}
	written, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read reconciled state: %v", err)
	}
	workflowState, err := written.GetWorkflowState()
	if err != nil {
		t.Fatalf("failed to decode reconciled state: %v", err)
	}
	if _, ok := workflowState.(*config.WorkflowStateStopped); !ok {
		t.Fatalf("expected stopped workflow state, got %T", workflowState)
	}
}

//nolint:paralleltest // The fake Podman executable requires a process-wide PATH override.
func TestLocalSLCRestartCheckUsesLinuxPodmanRuntime(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment, _ := newLinuxLocalWorkflowTestDeployment(t)
	installFakePodmanStatus(t, fakePodmanContainerRunning)

	if !isLocalDeploymentRunning(testManagerContext(t), deployment) {
		t.Fatal("expected SLC restart check to report the Podman container running")
	}
}

//nolint:paralleltest // The fake Podman executable requires a process-wide PATH override.
func TestDiagnoseLocalUsesLinuxPodmanRuntime(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment, _ := newLinuxLocalWorkflowTestDeployment(t)
	installFakePodmanStatus(t, fakePodmanContainerMissing)

	diagnostics, err := DiagnoseLocal(testManagerContext(t), deployment)
	if err != nil {
		t.Fatalf("expected Linux local diagnostics, got %v", err)
	}
	if !diagnostics.PlatformSupported {
		t.Fatalf("expected supported Linux platform, got %#v", diagnostics)
	}
	if diagnostics.VMRunning == nil || *diagnostics.VMRunning {
		t.Fatalf("expected compatibility status key to report stopped, got %#v", diagnostics)
	}
}

func requireLinuxLocalPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS != localLinuxOS ||
		(runtime.GOARCH != localLinuxAMD64 && runtime.GOARCH != localLinuxARM64) {
		t.Skip("test exercises the current-platform Linux runtime selector")
	}
}

func newLinuxLocalWorkflowTestDeployment(
	t *testing.T,
) (config.DeploymentDir, *config.ExasolPersonalState) {
	t.Helper()
	deployment := newLocalTestDeployment(t)
	state := &config.ExasolPersonalState{DeploymentId: "local-test"}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{}, deployment,
	); err != nil {
		t.Fatalf("failed to write Linux local workflow state: %v", err)
	}

	return deployment, state
}

type fakePodmanContainerState string

const (
	fakePodmanContainerMissing fakePodmanContainerState = "missing"
	fakePodmanContainerRunning fakePodmanContainerState = "running"
)

func installFakePodmanStatus(t *testing.T, state fakePodmanContainerState) {
	t.Helper()
	directory := t.TempDir()
	existsExit := "1"
	runningOutput := "false"
	if state == fakePodmanContainerRunning {
		existsExit = "0"
		runningOutput = "true"
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1 $2 $3\" = \"container exists exasol-db-local-test\" ]; then exit " +
		existsExit + "; fi\n" +
		"if [ \"$1 $2 $3 $5\" = \"container inspect --format exasol-db-local-test\" ]; then " +
		"echo " + runningOutput + "; exit 0; fi\n" +
		"echo \"unexpected podman command: $*\" >&2\nexit 64\n"
	podmanPath := filepath.Join(directory, "podman")
	//nolint:gosec // This test fixture must be executable.
	if err := os.WriteFile(podmanPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake podman: %v", err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}
