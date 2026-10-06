// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

type sidecarRuntimeStub struct {
	endpointRuntimeStub

	manager   *sidecarManagerStub
	running   bool
	available bool
	stopCalls int
}

func (runtime *sidecarRuntimeStub) Sidecars(
	_ context.Context,
) (localinstall.SidecarManager, error) {
	return runtime.manager, nil
}

func (runtime *sidecarRuntimeStub) SidecarHostRunning(context.Context) (bool, error) {
	return runtime.available, nil
}

func (runtime *sidecarRuntimeStub) Status(context.Context) (*localruntime.RuntimeStatus, error) {
	return &localruntime.RuntimeStatus{Running: runtime.running}, nil
}

func (runtime *sidecarRuntimeStub) Stop(context.Context, io.Writer, io.Writer) error {
	runtime.stopCalls++
	runtime.running = false

	return nil
}

type sidecarManagerStub struct {
	definitions map[string]sidecar.Container
	startError  error
	removeError error
	listError   error
	restart     bool
	ensureCalls int
	removeCalls []string
	sources     sidecar.Sources
	observed    []localinstall.SidecarContainer
}

func (manager *sidecarManagerStub) List(context.Context) ([]localinstall.SidecarContainer, error) {
	if manager.listError != nil {
		return nil, manager.listError
	}
	if manager.observed != nil {
		return slices.Clone(manager.observed), nil
	}
	containers := make([]localinstall.SidecarContainer, 0, len(manager.definitions))
	for _, name := range slices.Sorted(maps.Keys(manager.definitions)) {
		fingerprint, err := localinstall.SidecarDefinitionHash(manager.definitions[name])
		if err != nil {
			return nil, err
		}
		container := localinstall.SidecarContainer{
			ID: name, State: "running",
			Labels: map[string]string{
				"com.exasol.launcher.sidecar-name":       name,
				"com.exasol.launcher.sidecar-definition": fingerprint,
			},
		}
		for _, port := range manager.definitions[name].Defaults().Ports {
			container.Ports = append(container.Ports, localinstall.SidecarPort{
				HostIP: port.HostIP, HostPort: port.HostPort, ContainerPort: port.ContainerPort,
				Protocol: strings.ToLower(port.Protocol),
			})
		}
		containers = append(containers, container)
	}

	return containers, nil
}

func (manager *sidecarManagerStub) Ensure(
	_ context.Context, definition sidecar.Container, sources sidecar.Sources,
) error {
	manager.ensureCalls++
	manager.sources = sources
	if manager.startError != nil {
		return manager.startError
	}
	manager.definitions[definition.Name] = definition.Clone()

	return nil
}

func (manager *sidecarManagerStub) Remove(_ context.Context, name string) error {
	manager.removeCalls = append(manager.removeCalls, name)
	if manager.removeError != nil {
		return manager.removeError
	}
	delete(manager.definitions, name)

	return nil
}

func (manager *sidecarManagerStub) AttachDatabase(context.Context) (bool, error) {
	return manager.restart, nil
}

func (manager *sidecarManagerStub) DatabaseRestartRequired(context.Context) (bool, error) {
	return manager.restart, nil
}

func newSidecarOperationsFixture(t *testing.T) (*sidecarOperations, *sidecarRuntimeStub) {
	t.Helper()
	deployment := newTestDeploymentWithState(t)
	selected := &sidecarRuntimeStub{
		endpointRuntimeStub: endpointRuntimeStub{
			deployment: deployment,
			endpoint:   &localruntime.RuntimeEndpoint{DBPort: 28563},
		},
		manager:   &sidecarManagerStub{definitions: map[string]sidecar.Container{}},
		available: true, running: true,
	}
	state, err := config.ReadExasolPersonalState(deployment)
	require.NoError(t, err)
	require.NoError(t, state.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{},
		deployment,
	))

	return &sidecarOperations{
		deployment: deployment, catalog: sidecarTestCatalog(t),
		hosts: []sidecarHost{localSidecarHost(selected)}, architecture: "amd64",
	}, selected
}

