// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestValidateLocalPlatform_AcceptsMacOSAppleSilicon(t *testing.T) {
	t.Parallel()

	// Given / When
	err := validateLocalPlatform("darwin", "arm64", "")
	// Then
	if err != nil {
		t.Fatalf("expected platform to be accepted, got %v", err)
	}
}

func TestValidateLocalPlatform_RejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	// Given / When
	err := validateLocalPlatform("linux", "amd64", "")

	// Then
	if !errors.Is(err, errUnsupportedLocalPlatform) {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

func TestValidateLocalPlatform_AllowsUnsupportedPlatformForTests(t *testing.T) {
	t.Parallel()

	// Given / When
	err := validateLocalPlatform("linux", "amd64", "1")
	// Then
	if err != nil {
		t.Fatalf("expected override to accept platform, got %v", err)
	}
}

func TestResolveLocalRuntimeConfig_UsesDefaults(t *testing.T) {
	t.Parallel()

	// Given / When
	runtimeConfig, err := resolveLocalRuntimeConfig(&presets.InfrastructureManifest{})
	// Then
	if err != nil {
		t.Fatalf("expected default config, got %v", err)
	}
	if runtimeConfig.cpuCount != 2 || runtimeConfig.memoryMB != 2048 ||
		runtimeConfig.dataSizeGB != 100 {
		t.Fatalf("unexpected default local config: %#v", runtimeConfig)
	}
}

func TestResolveLocalRuntimeConfig_UsesManifestValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{
			CPUCount:   4,
			MemoryMB:   8192,
			DataSizeGB: 250,
		},
	}

	// When
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest)
	// Then
	if err != nil {
		t.Fatalf("expected local config, got %v", err)
	}
	if runtimeConfig.cpuCount != 4 || runtimeConfig.memoryMB != 8192 ||
		runtimeConfig.dataSizeGB != 250 {
		t.Fatalf("unexpected local config: %#v", runtimeConfig)
	}
}

func TestResolveLocalRuntimeConfig_RejectsInvalidValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{DataSizeGB: -1},
	}

	// When
	_, err := resolveLocalRuntimeConfig(manifest)

	// Then
	if err == nil {
		t.Fatal("expected invalid local config error, got nil")
	}
}

func TestLocalBackendSetupWorkspace_Noops(t *testing.T) {
	t.Parallel()

	// Given / When
	backend := newLocalBackend(
		config.NewDeploymentDir(t.TempDir()),
		&presets.InfrastructureManifest{},
	)
	err := backend.SetupWorkspace(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected local workspace setup to no-op, got %v", err)
	}
}

func TestLocalBackendReadConfiguration_ExposesSizingValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{
			CPUCount:   4,
			MemoryMB:   8192,
			DataSizeGB: 250,
		},
	}

	// When
	backend := newLocalBackend(config.NewDeploymentDir(t.TempDir()), manifest)
	values, err := backend.ReadConfiguration()
	// Then
	if err != nil {
		t.Fatalf("expected local configuration values, got %v", err)
	}
	assertConfigValue(t, values, localCPUCountConfigName, 4, localDefaultCPUCount)
	assertConfigValue(t, values, localMemoryMBConfigName, 8192, localDefaultMemoryMB)
	assertConfigValue(t, values, localDataSizeGBConfigName, 250, localDefaultDataSizeGB)
}

func TestLocalBackendConfigure_WritesSizingValuesToManifest(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o750); err != nil {
		t.Fatalf("failed to create infrastructure dir: %v", err)
	}
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}

	// When
	backend := newLocalBackend(deployment, manifest)
	err := backend.Configure(
		context.Background(),
		map[string]string{
			localCPUCountConfigName:   "4",
			localMemoryMBConfigName:   "8192",
			localDataSizeGBConfigName: "250",
		},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
	// Then
	if err != nil {
		t.Fatalf("expected local configuration to be written, got %v", err)
	}
	written, err := presets.ReadInfrastructureManifestFromDir(deployment.InfrastructureDir())
	if err != nil {
		t.Fatalf("expected local infrastructure manifest to be readable, got %v", err)
	}
	if written.Local == nil {
		t.Fatal("expected local manifest configuration, got nil")
	}
	if written.Local.CPUCount != 4 ||
		written.Local.MemoryMB != 8192 ||
		written.Local.DataSizeGB != 250 {
		t.Fatalf("unexpected local manifest configuration: %#v", written.Local)
	}
}

