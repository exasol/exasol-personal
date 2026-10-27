// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

type SidecarEndpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
	// Empty unless the port declares an application protocol.
	URL string `json:"url,omitempty"`
}

const (
	sidecarReconciliationComplete = "complete"
	sidecarReconciliationPending  = "pending"
	sidecarReconciliationUnknown  = "unknown"
)

type SidecarResult struct {
	Name               string            `json:"name"`
	Enabled            bool              `json:"enabled"`
	NothingToRemove    bool              `json:"nothingToRemove,omitempty"`
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
	// Unsupported says why, so a listing explains itself.
	Unsupported string `json:"unsupported,omitempty"`
}

type deploymentSidecars struct {
	deployment config.DeploymentDir
	templates  sidecar.Resolver
	runtime    sidecarRuntime
}

var ErrSidecarRestartRequired = errors.New(
	"enabled sidecars require a deployment restart",
)

func ListSidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]SidecarAvailability, error) {
	// The catalog stays browsable before a deployment exists.
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if errors.Is(err, config.ErrMissingInfrastructureManifest) {
		return catalogAvailability(sidecar.Document{}, runtime.GOARCH)
	}
	if err != nil {
		return nil, err
	}
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}
	if kind != backendTypeLocal {
		return unsupportedAvailability(kind, nil)
	}
	var result []SidecarAvailability
	err = withDeploymentSharedLock(ctx, deployment, func(directory config.DeploymentDir) error {
		document, err := config.ReadSidecars(directory)
		if err != nil {
			return err
		}
		result, err = catalogAvailability(document, runtime.GOARCH)

		return err
	})

	return result, err
}

// unsupportedAvailability reports the catalog against a deployment that
// cannot run sidecars at all.
func unsupportedAvailability(
	kind string, templates sidecar.Resolver,
) ([]SidecarAvailability, error) {
	catalog, err := sidecarCatalog(templates)
	if err != nil {
		return nil, err
	}
	entries := catalog.Entries()
	result := make([]SidecarAvailability, 0, len(entries))
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		result = append(result, SidecarAvailability{
			Name:          name,
			Description:   entries[name].Description,
			Architectures: slices.Clone(entries[name].Architectures),
			Unsupported:   "not supported for " + kind + " deployments",
		})
	}

	return result, nil
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
			Enabled:       enabledSidecar(document, name),
		})
	}

	return result, nil
}

func EnableSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return enableSidecarTemplate(ctx, deployment, nil, name)
}

func enableSidecarTemplate(
	ctx context.Context, deployment config.DeploymentDir, templates sidecar.Resolver,
	name string,
) (SidecarResult, error) {
	// Templates first, so a command can enable what the catalog does not carry.
	return withSidecar(
		ctx,
		deployment,
		templates,
		func(sidecars *deploymentSidecars) (SidecarResult, error) {
			return sidecars.enable(ctx, name)
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
		func(sidecars *deploymentSidecars) (SidecarResult, error) {
			return sidecars.disable(ctx, name, false)
		},
	)
}

// RemoveSidecar disables a sidecar and deletes its saved entry, so a later
// enable starts again from the catalog template.
func RemoveSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		nil,
		func(sidecars *deploymentSidecars) (SidecarResult, error) {
			return sidecars.disable(ctx, name, true)
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
		func(sidecars *deploymentSidecars) (SidecarResult, error) {
			// Only a caller-supplied name can be a typo.
			if err := sidecars.knownName(name); err != nil {
				return SidecarResult{}, err
			}

			return sidecars.status(ctx, name)
		},
	)
}

