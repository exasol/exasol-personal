// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/version_check"
)

const (
	localTestDeploymentID     = "exasol-local-test"
	localTestClusterIdentity  = "exasol-personal;exasol-local-test;local;local"
	localTestDatabasePort     = 28563
	localTestSSHForwardedPort = 20022
)

func TestToLocalRuntimeConfig_TranslatesPortableStartupState(t *testing.T) {
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		ClusterIdentity:     localTestClusterIdentity,
		VersionCheckEnabled: true,
		InstalledSLCs: []config.InstalledSLC{
			{
				Language: "java",
				Image:    "docker.io/exasol/script-language-container:python",
				Target:   "/exa/slc/python",
				Aliases:  []string{"PYTHON3"},
			},
			{
				Language: "python",
				Image:    "docker.io/exasol/script-language-container:java",
				Target:   "/exa/slc/java-17",
				Aliases:  []string{"jAvA", "JAVA17"},
			},
		},
		InstalledCustomSLCs: []config.InstalledCustomSLC{{
			Image:   "localhost/custom:sha256",
			Target:  "/exa/slc/custom",
			Package: "custom.tar.gz",
		}},
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}
	const expectedURL = "https://version-check.example.test"
	t.Setenv(version_check.VersionCheckURLEnvVar, expectedURL)

	// When
	actual, err := toLocalRuntimeConfig(deployment, localRuntimeConfig{
		ports:      "db:28563",
		cpuCount:   4,
		memoryMB:   8192,
		dataSizeGB: 50,
	})
	// Then
	if err != nil {
		t.Fatalf("expected startup state translation to succeed: %v", err)
	}
	if actual.Ports != "db:28563" || actual.CPUCount != 4 ||
		actual.MemoryMB != 8192 || actual.DataSizeGB != 50 {
		t.Fatalf("unexpected runtime settings: %#v", actual)
	}
	if !actual.VersionCheck.Enabled || actual.VersionCheck.URL != expectedURL ||
		actual.VersionCheck.Identity != localTestClusterIdentity ||
		actual.VersionCheck.OperatingSystem == "" {
		t.Fatalf("unexpected version-check settings: %#v", actual.VersionCheck)
	}
	if len(actual.SLCs) != 4 {
		t.Fatalf(
			"expected official, Java compatibility, and custom SLC mounts, got %#v",
			actual.SLCs,
		)
	}
	if actual.SLCs[0].Target != "/exa/slc/python" ||
		actual.SLCs[1].Target != "/exa/slc/java-17" ||
		actual.SLCs[2].Image != actual.SLCs[1].Image ||
		actual.SLCs[2].Target != currentJavaMountTarget ||
		actual.SLCs[3].Package != "custom.tar.gz" {
		t.Fatalf("unexpected SLC package translation: %#v", actual.SLCs)
	}
}

func TestToLocalRuntimeConfig_PreservesAuthoritativeEmptySLCSet(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteExasolPersonalState(
		&config.ExasolPersonalState{}, deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}

	// When
	actual, err := toLocalRuntimeConfig(deployment, localRuntimeConfig{})
	// Then
	if err != nil {
		t.Fatalf("expected startup state translation to succeed: %v", err)
	}
	if actual.SLCs == nil || len(actual.SLCs) != 0 {
		t.Fatalf("expected authoritative non-nil empty SLC set, got %#v", actual.SLCs)
	}
}

