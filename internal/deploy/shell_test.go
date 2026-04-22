// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestOpenHostShell_RejectsLocalDeployments(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	if err := config.WriteLocalDeploymentInfo(deploymentDir, &config.LocalDeploymentInfo{
		Backend:         config.DeploymentBackendLocal,
		DeploymentID:    "local-test",
		DeploymentState: StatusRunning,
		ClusterSize:     1,
		ClusterState:    localClusterStateRunning,
		Local: &config.LocalDeploymentRuntime{
			Host:                       "127.0.0.1",
			DBPort:                     8563,
			UIPort:                     8443,
			InsecureSkipCertValidation: true,
		},
	}); err != nil {
		t.Fatalf("failed to write local deployment info: %v", err)
	}

	// When
	err := OpenHostShell(
		context.Background(),
		config.NewDeploymentDir(deploymentDir),
		"",
	)

	// Then
	if !errors.Is(err, ErrLocalShellUnsupported) {
		t.Fatalf("expected ErrLocalShellUnsupported, got %v", err)
	}
}