func TestSidecarSavedDefinitionSurvivesCatalogChange(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	ctx := context.Background()
	_, err := operations.enable(ctx, "example", false)
	require.NoError(t, err)
	saved := selected.manager.definitions["example"].Clone()
	delete(operations.catalog.Sidecars, "example")
	delete(selected.manager.definitions, "example")
	// When
	require.NoError(t, operations.reconcile(ctx, false))
	status, err := operations.status(ctx, "example")
	// Then
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.True(t, status.Hosts[0].Running)
	require.Equal(t, saved, selected.manager.definitions["example"])
	// When
	status, err = operations.disable(ctx, "example")
	// Then
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Empty(t, selected.manager.definitions)
	require.True(t, selected.running)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	require.Empty(t, document.Containers)
}

func TestSidecarOperationsDeferredIntent(t *testing.T) {
	t.Parallel()
	for _, initialized := range []bool{true, false} {
		t.Run(
			map[bool]string{true: "initialized", false: "stopped"}[initialized],
			func(t *testing.T) {
				t.Parallel()
				// Given
				operations, selected := newSidecarOperationsFixture(t)
				state, err := config.ReadExasolPersonalState(operations.deployment)
				require.NoError(t, err)
				if initialized {
					err = state.SetWorkflowStateAndWrite(
						&config.WorkflowStateInitialized{},
						operations.deployment,
					)
				} else {
					err = state.SetWorkflowStateAndWrite(
						&config.WorkflowStateStopped{},
						operations.deployment,
					)
				}
				require.NoError(t, err)
				selected.running = false
				// When
				result, err := operations.enable(context.Background(), "example", false)
				// Then
				if err != nil || !result.Enabled || result.Hosts[0].Running ||
					selected.manager.ensureCalls != 0 {
					t.Fatalf("deferred enable: %+v, %v", result, err)
				}
				// When
				selected.running = true
				err = reconcileRuntimeSidecars(context.Background(), selected)
				// Then
				if err != nil || selected.manager.ensureCalls != 1 {
					t.Fatalf("deferred reconciliation: %v", err)
				}
			},
		)
	}
}

func TestSidecarOperationsPersistRestartRequirement(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	selected.manager.restart = true
	// When
	result, err := operations.enable(context.Background(), "example", false)
	// Then
	if err != nil || !result.Enabled || !result.Hosts[0].RestartRequired ||
		result.Hosts[0].Running {
		t.Fatalf("pending enable: %+v, %v", result, err)
	}
	if selected.manager.ensureCalls != 0 || selected.stopCalls != 0 {
		t.Fatal("pending enable changed containers")
	}
	// When
	err = reconcileRuntimeSidecars(context.Background(), selected)
	state, readErr := config.ReadSidecarState(operations.deployment)
	// Then
	require.ErrorIs(t, err, ErrSidecarRestartRequired)
	require.NoError(t, readErr)
	require.Contains(t, state["local/example"], ErrSidecarRestartRequired.Error())
	require.NotContains(t, state["local/example"], "exasol stop")
}

func TestSidecarOperationsRecoverFailedDisable(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example", false); err != nil {
		t.Fatal(err)
	}
	selected.manager.removeError = errors.New("container busy")
	// When
	_, first := operations.disable(context.Background(), "example")
	pending, statusErr := operations.status(context.Background(), "example")
	selected.manager.removeError = nil
	result, retry := operations.disable(context.Background(), "example")
	// Then
	if first == nil || statusErr != nil || pending.Enabled || !pending.Hosts[0].Running ||
		pending.Hosts[0].LastOperationError == "" {
		t.Fatalf("interrupted removal: %+v, %v, %v", pending, first, statusErr)
	}
	if retry != nil || result.Enabled || result.Hosts[0].Running ||
		result.Hosts[0].LastOperationError != "" ||
		!selected.running {
		t.Fatalf("removal retry: %+v, %v", result, retry)
	}
}