// GetAllSidecarStatus reports the saved sidecars in name order.
func GetAllSidecarStatus(
	ctx context.Context, deployment config.DeploymentDir,
) ([]SidecarResult, error) {
	var results []SidecarResult
	err := withDeploymentExclusiveLock(ctx, deployment, func(dir config.DeploymentDir) error {
		operations, err := newDeploymentSidecars(ctx, dir, nil)
		if err != nil {
			return err
		}
		document, err := config.ReadSidecars(dir)
		if err != nil {
			return err
		}
		names := document.Names()
		results = make([]SidecarResult, 0, len(names))
		for _, name := range names {
			result, err := operations.status(ctx, name)
			if err != nil {
				return err
			}
			results = append(results, result)
		}

		return nil
	})

	return results, err
}

func withSidecar(
	ctx context.Context, deployment config.DeploymentDir, templates sidecar.Resolver,
	action func(*deploymentSidecars) (SidecarResult, error),
) (SidecarResult, error) {
	var result SidecarResult
	err := withDeploymentExclusiveLock(ctx, deployment, func(dir config.DeploymentDir) error {
		operations, err := newDeploymentSidecars(ctx, dir, templates)
		if err != nil {
			return err
		}
		result, err = action(operations)

		return err
	})

	return result, err
}

func newDeploymentSidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
	templates sidecar.Resolver,
) (*deploymentSidecars, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, err
	}
	kind, err := resolveBackendKind(manifest)
	if err != nil {
		return nil, err
	}
	if kind != backendTypeLocal {
		return nil, ErrSidecarsUnsupported
	}
	selected, err := newLocalRuntime(deployment, runtimeartifacts.FromContext(ctx))
	if err != nil {
		return nil, err
	}

	return &deploymentSidecars{
		deployment: deployment,
		templates:  templates,
		runtime:    selected,
	}, nil
}

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

