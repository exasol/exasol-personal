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
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeadapter"
	"github.com/exasol/exasol-personal/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	localMacOS                 = "darwin"
	localMacArch               = "arm64"
	localWindowsOS             = "windows"
	localWindowsArch           = "amd64"
	localAllowUnsupportedEnv   = "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM"
	hostMemoryDefaultDivisor   = 2
	localDefaultCPUCount       = 2
	localMinimumMemoryMB       = 4096
	localMinimumHostMemoryMB   = 8192
	localInfraMemThresholdMB   = 8192
	localInfraMemoryNoticeText = "Info: For medium to heavy local workloads, " +
		"consider increasing VM memory to 8-16 GB."
	localDeploymentBackend    = "local"
	localDeploymentPublicHost = "127.0.0.1"
	localSSHUser              = "root"
	localDBUser               = "sys"
	localDBPassword           = "exasol"
	localManifestFileMode     = 0o600
	localCPUCountConfigName   = "cpu_count"
	localMemoryMBConfigName   = "memory_mb"
	localPortsConfigName      = "ports"
)

var errUnsupportedLocalPlatform = errors.New(
	"local deployments require macOS Apple Silicon or Windows amd64 with WSL2",
)

func newLocalBackend(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) *localBackend {
	return &localBackend{
		deployment: deployment,
		manifest:   manifest,
		platform:   runtime.GOOS,
	}
}

// localPlatformCapabilities returns only the product surface implemented for
// this build target. Unsupported development hosts must not advertise macOS
// capabilities.
func localPlatformCapabilities() runtimeadapter.RuntimeCapabilities {
	return runtimeadapter.PlatformCapabilities(runtime.GOOS)
}

