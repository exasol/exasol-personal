// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"maps"
	"runtime"
	"slices"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
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
	Name               string            `json:"name"`
	Enabled            bool              `json:"enabled"`
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
	deployment config.DeploymentDir
	templates  sidecar.Resolver
	backend    sidecarBackend
}

var ErrSidecarRestartRequired = errors.New(
	"enabled sidecars require a deployment restart",
)

func ListSidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]SidecarAvailability, error) {
	// The catalog stays browsable before a deployment exists.
	if _, err := config.ReadInfrastructureManifest(deployment); err != nil {
		return catalogAvailability(sidecar.Document{}, runtime.GOARCH)
	}
	var result []SidecarAvailability
	err := withDeploymentSharedLock(ctx, deployment, func(directory config.DeploymentDir) error {
		document, err := config.ReadSidecars(directory)
		if err != nil {
			return err
		}
		result, err = catalogAvailability(document, runtime.GOARCH)

		return err
	})

	return result, err
}

func catalogAvailability(
	document sidecar.Document,
	architecture string,
) ([]SidecarAvailability, error) {
	catalog, err := sidecarCatalog(nil)
	if err != nil {
		return nil, err
	}
	entries := catalog.Entries()
	result := make([]SidecarAvailability, 0, len(entries))
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		entry := entries[name]
		result = append(result, SidecarAvailability{
			Name:          name,
			Description:   entry.Description,
			Architectures: slices.Clone(entry.Architectures),
			Supported:     slices.Contains(entry.Architectures, architecture),
			Enabled:       containsSidecar(document, name),
		})
	}

	return result, nil
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
	sidecars, supported := backend.(sidecarBackend)
	if !supported {
		return nil, ErrSidecarsUnsupported
	}

	return &sidecarOperations{
		deployment: deployment,
		templates:  templates,
		backend:    sidecars,
	}, nil
}

// sidecarCatalog resolves selections against templates assembled for this
// command before the launcher's embedded entries.
func sidecarCatalog(templates sidecar.Resolver) (sidecar.Resolver, error) {
	embedded, err := sidecar.LoadCatalog(resources.SidecarCatalogYAML)
	if err != nil {
		return nil, err
	}
	if templates == nil {
		return sidecar.Catalogs{embedded}, nil
	}

	return sidecar.Catalogs{templates, embedded}, nil
}

func (ops *sidecarOperations) enable(
	ctx context.Context,
	name string,
) (SidecarResult, error) {
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	if !containsSidecar(document, name) {
		catalog, err := sidecarCatalog(ops.templates)
		if err != nil {
			return SidecarResult{}, err
		}
		container, err := catalog.Resolve(name, runtime.GOARCH)
		if err != nil {
			return SidecarResult{}, err
		}
		document.Containers = append(document.Containers, container)
		if err := config.WriteSidecars(ops.deployment, document); err != nil {
			return SidecarResult{}, err
		}
	}

	return ops.applyAndReport(ctx, name)
}

func (ops *sidecarOperations) disable(ctx context.Context, name string) (SidecarResult, error) {
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

	return ops.applyAndReport(ctx, name)
}

// applyAndReport converges the deployment unless it serves nothing yet, so a
// stopped deployment records intent and the next start applies it.
func (ops *sidecarOperations) applyAndReport(
	ctx context.Context, name string,
) (SidecarResult, error) {
	deferred, err := ops.mutationDeferred()
	if err != nil {
		return SidecarResult{}, err
	}
	var failures []error
	if !deferred {
		document, readErr := config.ReadSidecars(ops.deployment)
		failures = append(failures, readErr)
		if readErr == nil {
			failures = append(failures, ops.apply(ctx, document.Containers))
		}
	}
	result, err := ops.status(ctx, name)

	return result, errors.Join(append(failures, err)...)
}

// apply records each sidecar's own failure so status reports it against that
// sidecar after the command returns.
func (ops *sidecarOperations) apply(ctx context.Context, desired []sidecar.Container) error {
	result, applyErr := ops.backend.ApplySidecars(ctx, desired)
	if errors.Is(applyErr, ErrSidecarRestartRequired) {
		applyErr = nil
	}
	if !result.Applied {
		return applyErr
	}

	return errors.Join(applyErr, ops.recordFailures(result.Failures))
}

func (ops *sidecarOperations) status(ctx context.Context, name string) (SidecarResult, error) {
	result := SidecarResult{
		Name:           name,
		State:          "stopped",
		Reconciliation: sidecarReconciliationUnknown,
		Endpoints:      []SidecarEndpoint{},
	}
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return result, err
	}
	result.Enabled = containsSidecar(document, name)
	failures, err := config.ReadSidecarState(ops.deployment)
	if err != nil {
		return result, err
	}
	result.LastOperationError = failures[name]
	if err := ops.describe(ctx, name, document, &result); err != nil {
		result.State = slcApplyOutcomeUnknown
		result.Reconciliation = sidecarReconciliationUnknown
		result.Error = err.Error()
	}
	if result.Reconciliation != sidecarReconciliationComplete ||
		result.LastOperationError == "" {
		return result, nil
	}
	delete(failures, name)
	result.LastOperationError = ""

	return result, config.WriteSidecarState(ops.deployment, failures)
}

