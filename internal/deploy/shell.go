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

// OpenHostShell starts an interactive shell using stdin stdout & stderr.
func OpenHostShell(ctx context.Context, deploymentDir string, selectedNode string) error {
	return withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		sshRemote, err := sshRemoteForNodeUnsafe(dir, selectedNode)
		if err != nil {
			return err
		}

		return sshRemote.Shell(ctx, os.Stdout, os.Stderr)
	})
}

// OpenCOSShell opens an interactive COS session via the access node (n11).
func OpenCOSShell(ctx context.Context, deploymentDir string) error {
	return withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		sshRemote, err := sshRemoteForNodeUnsafe(dir, "n11")
		if err != nil {
			return err
		}

		cosCommand := "/usr/bin/env bash /opt/exasol_launcher/scripts/connectCos.sh"

		return sshRemote.RunInteractiveCommand(ctx, cosCommand, os.Stdout, os.Stderr)
	})
}

func sshRemoteForNodeUnsafe(deploymentDir string, selectedNode string) (*remote.SSHRemote, error) {
	nodeDetails, err := config.ReadNodeDetails(deploymentDir)
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

	sshDetails, err := nodeDetails.GetSSHDetails(selectedNode)
	if err != nil {
		return nil, err
	}

	keyFilePath := sshDetails.KeyFile.Abs(deploymentDir)
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
