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
	"os/exec"
	"path/filepath"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
)

const (
	localRunnerCommandName     = "internal-local-runtime-runner"
	localClusterSize           = 1
	localClusterStateStarting  = "starting"
	localClusterStateRunning   = "running"
	localClusterStateStopped   = "stopped"
	localLoopbackHost          = "127.0.0.1"
	localStopTimeout           = 60 * time.Second
	localRunnerKillWaitTimeout = 5 * time.Second
	localRunnerStartupTimeout  = 3 * time.Second
	localRunnerMaxBackoff      = 5
	localRunnerLogDirMode      = 0o700
	localRunnerLogFileMode     = 0o600
	localRunnerPollInterval    = 100 * time.Millisecond
)

var localRunnerCommand = startLocalRunnerCommand

type localBackend struct{}

func (localBackend) ValidateEnvironment() error { return validateLocalHostPlatform() }

func (localBackend) OpenHostShell(
	_ context.Context,
	_ config.DeploymentDir,
	_ string,
) error {
	return fmt.Errorf(
		"%w: `shell host` is unavailable because local deployments do not expose SSH host access",
		ErrLocalShellUnsupported,
	)
}

func (localBackend) OpenCOSShell(_ context.Context, _ config.DeploymentDir) error {
	return fmt.Errorf(
		"%w: `shell container` is unavailable because local deployments do not expose COS shells",
		ErrLocalShellUnsupported,
	)
}

func (localBackend) Deploy(
	ctx context.Context,
	deployment config.DeploymentDir,
	_ *presets.InfrastructureManifest,
	_, _ io.Writer,
	_ TofuLockfileMode,
) error {
	runtime := localruntime.New(deployment.Root())

	return ensureLocalRuntimeStarted(ctx, deployment, runtime, StartedDefaultTimeoutSeconds)
}

func (localBackend) Start(
	ctx context.Context,
	deployment config.DeploymentDir,
	_ *presets.InfrastructureManifest,
	_, _ io.Writer,
	waitTimeoutSeconds int,
) error {
	runtime := localruntime.New(deployment.Root())

	return ensureLocalRuntimeStarted(ctx, deployment, runtime, waitTimeoutSeconds)
}

func (localBackend) Stop(
	ctx context.Context,
	deployment config.DeploymentDir,
	_ *presets.InfrastructureManifest,
	_, _ io.Writer,
) error {
	runtime := localruntime.New(deployment.Root())

	running, pid, err := runtime.RunnerRunning()
	if err != nil {
		return err
	}
	if !running {
		if err := runtime.CleanupTransientState(); err != nil {
			return err
		}

		return writeLocalArtifacts(deployment, runtime, localClusterStateStopped, StatusStopped)
	}

	stopCtx, cancel := context.WithTimeout(ctx, localStopTimeout)
	defer cancel()

	if err := runtime.Controller().RequestGracefulStop(stopCtx); err != nil {
		slog.Warn("failed to request graceful local runtime stop", "error", err)
	}

	if err := runtime.WaitForRunnerExit(stopCtx, pid); err != nil {
		process, findErr := os.FindProcess(pid)
		if findErr != nil {
			return err
		}
		if killErr := process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return fmt.Errorf("failed to stop local runtime process %d: %w", pid, killErr)
		}

		killCtx, killCancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			localRunnerKillWaitTimeout,
		)
		defer killCancel()
		if waitErr := runtime.WaitForRunnerExit(killCtx, pid); waitErr != nil &&
			!errors.Is(waitErr, context.DeadlineExceeded) {
			return waitErr
		}
	}

	if err := runtime.CleanupTransientState(); err != nil {
		return err
	}

	return writeLocalArtifacts(deployment, runtime, localClusterStateStopped, StatusStopped)
}

func (localBackend) Destroy(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	out, outErr io.Writer,
) error {
	runtime := localruntime.New(deployment.Root())
	running, _, err := runtime.RunnerRunning()
	if err != nil {
		return err
	}
	if running {
		if err := (localBackend{}).Stop(ctx, deployment, manifest, out, outErr); err != nil {
			return err
		}
	}

	if err := os.RemoveAll(runtime.Layout().RuntimeRoot()); err != nil {
		return fmt.Errorf("failed to remove local runtime root: %w", err)
	}

	if path, exists, err := config.GetDeploymentInfoFilePath(deployment.Root()); err != nil {
		return err
	} else if exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local deployment info file: %w", err)
		}
	}

	if path, err := config.SecretsFilePath(deployment); err == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local secrets file: %w", err)
		}
	}

	return nil
}

