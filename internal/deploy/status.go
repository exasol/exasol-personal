// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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

type StatusOutput struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func StatusJSONFormatter(status StatusOutput) (string, error) {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func StatusTextFormatter(status StatusOutput) (string, error) {
	output := ""
	output += fmt.Sprintf("Status: %s\n", status.Status)

	if status.Message != "" {
		output += fmt.Sprintf("Message: %s\n", status.Message)
	}

	return output, nil
}

type StatusFormatter func(status StatusOutput) (string, error)

func Status(ctx context.Context, deploymentDir string, formatter StatusFormatter) (string, error) {
	return statusWithFormatter(ctx, deploymentDir, GetStatusWithLock, formatter)
}

func StatusUnsafe(
	ctx context.Context,
	deploymentDir string,
	formatter StatusFormatter,
) (string, error) {
	return statusWithFormatter(ctx, deploymentDir, GetStatus, formatter)
}

type statusGetter func(ctx context.Context,
	deploymentDir string,
	checkConnection bool,
) (*StatusOutput, error)

func statusWithFormatter(
	ctx context.Context,
	deploymentDir string,
	getStatus statusGetter,
	format StatusFormatter,
) (string, error) {
	slog.Debug("reading deployment status")

	status, err := getStatus(ctx, deploymentDir, true)
	if err != nil || status == nil {
		return "", err
	}

	return format(*status)
}

//nolint:contextcheck
func LogDeploymentStatus(deploymentDir string) {
	status, err := GetStatus(context.Background(), deploymentDir, true)
	if err != nil {
		slog.Error("failed to get status", "error", err.Error())
	}
	slog.Info(status.Message)
}

func GetStatusWithLock(
	ctx context.Context,
	deploymentDir string,
	checkConnection bool,
) (*StatusOutput, error) {
	var status *StatusOutput
	err := withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		var getErr error
		status, getErr = GetStatus(ctx, dir, checkConnection)

		return getErr
	})
	if err != nil {
		if errors.Is(err, ErrDeploymentDirectoryLocked) {
			lockMessage := deploymentLockMessage(err)
			if lockMessage == "" {
				lockMessage = lockUnavailableMessage
			}

			return &StatusOutput{
				Status:  StatusOperationInProgress,
				Message: lockMessage,
			}, nil
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		return nil, err
	}

	return status, nil
}

// nolint: revive
func GetStatus(
	ctx context.Context,
	deploymentDir string,
	checkConnection bool,
) (*StatusOutput, error) {
	exasolState, err := config.ReadExasolPersonalState(deploymentDir)
	if err != nil {
		return nil, err
	}

	workflowState, err := exasolState.GetWorkflowState()
	if errors.Is(err, config.ErrMissingConfigFile) {
		return &StatusOutput{
			Status: StatusNotInitialized,
			Message: "No workflow state file was found. " +
				"Run `init` or `install` to start a new deployment in this directory.",
		}, nil
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
			Status: StatusOperationInProgress,
			Message: fmt.Sprintf(
				"Operation '%s' is currently in progress. Please wait. "+
					"Do not use `unlock` unless you are certain that no other exasol "+
					"launcher instance is running.", currentOperation,
			),
		}, nil

	case *config.WorkflowStateDeploymentFailed:
		slog.Debug("Workflow State Deployment Failed")

		return &StatusOutput{
			Status:  StatusDeploymentFailed,
			Message: "Deployment failed. Run `destroy` before attempting to run `deploy` again.",
			Error:   state.Error,
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
			slog.Debug("Testing database connection")

			err = verifyDatabaseConnection(ctx, deploymentDir)
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
