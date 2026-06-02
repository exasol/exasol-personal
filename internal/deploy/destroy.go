// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/util"
)

//nolint:revive
func Destroy(ctx context.Context, deployment config.DeploymentDir, verbose bool) error {
	if err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			slog.Info("Destroying deployment and releasing all resources")

			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}

			// Set the workflowstate to destroy in-progress
			err = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
				Operation: config.DestroyOperation,
			}, deployment)
			if err != nil {
				slog.Error("failed to set workflow state to in-progress", "error", err.Error())
			}

			// Register signal handler for catching interruptions and set state
			// in case of interruption
			unregister, _ := util.RegisterOnceSignalHandler(func() {
				slog.Warn("Destroy interrupted")
				_ = exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
					Error:                      "Destroy interrupted via signal",
					InterruptedDuringOperation: config.DestroyOperation,
				}, deployment)
			})

			defer unregister()

			manifest, err := config.ReadInfrastructureManifest(deployment)
			if err != nil {
				return markDestroyInterrupted(exasolState, deployment, err)
			}
			backend, err := newDeploymentBackend(deployment, manifest)
			if err != nil {
				return markDestroyInterrupted(exasolState, deployment, err)
			}

			var externalCommandStandardOut io.Writer
			if verbose {
				externalCommandStandardOut = os.Stderr
			}

			if err := backend.Destroy(
				ctx,
				externalCommandStandardOut,
				externalCommandStandardOut,
			); err != nil {
				unregister()

				return markDestroyInterrupted(exasolState, deployment, err)
			}

			// Stop handling interrupts before committing final initialized state
			unregister()

			// Returning to the initialized state is required so that `deploy` can be run again.
			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateInitialized{},
				deployment,
			)
			if err != nil {
				return err
			}

			err = os.Remove(deployment.ConnectionInstructionsPath())
			if err != nil {
				slog.Debug(fmt.Sprintf("failed to remove connection instructions file: %v", err))
			}

			slog.Info("Successfully destroyed deployment and released all resources")

			return nil
		}); err != nil {
		return err
	}

	return nil
}

func markDestroyInterrupted(
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
	destroyErr error,
) error {
	stateErr := exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInterrupted{
		Error:                      destroyErr.Error(),
		InterruptedDuringOperation: config.DestroyOperation,
	}, deployment)
	if stateErr != nil {
		slog.Warn("failed to persist destroy failure state", "error", stateErr)
	}

	return destroyErr
}
