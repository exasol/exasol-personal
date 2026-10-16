// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
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
	definitions  map[string]sidecar.Container
	startError   error
	removeError  error
	listError    error
	restart      bool
	ensureCalls  int
	removeCalls  []string
	sources      sidecar.Sources
	matchSources sidecar.Sources
	observed     []localinstall.ContainerListing
}

func (manager *sidecarManagerStub) List(context.Context) ([]localinstall.ContainerListing, error) {
	if manager.listError != nil {
		return nil, manager.listError
	}
	if manager.observed != nil {
		return slices.Clone(manager.observed), nil
	}
	containers := make([]localinstall.ContainerListing, 0, len(manager.definitions))
	for _, name := range slices.Sorted(maps.Keys(manager.definitions)) {
		container := localinstall.ContainerListing{
			ID: name, State: "running",
			Labels: map[string]string{
				"com.exasol.launcher.sidecar-name":       name,
				"com.exasol.launcher.sidecar-definition": stubApplied(manager.definitions[name]),
			},
		}
		for _, port := range manager.definitions[name].Defaults().Ports {
			container.Ports = append(container.Ports, localinstall.ContainerPort{
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

// Stands in for the runtime's keyed fingerprint.
func stubApplied(definition sidecar.Container) string {
	return fmt.Sprintf("%+v", definition.Defaults())
}

func (manager *sidecarManagerStub) Matches(
	container localinstall.ContainerListing,
	definition sidecar.Container,
	sources sidecar.Sources,
) (bool, error) {
	manager.matchSources = sources
	if !container.Running() || container.SidecarName() != definition.Name {
		return false, nil
	}

	return container.Labels["com.exasol.launcher.sidecar-definition"] ==
		stubApplied(definition), nil
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
		deployment: deployment,
		endpoint:   &localruntime.RuntimeEndpoint{DBPort: 28563},
		manager:    &sidecarManagerStub{definitions: map[string]sidecar.Container{}},
		available:  true, running: true,
	}
	state, err := config.ReadExasolPersonalState(deployment)
	require.NoError(t, err)
	require.NoError(t, state.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{},
		deployment,
	))

	return &sidecarOperations{
		deployment: deployment, templates: sidecarTestCatalog(t),
		backend: &localSidecars{deployment: deployment, runtime: selected},
	}, selected
}

func TestSidecarDisableDefersPublicationWhileStopped(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	state, err := config.ReadExasolPersonalState(operations.deployment)
	require.NoError(t, err)
	require.NoError(t, state.SetWorkflowStateAndWrite(
		&config.WorkflowStateStopped{}, operations.deployment,
	))
	selected.publishCalls = 0
	// When
	_, err = operations.disable(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Zero(t, selected.publishCalls)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	require.Empty(t, document.Containers)
}

func TestSidecarSavedDefinitionSurvivesCatalogChange(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	ctx := context.Background()
	_, err := operations.enable(ctx, "example")
	require.NoError(t, err)
	saved := selected.manager.definitions["example"].Clone()
	delete(operations.templates.Entries(), "example")
	delete(selected.manager.definitions, "example")
	// When
	require.NoError(t, operations.reconcile(ctx, false))
	status, err := operations.status(ctx, "example")
	// Then
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.True(t, status.Running)
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
				result, err := operations.enable(context.Background(), "example")
				// Then
				if err != nil || !result.Enabled || result.Running ||
					selected.manager.ensureCalls != 0 {
					t.Fatalf("deferred enable: %+v, %v", result, err)
				}
				// When
				selected.running = true
				err = operations.reconcile(context.Background(), false)
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
	result, err := operations.enable(context.Background(), "example")
	// Then
	if err != nil || !result.Enabled || !result.RestartRequired ||
		result.Running {
		t.Fatalf("pending enable: %+v, %v", result, err)
	}
	if selected.manager.ensureCalls != 0 || selected.stopCalls != 0 {
		t.Fatal("pending enable changed containers")
	}
	// When
	err = operations.reconcile(context.Background(), false)
	state, readErr := config.ReadSidecarState(operations.deployment)
	// Then
	require.ErrorIs(t, err, ErrSidecarRestartRequired)
	require.NoError(t, readErr)
	require.Contains(t, state["example"], ErrSidecarRestartRequired.Error())
	require.NotContains(t, state["example"], "exasol stop")
}

func TestSidecarReconciliationUsesSavedDefinitions(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	selected.manager.definitions["orphan"] = sidecar.Container{Name: "orphan", Image: "caddy:2"}
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	document.Containers[0].Args = []string{"edited"}
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	err = operations.reconcile(context.Background(), false)
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
	_, startErr := operations.enable(context.Background(), "example")
	result, statusErr := operations.status(context.Background(), "example")
	state, stateErr := config.ReadExasolPersonalState(operations.deployment)
	// Then
	if startErr == nil || statusErr != nil || stateErr != nil || !result.Enabled ||
		result.Running ||
		!strings.Contains(result.LastOperationError, "image unavailable") ||
		!selected.running {
		t.Fatalf("failed enable: %+v, %v, %v, %v", result, startErr, statusErr, stateErr)
	}
	workflow, err := state.GetWorkflowState()
	if _, ok := workflow.(*config.WorkflowStateRunning); err != nil || !ok {
		t.Fatalf("database state changed: %T, %v", workflow, err)
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

func TestSidecarReconciliationClearsAbsentFailure(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	require.NoError(t, config.WriteSidecarState(operations.deployment,
		map[string]string{"removed": "sidecar removed: image unavailable"}))
	// When
	err := operations.reconcile(context.Background(), false)
	failures, readErr := config.ReadSidecarState(operations.deployment)
	// Then
	if err != nil || readErr != nil || len(failures) != 0 {
		t.Fatalf("stale failure cleanup: %v, %v, %v", failures, err, readErr)
	}
}

func TestSidecarStatusRefreshesRecordedFailures(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name           string
		reconciliation string
		disturb        func(document *sidecar.Document, runtime *sidecarRuntimeStub)
	}{
		{"recovered", "complete", nil},
		{
			"changed definition", "pending",
			func(document *sidecar.Document, _ *sidecarRuntimeStub) {
				document.Containers[0].Args = []string{"edited"}
			},
		},
		{
			"missing container", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.manager.observed = []localinstall.ContainerListing{}
			},
		},
		{
			"exited container", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.manager.observed[0].State = "exited"
			},
		},
		{
			"missing definition label", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				delete(runtime.manager.observed[0].Labels, "com.exasol.launcher.sidecar-definition")
			},
		},
		{
			"missing publication", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.manager.observed[0].Ports = nil
			},
		},
		{
			"duplicate containers", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.manager.observed = append(
					runtime.manager.observed,
					runtime.manager.observed[0],
				)
			},
		},
		{
			"legacy network", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.manager.restart = true
			},
		},
		{
			"disabled with container", "pending",
			func(document *sidecar.Document, _ *sidecarRuntimeStub) {
				document.Containers = nil
			},
		},
		{
			"disabled with orphan forward", "pending",
			func(document *sidecar.Document, runtime *sidecarRuntimeStub) {
				document.Containers = nil
				runtime.manager.observed[0].ID = ""
				runtime.manager.observed[0].State = "stopped"
			},
		},
		{
			"disabled and removed", "complete",
			func(document *sidecar.Document, runtime *sidecarRuntimeStub) {
				document.Containers = nil
				runtime.manager.observed = []localinstall.ContainerListing{}
			},
		},
		{
			"database stopped with container", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.running = false
			},
		},
		{
			"database stopped and removed", "complete",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.running = false
				runtime.manager.observed = []localinstall.ContainerListing{}
			},
		},
		{
			"host unavailable", "unknown",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.available = false
			},
		},
		{
			"inspection failed", "unknown",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.manager.listError = errors.New("runtime unreachable")
			},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// Given
			ops, runtime := newSidecarOperationsFixture(t)
			entry := ops.templates.Entries()["example"]
			entry.Container.Ports = []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}}
			ops.templates.Entries()["example"] = entry
			_, err := ops.enable(t.Context(), "example")
			require.NoError(t, err)
			runtime.manager.observed, err = runtime.manager.List(t.Context())
			require.NoError(t, err)
			document, err := config.ReadSidecars(ops.deployment)
			require.NoError(t, err)
			if scenario.disturb != nil {
				scenario.disturb(&document, runtime)
			}
			require.NoError(t, config.WriteSidecars(ops.deployment, document))
			saved, err := os.ReadFile(ops.deployment.SidecarsPath())
			require.NoError(t, err)
			require.NoError(t, config.WriteSidecarState(ops.deployment, map[string]string{
				"example": "previous failure", "unrelated": "unrelated failure",
			}))
			// When
			result, err := ops.status(t.Context(), "example")
			// Then
			require.NoError(t, err)
			require.Equal(t, scenario.reconciliation, result.Reconciliation)
			failures, err := config.ReadSidecarState(ops.deployment)
			require.NoError(t, err)
			require.Equal(t, "unrelated failure", failures["unrelated"])
			if scenario.reconciliation == "complete" {
				require.Empty(t, result.LastOperationError)
				require.NotContains(t, failures, "example")
			} else {
				require.Equal(t, "previous failure", result.LastOperationError)
				require.Equal(t, "previous failure", failures["example"])
			}
			if scenario.name == "inspection failed" {
				require.Equal(t, "unknown", result.State)
				require.Equal(t, "runtime unreachable", result.Error)
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
	selected.manager.removeError = errors.New("container busy")
	// When
	result, err := operations.disable(context.Background(), definition.Name)
	// Then
	require.ErrorContains(t, err, "container busy")
	require.False(t, result.Enabled)
	require.True(t, result.Running)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	require.Empty(t, document.Containers)
	// When
	selected.manager.removeError = nil
	result, err = operations.disable(context.Background(), definition.Name)
	// Then
	require.NoError(t, err)
	require.False(t, result.Enabled)
	require.False(t, result.Running)
	require.Empty(t, result.LastOperationError)
	require.True(t, selected.running)
}

func TestSidecarObservedEndpointsFollowRuntime(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example"); err != nil {
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
	if err != nil || !result.Running || !slices.Equal(result.Endpoints,
		[]SidecarEndpoint{{IP: "127.0.0.1", Port: 18080}}) {
		t.Fatalf("observed status: %+v, %v", result, err)
	}
}
