// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
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
			return newBlockedStateError(deployment, ErrUnspportedOperation)
		}

	case *config.WorkflowStateInterrupted:
		switch state.InterruptedDuringOperation {
		case config.StartOperation,
			config.StopOperation:
			return nil
		default:
			return newBlockedStateError(deployment, ErrUnspportedOperation)
		}
	}

	return newBlockedStateError(deployment, ErrUnexpectedDeploymentStatus)
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

			if err := reconcileLocalVMState(ctx, exasolState, deployment); err != nil {
				return err
			}

			if err := WorkflowStatePermitsStart(exasolState, deployment); err != nil {
				return err
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
			return newBlockedStateError(deployment, ErrUnspportedOperation)
		}

	case *config.WorkflowStateInterrupted:
		switch state.InterruptedDuringOperation {
		case config.StartOperation,
			config.StopOperation,
			config.DestroyOperation:
			return nil
		default:
			return newBlockedStateError(deployment, ErrUnspportedOperation)
		}
	}

	return newBlockedStateError(deployment, ErrUnexpectedDeploymentStatus)
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

			if err = reconcileLocalVMState(ctx, exasolState, deployment); err != nil {
				return err
			}

			if err = WorkflowStatePermitsStop(exasolState, deployment); err != nil {
				return err
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
}
