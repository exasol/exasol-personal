// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
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

func removeSidecarState(deployment config.DeploymentDir) error {
	err := os.Remove(deployment.SidecarStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

// runtimeSidecarBackend serves the runtime the caller already prepared, rather
// than resolving a second one part way through a lifecycle operation.
func runtimeSidecarBackend(selected localruntime.Runtime) *localSidecars {
	return &localSidecars{deployment: selected.Deployment(), runtime: selected}
}

func reconcileRuntimeSidecars(ctx context.Context, selected localruntime.Runtime) error {
	return applySidecars(ctx, selected.Deployment(), runtimeSidecarBackend(selected), false)
}

func cleanupRuntimeSidecars(ctx context.Context, selected localruntime.Runtime) error {
	return applySidecars(ctx, selected.Deployment(), runtimeSidecarBackend(selected), true)
}

func reconcileRunningSidecars(ctx context.Context, deployment config.DeploymentDir) error {
	backend, err := newDeploymentBackendForDeployment(ctx, deployment)
	if err != nil {
		return err
	}
	sidecars, supported := backend.(sidecarBackend)
	if !supported {
		return nil
	}

	return applySidecars(ctx, deployment, sidecars, false)
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
	untouched, err := sidecarsUntouched(deployment)
	if err != nil || untouched {
		return err
	}
	operations, err := newSidecarOperations(ctx, deployment, nil)
	if err != nil {
		return err
	}
	if err := operations.reconcile(ctx, true); err != nil {
		return fmt.Errorf("sidecar cleanup: %w", err)
	}

	return nil
}

func applySidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
	backend sidecarBackend,
	cleanup bool,
) error {
	untouched, err := sidecarsUntouched(deployment)
	if err != nil || untouched {
		return err
	}
	operations := &sidecarOperations{deployment: deployment, backend: backend}

	return operations.reconcile(ctx, cleanup)
}

// sidecarsUntouched reports a deployment that has never saved a definition or
// recorded a failure, which has nothing to converge or clean up.
func sidecarsUntouched(deployment config.DeploymentDir) (bool, error) {
	for _, path := range []string{
		deployment.SidecarsPath(), deployment.SidecarStatePath(),
	} {
		switch _, err := os.Stat(path); {
		case err == nil:
			return false, nil
		case !errors.Is(err, os.ErrNotExist):
			return false, err
		}
	}

	return true, nil
}

// reconcile converges the saved definitions, or removes everything the
// deployment serves when the database is going away.
func (ops *sidecarOperations) reconcile(ctx context.Context, cleanup bool) error {
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil && !cleanup {
		return err
	}
	desired := document.Containers
	if cleanup {
		desired = nil
	}
	result, applyErr := ops.backend.ApplySidecars(ctx, desired)
	if applyErr != nil {
		return ops.recordDefinitionFailures(document, applyErr)
	}
	if !result.Applied {
		return nil
	}

	return ops.recordFailures(result.Failures)
}

// recordDefinitionFailures attributes one deployment-wide failure to every
// saved definition, so status reports a cause against each affected sidecar.
func (ops *sidecarOperations) recordDefinitionFailures(
	document sidecar.Document,
	failure error,
) error {
	if len(document.Containers) == 0 {
		return failure
	}
	perSidecar := make(map[string]error, len(document.Containers))
	for _, definition := range document.Containers {
		perSidecar[definition.Name] = failure
	}

	return ops.recordFailures(perSidecar)
}
