// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeadapter"
)

func deployLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) error {
	return startLocalRuntime(ctx, deployment, runtimeConfig, waitTimeoutSeconds, out, outErr)
}

func startLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) error {
	prepared, err := prepareLocalRuntimeAdapter(ctx, deployment, runtimeConfig)
	if err != nil {
		return err
	}
	if err := prepared.adapter.Prerequisites(
		ctx,
		runtimePrerequisiteOptions(out),
	); err != nil {
		return err
	}
	status, err := prepared.adapter.Start(ctx, prepared.spec, out, outErr)
	if err != nil {
		return diagnoseLocalFailure(ctx, deployment, err)
	}
	if status.Database.Port <= 0 {
		return errors.New("runtime did not report a resolved database port")
	}
	if err := writeRuntimeAdapterArtifacts(
		deployment,
		status,
		prepared.adapter.Capabilities(),
	); err != nil {
		return err
	}
	return waitForRuntimeDatabase(ctx, deployment, waitTimeoutSeconds)
}

// reconcileLocalVMState corrects a stale WorkflowStateRunning caused by an unclean
// VM shutdown (e.g. SIGKILL). If the v2 runtime provider reports the VM is not
// running, the state is updated to WorkflowStateStopped so that subsequent permit
// checks in Start/Stop see a consistent state.
//
// Only reconciles Running→Stopped. Stopped→Running is not corrected, as a VM
// running outside the launcher's knowledge is an externally-caused inconsistency
// that should surface as an error rather than be silently accepted.
//
// Errors from the VM status check are logged and swallowed; reconciliation is
// best-effort and must not block the caller's primary operation.
// The caller must already hold the exclusive deployment lock.
func reconcileLocalVMState(
	ctx context.Context,
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) error {
	if !isLocalDeployment(deployment) {
		return nil
	}

	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Debug("could not read workflow state during reconciliation", "error", err)
		return nil
	}

	if _, ok := workflowState.(*config.WorkflowStateRunning); !ok {
		return nil
	}

	vmStatus, err := getLocalVMStatus(ctx, deployment)
	if err != nil {
		slog.Warn("could not determine local VM status during reconciliation", "error", err)
		return nil
	}

	if !vmStatus.Running {
		slog.Info("local VM is not running; correcting workflow state to stopped")
		return exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateStopped{}, deployment)
	}

	return nil
}

func isLocalDeployment(deployment config.DeploymentDir) bool {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return false
	}

	kind, err := resolveBackendKind(manifest)

	return err == nil && kind == backendTypeLocal
}

func getLocalVMStatus(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*localVMStatus, error) {
	status, err := getLocalRuntimeAdapterStatus(ctx, deployment)
	if err != nil {
		return nil, err
	}

	return &localVMStatus{
		Running: status.Phase == runtimeadapter.RuntimePhaseRunning ||
			status.Phase == runtimeadapter.RuntimePhaseDegraded,
	}, nil
}

type localVMStatus struct {
	Running bool
}

func getLocalRuntimeAdapterStatus(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*runtimeadapter.RuntimeStatus, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return nil, err
	}
	prepared, err := reconstructLocalRuntimeAdapter(deployment, runtimeConfig)
	if err != nil {
		return nil, err
	}
	status, err := prepared.adapter.Status(ctx, prepared.spec)
	if err != nil {
		return nil, err
	}

	return status, nil
}

func getLocalRuntimeAdapterHealth(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*runtimeadapter.RuntimeStatus, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return nil, err
	}
	prepared, err := reconstructLocalRuntimeAdapter(deployment, runtimeConfig)
	if err != nil {
		return nil, err
	}

	return prepared.adapter.Health(ctx, prepared.spec)
}

func stopLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	out, outErr io.Writer,
) error {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return err
	}
	prepared, err := reconstructLocalRuntimeAdapter(deployment, runtimeConfig)
	if err != nil {
		return err
	}
	if err := prepared.adapter.Stop(ctx, prepared.spec, out, outErr); err != nil {
		return err
	}

	return updateLocalDeploymentArtifactState(deployment, StatusStopped)
}

func destroyLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	out, outErr io.Writer,
) error {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return err
	}
	prepared, err := reconstructLocalRuntimeAdapter(deployment, runtimeConfig)
	if err != nil {
		return err
	}
	if err := prepared.adapter.Destroy(ctx, prepared.spec, out, outErr); err != nil {
		return err
	}
	for _, path := range []string{
		filepath.Join(deployment.Root(), "local", "control"),
		filepath.Join(deployment.Root(), "local", "generated"),
		filepath.Join(deployment.Root(), "local", "provider"),
		filepath.Join(deployment.Root(), "local", "runtime"),
	} {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	if err := runtimeadapter.RemovePersonalData(
		deployment.Root(),
		filepath.Join(deployment.Root(), "local", "data"),
	); err != nil {
		return err
	}

	for _, path := range []string{
		deployment.NodeDetailsPath(),
		deployment.SecretsPath(),
		deployment.ConnectionInstructionsPath(),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local deployment artifact %s: %w", path, err)
		}
	}

	return nil
}

func updateLocalDeploymentArtifactState(deployment config.DeploymentDir, state string) error {
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return fmt.Errorf("failed to read local deployment info after state change: %w", err)
	}

	info.DeploymentState = state
	info.ClusterState = state

	return config.WriteDeploymentInfo(deployment.Root(), info)
}
