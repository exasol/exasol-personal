// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	localSupportedOS           = "darwin"
	localSupportedArch         = "arm64"
	localAllowUnsupportedEnv   = "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM"
	hostMemoryDefaultDivisor   = 2
	localDefaultCPUCount       = 2
	localMinimumMemoryMB       = 4096
	localMinimumHostMemoryMB   = 8192
	localInfraMemThresholdMB   = 8192
	localInfraMemoryNoticeText = "Info: For medium to heavy local workloads, " +
		"consider increasing VM memory to 8-16 GB."
	localDefaultDataSizeGB     = 100
	localDeploymentBackend     = "local"
	localDeploymentPublicHost  = "127.0.0.1"
	localSSHUser               = "root"
	localDBUser                = "sys"
	localDBPassword            = "exasol"
	localDBContainerName       = "exasol-local-db"
	localLegacyDBContainerName = "exasol-nano-db"
	localManifestFileMode      = 0o600
	localCPUCountConfigName    = "cpu_count"
	localMemoryMBConfigName    = "memory_mb"
	localDataSizeGBConfigName  = "data_size_gb"
)

var errUnsupportedLocalPlatform = errors.New(
	"local deployments are only supported on macOS Apple Silicon",
)

func newLocalBackend(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) *localBackend {
	return &localBackend{deployment: deployment, manifest: manifest}
}

type localBackend struct {
	deployment config.DeploymentDir
	manifest   *presets.InfrastructureManifest
}

func (*localBackend) ValidateEnvironment() error {
	return validateLocalPlatform(runtime.GOOS, runtime.GOARCH, os.Getenv(localAllowUnsupportedEnv))
}

func (*localBackend) SetupWorkspace(_ context.Context) error {
	return nil
}

func (b *localBackend) Configure(
	ctx context.Context,
	overrides map[string]string,
	_ DeploymentMetadata,
	_ DeploymentLayout,
) error {
	if b.manifest == nil {
		return errors.New("local infrastructure manifest is missing")
	}

	local := ensureLocalManifestConfig(ctx, b.manifest)
	if err := applyLocalConfigOverrides(local, overrides); err != nil {
		return err
	}

	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(b.manifest)
	if err != nil {
		return fmt.Errorf("failed to encode local infrastructure manifest: %w", err)
	}
	if err := os.WriteFile(
		b.deployment.InfrastructureManifestPath(),
		data,
		localManifestFileMode,
	); err != nil {
		return fmt.Errorf("failed to write local infrastructure manifest: %w", err)
	}
	if runtimeConfig.memoryMB <= localInfraMemThresholdMB {
		slog.Warn(localInfraMemoryNoticeText, "memory_mb", runtimeConfig.memoryMB)
	}

	return nil
}

// applyLocalConfigOverrides applies raw config overrides onto a local manifest config.
func applyLocalConfigOverrides(
	local *presets.InfrastructureLocal,
	overrides map[string]string,
) error {
	for name, rawValue := range overrides {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		parsed, err := parseLocalPositiveIntConfig(name, strings.TrimSpace(rawValue))
		if err != nil {
			return err
		}

		switch canonicalLocalConfigName(name) {
		case canonicalLocalConfigName(localCPUCountConfigName):
			local.CPUCount = parsed
		case canonicalLocalConfigName(localMemoryMBConfigName):
			local.MemoryMB = parsed
		case canonicalLocalConfigName(localDataSizeGBConfigName):
			local.DataSizeGB = parsed
		default:
			return fmt.Errorf("unknown local infrastructure configuration option %q", name)
		}
	}

	return nil
}

// validateLocalInitMemory validates local memory limits before any files are
// written, so a rejected config leaves the deployment directory untouched. It
// is a no-op for non-local presets.
func validateLocalInitMemory(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
	overrides map[string]string,
) error {
	if manifest == nil || manifest.Backend != backendTypeLocal {
		return nil
	}

	candidate := presets.InfrastructureLocal{}
	if manifest.Local != nil {
		candidate = *manifest.Local
	}
	if err := applyLocalConfigOverrides(&candidate, overrides); err != nil {
		return err
	}

	_, err := resolveLocalRuntimeConfig(
		&presets.InfrastructureManifest{Local: &candidate},
		detectLocalHostMemoryMB(ctx),
	)

	return err
}

func (b *localBackend) ReadConfiguration() ([]DeploymentConfigValue, error) {
	detectedHostMemoryMB := detectLocalHostMemoryMB(context.Background())
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest, detectedHostMemoryMB)
	if err != nil {
		return nil, err
	}
	defaults := defaultLocalRuntimeConfig(detectedHostMemoryMB)

	return []DeploymentConfigValue{
		localIntConfigValue(
			localCPUCountConfigName,
			runtimeConfig.cpuCount,
			defaults.cpuCount,
		),
		localIntConfigValue(
			localMemoryMBConfigName,
			runtimeConfig.memoryMB,
			defaults.memoryMB,
		),
		localIntConfigValue(
			localDataSizeGBConfigName,
			runtimeConfig.dataSizeGB,
			defaults.dataSizeGB,
		),
	}, nil
}