func (sidecars *deploymentSidecars) enable(
	ctx context.Context,
	name string,
) (SidecarResult, error) {
	if _, err := sidecars.mutationDeferred(); err != nil {
		return SidecarResult{}, err
	}
	document, err := config.ReadSidecars(sidecars.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	definition, saved := document.Sidecars[name]
	if !saved {
		catalog, err := sidecarCatalog(sidecars.templates)
		if err != nil {
			return SidecarResult{}, err
		}
		entry, err := catalog.Resolve(name, runtime.GOARCH)
		if err != nil {
			return SidecarResult{}, err
		}
		definition = entry.Definition
	}
	if definition.Enabled == nil || !*definition.Enabled {
		definition.Enabled = new(true)
		document.Sidecars[name] = definition
		if err := config.WriteSidecars(sidecars.deployment, document); err != nil {
			return SidecarResult{}, err
		}
	}

	return sidecars.applyAndReport(ctx, name)
}

func (sidecars *deploymentSidecars) disable(
	ctx context.Context, name string, remove bool,
) (SidecarResult, error) {
	deferred, err := sidecars.mutationDeferred()
	if err != nil {
		return SidecarResult{}, err
	}
	document, err := config.ReadSidecars(sidecars.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	definition, saved := document.Sidecars[name]
	if !saved || (!remove && !definition.IsEnabled()) {
		failures, err := config.ReadSidecarState(sidecars.deployment)
		if err != nil {
			return SidecarResult{}, err
		}
		if _, pending := failures[name]; !pending {
			return SidecarResult{
				Name: name, State: "stopped", Reconciliation: sidecarReconciliationComplete,
				Endpoints: []SidecarEndpoint{}, NothingToRemove: true,
			}, nil
		}
	}
	if remove {
		delete(document.Sidecars, name)
	} else if saved {
		definition.Enabled = new(false)
		document.Sidecars[name] = definition
	}
	if err := config.WriteSidecars(sidecars.deployment, document); err != nil {
		return SidecarResult{}, err
	}

	result, err := sidecars.applyAndReport(ctx, name)
	if deferred || (err == nil && result.Reconciliation == sidecarReconciliationComplete) {
		return result, err
	}
	if err == nil {
		err = fmt.Errorf("sidecar %q cleanup could not be verified", name)
	}
	failures, readErr := config.ReadSidecarState(sidecars.deployment)
	if readErr != nil {
		return result, errors.Join(err, readErr)
	}
	failures[name] = sidecarDiagnostic(err)
	result.LastOperationError = failures[name]

	return result, errors.Join(err, config.WriteSidecarState(sidecars.deployment, failures))
}

// A stopped deployment records intent, and the next start applies it.
func (sidecars *deploymentSidecars) applyAndReport(
	ctx context.Context, name string,
) (SidecarResult, error) {
	deferred, err := sidecars.mutationDeferred()
	if err != nil {
		return SidecarResult{}, err
	}
	var failures []error
	if !deferred {
		document, readErr := config.ReadSidecars(sidecars.deployment)
		failures = append(failures, readErr)
		if readErr == nil {
			failures = append(failures, sidecars.apply(ctx, document.Containers()))
		}
	}
	result, err := sidecars.status(ctx, name)

	return result, errors.Join(append(failures, err)...)
}

func (sidecars *deploymentSidecars) apply(ctx context.Context, desired []sidecar.Container) error {
	result, applyErr := sidecars.applySidecars(ctx, desired)
	if errors.Is(applyErr, ErrSidecarRestartRequired) {
		applyErr = nil
	}
	if !result.Applied {
		return applyErr
	}

	return errors.Join(
		applyErr,
		sidecars.recordFailures(result.Failures),
		joinSidecarFailures(result.Failures),
	)
}

// knownName rejects a name the deployment has not saved and the catalog does
// not offer, so a typo does not read as a disabled sidecar.
func (sidecars *deploymentSidecars) knownName(name string) error {
	document, err := config.ReadSidecars(sidecars.deployment)
	if err != nil {
		return err
	}
	if containsSidecar(document, name) {
		return nil
	}
	failures, err := config.ReadSidecarState(sidecars.deployment)
	if err != nil {
		return err
	}
	if _, pending := failures[name]; pending {
		return nil
	}
	catalog, err := sidecarCatalog(sidecars.templates)
	if err != nil {
		return err
	}
	if _, known := catalog.Entries()[name]; known {
		return nil
	}

	return fmt.Errorf(
		"sidecar %q is not enabled and not in the catalog; supported choices: %s",
		name, strings.Join(slices.Sorted(maps.Keys(catalog.Entries())), ", "),
	)
}

func (sidecars *deploymentSidecars) status(
	ctx context.Context, name string,
) (SidecarResult, error) {
	result := SidecarResult{
		Name:           name,
		State:          "stopped",
		Reconciliation: sidecarReconciliationUnknown,
		Endpoints:      []SidecarEndpoint{},
	}
	document, err := config.ReadSidecars(sidecars.deployment)
	if err != nil {
		return result, err
	}
	result.Enabled = enabledSidecar(document, name)
	failures, err := config.ReadSidecarState(sidecars.deployment)
	if err != nil {
		return result, err
	}
	result.LastOperationError = failures[name]
	if err := sidecars.describe(ctx, name, document, &result); err != nil {
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

	return result, config.WriteSidecarState(sidecars.deployment, failures)
}

func (sidecars *deploymentSidecars) describe(
	ctx context.Context, name string, document sidecar.Document, result *SidecarResult,
) error {
	initialized, err := sidecars.deploymentInitialized()
	if err != nil || initialized {
		return err
	}
	inspection, err := sidecars.inspectSidecars(ctx, document)
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

func reconciliation(
	name string,
	document sidecar.Document,
	observed []SidecarObservation,
	result *SidecarResult,
	databaseRunning bool,
) string {
	// Reconciled once nothing of an unwanted sidecar remains.
	definition := document.Sidecars[name]
	if !enabledSidecar(document, name) || !databaseRunning {
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
	if !endpointsMatch(definition.Container, result.Endpoints) {
		return sidecarReconciliationPending
	}

	return sidecarReconciliationComplete
}

func endpointsMatch(definition sidecar.Container, endpoints []SidecarEndpoint) bool {
	declared := map[string]int{}
	for _, port := range definition.Defaults().Ports {
		if port.HostPort > 0 {
			declared[hostAddress(port.HostIP, port.HostPort)]++
		}
	}
	for _, endpoint := range endpoints {
		declared[hostAddress(endpoint.IP, endpoint.Port)]--
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

func (sidecars *deploymentSidecars) workflowState() (any, error) {
	state, err := config.ReadExasolPersonalState(sidecars.deployment)
	if err != nil {
		return nil, err
	}

	return state.GetWorkflowState()
}

func (sidecars *deploymentSidecars) deploymentInitialized() (bool, error) {
	workflow, err := sidecars.workflowState()
	if err != nil {
		return false, err
	}
	_, initialized := workflow.(*config.WorkflowStateInitialized)

	return initialized, nil
}

func (sidecars *deploymentSidecars) mutationDeferred() (bool, error) {
	workflow, err := sidecars.workflowState()
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

// Replaces the set, so a converged sidecar stops carrying an old diagnostic.
func (sidecars *deploymentSidecars) recordFailures(perSidecar map[string]error) error {
	failures := make(map[string]string, len(perSidecar))
	for name, failure := range perSidecar {
		failures[name] = sidecarDiagnostic(failure)
	}

	return config.WriteSidecarState(sidecars.deployment, failures)
}

func containsSidecar(document sidecar.Document, name string) bool {
	_, saved := document.Sidecars[name]

	return saved
}

func enabledSidecar(document sidecar.Document, name string) bool {
	definition, saved := document.Sidecars[name]

	return saved && definition.IsEnabled()
}

// Status keeps the full account; the command prints only the short cause.
func sidecarDiagnostic(failure error) string {
	if startup, ok := errors.AsType[*sidecar.StartupError](failure); ok {
		return startup.Diagnostic
	}

	return failure.Error()
}

// SidecarObservation reports one container; a repeated name surfaces twice.
type SidecarObservation struct {
	Name      string
	Running   bool
	State     string
	ExitCode  int
	Matches   bool
	Endpoints []SidecarEndpoint
}

type SidecarInspection struct {
	// An unavailable host leaves reconciliation undecided, not empty.
	Available       bool
	DatabaseRunning bool
	RestartRequired bool
	Sidecars        []SidecarObservation
}

type SidecarApplyResult struct {
	// False leaves recorded failures alone rather than clearing them unseen.
	Applied  bool
	Failures map[string]error
}

var ErrSidecarsUnsupported = errors.New(
	"sidecars are not supported for this deployment type",
)

type sidecarRuntime interface {
	localinstall.SidecarProvider
	localruntime.PortPublisher
	Status(ctx context.Context) (*localruntime.RuntimeStatus, error)
}

func (sidecars *deploymentSidecars) applySidecars(
	ctx context.Context, desired []sidecar.Container,
) (SidecarApplyResult, error) {
	running, err := sidecars.runtime.SidecarHostRunning(ctx)
	if err != nil || !running {
		return SidecarApplyResult{}, err
	}
	manager, err := sidecars.runtime.Sidecars(ctx)
	if err != nil {
		return SidecarApplyResult{}, err
	}
	// Only a deployment that will serve sidecars needs the database reachable.
	if len(desired) > 0 {
		restart, err := manager.AttachDatabase(ctx)
		if err != nil {
			return SidecarApplyResult{}, err
		}
		if restart {
			return SidecarApplyResult{}, ErrSidecarRestartRequired
		}
	}
	secrets, err := sidecarSecrets(sidecars.deployment)
	if err != nil {
		return SidecarApplyResult{}, err
	}
	result := SidecarApplyResult{Applied: true, Failures: map[string]error{}}
	for _, definition := range desired {
		if err := manager.Ensure(ctx, definition, secrets); err != nil {
			result.Failures[definition.Name] = err
		}
	}
	if err := removeUndesired(ctx, manager, desired, result.Failures); err != nil {
		return result, err
	}

	return result, sidecars.publish(ctx, manager, desired)
}

func (sidecars *deploymentSidecars) inspectSidecars(
	ctx context.Context, document sidecar.Document,
) (SidecarInspection, error) {
	available, err := sidecars.runtime.SidecarHostRunning(ctx)
	if err != nil || !available {
		return SidecarInspection{}, err
	}
	manager, err := sidecars.runtime.Sidecars(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	containers, err := manager.List(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	opened, err := sidecars.runtime.OpenedPorts(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	inspection := SidecarInspection{Available: true, Sidecars: []SidecarObservation{}}
	inspection.RestartRequired, err = manager.DatabaseRestartRequired(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	status, err := sidecars.runtime.Status(ctx)
	if err != nil {
		return SidecarInspection{}, err
	}
	inspection.DatabaseRunning = status.Running
	secrets, err := sidecarSecrets(sidecars.deployment)
	if err != nil {
		return SidecarInspection{}, err
	}
	for _, container := range containers {
		observation, err := observeSidecar(manager, container, document, opened, secrets)
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
	document sidecar.Document,
	opened []sidecar.PublishedPort,
	secrets sidecar.Secrets,
) (SidecarObservation, error) {
	name := container.SidecarName()
	observation := SidecarObservation{
		Name:      name,
		Running:   container.Running(),
		State:     container.State,
		ExitCode:  container.ExitCode,
		Endpoints: []SidecarEndpoint{},
	}
	definition, saved := document.Sidecars[name]
	if observation.Running {
		observation.Endpoints = sidecarEndpoints(
			container, openedFor(opened, name), declarationsFor(definition),
		)
	}
	if !saved {
		return observation, nil
	}
	matches, err := manager.Matches(container, definition.Container, secrets)
	observation.Matches = matches

	return observation, err
}

func removeUndesired(
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

func (sidecars *deploymentSidecars) publish(
	ctx context.Context, manager *localinstall.SidecarRuntime, desired []sidecar.Container,
) error {
	// A runtime whose containers bind on the user's machine opens nothing.
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

	return sidecars.runtime.OpenPorts(ctx, ports)
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

func hostAddress(hostIP string, hostPort int) string {
	return net.JoinHostPort(hostIP, strconv.Itoa(hostPort))
}

// A direct binding and a forwarded one both report the host address. A port
// the sidecar keeps internal has none.
type endpointDeclarations map[string]sidecar.Endpoint

// Defaults supply the host IP the observed binding reports.
func declarationsFor(definition sidecar.Definition) endpointDeclarations {
	declarations := make(endpointDeclarations, len(definition.Endpoints))
	container := definition.Container.Defaults()
	for _, endpoint := range definition.Endpoints {
		for _, port := range container.Ports {
			if port.Name == endpoint.Port && port.HostPort > 0 {
				declarations[hostAddress(port.HostIP, port.HostPort)] = endpoint
			}
		}
	}

	return declarations
}

// A virtual machine forwards a declared host port to one it assigns itself, so
// the observed address is the reachable one.
func (declarations endpointDeclarations) endpoint(hostIP string, hostPort int) SidecarEndpoint {
	address := hostAddress(hostIP, hostPort)
	endpoint := SidecarEndpoint{IP: hostIP, Port: hostPort}
	declaration, declared := declarations[address]
	if !declared {
		return endpoint
	}
	endpoint.URL = (&url.URL{
		Scheme: declaration.Scheme,
		Host:   address,
		Path:   declaration.Path,
	}).String()

	return endpoint
}

func sidecarEndpoints(
	container localinstall.ContainerListing,
	published []sidecar.PublishedPort,
	declarations endpointDeclarations,
) []SidecarEndpoint {
	// Opened endpoints replace the bindings that stay behind the boundary.
	endpoints := []SidecarEndpoint{}
	if len(published) > 0 {
		for _, port := range published {
			endpoints = append(endpoints, declarations.endpoint(port.HostIP, port.HostPort))
		}

		return endpoints
	}
	for _, port := range container.Ports {
		if port.HostPort > 0 {
			endpoints = append(endpoints, declarations.endpoint(port.HostIP, port.HostPort))
		}
	}

	return endpoints
}

func removeSidecarState(deployment config.DeploymentDir) error {
	err := os.Remove(deployment.SidecarStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func runtimeSidecars(selected localruntime.Runtime) *deploymentSidecars {
	return &deploymentSidecars{deployment: selected.Deployment(), runtime: selected}
}

func reconcileRuntimeSidecars(
	ctx context.Context, selected localruntime.Runtime,
) (map[string]error, error) {
	return runtimeSidecars(selected).reconcile(ctx, false)
}

func cleanupRuntimeSidecars(
	ctx context.Context, selected localruntime.Runtime,
) (map[string]error, error) {
	return runtimeSidecars(selected).reconcile(ctx, true)
}

func reconcileRunningSidecars(ctx context.Context, deployment config.DeploymentDir) error {
	if !isLocalDeployment(deployment) {
		return nil
	}
	selected, err := newLocalRuntime(deployment, runtimeartifacts.FromContext(ctx))
	if err != nil {
		return err
	}
	failures, err := runtimeSidecars(selected).reconcile(ctx, false)

	return errors.Join(err, joinSidecarFailures(failures))
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
	operations, err := newDeploymentSidecars(ctx, deployment, nil)
	if err != nil {
		return err
	}
	failures, err := operations.reconcile(ctx, true)
	if err := errors.Join(err, joinSidecarFailures(failures)); err != nil {
		return fmt.Errorf("sidecar cleanup: %w", err)
	}

	return nil
}

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

func (sidecars *deploymentSidecars) reconcile(
	ctx context.Context, cleanup bool,
) (map[string]error, error) {
	untouched, err := sidecarsUntouched(sidecars.deployment)
	if err != nil || untouched {
		return nil, err
	}
	document, err := config.ReadSidecars(sidecars.deployment)
	if err != nil && !cleanup {
		return nil, err
	}
	desired := document.Containers()
	if cleanup {
		desired = nil
	}
	result, applyErr := sidecars.applySidecars(ctx, desired)
	if applyErr != nil {
		return sidecars.recordDefinitionFailures(document, applyErr)
	}
	if !result.Applied {
		return map[string]error{}, nil
	}

	return result.Failures, sidecars.recordFailures(result.Failures)
}

// Status reports a cause against each affected sidecar.
func (sidecars *deploymentSidecars) recordDefinitionFailures(
	document sidecar.Document,
	failure error,
) (map[string]error, error) {
	desired := document.Containers()
	if len(desired) == 0 {
		return nil, failure
	}
	perSidecar := make(map[string]error, len(desired))
	for _, container := range desired {
		perSidecar[container.Name] = failure
	}

	return perSidecar, sidecars.recordFailures(perSidecar)
}

// A value the deployment cannot supply yet is omitted, so an optional
// reference to it drops instead of resolving to nothing.
func sidecarSecrets(deployment config.DeploymentDir) (sidecar.Secrets, error) {
	database := sidecar.Secret{
		"host": {Value: sidecar.DatabaseAlias},
		"port": {Value: strconv.Itoa(sidecar.DatabasePort)},
	}
	if _, err := os.Stat(deployment.NodeDetailsPath()); err == nil {
		info, err := config.ReadDeploymentInfo(deployment)
		if err != nil {
			return nil, err
		}
		if info.Connection != nil {
			database["username"] = sidecar.SecretValue{
				Value: cmp.Or(info.Connection.Username, "sys"),
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, err := os.Stat(deployment.SecretsPath()); err == nil {
		secrets, err := config.ReadSecrets(deployment)
		if err != nil {
			return nil, err
		}
		database["password"] = sidecar.SecretValue{
			Value: secrets.DbPassword, Sensitive: true,
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return sidecar.Secrets{sidecar.DatabaseSecret: database}, nil
}
