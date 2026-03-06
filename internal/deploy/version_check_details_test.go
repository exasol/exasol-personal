// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestGetVersionCheckURL_DefaultWhenEnvMissing(t *testing.T) {
	t.Setenv(VersionCheckURLEnvVar, "")

	url := GetVersionCheckURL()
	if url != DefaultVersionCheckURL {
		t.Fatalf("expected default URL %q, got %q", DefaultVersionCheckURL, url)
	}
}

func TestGetVersionCheckURL_UsesEnvOverride(t *testing.T) {
	const expected = "https://example.com/custom-check"
	t.Setenv(VersionCheckURLEnvVar, expected)

	url := GetVersionCheckURL()
	if url != expected {
		t.Fatalf("expected env override URL %q, got %q", expected, url)
	}
}

func TestGetVersionCheckDetails_EmptyClusterIdentityWhenNoDeploymentDir(t *testing.T) {
	t.Parallel()

	details := GetVersionCheckDetails("")
	if details.ClusterIdentity != "" {
		t.Fatalf("expected empty cluster identity, got %q", details.ClusterIdentity)
	}
}

func TestGetVersionCheckDetails_UsesPersistedClusterIdentityFromState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	expected := "exasol-personal;exasol-1234abcd;infra;install"

	state := &config.ExasolPersonalState{
		CurrentWorkflowState: config.WorkflowState{Initialized: &config.WorkflowStateInitialized{}},
		DeploymentId:         "exasol-1234abcd",
		ClusterIdentity:      expected,
		DeploymentVersion:    "0.0.0",
		LastVersionCheck:     time.Time{},
		VersionCheckEnabled:  true,
	}
	if err := config.WriteExasolPersonalState(state, dir); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	details := GetVersionCheckDetails(dir)
	if details.ClusterIdentity != expected {
		t.Fatalf(
			"expected persisted cluster identity %q, got %q",
			expected,
			details.ClusterIdentity,
		)
	}
}

func TestGetVersionCheckDetails_EmptyClusterIdentityWhenStateHasNoClusterIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	deploymentId := "exasol-1234abcd"

	state := &config.ExasolPersonalState{
		CurrentWorkflowState: config.WorkflowState{Initialized: &config.WorkflowStateInitialized{}},
		DeploymentId:         deploymentId,
		ClusterIdentity:      "",
		DeploymentVersion:    "0.0.0",
		LastVersionCheck:     time.Time{},
		VersionCheckEnabled:  true,
	}
	if err := config.WriteExasolPersonalState(state, dir); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	details := GetVersionCheckDetails(dir)
	if details.ClusterIdentity != "" {
		t.Fatalf(
			"expected empty cluster identity when state value is missing, got %q",
			details.ClusterIdentity,
		)
	}
}
