// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestGetConnectionInstructionsTextStoppedDoesNotRequireConnectionHost(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)
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

func TestGetConnectionInstructionsTextNotInitializedRendersTechnicalState(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	content, err := GetConnectionInstructionsText(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected missing deployment info to render without error, got: %v", err)
	}
	assertContains(t, content, "Deployment directory: "+deployment.Root())
	assertContains(t, content, "Deployment State: not_initialized")
}

func TestGetConnectionInstructionsTextNotInitializedHandlesMissingDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir() + "/missing")

	// When
	content, err := GetConnectionInstructionsText(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected missing deployment directory to render without error, got: %v", err)
	}
	assertContains(t, content, "Deployment directory: "+deployment.Root())
	assertContains(t, content, "Deployment State: not_initialized")
}

func initDeploymentForInfoTest(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	)
	if err != nil {
		t.Fatalf("failed to initialize deployment: %v", err)
	}
}

func writeStoppedWorkflowState(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read workflow state: %v", err)
	}
	err = state.SetWorkflowStateAndWrite(&config.WorkflowStateStopped{}, deployment)
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

func TestRenderConnectionInstructionsText_OmitsAdminUIWhenMetadataMissing(t *testing.T) {
	t.Parallel()

	// Given
	report := &DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentID:    "test-deployment",
		DeploymentState: StatusRunning,
		Deployment: &config.DeploymentInfo{
			ClusterSize:  1,
			ClusterState: StatusRunning,
		},
		Connection: &ConnectionDetails{
			Backend:         localDeploymentBackend,
			DisplayHost:     "127.0.0.1",
			DBPort:          28563,
			Username:        "sys",
			SecretsFilePath: "/deployment/secrets.json",
			SSHCommand:      "ssh -i key exasol@127.0.0.1 -p 20022",
			ShellSupported:  true,
		},
	}

	// When
	content, err := RenderConnectionInstructionsText(report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info to render: %v", err)
	}
	if !strings.Contains(content, "exasol connect") {
		t.Fatalf("expected CLI instructions, got %q", content)
	}
	if !strings.Contains(
		content,
		"  - UserId: sys\n  - Password: <stored in /deployment/secrets.json>",
	) {
		t.Fatalf("expected password line to remain separate, got %q", content)
	}
	if !strings.Contains(content, "Host Shell Instructions") {
		t.Fatalf("expected shell instructions to be preserved, got %q", content)
	}
	if strings.Contains(content, "container shell") {
		t.Fatalf("expected container shell instructions to be omitted, got %q", content)
	}
	if strings.Contains(content, "Administration UI") {
		t.Fatalf("expected Admin UI instructions to be omitted, got %q", content)
	}
}

func TestRenderConnectionInstructionsText_IncludesAdminUIWhenMetadataPresent(t *testing.T) {
	t.Parallel()

	// Given
	report := &DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentID:    "test-deployment",
		DeploymentState: StatusRunning,
		Deployment: &config.DeploymentInfo{
			ClusterSize:  1,
			ClusterState: StatusRunning,
		},
		Connection: &ConnectionDetails{
			DisplayHost:     "db.example.local",
			DBPort:          8563,
			Username:        "sys",
			SecretsFilePath: "/deployment/secrets.json",
			AdminUI: &config.DeploymentAdminUI{
				URL:                        "https://admin.example.local:8443",
				Username:                   "admin",
				InsecureSkipCertValidation: true,
			},
			AdminUISecured: true,
		},
	}

	// When
	content, err := RenderConnectionInstructionsText(report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info to render: %v", err)
	}
	for _, expected := range []string{
		"Administration UI",
		"https://admin.example.local:8443",
		"URL: https://admin.example.local:8443\n  Username: admin",
		"Username: admin",
		"Password: <stored in /deployment/secrets.json>",
		"Certificate Validation: accept the certificate if necessary",
		"exasol connect",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected instructions to contain %q, got %q", expected, content)
		}
	}
}
