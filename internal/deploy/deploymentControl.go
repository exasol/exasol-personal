// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/exasol/exasol-personal/internal/util"
)

func WorkflowStatePermitsStart(
	exasolState *config.ExasolPersonalState,
	deploymentDir string,
) error {
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateStopped:
		return nil

	case *config.WorkflowStateOperationInProgress:
		switch state.Operation {
		case config.StartOperation:
			return nil
		default:
			return ErrUnspportedOperation
		}

	case *config.WorkflowStateInterrupted:
		switch state.InterruptedDuringOperation {
		case config.StartOperation,
			config.StopOperation:
			return nil
		default:
			return ErrUnspportedOperation
		}
	}

	LogDeploymentStatus(deploymentDir)

	return ErrUnexpectedDeploymentStatus
}

//nolint:revive
func Start(ctx context.Context, deploymentDir string, verbose bool, waitTimeoutSeconds int) error {
	return withDeploymentExclusiveLock(ctx, deploymentDir,
		func(deploymentDir string) error {
			slog.Info("starting deployment. this may take a few minutes")

			exasolState, err := config.ReadExasolPersonalState(deploymentDir)
			if err != nil {
				return err
			}

			if err := WorkflowStatePermitsStart(exasolState, deploymentDir); err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}

			// Set the workflowstate to start operation in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.StartOperation,
			}, deploymentDir)
			if err != nil {
				slog.Error("failed to set workflow state to in-progress", "error", err.Error())
			}

			// Register signal handler for catching interruptions and set state
			// in case of interruption
			unregister, _ := util.RegisterOnceSignalHandler(func() {
				slog.Warn("Start Operation interrupted")
				_ = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
					Error:                      "Start Operation interrupted via signal",
					InterruptedDuringOperation: config.StartOperation,
				}, deploymentDir)
			})

			// Fallback cleanup
			defer unregister()

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			logBuffer := task_runner.NewLogBuffer()

			err = applyAction(
				ctx,
				deploymentDir,
				"power_state=running",
				util.CombineWriters(logBuffer, externalCommandOutput),
				util.CombineWriters(logBuffer, externalCommandOutput),
			)
			if err != nil {
				logBuffer.ReplayLogMessages(ctx)
				slog.Error("failed to start deployment")

				return err
			}

			// Attempt to refresh config/infrastructure
			instPollCond := func(ctx context.Context) (bool, error) {
				n11Details, err := Getn11Details(deploymentDir)
				if err != nil {
					return false, err
				}
				if n11Details.Host != "" {
					// Resources up-to-date
					return true, nil
				}
				err = applyAction(
					ctx,
					deploymentDir,
					"", // No Arg needed for refresh
					util.CombineWriters(logBuffer, externalCommandOutput),
					util.CombineWriters(logBuffer, externalCommandOutput),
				)
				if err != nil {
					logBuffer.ReplayLogMessages(ctx)
					slog.Error("ApplyAction failed while refreshing", "error", err)

					return false, err
				}

				return false, nil
			}

			waitCtx, cancel := context.WithTimeout(
				ctx,
				time.Duration(InstanceRefreshTimeoutSeconds)*time.Second,
			)
			defer cancel()

			err = PollWithBackoff(waitCtx, instPollCond, WaitParams{
				InitialBackoff: StartedInitialBackoff,
				MaxBackoff:     StartedMaxBackoff,
				LogPrefix:      "waiting to update EC2 Resources",
			})
			if err != nil {
				slog.Error("Updated EC2 resources not available in time")
				return err
			}

			// Use provided timeout if > 0; otherwise fallback to default (seconds)
			if waitTimeoutSeconds <= 0 {
				waitTimeoutSeconds = StartedDefaultTimeoutSeconds
			}

			// After starting the instance, wait until the database is ready for connections
			// Enforce the provided timeout using a child context with deadline
			waitCtx, cancel = context.WithTimeout(ctx,
				time.Duration(waitTimeoutSeconds)*time.Second,
			)
			defer cancel()

			if err := WaitForDatabaseStarted(waitCtx, deploymentDir); err != nil {
				slog.Error("database did not become operational with timeout", "error", err.Error())
				return err
			}

			// Stop handling interrupts before committing final running state
			unregister()

			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateRunning{}, deploymentDir,
			)
			if err != nil {
				slog.Error("failed to set workflow state", "error", err.Error())
			}

			slog.Info("database is ready to accept connections")

			slog.Warn(
				"parameters such as instance IP, DNS name may have changed. " +
					"Printing the connection instructions",
			)

			connectionInstructions, err := getConnectionInstructionsTextUnsafe(ctx, deploymentDir)
			if err != nil {
				return err
			}

			return writeConnectionInstructionsFile(deploymentDir, connectionInstructions)
		})
}

