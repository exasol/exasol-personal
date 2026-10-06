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
	noDBPassword bool,
) (sidecar.Container, error) {
	for index, container := range document.Containers {
		if container.Name == name {
			if !noDBPassword {
				return container, nil
			}

			container = sidecar.WithoutDatabasePassword(container)
			document.Containers[index] = container

			return container, config.WriteSidecars(deployment, document)
		}
	}
	container, err := catalog.Resolve(name, architecture)
	if err != nil {
		return sidecar.Container{}, err
	}
	if noDBPassword {
		container = sidecar.WithoutDatabasePassword(container)
	}
	document.Containers = append(document.Containers, container)
	if err := config.WriteSidecars(deployment, document); err != nil {
		return sidecar.Container{}, err
	}

	return container, nil
}
