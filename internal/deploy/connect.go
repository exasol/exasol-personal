// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/util"
)

type Output struct {
	Username string `yaml:"username"`
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
	Keyfile  string `yaml:"keyfile"`
}

func WorkflowStatePermitsConnect(deploymentDir string) error {
	// Check we are in state init or deploymentInterrupted.
	// - We are allowed to retry deployment when in the interrupted state, because
	//   tofu apply is idempotent
	exasolState, err := config.ReadExasolPersonalState(deploymentDir)
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

	LogDeploymentStatus(deploymentDir)

	return ErrUnexpectedDeploymentStatus
}

func Connect(ctx context.Context, opts *connect.Opts, deploymentDir string) error {
	err := withDeploymentSharedLock(ctx, deploymentDir,
		func(dir string) error {
			slog.Debug("establishing connection to Exaol DB")

			if err := WorkflowStatePermitsConnect(dir); err != nil {
				return util.LoggedError(err, "run `status` for more information")
			}

			return connect.Connect(ctx, opts, dir)
		})

	return err
}
