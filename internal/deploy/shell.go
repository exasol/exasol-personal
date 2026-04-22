// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/remote"
)

var ErrNoNodesFound = errors.New("no nodes found in the active deployment")
var ErrLocalShellUnsupported = errors.New("shell access is unsupported for local deployments")

// OpenHostShell starts an interactive shell using stdin stdout & stderr.
func OpenHostShell(
	ctx context.Context,
	deployment config.DeploymentDir,
	selectedNode string,
) error {
	if _, err := config.ReadLocalDeploymentInfo(deployment.Root()); err == nil {
		return fmt.Errorf(
			"%w: `shell host` is unavailable because local deployments do not expose SSH host access",
			ErrLocalShellUnsupported,
		)
	}

	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		sshRemote, err := sshRemoteForNodeUnsafe(deployment, selectedNode)
		if err != nil {
			return err
		}

		return sshRemote.Shell(ctx, os.Stdout, os.Stderr)
	})
}

// OpenCOSShell opens an interactive COS session via the access node (n11).
func OpenCOSShell(ctx context.Context, deployment config.DeploymentDir) error {
	if _, err := config.ReadLocalDeploymentInfo(deployment.Root()); err == nil {
		return fmt.Errorf(
			"%w: `shell container` is unavailable because local deployments do not expose COS shells",
			ErrLocalShellUnsupported,
		)
	}

	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		sshRemote, err := sshRemoteForNodeUnsafe(deployment, "n11")
		if err != nil {
			return err
		}

		cosCommand := "/usr/bin/env bash /opt/exasol_launcher/scripts/connectCos.sh"

		return sshRemote.RunInteractiveCommand(ctx, cosCommand, os.Stdout, os.Stderr)
	})
}

func sshRemoteForNodeUnsafe(
	deployment config.DeploymentDir,
	selectedNode string,
) (*remote.SSHRemote, error) {
	nodeDetails, err := config.ReadNodeDetails(deployment)
	if err != nil {
		return nil, err
	}

	if selectedNode == "" {
		nodes := nodeDetails.ListNodes()

		if len(nodes) == 0 {
			return nil, ErrNoNodesFound
		}

		selectedNode = nodes[0]
	}

	sshDetails, err := nodeDetails.GetSSHDetails(selectedNode, deployment)
	if err != nil {
		return nil, err
	}

	keyFilePath := sshDetails.KeyFile
	keyData, err := os.ReadFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w: could not read SSH key file %s", err, keyFilePath)
	}

	sshRemote := remote.NewSshRemote(&remote.SSHConnectionOptions{
		Host: sshDetails.Host,
		User: sshDetails.User,
		Port: sshDetails.Port,
		Key:  keyData,
	})

	return sshRemote, nil
}
