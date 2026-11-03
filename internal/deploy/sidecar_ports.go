// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

func savedSidecarHostPorts(document sidecar.Document, except string) map[int]string {
	saved := map[int]string{}
	for _, name := range document.Names() {
		if name == except {
			continue
		}
		for _, port := range document.Sidecars[name].Container.Ports {
			if port.HostPort > 0 {
				saved[port.HostPort] = fmt.Sprintf("sidecar %q", name)
			}
		}
	}

	return saved
}

func (sidecars *deploymentSidecars) reservedHostPorts(
	document sidecar.Document, name string,
) map[int]string {
	reserved := savedSidecarHostPorts(document, name)
	// An unreadable mapping fails the database's own configuration instead.
	mappings, _, err := parseLocalPortMappings(sidecars.databasePorts)
	if err != nil {
		return reserved
	}
	for service, port := range mappings {
		if port > 0 {
			reserved[port] = fmt.Sprintf("local service %q", service)
		}
	}

	return reserved
}

func (sidecars *deploymentSidecars) selectHostPorts(
	ctx context.Context, document sidecar.Document, name string, container *sidecar.Container,
) error {
	reserved := sidecars.reservedHostPorts(document, name)
	services := make([]localService, 0, len(container.Ports))
	for _, port := range container.Ports {
		switch {
		case port.HostPort == 0:
		// A port without a name cannot be selected, so it keeps its host port.
		case port.Name == "":
			reserved[port.HostPort] = fmt.Sprintf("sidecar %q", name)
		default:
			services = append(services, localService{
				name:            port.Name,
				guestPort:       port.ContainerPort,
				defaultHostPort: max(port.HostPort, localMinimumAutomaticPort),
			})
		}
	}
	selected, err := sidecars.ports.allocate(ctx, services, reserved)
	if err != nil {
		return fmt.Errorf("sidecar %q: %w", name, err)
	}
	for index := range container.Ports {
		if port, found := selected[container.Ports[index].Name]; found {
			container.Ports[index].HostPort = port
		}
	}

	return nil
}
