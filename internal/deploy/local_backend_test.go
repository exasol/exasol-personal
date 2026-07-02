// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
)

type logCaptureHandler struct {
	records []slog.Record
}

func (*logCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *logCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	h.records = append(h.records, record.Clone())

	return nil
}

func (h *logCaptureHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *logCaptureHandler) WithGroup(string) slog.Handler {
	return h
}

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
	runtimeConfig, err := resolveLocalRuntimeConfig(&presets.InfrastructureManifest{}, 0)
	expectedDefaults := defaultLocalRuntimeConfig(0)
	// Then
	if err != nil {
		t.Fatalf("expected default config, got %v", err)
	}
	if runtimeConfig.cpuCount != expectedDefaults.cpuCount ||
		runtimeConfig.memoryMB != expectedDefaults.memoryMB ||
		runtimeConfig.dataSizeGB != expectedDefaults.dataSizeGB {
		t.Fatalf("unexpected default local config: %#v", runtimeConfig)
	}
}

func TestDefaultLocalRuntimeConfig_UsesHalfHostMemoryWhenAvailable(t *testing.T) {
	t.Parallel()

	expectedMemoryMB := 12288
	runtimeConfig := defaultLocalRuntimeConfig(24576)
	if runtimeConfig.memoryMB != expectedMemoryMB {
		t.Fatalf(
			"expected default memory %d MB, got %d",
			expectedMemoryMB,
			runtimeConfig.memoryMB,
		)
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
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, 0)
	// Then
	if err != nil {
		t.Fatalf("expected local config, got %v", err)
	}
	if runtimeConfig.cpuCount != 4 || runtimeConfig.memoryMB != 8192 ||
		runtimeConfig.dataSizeGB != 250 {
		t.Fatalf("unexpected local config: %#v", runtimeConfig)
	}
}

func TestResolveLocalRuntimeConfig_ExplicitMemoryOverridesComputedDefault(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{
			MemoryMB: 8192,
		},
	}

	// When
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, 24576)
	// Then
	if err != nil {
		t.Fatalf("expected local config, got %v", err)
	}
	if runtimeConfig.memoryMB != 8192 {
		t.Fatalf("expected explicit memory override to win, got %d", runtimeConfig.memoryMB)
	}
}

func TestResolveLocalRuntimeConfig_RejectsInvalidValues(t *testing.T) {
	t.Parallel()

	// Given
	manifest := &presets.InfrastructureManifest{
		Local: &presets.InfrastructureLocal{DataSizeGB: -1},
	}

	// When
	_, err := resolveLocalRuntimeConfig(manifest, 0)

	// Then
	if err == nil {
		t.Fatal("expected invalid local config error, got nil")
	}
}

func TestValidateLocalRuntimeConfig_RejectsHostMemoryBelowMinimum(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 4096, dataSizeGB: 100},
		6144,
	)
	if err == nil {
		t.Fatal("expected host memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires at least 8192 MB host memory") {
		t.Fatalf("unexpected host memory error: %v", err)
	}
	if !strings.Contains(err.Error(), "detected 6144 MB") {
		t.Fatalf("unexpected host memory error: %v", err)
	}
}

func TestValidateLocalRuntimeConfig_PrefersHostMemoryError(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 2048, dataSizeGB: 100},
		6144,
	)
	if err == nil {
		t.Fatal("expected host memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires at least 8192 MB host memory") {
		t.Fatalf("unexpected host memory error: %v", err)
	}
	if strings.Contains(err.Error(), "memory-mb must be at least 4096 MB") {
		t.Fatalf("expected host memory error to take precedence, got %v", err)
	}
}

func TestValidateLocalRuntimeConfig_RejectsMemoryBelowMinimum(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 4095, dataSizeGB: 100},
		8192,
	)
	if err == nil {
		t.Fatal("expected minimum memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "local memory-mb must be at least 4096 MB") {
		t.Fatalf("unexpected minimum memory error: %v", err)
	}
}