func TestSidecarReconciliationUsesSavedDefinitions(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example", false); err != nil {
		t.Fatal(err)
	}
	selected.manager.definitions["orphan"] = sidecar.Container{Name: "orphan", Image: "caddy:2"}
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	document.Containers[0].Args = []string{"edited"}
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	err = reconcileRuntimeSidecars(context.Background(), selected)
	// Then
	if err != nil || len(selected.manager.definitions) != 1 ||
		!slices.Equal(selected.manager.definitions["example"].Args, []string{"edited"}) {
		t.Fatalf("reconciliation: %+v, %v", selected.manager.definitions, err)
	}
}

func TestSidecarStartupFailureRetainsDesiredAndDatabaseState(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	selected.manager.startError = errors.New("sidecar example: image unavailable")
	// When
	_, startErr := operations.enable(context.Background(), "example", false)
	result, statusErr := operations.status(context.Background(), "example")
	state, stateErr := config.ReadExasolPersonalState(operations.deployment)
	// Then
	if startErr == nil || statusErr != nil || stateErr != nil || !result.Enabled ||
		result.Hosts[0].Running ||
		!strings.Contains(result.Hosts[0].LastOperationError, "image unavailable") ||
		!selected.running {
		t.Fatalf("failed enable: %+v, %v, %v, %v", result, startErr, statusErr, stateErr)
	}
	workflow, err := state.GetWorkflowState()
	if _, ok := workflow.(*config.WorkflowStateRunning); err != nil || !ok {
		t.Fatalf("database state changed: %T, %v", workflow, err)
	}
}

func TestSidecarCleanupFailureStillStopsDatabase(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	require.NoError(t, writeLocalDeploymentArtifacts(operations.deployment,
		&localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}))
	if _, err := operations.enable(context.Background(), "example", false); err != nil {
		t.Fatal(err)
	}
	selected.manager.definitions["second"] = sidecar.Container{Name: "second", Image: "caddy:2"}
	selected.manager.removeError = errors.New("cleanup failed")
	// When
	err := stopLocalRuntime(context.Background(), selected, nil, nil)
	sidecarErr, databaseErr := splitSidecarFailure(err)
	// Then
	if sidecarErr == nil || databaseErr != nil || selected.running || selected.stopCalls != 1 ||
		len(selected.manager.removeCalls) != 2 {
		t.Fatalf("stop result: sidecars=%v database=%v stopped=%d cleanup=%v",
			sidecarErr, databaseErr, selected.stopCalls, selected.manager.removeCalls)
	}
}

func TestSidecarLifecycleFailureClassification(t *testing.T) {
	t.Parallel()
	// Given
	serviceFailure := errors.New("sidecar failure")
	databaseFailure := errors.New("database failure")
	// When
	serviceOnly, primary := splitSidecarFailure(sidecarLifecycleResult(nil, serviceFailure))
	combinedService, combinedPrimary := splitSidecarFailure(
		sidecarLifecycleResult(databaseFailure, serviceFailure))
	// Then
	if serviceOnly == nil || primary != nil || combinedService != nil ||
		!errors.Is(
			combinedPrimary,
			databaseFailure,
		) || !errors.Is(combinedPrimary, serviceFailure) {
		t.Fatal("sidecar failure masked database outcome")
	}
}

func TestLocalRuntimeArtifactsKeepCurrentCredentials(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	endpoint := &localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}
	require.NoError(t, writeLocalDeploymentArtifacts(operations.deployment, endpoint))
	if err := config.WriteSecrets(operations.deployment.Root(), &config.Secrets{
		DbPassword: "disposable-rotated-password",
	}); err != nil {
		t.Fatal(err)
	}
	// When
	err := writeLocalDeploymentArtifacts(operations.deployment, endpoint)
	sources, readErr := config.SidecarSources(operations.deployment)
	// Then
	if err != nil || readErr != nil ||
		sources[sidecar.DatabaseSource]["password"] != "disposable-rotated-password" {
		t.Fatalf("credential refresh: %v, %v", err, readErr)
	}
}

func TestSidecarReconciliationClearsAbsentFailure(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	require.NoError(t, config.WriteSidecarState(operations.deployment,
		map[string]string{"local/removed": "sidecar removed: image unavailable"}))
	// When
	err := reconcileRuntimeSidecars(context.Background(), selected)
	failures, readErr := config.ReadSidecarState(operations.deployment)
	// Then
	if err != nil || readErr != nil || len(failures) != 0 {
		t.Fatalf("stale failure cleanup: %v, %v, %v", failures, err, readErr)
	}
}

