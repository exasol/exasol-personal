// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/util"
)

const lockUnavailableMessage = "Deployment directory is locked by another operation. " +
	"Please wait. Do not use `unlock` unless you are certain that no other exasol " +
	"launcher instance is running."

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

	if exclusive {
		err = mutex.AcquireExclusive(ctx)
	} else {
		err = mutex.AcquireShared(ctx)
	}

	if err != nil {
		return mapLockAcquireError(ctx, deployment, err)
	}

	releaseCtx := context.WithoutCancel(ctx)
	release := func() error {
		if exclusive {
			return mutex.ReleaseExclusive(releaseCtx)
		}

		return mutex.ReleaseShared(releaseCtx)
	}
	releaseOnce := releaseOnInterruptOnce(release)

	var callbackErr error
	defer func() {
		releaseErr := releaseOnce()
		if releaseErr == nil {
			return
		}
		if callbackErr == nil {
			callbackErr = releaseErr
			return
		}
		callbackErr = errors.Join(callbackErr, releaseErr)
	}()

	callbackErr = callWithPanicSafetyError(function, deployment)

	return callbackErr
}

func mapLockAcquireError(ctx context.Context, deployment config.DeploymentDir, err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	if !errors.Is(err, directorymutex.ErrAcquireTimeout) {
		return err
	}
	if ctx == nil {
		return &deploymentDirectoryLockedError{message: lockUnavailableMessage}
	}

	statusCtx := context.WithoutCancel(ctx)
	status, statusErr := GetStatus(statusCtx, deployment, false)
	if statusErr != nil {
		return &deploymentDirectoryLockedError{message: lockUnavailableMessage}
	}
	if status != nil && status.Status == StatusOperationInProgress && status.Message != "" {
		return &deploymentDirectoryLockedError{message: status.Message}
	}

	return &deploymentDirectoryLockedError{message: lockUnavailableMessage}
}

func callWithPanicSafetyError(
	function func(deployment config.DeploymentDir) error,
	deployment config.DeploymentDir,
) error {
	defer func() {
		if recovered := recover(); recovered != nil {
			panic(recovered)
		}
	}()

	return function(deployment)
}

func releaseOnInterruptOnce(release func() error) func() error {
	// Run the release function no matter what
	unregister, _ := util.RegisterOnceSignalHandler(func() {
		_ = release()
	})

	return func() error {
		if unregister() {
			return release()
		}

		return nil
	}
}

func deploymentLockMessage(err error) string {
	var lockErr *deploymentDirectoryLockedError
	if errors.As(err, &lockErr) {
		return lockErr.message
	}

	return ""
}
