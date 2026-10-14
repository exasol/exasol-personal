// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"slices"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

// SidecarObservation reports one observed container. A backend reports one per
// container it owns, so a name observed twice surfaces as two observations.
type SidecarObservation struct {
	Name      string
	Running   bool
	State     string
	ExitCode  int
	Matches   bool
	Endpoints []SidecarEndpoint
}

// SidecarInspection reports what a backend currently serves. An unavailable
// host contributes no observations, which leaves reconciliation undecided
// rather than looking like nothing is enabled.
type SidecarInspection struct {
	Available       bool
	DatabaseRunning bool
	RestartRequired bool
	Sidecars        []SidecarObservation
}

// SidecarApplyResult reports what convergence achieved. A host that serves
// nothing yet reports no application, which leaves recorded failures alone
// instead of clearing them against an unseen runtime.
type SidecarApplyResult struct {
	Applied  bool
	Failures map[string]error
}

// sidecarBackend runs a deployment's sidecars. Deployments whose backend does
// not implement it report sidecars as unsupported for their type.
type sidecarBackend interface {
	// ApplySidecars makes the deployment serve exactly the desired sidecars,
	// reporting per-sidecar failures separately from a deployment-wide one.
	ApplySidecars(
		ctx context.Context, desired []sidecar.Container,
	) (SidecarApplyResult, error)
	InspectSidecars(
		ctx context.Context, desired []sidecar.Container,
	) (SidecarInspection, error)
}

var ErrSidecarsUnsupported = errors.New(
	"sidecars are not supported for this deployment type",
)

// sidecarRuntime is what serving sidecars needs from a local runtime: a
// container manager, and whatever publication its platform requires.
type sidecarRuntime interface {
	localinstall.SidecarProvider
	localruntime.PortPublisher
	Status(ctx context.Context) (*localruntime.RuntimeStatus, error)
}

// localSidecars serves a deployment's sidecars through one local runtime.
type localSidecars struct {
	deployment config.DeploymentDir
	runtime    sidecarRuntime
}

func (backend *localBackend) ApplySidecars(
	ctx context.Context, desired []sidecar.Container,
) (SidecarApplyResult, error) {
	return backend.sidecars().ApplySidecars(ctx, desired)
}

func (backend *localBackend) InspectSidecars(
	ctx context.Context, desired []sidecar.Container,
) (SidecarInspection, error) {
	return backend.sidecars().InspectSidecars(ctx, desired)
}

func (backend *localBackend) sidecars() *localSidecars {
	return &localSidecars{deployment: backend.deployment, runtime: backend.runtime}
}

func (backend *localSidecars) ApplySidecars(
	ctx context.Context, desired []sidecar.Container,
) (SidecarApplyResult, error) {
	running, err := backend.runtime.SidecarHostRunning(ctx)
	if err != nil || !running {
		return SidecarApplyResult{}, err
	}
	manager, err := backend.runtime.Sidecars(ctx)
	if err != nil {
		return SidecarApplyResult{}, err
	}
	// Only a deployment that will serve sidecars needs the database reachable
	// among them, so removing them all stays possible either way.
	if len(desired) > 0 {
		restart, err := manager.AttachDatabase(ctx)
		if err != nil {
			return SidecarApplyResult{}, err
		}
		if restart {
			return SidecarApplyResult{}, ErrSidecarRestartRequired
		}
	}
	sources, err := config.SidecarSources(backend.deployment)
	if err != nil {
		return SidecarApplyResult{}, err
	}
	result := SidecarApplyResult{Applied: true, Failures: map[string]error{}}
	for _, definition := range desired {
		if err := manager.Ensure(ctx, definition, sources); err != nil {
			result.Failures[definition.Name] = err
		}
	}
	if err := backend.removeUndesired(ctx, manager, desired, result.Failures); err != nil {
		return result, err
	}

	return result, backend.publish(ctx, manager, desired)
}

