// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestAppendDeployFailureHint_AddsLauncherLogPath(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("deployment failed")
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	err := appendDeployFailureHint(deployment, baseErr)

	// Then
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("expected wrapped error to match base error, got: %v", err)
	}
	if !strings.Contains(err.Error(), deployment.Resolve("deployment.log")) {
		t.Fatalf("expected error to include deployment log path, got: %q", err.Error())
	}
}

func TestAppendDeployFailureHint_AddsDeploymentInfoPathWhenPresent(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("deployment failed")
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Connection: &config.DeploymentConnection{
			Host:           "example.local",
			DisplayHost:    "example.local",
			DBPort:         8563,
			UIPort:         8443,
			Username:       "sys",
			ShellSupported: true,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	err := appendDeployFailureHint(deployment, baseErr)

	// Then
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), deployment.NodeDetailsPath()) {
		t.Fatalf("expected error to include deployment info path, got: %q", err.Error())
	}
}

func TestAppendDeployFailureHintNilInput(t *testing.T) {
	t.Parallel()

	if err := appendDeployFailureHint(config.NewDeploymentDir(t.TempDir()), nil); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestDeployFailureResourceHint_LocalBackendMentionsLocalVM(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newLocalTestDeployment(t)

	// Then
	if hint := deployFailureResourceHint(deployment); hint != localDeployFailureResourceHint {
		t.Fatalf("expected the local-backend hint, got %q", hint)
	}
}

func TestDeployFailureResourceHint_MissingManifestFallsBackToCloudHint(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// Then
	if hint := deployFailureResourceHint(deployment); hint != cloudDeployFailureResourceHint {
		t.Fatalf("expected the cloud fallback hint, got %q", hint)
	}
}

func TestWorkflowStatePermitsDeploy_ReturnsErrorWhenWorkflowStateUnreadable(t *testing.T) {
	t.Parallel()

	// Given
	exasolState := &config.ExasolPersonalState{}

	// When
	err := WorkflowStatePermitsDeploy(exasolState, config.NewDeploymentDir(t.TempDir()))
	// Then
	if err == nil {
		t.Fatal("expected an error when no workflow state is set")
	}
}

func TestDeploy_BlockedStateReturnsErrorWithoutInvokingBackend(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateStopped{}, deployment,
	); err != nil {
		t.Fatalf("write workflow state failed: %v", err)
	}

	// When
	err = Deploy(context.Background(), deployment, false, DeployOptions{})
	// Then
	if !errors.Is(err, ErrUnexpectedDeploymentStatus) {
		t.Fatalf("expected ErrUnexpectedDeploymentStatus, got %v", err)
	}
}

func TestDeploy_MissingStateReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	err := Deploy(context.Background(), deployment, false, DeployOptions{})
	// Then
	if err == nil {
		t.Fatal("expected an error when no launcher state has been persisted")
	}
}
