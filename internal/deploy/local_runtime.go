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
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

// Internal escape hatch for fake local-runner integration tests.
const localSkipDatabaseWaitEnv = "EXASOL_LOCAL_SKIP_DB_WAIT"

func startLocalRuntime(
	ctx context.Context,
	runtime *localruntime.Runtime,
	runtimeConfig localRuntimeConfig,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) error {
	if err := runtime.Prepare(ctx, out, outErr); err != nil {
		return err
	}

	return startPreparedLocalRuntime(ctx, runtime, runtimeConfig, waitTimeoutSeconds, out, outErr)
}

func startPreparedLocalRuntime(
	ctx context.Context,
	runtime *localruntime.Runtime,
	runtimeConfig localRuntimeConfig,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) error {
	paths := runtime.Paths()
	if err := os.Remove(paths.StatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove stale local VM state: %w", err)
	}

	localConfig := toLocalRuntimeConfig(runtimeConfig)
	startArgs := []string{"start"}
	versionCheckArgs, err := localRunnerVersionCheckArgs(runtime.Deployment())
	if err != nil {
		return err
	}
	startArgs = append(startArgs, versionCheckArgs...)
	slcArgs, err := localRunnerSlcArgs(runtime.Deployment())
	if err != nil {
		return err
	}
	startArgs = append(startArgs, slcArgs...)
	if localConfig.Ports != "" {
		startArgs = append(startArgs, "--ports", localConfig.Ports)
	}
	startArgs = append(startArgs,
		strconv.Itoa(localConfig.CPUCount),
		strconv.Itoa(localConfig.MemoryMB),
		strconv.Itoa(localConfig.DataSizeGB),
	)
	if err := runtime.RunCommand(ctx, startArgs, out, outErr); err != nil {
		return diagnoseLocalFailure(ctx, runtime, err)
	}

	state, err := runtime.ReadState()
	if err != nil {
		return err
	}

	return writeLocalRuntimeArtifactsAndWait(ctx, runtime, state, waitTimeoutSeconds)
}

func localRunnerVersionCheckArgs(deployment config.DeploymentDir) ([]string, error) {
	launcherState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to read local version-check settings: %w", err)
	}

	if !launcherState.VersionCheckEnabled {
		return []string{"--version-check-enabled=false"}, nil
	}

	clusterIdentity := strings.TrimSpace(launcherState.ClusterIdentity)
	if clusterIdentity == "" {
		return nil, errors.New("deployment state is missing cluster identity")
	}

	return []string{
		"--version-check-enabled=true",
		"--version-check-url", GetVersionCheckURL(),
		"--version-check-identity", clusterIdentity,
	}, nil
}

// reconcileLocalVMState corrects a stale WorkflowStateRunning caused by an unclean
// VM shutdown (e.g. SIGKILL). If the mac-runner socket reports the daemon is not
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

	manager, err := newResourceManager()
	if err != nil {
		slog.Warn("could not construct resource manager during reconciliation", "error", err)
		return nil
	}

	vmStatus, err := localruntime.New(deployment, manager).Status(ctx)
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

func stopLocalRuntime(
	ctx context.Context,
	runtime *localruntime.Runtime,
	out, outErr io.Writer,
) error {
	if err := runtime.Stop(ctx, out, outErr); err != nil {
		return err
	}

	return updateLocalDeploymentArtifactState(runtime.Deployment(), StatusStopped)
}

func destroyLocalRuntime(
	ctx context.Context,
	runtime *localruntime.Runtime,
	out, outErr io.Writer,
) error {
	if err := runtime.Destroy(ctx, out, outErr); err != nil {
		return err
	}

	deployment := runtime.Deployment()
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

func toLocalRuntimeConfig(runtimeConfig localRuntimeConfig) localruntime.Config {
	return localruntime.Config{
		CPUCount:   runtimeConfig.cpuCount,
		MemoryMB:   runtimeConfig.memoryMB,
		DataSizeGB: runtimeConfig.dataSizeGB,
		Ports:      runtimeConfig.ports,
	}
}

func writeLocalRuntimeArtifactsAndWait(
	ctx context.Context,
	runtime *localruntime.Runtime,
	state *localruntime.State,
	waitTimeoutSeconds int,
) error {
	if err := writeLocalDeploymentArtifacts(runtime.Deployment(), state); err != nil {
		return err
	}
	if os.Getenv(localSkipDatabaseWaitEnv) != "" {
		return nil
	}

	if waitTimeoutSeconds <= 0 {
		waitTimeoutSeconds = LocalDatabaseStartedDefaultTimeoutSeconds
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(waitTimeoutSeconds)*time.Second)
	defer cancel()

	return WaitForLocalDatabaseStarted(waitCtx, runtime)
}

func writeLocalDeploymentArtifacts(
	deployment config.DeploymentDir,
	state *localruntime.State,
) error {
	if state == nil {
		return errors.New("local runtime endpoint state is required")
	}

	deploymentID := "local"
	if launcherState, err := config.ReadExasolPersonalState(deployment); err == nil {
		if strings.TrimSpace(launcherState.DeploymentId) != "" {
			deploymentID = launcherState.DeploymentId
		}
	}

	sshPort := strconv.Itoa(state.SSHPort)
	sshCommand := fmt.Sprintf(
		"ssh -i %s %s@%s -p %s",
		state.PrivateKeyRelativePath,
		localSSHUser,
		localDeploymentPublicHost,
		sshPort,
	)

	info := &config.DeploymentInfo{
		Backend:         localDeploymentBackend,
		DeploymentId:    deploymentID,
		DeploymentState: StatusRunning,
		ClusterSize:     1,
		ClusterState:    StatusRunning,
		InstanceType:    "exasol-local",
		Connection: &config.DeploymentConnection{
			Host:                       localDeploymentPublicHost,
			DisplayHost:                localDeploymentPublicHost,
			PublicIp:                   localDeploymentPublicHost,
			DBPort:                     state.DBPort,
			Username:                   localDBUser,
			InsecureSkipCertValidation: true,
			SSHCommand:                 sshCommand,
			SSHPort:                    sshPort,
			ShellSupported:             true,
		},
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), info); err != nil {
		return err
	}

	return config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword: localDBPassword,
	})
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
