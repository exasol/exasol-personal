// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
)

const lockUnavailableMessage = "Deployment directory is locked by another exasol command. " +
	"Wait for it to finish or stop it."

var (
	ErrDeploymentDirectoryLocked = errors.New("deployment directory is locked")
	ErrMissingContext            = errors.New("missing context")
)

type deploymentDirectoryLockedError struct {
	message string
}

func (e *deploymentDirectoryLockedError) Error() string {
	return e.message
}

func (*deploymentDirectoryLockedError) Is(target error) bool {
	return target == ErrDeploymentDirectoryLocked
}

func withDeploymentSharedLock(
	ctx context.Context,
	deployment config.DeploymentDir,
	function func(deployment config.DeploymentDir) error,
) error {
	return withDeploymentLock(ctx, deployment, false, function)
}

func withDeploymentExclusiveLock(
	ctx context.Context,
	deployment config.DeploymentDir,
	function func(deployment config.DeploymentDir) error,
) error {
	return withDeploymentLock(ctx, deployment, true, function)
}

// nolint: revive
func withDeploymentLock(
	ctx context.Context,
	deployment config.DeploymentDir,
	exclusive bool,
	function func(deployment config.DeploymentDir) error,
) error {
	if ctx == nil {
		return ErrMissingContext
	}

	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		return err
	}

	wrapped := func(any) error {
		return function(deployment)
	}

	if exclusive {
		err = mutex.WithExclusive(ctx, nil, wrapped)
	} else {
		err = mutex.WithShared(ctx, nil, wrapped)
	}

	if err != nil {
		return mapDeploymentLockError(deployment, err)
	}

	return nil
}

func mapDeploymentLockError(deployment config.DeploymentDir, err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	if !errors.Is(err, directorymutex.ErrAcquireTimeout) {
		return err
	}

	if message := olderLauncherLockMessage(deployment); message != "" {
		return &deploymentDirectoryLockedError{message: message}
	}
	if operation := lockedOperationName(deployment); operation != "" {
		return &deploymentDirectoryLockedError{
			message: activeOperationInProgressMessage(operation),
		}
	}

	return &deploymentDirectoryLockedError{message: lockUnavailableMessage}
}

func lockedOperationName(deployment config.DeploymentDir) string {
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return ""
	}
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		return ""
	}
	state, ok := workflowState.(*config.WorkflowStateOperationInProgress)
	if !ok {
		return ""
	}

	return state.Operation
}

func activeOperationInProgressMessage(operation string) string {
	return "Operation '" + operation + "' is currently in progress in another exasol command. " +
		"Wait for it to finish or stop it."
}

// olderLauncherLockMessage describes a lock held by an older launcher's
// marker file, which no running process may release.
func olderLauncherLockMessage(deployment config.DeploymentDir) string {
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		return ""
	}
	status, err := mutex.Status()
	if err != nil || status.MarkerPath == "" {
		return ""
	}

	return "Deployment directory is locked by an older exasol launcher version through " +
		status.MarkerPath + ". If no older launcher is running, clear the lock with " +
		"`exasol diag unlock" + deployment.CLISelection() + "`."
}

func deploymentLockMessage(err error) string {
	if lockErr, ok := errors.AsType[*deploymentDirectoryLockedError](err); ok {
		return lockErr.message
	}

	return ""
}
