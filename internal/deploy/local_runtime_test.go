// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

const (
	localTestDeploymentID     = "exasol-local-test"
	localTestClusterIdentity  = "exasol-personal;exasol-local-test;local;local"
	localTestDatabasePort     = 28563
	localTestSSHForwardedPort = 20022
)

func TestLocalRunnerVersionCheckArgs_PassesLauncherVersionCheckSettings(t *testing.T) {
	// Given
	deployment := newTestDeploymentWithVersionCheckState(t, true, localTestClusterIdentity)
	const expectedURL = "https://example.test/v1/version-check"
	t.Setenv(VersionCheckURLEnvVar, expectedURL)

	// When
	args, err := localRunnerVersionCheckArgs(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected version-check args, got %v", err)
	}
	expected := []string{
		"--version-check-enabled=true",
		"--version-check-url", expectedURL,
		"--version-check-identity", localTestClusterIdentity,
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected args %#v, got %#v", expected, args)
	}
}

func TestLocalRunnerVersionCheckArgs_DisablesRunnerWhenLauncherVersionCheckDisabled(
	t *testing.T,
) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithVersionCheckState(t, false, "")

	// When
	args, err := localRunnerVersionCheckArgs(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected disabled version-check args, got %v", err)
	}
	expected := []string{"--version-check-enabled=false"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected args %#v, got %#v", expected, args)
	}
}

func TestWriteLocalDeploymentArtifacts_WritesEndpointConnectionAndSecrets(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	state := &localruntime.State{
		VMIP:                   "192.168.64.2",
		SSHPort:                localTestSSHForwardedPort,
		DBPort:                 localTestDatabasePort,
		UIPort:                 28443,
		PrivateKeyRelativePath: "local/node_access.pem",
	}

	// When
	err := writeLocalDeploymentArtifacts(deployment, state)
	// Then
	if err != nil {
		t.Fatalf("expected artifacts to be written, got %v", err)
	}
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		t.Fatalf("expected deployment info to be readable, got %v", err)
	}
	if info.Backend != localDeploymentBackend {
		t.Fatalf("expected backend %q, got %q", localDeploymentBackend, info.Backend)
	}
	if len(info.Nodes) != 0 {
		t.Fatalf("expected local deployment artifacts to omit nodes, got %#v", info.Nodes)
	}
	if info.Connection == nil {
		t.Fatal("expected connection details, got nil")
	}
	if info.Connection.Host != localDeploymentPublicHost {
		t.Fatalf("expected host %q, got %q", localDeploymentPublicHost, info.Connection.Host)
	}
	if info.Connection.DBPort != localTestDatabasePort {
		t.Fatalf("unexpected connection ports: %#v", info.Connection)
	}
	if info.Connection.UIPort != 0 {
		t.Fatalf("expected no local UI port metadata, got %d", info.Connection.UIPort)
	}
	if info.Connection.AdminUI != nil {
		t.Fatalf("expected no local Admin UI metadata, got %#v", info.Connection.AdminUI)
	}
	if !info.Connection.InsecureSkipCertValidation {
		t.Fatal("expected insecure cert validation flag for local deployment")
	}
	if info.Connection.SSHPort != "20022" {
		t.Fatalf("expected SSH port %q, got %q", "20022", info.Connection.SSHPort)
	}
	expectedSSHCommand := "ssh -i local/node_access.pem root@127.0.0.1 -p 20022"
	if info.Connection.SSHCommand != expectedSSHCommand {
		t.Fatalf("expected SSH command %q, got %q", expectedSSHCommand, info.Connection.SSHCommand)
	}

	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		t.Fatalf("expected secrets to be readable, got %v", err)
	}
	if secrets.DbPassword != localDBPassword {
		t.Fatalf("expected local DB password %q, got %q", localDBPassword, secrets.DbPassword)
	}
	if secrets.AdminUiPassword != "" {
		t.Fatalf("expected no local Admin UI password, got %q", secrets.AdminUiPassword)
	}
}