type localBackend struct {
	deployment config.DeploymentDir
	manifest   *presets.InfrastructureManifest
	platform   string
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

	capabilities := runtimeadapter.PlatformCapabilities(b.platform)
	local := ensureLocalManifestConfig(ctx, b.manifest, capabilities)
	if err := applyLocalConfigOverrides(local, overrides, capabilities); err != nil {
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
	if runtime.GOOS != localWindowsOS && runtimeConfig.memoryMB <= localInfraMemThresholdMB {
		slog.Warn(localInfraMemoryNoticeText, "memory_mb", runtimeConfig.memoryMB)
	}

	return nil
}

// applyLocalConfigOverrides applies raw config overrides onto a local manifest config.
func applyLocalConfigOverrides(
	local *presets.InfrastructureLocal,
	overrides map[string]string,
	capabilities runtimeadapter.RuntimeCapabilities,
) error {
	for name, rawValue := range overrides {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		rawValue = strings.TrimSpace(rawValue)

		canonical := canonicalLocalConfigName(name)
		switch {
		case canonical == canonicalLocalConfigName(localPortsConfigName):
			local.Ports = rawValue
		case canonical == canonicalLocalConfigName(localCPUCountConfigName):
			if !capabilities.Resources {
				return errors.New("CPU sizing is not supported on this local platform")
			}
			parsed, err := parseLocalPositiveIntConfig(name, rawValue)
			if err != nil {
				return err
			}
			local.CPUCount = parsed
		case canonical == canonicalLocalConfigName(localMemoryMBConfigName):
			if !capabilities.Resources {
				return errors.New("memory sizing is not supported on this local platform")
			}
			parsed, err := parseLocalPositiveIntConfig(name, rawValue)
			if err != nil {
				return err
			}
			local.MemoryMB = parsed
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
	return validateLocalInitMemoryWithCapabilities(
		ctx,
		manifest,
		overrides,
		localPlatformCapabilities(),
	)
}

func validateLocalInitMemoryWithCapabilities(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
	overrides map[string]string,
	capabilities runtimeadapter.RuntimeCapabilities,
) error {
	if manifest == nil || manifest.Backend != backendTypeLocal {
		return nil
	}

	candidate := presets.InfrastructureLocal{}
	if manifest.Local != nil {
		candidate = *manifest.Local
	}
	if err := applyLocalConfigOverrides(&candidate, overrides, capabilities); err != nil {
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

	values := []DeploymentConfigValue{
		{
			Name:       localPortsConfigName,
			Type:       ConfigVariableTypeString,
			Value:      runtimeConfig.ports,
			Default:    "",
			RawValue:   runtimeConfig.ports,
			RawDefault: "",
		},
	}
	if !runtimeadapter.PlatformCapabilities(b.platform).Resources {
		return values, nil
	}
	values = append(values,
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
	)

	return values, nil
}

func (b *localBackend) ReadDeploymentConfigVariables() (
	map[string]ConfigVariableDefinition,
	error,
) {
	return localConfigVariableDefinitionsWithCapabilities(
		b.manifest,
		runtimeadapter.PlatformCapabilities(b.platform),
	), nil
}

func validateLocalPlatform(goos, goarch, allowUnsupported string) error {
	if allowUnsupported != "" {
		return nil
	}
	if (goos == localMacOS && goarch == localMacArch) ||
		(goos == localWindowsOS && goarch == localWindowsArch) {
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
	return localConfigVariableDefinitionsWithCapabilities(
		manifest,
		localPlatformCapabilities(),
	)
}

func localConfigVariableDefinitionsWithCapabilities(
	manifest *presets.InfrastructureManifest,
	capabilities runtimeadapter.RuntimeCapabilities,
) map[string]ConfigVariableDefinition {
	detectedHostMemoryMB := detectLocalHostMemoryMB(context.Background())
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest, detectedHostMemoryMB)
	if err != nil {
		runtimeConfig = defaultLocalRuntimeConfig(detectedHostMemoryMB)
	}

	definitions := map[string]ConfigVariableDefinition{
		localPortsConfigName: {
			Name:        localPortsConfigName,
			Description: "Database port override for the local deployment",
			Type:        ConfigVariableTypeString,
		},
	}
	if !capabilities.Resources {
		return definitions
	}
	definitions[localCPUCountConfigName] = ConfigVariableDefinition{
		Name:           localCPUCountConfigName,
		Description:    "Number of CPUs for the Exasol Local VM",
		Type:           ConfigVariableTypeNumber,
		DefaultDisplay: strconv.Itoa(runtimeConfig.cpuCount),
	}
	definitions[localMemoryMBConfigName] = ConfigVariableDefinition{
		Name:           localMemoryMBConfigName,
		Description:    "Memory in MB for the Exasol Local VM",
		Type:           ConfigVariableTypeNumber,
		DefaultDisplay: strconv.Itoa(runtimeConfig.memoryMB),
	}

	return definitions
}

func ensureLocalManifestConfig(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
	capabilities runtimeadapter.RuntimeCapabilities,
) *presets.InfrastructureLocal {
	if manifest.Local == nil {
		if !capabilities.Resources {
			manifest.Local = &presets.InfrastructureLocal{}
			return manifest.Local
		}
		defaults := defaultLocalRuntimeConfig(detectLocalHostMemoryMB(ctx))
		manifest.Local = &presets.InfrastructureLocal{
			CPUCount: defaults.cpuCount,
			MemoryMB: defaults.memoryMB,
		}
	}

	return manifest.Local
}

func (b *localBackend) OpenHostShell(
	ctx context.Context,
	_ string,
) error {
	prepared, err := b.reconstructedRuntimeAdapter(ctx)
	if err != nil {
		return err
	}
	if !prepared.adapter.Capabilities().VMShell {
		return errors.New("VM shells are not supported on this local platform")
	}

	return prepared.adapter.Shell(
		ctx,
		prepared.spec,
		runtimeadapter.ShellVM,
		os.Stdin,
		os.Stdout,
		os.Stderr,
	)
}

func (b *localBackend) OpenCOSShell(ctx context.Context) error {
	prepared, err := b.reconstructedRuntimeAdapter(ctx)
	if err != nil {
		return err
	}
	if !prepared.adapter.Capabilities().ContainerShell {
		return errors.New("container shells are not supported on this local platform")
	}

	return prepared.adapter.Shell(
		ctx,
		prepared.spec,
		runtimeadapter.ShellContainer,
		os.Stdin,
		os.Stdout,
		os.Stderr,
	)
}

type localRuntimeConfig struct {
	cpuCount int
	memoryMB int
	ports    string
}

func defaultLocalRuntimeConfig(detectedHostMemoryMB int) localRuntimeConfig {
	// Fall back to the minimum supported VM memory when host detection is unavailable
	memoryMB := localMinimumMemoryMB
	if detectedHostMemoryMB > 0 {
		// The host-memory gate ensures this computed default stays at or above the minimum.
		memoryMB = detectedHostMemoryMB / hostMemoryDefaultDivisor
	}

	return localRuntimeConfig{
		cpuCount: localDefaultCPUCount,
		memoryMB: memoryMB,
	}
}

func detectLocalHostMemoryMB(ctx context.Context) int {
	// Host memory detection is only implemented for macOS today; other platforms
	// return an error here and fall back to the fixed default, which keeps local
	// sizing deterministic where local deployments are not yet supported.
	//nolint:staticcheck // SA4023: See comment below.
	memoryMB, err := util.GetTotalMemoryMB(ctx)
	//nolint:staticcheck // SA4023: only true for the unsupported-platform build of
	// util.GetTotalMemoryMB; on darwin it can succeed.
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
		// DataSizeGB is intentionally ignored when reading legacy manifests.
		runtimeConfig.ports = manifest.Local.Ports
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

	// Deploy has no caller-supplied timeout in the backend interface, unlike
	// Start; 0 falls back to LocalDatabaseStartedDefaultTimeoutSeconds.
	return deployLocalRuntime(ctx, b.deployment, runtimeConfig, 0, out, outErr)
}

func (b *localBackend) Start(
	ctx context.Context,
	out, outErr io.Writer,
	waitTimeoutSeconds int,
) error {
	if err := b.ValidateEnvironment(); err != nil {
		return err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return err
	}

	return startLocalRuntime(ctx, b.deployment, runtimeConfig, waitTimeoutSeconds, out, outErr)
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

func (b *localBackend) reconstructedRuntimeAdapter(
	ctx context.Context,
) (*preparedLocalRuntime, error) {
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest, detectLocalHostMemoryMB(ctx))
	if err != nil {
		return nil, err
	}
	prepared, err := reconstructLocalRuntimeAdapter(
		b.deployment,
		runtimeConfig,
	)

	return prepared, err
}
