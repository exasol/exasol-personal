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
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	localMacOS                 = "darwin"
	localMacArch               = "arm64"
	localLinuxOS               = "linux"
	localLinuxAMD64            = "amd64"
	localLinuxARM64            = "arm64"
	localWindowsOS             = "windows"
	localWindowsAMD64          = "amd64"
	hostMemoryDefaultDivisor   = 2
	localDefaultCPUCount       = 2
	localMinimumMemoryMB       = 4096
	localMinimumHostMemoryMB   = 8192
	localInfraMemThresholdMB   = 8192
	localInfraMemoryNoticeText = "Info: For medium to heavy local workloads, " +
		"consider increasing VM memory to 8-16 GB."
	localDefaultDataSizeGB         = 100
	localDeploymentBackend         = "local"
	localDeploymentPublicHost      = "127.0.0.1"
	localDBUser                    = "sys"
	localDBPassword                = "exasol"
	localManifestFileMode          = 0o600
	localCPUCountConfigName        = "cpu_count"
	localMemoryMBConfigName        = "memory_mb"
	localDataSizeGBConfigName      = "data_size_gb"
	localPortsConfigName           = "ports"
	localContainerEngineConfigName = "container_engine"
	localContainerEngineAuto       = "auto"
)

var errUnsupportedLocalPlatform = errors.New(
	"local deployments are only supported on macOS Apple Silicon, " +
		"Linux amd64/arm64, and Windows amd64",
)

// The local backend needs nothing per invocation: its settings are fixed by the
// host platform or persisted as deployment configuration.
var localBackendDescriptor = backendDescriptor{
	kind: backendTypeLocal,
	newBackend: func(
		ctx context.Context,
		deployment config.DeploymentDir,
		manifest *presets.InfrastructureManifest,
		manager *runtimeartifacts.Manager,
	) (deploymentBackend, error) {
		localRuntime, err := newLocalRuntimeForPlatform(
			ctx, deployment, manifest, manager, runtime.GOOS, runtime.GOARCH,
			localruntime.DetectContainerEngine,
		)
		if err != nil {
			return nil, err
		}

		return newLocalBackend(deployment, manifest, localRuntime), nil
	},
	presetConfigVariables: func(
		ctx context.Context,
		_ PresetRef,
		manifest *presets.InfrastructureManifest,
	) (map[string]ConfigVariableDefinition, error) {
		return localConfigVariableDefinitions(ctx, manifest), nil
	},
}

func newLocalBackend(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	localRuntime localruntime.Runtime,
) *localBackend {
	return newLocalBackendForPlatform(
		deployment, manifest, localRuntime, runtime.GOOS, runtime.GOARCH,
	)
}

func newLocalBackendForPlatform(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	localRuntime localruntime.Runtime,
	goos, goarch string,
) *localBackend {
	return &localBackend{
		deployment: deployment,
		manifest:   manifest,
		runtime:    localRuntime,
		goos:       goos,
		goarch:     goarch,
		ports:      newLocalPortAllocator(),
	}
}

type localBackend struct {
	deployment config.DeploymentDir
	manifest   *presets.InfrastructureManifest
	runtime    localruntime.Runtime
	goos       string
	goarch     string
	ports      localPortAllocator
}

type localPlatformCapabilities struct {
	vmSizing         bool
	containerEngines []localinstall.ContainerEngine
}

// Prepare validates the platform, then satisfies the runtime's host
// prerequisites. Both run before the deployment records an operation in
// progress.
func (b *localBackend) Prepare(
	ctx context.Context,
	out, outErr io.Writer,
	options localruntime.PrepareOptions,
) error {
	if err := b.ValidateEnvironment(); err != nil {
		return err
	}
	if err := b.normalizeAutomaticPorts(ctx); err != nil {
		return err
	}

	return b.runtime.Prepare(ctx, out, outErr, options)
}

func (b *localBackend) ValidateEnvironment() error {
	return validateLocalPlatform(b.goos, b.goarch)
}

func (*localBackend) SetupWorkspace(_ context.Context) error {
	return nil
}

