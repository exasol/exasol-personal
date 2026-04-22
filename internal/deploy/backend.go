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

type deploymentBackend interface {
	ValidateEnvironment() error
	OpenHostShell(ctx context.Context, deployment config.DeploymentDir, selectedNode string) error
	OpenCOSShell(ctx context.Context, deployment config.DeploymentDir) error
	Deploy(
		ctx context.Context,
		deployment config.DeploymentDir,
		manifest *presets.InfrastructureManifest,
		out, outErr io.Writer,
		tofuLockfileMode TofuLockfileMode,
	) error
	Start(
		ctx context.Context,
		deployment config.DeploymentDir,
		manifest *presets.InfrastructureManifest,
		out, outErr io.Writer,
		waitTimeoutSeconds int,
	) error
	Stop(
		ctx context.Context,
		deployment config.DeploymentDir,
		manifest *presets.InfrastructureManifest,
		out, outErr io.Writer,
	) error
	Destroy(
		ctx context.Context,
		deployment config.DeploymentDir,
		manifest *presets.InfrastructureManifest,
		out, outErr io.Writer,
	) error
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

	return backend, nil
}

func resolveBackendForDeployment(
	deployment config.DeploymentDir,
) (deploymentBackend, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}

	backend, err := resolveBackendForManifest(manifest)
	if err != nil {
		return nil, err
	}

	return backend, nil
}

func resolveBackendForManifest(
	manifest *presets.InfrastructureManifest,
) (deploymentBackend, error) {
	backend, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}

	switch backend {
	case backendTypeTofu:
		return tofuBackend{}, nil
	case backendTypeLocal:
		return localBackend{}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDeploymentType, backend)
	}
}
