// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

// The deployment operation owns the lock so runtime reconciliation sees the same intent.
func materializeSidecarLocked(
	deployment config.DeploymentDir,
	document sidecar.Document,
	catalog *sidecar.Catalog,
	name, architecture string,
) (sidecar.Container, error) {
	for _, container := range document.Containers {
		if container.Name == name {
			return container, nil
		}
	}
	container, err := catalog.Resolve(name, architecture)
	if err != nil {
		return sidecar.Container{}, err
	}
	document.Containers = append(document.Containers, container)
	if err := config.WriteSidecars(deployment, document); err != nil {
		return sidecar.Container{}, err
	}

	return container, nil
}