func localPortsNeedNormalization(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "auto") {
		return true
	}
	mappings, _, err := parseLocalPortMappings(trimmed)
	if err != nil {
		return false
	}
	for _, service := range localServiceCatalog {
		if port, configured := mappings[service.name]; !configured || port == 0 {
			return true
		}
	}

	return false
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

	capabilities := localCapabilitiesForPlatform(b.goos, b.goarch)
	if len(capabilities.containerEngines) > 0 {
		if err := rejectRecordedContainerEngineChange(b.deployment, overrides); err != nil {
			return err
		}
	}
	local := ensureLocalManifestConfig(ctx, b.manifest, capabilities)
	if err := applyLocalConfigOverrides(local, overrides, capabilities); err != nil {
		return err
	}
	ports, err := b.ports.resolve(ctx, local.Ports, localServiceCatalog)
	if err != nil {
		return err
	}
	local.Ports = ports

	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		b.manifest, detectLocalHostMemoryMB(ctx), b.goos, b.goarch,
	)
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
	if capabilities.vmSizing && runtimeConfig.memoryMB <= localInfraMemThresholdMB {
		slog.Warn(localInfraMemoryNoticeText, "memory_mb", runtimeConfig.memoryMB)
	}

	return nil
}

// applyLocalConfigOverrides applies raw config overrides onto a local manifest config.
func applyLocalConfigOverrides(
	local *presets.InfrastructureLocal,
	overrides map[string]string,
	capabilities localPlatformCapabilities,
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
			if !capabilities.vmSizing {
				continue
			}
			parsed, err := parseLocalPositiveIntConfig(name, rawValue)
			if err != nil {
				return err
			}
			local.CPUCount = parsed
		case canonical == canonicalLocalConfigName(localMemoryMBConfigName):
			if !capabilities.vmSizing {
				continue
			}
			parsed, err := parseLocalPositiveIntConfig(name, rawValue)
			if err != nil {
				return err
			}
			local.MemoryMB = parsed
		case canonical == canonicalLocalConfigName(localDataSizeGBConfigName):
			if !capabilities.vmSizing {
				continue
			}
			parsed, err := parseLocalPositiveIntConfig(name, rawValue)
			if err != nil {
				return err
			}
			local.DataSizeGB = parsed
		case canonical == canonicalLocalConfigName(localContainerEngineConfigName):
			if err := applyLocalContainerEngineOverride(
				local, name, rawValue, capabilities.containerEngines,
			); err != nil {
				return err
			}
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
	return validateLocalInitMemoryForPlatform(
		ctx, manifest, overrides, runtime.GOOS, runtime.GOARCH,
	)
}

// nolint: unparam
func validateLocalInitMemoryForPlatform(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
	overrides map[string]string,
	goos, goarch string,
) error {
	if manifest == nil || manifest.Backend != backendTypeLocal {
		return nil
	}
	if !localPlatformSupportsVMSizing(goos, goarch) {
		return nil
	}

	candidate := presets.InfrastructureLocal{}
	if manifest.Local != nil {
		candidate = *manifest.Local
	}
	capabilities := localCapabilitiesForPlatform(goos, goarch)
	if err := applyLocalConfigOverrides(&candidate, overrides, capabilities); err != nil {
		return err
	}

	_, err := resolveLocalRuntimeConfigForPlatform(
		&presets.InfrastructureManifest{Local: &candidate},
		detectLocalHostMemoryMB(ctx),
		goos,
		goarch,
	)

	return err
}

func (b *localBackend) ReadConfiguration() ([]DeploymentConfigValue, error) {
	detectedHostMemoryMB := 0
	capabilities := localCapabilitiesForPlatform(b.goos, b.goarch)
	if capabilities.vmSizing {
		detectedHostMemoryMB = detectLocalHostMemoryMB(context.Background())
	}
	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		b.manifest, detectedHostMemoryMB, b.goos, b.goarch,
	)
	if err != nil {
		return nil, err
	}
	defaults := defaultLocalRuntimeConfig(detectedHostMemoryMB)

	// nolint: prealloc
	values := []DeploymentConfigValue{{
		Name:       localPortsConfigName,
		Type:       ConfigVariableTypeString,
		Value:      runtimeConfig.ports,
		Default:    "",
		RawValue:   runtimeConfig.ports,
		RawDefault: "",
	}}

	if capabilities.vmSizing {
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
			localIntConfigValue(
				localDataSizeGBConfigName,
				runtimeConfig.dataSizeGB,
				defaults.dataSizeGB,
			),
		)
	}

	if len(capabilities.containerEngines) > 0 {
		stored := ""
		if b.manifest != nil && b.manifest.Local != nil {
			stored = b.manifest.Local.ContainerEngine
		}
		values = append(values,
			localContainerEngineConfigValue(stored),
		)
	}

	return values, nil
}

