// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	StatusNotInitialized           = "not_initialized"
	StatusInitialized              = "initialized"
	StatusOperationInProgress      = "operation_in_progress"
	StatusInterrupted              = "interrupted"
	StatusStopped                  = "stopped"
	StatusRunning                  = "running"
	StatusDeploymentFailed         = "deployment_failed"
	StatusDatabaseConnectionFailed = "database_connection_failed"
	StatusDatabaseReady            = "database_ready"
)

const notInitializedMessage = "No Exasol Personal deployment exists in this " +
	"deployment directory yet. " +
	"Run `exasol install <infra preset>` to create and start one, or pass " +
	"`--deployment-dir <path>` to inspect an existing deployment."

type StatusOutput struct {
	DeploymentDir string `json:"deploymentDir"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
	Error         string `json:"error,omitempty"`
}

func Status(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*StatusOutput, error) {
	return statusWithGetter(ctx, deployment, GetStatusWithLock)
}

func StatusUnsafe(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*StatusOutput, error) {
	return statusWithGetter(ctx, deployment, GetStatus)
}

type statusGetter func(ctx context.Context,
	deployment config.DeploymentDir,
	checkConnection bool,
) (*StatusOutput, error)

func statusWithGetter(
	ctx context.Context,
	deployment config.DeploymentDir,
	getStatus statusGetter,
) (*StatusOutput, error) {
	slog.Debug("reading deployment status")

	status, err := getStatus(ctx, deployment, true)
	if err != nil || status == nil {
		return nil, err
	}
	status.DeploymentDir = deployment.Root()

	return status, nil
}

func GetStatusWithLock(
	ctx context.Context,
	deployment config.DeploymentDir,
	checkConnection bool,
) (*StatusOutput, error) {
	var status *StatusOutput
	err := withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		var getErr error
		status, getErr = GetStatus(ctx, deployment, checkConnection)

		return getErr
	})
	if err != nil {
		return statusFromLockError(err)
	}

	return status, nil
}

func statusFromLockError(err error) (*StatusOutput, error) {
	if errors.Is(err, os.ErrNotExist) {
		return notInitializedStatus(), nil
	}
	if errors.Is(err, ErrDeploymentDirectoryLocked) {
		return operationInProgressStatus(deploymentLockMessage(err)), nil
	}
	if errors.Is(err, context.Canceled) {
		return nil, err
	}

	return nil, err
}

func operationInProgressStatus(lockMessage string) *StatusOutput {
	if lockMessage == "" {
		lockMessage = lockUnavailableMessage
	}

	return &StatusOutput{
		Status:  StatusOperationInProgress,
		Message: lockMessage,
	}
}

// nolint: revive
func GetStatus(
	ctx context.Context,
	deployment config.DeploymentDir,
	checkConnection bool,
) (*StatusOutput, error) {
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		if errors.Is(err, config.ErrMissingConfigFile) {
			return notInitializedStatus(), nil
		}

		return nil, err
	}

	workflowState, err := exasolState.GetWorkflowState()
	if errors.Is(err, config.ErrMissingConfigFile) {
		return notInitializedStatus(), nil
	} else if err != nil {
		return nil, err
	}

	switch state := workflowState.(type) {
	case *config.WorkflowStateInitialized:
		slog.Debug("Workflow State Initialized")

		return &StatusOutput{
			Status: StatusInitialized,
			Message: "Ready for deployment. " +
				"Run `deploy` to provision resources and start the database.",
		}, nil

	case *config.WorkflowStateInterrupted:
		slog.Debug("Workflow State Deployment Interrupted")

		msg := buildInterruptMessage(state.InterruptedDuringOperation)

		return &StatusOutput{
			Status:  StatusInterrupted,
			Message: msg,
			Error:   state.Error,
		}, nil

	case *config.WorkflowStateOperationInProgress:
		slog.Debug("Workflow State Operation In Progress")
		currentOperation := state.Operation

		return &StatusOutput{
			Status:  StatusOperationInProgress,
			Message: staleOperationInProgressMessage(currentOperation),
		}, nil

	case *config.WorkflowStateDeploymentFailed:
		slog.Debug("Workflow State Deployment Failed")

		return &StatusOutput{
			Status: StatusDeploymentFailed,
			Message: "Deployment failed. Fix the problem and run `deploy` or the same " +
				"`install` command again, or run `destroy` to clean up resources.",
			Error: state.Error,
		}, nil

	case *config.WorkflowStateStopped:
		slog.Debug("Workflow State Stopped")

		return &StatusOutput{
			Status: StatusStopped,
			Message: "Deployment stopped. Run `start` to restart " +
				"or `destroy` to delete resources.",
		}, nil

	case *config.WorkflowStateRunning:
		if checkConnection {
			if s := localVMStoppedStatus(ctx, deployment); s != nil {
				return s, nil
			}

			slog.Debug("Testing database connection")

			err = verifyDatabaseConnection(ctx, deployment)
			if err != nil {
				slog.Debug("Database connection verification failed")

				//nolint:nilerr
				return &StatusOutput{
					Status: StatusDatabaseConnectionFailed,
					Error:  err.Error(),
				}, nil
			}

			slog.Debug("Database connection verification succeeded")

			return &StatusOutput{
				Status:  StatusDatabaseReady,
				Message: "The database is running and ready for connections.",
			}, nil
		}

		return &StatusOutput{
			Status:  StatusRunning,
			Message: "The database has been started. Run `status` to check database connection.",
		}, nil

	default:
		panic("unknown workflow state")
	}
}

// localVMStoppedStatus returns a StatusStopped output when the local VM daemon
// is not running, so GetStatus can short-circuit before the slower DB probe.
// Returns nil for non-local deployments or when the VM is running.
func localVMStoppedStatus(ctx context.Context, deployment config.DeploymentDir) *StatusOutput {
	if !isLocalDeployment(deployment) {
		return nil
	}

	vmStatus, err := getLocalVMStatus(ctx, deployment)
	if err != nil {
		slog.Debug("local VM status check failed", "error", err)
		return nil
	}

	if !vmStatus.Running {
		slog.Debug("local VM is not running")

		return &StatusOutput{
			Status:  StatusStopped,
			Message: "Deployment stopped. Run `start` to restart or `destroy` to delete resources.",
		}
	}

	return nil
}

func staleOperationInProgressMessage(operation string) string {
	switch operation {
	case config.DestroyOperation:
		return "A previous destroy operation did not finish cleanly. " +
			"If deployment resources may still exist, run `destroy` again. " +
			"If resources were already deleted or cannot be destroyed through the launcher, " +
			"run `remove` to delete only the local deployment directory."
	case config.DeployOperation:
		return "A previous deploy operation did not finish cleanly. " +
			"Fix the problem and run `deploy` or the same `install` command again, " +
			"or run `destroy` to clean up resources."
	default:
		return fmt.Sprintf(
			"A previous %s operation did not finish cleanly. Retry the operation, "+
				"or run `status` again to inspect the deployment.",
			operation,
		)
	}
}

func notInitializedStatus() *StatusOutput {
	return &StatusOutput{
		Status:  StatusNotInitialized,
		Message: notInitializedMessage,
	}
}

func buildInterruptMessage(operation string) string {
	msg := fmt.Sprintf("Interrupted during %s.", operation)
	switch operation {
	case config.DeployOperation:
		msg += " Please run `deploy`."
	case config.DestroyOperation:
		msg += " Please run `destroy`."
	default:
		msg += " Please run `start` or `stop`."
	}

	return msg
}
