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

type sidecarOperations struct {
	deployment   config.DeploymentDir
	catalog      *sidecar.Catalog
	hosts        []sidecarHost
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
		ops, err := newSidecarOperations(ctx, directory)
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
		result = make([]SidecarAvailability, 0, len(ops.catalog.Sidecars))
		for _, name := range slices.Sorted(maps.Keys(ops.catalog.Sidecars)) {
			entry := ops.catalog.Sidecars[name]
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
	ctx context.Context, deployment config.DeploymentDir, name string, noDBPassword bool,
) (SidecarResult, error) {
	return withSidecar(ctx, deployment, func(ops *sidecarOperations) (SidecarResult, error) {
		return ops.enable(ctx, name, noDBPassword)
	})
}

func DisableSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(ctx, deployment, func(ops *sidecarOperations) (SidecarResult, error) {
		return ops.disable(ctx, name)
	})
}

func GetSidecarStatus(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(ctx, deployment, func(ops *sidecarOperations) (SidecarResult, error) {
		return ops.status(ctx, name)
	})
}

func withSidecar(
	ctx context.Context, deployment config.DeploymentDir,
	action func(*sidecarOperations) (SidecarResult, error),
) (SidecarResult, error) {
	var result SidecarResult
	err := withDeploymentExclusiveLock(ctx, deployment, func(dir config.DeploymentDir) error {
		operations, err := newSidecarOperations(ctx, dir)
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
		hosts:        hosts,
		architecture: architecture,
	}, nil
}

func (ops *sidecarOperations) loadCatalog() error {
	if ops.catalog != nil {
		return nil
	}
	var err error
	ops.catalog, err = sidecar.LoadCatalog(resources.SidecarCatalogYAML)

	return err
}

func (ops *sidecarOperations) enable(
	ctx context.Context,
	name string,
	noDBPassword bool,
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
		noDBPassword,
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
	if host.databasePort != "" {
		sources[sidecar.DatabaseSource]["port"] = host.databasePort
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
	changed := false
	for _, host := range ops.hosts {
		observed := SidecarHostResult{
			Name:               host.name,
			State:              "stopped",
			Reconciliation:     sidecarReconciliationUnknown,
			Endpoints:          []SidecarEndpoint{},
			LastOperationError: failures[host.name+"/"+name],
		}
		if err := ops.inspect(ctx, host, name, document, &observed); err != nil {
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
	for _, container := range containers {
		result.Running = container.Running()
		result.State, result.ExitCode = container.State, container.ExitCode
		if result.Running {
			for _, port := range container.Ports {
				if port.HostPort > 0 {
					result.Endpoints = append(
						result.Endpoints,
						SidecarEndpoint{IP: port.HostIP, Port: port.HostPort},
					)
				}
			}
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
		if len(containers) == 0 {
			result.Reconciliation = sidecarReconciliationComplete
		}

		return nil
	}
	result.RestartRequired, err = manager.DatabaseRestartRequired(ctx)
	if err != nil || result.RestartRequired || len(containers) != 1 {
		return err
	}
	matches, err := containers[0].Matches(document.Containers[wanted])
	if matches {
		result.Reconciliation = sidecarReconciliationComplete
	}

	return err
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
