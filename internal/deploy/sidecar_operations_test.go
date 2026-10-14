// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
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

	containers *containerRuntimeFake
	manager    *localinstall.SidecarRuntime
	running    bool
	available  bool
	stopCalls  int
}

func (runtime *sidecarRuntimeStub) Sidecars(
	_ context.Context,
) (*localinstall.SidecarRuntime, error) {
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

const sidecarTestOwner = "exasol-db-sidecar-test"

func newSidecarOperationsFixture(t *testing.T) (*sidecarOperations, *sidecarRuntimeStub) {
	t.Helper()
	deployment := newTestDeploymentWithState(t)
	fake := newContainerRuntimeFake(sidecarTestOwner)
	manager, err := localinstall.NewSidecarRuntime(
		fake, sidecarTestOwner, localinstall.IdentityRuntimePath(t.TempDir()),
	)
	require.NoError(t, err)
	manager.Salt = "sidecar-operations-test"
	selected := &sidecarRuntimeStub{
		deployment: deployment,
		endpoint:   &localruntime.RuntimeEndpoint{DBPort: 28563},
		containers: fake, manager: manager,
		available: true, running: true,
	}
	state, stateErr := config.ReadExasolPersonalState(deployment)
	require.NoError(t, stateErr)
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
	saved := selected.exampleContainer()
	require.NotNil(t, saved)
	delete(operations.templates.Entries(), "example")
	selected.containers.Clear()
	// When
	require.NoError(t, operations.reconcile(ctx, false))
	status, err := operations.status(ctx, "example")
	// Then
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.True(t, status.Running)
	require.Equal(t,
		saved.labels["com.exasol.launcher.sidecar-definition"],
		selected.exampleContainer().labels["com.exasol.launcher.sidecar-definition"],
	)
	// When
	status, err = operations.disable(ctx, "example")
	// Then
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Empty(t, selected.containers.Sidecars())
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
					selected.created() != 0 {
					t.Fatalf("deferred enable: %+v, %v", result, err)
				}
				// When
				selected.running = true
				err = operations.reconcile(context.Background(), false)
				// Then
				if err != nil || selected.created() != 1 {
					t.Fatalf("deferred reconciliation: %v", err)
				}
			},
		)
	}
}

func TestSidecarCleanupIgnoresDatabaseAttachment(t *testing.T) {
	t.Parallel()
	// Given a running sidecar and a database that no longer answers to its alias
	operations, selected := newSidecarOperationsFixture(t)
	_, err := operations.enable(context.Background(), "example")
	require.NoError(t, err)
	selected.containers.LeaveServiceNetwork()
	// When
	err = operations.reconcile(context.Background(), true)
	// Then
	require.NoError(t, err)
	require.Empty(t, selected.containers.Sidecars())
}

func TestSidecarOperationsPersistRestartRequirement(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	selected.containers.LeaveServiceNetwork()
	// When
	result, err := operations.enable(context.Background(), "example")
	// Then
	if err != nil || !result.Enabled || !result.RestartRequired ||
		result.Running {
		t.Fatalf("pending enable: %+v, %v", result, err)
	}
	if selected.created() != 0 || selected.stopCalls != 0 {
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
	selected.containers.Add("orphan")
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	document.Containers[0].Args = []string{"edited"}
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	err = operations.reconcile(context.Background(), false)
	// Then
	sidecars := selected.containers.Sidecars()
	if err != nil || len(sidecars) != 1 || sidecars["example"] == nil {
		t.Fatalf("reconciliation: %+v, %v", sidecars, err)
	}
}

func TestSidecarStartupFailureRetainsDesiredAndDatabaseState(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	selected.containers.Fail("run", errors.New("image unavailable"))
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
				runtime.containers.Clear()
			},
		},
		{
			"exited container", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.exampleContainer().running = false
			},
		},
		{
			"missing definition label", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				delete(
					runtime.exampleContainer().labels,
					"com.exasol.launcher.sidecar-definition",
				)
			},
		},
		{
			"missing publication", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.exampleContainer().ports = nil
			},
		},
		{
			"duplicate containers", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.containers.Duplicate("example")
			},
		},
		{
			"legacy network", "pending",
			func(_ *sidecar.Document, runtime *sidecarRuntimeStub) {
				runtime.containers.LeaveServiceNetwork()
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
				container := runtime.exampleContainer()
				container.id = ""
				container.running = false
			},
		},
		{
			"disabled and removed", "complete",
			func(document *sidecar.Document, runtime *sidecarRuntimeStub) {
				document.Containers = nil
				runtime.containers.Clear()
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
				runtime.containers.Clear()
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
				runtime.containers.Fail("ps", errors.New("runtime unreachable"))
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
			_, err = runtime.manager.List(t.Context())
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
			require.Equal(t, 1, runtime.created())
			require.Empty(t, runtime.removals())
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
	selected.containers.Add(definition.Name)
	selected.containers.Fail("rm", errors.New("container busy"))
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
	selected.containers.Fail("rm", nil)
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
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	if _, err := operations.enable(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	// When
	result, err := operations.status(context.Background(), "example")
	// Then
	if err != nil || !result.Running || !slices.Equal(result.Endpoints,
		[]SidecarEndpoint{{IP: "127.0.0.1", Port: 18080}}) {
		t.Fatalf("observed status: %+v, %v", result, err)
	}
}

// created reports how many containers the driver asked the runtime to make.
func (runtime *sidecarRuntimeStub) created() int {
	created, _ := runtime.containers.Calls()

	return created
}

// removals reports the containers the driver asked the runtime to remove.
func (runtime *sidecarRuntimeStub) removals() []string {
	_, removed := runtime.containers.Calls()

	return removed
}

// lastEnvironment reports what the most recent creation attempt carried
// through its environment file.
func (runtime *sidecarRuntimeStub) lastEnvironment() string {
	return runtime.containers.LastEnvironment()
}

// exampleContainer reports what the driver created for the fixture's sidecar.
func (runtime *sidecarRuntimeStub) exampleContainer() *fakeContainer {
	return runtime.containers.Sidecars()["example"]
}