func (backend *localSidecars) InspectSidecars(
	ctx context.Context, desired []sidecar.Container,
) (SidecarInspection, error) {
	available, err := backend.runtime.SidecarHostRunning(ctx)
	if err != nil || !available {
		return SidecarInspection{}, err
	}
	manager, err := backend.runtime.Sidecars(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	containers, err := manager.List(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	opened, err := backend.runtime.OpenedPorts(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	inspection := SidecarInspection{Available: true, Sidecars: []SidecarObservation{}}
	inspection.RestartRequired, err = manager.DatabaseRestartRequired(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	status, err := backend.runtime.Status(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	inspection.DatabaseRunning = status.Running
	sources, err := config.SidecarSources(backend.deployment)
	if err != nil {
		return SidecarInspection{}, err
	}
	for _, container := range containers {
		observation, err := observeSidecar(manager, container, desired, opened, sources)
		if err != nil {
			return SidecarInspection{}, err
		}
		inspection.Sidecars = append(inspection.Sidecars, observation)
	}

	return inspection, nil
}

func observeSidecar(
	manager *localinstall.SidecarRuntime,
	container localinstall.ContainerListing,
	desired []sidecar.Container,
	opened []sidecar.PublishedPort,
	sources sidecar.Sources,
) (SidecarObservation, error) {
	name := container.SidecarName()
	observation := SidecarObservation{
		Name:      name,
		Running:   container.Running(),
		State:     container.State,
		ExitCode:  container.ExitCode,
		Endpoints: []SidecarEndpoint{},
	}
	if observation.Running {
		observation.Endpoints = sidecarEndpoints(container, openedFor(opened, name))
	}
	index := slices.IndexFunc(desired, func(c sidecar.Container) bool { return c.Name == name })
	if index < 0 {
		return observation, nil
	}
	matches, err := manager.Matches(container, desired[index], sources)
	observation.Matches = matches

	return observation, err
}

func (backend *localSidecars) removeUndesired(
	ctx context.Context,
	manager *localinstall.SidecarRuntime,
	desired []sidecar.Container,
	failures map[string]error,
) error {
	containers, err := manager.List(ctx)
	if err != nil {
		return err
	}
	for _, name := range ownedNames(containers) {
		if slices.ContainsFunc(desired, func(c sidecar.Container) bool { return c.Name == name }) {
			continue
		}
		if failure := manager.Remove(ctx, name); failure != nil {
			failures[name] = failure
		}
	}

	return nil
}

// publish reports the endpoints the deployment should serve. A runtime whose
// containers bind on the user's machine opens nothing; one behind a boundary
// forwards each declared port to the port the container runtime bound.
func (backend *localSidecars) publish(
	ctx context.Context, manager *localinstall.SidecarRuntime, desired []sidecar.Container,
) error {
	containers, err := manager.List(ctx)
	if err != nil {
		return err
	}
	bound := boundContainerPorts(containers)
	ports := []sidecar.PublishedPort{}
	for _, definition := range desired {
		for index, port := range definition.Defaults().Ports {
			if port.HostPort == 0 {
				continue
			}
			ports = append(ports, sidecar.PublishedPort{
				Sidecar: definition.Name, Index: index,
				HostIP: port.HostIP, HostPort: port.HostPort,
				RuntimePort: bound[sidecarPortKey{definition.Name, port.ContainerPort}],
			})
		}
	}

	return backend.runtime.OpenPorts(ctx, ports)
}

func ownedNames(containers []localinstall.ContainerListing) []string {
	var names []string
	for _, container := range containers {
		if name := container.SidecarName(); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}

	return names
}

func openedFor(opened []sidecar.PublishedPort, name string) []sidecar.PublishedPort {
	ports := make([]sidecar.PublishedPort, 0, len(opened))
	for _, port := range opened {
		if port.Sidecar == name {
			ports = append(ports, port)
		}
	}

	return ports
}

type sidecarPortKey struct {
	sidecar       string
	containerPort int
}

func boundContainerPorts(containers []localinstall.ContainerListing) map[sidecarPortKey]int {
	bound := map[sidecarPortKey]int{}
	for _, container := range containers {
		if !container.Running() {
			continue
		}
		for _, port := range container.Ports {
			key := sidecarPortKey{container.SidecarName(), port.ContainerPort}
			if port.HostPort > 0 && bound[key] == 0 {
				bound[key] = port.HostPort
			}
		}
	}

	return bound
}

// sidecarEndpoints reports the addresses a user can reach. Endpoints the
// runtime opened replace the container runtime's own bindings, which stay
// behind the boundary the runtime crossed.
func sidecarEndpoints(
	container localinstall.ContainerListing,
	published []sidecar.PublishedPort,
) []SidecarEndpoint {
	endpoints := []SidecarEndpoint{}
	if len(published) > 0 {
		for _, port := range published {
			endpoints = append(endpoints, SidecarEndpoint{IP: port.HostIP, Port: port.HostPort})
		}

		return endpoints
	}
	for _, port := range container.Ports {
		if port.HostPort > 0 {
			endpoints = append(endpoints, SidecarEndpoint{IP: port.HostIP, Port: port.HostPort})
		}
	}

	return endpoints
}