func (b *localBackend) ReadDeploymentConfigVariables() (
	map[string]ConfigVariableDefinition,
	error,
) {
	return localConfigVariableDefinitions(b.manifest), nil
}

func validateLocalPlatform(goos, goarch, allowUnsupported string) error {
	if allowUnsupported != "" {
		return nil
	}
	if goos == localSupportedOS && goarch == localSupportedArch {
		return nil
	}

	return fmt.Errorf("%w (current platform: %s/%s)", errUnsupportedLocalPlatform, goos, goarch)
}

func canonicalLocalConfigName(name string) string {
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")

	return strings.ToLower(strings.TrimSpace(name))
}

func parseLocalPositiveIntConfig(name, rawValue string) (int, error) {
	if rawValue == "" {
		return 0, fmt.Errorf("local infrastructure configuration option %q is empty", name)
	}
	parsed, err := strconv.Atoi(rawValue)
	if err != nil {
		return 0, fmt.Errorf(
			"local infrastructure configuration option %q must be an integer: %w",
			name,
			err,
		)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf(
			"local infrastructure configuration option %q must be greater than zero",
			name,
		)
	}

	return parsed, nil
}

func localIntConfigValue(name string, value, defaultValue int) DeploymentConfigValue {
	return DeploymentConfigValue{
		Name:       name,
		Type:       ConfigVariableTypeNumber,
		Value:      value,
		Default:    defaultValue,
		RawValue:   strconv.Itoa(value),
		RawDefault: strconv.Itoa(defaultValue),
	}
}

func localConfigVariableDefinitions(
	manifest *presets.InfrastructureManifest,
) map[string]ConfigVariableDefinition {
	detectedHostMemoryMB := detectLocalHostMemoryMB(context.Background())
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, detectedHostMemoryMB)
	if err != nil {
		runtimeConfig = defaultLocalRuntimeConfig(detectedHostMemoryMB)
	}

	return map[string]ConfigVariableDefinition{
		localCPUCountConfigName: {
			Name:           localCPUCountConfigName,
			Description:    "Number of CPUs for the Exasol Local VM",
			Type:           ConfigVariableTypeNumber,
			DefaultDisplay: strconv.Itoa(runtimeConfig.cpuCount),
		},
		localMemoryMBConfigName: {
			Name:           localMemoryMBConfigName,
			Description:    "Memory in MB for the Exasol Local VM",
			Type:           ConfigVariableTypeNumber,
			DefaultDisplay: strconv.Itoa(runtimeConfig.memoryMB),
		},
		localDataSizeGBConfigName: {
			Name:           localDataSizeGBConfigName,
			Description:    "Data disk size in GB for the Exasol Local VM",
			Type:           ConfigVariableTypeNumber,
			DefaultDisplay: strconv.Itoa(runtimeConfig.dataSizeGB),
		},
	}
}

func ensureLocalManifestConfig(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
) *presets.InfrastructureLocal {
	if manifest.Local == nil {
		defaults := defaultLocalRuntimeConfig(detectLocalHostMemoryMB(ctx))
		manifest.Local = &presets.InfrastructureLocal{
			CPUCount:   defaults.cpuCount,
			MemoryMB:   defaults.memoryMB,
			DataSizeGB: defaults.dataSizeGB,
		}
	}

	return manifest.Local
}

func (b *localBackend) OpenHostShell(
	ctx context.Context,
	_ string,
) error {
	sshRemote, err := localSSHRemoteUnsafe(b.deployment)
	if err != nil {
		return err
	}

	return sshRemote.Shell(ctx, os.Stdout, os.Stderr)
}

func (b *localBackend) OpenCOSShell(ctx context.Context) error {
	sshRemote, err := localSSHRemoteUnsafe(b.deployment)
	if err != nil {
		return err
	}

	command, err := localContainerShellCommand()
	if err != nil {
		return err
	}

	return sshRemote.RunInteractiveCommand(ctx, command, os.Stdout, os.Stderr)
}

func localContainerShellCommand() (string, error) {
	return readLocalInfrastructureAsset(localContainerShellScriptAssetPath)
}

// localSSHRemoteUnsafe follows the deploy package convention that Unsafe helpers
// must only be called from code that already owns the required deployment lock.
// It does not mean the SSH connection skips additional security checks.
func localSSHRemoteUnsafe(deployment config.DeploymentDir) (*remote.SSHRemote, error) {
	options, err := localSSHConnectionOptions(deployment)
	if err != nil {
		return nil, err
	}

	return remote.NewSshRemote(options), nil
}