func (b *localBackend) ReadDeploymentConfigVariables() (
	map[string]ConfigVariableDefinition,
	error,
) {
	return localConfigVariableDefinitionsForPlatform(
		context.Background(), b.manifest, b.goos, b.goarch,
	), nil
}

func validateLocalPlatform(goos, goarch string) error {
	if (goos == localMacOS && goarch == localMacArch) ||
		(goos == localLinuxOS && (goarch == localLinuxAMD64 || goarch == localLinuxARM64)) ||
		(goos == localWindowsOS && goarch == localWindowsAMD64) {
		return nil
	}

	return fmt.Errorf("%w (current platform: %s/%s)", errUnsupportedLocalPlatform, goos, goarch)
}

// localContainerEnginesForPlatform lists the engines a deployment may select,
// most preferred first: `auto` selects the first one installed. The macOS VM
// ships its own engine, so it offers no choice.
func localContainerEnginesForPlatform(goos, goarch string) []localinstall.ContainerEngine {
	switch {
	case goos == localMacOS && goarch == localMacArch:
		return nil
	case goos == localWindowsOS:
		return []localinstall.ContainerEngine{localinstall.EnginePodman, localinstall.EngineDocker}
	default:
		return []localinstall.ContainerEngine{localinstall.EnginePodman, localinstall.EngineDocker}
	}
}

func formatContainerEngines(engines []localinstall.ContainerEngine) string {
	names := make([]string, 0, len(engines))
	for _, engine := range engines {
		names = append(names, string(engine))
	}

	return strings.Join(names, ", ")
}

func localPlatformSupportsVMSizing(goos, goarch string) bool {
	return goos == localMacOS && goarch == localMacArch
}

func localCapabilitiesForPlatform(goos, goarch string) localPlatformCapabilities {
	return localPlatformCapabilities{
		vmSizing:         localPlatformSupportsVMSizing(goos, goarch),
		containerEngines: localContainerEnginesForPlatform(goos, goarch),
	}
}

func canonicalLocalConfigName(name string) string {
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")

	return strings.ToLower(strings.TrimSpace(name))
}

// containerEngineDetector resolves `auto` to an engine on the current host,
// given the platform's engines in order of preference.
type containerEngineDetector func(
	context.Context, []localinstall.ContainerEngine,
) localinstall.ContainerEngine

// localContainerEngine returns the engine a deployment runs on. A deployment
// keeps the engine it was first prepared with; before that, an explicit choice
// is used as given and `auto` is resolved on the current host.
func localContainerEngine(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	goos, goarch string,
	detect containerEngineDetector,
) (localinstall.ContainerEngine, error) {
	engines := localContainerEnginesForPlatform(goos, goarch)
	if len(engines) == 0 {
		return "", fmt.Errorf(
			"%w (current platform: %s/%s)", errUnsupportedLocalPlatform, goos, goarch,
		)
	}
	recorded, isRecorded, err := localruntime.RecordedContainerEngine(deployment)
	if err != nil {
		return "", err
	}
	if isRecorded {
		if !slices.Contains(engines, recorded) {
			return "", fmt.Errorf(
				"the deployment runs on %q, which this platform does not support", recorded,
			)
		}

		return recorded, nil
	}
	configured := ""
	if manifest != nil && manifest.Local != nil {
		configured = manifest.Local.ContainerEngine
	}
	choice, err := parseLocalContainerEngineChoice(
		localContainerEngineConfigName, configured, engines,
	)
	if err != nil {
		return "", err
	}
	if choice == localContainerEngineAuto {
		return detect(ctx, engines), nil
	}

	return localinstall.ContainerEngine(choice), nil
}

// applyLocalContainerEngineOverride stores a given engine choice. Only a
// manifest without the option means auto, so a given value must name one.
func applyLocalContainerEngineOverride(
	local *presets.InfrastructureLocal,
	name, rawValue string,
	supported []localinstall.ContainerEngine,
) error {
	if len(supported) == 0 {
		return nil
	}
	if rawValue == "" {
		return containerEngineChoiceError(name, rawValue, supported)
	}
	choice, err := parseLocalContainerEngineChoice(name, rawValue, supported)
	if err != nil {
		return err
	}
	local.ContainerEngine = choice

	return nil
}

