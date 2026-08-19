// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

type recordingHostRuntimeEnvironmentPreparer struct {
	called  bool
	options PrepareOptions
	err     error
}

func (preparer *recordingHostRuntimeEnvironmentPreparer) EnsureReady(
	_ context.Context,
	options PrepareOptions,
) error {
	preparer.called = true
	preparer.options = options

	return preparer.err
}

func TestHostRuntimePrepare_DelegatesEnvironmentReadinessBeforeCreatingWorkDir(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	prepareErr := errors.New("environment unavailable")
	preparer := &recordingHostRuntimeEnvironmentPreparer{err: prepareErr}
	localRuntime := newHostRuntime(deployment, nil, preparer)
	progress := &bytes.Buffer{}
	options := PrepareOptions{Progress: progress}

	err := localRuntime.Prepare(context.Background(), nil, nil, options)

	if !errors.Is(err, prepareErr) {
		t.Fatalf("expected preparation failure, got %v", err)
	}
	if !preparer.called {
		t.Fatal("expected environment preparation to run")
	}
	if preparer.options.Progress != progress {
		t.Fatal("expected preparation options to be forwarded")
	}
	if _, statErr := os.Stat(localRuntime.paths.WorkDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no runtime directory before environment readiness, got %v", statErr)
	}
}

func TestHostRuntimePrepare_CreatesWorkDirAfterEnvironmentIsReady(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	preparer := &recordingHostRuntimeEnvironmentPreparer{}
	localRuntime := newHostRuntime(deployment, nil, preparer)

	if err := localRuntime.Prepare(
		context.Background(), nil, nil, PrepareOptions{},
	); err != nil {
		t.Fatalf("expected preparation to succeed, got %v", err)
	}
	if stat, err := os.Stat(localRuntime.paths.WorkDir); err != nil || !stat.IsDir() {
		t.Fatalf("expected runtime work directory, got stat=%v err=%v", stat, err)
	}
}

func TestLinuxHostPodmanStartConfig_UsesReferenceDefaults(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := NewHostLinuxRuntime(deployment, nil)

	// When
	startConfig, err := localRuntime.podmanStartConfig(RuntimeConfig{})
	// Then
	if err != nil {
		t.Fatalf("expected default config, got %v", err)
	}
	if startConfig.ContainerDBPort != nanoDBPort {
		t.Fatalf("unexpected default DB port: %#v", startConfig)
	}
	if startConfig.DataDir != filepath.Join(localRuntime.paths.WorkDir, nanoDataDirName) {
		t.Fatalf("unexpected Nano data directory %q", startConfig.DataDir)
	}
	if len(startConfig.InitParams) != 1 ||
		startConfig.InitParams[0] != "maxConnectionsLicenseLimit=20" {
		t.Fatalf("unexpected Nano init params: %#v", startConfig.InitParams)
	}
}

func TestLinuxHostPodmanStartConfig_UsesOnlyCommonRuntimeConfig(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	vmConfig := VMConfig{
		RuntimeConfig: RuntimeConfig{
			Ports: "ssh:20022, db:28563, ui:28443",
			VersionCheck: localinstall.VersionCheckConfig{
				Enabled:  true,
				URL:      "https://example.test",
				Identity: "cluster-id",
			},
			SLCs: []localinstall.SLCConfig{},
		},
		CPUCount:   32,
		MemoryMB:   131072,
		DataSizeGB: 4096,
	}

	// When
	startConfig, err := localRuntime.podmanStartConfig(vmConfig.RuntimeConfig)
	// Then
	if err != nil {
		t.Fatalf("expected host port override, got %v", err)
	}
	if startConfig.ContainerDBPort != 28563 {
		t.Fatalf("expected published DB port override, got %#v", startConfig)
	}
	if !reflect.DeepEqual(startConfig.VersionCheck, vmConfig.VersionCheck) {
		t.Fatalf("expected portable version-check settings, got %#v", startConfig.VersionCheck)
	}
	if startConfig.SLCs == nil || len(startConfig.SLCs) != 0 {
		t.Fatalf("expected authoritative empty SLC set, got %#v", startConfig.SLCs)
	}
}

func TestLinuxHostReadEndpoint_ReturnsPublishedDatabasePort(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	localRuntime.endpoint = &RuntimeEndpoint{DBPort: 28563}

	// When
	endpoint, err := localRuntime.ReadEndpoints()
	// Then
	if err != nil {
		t.Fatalf("expected endpoint, got %v", err)
	}
	if endpoint.DBPort != 28563 {
		t.Fatalf("expected published DB port 28563, got %#v", endpoint)
	}
	if endpoint.ShellSupported {
		t.Fatalf("expected Linux host shell to be unsupported, got %#v", endpoint)
	}
}

func TestLinuxHostReadEndpoint_RejectsReadBeforeStart(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	// When
	_, err := localRuntime.ReadEndpoints()
	// Then
	if err == nil || !strings.Contains(err.Error(), "endpoint is unavailable") {
		t.Fatalf("expected endpoint availability error, got %v", err)
	}
}