func TestWriteLocalDeploymentArtifacts_WritesEndpointConnectionAndSecrets(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	endpoint := &localruntime.VMRuntimeEndpoint{
		RuntimeEndpoint: localruntime.RuntimeEndpoint{
			DBPort:         localTestDatabasePort,
			UIPort:         28443,
			ShellSupported: true,
		},
	}

	// When
	err := writeLocalDeploymentArtifacts(deployment, endpoint)
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
	if !info.Connection.ShellSupported {
		t.Fatal("expected runtime-provided shell support")
	}
	if info.Connection.SSHPort != "" || info.Connection.SSHCommand != "" {
		t.Fatalf("expected no SSH transport metadata, got %#v", info.Connection)
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
	endpoint := &localruntime.VMRuntimeEndpoint{
		RuntimeEndpoint: localruntime.RuntimeEndpoint{
			DBPort:         localTestDatabasePort,
			UIPort:         28443,
			ShellSupported: true,
		},
	}

	// When
	err := writeLocalDeploymentArtifacts(deployment, endpoint)
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
	if _, exists := connection["sshPort"]; exists {
		t.Fatalf("expected local deployment JSON to omit sshPort, got %s", string(data))
	}
	if _, exists := connection["sshCommand"]; exists {
		t.Fatalf("expected local deployment JSON to omit sshCommand, got %s", string(data))
	}
}

func TestStartPreparedLocalRuntime_WritesHostEndpointArtifacts(t *testing.T) {
	// Given
	t.Setenv(localSkipDatabaseWaitEnv, "true")
	deployment := newTestDeploymentWithState(t)
	runtime := &endpointRuntimeStub{
		deployment: deployment,
		endpoint:   &localruntime.RuntimeEndpoint{DBPort: localTestDatabasePort},
	}

	// When
	err := startPreparedLocalRuntime(
		context.Background(), runtime, localRuntimeConfig{}, 0, nil, nil,
	)
	// Then
	if err != nil {
		t.Fatalf("expected host runtime start to succeed, got %v", err)
	}
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		t.Fatalf("expected deployment info to be readable, got %v", err)
	}
	if info.Connection == nil || info.Connection.DBPort != localTestDatabasePort {
		t.Fatalf("expected published database endpoint, got %#v", info.Connection)
	}
	if info.Connection.ShellSupported || info.Connection.SSHCommand != "" ||
		info.Connection.SSHPort != "" {
		t.Fatalf("expected host endpoint to omit VM shell details, got %#v", info.Connection)
	}
	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		t.Fatalf("expected secrets to be readable, got %v", err)
	}
	if secrets.DbPassword != localDBPassword {
		t.Fatalf("expected local database password, got %#v", secrets)
	}
}

func TestWaitForLocalDatabaseAndSyncRunsSyncAfterReadiness(t *testing.T) {
	t.Parallel()

	runtime := &endpointRuntimeStub{deployment: newTestDeploymentWithState(t)}
	var stdout, stderr bytes.Buffer
	waitCalls := 0
	waitForDatabase := func(context.Context, localruntime.Runtime) error {
		waitCalls++
		if runtime.syncCalls != 0 {
			t.Fatal("storage synchronized before database readiness")
		}

		return nil
	}

	err := waitForLocalDatabaseAndSync(
		context.Background(), runtime, 1, &stdout, &stderr, waitForDatabase,
	)
	if err != nil {
		t.Fatalf("expected post-ready sync to succeed, got %v", err)
	}
	if waitCalls != 1 || runtime.syncCalls != 1 {
		t.Fatalf(
			"expected one readiness check and sync, got wait=%d sync=%d",
			waitCalls,
			runtime.syncCalls,
		)
	}
	if runtime.syncOut != &stdout || runtime.syncOutErr != &stderr {
		t.Fatal("expected startup output writers to be forwarded to sync")
	}
}

func TestWaitForLocalDatabaseAndSyncPreservesFailures(t *testing.T) {
	t.Parallel()

	t.Run("readiness", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("database did not become ready")
		runtime := &endpointRuntimeStub{deployment: newTestDeploymentWithState(t)}

		err := waitForLocalDatabaseAndSync(
			context.Background(), runtime, 1, nil, nil,
			func(context.Context, localruntime.Runtime) error { return expectedErr },
		)

		if !errors.Is(err, expectedErr) || runtime.syncCalls != 0 {
			t.Fatalf(
				"expected readiness failure without sync, got err=%v sync=%d",
				err,
				runtime.syncCalls,
			)
		}
	})

	t.Run("sync", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("storage sync failed")
		runtime := &endpointRuntimeStub{
			deployment: newTestDeploymentWithState(t),
			syncErr:    expectedErr,
		}

		err := waitForLocalDatabaseAndSync(
			context.Background(), runtime, 1, nil, nil,
			func(context.Context, localruntime.Runtime) error { return nil },
		)

		if !errors.Is(err, expectedErr) || runtime.syncCalls != 1 {
			t.Fatalf(
				"expected sync failure after readiness, got err=%v sync=%d",
				err,
				runtime.syncCalls,
			)
		}
	})
}

