// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
)

func TestRenderDeploymentInfoJSONSerializesReportDirectly(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentID:    "test-deployment",
		DeploymentState: deploy.StatusRunning,
		Presets: &deploy.DeploymentPresetSummary{
			Infrastructure: deploy.PresetIdentityInfo{
				Selector:    "name:aws",
				Kind:        "name",
				Name:        "aws",
				DisplayName: "AWS",
			},
			Installation: deploy.PresetIdentityInfo{
				Selector:    "name:ubuntu",
				Kind:        "name",
				Name:        "ubuntu",
				DisplayName: "Ubuntu",
			},
		},
		Deployment: &config.DeploymentInfo{
			ClusterSize:  3,
			ClusterState: deploy.StatusRunning,
		},
		Connection: &deploy.ConnectionDetails{
			Hostname:        "db.example.local",
			DisplayHost:     "db.example.local",
			DBPort:          8563,
			Username:        "sys",
			CertFingerprint: "fingerprint",
			AdminUI: &config.DeploymentAdminUI{
				URL:      "https://admin.example.local:8443",
				Username: "admin",
			},
			ShellSupported: true,
			SSHCommand:     "ssh exasol@example.local",
		},
	}
	var output bytes.Buffer

	// When
	err := renderDeploymentInfoJSON(&output, report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info JSON to render: %v", err)
	}
	var details deploy.DeploymentInfoReport
	if err := json.Unmarshal(output.Bytes(), &details); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output.String(), err)
	}
	if details.DeploymentID != "test-deployment" {
		t.Fatalf("expected deployment id to be preserved, got %q", details.DeploymentID)
	}
	if details.Presets.Infrastructure.Name != "aws" {
		t.Fatalf("expected infrastructure preset in JSON, got %#v", details.Presets)
	}
	if details.Deployment.ClusterSize != 3 {
		t.Fatalf("expected cluster size in JSON, got %#v", details.Deployment)
	}
	if details.Connection.DBPort != 8563 {
		t.Fatalf("expected connection details in JSON, got %#v", details.Connection)
	}
}

func TestRenderDeploymentInfoJSONOmitsTerminalOnlyGuidance(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentState: deploy.StatusNotInitialized,
	}
	var output bytes.Buffer

	// When
	err := renderDeploymentInfoJSON(&output, report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info JSON to render: %v", err)
	}
	var details map[string]any
	if err := json.Unmarshal(output.Bytes(), &details); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output.String(), err)
	}
	if details["deploymentState"] != deploy.StatusNotInitialized {
		t.Fatalf(
			"expected state %q, got %#v",
			deploy.StatusNotInitialized,
			details["deploymentState"],
		)
	}
	for _, terminalOnlyField := range []string{"message", "error", "actions", "documentation"} {
		if _, exists := details[terminalOnlyField]; exists {
			t.Fatalf("expected JSON to omit %q, got %s", terminalOnlyField, output.String())
		}
	}
}

func TestRenderDeploymentInfoTextOmitsStateGuidance(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentState: deploy.StatusNotInitialized,
	}
	var output bytes.Buffer

	// When
	err := renderDeploymentInfoText(&output, report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info text to render: %v", err)
	}
	content := output.String()
	if !strings.Contains(content, "Deployment State: not_initialized") {
		t.Fatalf("expected text output to contain deployment state, got:\n%s", content)
	}
	for _, guidance := range []string{
		"Exasol Product Documentation",
		"No Exasol Personal deployment exists",
		"exasol install <infra preset>",
		"exasol presets list",
	} {
		if strings.Contains(content, guidance) {
			t.Fatalf("expected text output to omit guidance %q, got:\n%s", guidance, content)
		}
	}
}

func TestRenderDeploymentInfoTextIncludesInitializedOverview(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentID:    "test-deployment",
		DeploymentState: deploy.StatusInitialized,
		Presets: &deploy.DeploymentPresetSummary{
			Infrastructure: deploy.PresetIdentityInfo{DisplayName: "AWS"},
			Installation:   deploy.PresetIdentityInfo{DisplayName: "Ubuntu"},
		},
	}
	var output bytes.Buffer

	// When
	err := renderDeploymentInfoText(&output, report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info text to render: %v", err)
	}
	content := output.String()
	for _, expected := range []string{
		"Deployment directory: /deployment",
		"Deployment ID: test-deployment",
		"Deployment State: initialized",
		"Infrastructure preset: AWS",
		"Installation preset: Ubuntu",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected text output to contain %q, got:\n%s", expected, content)
		}
	}
}

func TestRenderDeploymentInfoTextExplainsActiveOperation(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentState: deploy.StatusOperationInProgress,
	}
	var output bytes.Buffer

	// When
	err := renderDeploymentInfoText(&output, report)
	// Then
	if err != nil {
		t.Fatalf("expected deployment info text to render: %v", err)
	}
	content := output.String()
	for _, expected := range []string{
		"Deployment State: operation_in_progress",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected text output to contain %q, got:\n%s", expected, content)
		}
	}
	if strings.Contains(content, "SQL clients documentation") {
		t.Fatalf("expected active operation output to omit SQL docs, got:\n%s", content)
	}
	if strings.Contains(content, "Please wait") {
		t.Fatalf("expected active operation guidance to stay out of stdout, got:\n%s", content)
	}
}

func TestFormatDeploymentInfoNoticeIncludesStateGuidance(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentState: deploy.StatusNotInitialized,
	}

	// When
	content, err := formatDeploymentInfoNotice(report)
	if err != nil {
		t.Fatalf("expected terminal notice to render: %v", err)
	}

	// Then
	for _, expected := range []string{
		"Exasol Product Documentation",
		"No Exasol Personal deployment exists",
		"exasol install <infra preset>",
		"exasol presets list",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected terminal notice to contain %q, got:\n%s", expected, content)
		}
	}
}

func TestFormatDeploymentInfoNoticeExplainsActiveOperation(t *testing.T) {
	t.Parallel()

	// Given
	report := &deploy.DeploymentInfoReport{
		DeploymentDir:   "/deployment",
		DeploymentState: deploy.StatusOperationInProgress,
	}

	// When
	content, err := formatDeploymentInfoNotice(report)
	if err != nil {
		t.Fatalf("expected terminal notice to render: %v", err)
	}

	// Then
	for _, expected := range []string{
		"operation is in progress",
		"Please wait",
		"exasol status",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected terminal notice to contain %q, got:\n%s", expected, content)
		}
	}
}