func TestLinuxHostStatus_MapsExactPodmanContainerStatus(t *testing.T) {
	t.Parallel()
	if os.PathSeparator == '\\' {
		t.Skip("fake Podman executable is a POSIX shell script")
	}

	tests := []struct {
		name            string
		containerExists bool
		running         bool
	}{
		{name: "missing"},
		{name: "stopped", containerExists: true},
		{name: "running", containerExists: true, running: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			deployment := newLinuxRuntimeStatusDeployment(t)
			scenarioDir := t.TempDir()
			if test.containerExists {
				writeLinuxRuntimeTestFile(t, filepath.Join(scenarioDir, "existing"), "present")
			}
			if test.running {
				writeLinuxRuntimeTestFile(t, filepath.Join(scenarioDir, "running"), "present")
			}
			scriptPath := filepath.Join(t.TempDir(), "fake-podman.sh")
			writeLinuxRuntimeTestFile(t, scriptPath, fakeLinuxRuntimePodmanScript)
			localRuntime := NewHostLinuxRuntime(deployment, nil)
			localRuntime.runtimeExec = []string{"/bin/sh", scriptPath, scenarioDir}

			// When
			status, err := localRuntime.Status(context.Background())
			// Then
			if err != nil {
				t.Fatalf("expected Linux status to succeed: %v", err)
			}
			if status.Running != test.running {
				t.Fatalf("expected running=%t, got %#v", test.running, status)
			}
		})
	}
}

func TestLinuxHostReadEndpoint_RecoversPublishedPortFromDeploymentInfo(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeLinuxDeploymentInfo(t, deployment, 28563)
	localRuntime := NewHostLinuxRuntime(deployment, nil)

	// When
	endpoint, err := localRuntime.ReadEndpoints()
	// Then
	if err != nil {
		t.Fatalf("expected deployment endpoint recovery: %v", err)
	}
	if endpoint.DBPort != 28563 {
		t.Fatalf("expected recovered DB port 28563, got %#v", endpoint)
	}
}

func TestLinuxHostHealthCheck_ProbesRecoveredPublishedPort(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeLinuxDeploymentInfo(t, deployment, 28563)
	localRuntime := NewHostLinuxRuntime(deployment, nil)
	var dialedNetwork, dialedAddress string
	connection, peer := net.Pipe()
	t.Cleanup(func() {
		_ = connection.Close()
		_ = peer.Close()
	})
	localRuntime.dialContext = func(_ context.Context, network, address string) (net.Conn, error) {
		dialedNetwork, dialedAddress = network, address
		return connection, nil
	}

	// When
	health, err := localRuntime.HealthCheck(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected Linux health check: %v", err)
	}
	if health.Ports["db"].State != PortStateReachable {
		t.Fatalf("expected reachable published DB port, got %#v", health)
	}
	if dialedNetwork != "tcp" || dialedAddress != "127.0.0.1:28563" {
		t.Fatalf("expected published loopback probe, got %s %s", dialedNetwork, dialedAddress)
	}
}

func newLinuxRuntimeStatusDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{DeploymentId: "linux-runtime-status"}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{}, deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}

	return deployment
}

func writeLinuxDeploymentInfo(t *testing.T, deployment config.DeploymentDir, dbPort int) {
	t.Helper()
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		DeploymentId: "linux-runtime-status",
		Connection:   &config.DeploymentConnection{Host: "127.0.0.1", DBPort: dbPort},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
}

func writeLinuxRuntimeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

const fakeLinuxRuntimePodmanScript = `#!/bin/sh
set -eu
scenario_dir=$1
shift
if [ "$1" != "podman" ] || [ "$2" != "container" ]; then
  exit 90
fi
case "$3" in
  exists)
    if [ ! -f "$scenario_dir/existing" ] && [ ! -f "$scenario_dir/running" ]; then
      exit 1
    fi
    ;;
  inspect)
    if [ -f "$scenario_dir/running" ]; then
      printf 'true\n'
    else
      printf 'false\n'
    fi
    ;;
  *) exit 91 ;;
esac
`

func TestResolveHostDBPort_RejectsInvalidMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ports string
	}{
		{name: "missing separator", ports: "db"},
		{name: "missing service", ports: ":8563"},
		{name: "missing port", ports: "db:"},
		{name: "non numeric", ports: "db:abc"},
		{name: "zero", ports: "db:0"},
		{name: "too large", ports: "db:65536"},
		{name: "duplicate DB", ports: "db:8563,db:28563"},
		{name: "malformed ignored service", ports: "ssh:not-a-port,db:8563"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given / When
			_, err := resolveHostDBPort(test.ports)

			// Then
			if err == nil {
				t.Fatalf("expected invalid mapping error for %q", test.ports)
			}
			if !strings.Contains(err.Error(), "local") {
				t.Fatalf("expected contextual local port error, got %v", err)
			}
		})
	}
}
