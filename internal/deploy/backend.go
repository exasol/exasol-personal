// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// DeploymentLifecycleResult keeps a sidecar failure from standing in for the
// database's own condition.
type DeploymentLifecycleResult struct {
	Database error
	Sidecars map[string]error
	// What no single sidecar can be blamed for, such as an unreadable document.
	SidecarReconciliation error
}

func (result DeploymentLifecycleResult) Err() error {
	return errors.Join(
		result.Database,
		result.SidecarReconciliation,
		joinSidecarFailures(result.Sidecars),
	)
}

func joinSidecarFailures(perSidecar map[string]error) error {
	failures := make([]error, 0, len(perSidecar))
	for _, name := range slices.Sorted(maps.Keys(perSidecar)) {
		failures = append(failures, perSidecar[name])
	}

	return errors.Join(failures...)
}

const (
	backendTypeTofu  = "tofu"
	backendTypeLocal = "local"
)

// DeployOptions carries the options for a single deploy invocation.
type DeployOptions struct {
	// Keyed by option name. Validated against the backend's declaration before
	// the backend runs, so a backend only receives options it declared.
	BackendOptions map[string]string

	// RuntimePreparation carries the approval and progress policy applied
	// before the deployment records an operation in progress.
	RuntimePreparation localruntime.PrepareOptions
}

// StartOptions carries options for a single start invocation.
type StartOptions struct {
	WaitTimeoutSeconds int
	RuntimePreparation localruntime.PrepareOptions
}

// deploymentBackend exposes the lifecycle and configuration operations the
// launcher needs from an infrastructure backend, bound to a specific
// deployment directory and its infrastructure manifest.
//
// All methods operate against the deployment and manifest that were supplied
// when the backend was constructed (see newDeploymentBackend).
// nolint: interfacebloat
type deploymentBackend interface {
	// Prepare satisfies host prerequisites. It runs before the deployment
	// records an operation in progress, so a declined or failed
	// prerequisite leaves the deployment retryable.
	Prepare(
		ctx context.Context,
		out, outErr io.Writer,
		options localruntime.PrepareOptions,
	) error
	ValidateEnvironment() error
	SetupWorkspace(ctx context.Context) error
	Configure(
		ctx context.Context,
		overrides map[string]string,
		metadata DeploymentMetadata,
		layout DeploymentLayout,
	) error
	ReadConfiguration() ([]DeploymentConfigValue, error)
	ReadDeploymentConfigVariables() (map[string]ConfigVariableDefinition, error)
	OpenHostShell(ctx context.Context, selectedNode string, command []string) error
	OpenCOSShell(ctx context.Context) error
	Deploy(
		ctx context.Context, out, outErr io.Writer, options DeployOptions,
	) DeploymentLifecycleResult
	Start(
		ctx context.Context, out, outErr io.Writer, waitTimeoutSeconds int,
	) DeploymentLifecycleResult
	Stop(ctx context.Context, out, outErr io.Writer) DeploymentLifecycleResult
	Destroy(ctx context.Context, out, outErr io.Writer) error
}

func resolveBackendKind(manifest *presets.InfrastructureManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("%w: missing infrastructure manifest", ErrUnknownDeploymentType)
	}

	backend := strings.TrimSpace(manifest.Backend)
	if backend == "" && manifest.Tofu != nil {
		backend = backendTypeTofu
	}

	if backend == "" {
		return "", fmt.Errorf(
			"%w: infrastructure manifest does not declare a supported backend",
			ErrUnknownDeploymentType,
		)
	}
	if _, err := backendDescriptorForKind(backend); err != nil {
		return "", err
	}

	return backend, nil
}

func newDeploymentBackendForDeployment(
	ctx context.Context,
	deployment config.DeploymentDir,
) (deploymentBackend, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}

	return newDeploymentBackend(ctx, deployment, manifest)
}

func newDeploymentBackend(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) (deploymentBackend, error) {
	descriptor, err := backendDescriptorForManifest(manifest)
	if err != nil {
		return nil, err
	}

	return descriptor.newBackend(ctx, deployment, manifest, runtimeartifacts.FromContext(ctx))
}

// readInfrastructurePresetConfigVariables exposes a preset's configurable
// infrastructure variables without requiring a deployment directory.
//
// It is used by the CLI to render preset-specific flags on `init` and
// `install`, before any deployment exists.
func readInfrastructurePresetConfigVariables(
	ctx context.Context,
	preset PresetRef,
	manifest *presets.InfrastructureManifest,
) (map[string]ConfigVariableDefinition, error) {
	descriptor, err := backendDescriptorForManifest(manifest)
	if err != nil {
		return nil, err
	}

	return descriptor.presetConfigVariables(ctx, preset, manifest)
}

// newLocalRuntime selects the runtime of an existing deployment from its
// stored manifest.
func newLocalRuntime(
	ctx context.Context,
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) (localruntime.Runtime, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}

	return newLocalRuntimeForPlatform(
		ctx, deployment, manifest, manager, runtime.GOOS, runtime.GOARCH,
		localruntime.DetectContainerEngine,
	)
}

// newLocalRuntimeForPlatform takes the manifest rather than reading it, because
// init selects a runtime before the deployment directory holds one.
func newLocalRuntimeForPlatform(
	ctx context.Context,
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
	manager *runtimeartifacts.Manager,
	goos, goarch string,
	detect containerEngineDetector,
) (localruntime.Runtime, error) {
	switch {
	case goos == localMacOS && goarch == localMacArch:
		return localruntime.NewMacVMRuntime(deployment, manager), nil
	case goos == localLinuxOS && (goarch == localLinuxAMD64 || goarch == localLinuxARM64):
		engine, err := localContainerEngine(ctx, deployment, manifest, goos, goarch, detect)
		if err != nil {
			return nil, err
		}

		return localruntime.NewHostLinuxRuntime(deployment, manager, engine), nil
	case goos == localWindowsOS && goarch == localWindowsAMD64:
		engine, err := localContainerEngine(ctx, deployment, manifest, goos, goarch, detect)
		if err != nil {
			return nil, err
		}

		return localruntime.NewHostWindowsRuntime(deployment, manager, engine), nil
	default:
		return nil, fmt.Errorf(
			"%w (current platform: %s/%s)", errUnsupportedLocalPlatform, goos, goarch,
		)
	}
}
