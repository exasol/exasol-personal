// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/assets/localworkloadbin"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeadapter"
)

const (
	localTestDeploymentID    = "exasol-local-test"
	localTestClusterIdentity = "exasol-personal;exasol-local-test;local;local"
)

func TestRuntimeVersionCheckSettingsComeFromLauncherState(t *testing.T) {
	// Given
	deployment := newLocalTestDeployment(t)
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatal(err)
	}
	state.VersionCheckEnabled = true
	state.ClusterIdentity = localTestClusterIdentity
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatal(err)
	}
	const expectedURL = "https://example.test/v1/version-check"
	t.Setenv(VersionCheckURLEnvVar, expectedURL)

	// When
	settings := deriveRuntimeVersionCheckSettings(deployment, state)

	// Then
	if !settings.Enabled || settings.URL != expectedURL ||
		settings.Identity != localTestClusterIdentity ||
		settings.IntervalSeconds != nanoVersionCheckInterval {
		t.Fatalf("unexpected version-check settings: %#v", settings)
	}
}

func TestWriteRuntimeAdapterArtifactsPreservesLocalConnectionContract(t *testing.T) {
	t.Parallel()

	deployment := newLocalTestDeployment(t)
	privateKey := filepath.Join(deployment.Root(), "local", "runtime", "id_ed25519")
	status := &runtimeadapter.RuntimeStatus{
		Phase: runtimeadapter.RuntimePhaseRunning,
		Database: runtimeadapter.RuntimeEndpoint{
			Address: "127.0.0.1",
			Port:    28563,
		},
		VM: &runtimeadapter.VMDetails{
			SSH:            &runtimeadapter.RuntimeEndpoint{Address: "127.0.0.1", Port: 20022},
			PrivateKeyPath: privateKey,
		},
	}
	capabilities := runtimeadapter.RuntimeCapabilities{ContainerShell: true}

	if err := writeRuntimeAdapterArtifacts(deployment, status, capabilities); err != nil {
		t.Fatal(err)
	}
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if info.Backend != localDeploymentBackend ||
		info.DeploymentState != StatusRunning ||
		info.Connection == nil ||
		info.Connection.DBPort != 28563 ||
		info.Connection.SSHPort != "20022" ||
		!info.Connection.ShellSupported ||
		!strings.Contains(info.Connection.SSHCommand, "local/runtime/id_ed25519") {
		t.Fatalf("unexpected local deployment info: %#v", info)
	}
	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if secrets.DbPassword != localDBPassword {
		t.Fatalf("database password = %q", secrets.DbPassword)
	}
}

//nolint:paralleltest // The test temporarily replaces embedded workload readers.
func TestDestroyLocalRuntimeDeletesOnlyExplicitPersonalLocalData(t *testing.T) {
	t.Setenv(localAllowUnsupportedEnv, "1")
	deployment := newLocalTestDeployment(t)
	installLocalRuntimeTestAssets(t)
	writeFakeV2Provider(t, deployment, map[string]any{
		"schemaVersion": 1,
		"phase":         "stopped",
		"hook":          map[string]any{"phase": "none"},
	})
	for _, path := range []string{
		filepath.Join(deployment.Root(), "local", "control", "workload.yaml"),
		filepath.Join(deployment.Root(), "local", "generated", "local-vm.json"),
		filepath.Join(deployment.Root(), "local", "runtime", "legacy-state"),
		filepath.Join(deployment.Root(), "local", "data", "exa", "row-data"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(deployment.Root(), "preserve")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := destroyLocalRuntime(
		context.Background(),
		deployment,
		io.Discard,
		io.Discard,
	); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(deployment.Root(), "local", "data")
	if _, err := os.Stat(dataPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit deployment destroy did not remove Personal data: %v", err)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "outside" {
		t.Fatalf("destroy changed data outside local ownership: content=%q err=%v", content, err)
	}
}

//nolint:paralleltest // The test temporarily replaces workload asset readers.
func TestReconstructLocalRuntimeAdapterDoesNotChangeDeployment(t *testing.T) {
	// Given
	deployment := newLocalTestDeployment(t)
	installLocalRuntimeTestAssets(t)
	before, err := os.ReadFile(deployment.ExasolPersonalStatePath())
	if err != nil {
		t.Fatal(err)
	}

	// When
	prepared, err := reconstructLocalRuntimeAdapter(
		deployment,
		localRuntimeConfig{cpuCount: 2, memoryMB: 8192},
	)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if prepared.spec.LoadImageArchive == nil {
		t.Fatal("runtime reconstruction did not retain the lazy Nano archive loader")
	}
	after, err := os.ReadFile(deployment.ExasolPersonalStatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf(
			"read-only reconstruction changed launcher state:\nbefore=%s\nafter=%s",
			before,
			after,
		)
	}
}

func TestParseLocalDatabasePort(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		raw          string
		port         int
		userOverride bool
		wantError    bool
	}{
		{name: "default", port: 8563},
		{name: "db", raw: "db:9000", port: 9000, userOverride: true},
		{name: "database", raw: "database:9001", port: 9001, userOverride: true},
		{name: "unknown service", raw: "ui:2580", wantError: true},
		{name: "duplicate", raw: "db:9000,database:9001", wantError: true},
		{name: "out of range", raw: "db:65536", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			port, userOverride, err := parseLocalDatabasePort(test.raw)
			if (err != nil) != test.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && (port != test.port || userOverride != test.userOverride) {
				t.Fatalf("got port=%d override=%v", port, userOverride)
			}
		})
	}
}

