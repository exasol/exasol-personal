// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestGetDiagnosticDeploymentInfo_ReadsPersistedInfo(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId: "test-deployment",
		ClusterSize:  1,
	}); err != nil {
		t.Fatalf("failed to seed deployment info: %v", err)
	}

	info, err := GetDiagnosticDeploymentInfo(context.Background(), deployment)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.DeploymentId != "test-deployment" {
		t.Fatalf("expected the persisted deployment info, got %+v", info)
	}
}

func TestGetDiagnosticDeploymentInfo_MissingInfoReturnsError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	if _, err := GetDiagnosticDeploymentInfo(context.Background(), deployment); err == nil {
		t.Fatal("expected an error when no deployment info has been persisted")
	}
}
