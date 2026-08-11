// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

//nolint:paralleltest // Fake runner resource resolution is process-sensitive.
func TestDiagnoseLocalUnsafe_NonLocalDeployment(t *testing.T) {
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	diagnostics := diagnoseLocalUnsafe(
		context.Background(),
		localruntime.NewMacVMRuntime(deployment, nil),
	)

	if !diagnostics.PlatformSupported {
		t.Fatal("expected platform to be supported")
	}
	if diagnostics.VMRunning != nil {
		t.Fatalf("expected no VM status check for a non-local deployment, got %+v", diagnostics)
	}
}

//nolint:paralleltest // Fake runner resource resolution is process-sensitive.
func TestDiagnoseLocalUnsafe_VMNotRunning(t *testing.T) {
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	manager := writeFakeCombinedRunner(t, `{"running":false}`, "")

	diagnostics := diagnoseLocalUnsafe(
		context.Background(), localruntime.NewMacVMRuntime(deployment, manager),
	)

	if diagnostics.VMRunning == nil || *diagnostics.VMRunning {
		t.Fatalf("expected VMRunning to be false, got %+v", diagnostics)
	}
	if diagnostics.Message == "" {
		t.Fatal("expected a concise ready-to-run message when the VM is not running")
	}
	if diagnostics.PortHealth != nil || diagnostics.DatabaseReady != nil {
		t.Fatalf("expected no reachability/readiness checks when VM not running, got %+v",
			diagnostics)
	}
}

//nolint:paralleltest // Fake runner resource resolution is process-sensitive.
func TestDiagnoseLocalUnsafe_VMRunningReportsPortsAndHealth(t *testing.T) {
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	healthJSON := `{"ports":{"db":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, `{"running":true}`, healthJSON)
	writeFakeVMState(t, deployment, 28563)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	diagnostics := diagnoseLocalUnsafe(
		ctx, localruntime.NewMacVMRuntime(deployment, manager),
	)

	if diagnostics.VMRunning == nil || !*diagnostics.VMRunning {
		t.Fatalf("expected VMRunning to be true, got %+v", diagnostics)
	}
	if diagnostics.Ports["db"] != 28563 {
		t.Fatalf("expected bound host ports to be reported, got %+v", diagnostics.Ports)
	}
	if diagnostics.PortHealth["db"] != "blocked" {
		t.Fatalf("expected per-port health to be reported, got %+v", diagnostics.PortHealth)
	}
	if diagnostics.DatabaseReady == nil {
		t.Fatal("expected a database readiness check to have run")
	}
}

//nolint:paralleltest // Fake runner resource resolution is process-sensitive.
func TestDiagnoseLocalUnsafe_VMRunningMatchesRunningState_NoWarning(t *testing.T) {
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	manager := writeFakeCombinedRunner(t, `{"running":true}`, `{"ports":{}}`)
	writeFakeWorkflowState(t, deployment, &config.WorkflowStateRunning{})
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	diagnostics := diagnoseLocalUnsafe(
		ctx,
		localruntime.NewMacVMRuntime(deployment, manager),
	)

	if diagnostics.Warning != "" {
		t.Fatalf("expected no warning when workflow state matches a running VM, got %q",
			diagnostics.Warning)
	}
}

//nolint:paralleltest // Fake runner resource resolution is process-sensitive.
func TestDiagnoseLocalUnsafe_VMRunningButStateNotRunning_Warning(t *testing.T) {
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	manager := writeFakeCombinedRunner(t, `{"running":true}`, `{"ports":{}}`)
	writeFakeWorkflowState(t, deployment, &config.WorkflowStateInterrupted{
		Error:                      "boom",
		InterruptedDuringOperation: "start",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	diagnostics := diagnoseLocalUnsafe(
		ctx,
		localruntime.NewMacVMRuntime(deployment, manager),
	)

	if diagnostics.Warning == "" {
		t.Fatal("expected a warning when a VM is running but the workflow state doesn't expect one")
	}
}

// writeFakeWorkflowState persists the given workflow state (one of the
// config.WorkflowState* structs) to the deployment's launcher state file, so
// diagnoseLocalUnsafe's orphaned-VM check has something concrete to compare
// the fake runner's reported VM status against.
func writeFakeWorkflowState(t *testing.T, deployment config.DeploymentDir, state any) {
	t.Helper()

	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read fake workflow state: %v", err)
	}
	if err := exasolState.SetWorkflowStateAndWrite(state, deployment); err != nil {
		t.Fatalf("failed to write fake workflow state: %v", err)
	}
}

func writeFakeVMState(
	t *testing.T,
	deployment config.DeploymentDir,
	dbPort int,
) {
	t.Helper()

	paths := newLocalRuntimeTestPaths(deployment)
	data, err := json.Marshal(map[string]any{
		"vm_name":    "exasol-local-vm",
		"shared_dir": "./vm-shared",
		"forwards": map[string]any{
			"db": map[string]any{"guest_port": 8563, "host_port": dbPort},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal fake VM state: %v", err)
	}
	if err := os.WriteFile(paths.StatePath, data, 0o600); err != nil {
		t.Fatalf("failed to write fake VM state: %v", err)
	}
}
