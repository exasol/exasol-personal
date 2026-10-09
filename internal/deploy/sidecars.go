// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

type SidecarEndpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

const (
	sidecarReconciliationComplete = "complete"
	sidecarReconciliationPending  = "pending"
	sidecarReconciliationUnknown  = "unknown"
)

type SidecarResult struct {
	Name    string              `json:"name"`
	Enabled bool                `json:"enabled"`
	Hosts   []SidecarHostResult `json:"hosts"`
}

type SidecarHostResult struct {
	Name               string            `json:"name"`
	Running            bool              `json:"running"`
	RestartRequired    bool              `json:"restartRequired"`
	State              string            `json:"state"`
	Reconciliation     string            `json:"reconciliation"`
	ExitCode           int               `json:"exitCode"`
	Error              string            `json:"error,omitempty"`
	LastOperationError string            `json:"lastOperationError,omitempty"`
	Endpoints          []SidecarEndpoint `json:"endpoints"`
}

type SidecarAvailability struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Architectures []string `json:"architectures"`
	Supported     bool     `json:"supported"`
	Enabled       bool     `json:"enabled"`
}

// sidecarPorts opens the deployment's declared endpoints beyond the hosts that
// run the containers. Deployment backends and local runtimes both answer it.
type sidecarPorts interface {
	OpenPorts(ctx context.Context, ports []sidecar.PublishedPort) error
	OpenedPorts(ctx context.Context) ([]sidecar.PublishedPort, error)
}

type sidecarOperations struct {
	deployment   config.DeploymentDir
	templates    sidecar.Resolver
	catalog      sidecar.Resolver
	hosts        []sidecarHost
	ports        sidecarPorts
	architecture string
}

var ErrSidecarRestartRequired = errors.New(
	"enabled sidecars require a deployment restart",
)

func ListSidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]SidecarAvailability, error) {
	var result []SidecarAvailability
	err := withDeploymentSharedLock(ctx, deployment, func(directory config.DeploymentDir) error {
		ops, err := newSidecarOperations(ctx, directory, nil)
		if err != nil {
			return err
		}
		document, err := config.ReadSidecars(directory)
		if err != nil {
			return err
		}
		if err := ops.loadCatalog(); err != nil {
			return err
		}
		entries := ops.catalog.Entries()
		result = make([]SidecarAvailability, 0, len(entries))
		for _, name := range slices.Sorted(maps.Keys(entries)) {
			entry := entries[name]
			supported := slices.Contains(entry.Architectures, ops.architecture)
			for _, host := range ops.hosts {
				supported = supported && slices.Contains(entry.Architectures, host.architecture)
			}
			result = append(result, SidecarAvailability{
				Name:          name,
				Description:   entry.Description,
				Architectures: slices.Clone(entry.Architectures),
				Supported:     supported,
				Enabled:       containsSidecar(document, name),
			})
		}

		return nil
	})

	return result, err
}

func EnableSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return enableSidecarTemplate(ctx, deployment, nil, name)
}

// enableSidecarTemplate resolves the selection against templates assembled for
// this command before the embedded catalog, so a command can enable a sidecar
// the catalog does not carry.
func enableSidecarTemplate(
	ctx context.Context, deployment config.DeploymentDir, templates sidecar.Resolver,
	name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		templates,
		func(ops *sidecarOperations) (SidecarResult, error) {
			return ops.enable(ctx, name)
		},
	)
}

func DisableSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		nil,
		func(ops *sidecarOperations) (SidecarResult, error) {
			return ops.disable(ctx, name)
		},
	)
}

func GetSidecarStatus(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		nil,
		func(ops *sidecarOperations) (SidecarResult, error) {
			return ops.status(ctx, name)
		},
	)
}

func withSidecar(
	ctx context.Context, deployment config.DeploymentDir, templates sidecar.Resolver,
	action func(*sidecarOperations) (SidecarResult, error),
) (SidecarResult, error) {
	var result SidecarResult
	err := withDeploymentExclusiveLock(ctx, deployment, func(dir config.DeploymentDir) error {
		operations, err := newSidecarOperations(ctx, dir, templates)
		if err != nil {
			return err
		}
		result, err = action(operations)

		return err
	})

	return result, err
}