func TestStartPreparedLocalRuntime_PreservesHostStartFailure(t *testing.T) {
	t.Parallel()

	// Given
	expectedErr := errors.New("Podman start failed")
	runtime := &endpointRuntimeStub{
		deployment: newTestDeploymentWithState(t),
		startErr:   expectedErr,
	}

	// When
	err := startPreparedLocalRuntime(
		context.Background(), runtime, localRuntimeConfig{}, 0, nil, nil,
	)
	// Then
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected host start error to be preserved, got %v", err)
	}
}

func TestDestroyLocalRuntime_RemovesLocalRuntimeAndArtifacts(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	paths := newLocalRuntimeTestPaths(deployment)
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
	err := destroyLocalRuntime(
		context.Background(),
		localruntime.NewMacVMRuntime(deployment, nil),
		nil,
		nil,
	)
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
	paths := newLocalRuntimeTestPaths(deployment)
	if err := os.MkdirAll(paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime work dir: %v", err)
	}
	manager := newTestManagerForRunner(t, []byte(`#!/bin/sh
case "$1" in
  status) printf '{"running":false}\n' ;;
  stop) exit 0 ;;
  *) exit 2 ;;
esac
`))
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:         localDeploymentBackend,
		DeploymentId:    localTestDeploymentID,
		DeploymentState: StatusRunning,
		ClusterState:    StatusRunning,
		ClusterSize:     1,
		InstanceType:    "exasol-local",
		Connection: &config.DeploymentConnection{
			Host:   localDeploymentPublicHost,
			DBPort: localTestDatabasePort,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	err := stopLocalRuntime(
		context.Background(),
		localruntime.NewMacVMRuntime(deployment, manager),
		nil,
		nil,
	)
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

type endpointRuntimeStub struct {
	deployment config.DeploymentDir
	endpoint   *localruntime.RuntimeEndpoint
	startErr   error
	syncErr    error
	syncCalls  int
	syncOut    io.Writer
	syncOutErr io.Writer
}

func (runtime *endpointRuntimeStub) Deployment() config.DeploymentDir {
	return runtime.deployment
}

func (*endpointRuntimeStub) Prepare(
	context.Context, io.Writer, io.Writer, localruntime.PrepareOptions,
) error {
	return nil
}

func (runtime *endpointRuntimeStub) Start(
	context.Context,
	io.Writer,
	io.Writer,
	localruntime.VMConfig,
) error {
	return runtime.startErr
}

func (*endpointRuntimeStub) Stop(context.Context, io.Writer, io.Writer) error {
	return nil
}

func (*endpointRuntimeStub) Status(context.Context) (*localruntime.RuntimeStatus, error) {
	return &localruntime.RuntimeStatus{Running: true}, nil
}

func (*endpointRuntimeStub) Destroy(context.Context, io.Writer, io.Writer) error {
	return nil
}

func (runtime *endpointRuntimeStub) WorkaroundNanoStartupDurability(
	_ context.Context,
	out, outErr io.Writer,
) error {
	runtime.syncCalls++
	runtime.syncOut = out
	runtime.syncOutErr = outErr

	return runtime.syncErr
}

func (runtime *endpointRuntimeStub) ReadEndpoints() (*localruntime.VMRuntimeEndpoint, error) {
	return &localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *runtime.endpoint}, nil
}

func (*endpointRuntimeStub) HealthCheck(
	context.Context,
) (*localruntime.HealthCheckResult, error) {
	return nil, errors.New("not implemented")
}

func (*endpointRuntimeStub) OpenHostShell(
	context.Context,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return localruntime.ErrHostShellUnsupported
}

func (*endpointRuntimeStub) OpenContainerShell(
	context.Context,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return localruntime.ErrContainerShellUnsupported
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
