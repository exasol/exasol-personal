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
	"github.com/exasol/exasol-personal/internal/runtimeadapter"
)

// LocalDiagnostics is a read-only snapshot of the local deployment preset's
// runtime and reachability state, usable at any time and not only on
// failure.
type LocalDiagnostics struct {
	SchemaVersion     int                                `json:"schemaVersion"`
	Platform          string                             `json:"platform"`
	PlatformSupported bool                               `json:"platformSupported"`
	Capabilities      runtimeadapter.RuntimeCapabilities `json:"capabilities"`
	RuntimeKind       string                             `json:"runtimeKind,omitempty"`
	RuntimeRunning    *bool                              `json:"runtimeRunning,omitempty"`
	WorkloadName      string                             `json:"workloadName,omitempty"`
	ContainerID       string                             `json:"containerId,omitempty"`
	VMRunning         *bool                              `json:"vmRunning,omitempty"`
	GuestIP           string                             `json:"guestIp,omitempty"`
	Ports             map[string]int                     `json:"ports,omitempty"`
	PortHealth        map[string]string                  `json:"portHealth,omitempty"`
	WorkloadPhase     string                             `json:"workloadPhase,omitempty"`
	HookPhase         string                             `json:"hookPhase,omitempty"`
	RequestedPort     int                                `json:"requestedPort,omitempty"`
	ResolvedPort      int                                `json:"resolvedPort,omitempty"`
	DatabaseReady     *bool                              `json:"databaseReady,omitempty"`
	DatabaseError     string                             `json:"databaseError,omitempty"`
	Warning           string                             `json:"warning,omitempty"`
	Message           string                             `json:"message,omitempty"`
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
		SchemaVersion: 1,
		Platform:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
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

	status, err := getLocalRuntimeAdapterHealth(ctx, deployment)
	if err != nil {
		diagnostics.Message = fmt.Sprintf("could not determine local runtime status: %s", err)
		return diagnostics
	}

	running := status.Phase == runtimeadapter.RuntimePhaseRunning ||
		status.Phase == runtimeadapter.RuntimePhaseDegraded
	diagnostics.RuntimeRunning = &running
	diagnostics.WorkloadName = status.WorkloadName
	diagnostics.ContainerID = status.ContainerID
	diagnostics.WorkloadPhase = string(status.Phase)
	diagnostics.Message = status.Message
	if manifest, manifestErr := config.ReadInfrastructureManifest(deployment); manifestErr == nil {
		if runtimeConfig, configErr := resolveLocalRuntimeConfig(manifest, 0); configErr == nil {
			if requested, _, portErr := parseLocalDatabasePort(runtimeConfig.ports); portErr == nil {
				diagnostics.RequestedPort = requested
			}
		}
	}
	diagnostics.ResolvedPort = status.Database.Port
	diagnostics.Capabilities = localPlatformCapabilities()
	diagnostics.RuntimeKind = "podman"
	if status.VM != nil {
		diagnostics.RuntimeKind = "local-vm"
		vmRunning := status.VM.Phase == "running" || status.VM.Phase == "degraded"
		diagnostics.VMRunning = &vmRunning
	}
	diagnostics.Ports = map[string]int{"db": status.Database.Port}
	if status.Database.Health != "" {
		diagnostics.PortHealth = map[string]string{"db": status.Database.Health}
	}
	if status.VM != nil {
		diagnostics.GuestIP = status.VM.GuestIP
		diagnostics.HookPhase = status.VM.Hook
		if status.VM.SSH != nil {
			diagnostics.Ports["ssh"] = status.VM.SSH.Port
		}
		for name, endpoint := range status.VM.Forwards {
			diagnostics.Ports[name] = endpoint.Port
			if endpoint.Health != "" {
				if diagnostics.PortHealth == nil {
					diagnostics.PortHealth = map[string]string{}
				}
				diagnostics.PortHealth[name] = endpoint.Health
			}
		}
	}
	if !running {
		diagnostics.Message = "The platform is ready to run the local deployment. " +
			"Run `exasol start` to start it."

		return diagnostics
	}

	diagnostics.Warning = unexpectedRunningVMWarning(deployment)

	dbCtx, cancel := context.WithTimeout(ctx, LocalDatabaseStartedDefaultTimeoutSeconds*time.Second)
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
		"VM storage conflict. Look for a `local-vm` process for this deployment and stop " +
		"it, then retry."
}
