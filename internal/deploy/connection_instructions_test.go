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

func TestGetSQLInstructions_OmitsAdminUIWhenMetadataMissing(t *testing.T) {
	t.Parallel()

	// Given
	connectionDetails := &ConnectionDetails{
		Backend:         localDeploymentBackend,
		DisplayHost:     "127.0.0.1",
		DBPort:          "28563",
		Username:        "sys",
		SecretsFilePath: "/deployment/secrets.json",
		SSHCommand:      "ssh -i key exasol@127.0.0.1 -p 20022",
		ShellSupported:  true,
	}

	// When
	instructions := GetSQLInstructions(connectionDetails)

	// Then
	if !strings.Contains(instructions, "exasol connect") {
		t.Fatalf("expected CLI instructions, got %q", instructions)
	}
	if !strings.Contains(instructions, "Local Shell Instructions") {
		t.Fatalf("expected shell instructions to be preserved, got %q", instructions)
	}
	if strings.Contains(instructions, "Administration UI") {
		t.Fatalf("expected Admin UI instructions to be omitted, got %q", instructions)
	}
}

func TestGetSQLInstructions_IncludesAdminUIWhenMetadataPresent(t *testing.T) {
	t.Parallel()

	// Given
	connectionDetails := &ConnectionDetails{
		DisplayHost:     "db.example.local",
		DBPort:          "8563",
		Username:        "sys",
		SecretsFilePath: "/deployment/secrets.json",
		AdminUI: &config.DeploymentAdminUI{
			URL:                        "https://admin.example.local:8443",
			Username:                   "admin",
			InsecureSkipCertValidation: true,
		},
		AdminUISecured: true,
	}

	// When
	instructions := GetSQLInstructions(connectionDetails)

	// Then
	for _, expected := range []string{
		"Administration UI",
		"https://admin.example.local:8443",
		"Username: admin",
		"Password: <stored in /deployment/secrets.json>",
		"Certificate Validation: accept the certificate if necessary",
		"exasol connect",
	} {
		if !strings.Contains(instructions, expected) {
			t.Fatalf("expected instructions to contain %q, got %q", expected, instructions)
		}
	}
}

func TestGetSQLInstructions_OmitsAILabWhenMetadataMissing(t *testing.T) {
	t.Parallel()

	// Given
	connectionDetails := &ConnectionDetails{
		DisplayHost:     "db.example.local",
		DBPort:          "8563",
		Username:        "sys",
		SecretsFilePath: "/deployment/secrets.json",
	}

	// When
	instructions := GetSQLInstructions(connectionDetails)

	// Then
	if strings.Contains(instructions, "AI Lab") {
		t.Fatalf("expected AI Lab instructions to be omitted, got %q", instructions)
	}
	if !strings.Contains(instructions, "exasol connect") {
		t.Fatalf("expected SQL instructions to be preserved, got %q", instructions)
	}
}

func TestGetSQLInstructions_IncludesAILabWhenMetadataPresent(t *testing.T) {
	t.Parallel()

	// Given
	connectionDetails := &ConnectionDetails{
		DisplayHost:     "db.example.local",
		DBPort:          "8563",
		Username:        "sys",
		SecretsFilePath: "/deployment/secrets.json",
		AILab: &config.DeploymentAILab{
			URL: "http://ai.example.local:49494",
		},
		AILabSecured: true,
	}

	// When
	instructions := GetSQLInstructions(connectionDetails)

	// Then
	for _, expected := range []string{
		"How to open the AI Lab",
		"http://ai.example.local:49494",
		"Jupyter password: <stored in /deployment/secrets.json>",
		"Config-store master password: <stored in /deployment/secrets.json>",
		"pre-configured",
	} {
		if !strings.Contains(instructions, expected) {
			t.Fatalf("expected instructions to contain %q, got %q", expected, instructions)
		}
	}
}