func newSidecarOperations(
	ctx context.Context,
	deployment config.DeploymentDir,
	templates sidecar.Resolver,
) (*sidecarOperations, error) {
	backend, err := newDeploymentBackendForDeployment(ctx, deployment)
	if err != nil {
		return nil, err
	}
	hosts, err := backend.SidecarHosts(ctx)
	if err != nil {
		return nil, err
	}
	architecture := localLinuxAMD64
	if isLocalDeployment(deployment) {
		architecture = runtime.GOARCH
	}

	return &sidecarOperations{
		deployment:   deployment,
		templates:    templates,
		hosts:        hosts,
		ports:        backend,
		architecture: architecture,
	}, nil
}

func (ops *sidecarOperations) loadCatalog() error {
	if ops.catalog != nil {
		return nil
	}
	embedded, err := sidecar.LoadCatalog(resources.SidecarCatalogYAML)
	if err != nil {
		return err
	}
	catalogs := sidecar.Catalogs{embedded}
	if ops.templates != nil {
		catalogs = sidecar.Catalogs{ops.templates, embedded}
	}
	ops.catalog = catalogs

	return nil
}

func (ops *sidecarOperations) enable(
	ctx context.Context,
	name string,
) (SidecarResult, error) {
	deferred, err := ops.mutationDeferred()
	if err != nil {
		return SidecarResult{}, err
	}
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	if !containsSidecar(document, name) {
		if err := ops.loadCatalog(); err != nil {
			return SidecarResult{}, err
		}
		for _, host := range ops.hosts {
			if _, err := ops.catalog.Resolve(name, host.architecture); err != nil {
				return SidecarResult{}, err
			}
		}
	}
	definition, err := materializeSidecarLocked(
		ops.deployment,
		document,
		ops.catalog,
		name,
		ops.architecture,
	)
	if err != nil {
		return SidecarResult{}, err
	}
	var failures []error
	if !deferred {
		for _, host := range ops.hosts {
			err := ops.ensure(ctx, host, definition)
			if errors.Is(err, ErrSidecarRestartRequired) {
				err = nil
			}
			failures = append(failures, ops.recordFailure(host.name, name, err))
		}
		failures = append(failures, ops.publishPorts(ctx))
	}
	result, err := ops.status(ctx, name)

	return result, errors.Join(append(failures, err)...)
}

func (ops *sidecarOperations) ensure(
	ctx context.Context,
	host sidecarHost,
	definition sidecar.Container,
) error {
	running, err := host.running(ctx)
	if err != nil || !running {
		return err
	}
	manager, err := host.provider.Sidecars(ctx)
	if err != nil {
		return err
	}
	restart, err := manager.AttachDatabase(ctx)
	if err != nil {
		return err
	}
	if restart {
		return ErrSidecarRestartRequired
	}
	sources, err := config.SidecarSources(ops.deployment)
	if err != nil {
		return err
	}

	return manager.Ensure(ctx, definition, sources)
}

func (ops *sidecarOperations) disable(ctx context.Context, name string) (SidecarResult, error) {
	if _, err := ops.mutationDeferred(); err != nil {
		return SidecarResult{}, err
	}
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	document.Containers = slices.DeleteFunc(
		document.Containers,
		func(container sidecar.Container) bool { return container.Name == name },
	)
	if err := config.WriteSidecars(ops.deployment, document); err != nil {
		return SidecarResult{}, err
	}
	var failures []error
	for _, host := range ops.hosts {
		available, err := ops.hostAvailable(ctx, host)
		if err == nil && available {
			var manager localinstall.SidecarManager
			manager, err = host.provider.Sidecars(ctx)
			if err == nil {
				err = manager.Remove(ctx, name)
			}
		}
		if err != nil || available {
			failures = append(failures, ops.recordFailure(host.name, name, err))
		}
	}
	failures = append(failures, ops.publishPorts(ctx))
	result, err := ops.status(ctx, name)

	return result, errors.Join(append(failures, err)...)
}

