// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestOpenHostShell_MissingManifestReturnsErrorWithoutOpeningShell(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	if err := OpenHostShell(context.Background(), deployment, ""); err == nil {
		t.Fatal("expected an error when no infrastructure manifest has been extracted")
	}
}

func TestOpenCOSShell_MissingManifestReturnsErrorWithoutOpeningShell(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	if err := OpenCOSShell(context.Background(), deployment); err == nil {
		t.Fatal("expected an error when no infrastructure manifest has been extracted")
	}
}

func writeTestSSHKeyFile(t *testing.T, deployment config.DeploymentDir, relPath string) {
	t.Helper()

	absPath := deployment.Resolve(relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		t.Fatalf("failed to create key directory: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("fake-private-key"), 0o600); err != nil {
		t.Fatalf("failed to write fake key file: %v", err)
	}
}

func TestSshRemoteForNodeUnsafeReturnsErrNoNodesFoundWhenDeploymentHasNoNodes(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	_, err := sshRemoteForNodeUnsafe(deployment, "")

	if !errors.Is(err, ErrNoNodesFound) {
		t.Fatalf("expected ErrNoNodesFound, got %v", err)
	}
}

func TestSshRemoteForNodeUnsafeReturnsErrorForUnknownExplicitNode(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Nodes: map[string]config.DeploymentNode{
			"n11": {PublicIp: "203.0.113.10"},
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	_, err := sshRemoteForNodeUnsafe(deployment, "does-not-exist")

	if !errors.Is(err, config.ErrUnknownNodeName) {
		t.Fatalf("expected ErrUnknownNodeName, got %v", err)
	}
}

func TestSshRemoteForNodeUnsafeWrapsMissingKeyFileError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Nodes: map[string]config.DeploymentNode{
			"n11": {
				PublicIp: "203.0.113.10",
				Ssh: config.DeploymentSSH{
					Username: "sys",
					Port:     "22",
					KeyFile:  "keys/missing.pem",
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	_, err := sshRemoteForNodeUnsafe(deployment, "")

	if err == nil || !strings.Contains(err.Error(), "could not read SSH key file") {
		t.Fatalf("expected a missing SSH key file error, got %v", err)
	}
}

func TestSshRemoteForNodeUnsafeResolvesSingleNodeByDefault(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeTestSSHKeyFile(t, deployment, "keys/n11.pem")
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Nodes: map[string]config.DeploymentNode{
			"n11": {
				PublicIp: "203.0.113.10",
				Ssh: config.DeploymentSSH{
					Username: "sys",
					Port:     "22",
					KeyFile:  "keys/n11.pem",
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	remoteHost, err := sshRemoteForNodeUnsafe(deployment, "")
	if err != nil {
		t.Fatalf("expected the sole node to resolve without an explicit selection, got %v", err)
	}
	if remoteHost == nil {
		t.Fatal("expected a non-nil SSH remote")
	}
}
