// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/remote"
)

var ErrNoNodesFound = errors.New("no nodes found in the active deployment")

// Start an interactive shell using stdin stdout & stderr.
func Shell(ctx context.Context, deploymentDir string, selectedNode string) error {
	return withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		return shellUnsafe(ctx, dir, selectedNode)
	})
}

func shellUnsafe(ctx context.Context, deploymentDir string, selectedNode string) error {
	nodeDetails, err := config.ReadNodeDetails(deploymentDir)
	if err != nil {
		return err
	}

	if selectedNode == "" {
		nodes := nodeDetails.ListNodes()

		if len(nodes) == 0 {
			return ErrNoNodesFound
		}

		selectedNode = nodes[0]
	}

	sshDetails, err := nodeDetails.GetSSHDetails(selectedNode)
	if err != nil {
		return err
	}

	keyFilePath := sshDetails.KeyFile
	if !filepath.IsAbs(keyFilePath) {
		keyFilePath = filepath.Join(deploymentDir, keyFilePath)
	}

	keyData, err := os.ReadFile(keyFilePath)
	if err != nil {
		return fmt.Errorf("%w: could not read SSH key file %s", err, keyFilePath)
	}

	sshRemote := remote.NewSshRemote(&remote.SSHConnectionOptions{
		Host: sshDetails.Host,
		User: sshDetails.User,
		Port: sshDetails.Port,
		Key:  keyData,
	})

	return sshRemote.Shell(ctx, os.Stdout, os.Stderr)
}