func (ops *sidecarOperations) status(ctx context.Context, name string) (SidecarResult, error) {
	result := SidecarResult{Name: name, Hosts: []SidecarHostResult{}}
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return result, err
	}
	result.Enabled = containsSidecar(document, name)
	failures, err := config.ReadSidecarState(ops.deployment)
	if err != nil {
		return result, err
	}
	opened, err := ops.ports.OpenedPorts(ctx)
	if err != nil {
		return result, err
	}
	changed := false
	for _, host := range ops.hosts {
		observed := SidecarHostResult{
			Name:               host.name,
			State:              "stopped",
			Reconciliation:     sidecarReconciliationUnknown,
			Endpoints:          []SidecarEndpoint{},
			LastOperationError: failures[host.name+"/"+name],
		}
		if err := ops.inspect(ctx, host, name, document, opened, &observed); err != nil {
			observed.State = slcApplyOutcomeUnknown
			observed.Reconciliation = sidecarReconciliationUnknown
			observed.Error = err.Error()
		}
		if observed.Reconciliation == sidecarReconciliationComplete &&
			observed.LastOperationError != "" {
			delete(failures, host.name+"/"+name)
			observed.LastOperationError = ""
			changed = true
		}
		result.Hosts = append(result.Hosts, observed)
	}
	if changed {
		return result, config.WriteSidecarState(ops.deployment, failures)
	}

	return result, nil
}

func (ops *sidecarOperations) inspect(
	ctx context.Context,
	host sidecarHost,
	name string,
	document sidecar.Document,
	opened []sidecar.PublishedPort,
	result *SidecarHostResult,
) error {
	available, err := ops.hostAvailable(ctx, host)
	if err != nil || !available {
		return err
	}
	manager, err := host.provider.Sidecars(ctx)
	if err != nil {
		return err
	}
	containers, err := manager.List(ctx)
	if err != nil {
		return err
	}
	containers = slices.DeleteFunc(containers, func(container localinstall.SidecarContainer) bool {
		return container.Name() != name
	})
	published := sidecarOpenedPorts(opened, name)
	for _, container := range containers {
		result.Running = container.Running()
		result.State, result.ExitCode = container.State, container.ExitCode
		if result.Running {
			result.Endpoints = sidecarEndpoints(container, published)
		}
	}
	wanted := slices.IndexFunc(document.Containers, func(container sidecar.Container) bool {
		return container.Name == name
	})
	result.Reconciliation = sidecarReconciliationPending
	running := false
	if wanted >= 0 {
		running, err = host.running(ctx)
		if err != nil {
			return err
		}
	}
	if !running {
		if len(containers) == 0 && len(published) == 0 {
			result.Reconciliation = sidecarReconciliationComplete
		}

		return nil
	}
	result.RestartRequired, err = manager.DatabaseRestartRequired(ctx)
	if err != nil || result.RestartRequired || len(containers) != 1 {
		return err
	}
	matches, err := containers[0].Matches(document.Containers[wanted])
	if matches && publicationMatches(document.Containers[wanted], containers[0], published) {
		result.Reconciliation = sidecarReconciliationComplete
	}

	return err
}

type sidecarEndpointKey struct {
	ip   string
	port int
}

// publicationMatches reports whether the deployment serves exactly the
// endpoints the definition declares. A host that publishes directly binds them
// in the container runtime; a host behind a boundary opens them itself, and
// each opened endpoint has to lead to a port the container runtime binds.
func publicationMatches(
	definition sidecar.Container,
	container localinstall.SidecarContainer,
	published []sidecar.PublishedPort,
) bool {
	declared := map[sidecarEndpointKey]int{}
	for _, port := range definition.Defaults().Ports {
		if port.HostPort > 0 {
			declared[sidecarEndpointKey{port.HostIP, port.HostPort}]++
		}
	}
	bound := map[int]bool{}
	for _, port := range container.Ports {
		if port.HostPort > 0 {
			bound[port.HostPort] = true
		}
	}
	if len(published) == 0 {
		for _, port := range container.Ports {
			if port.HostPort > 0 {
				declared[sidecarEndpointKey{port.HostIP, port.HostPort}]--
			}
		}
	}
	for _, port := range published {
		declared[sidecarEndpointKey{port.HostIP, port.HostPort}]--
		if !bound[port.RuntimePort] {
			return false
		}
	}
	for _, count := range declared {
		if count != 0 {
			return false
		}
	}

	return true
}

