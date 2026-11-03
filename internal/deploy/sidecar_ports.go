// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

type SidecarConfiguration struct {
	Name string `json:"name"`
	// Comma-separated <port-name>:<host-port> entries in declaration order.
	Ports string `json:"ports"`
	// The lifecycle command that applies a saved change the command could not
	// apply itself, or empty when nothing is pending.
	ApplyCommand string `json:"-"`
	// Whether the running sidecar was recreated with the saved ports.
	Applied bool `json:"-"`
}

func newSidecarConfiguration(name string, definition sidecar.Definition) SidecarConfiguration {
	return SidecarConfiguration{Name: name, Ports: hostPortEntries(definition.Container.Ports)}
}

func hostPortEntries(ports []sidecar.Port) string {
	entries := []string{}
	for _, port := range ports {
		if port.HostPort > 0 && port.Name != "" {
			entries = append(entries, port.Name+":"+strconv.Itoa(port.HostPort))
		}
	}

	return strings.Join(entries, ",")
}

func GetSidecarConfiguration(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarConfiguration, error) {
	return withSidecar(
		ctx, deployment, nil,
		func(sidecars *deploymentSidecars) (SidecarConfiguration, error) {
			_, definition, err := sidecars.saved(name)

			return newSidecarConfiguration(name, definition), err
		},
	)
}

// ConfigureSidecarPorts saves the given host ports, or selects free ones as
// enable does when ports is empty, starting at the catalog's host port or,
// when the catalog has none, at the saved one. A running enabled sidecar is
// updated without waiting for it to serve.
func ConfigureSidecarPorts(
	ctx context.Context, deployment config.DeploymentDir, name, ports string,
) (SidecarConfiguration, error) {
	return withSidecar(
		ctx, deployment, nil,
		func(sidecars *deploymentSidecars) (SidecarConfiguration, error) {
			return sidecars.configurePorts(ctx, name, ports)
		},
	)
}

func (sidecars *deploymentSidecars) saved(
	name string,
) (sidecar.Document, sidecar.Definition, error) {
	document, err := config.ReadSidecars(sidecars.deployment)
	if err != nil {
		return sidecar.Document{}, sidecar.Definition{}, err
	}
	definition, saved := document.Sidecars[name]
	if !saved {
		return document, definition, fmt.Errorf(
			"sidecar %q has no saved definition; run `exasol sidecar enable %s` first",
			name, name,
		)
	}

	return document, definition, nil
}

func (sidecars *deploymentSidecars) configurePorts(
	ctx context.Context, name, ports string,
) (SidecarConfiguration, error) {
	deferred, err := sidecars.mutationDeferred()
	if err != nil {
		return SidecarConfiguration{}, err
	}
	document, definition, err := sidecars.saved(name)
	if err != nil {
		return SidecarConfiguration{}, err
	}
	if ports == "" {
		err = sidecars.resetHostPorts(ctx, document, name, &definition.Container, deferred)
	} else {
		err = sidecars.setHostPorts(document, name, &definition.Container, ports)
	}
	if err != nil {
		return SidecarConfiguration{}, err
	}
	document.Sidecars[name] = definition
	if err := config.WriteSidecars(sidecars.deployment, document); err != nil {
		return SidecarConfiguration{}, err
	}
	result := newSidecarConfiguration(name, definition)
	if !definition.IsEnabled() {
		return result, nil
	}
	if deferred {
		state, err := config.ReadExasolPersonalState(sidecars.deployment)
		if err != nil {
			return result, err
		}
		if result.ApplyCommand, err = configurationApplyCommand(state); err != nil {
			return result, err
		}
	}
	_, err = sidecars.applyAndReport(ctx, name)
	result.Applied = !deferred && err == nil

	return result, err
}

func (sidecars *deploymentSidecars) resetHostPorts(
	ctx context.Context,
	document sidecar.Document,
	name string,
	container *sidecar.Container,
	deferred bool,
) error {
	// Sidecars run only with the database, so a deferred change has no running
	// container whose ports it could take over.
	owned := map[int]bool{}
	if !deferred {
		var err error
		if owned, err = sidecars.ownedHostPorts(ctx, document, name); err != nil {
			return err
		}
	}
	if entry, err := sidecars.template(name); err == nil {
		for index, port := range container.Ports {
			template := slices.IndexFunc(entry.Container.Ports, func(candidate sidecar.Port) bool {
				return candidate.Name == port.Name && candidate.HostPort > 0
			})
			if port.HostPort > 0 && template >= 0 {
				container.Ports[index].HostPort = entry.Container.Ports[template].HostPort
			}
		}
	}

	return sidecars.selectHostPorts(ctx, document, name, container, owned)
}

// ownedHostPorts lists the host ports the sidecar's running container
// publishes, which its replacement container can take over.
func (sidecars *deploymentSidecars) ownedHostPorts(
	ctx context.Context, document sidecar.Document, name string,
) (map[int]bool, error) {
	inspection, err := sidecars.inspectSidecars(ctx, document)
	if err != nil {
		return nil, err
	}
	owned := map[int]bool{}
	for _, observation := range observationsFor(inspection.Sidecars, name) {
		for _, endpoint := range observation.Endpoints {
			owned[endpoint.Port] = true
		}
	}

	return owned, nil
}

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
	ctx context.Context,
	document sidecar.Document,
	name string,
	container *sidecar.Container,
	owned map[int]bool,
) error {
	allocator := sidecars.ports
	allocator.owned = owned
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
	selected, err := allocator.allocate(ctx, services, reserved)
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
