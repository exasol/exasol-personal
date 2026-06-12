// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/remote"
	"gopkg.in/yaml.v3"
)

const (
	localSupportedOS                        = "darwin"
	localSupportedArch                      = "arm64"
	localAllowUnsupportedEnv                = "EXASOL_LOCAL_ALLOW_UNSUPPORTED_PLATFORM"
	localSkipDatabaseWaitEnv                = "EXASOL_LOCAL_SKIP_DB_WAIT"
	localDefaultCPUCount                    = 2
	localDefaultMemoryMB                    = 2048
	localDefaultDataSizeGB                  = 100
	localDeploymentBackend                  = "local"
	localDeploymentPublicHost               = "127.0.0.1"
	localSSHUser                            = "root"
	localDBUser                             = "sys"
	localDBPassword                         = "exasol"
	localDBContainerName                    = "exasol-local-db"
	localRunnerCompatibilityDBContainerName = "exasol-nano-db"
	localManifestFileMode                   = 0o600
	localCPUCountConfigName                 = "cpu_count"
	localMemoryMBConfigName                 = "memory_mb"
	localDataSizeGBConfigName               = "data_size_gb"
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
	_ context.Context,
	overrides map[string]string,
	_ DeploymentMetadata,
	_ DeploymentLayout,
) error {
	if b.manifest == nil {
		return errors.New("local infrastructure manifest is missing")
	}

	local := ensureLocalManifestConfig(b.manifest)

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

	if _, err := resolveLocalRuntimeConfig(b.manifest); err != nil {
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

	return nil
}

func (b *localBackend) ReadConfiguration() ([]DeploymentConfigValue, error) {
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest)
	if err != nil {
		return nil, err
	}
	defaults := defaultLocalRuntimeConfig()

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
	runtimeConfig, err := resolveLocalRuntimeConfig(manifest)
	if err != nil {
		runtimeConfig = defaultLocalRuntimeConfig()
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
	manifest *presets.InfrastructureManifest,
) *presets.InfrastructureLocal {
	if manifest.Local == nil {
		defaults := defaultLocalRuntimeConfig()
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
	command, err := readLocalAsset(localContainerShellScriptAssetPath)
	if err != nil {
		return "", err
	}

	command = strings.ReplaceAll(command, "__LOCAL_DB_CONTAINER_NAME__", localDBContainerName)
	command = strings.ReplaceAll(
		command,
		"__LOCAL_RUNNER_COMPATIBILITY_DB_CONTAINER_NAME__",
		localRunnerCompatibilityDBContainerName,
	)

	return command, nil
}

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

func defaultLocalRuntimeConfig() localRuntimeConfig {
	return localRuntimeConfig{
		cpuCount:   localDefaultCPUCount,
		memoryMB:   localDefaultMemoryMB,
		dataSizeGB: localDefaultDataSizeGB,
	}
}

func resolveLocalRuntimeConfig(
	manifest *presets.InfrastructureManifest,
) (localRuntimeConfig, error) {
	runtimeConfig := defaultLocalRuntimeConfig()
	if manifest == nil || manifest.Local == nil {
		return runtimeConfig, nil
	}

	if manifest.Local.CPUCount != 0 {
		runtimeConfig.cpuCount = manifest.Local.CPUCount
	}
	if manifest.Local.MemoryMB != 0 {
		runtimeConfig.memoryMB = manifest.Local.MemoryMB
	}
	if manifest.Local.DataSizeGB != 0 {
		runtimeConfig.dataSizeGB = manifest.Local.DataSizeGB
	}
	if runtimeConfig.cpuCount <= 0 {
		return localRuntimeConfig{}, errors.New("local cpuCount must be greater than zero")
	}
	if runtimeConfig.memoryMB <= 0 {
		return localRuntimeConfig{}, errors.New("local memoryMB must be greater than zero")
	}
	if runtimeConfig.dataSizeGB <= 0 {
		return localRuntimeConfig{}, errors.New("local dataSizeGB must be greater than zero")
	}

	return runtimeConfig, nil
}

func (b *localBackend) Deploy(
	ctx context.Context,
	out, outErr io.Writer,
	_ DeployOptions,
) error {
	if err := b.ValidateEnvironment(); err != nil {
		return err
	}
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest)
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
	runtimeConfig, err := resolveLocalRuntimeConfig(b.manifest)
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
