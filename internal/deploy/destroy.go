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
			return destroyLocked(ctx, deployment, verbose)
		}); err != nil {
		return err
	}

	return nil
}

func destroyLocked(ctx context.Context, deployment config.DeploymentDir, verbose bool) error {
	slog.Info("Destroying deployment and releasing all resources")

	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}

	// Set the workflowstate to destroy in-progress
	if err := exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateOperationInProgress{
		Operation: config.DestroyOperation,
	}, deployment); err != nil {
		slog.Error("failed to set workflow state to in-progress", "error", err.Error())
	}

	return runDestroyBackend(ctx, exasolState, deployment, verbose)
}

// runDestroyBackend registers the interruption signal handler, invokes the
// backend's destroy operation, and commits the final initialized state. It is
// split out from destroyLocked so the signal-handler lifetime (registered
// right before the backend call, unregistered right after) stays scoped
// to a single, easy-to-audit block.
//
//nolint:revive // verbose mirrors the command-level --verbose flag.
func runDestroyBackend(
	ctx context.Context,
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
	verbose bool,
) error {
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
	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		return err
	}

	if err := os.Remove(deployment.ConnectionInstructionsPath()); err != nil {
		slog.Debug(fmt.Sprintf("failed to remove connection instructions file: %v", err))
	}

	slog.Info("Successfully destroyed deployment and released all resources")

	return nil
}

func markDestroyInterrupted(
	exasolState *config.ExasolPersonalState,
	deployment config.DeploymentDir,
	destroyErr error,
) error {
	return markOperationInterrupted(exasolState, deployment, config.DestroyOperation, destroyErr)
}