func TestLocalBackendConfigure_RejectsInvalidSizingValues(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}

	// When
	backend := newLocalBackend(deployment, manifest)
	err := backend.Configure(
		context.Background(),
		map[string]string{localCPUCountConfigName: "0"},
		DeploymentMetadata{},
		DeploymentLayout{},
	)

	// Then
	if err == nil {
		t.Fatal("expected invalid local configuration error, got nil")
	}
}

func TestLocalSSHConnectionOptions_UsesConnectionMetadataWithoutNodes(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	keyData := []byte("fake private key")
	keyPath := localruntime.NewPaths(deployment).PrivateKeyPath
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o750); err != nil {
		t.Fatalf("failed to create key directory: %v", err)
	}
	if err := os.WriteFile(keyPath, keyData, 0o600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      localDeploymentBackend,
		DeploymentId: "local-test",
		Connection: &config.DeploymentConnection{
			Host:    "127.0.0.1",
			DBPort:  28563,
			SSHPort: "20022",
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	options, err := localSSHConnectionOptions(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected local SSH options, got %v", err)
	}
	if options.Host != "127.0.0.1" {
		t.Fatalf("expected host %q, got %q", "127.0.0.1", options.Host)
	}
	if options.Port != "20022" {
		t.Fatalf("expected port %q, got %q", "20022", options.Port)
	}
	if options.User != "root" {
		t.Fatalf("expected user %q, got %q", "root", options.User)
	}
	if string(options.Key) != string(keyData) {
		t.Fatal("expected SSH key data to be read from local runtime path")
	}
}

func TestLocalContainerShellCommand_UsesPodmanDirectlyWithMountedRootfsFallback(t *testing.T) {
	t.Parallel()

	// Given / When
	command, err := localContainerShellCommand()
	// Then
	if err != nil {
		t.Fatalf("expected local shell command to render, got error: %v", err)
	}
	if strings.Contains(command, "doas") {
		t.Fatalf("expected local shell command to avoid doas, got %q", command)
	}
	if !strings.Contains(command, "podman exec -it") {
		t.Fatalf("expected local shell command to run podman exec interactively, got %q", command)
	}
	if !strings.Contains(command, "podman mount") {
		t.Fatalf(
			"expected local shell command to fall back to mounted container rootfs, got %q",
			command,
		)
	}
	if !strings.Contains(command, "nsenter") {
		t.Fatalf("expected local shell command to enter container namespaces, got %q", command)
	}
	if !strings.Contains(command, localDBContainerName) {
		t.Fatalf("expected local shell command to target %q, got %q", localDBContainerName, command)
	}
	if !strings.Contains(command, localRunnerCompatibilityDBContainerName) {
		t.Fatalf(
			"expected local shell command to support runner container %q, got %q",
			localRunnerCompatibilityDBContainerName,
			command,
		)
	}
	if strings.Contains(command, "container_name=container") {
		t.Fatalf("expected no generic container fallback, got %q", command)
	}
}

func assertConfigValue(
	t *testing.T,
	values []DeploymentConfigValue,
	name string,
	expectedValue int,
	expectedDefault int,
) {
	t.Helper()

	for _, value := range values {
		if value.Name != name {
			continue
		}
		if value.Type != ConfigVariableTypeNumber {
			t.Fatalf("expected %s type %q, got %q", name, ConfigVariableTypeNumber, value.Type)
		}
		if value.Value != expectedValue {
			t.Fatalf("expected %s value %d, got %v", name, expectedValue, value.Value)
		}
		if value.Default != expectedDefault {
			t.Fatalf("expected %s default %d, got %v", name, expectedDefault, value.Default)
		}

		return
	}

	t.Fatalf("expected configuration value %q in %#v", name, values)
}
