// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploymentcompatibility

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func newUninitializedDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()

	return config.NewDeploymentDir(t.TempDir())
}

func newInitializedDeployment(t *testing.T, deploymentVersion string) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}
	if deploymentVersion != "" {
		if err := config.WriteDeploymentVersionMarker(deployment, deploymentVersion); err != nil {
			t.Fatalf("failed to write version marker: %v", err)
		}
	}

	return deployment
}

func TestEnforce_UninitializedDeploymentIsAllowedWhenNotRequired(t *testing.T) {
	t.Parallel()

	deployment := newUninitializedDeployment(t)

	err := EnforceDeploymentDirectoryCompatibility(
		deployment, "1.0.0", Requirement{CommandName: "init"}, DeploymentDirMayBeUninitialized,
	)
	if err != nil {
		t.Fatalf("expected no error for an uninitialized dir when not required, got %v", err)
	}
}

func TestEnforce_UninitializedDeploymentIsRejectedWhenRequired(t *testing.T) {
	t.Parallel()

	deployment := newUninitializedDeployment(t)

	err := EnforceDeploymentDirectoryCompatibility(
		deployment, "1.0.0", Requirement{CommandName: "status"}, DeploymentDirMustBeInitialized,
	)

	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("expected a 'not initialized' error, got %v", err)
	}
}

func TestEnforce_LegacyDeploymentLayoutIsAlwaysRejected(t *testing.T) {
	t.Parallel()

	deployment := newUninitializedDeployment(t)
	legacyPath := deployment.Resolve(legacyWorkflowStateFileName)
	if err := os.WriteFile(legacyPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write legacy workflow state file: %v", err)
	}

	err := EnforceDeploymentDirectoryCompatibility(
		deployment, "1.0.0", Requirement{CommandName: "status"}, DeploymentDirMayBeUninitialized,
	)

	if err == nil || !strings.Contains(err.Error(), "older version") {
		t.Fatalf("expected a legacy-layout error, got %v", err)
	}
}

func TestEnforce_InitializedDeploymentMissingVersionMarkerIsRejected(t *testing.T) {
	t.Parallel()

	deployment := newInitializedDeployment(t, "")

	err := EnforceDeploymentDirectoryCompatibility(
		deployment, "1.0.0", Requirement{CommandName: "status"}, DeploymentDirMustBeInitialized,
	)

	if err == nil || !strings.Contains(err.Error(), "missing deployment") {
		t.Fatalf("expected a missing-version-marker error, got %v", err)
	}
}

func TestEnforce_InitializedDeploymentWithEmptyVersionMarkerIsRejected(t *testing.T) {
	t.Parallel()

	deployment := newInitializedDeployment(t, "")
	markerPath := deployment.DeploymentVersionMarkerPath()
	if err := os.WriteFile(markerPath, []byte("  "), 0o600); err != nil {
		t.Fatalf("failed to write empty version marker: %v", err)
	}

	err := EnforceDeploymentDirectoryCompatibility(
		deployment, "1.0.0", Requirement{CommandName: "status"}, DeploymentDirMustBeInitialized,
	)

	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("expected an empty-version-marker error, got %v", err)
	}
}

func TestEnforce_CompatibleVersionsAreAllowed(t *testing.T) {
	t.Parallel()

	deployment := newInitializedDeployment(t, "1.2.0")

	err := EnforceDeploymentDirectoryCompatibility(
		deployment,
		"2.0.0",
		Requirement{CommandName: "status", MinSupportedDeploymentVersion: "1.0.0"},
		DeploymentDirMustBeInitialized,
	)
	if err != nil {
		t.Fatalf("expected compatible versions to be allowed, got %v", err)
	}
}

func TestEnforce_IncompatibleVersionsReturnIncompatibleError(t *testing.T) {
	t.Parallel()

	deployment := newInitializedDeployment(t, "1.0.0")

	err := EnforceDeploymentDirectoryCompatibility(
		deployment,
		"2.0.0",
		Requirement{CommandName: "deploy", MinSupportedDeploymentVersion: "1.5.0"},
		DeploymentDirMustBeInitialized,
	)

	var inc *IncompatibleError
	if !errors.As(err, &inc) {
		t.Fatalf("expected an IncompatibleError, got %T: %v", err, err)
	}
}

func TestEnforce_InvalidDeploymentVersionMarkerReturnsInvalidVersionError(t *testing.T) {
	t.Parallel()

	deployment := newInitializedDeployment(t, "not-a-version")

	err := EnforceDeploymentDirectoryCompatibility(
		deployment,
		"2.0.0",
		Requirement{CommandName: "deploy", MinSupportedDeploymentVersion: "1.0.0"},
		DeploymentDirMustBeInitialized,
	)

	var inv *InvalidVersionError
	if !errors.As(err, &inv) {
		t.Fatalf("expected an InvalidVersionError, got %T: %v", err, err)
	}
}

func TestIncompatibleErrorDefaultReasonMessage(t *testing.T) {
	t.Parallel()

	err := &IncompatibleError{CommandName: "deploy", RequiredAction: "manual_fix"}

	if !strings.Contains(err.Error(), "manual_fix") {
		t.Fatalf("expected the default-case message to mention the required action, got %q",
			err.Error())
	}
}
