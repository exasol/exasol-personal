// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

// LocalDiagnostics is a read-only snapshot of the local deployment preset's
// runtime and reachability state, usable at any time and not only on
// failure.
type LocalDiagnostics struct {
	Platform          string            `json:"platform"`
	PlatformSupported bool              `json:"platformSupported"`
	VMRunning         *bool             `json:"vmRunning,omitempty"`
	GuestIP           string            `json:"guestIp,omitempty"`
	Ports             map[string]int    `json:"ports,omitempty"`
	PortHealth        map[string]string `json:"portHealth,omitempty"`
	DatabaseReady     *bool             `json:"databaseReady,omitempty"`
	DatabaseError     string            `json:"databaseError,omitempty"`
	Warning           string            `json:"warning,omitempty"`
	Message           string            `json:"message,omitempty"`
}

func DiagnoseLocal(ctx context.Context, deployment config.DeploymentDir, writer io.Writer) error {
	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		diagnostics := diagnoseLocalUnsafe(ctx, deployment)

		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")

		return encoder.Encode(diagnostics)
	})
}

// diagnoseLocalUnsafe never fails on its own: each check populates whatever
// it can and stops at the first one that doesn't apply, so a diagnostic
// command doesn't itself become another thing that can error out.
func diagnoseLocalUnsafe(ctx context.Context, deployment config.DeploymentDir) *LocalDiagnostics {
	diagnostics := &LocalDiagnostics{
		Platform: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	allowUnsupported := os.Getenv(localAllowUnsupportedEnv)
	if err := validateLocalPlatform(runtime.GOOS, runtime.GOARCH, allowUnsupported); err != nil {
		diagnostics.Message = err.Error()
		return diagnostics
	}
	diagnostics.PlatformSupported = true

	if !isLocalDeployment(deployment) {
		diagnostics.Message = "this deployment is not using the local preset"
		return diagnostics
	}

	vmStatus, err := getLocalVMStatus(ctx, deployment)
	if err != nil {
		diagnostics.Message = fmt.Sprintf("could not determine local VM status: %s", err)
		return diagnostics
	}

	running := vmStatus.Running
	diagnostics.VMRunning = &running
	if !running {
		diagnostics.Message = "The platform is ready to run the local deployment. " +
			"Run `exasol start` to start it, then run `exasol diag local` again for more detail."

		return diagnostics
	}

	diagnostics.Warning = unexpectedRunningVMWarning(deployment)

	if state, err := localruntime.ReadState(deployment); err == nil {
		diagnostics.Ports = map[string]int{"ssh": state.SSHPort, "db": state.DBPort}
		if state.UIPort != 0 {
			diagnostics.Ports["ui"] = state.UIPort
		}
		diagnostics.GuestIP = state.VMIP
	}

	if health, err := localruntime.HealthCheck(ctx, deployment); err == nil {
		diagnostics.PortHealth = make(map[string]string, len(health.Ports))
		for name, portHealth := range health.Ports {
			diagnostics.PortHealth[name] = string(portHealth.State)
		}
	}

	dbCtx, cancel := context.WithTimeout(
		ctx,
		LocalDatabaseStartedDefaultTimeoutSeconds*time.Second,
	)
	defer cancel()

	dbErr := verifyDatabaseConnection(dbCtx, deployment)
	ready := dbErr == nil
	diagnostics.DatabaseReady = &ready
	if dbErr != nil {
		diagnostics.DatabaseError = dbErr.Error()
	}

	return diagnostics
}

// unexpectedRunningVMWarning returns a non-empty message when a local VM
// process is running but the recorded workflow state doesn't expect one --
// e.g. a daemon orphaned by a prior crash or a manually killed launcher
// invocation, which can leave the next start/install failing with a VM
// storage conflict instead of a clear explanation (see reconcileLocalVMState,
// which deliberately only auto-corrects the opposite direction).
//
// DiagnoseLocal holds the deployment's shared lock for its whole run, and a
// shared-lock acquisition blocks for as long as any real start/install/stop
// is holding the exclusive lock (see internal/directorymutex). So by the
// time this runs, no genuine concurrent operation can be in flight: any
// mismatch found here reflects a stale process, not a live race with a
// legitimate one.
func unexpectedRunningVMWarning(deployment config.DeploymentDir) string {
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return ""
	}

	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		return ""
	}

	if _, ok := workflowState.(*config.WorkflowStateRunning); ok {
		return ""
	}

	return "a local VM process is running, but the recorded deployment state does not " +
		"expect one. This is likely a process orphaned by an earlier crash or a manually " +
		"killed launcher invocation, and can cause a future start/install to fail with a " +
		"VM storage conflict. Look for a `mac-runner` process for this deployment and stop " +
		"it, then retry."
}
