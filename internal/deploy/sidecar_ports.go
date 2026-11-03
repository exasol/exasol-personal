// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

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

func (sidecars *deploymentSidecars) setHostPorts(
	document sidecar.Document, name string, container *sidecar.Container, raw string,
) error {
	reserved := sidecars.reservedHostPorts(document, name)
	named := map[string]bool{}
	for entry := range strings.SplitSeq(raw, ",") {
		portName, _, _ := strings.Cut(entry, ":")
		named[strings.TrimSpace(portName)] = true
	}
	for _, port := range container.Ports {
		if port.HostPort > 0 && port.Name == "" {
			reserved[port.HostPort] = fmt.Sprintf("sidecar %q", name)
		} else if port.HostPort > 0 && !named[port.Name] {
			reserved[port.HostPort] = fmt.Sprintf("sidecar %q port %q", name, port.Name)
		}
	}
	seen := map[string]bool{}
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		portName, rawPort, found := strings.Cut(entry, ":")
		portName = strings.TrimSpace(portName)
		port, err := strconv.Atoi(strings.TrimSpace(rawPort))
		index := slices.IndexFunc(container.Ports, func(candidate sidecar.Port) bool {
			return candidate.Name == portName && candidate.HostPort > 0
		})
		switch {
		case !found || portName == "" || err != nil:
			return fmt.Errorf("port entry %q: expected <port-name>:<host-port>", entry)
		case seen[portName]:
			return fmt.Errorf("port entry %q: port %q is given more than once", entry, portName)
		case port < 1 || port > localMaximumPort:
			return fmt.Errorf("port entry %q: expected a host port from 1 through 65535", entry)
		case index < 0:
			return fmt.Errorf(
				"port entry %q: sidecar %q publishes no port named %q", entry, name, portName,
			)
		}
		if owner, taken := reserved[port]; taken {
			return fmt.Errorf("port entry %q: host port %d is saved for %s", entry, port, owner)
		}
		seen[portName] = true
		reserved[port] = fmt.Sprintf("sidecar %q port %q", name, portName)
		container.Ports[index].HostPort = port
	}

	return nil
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
