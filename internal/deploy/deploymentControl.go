// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
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
	deployment config.DeploymentDir,
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

	LogDeploymentStatus(deployment)

	return ErrUnexpectedDeploymentStatus
}

//
//nolint:revive
func Start(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	waitTimeoutSeconds int,
) error {
	return withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			slog.Info("starting deployment. this may take a few minutes")

			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}

			if err := WorkflowStatePermitsStart(exasolState, deployment); err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}

			// Set the workflowstate to start operation in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.StartOperation,
			}, deployment)
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
				}, deployment)
			})

			// Fallback cleanup
			defer unregister()

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			logBuffer := task_runner.NewLogBuffer()

			if err = doPowerControl(ctx, deployment, powerStart); err != nil {
				logBuffer.ReplayLogMessages(ctx)
				slog.Error("failed to start VMs")

				return err
			}

			err = applyAction(
				ctx,
				deployment,
				"",
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
				n11Details, err := Getn11Details(deployment)
				if err != nil {
					return false, err
				}
				if n11Details.Host != "" {
					// Resources up-to-date
					return true, nil
				}
				err = applyAction(
					ctx,
					deployment,
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

			if err := WaitForDatabaseStarted(waitCtx, deployment); err != nil {
				slog.Error("database did not become operational with timeout", "error", err.Error())
				return err
			}

			// Stop handling interrupts before committing final running state
			unregister()

			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateRunning{}, deployment,
			)
			if err != nil {
				slog.Error("failed to set workflow state", "error", err.Error())
			}

			slog.Info("database is ready to accept connections")

			slog.Warn(
				"parameters such as instance IP, DNS name may have changed. " +
					"Printing the connection instructions",
			)

			connectionInstructions, err := getConnectionInstructionsTextUnsafe(ctx, deployment)
			if err != nil {
				return err
			}

			return writeConnectionInstructionsFile(deployment, connectionInstructions)
		})
}

func WorkflowStatePermitsStop(
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) error {
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

	LogDeploymentStatus(deployment)

	return ErrUnexpectedDeploymentStatus
}

//nolint:revive
func Stop(ctx context.Context, deployment config.DeploymentDir, verbose bool) error {
	return withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			slog.Info("stopping deployment. this may take a few minutes")

			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}

			if err = WorkflowStatePermitsStop(exasolState, deployment); err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}

			// Set the workflowstate to stop in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.StopOperation,
			}, deployment)
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
				}, deployment)
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
				deployment,
				"",
				util.CombineWriters(logBuffer, externalCommandOutput),
				util.CombineWriters(logBuffer, externalCommandOutput),
			)
			if err != nil {
				logBuffer.ReplayLogMessages(ctx)
				slog.Error("failed to stop the deployment")

				return err
			}

			if err = doPowerControl(ctx, deployment, powerStop); err != nil {
				logBuffer.ReplayLogMessages(ctx)
				slog.Error("failed to stop VMs")

				return err
			}

			// Stop handling interrupts before committing final stopped state
			unregister()

			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateStopped{}, deployment)
			if err != nil {
				slog.Error("failed to set workflow state", "error", err.Error())
			}

			slog.Info("database is stopped and no longer accepts connections")

			return nil
		})
}

type powerAction int

const (
	powerStop  powerAction = iota
	powerStart powerAction = iota
)

func doPowerControl(
	ctx context.Context,
	deployment config.DeploymentDir,
	action powerAction,
) error {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return fmt.Errorf("failed to read infrastructure manifest for power control: %w", err)
	}

	if manifest.PowerControl == nil {
		return nil
	}

	nodeDetails, err := config.ReadNodeDetails(deployment)
	if err != nil {
		return fmt.Errorf("failed to read node details for power control: %w", err)
	}

	instanceIDs := make([]string, 0, len(nodeDetails.Nodes))
	for _, node := range nodeDetails.Nodes {
		if node.InstanceId != "" {
			instanceIDs = append(instanceIDs, node.InstanceId)
		}
	}

	switch manifest.PowerControl.Provider {
	case "hetzner":
		if action == powerStop {
			return hetznerStopServers(ctx, instanceIDs)
		}

		return hetznerStartServers(ctx, instanceIDs)
	default:
		return fmt.Errorf("unknown power control provider: %q", manifest.PowerControl.Provider)
	}
}

func applyAction(
	ctx context.Context,
	deployment config.DeploymentDir,
	startStopArg string,
	out, outErr io.Writer,
) error {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return err
	}
	if manifest.Tofu == nil {
		slog.Info("no tofu configuration defined; skipping apply action")
		return nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)
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
