// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

const (
	backendTypeTofu  = "tofu"
	backendTypeLocal = "local"
)

// DeployOptions carries backend-agnostic options for a single deploy invocation.
// Individual backends interpret the options they understand and ignore the rest.
type DeployOptions struct {
	// UpdateDependencyLockfile signals that the backend may update any
	// dependency lockfile during initialization (e.g. OpenTofu's
	// .terraform.lock.hcl). When false, the backend must treat any such
	// lockfile as read-only.
	UpdateDependencyLockfile bool
}

// deploymentBackend exposes the lifecycle and configuration operations the
// launcher needs from an infrastructure backend, bound to a specific
// deployment directory and its infrastructure manifest.
//
// All methods operate against the deployment and manifest that were supplied
// when the backend was constructed (see newDeploymentBackend).
// nolint: interfacebloat
type deploymentBackend interface {
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
	OpenHostShell(ctx context.Context, selectedNode string) error
	OpenCOSShell(ctx context.Context) error
	Deploy(ctx context.Context, out, outErr io.Writer, options DeployOptions) error
	Start(ctx context.Context, out, outErr io.Writer, waitTimeoutSeconds int) error
	Stop(ctx context.Context, out, outErr io.Writer) error
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

	switch backend {
	case backendTypeTofu:
		return backend, nil
	case backendTypeLocal:
		return backend, nil
	case "":
		return "", fmt.Errorf(
			"%w: infrastructure manifest does not declare a supported backend",
			ErrUnknownDeploymentType,
		)
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownDeploymentType, backend)
	}
}

func newDeploymentBackendForDeployment(
	deployment config.DeploymentDir,
) (deploymentBackend, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}

	return newDeploymentBackend(deployment, manifest)
}

func newDeploymentBackend(
	deployment config.DeploymentDir,
	manifest *presets.InfrastructureManifest,
) (deploymentBackend, error) {
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}

	switch kind {
	case backendTypeTofu:
		return newTofuBackend(deployment, manifest), nil
	case backendTypeLocal:
		return newLocalBackend(deployment, manifest), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDeploymentType, kind)
	}
}

// readInfrastructurePresetConfigVariables exposes a preset's configurable
// infrastructure variables without requiring a deployment directory.
//
// It is used by the CLI to render preset-specific flags on `init` and
// `install`, before any deployment exists.
func readInfrastructurePresetConfigVariables(
	preset PresetRef,
	manifest *presets.InfrastructureManifest,
) (map[string]ConfigVariableDefinition, error) {
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}

	switch kind {
	case backendTypeTofu:
		if manifest.Tofu == nil {
			return map[string]ConfigVariableDefinition{}, nil
		}

		return readTofuPresetConfigVariables(preset, *manifest.Tofu)
	case backendTypeLocal:
		return localConfigVariableDefinitions(manifest), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDeploymentType, kind)
	}
}