func WorkflowStatePermitsStop(exasolState *config.ExasolPersonalState, deploymentDir string) error {
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateRunning:
		return nil

	case *config.WorkflowStateOperationInProgress:
		switch state.Operation {
		case config.StopOperation:
			return nil
		default:
			return ErrUnspportedOperation
		}

	case *config.WorkflowStateInterrupted:
		switch state.InterruptedDuringOperation {
		case config.StartOperation,
			config.StopOperation,
			config.DestroyOperation:
			return nil
		default:
			return ErrUnspportedOperation
		}
	}

	LogDeploymentStatus(deploymentDir)

	return ErrUnexpectedDeploymentStatus
}

//nolint:revive
func Stop(ctx context.Context, deploymentDir string, verbose bool) error {
	return withDeploymentExclusiveLock(ctx, deploymentDir,
		func(dir string) error {
			slog.Info("stopping deployment. this may take a few minutes")

			exasolState, err := config.ReadExasolPersonalState(dir)
			if err != nil {
				return err
			}

			if err = WorkflowStatePermitsStop(exasolState, dir); err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}

			// Set the workflowstate to stop in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.StopOperation,
			}, dir)
			if err != nil {
				slog.Error("failed to set workflow state to in-progress", "error", err.Error())
			}

			// Register signal handler for catching interruptions and set state
			// in case of interruption
			unregister, _ := util.RegisterOnceSignalHandler(func() {
				slog.Warn("Stop Operation interrupted")
				_ = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
					Error:                      "Stop Operation interrupted via signal",
					InterruptedDuringOperation: config.StopOperation,
				}, dir)
			})

			// Fallback cleanup
			defer unregister()

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			logBuffer := task_runner.NewLogBuffer()

			err = applyAction(
				ctx,
				dir,
				"power_state=stopped",
				util.CombineWriters(logBuffer, externalCommandOutput),
				util.CombineWriters(logBuffer, externalCommandOutput),
			)
			if err != nil {
				logBuffer.ReplayLogMessages(ctx)
				slog.Error("failed to stop the deployment")
				// should this be a failure state that requires destroy?
				return err
			}

			// Stop handling interrupts before committing final stopped state
			unregister()

			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateStopped{}, dir)
			if err != nil {
				slog.Error("failed to set workflow state", "error", err.Error())
			}

			slog.Info("database is stopped and no longer accepts connections")

			return nil
		})
}

func applyAction(
	ctx context.Context,
	deploymentDir string,
	startStopArg string,
	out, outErr io.Writer,
) error {
	manifest, err := config.ReadInfrastructureManifest(deploymentDir)
	if err != nil {
		return err
	}
	if manifest.Tofu == nil {
		slog.Info("no tofu configuration defined; skipping apply action")
		return nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deploymentDir, *manifest.Tofu)
	if err := tofu.ApplyAction(
		ctx,
		*tofuCfg,
		startStopArg,
		out,
		outErr,
	); err != nil {
		slog.Error("Tofu Apply Failed:", "error", err.Error())
		return err
	}

	return nil
}
