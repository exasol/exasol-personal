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

// OpenHostShell starts an interactive shell using stdin stdout & stderr. If
// command is non-empty, it is run non-interactively instead of starting a
// shell, and the connection closes once it completes.
func OpenHostShell(
	ctx context.Context,
	deployment config.DeploymentDir,
	selectedNode string,
	command string,
) error {
	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		backend, err := newDeploymentBackendForDeployment(deployment)
		if err != nil {
			return err
		}

		return backend.OpenHostShell(ctx, selectedNode, command)
	})
}

// OpenCOSShell opens an interactive COS session via the access node (n11).
func OpenCOSShell(ctx context.Context, deployment config.DeploymentDir) error {
	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		backend, err := newDeploymentBackendForDeployment(deployment)
		if err != nil {
			return err
		}

		return backend.OpenCOSShell(ctx)
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
