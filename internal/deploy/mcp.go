// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"

	"github.com/exasol/exasol-personal/internal/config"
)

var (
	ErrMCPServerNotEnabled = errors.New("the MCP server is not enabled for this deployment")
	ErrMCPServerNoEndpoint = errors.New(
		"the saved MCP server definition declares no published endpoint",
	)
)

// MCPServerURL reads the endpoint from the saved definition, so it is known
// before the server runs and follows an edited host port.
func MCPServerURL(ctx context.Context, deployment config.DeploymentDir) (string, error) {
	var endpoint string
	err := withDeploymentSharedLock(ctx, deployment, func(directory config.DeploymentDir) error {
		manifest, err := config.ReadInfrastructureManifest(directory)
		if errors.Is(err, config.ErrMissingInfrastructureManifest) {
			return ErrMCPServerNotEnabled
		}
		if err != nil {
			return err
		}
		if kind, err := resolveBackendKind(manifest); err != nil || kind != backendTypeLocal {
			return ErrSidecarsUnsupported
		}
		document, err := config.ReadSidecars(directory)
		if err != nil {
			return err
		}
		definition, found := document.Sidecars[MCPSidecarName]
		if !found || !definition.IsEnabled() {
			return ErrMCPServerNotEnabled
		}
		for _, candidate := range savedSidecarEndpoints(definition) {
			if candidate.URL != "" {
				endpoint = candidate.URL

				return nil
			}
		}

		return ErrMCPServerNoEndpoint
	})

	return endpoint, err
}

// SuggestMCPServer reports whether a deployment could enable the MCP server
// but has not.
func SuggestMCPServer(deployment config.DeploymentDir) bool {
	if !supportsSidecars(deployment) || !CatalogSupports(MCPSidecarName) {
		return false
	}
	document, err := config.ReadSidecars(deployment)

	return err == nil && !enabledSidecar(document, MCPSidecarName)
}