// sidecarEndpoints reports the addresses a user can reach. Endpoints the
// backend opened replace the container runtime's own bindings, which stay
// behind the host boundary the backend crossed.
func sidecarEndpoints(
	container localinstall.SidecarContainer,
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
			endpoints = append(
				endpoints,
				SidecarEndpoint{IP: port.HostIP, Port: port.HostPort},
			)
		}
	}

	return endpoints
}

func sidecarOpenedPorts(opened []sidecar.PublishedPort, name string) []sidecar.PublishedPort {
	ports := make([]sidecar.PublishedPort, 0, len(opened))
	for _, port := range opened {
		if port.Sidecar == name {
			ports = append(ports, port)
		}
	}

	return ports
}

// publishPorts hands the backend every endpoint the deployment declares,
// annotated with the port the container runtime bound for it. A zero runtime
// port means the endpoint is not currently served, so the backend withdraws it
// while enablement is preserved.
func (ops *sidecarOperations) publishPorts(ctx context.Context) error {
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return err
	}
	bound := ops.boundPorts(ctx)
	ports := []sidecar.PublishedPort{}
	for _, container := range document.Containers {
		for index, port := range container.Defaults().Ports {
			if port.HostPort == 0 {
				continue
			}
			ports = append(ports, sidecar.PublishedPort{
				Sidecar: container.Name, Index: index,
				HostIP: port.HostIP, HostPort: port.HostPort,
				RuntimePort: bound[sidecarPortKey{container.Name, port.ContainerPort}],
			})
		}
	}

	return ops.ports.OpenPorts(ctx, ports)
}

type sidecarPortKey struct {
	sidecar       string
	containerPort int
}

// boundPorts reports where the container runtimes bound each container port.
// Cloud nodes share one definition and use host networking, so every node that
// serves an endpoint reports the same binding. A host the launcher cannot
// inspect contributes nothing, leaving its endpoints withdrawn until it
// answers again.
func (ops *sidecarOperations) boundPorts(ctx context.Context) map[sidecarPortKey]int {
	bound := map[sidecarPortKey]int{}
	for _, host := range ops.hosts {
		available, err := ops.hostAvailable(ctx, host)
		if err != nil || !available {
			continue
		}
		manager, err := host.provider.Sidecars(ctx)
		if err != nil {
			continue
		}
		containers, err := manager.List(ctx)
		if err != nil {
			continue
		}
		for _, container := range containers {
			if !container.Running() {
				continue
			}
			for _, port := range container.Ports {
				key := sidecarPortKey{container.Name(), port.ContainerPort}
				if port.HostPort > 0 && bound[key] == 0 {
					bound[key] = port.HostPort
				}
			}
		}
	}

	return bound
}

func (ops *sidecarOperations) hostAvailable(ctx context.Context, host sidecarHost) (bool, error) {
	workflow, err := ops.workflowState()
	if err != nil {
		return false, err
	}
	if _, initialized := workflow.(*config.WorkflowStateInitialized); initialized {
		return false, nil
	}

	return host.provider.SidecarHostRunning(ctx)
}

func (ops *sidecarOperations) workflowState() (any, error) {
	state, err := config.ReadExasolPersonalState(ops.deployment)
	if err != nil {
		return nil, err
	}

	return state.GetWorkflowState()
}

func (ops *sidecarOperations) mutationDeferred() (bool, error) {
	workflow, err := ops.workflowState()
	if err != nil {
		return false, err
	}
	switch workflow.(type) {
	case *config.WorkflowStateInitialized, *config.WorkflowStateStopped:
		return true, nil
	case *config.WorkflowStateRunning:
		return false, nil
	default:
		return false, errors.New(
			"sidecar changes require an initialized, running, or stopped deployment",
		)
	}
}

func (ops *sidecarOperations) recordFailure(host, name string, failure error) error {
	if failure != nil {
		failure = fmt.Errorf("sidecar %s on host %s: %w", name, host, failure)
	}
	failures, err := config.ReadSidecarState(ops.deployment)
	if err != nil {
		return errors.Join(failure, err)
	}
	key := host + "/" + name
	delete(failures, key)
	if failure != nil {
		failures[key] = failure.Error()
	}

	return errors.Join(failure, config.WriteSidecarState(ops.deployment, failures))
}

func containsSidecar(document sidecar.Document, name string) bool {
	return slices.ContainsFunc(
		document.Containers,
		func(container sidecar.Container) bool { return container.Name == name },
	)
}
