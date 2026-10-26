// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
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
	listing := "[]"
	if state == fakePodmanContainerRunning {
		listing = `[{"id":"fake","names":["exasol-db-local-test"],"state":"running"}]`
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"ps\" ]; then printf '%s\\n' '" + listing + "'; exit 0; fi\n" +
		"echo \"unexpected podman command: $*\" >&2\nexit 64\n"
	podmanPath := filepath.Join(directory, "podman")
	//nolint:gosec // This test fixture must be executable.
	if err := os.WriteFile(podmanPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake podman: %v", err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Regression: an unanswering Docker daemon read as a stopped deployment, so
// `exasol stop` recorded it as stopped although Docker restarted it later.
//
//nolint:paralleltest // The fake Docker executable requires a process-wide PATH override.
func TestLocalStateKeepsRunningWhileDockerIsUnreachable(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment, state := newLinuxLocalWorkflowTestDeployment(t)
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: local
local:
  containerEngine: docker
`)
	installFakeExecutable(t, "docker",
		"#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n")
	ctx := testManagerContext(t)

	status := localVMStoppedStatus(ctx, deployment)
	if status == nil || status.Status != StatusDatabaseConnectionFailed ||
		status.Message != containerHostUnreachableMessage {
		t.Fatalf("expected the status to report an unreachable engine, got %#v", status)
	}

	err := reconcileLocalVMState(ctx, state, deployment)
	if !errors.Is(err, localruntime.ErrContainerHostUnreachable) {
		t.Fatalf("expected reconciliation to report the unreachable engine, got %v", err)
	}
	written, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	workflowState, err := written.GetWorkflowState()
	if err != nil {
		t.Fatalf("failed to decode state: %v", err)
	}
	if _, ok := workflowState.(*config.WorkflowStateRunning); !ok {
		t.Fatalf("expected the running workflow state to stay, got %T", workflowState)
	}
}

// Regression: a Docker daemon that stopped answering during `exasol stop` left
// the deployment recorded as stopped, so a retried stop skipped the database
// container that Docker restarted once its daemon was back.
//
//nolint:paralleltest // The fake Docker executable requires a process-wide PATH override.
func TestStopFailureKeepsStopRetryableWhileDockerIsUnreachable(t *testing.T) {
	requireLinuxLocalPlatform(t)
	deployment, state := newLinuxLocalWorkflowTestDeployment(t)
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: local
local:
  containerEngine: docker
`)
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateOperationInProgress{Operation: config.StopOperation}, deployment,
	); err != nil {
		t.Fatalf("failed to write workflow state: %v", err)
	}
	installFakeExecutable(t, "docker",
		"#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n")
	cause := errors.New("failed to remove the database container")

	failure := recordLifecycleFailure(
		testManagerContext(t), state, deployment, config.StopOperation,
		&config.WorkflowStateStopped{}, cause,
	)

	if !errors.Is(failure, cause) {
		t.Fatalf("expected the stop failure to be reported, got %v", failure)
	}
	written, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	decision, err := workflowStatePermitsStop(written)
	if err != nil {
		t.Fatalf("failed to decide whether stop may run: %v", err)
	}
	if !decision.shouldRun {
		t.Fatalf("expected a retried stop to run, got guidance %q", decision.guidance)
	}
}

// Regression: building a local backend detected the container engine, so init
// waited for an unresponsive Docker daemon although it starts no container.
//
//nolint:paralleltest // The fake Docker executable requires a process-wide PATH override.
func TestLocalBackendConfigurationLeavesTheContainerEngineAlone(t *testing.T) {
	requireLinuxLocalPlatform(t)
	directory := t.TempDir()
	calls := filepath.Join(directory, "calls")
	//nolint:gosec // This test fixture must be executable.
	if err := os.WriteFile(filepath.Join(directory, "docker"),
		[]byte("#!/bin/sh\necho \"$*\" >> '"+calls+"'\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("failed to write fake docker: %v", err)
	}
	t.Setenv("PATH", directory)
	deployment := newLocalTestDeployment(t)
	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	backend, err := newDeploymentBackend(testManagerContext(t), deployment, manifest)
	if err != nil {
		t.Fatalf("expected the backend to be created, got %v", err)
	}
	if err := backend.ValidateEnvironment(); err != nil {
		t.Fatalf("expected the environment to be valid, got %v", err)
	}
	if _, err := backend.ReadConfiguration(); err != nil {
		t.Fatalf("expected the configuration to be read, got %v", err)
	}

	if recorded, err := os.ReadFile(calls); err == nil {
		t.Fatalf("expected no docker commands, got %q", recorded)
	}
}

func installFakeExecutable(t *testing.T, name, script string) {
	t.Helper()
	directory := t.TempDir()
	//nolint:gosec // This test fixture must be executable.
	if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake %s: %v", name, err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}
