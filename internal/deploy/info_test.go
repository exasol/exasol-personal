// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestGetDeploymentInfoReportInitializedIncludesOverviewAndPresets(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)

	// When
	report, err := GetDeploymentInfoReport(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected initialized deployment info report, got error: %v", err)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf("expected deployment dir %q, got %q", deployment.Root(), report.DeploymentDir)
	}
	if report.DeploymentID == "" {
		t.Fatal("expected deployment ID")
	}
	if report.DeploymentState != StatusInitialized {
		t.Fatalf("expected state %q, got %q", StatusInitialized, report.DeploymentState)
	}
	if report.Presets == nil {
		t.Fatal("expected preset summary")
	}
}

func TestDeploymentInfoReportOperationInProgressOmitsPartialDeploymentDetails(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId:    "test-deployment",
		ClusterSize:     1,
		ClusterState:    StatusRunning,
		DeploymentState: StatusRunning,
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	report, err := deploymentInfoReportFromState(
		deployment,
		StatusOperationInProgress,
	)
	// Then
	if err != nil {
		t.Fatalf("expected operation in progress report, got error: %v", err)
	}
	if report.DeploymentState != StatusOperationInProgress {
		t.Fatalf("expected state %q, got %q", StatusOperationInProgress, report.DeploymentState)
	}
	if report.Presets == nil {
		t.Fatal("expected stable preset summary")
	}
	if report.Deployment != nil {
		t.Fatalf("expected partial deployment attributes to be omitted, got %#v", report.Deployment)
	}
	if report.Connection != nil {
		t.Fatalf("expected connection details to be omitted, got %#v", report.Connection)
	}
}

func TestDeploymentInfoReportOperationInProgressToleratesMissingIdentity(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir() + "/missing")

	// When
	report, err := deploymentInfoReportFromState(
		deployment,
		StatusOperationInProgress,
	)
	// Then
	if err != nil {
		t.Fatalf("expected operation in progress report without identity files, got error: %v", err)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf("expected deployment dir %q, got %q", deployment.Root(), report.DeploymentDir)
	}
	if report.DeploymentState != StatusOperationInProgress {
		t.Fatalf("expected state %q, got %q", StatusOperationInProgress, report.DeploymentState)
	}
	if report.Presets != nil {
		t.Fatalf("expected presets to remain optional, got %#v", report.Presets)
	}
	if report.Deployment != nil {
		t.Fatalf("expected deployment attributes to remain optional, got %#v", report.Deployment)
	}
	if report.Connection != nil {
		t.Fatalf("expected connection details to remain optional, got %#v", report.Connection)
	}
}

func TestGetDeploymentInfoReportNotInitializedIncludesStructuredState(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	report, err := GetDeploymentInfoReport(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected missing deployment info to resolve without error, got: %v", err)
	}
	if report.DeploymentState != StatusNotInitialized {
		t.Fatalf("expected state %q, got %q", StatusNotInitialized, report.DeploymentState)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf(
			"expected deployment dir %q, got %q",
			deployment.Root(),
			report.DeploymentDir,
		)
	}
	if report.Presets != nil {
		t.Fatalf("expected no presets for not-initialized deployment, got %#v", report.Presets)
	}
	if report.Deployment != nil {
		t.Fatalf("expected no deployment attributes, got %#v", report.Deployment)
	}
	if report.Connection != nil {
		t.Fatalf("expected no connection details, got %#v", report.Connection)
	}
}

func TestGetDeploymentInfoReportNotInitializedHandlesMissingDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir() + "/missing")

	// When
	report, err := GetDeploymentInfoReport(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected missing deployment directory to resolve without error, got: %v", err)
	}
	if report.DeploymentState != StatusNotInitialized {
		t.Fatalf("expected state %q, got %q", StatusNotInitialized, report.DeploymentState)
	}
	if report.DeploymentDir != deployment.Root() {
		t.Fatalf(
			"expected deployment dir %q, got %q",
			deployment.Root(),
			report.DeploymentDir,
		)
	}
}