func (ops *sidecarOperations) describe(
	ctx context.Context, name string, document sidecar.Document, result *SidecarResult,
) error {
	initialized, err := ops.deploymentInitialized()
	if err != nil || initialized {
		return err
	}
	inspection, err := ops.backend.InspectSidecars(ctx, document.Containers)
	if err != nil || !inspection.Available {
		return err
	}
	observed := observationsFor(inspection.Sidecars, name)
	for _, observation := range observed {
		result.Running = observation.Running
		result.State, result.ExitCode = observation.State, observation.ExitCode
		if observation.Running {
			result.Endpoints = observation.Endpoints
		}
	}
	result.RestartRequired = inspection.RestartRequired
	result.Reconciliation = reconciliation(
		name, document, observed, result, inspection.DatabaseRunning,
	)

	return nil
}

// reconciliation compares what the deployment serves against what the saved
// definitions ask for. A sidecar the deployment should not serve is reconciled
// once nothing of it remains.
func reconciliation(
	name string,
	document sidecar.Document,
	observed []SidecarObservation,
	result *SidecarResult,
	databaseRunning bool,
) string {
	wanted := slices.IndexFunc(
		document.Containers,
		func(container sidecar.Container) bool { return container.Name == name },
	)
	if wanted < 0 || !databaseRunning {
		if len(observed) == 0 && len(result.Endpoints) == 0 {
			return sidecarReconciliationComplete
		}

		return sidecarReconciliationPending
	}
	if result.RestartRequired || len(observed) != 1 {
		return sidecarReconciliationPending
	}
	if !observed[0].Running || !observed[0].Matches {
		return sidecarReconciliationPending
	}
	if !endpointsMatch(document.Containers[wanted], result.Endpoints) {
		return sidecarReconciliationPending
	}

	return sidecarReconciliationComplete
}

// endpointsMatch reports whether the deployment serves exactly the endpoints
// the definition declares, counting repeats on both sides.
func endpointsMatch(definition sidecar.Container, endpoints []SidecarEndpoint) bool {
	declared := map[SidecarEndpoint]int{}
	for _, port := range definition.Defaults().Ports {
		if port.HostPort > 0 {
			declared[SidecarEndpoint{IP: port.HostIP, Port: port.HostPort}]++
		}
	}
	for _, endpoint := range endpoints {
		declared[endpoint]--
	}
	for _, count := range declared {
		if count != 0 {
			return false
		}
	}

	return true
}

func observationsFor(observations []SidecarObservation, name string) []SidecarObservation {
	matching := make([]SidecarObservation, 0, len(observations))
	for _, observation := range observations {
		if observation.Name == name {
			matching = append(matching, observation)
		}
	}

	return matching
}

func (ops *sidecarOperations) workflowState() (any, error) {
	state, err := config.ReadExasolPersonalState(ops.deployment)
	if err != nil {
		return nil, err
	}

	return state.GetWorkflowState()
}

func (ops *sidecarOperations) deploymentInitialized() (bool, error) {
	workflow, err := ops.workflowState()
	if err != nil {
		return false, err
	}
	_, initialized := workflow.(*config.WorkflowStateInitialized)

	return initialized, nil
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
		return false, errors.New("deployment state does not allow sidecar changes")
	}
}

// recordFailures replaces the recorded set, so a sidecar that converged or was
// removed stops carrying a diagnostic from an earlier attempt.
func (ops *sidecarOperations) recordFailures(perSidecar map[string]error) error {
	failures := make(map[string]string, len(perSidecar))
	reported := make([]error, 0, len(perSidecar))
	for _, name := range slices.Sorted(maps.Keys(perSidecar)) {
		failures[name] = sidecarDiagnostic(perSidecar[name])
		reported = append(reported, perSidecar[name])
	}

	return errors.Join(
		errors.Join(reported...), config.WriteSidecarState(ops.deployment, failures),
	)
}

func containsSidecar(document sidecar.Document, name string) bool {
	return slices.ContainsFunc(
		document.Containers,
		func(container sidecar.Container) bool { return container.Name == name },
	)
}

// sidecarDiagnostic keeps the full account of a failed start for status, while
// the command that triggered it prints only the short cause.
func sidecarDiagnostic(failure error) string {
	if startup, ok := errors.AsType[*sidecar.StartupError](failure); ok {
		return startup.Diagnostic
	}

	return failure.Error()
}