func localSSHConnectionOptions(
	deployment config.DeploymentDir,
) (*remote.SSHConnectionOptions, error) {
	info, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return nil, err
	}
	if info.Connection == nil {
		return nil, errors.New("local connection details are missing")
	}

	host := strings.TrimSpace(info.Connection.Host)
	if host == "" {
		host = strings.TrimSpace(info.Connection.PublicIp)
	}
	if host == "" {
		host = localDeploymentPublicHost
	}
	sshPort := strings.TrimSpace(info.Connection.SSHPort)
	if sshPort == "" {
		return nil, errors.New("local SSH port is missing")
	}

	keyPath := localruntime.NewPaths(deployment).PrivateKeyPath
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: could not read SSH key file %s", err, keyPath)
	}

	return &remote.SSHConnectionOptions{
		Host: host,
		User: localSSHUser,
		Port: sshPort,
		Key:  keyData,
	}, nil
}

type localRuntimeConfig struct {
	cpuCount   int
	memoryMB   int
	dataSizeGB int
}

func defaultLocalRuntimeConfig(detectedHostMemoryMB int) localRuntimeConfig {
	// Fall back to the minimum supported VM memory when host detection is unavailable
	memoryMB := localMinimumMemoryMB
	if detectedHostMemoryMB > 0 {
		// The host-memory gate ensures this computed default stays at or above the minimum.
		memoryMB = detectedHostMemoryMB / hostMemoryDefaultDivisor
	}

	return localRuntimeConfig{
		cpuCount:   localDefaultCPUCount,
		memoryMB:   memoryMB,
		dataSizeGB: localDefaultDataSizeGB,
	}
}

func detectLocalHostMemoryMB(ctx context.Context) int {
	// Host memory detection is only implemented for macOS today; other platforms
	// return an error here and fall back to the fixed default, which keeps local
	// sizing deterministic where local deployments are not yet supported.
	memoryMB, err := util.GetTotalMemoryMB(ctx)
	if err != nil {
		return 0
	}

	return int(memoryMB)
}

func resolveLocalRuntimeConfig(
	manifest *presets.InfrastructureManifest,
	detectedHostMemoryMB int,
) (localRuntimeConfig, error) {
	runtimeConfig := defaultLocalRuntimeConfig(detectedHostMemoryMB)
	if manifest != nil && manifest.Local != nil {
		if manifest.Local.CPUCount != 0 {
			runtimeConfig.cpuCount = manifest.Local.CPUCount
		}
		if manifest.Local.MemoryMB != 0 {
			runtimeConfig.memoryMB = manifest.Local.MemoryMB
		}
		if manifest.Local.DataSizeGB != 0 {
			runtimeConfig.dataSizeGB = manifest.Local.DataSizeGB
		}
	}

	if err := validateLocalRuntimeConfig(runtimeConfig, detectedHostMemoryMB); err != nil {
		return localRuntimeConfig{}, err
	}

	return runtimeConfig, nil
}

func validateLocalRuntimeConfig(
	runtimeConfig localRuntimeConfig,
	detectedHostMemoryMB int,
) error {
	if runtimeConfig.cpuCount <= 0 {
		return errors.New("local cpuCount must be greater than zero")
	}
	if runtimeConfig.dataSizeGB <= 0 {
		return errors.New("local dataSizeGB must be greater than zero")
	}
	if detectedHostMemoryMB > 0 && detectedHostMemoryMB < localMinimumHostMemoryMB {
		return fmt.Errorf(
			"local deployment requires at least %d MB host memory (detected %d MB)",
			localMinimumHostMemoryMB,
			detectedHostMemoryMB,
		)
	}
	if runtimeConfig.memoryMB < localMinimumMemoryMB {
		return fmt.Errorf(
			"local memory-mb must be at least %d MB",
			localMinimumMemoryMB,
		)
	}

	return nil
}

func (b *localBackend) Deploy(
	ctx context.Context,
	out, outErr io.Writer,
	_ DeployOptions,
) error {
	if err := b.ValidateEnvironment(); err != nil {
		return err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return err
	}

	return deployLocalRuntime(ctx, b.deployment, runtimeConfig, out, outErr)
}

func (b *localBackend) Start(
	ctx context.Context,
	out, outErr io.Writer,
	_ int,
) error {
	if err := b.ValidateEnvironment(); err != nil {
		return err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return err
	}

	return startLocalRuntime(ctx, b.deployment, runtimeConfig, out, outErr)
}

func (b *localBackend) Stop(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	return stopLocalRuntime(ctx, b.deployment, out, outErr)
}

func (b *localBackend) Destroy(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	return destroyLocalRuntime(ctx, b.deployment, out, outErr)
}