func ensureLocalRuntimeStarted(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtime *localruntime.Runtime,
	waitTimeoutSeconds int,
) error {
	if err := runtime.EnsureRoot(); err != nil {
		return err
	}
	if _, err := runtime.EnsurePayloadSelected(ctx); err != nil {
		return err
	}
	if _, err := runtime.EnsureConnectionPorts(); err != nil {
		return err
	}
	if _, err := ensureLocalSecrets(deployment); err != nil {
		return err
	}

	if err := writeLocalArtifacts(
		deployment,
		runtime,
		localClusterStateStarting,
		StatusOperationInProgress,
	); err != nil {
		return err
	}

	running, _, err := runtime.RunnerRunning()
	if err != nil {
		return err
	}
	if !running {
		if err := localRunnerCommand(deployment, runtime); err != nil {
			return err
		}
	}

	if waitTimeoutSeconds <= 0 {
		waitTimeoutSeconds = StartedDefaultTimeoutSeconds
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(waitTimeoutSeconds)*time.Second)
	defer cancel()

	if err := waitForLocalRuntimeStarted(waitCtx, deployment, runtime); err != nil {
		return err
	}
	if err := ensureLocalDatabaseCredentialsFn(waitCtx, deployment); err != nil {
		return err
	}

	return writeLocalArtifacts(deployment, runtime, localClusterStateRunning, StatusRunning)
}

func waitForLocalRuntimeStarted(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtime *localruntime.Runtime,
) error {
	return PollWithBackoff(ctx, func(ctx context.Context) (bool, error) {
		dbReadyErr := verifyDatabaseConnectionFn(ctx, deployment)
		if dbReadyErr == nil {
			return true, nil
		}

		running, pid, runnerErr := runtime.RunnerRunning()
		if runnerErr != nil {
			return false, runnerErr
		}
		if !running {
			return false, fmt.Errorf(
				"local runtime runner is not active; inspect %s",
				runtime.Layout().RunnerLogFile(),
			)
		}

		slog.Debug(
			"local runtime database readiness probe not ready yet",
			"pid", pid,
			"error", dbReadyErr,
		)

		return false, dbReadyErr
	}, WaitParams{
		InitialBackoff: 1,
		MaxBackoff:     localRunnerMaxBackoff,
		ReadyMode:      true,
		LogPrefix:      "waiting for local database to start",
	})
}

func writeLocalArtifacts(
	deployment config.DeploymentDir,
	runtime *localruntime.Runtime,
	clusterState string,
	deploymentState string,
) error {
	state, err := runtime.LoadState()
	if err != nil {
		return err
	}

	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}

	dbPort := state.Ports["db"]
	uiPort := state.Ports["ui"]
	if dbPort <= 0 || uiPort <= 0 {
		return errors.New("local runtime ports are not initialized")
	}

	if _, err := ensureLocalSecrets(deployment); err != nil {
		return err
	}

	info := &config.DeploymentInfo{
		Backend:         config.DeploymentBackendLocal,
		DeploymentId:    exasolState.DeploymentId,
		DeploymentState: deploymentState,
		ClusterSize:     localClusterSize,
		ClusterState:    clusterState,
		Connection: &config.DeploymentConnection{
			Host:                       localLoopbackHost,
			DisplayHost:                "localhost",
			DBPort:                     dbPort,
			UIPort:                     uiPort,
			Username:                   localDefaultDatabaseUser,
			InsecureSkipCertValidation: true,
			ShellSupported:             false,
		},
		Runtime: &config.DeploymentRuntime{
			Host:                       localLoopbackHost,
			DBPort:                     dbPort,
			UIPort:                     uiPort,
			Username:                   localDefaultDatabaseUser,
			InsecureSkipCertValidation: true,
			RuntimeRoot:                runtime.Layout().RuntimeRoot(),
			ControlSocketPath:          runtime.Layout().ControlSocketPath(),
			RuntimeStatePath:           runtime.Layout().RuntimeStatePath(),
			PIDFilePath:                runtime.Layout().PIDFilePath(),
			ConsoleLogPath:             runtime.Layout().ConsoleLogFile(),
			RunnerLogPath:              runtime.Layout().RunnerLogFile(),
		},
	}

	if err := config.WriteDeploymentInfo(deployment.Root(), info); err != nil {
		return fmt.Errorf("failed to write local deployment info: %w", err)
	}

	return nil
}

func startLocalRunnerCommand(deployment config.DeploymentDir, runtime *localruntime.Runtime) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve launcher executable: %w", err)
	}
	logPath := runtime.Layout().RunnerLogFile()

	if err := os.MkdirAll(filepath.Dir(logPath), localRunnerLogDirMode); err != nil {
		return fmt.Errorf("failed to create local runner log dir: %w", err)
	}

	logFile, err := os.OpenFile(
		logPath,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		localRunnerLogFileMode,
	)
	if err != nil {
		return fmt.Errorf("failed to open local runner log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.CommandContext(
		context.Background(),
		executable,
		localRunnerCommandName,
		"--deployment-dir",
		deployment.Root(),
	)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start local runtime runner: %w", err)
	}

	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return err
	}

	deadline := time.Now().Add(localRunnerStartupTimeout)
	for time.Now().Before(deadline) {
		running, _, err := runtime.RunnerRunning()
		if err == nil && running {
			return nil
		}
		if !localruntime.IsProcessRunning(pid) {
			return fmt.Errorf("local runtime runner exited early; inspect %s", logPath)
		}

		time.Sleep(localRunnerPollInterval)
	}

	return nil
}
