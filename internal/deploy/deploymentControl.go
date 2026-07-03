// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/util"
)

type lifecycleActionDecision struct {
	shouldRun                  bool
	guidance                   string
	showConnectionInstructions bool
}

func workflowStatePermitsStart(
	ctx context.Context,
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) (lifecycleActionDecision, error) {
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return noLifecycleAction(), err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateInitialized:
		return guidanceOnly(
			"deployment is initialized but not deployed yet; run `exasol deploy` " +
				"or the same `exasol install <infra preset>` command to create and start it",
		), nil

	case *config.WorkflowStateStopped:
		return runLifecycleAction(), nil

	case *config.WorkflowStateRunning:
		status, err := getStatusForStart(ctx, deployment, true)
		if err != nil {
			return noLifecycleAction(), err
		}
		if status.Status == StatusDatabaseReady {
			return skipLifecycleActionWithConnectionInstructions(
				"database is already ready to accept connections",
			), nil
		}

		return guidanceOnly(
			"deployment state is running, but the database is not ready; run `exasol status` " +
				"for details, then run `exasol stop` and `exasol start` to restart it or " +
				"`exasol destroy` to clean up resources",
		), nil

	case *config.WorkflowStateDeploymentFailed:
		return guidanceOnly(
			"deployment is in a failed state; fix the reported problem and run `exasol deploy` " +
				"or the same `exasol install <infra preset>` command again, or run " +
				"`exasol destroy` to clean up resources",
		), nil

	case *config.WorkflowStateOperationInProgress:
		switch state.Operation {
		case config.StartOperation:
			return runLifecycleAction(), nil
		default:
			return skipLifecycleAction(
				operationInProgressGuidance(state.Operation, "start"),
			), nil
		}

	case *config.WorkflowStateInterrupted:
		switch state.InterruptedDuringOperation {
		case config.StartOperation,
			config.StopOperation:
			return runLifecycleAction(), nil
		default:
			return skipLifecycleAction(
				interruptedOperationGuidance(state.InterruptedDuringOperation, "start"),
			), nil
		}
	}

	return guidanceOnly(
		"deployment is in an unexpected state; run `exasol status` for details",
	), nil
}

var getStatusForStart = GetStatus

func noLifecycleAction() lifecycleActionDecision {
	return lifecycleActionDecision{
		shouldRun:                  false,
		guidance:                   "",
		showConnectionInstructions: false,
	}
}

func runLifecycleAction() lifecycleActionDecision {
	return lifecycleActionDecision{
		shouldRun:                  true,
		guidance:                   "",
		showConnectionInstructions: false,
	}
}

func guidanceOnly(message string) lifecycleActionDecision {
	return skipLifecycleAction(message)
}

func skipLifecycleAction(message string) lifecycleActionDecision {
	return lifecycleActionDecision{
		shouldRun:                  false,
		guidance:                   message,
		showConnectionInstructions: false,
	}
}

func skipLifecycleActionWithConnectionInstructions(message string) lifecycleActionDecision {
	return lifecycleActionDecision{
		shouldRun:                  false,
		guidance:                   message,
		showConnectionInstructions: true,
	}
}

func operationInProgressGuidance(operation, requestedAction string) string {
	return "operation `" + operation + "` is in progress or did not finish cleanly; " +
		"run `exasol status` for recovery guidance before running `exasol " + requestedAction + "`"
}

func interruptedOperationGuidance(operation, requestedAction string) string {
	return "a previous `" + operation + "` operation was interrupted; run `exasol status` " +
		"for recovery guidance before running `exasol " + requestedAction + "`"
}

func logLifecycleGuidance(message string) {
	if message != "" {
		slog.Warn(message)
	}
}

//
//nolint:revive
func Start(
	ctx context.Context,
	deployment config.DeploymentDir,
	verbose bool,
	waitTimeoutSeconds int,
) error {
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}

			if err := reconcileLocalVMState(ctx, exasolState, deployment); err != nil {
				return err
			}

			decision, err := workflowStatePermitsStart(ctx, exasolState, deployment)
			if err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}
			if !decision.shouldRun {
				logLifecycleGuidance(decision.guidance)
				if decision.showConnectionInstructions {
					return nil
				}

				return ErrLifecycleActionSkipped
			}

			slog.Info("starting deployment. this may take a few minutes")

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

			manifest, err := config.ReadInfrastructureManifest(deployment)
			if err != nil {
				return err
			}
			backend, err := newDeploymentBackend(deployment, manifest)
			if err != nil {
				return err
			}

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			if err := backend.Start(
				ctx,
				externalCommandOutput,
				externalCommandOutput,
				waitTimeoutSeconds,
			); err != nil {
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
	if errors.Is(err, ErrDeploymentDirectoryLocked) {
		slog.Warn(err.Error())
		return ErrLifecycleActionSkipped
	}

	return err
}

func workflowStatePermitsStop(
	exasolState *config.ExasolPersonalState,
) (lifecycleActionDecision, error) {
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return noLifecycleAction(), err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateInitialized:
		return guidanceOnly(
			"deployment is initialized but not deployed yet; there is nothing to stop; " +
				"run `exasol deploy` or the same `exasol install <infra preset>` command " +
				"to create and start it",
		), nil

	case *config.WorkflowStateRunning:
		return runLifecycleAction(), nil

	case *config.WorkflowStateStopped:
		return guidanceOnly("deployment is already stopped"), nil

	case *config.WorkflowStateDeploymentFailed:
		return guidanceOnly(
			"deployment is in a failed state; run `exasol status` for details, then retry " +
				"`exasol deploy` or run `exasol destroy` to clean up resources",
		), nil

	case *config.WorkflowStateOperationInProgress:
		switch state.Operation {
		case config.StopOperation:
			return runLifecycleAction(), nil
		default:
			return skipLifecycleAction(
				operationInProgressGuidance(state.Operation, "stop"),
			), nil
		}

	case *config.WorkflowStateInterrupted:
		switch state.InterruptedDuringOperation {
		case config.StartOperation,
			config.StopOperation,
			config.DestroyOperation:
			return runLifecycleAction(), nil
		default:
			return skipLifecycleAction(
				interruptedOperationGuidance(state.InterruptedDuringOperation, "stop"),
			), nil
		}
	}

	return guidanceOnly(
		"deployment is in an unexpected state; run `exasol status` for details",
	), nil
}

//nolint:revive
func Stop(ctx context.Context, deployment config.DeploymentDir, verbose bool) error {
	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}

			if err = reconcileLocalVMState(ctx, exasolState, deployment); err != nil {
				return err
			}

			decision, err := workflowStatePermitsStop(exasolState)
			if err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}
			if !decision.shouldRun {
				logLifecycleGuidance(decision.guidance)

				return nil
			}

			slog.Info("stopping deployment. this may take a few minutes")

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

			manifest, err := config.ReadInfrastructureManifest(deployment)
			if err != nil {
				return err
			}
			backend, err := newDeploymentBackend(deployment, manifest)
			if err != nil {
				return err
			}

			var externalCommandOutput io.Writer
			if verbose {
				externalCommandOutput = os.Stderr
			}

			if err := backend.Stop(
				ctx,
				externalCommandOutput,
				externalCommandOutput,
			); err != nil {
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
	if errors.Is(err, ErrDeploymentDirectoryLocked) {
		slog.Warn(err.Error())
		return nil
	}

	return err
}