func TestSidecarStatusRefreshesRecordedFailures(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct{ name, reconciliation string }{
		{"recovered", "complete"},
		{"changed definition", "pending"},
		{"missing container", "pending"},
		{"exited container", "pending"},
		{"missing definition label", "pending"},
		{"missing publication", "pending"},
		{"duplicate containers", "pending"},
		{"legacy network", "pending"},
		{"disabled with container", "pending"},
		{"disabled with orphan forward", "pending"},
		{"disabled and removed", "complete"},
		{"database stopped with container", "pending"},
		{"database stopped and removed", "complete"},
		{"host unavailable", "unknown"},
		{"inspection failed", "unknown"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// Given
			ops, runtime := newSidecarOperationsFixture(t)
			entry := ops.catalog.Sidecars["example"]
			entry.Container.Ports = []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}}
			ops.catalog.Sidecars["example"] = entry
			_, err := ops.enable(t.Context(), "example", false)
			require.NoError(t, err)
			runtime.manager.observed, err = runtime.manager.List(t.Context())
			require.NoError(t, err)
			document, err := config.ReadSidecars(ops.deployment)
			require.NoError(t, err)
			switch scenario.name {
			case "changed definition":
				document.Containers[0].Args = []string{"edited"}
			case "missing container":
				runtime.manager.observed = []localinstall.SidecarContainer{}
			case "exited container":
				runtime.manager.observed[0].State = "exited"
			case "missing definition label":
				delete(runtime.manager.observed[0].Labels, "com.exasol.launcher.sidecar-definition")
			case "missing publication":
				runtime.manager.observed[0].Ports = nil
			case "duplicate containers":
				runtime.manager.observed = append(
					runtime.manager.observed,
					runtime.manager.observed[0],
				)
			case "legacy network":
				runtime.manager.restart = true
			case "disabled with container", "disabled with orphan forward", "disabled and removed":
				document.Containers = nil
				switch scenario.name {
				case "disabled with orphan forward":
					runtime.manager.observed[0].ID = ""
					runtime.manager.observed[0].State = "stopped"
				case "disabled and removed":
					runtime.manager.observed = []localinstall.SidecarContainer{}
				default:
					require.Equal(t, "disabled with container", scenario.name)
				}
			case "database stopped with container", "database stopped and removed":
				runtime.running = false
				if scenario.name == "database stopped and removed" {
					runtime.manager.observed = []localinstall.SidecarContainer{}
				}
			case "host unavailable":
				runtime.available = false
			case "inspection failed":
				runtime.manager.listError = errors.New("runtime unreachable")
			default:
				require.Equal(t, "recovered", scenario.name)
			}
			require.NoError(t, config.WriteSidecars(ops.deployment, document))
			saved, err := os.ReadFile(ops.deployment.SidecarsPath())
			require.NoError(t, err)
			require.NoError(t, config.WriteSidecarState(ops.deployment, map[string]string{
				"local/example": "previous failure", "other/example": "other host failure",
			}))
			// When
			result, err := ops.status(t.Context(), "example")
			// Then
			require.NoError(t, err)
			require.Equal(t, scenario.reconciliation, result.Hosts[0].Reconciliation)
			failures, err := config.ReadSidecarState(ops.deployment)
			require.NoError(t, err)
			require.Equal(t, "other host failure", failures["other/example"])
			if scenario.reconciliation == "complete" {
				require.Empty(t, result.Hosts[0].LastOperationError)
				require.NotContains(t, failures, "local/example")
			} else {
				require.Equal(t, "previous failure", result.Hosts[0].LastOperationError)
				require.Equal(t, "previous failure", failures["local/example"])
			}
			if scenario.name == "inspection failed" {
				require.Equal(t, "unknown", result.Hosts[0].State)
				require.Equal(t, "runtime unreachable", result.Hosts[0].Error)
			}
			unchanged, err := os.ReadFile(ops.deployment.SidecarsPath())
			require.NoError(t, err)
			require.Equal(t, saved, unchanged)
			require.Equal(t, 1, runtime.manager.ensureCalls)
			require.Empty(t, runtime.manager.removeCalls)
		})
	}
}