func TestPreferredLocalDatabasePortUsesStandardDeploymentInfo(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newLocalTestDeployment(t)
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Connection: &config.DeploymentConnection{DBPort: 28563},
	}); err != nil {
		t.Fatal(err)
	}

	// When / Then
	if got := preferredLocalDatabasePort(deployment, 8563, false); got != 28563 {
		t.Fatalf("preferred default port = %d, want previous resolved port 28563", got)
	}
	if got := preferredLocalDatabasePort(deployment, 9000, true); got != 9000 {
		t.Fatalf("preferred explicit port = %d, want strict override 9000", got)
	}
}

func TestResolveLocalDatabasePortPolicy(t *testing.T) {
	t.Parallel()

	const requested = 8563
	listen := func(_ context.Context, address string) (net.Listener, error) {
		if strings.HasSuffix(address, ":0") {
			return &localPortListener{
				address: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 28563},
			}, nil
		}

		return nil, errors.New("address already in use")
	}

	tests := []struct {
		name      string
		policy    localPortSelectionPolicy
		platform  string
		wantPort  func(int) bool
		wantError bool
	}{
		{
			name:     "macOS default delegates dynamic allocation to provider",
			platform: localMacOS,
			wantPort: func(port int) bool {
				return port == 0
			},
		},
		{
			name:      "user override is strict",
			policy:    localPortMustBeExact,
			platform:  localMacOS,
			wantError: true,
		},
		{
			name:     "direct Podman default resolves concrete fallback",
			platform: localWindowsOS,
			wantPort: func(port int) bool {
				return port > 0 && port != requested
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			port, err := resolveLocalDatabasePortUsing(
				context.Background(),
				requested,
				test.policy,
				test.platform,
				listen,
			)
			if (err != nil) != test.wantError {
				t.Fatalf("resolveLocalDatabasePortUsing() error = %v", err)
			}
			if err == nil && !test.wantPort(port) {
				t.Fatalf("resolved port = %d", port)
			}
		})
	}
}

type localPortListener struct {
	address net.Addr
}

func (*localPortListener) Accept() (net.Conn, error) {
	return nil, errors.New("not implemented")
}

func (*localPortListener) Close() error {
	return nil
}

func (listener *localPortListener) Addr() net.Addr {
	return listener.address
}

func newLocalTestDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: local
`)
	state := &config.ExasolPersonalState{
		DeploymentId:      localTestDeploymentID,
		DeploymentVersion: "2.0.0",
	}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatal(err)
	}

	return deployment
}

func installLocalRuntimeTestAssets(t *testing.T) {
	t.Helper()
	oldMetadata := readLocalWorkloadMetadata
	oldArchive := loadLocalWorkloadArchive
	digest := "sha256:" + strings.Repeat("a", 64)
	readLocalWorkloadMetadata = func() (localworkloadbin.Metadata, error) {
		return localworkloadbin.Metadata{
			ImageReference: "example.test/nano@" + digest,
			ImageDigest:    digest,
			ArchiveSHA256:  "sha256:" + strings.Repeat("b", 64),
			Platform:       "darwin/arm64",
		}, nil
	}
	loadLocalWorkloadArchive = func() ([]byte, error) {
		return []byte("test-image"), nil
	}
	t.Cleanup(func() {
		readLocalWorkloadMetadata = oldMetadata
		loadLocalWorkloadArchive = oldArchive
	})
}
