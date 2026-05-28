// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestGetConnectionInstructionsTextStoppedDoesNotRequireConnectionHost(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeStoppedWorkflowState(t, deployment)
	writeDeploymentInfoWithoutConnectionHost(t, deployment)

	// When
	content, err := getConnectionInstructionsTextUnsafe(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected stopped deployment info to be rendered, got error: %v", err)
	}
	assertContains(t, content, "Deployment State: stopped")
	assertContains(t, content, "Cluster Size: 1")
	assertContains(t, content, "Cluster State: stopped")
}

func writeStoppedWorkflowState(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	state := &config.ExasolPersonalState{}
	err := state.SetWorkflowStateAndWrite(&config.WorkflowStateStopped{}, deployment)
	if err != nil {
		t.Fatalf("failed to write stopped workflow state: %v", err)
	}
}

func writeDeploymentInfoWithoutConnectionHost(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId: "test-deployment",
		ClusterSize:  1,
		ClusterState: "stopped",
		Connection: &config.DeploymentConnection{
			DBPort: 8563,
			UIPort: 8443,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
}

func assertContains(t *testing.T, content string, expected string) {
	t.Helper()

	if !strings.Contains(content, expected) {
		t.Fatalf("expected content to contain %q, got:\n%s", expected, content)
	}
}