func TestSidecarStatusRequiresExclusiveLock(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	require.NoError(t, err)
	require.NoError(t, mutex.AcquireShared(t.Context()))
	t.Cleanup(func() { _ = mutex.ReleaseShared(context.Background()) })
	// When
	_, err = GetSidecarStatus(t.Context(), deployment, "example")
	// Then
	require.EqualError(t, err, lockUnavailableMessage)
}

func TestSidecarDisableUsesEditedNames(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	definition := sidecar.Container{Name: "user-added", Image: "caddy:2"}
	require.NoError(t, config.WriteSidecars(operations.deployment,
		sidecar.Document{Version: 1, Containers: []sidecar.Container{definition}}))
	selected.manager.definitions[definition.Name] = definition
	operations.catalog = nil
	selected.manager.removeError = errors.New("container busy")
	// When
	result, err := operations.disable(context.Background(), definition.Name)
	// Then
	require.ErrorContains(t, err, "container busy")
	require.False(t, result.Enabled)
	require.True(t, result.Hosts[0].Running)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	require.Empty(t, document.Containers)
	// When
	selected.manager.removeError = nil
	result, err = operations.disable(context.Background(), definition.Name)
	// Then
	require.NoError(t, err)
	require.False(t, result.Enabled)
	require.False(t, result.Hosts[0].Running)
	require.Empty(t, result.Hosts[0].LastOperationError)
	require.True(t, selected.running)
}

func TestSidecarObservedEndpointsFollowRuntime(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example", false); err != nil {
		t.Fatal(err)
	}
	selected.manager.definitions["example"] = sidecar.Container{
		Name: "example", Image: "caddy:2", Ports: []sidecar.Port{{
			ContainerPort: 8080, HostPort: 18080, HostIP: "127.0.0.1",
		}},
	}
	// When
	result, err := operations.status(context.Background(), "example")
	// Then
	if err != nil || !result.Hosts[0].Running || !slices.Equal(result.Hosts[0].Endpoints,
		[]SidecarEndpoint{{IP: "127.0.0.1", Port: 18080}}) {
		t.Fatalf("observed status: %+v, %v", result, err)
	}
}

func TestSidecarStartupRunsBeforeDatabaseWait(t *testing.T) {
	// Given
	t.Setenv(localSkipDatabaseWaitEnv, "1")
	operations, selected := newSidecarOperationsFixture(t)
	materializeTestSidecar(t, operations.deployment, operations.catalog)
	selected.manager.startError = errors.New("sidecar example: image unavailable")
	endpoint := &localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}
	// When
	err := writeLocalRuntimeArtifactsAndWait(context.Background(), selected, endpoint, 0, nil, nil)
	sidecarErr, databaseErr := splitSidecarFailure(err)
	// Then
	if selected.manager.ensureCalls != 1 || sidecarErr == nil || databaseErr != nil {
		t.Fatalf("startup without SQL wait: %d, %v, %v",
			selected.manager.ensureCalls, sidecarErr, databaseErr)
	}
	if selected.manager.sources[sidecar.DatabaseSource]["password"] != localDBPassword {
		t.Fatal("connection sources were unavailable at container startup")
	}
}

func TestSidecarDestroyRemovesObservedState(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example", false); err != nil {
		t.Fatal(err)
	}
	// When
	err := destroyLocalRuntime(context.Background(), selected, nil, nil)
	_, stateErr := os.Stat(operations.deployment.SidecarStatePath())
	document, readErr := config.ReadSidecars(operations.deployment)
	// Then
	if err != nil || len(selected.manager.definitions) != 0 ||
		!errors.Is(stateErr, os.ErrNotExist) {
		t.Fatalf("destroyed sidecar state: %v, %v", err, stateErr)
	}
	if readErr != nil || len(document.Containers) != 1 {
		t.Fatalf("desired configuration after destroy: %+v, %v", document, readErr)
	}
}