// parseLocalContainerEngineChoice accepts `auto` or an engine the platform
// supports. An empty value is `auto`, which is what manifests without the
// option mean.
func parseLocalContainerEngineChoice(
	name, rawValue string,
	supported []localinstall.ContainerEngine,
) (string, error) {
	choice := strings.ToLower(strings.TrimSpace(rawValue))
	if choice == "" || choice == localContainerEngineAuto {
		return localContainerEngineAuto, nil
	}
	if !slices.Contains(supported, localinstall.ContainerEngine(choice)) {
		return "", containerEngineChoiceError(name, rawValue, supported)
	}

	return choice, nil
}

func containerEngineChoiceError(
	name, rawValue string,
	supported []localinstall.ContainerEngine,
) error {
	return fmt.Errorf(
		"local infrastructure configuration option %q must be one of %s, %s, got %q",
		name, localContainerEngineAuto, formatContainerEngines(supported), rawValue,
	)
}

// rejectRecordedContainerEngineChange refuses to switch a prepared deployment
// to another engine: its data and images live with the engine it started on.
func rejectRecordedContainerEngineChange(
	deployment config.DeploymentDir,
	overrides map[string]string,
) error {
	for name, rawValue := range overrides {
		if canonicalLocalConfigName(name) !=
			canonicalLocalConfigName(localContainerEngineConfigName) {
			continue
		}
		choice := strings.ToLower(strings.TrimSpace(rawValue))
		if choice == "" || choice == localContainerEngineAuto {
			continue
		}
		recorded, isRecorded, err := localruntime.RecordedContainerEngine(deployment)
		if err != nil {
			return err
		}
		if isRecorded && string(recorded) != choice {
			return fmt.Errorf(
				"the deployment runs on %s; run `exasol destroy` before changing its "+
					"container engine to %s. Destroying deletes the database and its data; "+
					"then run `exasol config set --container-engine %s` and `exasol deploy`",
				recorded, choice, choice,
			)
		}
	}

	return nil
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

// localContainerEngineConfigValue reports an unset engine as `auto`, which is
// what manifests without the option mean.
func localContainerEngineConfigValue(stored string) DeploymentConfigValue {
	value := stored
	if value == "" {
		value = localContainerEngineAuto
	}

	return DeploymentConfigValue{
		Name:       localContainerEngineConfigName,
		Type:       ConfigVariableTypeString,
		Value:      value,
		Default:    localContainerEngineAuto,
		RawValue:   value,
		RawDefault: localContainerEngineAuto,
	}
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
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
) map[string]ConfigVariableDefinition {
	return localConfigVariableDefinitionsForPlatform(
		ctx, manifest, runtime.GOOS, runtime.GOARCH,
	)
}

func localConfigVariableDefinitionsForPlatform(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
	goos, goarch string,
) map[string]ConfigVariableDefinition {
	definitions := map[string]ConfigVariableDefinition{
		localPortsConfigName: {
			Name:        localPortsConfigName,
			Description: "Database port override for the local deployment",
			Type:        ConfigVariableTypeString,
		},
	}
	if engines := localCapabilitiesForPlatform(goos, goarch).containerEngines; len(engines) > 0 {
		definitions[localContainerEngineConfigName] = ConfigVariableDefinition{
			Name: localContainerEngineConfigName,
			Description: "Container engine for the local deployment: " +
				localContainerEngineAuto + ", " + formatContainerEngines(engines) +
				". " + localContainerEngineAuto + " uses the first of them that is installed",
			Type: ConfigVariableTypeString,
			// Definitions cannot enumerate allowed values, so the description
			// lists them.
			DefaultDisplay: localContainerEngineAuto,
		}
	}
	if !localPlatformSupportsVMSizing(goos, goarch) {
		return definitions
	}

	detectedHostMemoryMB := detectLocalHostMemoryMB(ctx)
	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		manifest, detectedHostMemoryMB, goos, goarch,
	)
	if err != nil {
		runtimeConfig = defaultLocalRuntimeConfig(detectedHostMemoryMB)
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
	definitions[localDataSizeGBConfigName] = ConfigVariableDefinition{
		Name:           localDataSizeGBConfigName,
		Description:    "VM runtime disk size in GB for Podman images and runtime state.",
		Type:           ConfigVariableTypeNumber,
		DefaultDisplay: strconv.Itoa(runtimeConfig.dataSizeGB),
	}

	return definitions
}

func ensureLocalManifestConfig(
	ctx context.Context,
	manifest *presets.InfrastructureManifest,
	capabilities localPlatformCapabilities,
) *presets.InfrastructureLocal {
	if manifest.Local == nil {
		if !capabilities.vmSizing {
			manifest.Local = &presets.InfrastructureLocal{}
			return manifest.Local
		}
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
	command []string,
) error {
	return b.runtime.OpenHostShell(ctx, command, os.Stdin, os.Stdout, os.Stderr)
}

func (b *localBackend) OpenCOSShell(ctx context.Context) error {
	return b.runtime.OpenContainerShell(ctx, os.Stdin, os.Stdout, os.Stderr)
}

type localRuntimeConfig struct {
	cpuCount   int
	memoryMB   int
	dataSizeGB int
	ports      string
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
	// Host memory is only used for macOS VM sizing. Linux host deployments do not
	// impose launcher-managed resource limits.
	//nolint:staticcheck // SA4023: See comment below.
	memoryMB, err := util.GetTotalMemoryMB(ctx)
	//nolint:staticcheck // SA4023: only true for the unsupported-platform build of
	// util.GetTotalMemoryMB; on darwin it can succeed.
	if err != nil {
		return 0
	}
	if memoryMB > uint64(^uint(0)>>1) {
		return 0
	}

	return int(memoryMB)
}

func resolveLocalRuntimeConfigForPlatform(
	manifest *presets.InfrastructureManifest,
	detectedHostMemoryMB int,
	goos, goarch string,
) (localRuntimeConfig, error) {
	if !localPlatformSupportsVMSizing(goos, goarch) {
		runtimeConfig := localRuntimeConfig{}
		if manifest != nil && manifest.Local != nil {
			runtimeConfig.ports = manifest.Local.Ports
		}

		return runtimeConfig, nil
	}
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
) DeploymentLifecycleResult {
	// Deploy has no caller-supplied timeout in the backend interface, unlike
	// Start; 0 falls back to LocalDatabaseStartedDefaultTimeoutSeconds.
	return b.deployOrStart(ctx, 0, out, outErr)
}

func (b *localBackend) Start(
	ctx context.Context,
	out, outErr io.Writer,
	waitTimeoutSeconds int,
) DeploymentLifecycleResult {
	return b.deployOrStart(ctx, waitTimeoutSeconds, out, outErr)
}

func (b *localBackend) Stop(
	ctx context.Context,
	out, outErr io.Writer,
) DeploymentLifecycleResult {
	return stopLocalRuntime(ctx, b.runtime, out, outErr)
}

func (b *localBackend) Destroy(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	return destroyLocalRuntime(ctx, b.runtime, out, outErr)
}

// normalizeAutomaticPorts persists legacy automatic mappings before any local
// runtime preparation or launch. Configure writes only after allocation succeeds.
func (b *localBackend) normalizeAutomaticPorts(ctx context.Context) error {
	if b.manifest == nil {
		return errors.New("local infrastructure manifest is missing")
	}
	rawPorts := ""
	if b.manifest.Local != nil {
		rawPorts = b.manifest.Local.Ports
	}
	if !localPortsNeedNormalization(rawPorts) {
		return nil
	}

	return b.Configure(
		ctx,
		map[string]string{localPortsConfigName: rawPorts},
		DeploymentMetadata{},
		DeploymentLayout{},
	)
}

// deployOrStart resolves runtime config and starts an already-prepared
// runtime. Deploy and Start differ only in the timeout they pass through.
func (b *localBackend) deployOrStart(
	ctx context.Context,
	waitTimeoutSeconds int,
	out, outErr io.Writer,
) DeploymentLifecycleResult {
	if err := b.ValidateEnvironment(); err != nil {
		return DeploymentLifecycleResult{Database: err}
	}
	runtimeConfig, err := resolveLocalRuntimeConfigForPlatform(
		b.manifest, detectLocalHostMemoryMB(ctx), b.goos, b.goarch,
	)
	if err != nil {
		return DeploymentLifecycleResult{Database: err}
	}

	return startPreparedLocalRuntime(
		ctx, b.runtime, runtimeConfig, waitTimeoutSeconds, out, outErr,
	)
}
