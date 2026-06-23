// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

// Internal escape hatch for fake local-runner integration tests.
const localSkipDatabaseWaitEnv = "EXASOL_LOCAL_SKIP_DB_WAIT"

func deployLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
	out, outErr io.Writer,
) error {
	return startLocalRuntime(ctx, deployment, runtimeConfig, out, outErr)
}

func startLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
	out, outErr io.Writer,
) error {
	if err := localruntime.Prepare(
		ctx,
		deployment,
		toLocalRuntimeConfig(runtimeConfig),
		out,
		outErr,
	); err != nil {
		return err
	}

	return startPreparedLocalRuntime(ctx, deployment, runtimeConfig, out, outErr)
}

func startPreparedLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig localRuntimeConfig,
	out, outErr io.Writer,
) error {
	paths := localruntime.NewPaths(deployment)
	if err := os.Remove(paths.StatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove stale local VM state: %w", err)
	}

	localConfig := toLocalRuntimeConfig(runtimeConfig)
	startArgs := []string{"start"}
	if localConfig.Ports != "" {
		startArgs = append(startArgs, "--ports", localConfig.Ports)
	}
	startArgs = append(startArgs,
		strconv.Itoa(localConfig.CPUCount),
		strconv.Itoa(localConfig.MemoryMB),
		strconv.Itoa(localConfig.DataSizeGB),
	)
	if err := localruntime.RunCommand(
		ctx,
		deployment,
		startArgs,
		out,
		outErr,
	); err != nil {
		return err
	}

	state, err := localruntime.ReadState(deployment)
	if err != nil {
		return err
	}

	return writeLocalRuntimeArtifactsAndWait(ctx, deployment, state)
}

func stopLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	out, outErr io.Writer,
) error {
	if err := localruntime.Stop(ctx, deployment, out, outErr); err != nil {
		return err
	}

	return updateLocalDeploymentArtifactState(deployment, StatusStopped)
}

func destroyLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	out, outErr io.Writer,
) error {
	if err := localruntime.Destroy(ctx, deployment, out, outErr); err != nil {
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
	deployment config.DeploymentDir,
	state *localruntime.State,
) error {
	if err := writeLocalDeploymentArtifacts(deployment, state); err != nil {
		return err
	}
	if os.Getenv(localSkipDatabaseWaitEnv) != "" {
		return nil
	}

	return WaitForLocalDatabaseStarted(ctx, deployment)
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
