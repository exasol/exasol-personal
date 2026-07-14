// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

func TestDiagnoseLocalUnsafe_UnsupportedPlatform(t *testing.T) {
	t.Parallel()

	// Deliberately does not set localAllowUnsupportedEnv, unlike the
	// "supported platform" tests below (which cannot run in parallel with
	// each other since t.Setenv forbids that, but do not conflict with this
	// test since Go runs all non-parallel tests to completion first).
	deployment := newLocalTestDeployment(t)

	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

	if diagnostics.PlatformSupported {
		t.Fatalf("expected unsupported platform, got %+v", diagnostics)
	}
	if diagnostics.Message == "" {
		t.Fatal("expected a message explaining the unsupported platform")
	}
}

func TestDiagnoseLocalUnsafe_NonLocalDeployment(t *testing.T) {
	t.Setenv(localAllowUnsupportedEnv, "1")

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

	if !diagnostics.PlatformSupported {
		t.Fatal("expected platform support to bypass via EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM")
	}
	if diagnostics.VMRunning != nil {
		t.Fatalf("expected no VM status check for a non-local deployment, got %+v", diagnostics)
	}
}

func TestDiagnoseLocalUnsafe_VMNotRunning(t *testing.T) {
	skipOnWindows(t)
	t.Setenv(localAllowUnsupportedEnv, "1")

	deployment := newLocalTestDeployment(t)
	writeFakeCombinedRunner(t, deployment, `{"running":false}`, "")

	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

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

func TestDiagnoseLocalUnsafe_VMRunningReportsPortsAndHealth(t *testing.T) {
	skipOnWindows(t)
	t.Setenv(localAllowUnsupportedEnv, "1")

	deployment := newLocalTestDeployment(t)
	healthJSON := `{"ports":{"ssh":{"state":"reachable"},"db":{"state":"blocked"}}}`
	writeFakeCombinedRunner(t, deployment, `{"running":true}`, healthJSON)
	writeFakeVMState(t, deployment, "192.168.64.5", 20022, 28563, 0)

	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

	if diagnostics.VMRunning == nil || !*diagnostics.VMRunning {
		t.Fatalf("expected VMRunning to be true, got %+v", diagnostics)
	}
	if diagnostics.GuestIP != "192.168.64.5" {
		t.Fatalf("expected guest IP to be reported, got %q", diagnostics.GuestIP)
	}
	if diagnostics.Ports["ssh"] != 20022 || diagnostics.Ports["db"] != 28563 {
		t.Fatalf("expected bound host ports to be reported, got %+v", diagnostics.Ports)
	}
	if diagnostics.PortHealth["ssh"] != "reachable" || diagnostics.PortHealth["db"] != "blocked" {
		t.Fatalf("expected per-port health to be reported, got %+v", diagnostics.PortHealth)
	}
	if diagnostics.DatabaseReady == nil {
		t.Fatal("expected a database readiness check to have run")
	}
}

func TestDiagnoseLocalUnsafe_VMRunningMatchesRunningState_NoWarning(t *testing.T) {
	skipOnWindows(t)
	t.Setenv(localAllowUnsupportedEnv, "1")

	deployment := newLocalTestDeployment(t)
	writeFakeCombinedRunner(t, deployment, `{"running":true}`, `{"ports":{}}`)
	writeFakeWorkflowState(t, deployment, &config.WorkflowStateRunning{})

	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

	if diagnostics.Warning != "" {
		t.Fatalf("expected no warning when workflow state matches a running VM, got %q",
			diagnostics.Warning)
	}
}

func TestDiagnoseLocalUnsafe_VMRunningButStateNotRunning_Warning(t *testing.T) {
	skipOnWindows(t)
	t.Setenv(localAllowUnsupportedEnv, "1")

	deployment := newLocalTestDeployment(t)
	writeFakeCombinedRunner(t, deployment, `{"running":true}`, `{"ports":{}}`)
	writeFakeWorkflowState(t, deployment, &config.WorkflowStateInterrupted{
		Error:                      "boom",
		InterruptedDuringOperation: "start",
	})

	diagnostics := diagnoseLocalUnsafe(context.Background(), deployment)

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

	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowStateAndWrite(state, deployment); err != nil {
		t.Fatalf("failed to write fake workflow state: %v", err)
	}
}

func writeFakeVMState(
	t *testing.T,
	deployment config.DeploymentDir,
	vmIP string,
	sshPort, dbPort, uiPort int,
) {
	t.Helper()

	paths := localruntime.NewPaths(deployment)
	data, err := json.Marshal(map[string]any{
		"vm_name": "exasol-local-vm",
		"vm_ip":   vmIP,
		"ports": map[string]any{
			"ssh": sshPort,
			"db":  dbPort,
			"ui":  uiPort,
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal fake VM state: %v", err)
	}
	if err := os.WriteFile(paths.StatePath, data, 0o600); err != nil {
		t.Fatalf("failed to write fake VM state: %v", err)
	}
}