func TestWriteLocalDeploymentArtifacts_OmitsLocalOnlyCloudMetadataInJSON(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	state := &localruntime.State{
		SSHPort:                localTestSSHForwardedPort,
		DBPort:                 localTestDatabasePort,
		UIPort:                 28443,
		PrivateKeyRelativePath: "local/node_access.pem",
	}

	// When
	err := writeLocalDeploymentArtifacts(deployment, state)
	// Then
	if err != nil {
		t.Fatalf("expected artifacts to be written, got %v", err)
	}
	data, err := os.ReadFile(deployment.NodeDetailsPath())
	if err != nil {
		t.Fatalf("expected deployment info file to be readable, got %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("expected deployment info JSON to parse, got %v", err)
	}
	if _, exists := raw["nodes"]; exists {
		t.Fatalf("expected local deployment JSON to omit nodes, got %s", string(data))
	}
	connection, ok := raw["connection"].(map[string]any)
	if !ok {
		t.Fatalf("expected connection object in deployment info JSON, got %s", string(data))
	}
	if _, exists := connection["adminUi"]; exists {
		t.Fatalf("expected local deployment JSON to omit adminUi, got %s", string(data))
	}
	if _, exists := connection["uiPort"]; exists {
		t.Fatalf("expected local deployment JSON to omit uiPort, got %s", string(data))
	}
}

func TestDestroyLocalRuntime_RemovesLocalRuntimeAndArtifacts(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	paths := localruntime.NewPaths(deployment)
	if err := os.MkdirAll(paths.Root, 0o750); err != nil {
		t.Fatalf("failed to create local runtime root: %v", err)
	}
	for _, path := range []string{
		filepath.Join(paths.Root, "disk.img"),
		deployment.NodeDetailsPath(),
		deployment.SecretsPath(),
		deployment.ConnectionInstructionsPath(),
	} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("failed to write test file %s: %v", path, err)
		}
	}

	// When: paths.VMDir was never created, so destroyLocalRuntime never needs
	// to resolve a runner, and a nil manager is safe here.
	err := destroyLocalRuntime(context.Background(), localruntime.New(deployment, nil), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected destroy cleanup to succeed, got %v", err)
	}
	for _, path := range []string{
		paths.Root,
		deployment.NodeDetailsPath(),
		deployment.SecretsPath(),
		deployment.ConnectionInstructionsPath(),
	} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("expected %s to be removed, got stat error %v", path, statErr)
		}
	}
}

func TestStopLocalRuntime_UpdatesDeploymentInfoState(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// Given
	deployment := newTestDeploymentWithState(t)
	paths := localruntime.NewPaths(deployment)
	if err := os.MkdirAll(paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime work dir: %v", err)
	}
	manager := newTestManagerForRunner(t, []byte("#!/bin/sh\nexit 0\n"))
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:         localDeploymentBackend,
		DeploymentId:    localTestDeploymentID,
		DeploymentState: StatusRunning,
		ClusterState:    StatusRunning,
		ClusterSize:     1,
		InstanceType:    "exasol-local",
		Connection: &config.DeploymentConnection{
			Host:    localDeploymentPublicHost,
			DBPort:  localTestDatabasePort,
			SSHPort: "20022",
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	err := stopLocalRuntime(context.Background(), localruntime.New(deployment, manager), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected local stop to succeed, got %v", err)
	}
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		t.Fatalf("expected deployment info to be readable, got %v", err)
	}
	if info.DeploymentState != StatusStopped {
		t.Fatalf("expected deployment state %q, got %q", StatusStopped, info.DeploymentState)
	}
	if info.ClusterState != StatusStopped {
		t.Fatalf("expected cluster state %q, got %q", StatusStopped, info.ClusterState)
	}
}

func newTestDeploymentWithState(t *testing.T) config.DeploymentDir {
	t.Helper()

	return newTestDeploymentWithVersionCheckState(t, false, "")
}

func newTestDeploymentWithVersionCheckState(
	t *testing.T,
	versionCheckEnabled bool,
	clusterIdentity string,
) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		DeploymentId:        localTestDeploymentID,
		ClusterIdentity:     clusterIdentity,
		VersionCheckEnabled: versionCheckEnabled,
		DeploymentVersion:   "0.0.0",
	}
	workflowState := &config.WorkflowStateInitialized{}
	if err := state.SetWorkflowStateAndWrite(workflowState, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	return deployment
}
