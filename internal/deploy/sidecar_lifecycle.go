// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

// The marker means the database operation succeeded despite a sidecar failure.
type sidecarLifecycleError struct{ failure error }

func (failure *sidecarLifecycleError) Error() string { return failure.failure.Error() }
func (failure *sidecarLifecycleError) Unwrap() error { return failure.failure }

//nolint:nonamedreturns // Names distinguish independent service and database outcomes.
func splitSidecarFailure(err error) (sidecarError, databaseError error) {
	failure := &sidecarLifecycleError{}
	if errors.As(err, &failure) {
		return failure, nil
	}

	return nil, err
}

func sidecarLifecycleResult(databaseError, sidecarError error) error {
	if databaseError != nil {
		return errors.Join(databaseError, sidecarError)
	}
	if sidecarError != nil {
		return &sidecarLifecycleError{failure: sidecarError}
	}

	return nil
}

type sidecarHooks struct {
	deployment config.DeploymentDir
	hosts      func(context.Context) ([]sidecarHost, error)
}

func backendSidecarHooks(deployment config.DeploymentDir, backend deploymentBackend) sidecarHooks {
	return sidecarHooks{
		deployment: deployment,
		hosts:      backend.SidecarHosts,
	}
}

func runtimeSidecarHooks(selected localruntime.Runtime) sidecarHooks {
	return sidecarHooks{
		deployment: selected.Deployment(),
		hosts: func(context.Context) ([]sidecarHost, error) {
			return []sidecarHost{localSidecarHost(selected)}, nil
		},
	}
}

func (hooks sidecarHooks) HostsReady(ctx context.Context) error {
	return hooks.reconcile(ctx, false)
}

func (hooks sidecarHooks) BeforeStop(ctx context.Context) error {
	return hooks.reconcile(ctx, true)
}

func (hooks sidecarHooks) AfterDestroy() error {
	err := os.Remove(hooks.deployment.SidecarStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func (hooks sidecarHooks) reconcile(ctx context.Context, cleanup bool) error {
	if _, err := os.Stat(hooks.deployment.SidecarsPath()); errors.Is(err, os.ErrNotExist) {
		if _, failureErr := os.Stat(
			hooks.deployment.SidecarStatePath(),
		); errors.Is(
			failureErr,
			os.ErrNotExist,
		) {
			return nil
		} else if failureErr != nil {
			return failureErr
		}
	} else if err != nil {
		return err
	}
	hosts, err := hooks.hosts(ctx)
	if err != nil {
		return err
	}
	operations := &sidecarOperations{deployment: hooks.deployment, hosts: hosts}

	return operations.reconcile(ctx, cleanup)
}

func reconcileRuntimeSidecars(ctx context.Context, selected localruntime.Runtime) error {
	return runtimeSidecarHooks(selected).HostsReady(ctx)
}

func (operations *sidecarOperations) reconcile(ctx context.Context, cleanup bool) error {
	document, err := config.ReadSidecars(operations.deployment)
	if err != nil && !cleanup {
		return err
	}
	wanted := document
	if cleanup {
		wanted.Containers = nil
	}
	var failures []error
	for _, host := range operations.hosts {
		if cleanup {
			available, err := operations.hostAvailable(ctx, host)
			if err != nil || !available {
				if err != nil {
					failures = append(
						failures,
						operations.recordDefinitionFailures(host.name, document, err),
					)
				}

				continue
			}
		}
		manager, err := host.provider.Sidecars(ctx)
		if err != nil {
			failures = append(
				failures,
				operations.recordDefinitionFailures(host.name, document, err),
			)

			continue
		}
		if err := pruneRuntimeSidecars(ctx, manager, operations, host.name, wanted); err != nil {
			failures = append(
				failures,
				operations.recordDefinitionFailures(host.name, document, err),
			)

			continue
		}
		for _, definition := range wanted.Containers {
			failure := operations.ensure(ctx, host, definition)
			failures = append(
				failures,
				operations.recordFailure(host.name, definition.Name, failure),
			)
		}
	}

	return errors.Join(failures...)
}

func pruneRuntimeSidecars(
	ctx context.Context, manager localinstall.SidecarManager,
	operations *sidecarOperations, host string, document sidecar.Document,
) error {
	containers, err := manager.List(ctx)
	if err != nil {
		return err
	}
	previous, err := config.ReadSidecarState(operations.deployment)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for key := range previous {
		if name, ok := strings.CutPrefix(key, host+"/"); ok {
			names[name] = true
		}
	}
	for _, container := range containers {
		names[container.Name()] = true
	}
	var failures []error
	for _, name := range slices.Sorted(maps.Keys(names)) {
		if containsSidecar(document, name) {
			continue
		}
		failure := manager.Remove(ctx, name)
		failures = append(failures, operations.recordFailure(host, name, failure))
	}

	return errors.Join(failures...)
}

func (operations *sidecarOperations) recordDefinitionFailures(
	host string,
	document sidecar.Document,
	failure error,
) error {
	failures := make([]error, 0, len(document.Containers))
	for _, definition := range document.Containers {
		failures = append(failures, operations.recordFailure(host, definition.Name, failure))
	}

	if len(failures) == 0 {
		return fmt.Errorf("sidecar host %s: %w", host, failure)
	}

	return errors.Join(failures...)
}

func cleanupRuntimeSidecars(ctx context.Context, selected localruntime.Runtime) error {
	return runtimeSidecarHooks(selected).BeforeStop(ctx)
}

func reconcileRunningSidecars(ctx context.Context, deployment config.DeploymentDir) error {
	backend, err := newDeploymentBackendForDeployment(ctx, deployment)
	if err != nil {
		return err
	}

	return backendSidecarHooks(deployment, backend).HostsReady(ctx)
}

func cleanupStoppedSidecars(ctx context.Context, deployment config.DeploymentDir) error {
	if !isLocalDeployment(deployment) {
		return nil
	}
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}
	workflow, err := state.GetWorkflowState()
	if err != nil {
		return err
	}
	if _, stopped := workflow.(*config.WorkflowStateStopped); !stopped {
		return nil
	}
	operations, err := newSidecarOperations(ctx, deployment)
	if err != nil {
		return err
	}
	if err := operations.reconcile(ctx, true); err != nil {
		return fmt.Errorf("sidecar cleanup: %w", err)
	}

	return nil
}
