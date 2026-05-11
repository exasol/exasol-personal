// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
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