func TestValidateLocalRuntimeConfig_AcceptsMinimumMemory(t *testing.T) {
	t.Parallel()

	err := validateLocalRuntimeConfig(
		localRuntimeConfig{cpuCount: 2, memoryMB: 4096, dataSizeGB: 100},
		8192,
	)
	if err != nil {
		t.Fatalf("expected minimum memory to be accepted, got %v", err)
	}
}

func TestValidateLocalInitMemory_RejectsOverrideBelowMinimum(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	err := validateLocalInitMemory(
		context.Background(),
		manifest,
		map[string]string{localMemoryMBConfigName: "4095"},
	)
	if err == nil {
		t.Fatal("expected minimum memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "local memory-mb must be at least 4096 MB") {
		t.Fatalf("unexpected minimum memory error: %v", err)
	}
}

func TestValidateLocalInitMemory_AcceptsValidOverride(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeLocal}

	err := validateLocalInitMemory(
		context.Background(),
		manifest,
		map[string]string{localMemoryMBConfigName: "4096"},
	)
	if err != nil {
		t.Fatalf("expected valid override to be accepted, got %v", err)
	}
}

func TestValidateLocalInitMemory_IgnoresNonLocalBackend(t *testing.T) {
	t.Parallel()

	manifest := &presets.InfrastructureManifest{Backend: backendTypeTofu}

	err := validateLocalInitMemory(
		context.Background(),
		manifest,
		map[string]string{localMemoryMBConfigName: "4095"},
	)
	if err != nil {
		t.Fatalf("expected non-local backend to be ignored, got %v", err)
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
	defaults := defaultLocalRuntimeConfig(detectLocalHostMemoryMB(context.Background()))
	assertConfigValue(t, values, localCPUCountConfigName, 4, localDefaultCPUCount)
	assertConfigValue(t, values, localMemoryMBConfigName, 8192, defaults.memoryMB)
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

//nolint:paralleltest // This test temporarily replaces the process-wide default logger.
func TestLocalBackendConfigure_WarnsForLowMemory(t *testing.T) {
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
	logCapture := &logCaptureHandler{}
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(logCapture))
	defer slog.SetDefault(originalLogger)

	// When
	backend := newLocalBackend(deployment, manifest)
	err := backend.Configure(
		context.Background(),
		map[string]string{localMemoryMBConfigName: "8192"},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
	// Then
	if err != nil {
		t.Fatalf("expected local configuration to be written, got %v", err)
	}
	for _, record := range logCapture.records {
		if record.Level == slog.LevelWarn && record.Message == localInfraMemoryNoticeText {
			return
		}
	}
	t.Fatalf("expected warning log %q, got %#v", localInfraMemoryNoticeText, logCapture.records)
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

func TestLocalBackendConfigure_RejectsMemoryBelowMinimum(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	manifest := &presets.InfrastructureManifest{
		Name:        "Local",
		Description: "Local preset",
		Backend:     backendTypeLocal,
		Local:       &presets.InfrastructureLocal{},
	}

	backend := newLocalBackend(deployment, manifest)
	err := backend.Configure(
		context.Background(),
		map[string]string{localMemoryMBConfigName: "4095"},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
	if err == nil {
		t.Fatal("expected minimum memory validation error, got nil")
	}
	if !strings.Contains(err.Error(), "local memory-mb must be at least 4096 MB") {
		t.Fatalf("unexpected minimum memory error: %v", err)
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

func TestLocalContainerShellCommand_UsesMountedContainerRootfs(t *testing.T) {
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
	if strings.Contains(command, "podman exec") {
		t.Fatalf("expected local shell command not to probe container shells, got %q", command)
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
	if !strings.Contains(command, "exasol-local-db") {
		t.Fatalf("expected local shell command to target exasol-local-db, got %q", command)
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
