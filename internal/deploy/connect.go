// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
)

type Output struct {
	Username string `yaml:"username"`
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
	Keyfile  string `yaml:"keyfile"`
}

func WorkflowStatePermitsConnect(deployment config.DeploymentDir) error {
	// Check we are in state init or deploymentInterrupted.
	// - We are allowed to retry deployment when in the interrupted state, because
	//   tofu apply is idempotent
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		slog.Error("failed to read exasol personal state")
		return err
	}

	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		slog.Error("failed to read workflow state")
		return err
	}

	if _, ok := workflowState.(*config.WorkflowStateRunning); ok {
		return nil
	}

	return newBlockedStateError(deployment, ErrUnexpectedDeploymentStatus)
}

func Connect(ctx context.Context, opts *connect.Opts, deployment config.DeploymentDir) error {
	err := withDeploymentSharedLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			slog.Debug("establishing connection to Exaol DB")

			if err := WorkflowStatePermitsConnect(deployment); err != nil {
				return err
			}

			connectionInfo, err := config.ResolveConnectionInfo(deployment)
			if err != nil {
				return err
			}

			if err := connect.Connect(ctx, opts, deployment, connectionInfo); err != nil {
				return diagnoseLocalFailure(ctx, deployment, err)
			}

			return nil
		})

	return err
}
