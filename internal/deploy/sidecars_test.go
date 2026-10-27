// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
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

func TestSidecarCleanupFailureStillStopsDatabase(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	require.NoError(t, writeLocalDeploymentArtifacts(operations.deployment,
		&localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}))
	if _, err := operations.enable(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	selected.containers.Add("second")
	selected.containers.Fail("rm", errors.New("cleanup failed"))
	// When
	result := stopLocalRuntime(context.Background(), selected, nil, nil)
	// Then
	if len(result.Sidecars) == 0 || result.Database != nil || selected.running ||
		selected.stopCalls != 1 || len(selected.removals()) != 2 {
		t.Fatalf("stop result: sidecars=%v database=%v stopped=%d cleanup=%v",
			result.Sidecars, result.Database, selected.stopCalls, selected.removals())
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
	secrets, readErr := sidecarSecrets(operations.deployment)
	// Then
	if err != nil || readErr != nil ||
		secrets[sidecar.DatabaseSecret]["password"].Value != "disposable-rotated-password" {
		t.Fatalf("credential refresh: %v, %v", err, readErr)
	}
}

func TestSidecarStartupRunsBeforeDatabaseWait(t *testing.T) {
	// Given
	t.Setenv(localSkipDatabaseWaitEnv, "1")
	operations, selected := newSidecarOperationsFixture(t)
	materializeTestSidecar(t, operations.deployment, sidecarTestCredentialCatalog(t))
	selected.containers.Fail("run", errors.New("image unavailable"))
	endpoint := &localruntime.VMRuntimeEndpoint{RuntimeEndpoint: *selected.endpoint}
	// When
	result := writeLocalRuntimeArtifactsAndWait(
		context.Background(), selected, endpoint, 0, nil, nil,
	)
	// Then
	if selected.created() != 1 || len(result.Sidecars) == 0 || result.Database != nil {
		t.Fatalf("startup without SQL wait: %d, %v, %v",
			selected.created(), result.Sidecars, result.Database)
	}
	if !strings.Contains(selected.lastEnvironment(), localDBPassword) {
		t.Fatal("connection values were unavailable at container startup")
	}
}

func TestSidecarDestroyRemovesObservedState(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	if _, err := operations.enable(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	// When
	err := destroyLocalRuntime(context.Background(), selected, nil, nil)
	_, stateErr := os.Stat(operations.deployment.SidecarStatePath())
	document, readErr := config.ReadSidecars(operations.deployment)
	// Then
	if err != nil || len(selected.containers.Sidecars()) != 0 ||
		!errors.Is(stateErr, os.ErrNotExist) {
		t.Fatalf("destroyed sidecar state: %v, %v", err, stateErr)
	}
	if readErr != nil || len(document.Sidecars) != 1 {
		t.Fatalf("desired configuration after destroy: %+v, %v", document, readErr)
	}
}

//nolint:paralleltest // Mutates package-level verification functions.
func TestSidecarFailureLeavesTheStartedDatabaseVerified(t *testing.T) {
	// Given
	calls := withFakeStoredCredentialVerification(t,
		func(context.Context, config.DeploymentDir) error { return nil })
	deployment := newTestDeploymentWithState(t)
	givenStoredCredential(t, deployment, &localCredentialMarker{DbPasswordGenerated: true})
	materializeTestSidecar(t, deployment, sidecarTestCredentialCatalog(t))
	runtime := &endpointRuntimeStub{
		deployment: deployment,
		endpoint:   &localruntime.RuntimeEndpoint{DBPort: localTestDatabasePort},
	}
	// When
	result := startPreparedLocalRuntime(
		context.Background(), runtime, localRuntimeConfig{}, 0, nil, nil,
	)
	// Then
	if result.Database != nil || len(result.Sidecars) == 0 || runtime.stopCalls != 0 ||
		*calls != 1 {
		t.Fatalf("start result: database=%v sidecars=%v stops=%d verifications=%d",
			result.Database, result.Sidecars, runtime.stopCalls, *calls)
	}
	assertCredentialVerified(t, deployment, true)
}

func sidecarTestCatalog(t *testing.T) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog([]byte(`version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
`))
	require.NoError(t, err)

	return catalog
}

// Consumes the database password, so tests can see resolved values arrive.
func sidecarTestCredentialCatalog(t *testing.T) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog([]byte(`version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: exasol-database
              key: password
`))
	require.NoError(t, err)

	return catalog
}

// Publishes a port, so tests can observe the endpoint a user would reach.
func sidecarTestPublishedCatalog(t *testing.T) *sidecar.Catalog {
	t.Helper()
	catalog, err := sidecar.LoadCatalog([]byte(`version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      ports:
        - containerPort: 8080
          hostPort: 18080
          hostIP: 127.0.0.1
`))
	require.NoError(t, err)

	return catalog
}

const sidecarEndpointCatalogYAML = `version: 1
sidecars:
  example:
    description: Test service
    architectures: [amd64, arm64]
    container:
      image: docker.io/library/caddy:2-alpine
      ports:
        - name: api
          containerPort: 8080
          hostPort: 18080
          hostIP: 127.0.0.1
    endpoints:
      - port: api
        scheme: http
        path: /v1
`

// Without publication, the declaration lands on a port the sidecar keeps
// internal.
func sidecarTestEndpointCatalog(t *testing.T, published bool) *sidecar.Catalog {
	t.Helper()
	definition := sidecarEndpointCatalogYAML
	if !published {
		definition = strings.Replace(
			definition, "          hostPort: 18080\n          hostIP: 127.0.0.1\n", "", 1,
		)
	}
	catalog, err := sidecar.LoadCatalog([]byte(definition))
	require.NoError(t, err)

	return catalog
}

func materializeTestSidecar(
	t *testing.T,
	deployment config.DeploymentDir,
	catalog sidecar.Resolver,
) {
	t.Helper()
	err := withDeploymentExclusiveLock(
		context.Background(),
		deployment,
		func(directory config.DeploymentDir) error {
			document, err := config.ReadSidecars(directory)
			if err != nil {
				return err
			}
			if containsSidecar(document, "example") {
				return nil
			}
			entry, err := catalog.Resolve("example", goruntime.GOARCH)
			if err != nil {
				return err
			}
			document.Sidecars["example"] = entry.Definition

			return config.WriteSidecars(directory, document)
		},
	)
	require.NoError(t, err)
}

// A saved entry is rewritten rather than edited, because a map value cannot be
// assigned through.
func editSavedDefinition(
	document *sidecar.Document, name string, edit func(*sidecar.Definition),
) {
	definition := document.Sidecars[name]
	edit(&definition)
	document.Sidecars[name] = definition
}

func TestSidecarEnableMaterializesTemplate(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	catalog := sidecarTestCatalog(t)
	// When
	materializeTestSidecar(t, deployment, catalog)
	// Then
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)
	saved := document.Sidecars["example"].Container
	if len(document.Sidecars) != 1 || saved.Name != "example" ||
		saved.Image != "docker.io/library/caddy:2-alpine" ||
		saved.RestartPolicy != sidecar.OnFailure {
		t.Fatalf("unexpected definition: %+v", document)
	}
	info, err := os.Stat(deployment.SidecarsPath())
	require.NoError(t, err)
	if goruntime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("file permissions = %o", info.Mode().Perm())
	}
}

func TestSidecarEnablePreservesEdits(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	catalog := sidecarTestCatalog(t)
	materializeTestSidecar(t, deployment, catalog)
	edited := []byte(
		"version: 1\nsidecars:\n  example:\n    container:\n      image: caddy:edited\n" +
			"      args: [custom]\n",
	)
	require.NoError(t, os.WriteFile(deployment.SidecarsPath(), edited, 0o600))
	// When
	materializeTestSidecar(t, deployment, catalog)
	// Then
	actual, err := os.ReadFile(deployment.SidecarsPath())
	require.NoError(t, err)
	if string(actual) != string(edited) {
		t.Fatalf("definition changed: %s", actual)
	}
}

func TestSidecarEnableSavesEndpointDeclarations(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestEndpointCatalog(t, true)
	// When
	_, err := operations.enable(t.Context(), "example")
	// Then
	require.NoError(t, err)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	require.Equal(
		t,
		[]sidecar.Endpoint{{Port: "api", Scheme: "http", Path: "/v1"}},
		document.Sidecars["example"].Endpoints,
	)
}

func TestSidecarEnableStoresConnectionReferences(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	catalog := sidecarTestCatalog(t)
	entry := catalog.Sidecars["example"]
	entry.Container.Env = []sidecar.EnvVar{{
		Name: "DB_PASSWORD",
		ValueFrom: &sidecar.EnvSource{SecretKeyRef: &sidecar.SecretKeyRef{
			Name: sidecar.DatabaseSecret, Key: "password",
		}},
	}}
	catalog.Sidecars["example"] = entry
	// When
	materializeTestSidecar(t, deployment, catalog)
	// Then
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)
	require.Len(t, document.Sidecars, 1)
	saved := document.Sidecars["example"].Container.Env
	require.Len(t, saved, 1)
	require.Nil(t, saved[0].Value)
	require.Equal(t, sidecar.DatabaseSecret, saved[0].ValueFrom.SecretKeyRef.Name)
	require.Equal(t, "password", saved[0].ValueFrom.SecretKeyRef.Key)
}

func TestSidecarDisableAbsentNamePreservesOtherDefinitions(t *testing.T) {
	t.Parallel()
	// Given
	deployment := newTestDeploymentWithState(t)
	require.NoError(t, os.MkdirAll(
		filepath.Dir(deployment.InfrastructureManifestPath()), 0o700,
	))
	require.NoError(t, os.WriteFile(
		deployment.InfrastructureManifestPath(), []byte("backend: local\n"), 0o600,
	))
	materializeTestSidecar(t, deployment, sidecarTestCatalog(t))
	// When
	result, err := DisableSidecar(testManagerContext(t), deployment, "absent")
	// Then
	require.NoError(t, err)
	require.True(t, result.NothingToRemove)
	document, readErr := config.ReadSidecars(deployment)
	require.NoError(t, readErr)
	if len(document.Sidecars) != 1 {
		t.Fatalf("saved definitions changed: %+v", document)
	}
}

func TestSidecarStatusReportsEveryEnabledSidecar(t *testing.T) {
	t.Parallel()
	// Given
	deployment := newTestDeploymentWithState(t)
	require.NoError(t, os.MkdirAll(
		filepath.Dir(deployment.InfrastructureManifestPath()), 0o700,
	))
	require.NoError(t, os.WriteFile(
		deployment.InfrastructureManifestPath(), []byte("backend: local\n"), 0o600,
	))
	materializeTestSidecar(t, deployment, sidecarTestCatalog(t))
	// When
	results, err := GetAllSidecarStatus(testManagerContext(t), deployment)
	// Then
	require.NoError(t, err)
	if len(results) != 1 || results[0].Name != "example" || !results[0].Enabled {
		t.Fatalf("unexpected status: %+v", results)
	}
}

func TestSidecarStatusReportsNothingWhenNoneAreEnabled(t *testing.T) {
	t.Parallel()
	// Given
	deployment := newTestDeploymentWithState(t)
	require.NoError(t, os.MkdirAll(
		filepath.Dir(deployment.InfrastructureManifestPath()), 0o700,
	))
	require.NoError(t, os.WriteFile(
		deployment.InfrastructureManifestPath(), []byte("backend: local\n"), 0o600,
	))
	// When
	results, err := GetAllSidecarStatus(testManagerContext(t), deployment)
	// Then
	require.NoError(t, err)
	if len(results) != 0 {
		t.Fatalf("unexpected status: %+v", results)
	}
}

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

func reconcileForTest(
	ctx context.Context, operations *deploymentSidecars, cleanup bool,
) error {
	failures, err := operations.reconcile(ctx, cleanup)

	return errors.Join(err, joinSidecarFailures(failures))
}

func newSidecarOperationsFixture(t *testing.T) (*deploymentSidecars, *sidecarRuntimeStub) {
	t.Helper()
	deployment := newTestDeploymentWithState(t)
	fake := newContainerRuntimeFake(sidecarTestOwner)
	manager, err := localinstall.NewSidecarRuntime(
		fake, sidecarTestOwner, localinstall.IdentityRuntimePath(t.TempDir()),
	)
	require.NoError(t, err)
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

	return &deploymentSidecars{
		deployment: deployment,
		templates:  sidecarTestCatalog(t),
		runtime:    selected,
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
	_, err = operations.disable(t.Context(), "example", false)
	// Then
	require.NoError(t, err)
	require.Zero(t, selected.publishCalls)
	requireSavedDisabled(t, operations.deployment, "example")
}

func requireSavedDisabled(t *testing.T, deployment config.DeploymentDir, name string) {
	t.Helper()
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)
	require.Contains(t, document.Sidecars, name)
	require.Equal(t, new(false), document.Sidecars[name].Enabled)
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
	require.NoError(t, reconcileForTest(ctx, operations, false))
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
	status, err = operations.disable(ctx, "example", false)
	// Then
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Empty(t, selected.containers.Sidecars())
	require.True(t, selected.running)
	requireSavedDisabled(t, operations.deployment, "example")
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
				err = reconcileForTest(context.Background(), operations, false)
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
	err = reconcileForTest(context.Background(), operations, true)
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
	err = reconcileForTest(context.Background(), operations, false)
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
	editSavedDefinition(&document, "example", func(definition *sidecar.Definition) {
		definition.Container.Args = []string{"edited"}
	})
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	err = reconcileForTest(context.Background(), operations, false)
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
	serviceOnly := DeploymentLifecycleResult{
		Sidecars: map[string]error{"example": serviceFailure},
	}
	combined := DeploymentLifecycleResult{
		Database: databaseFailure,
		Sidecars: map[string]error{"example": serviceFailure},
	}
	// Then
	if serviceOnly.Database != nil || !errors.Is(serviceOnly.Err(), serviceFailure) ||
		!errors.Is(combined.Err(), databaseFailure) ||
		!errors.Is(combined.Err(), serviceFailure) {
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
	err := reconcileForTest(context.Background(), operations, false)
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
				editSavedDefinition(document, "example", func(definition *sidecar.Definition) {
					definition.Container.Args = []string{"edited"}
				})
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
				document.Sidecars = nil
			},
		},
		{
			"disabled with orphan forward", "pending",
			func(document *sidecar.Document, runtime *sidecarRuntimeStub) {
				document.Sidecars = nil
				container := runtime.exampleContainer()
				container.id = ""
				container.running = false
			},
		},
		{
			"disabled and removed", "complete",
			func(document *sidecar.Document, runtime *sidecarRuntimeStub) {
				document.Sidecars = nil
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
			sidecars, runtime := newSidecarOperationsFixture(t)
			entry := sidecars.templates.Entries()["example"]
			entry.Container.Ports = []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}}
			sidecars.templates.Entries()["example"] = entry
			_, err := sidecars.enable(t.Context(), "example")
			require.NoError(t, err)
			delete(sidecars.templates.Entries(), "example")
			_, err = runtime.manager.List(t.Context())
			require.NoError(t, err)
			document, err := config.ReadSidecars(sidecars.deployment)
			require.NoError(t, err)
			if scenario.disturb != nil {
				scenario.disturb(&document, runtime)
			}
			require.NoError(t, config.WriteSidecars(sidecars.deployment, document))
			saved, err := os.ReadFile(sidecars.deployment.SidecarsPath())
			require.NoError(t, err)
			require.NoError(t, config.WriteSidecarState(sidecars.deployment, map[string]string{
				"example": "previous failure", "unrelated": "unrelated failure",
			}))
			// When
			require.NoError(t, sidecars.knownName("example"))
			result, err := sidecars.status(t.Context(), "example")
			// Then
			require.NoError(t, err)
			require.Equal(t, scenario.reconciliation, result.Reconciliation)
			failures, err := config.ReadSidecarState(sidecars.deployment)
			require.NoError(t, err)
			require.Equal(t, "unrelated failure", failures["unrelated"])
			if scenario.reconciliation == "complete" {
				require.Empty(t, result.LastOperationError)
				require.NotContains(t, failures, "example")
				if scenario.name == "disabled and removed" {
					require.Error(t, sidecars.knownName("example"))
				}
			} else {
				require.Equal(t, "previous failure", result.LastOperationError)
				require.Equal(t, "previous failure", failures["example"])
			}
			if scenario.name == "inspection failed" {
				require.Equal(t, "unknown", result.State)
				require.Equal(t, "runtime unreachable", result.Error)
			}
			unchanged, err := os.ReadFile(sidecars.deployment.SidecarsPath())
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
		sidecar.Document{Version: 1, Sidecars: map[string]sidecar.Definition{
			definition.Name: {Container: definition},
		}}))
	selected.containers.Add(definition.Name)
	selected.containers.Fail("rm", errors.New("container busy"))
	// When
	result, err := operations.disable(context.Background(), definition.Name, false)
	// Then
	require.ErrorContains(t, err, "container busy")
	require.False(t, result.Enabled)
	require.True(t, result.Running)
	requireSavedDisabled(t, operations.deployment, definition.Name)
	// When
	selected.containers.Fail("rm", nil)
	result, err = operations.disable(context.Background(), definition.Name, false)
	// Then
	require.NoError(t, err)
	require.False(t, result.Enabled)
	require.False(t, result.Running)
	require.Empty(t, result.LastOperationError)
	require.True(t, selected.running)
}

func TestSidecarEnableSavesEnabledState(t *testing.T) {
	t.Parallel()
	edited := sidecar.Container{Name: "example", Image: "caddy:edited"}
	for name, saved := range map[string]*sidecar.Definition{
		"new":        nil,
		"field-less": {Container: edited},
		"disabled":   {Enabled: new(false), Container: edited},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given
			operations, _ := newSidecarOperationsFixture(t)
			if saved != nil {
				require.NoError(t, config.WriteSidecars(operations.deployment, sidecar.Document{
					Version: 1, Sidecars: map[string]sidecar.Definition{"example": *saved},
				}))
			}
			// When
			result, err := operations.enable(t.Context(), "example")
			// Then
			require.NoError(t, err)
			require.True(t, result.Enabled)
			document, err := config.ReadSidecars(operations.deployment)
			require.NoError(t, err)
			require.Equal(t, new(true), document.Sidecars["example"].Enabled)
			if saved != nil {
				require.Equal(t, edited.Image, document.Sidecars["example"].Container.Image)
			}
		})
	}
}

func TestSidecarReenableUsesTheDisabledDefinition(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	edited := document.Sidecars["example"]
	edited.Container.Args = []string{"edited"}
	document.Sidecars["example"] = edited
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	disabled, disableErr := operations.disable(t.Context(), "example", false)
	stopped := selected.exampleContainer()
	enabled, enableErr := operations.enable(t.Context(), "example")
	// Then
	require.NoError(t, disableErr)
	require.False(t, disabled.Enabled)
	require.Nil(t, stopped)
	require.NoError(t, enableErr)
	require.True(t, enabled.Running)
	require.Equal(t, sidecarReconciliationComplete, enabled.Reconciliation)
	document, err = config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	require.Equal(t, []string{"edited"}, document.Sidecars["example"].Container.Args)
}

func TestSidecarDisableAlreadyDisabledIsNoOp(t *testing.T) {
	t.Parallel()
	// Given
	operations, runtime := newSidecarOperationsFixture(t)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	_, err = operations.disable(t.Context(), "example", false)
	require.NoError(t, err)
	removals := len(runtime.removals())
	// When
	result, err := operations.disable(t.Context(), "example", false)
	// Then
	require.NoError(t, err)
	require.True(t, result.NothingToRemove)
	require.False(t, result.Enabled)
	require.Len(t, runtime.removals(), removals)
	requireSavedDisabled(t, operations.deployment, "example")
}

func TestSidecarReconciliationRemovesDisabledSidecar(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := newSidecarOperationsFixture(t)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	disabled := document.Sidecars["example"]
	disabled.Enabled = new(false)
	document.Sidecars["example"] = disabled
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	err = reconcileForTest(t.Context(), operations, false)
	// Then
	require.NoError(t, err)
	require.Empty(t, selected.containers.Sidecars())
	requireSavedDisabled(t, operations.deployment, "example")
}

func TestSidecarRemoveDeletesTheSavedEntry(t *testing.T) {
	t.Parallel()
	for name, disabledFirst := range map[string]bool{"enabled": false, "disabled": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given
			operations, selected := newSidecarOperationsFixture(t)
			_, err := operations.enable(t.Context(), "example")
			require.NoError(t, err)
			if disabledFirst {
				_, err = operations.disable(t.Context(), "example", false)
				require.NoError(t, err)
			}
			// When
			removed, removeErr := operations.disable(t.Context(), "example", true)
			repeated, repeatErr := operations.disable(t.Context(), "example", true)
			// Then
			require.NoError(t, removeErr)
			require.False(t, removed.NothingToRemove)
			require.False(t, removed.Enabled)
			require.Empty(t, selected.containers.Sidecars())
			require.NoError(t, repeatErr)
			require.True(t, repeated.NothingToRemove)
			document, err := config.ReadSidecars(operations.deployment)
			require.NoError(t, err)
			require.NotContains(t, document.Sidecars, "example")
		})
	}
}

func TestSidecarDisableAlreadyAbsentIsNoOp(t *testing.T) {
	t.Parallel()
	// Given
	operations, runtime := newSidecarOperationsFixture(t)
	// When
	result, err := operations.disable(t.Context(), "custom", false)
	// Then
	require.NoError(t, err)
	require.True(t, result.NothingToRemove)
	require.False(t, result.Enabled)
	require.Equal(t, sidecarReconciliationComplete, result.Reconciliation)
	require.Zero(t, runtime.created())
	require.Empty(t, runtime.removals())
}

func TestSidecarDisableRecordsUnverifiedCleanup(t *testing.T) {
	t.Parallel()
	// Given
	operations, runtime := newSidecarOperationsFixture(t)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	runtime.available = false
	// When
	result, err := operations.disable(t.Context(), "example", false)
	failures, readErr := config.ReadSidecarState(operations.deployment)
	// Then
	require.ErrorContains(t, err, "cleanup could not be verified")
	require.NoError(t, readErr)
	require.False(t, result.NothingToRemove)
	require.Contains(t, failures, "example")
	require.NoError(t, operations.knownName("example"))
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

func TestSidecarStatusReportsDeclaredEndpointURL(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestEndpointCatalog(t, true)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	// When
	result, err := operations.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.True(t, result.Running)
	require.Equal(t, []SidecarEndpoint{
		{IP: "127.0.0.1", Port: 18080, URL: "http://127.0.0.1:18080/v1"},
	}, result.Endpoints)
}

func TestSidecarStatusOmitsURLForInternalPort(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestEndpointCatalog(t, false)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	// When
	result, err := operations.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.True(t, result.Running)
	require.Empty(t, result.Endpoints)
}

func TestSidecarDeclaredEndpointSurvivesCatalogChange(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestEndpointCatalog(t, true)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	delete(operations.templates.Entries(), "example")
	// When
	result, err := operations.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:18080/v1", result.Endpoints[0].URL)
}

func TestSidecarEditedDeclarationNeedsNoRecreation(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestEndpointCatalog(t, true)
	_, err := operations.enable(t.Context(), "example")
	require.NoError(t, err)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	editSavedDefinition(&document, "example", func(definition *sidecar.Definition) {
		definition.Endpoints[0].Path = "/edited"
	})
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	result, err := operations.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:18080/edited", result.Endpoints[0].URL)
	require.Equal(t, sidecarReconciliationComplete, result.Reconciliation)
}

func TestSidecarEndpointsResolveAgainstTheObservedAddress(t *testing.T) {
	t.Parallel()
	declared := sidecar.Container{Name: "example", Ports: []sidecar.Port{
		{Name: "api", ContainerPort: 8080, HostPort: 18080, HostIP: "127.0.0.1"},
		{Name: "api6", ContainerPort: 8080, HostPort: 18080, HostIP: "::1"},
		{Name: "metrics", ContainerPort: 9090, HostPort: 19090, HostIP: "127.0.0.1"},
		{Name: "alt", ContainerPort: 8080, HostPort: 18081, HostIP: "127.0.0.1"},
	}}
	apiDeclarations := declarationsFor(sidecar.Definition{
		Container: declared,
		Endpoints: []sidecar.Endpoint{
			{Port: "api", Scheme: "http", Path: "/v1"},
			{Port: "api6", Scheme: "http", Path: "/v1"},
		},
	})
	for _, fixture := range []struct {
		name string
		// Defaults to the declaration above.
		declarations endpointDeclarations
		container    localinstall.ContainerListing
		published    []sidecar.PublishedPort
		want         []SidecarEndpoint
	}{
		{
			name:      "direct binding",
			container: listingWithPorts(boundPort("127.0.0.1", 18080, 8080)),
			want: []SidecarEndpoint{
				{IP: "127.0.0.1", Port: 18080, URL: "http://127.0.0.1:18080/v1"},
			},
		},
		{
			name:      "forwarded binding",
			container: listingWithPorts(),
			published: []sidecar.PublishedPort{
				{
					Sidecar: "example", Index: 0, HostIP: "127.0.0.1", HostPort: 18080,
					RuntimePort: 49153,
				},
			},
			want: []SidecarEndpoint{
				{IP: "127.0.0.1", Port: 18080, URL: "http://127.0.0.1:18080/v1"},
			},
		},
		{
			name:      "IPv6 loopback",
			container: listingWithPorts(boundPort("::1", 18080, 8080)),
			want: []SidecarEndpoint{
				{IP: "::1", Port: 18080, URL: "http://[::1]:18080/v1"},
			},
		},
		{
			name:      "undeclared port",
			container: listingWithPorts(boundPort("127.0.0.1", 19090, 9090)),
			want:      []SidecarEndpoint{{IP: "127.0.0.1", Port: 19090}},
		},
		{
			name: "second host port on the same container port",
			container: listingWithPorts(
				boundPort("127.0.0.1", 18080, 8080), boundPort("127.0.0.1", 18081, 8080),
			),
			want: []SidecarEndpoint{
				{IP: "127.0.0.1", Port: 18080, URL: "http://127.0.0.1:18080/v1"},
				{IP: "127.0.0.1", Port: 18081},
			},
		},
		{
			name:      "forward opened before the definition was reordered",
			container: listingWithPorts(),
			published: []sidecar.PublishedPort{
				{
					Sidecar: "example", Index: 1, HostIP: "127.0.0.1", HostPort: 18080,
					RuntimePort: 49153,
				},
			},
			want: []SidecarEndpoint{
				{IP: "127.0.0.1", Port: 18080, URL: "http://127.0.0.1:18080/v1"},
			},
		},
		{
			name: "same host port on a second loopback address",
			declarations: declarationsFor(sidecar.Definition{
				Container: sidecar.Container{Name: "example", Ports: []sidecar.Port{
					{Name: "api", ContainerPort: 8080, HostPort: 18080},
					{
						Name: "admin", ContainerPort: 8081,
						HostPort: 18080, HostIP: "127.0.0.2",
					},
				}},
				Endpoints: []sidecar.Endpoint{{Port: "api", Scheme: "http", Path: "/v1"}},
			}),
			container: listingWithPorts(
				boundPort("127.0.0.1", 18080, 8080), boundPort("127.0.0.2", 18080, 8081),
			),
			want: []SidecarEndpoint{
				{IP: "127.0.0.1", Port: 18080, URL: "http://127.0.0.1:18080/v1"},
				{IP: "127.0.0.2", Port: 18080},
			},
		},
		{
			name: "non-canonical declared host IP",
			declarations: declarationsFor(sidecar.Definition{
				Container: sidecar.Container{Name: "example", Ports: []sidecar.Port{{
					Name: "api", ContainerPort: 8080,
					HostPort: 18080, HostIP: "0:0:0:0:0:0:0:1",
				}}},
				Endpoints: []sidecar.Endpoint{{Port: "api", Scheme: "http", Path: "/v1"}},
			}),
			container: listingWithPorts(boundPort("::1", 18080, 8080)),
			want: []SidecarEndpoint{
				{IP: "::1", Port: 18080, URL: "http://[::1]:18080/v1"},
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			declarations := fixture.declarations
			if declarations == nil {
				declarations = apiDeclarations
			}
			// When
			actual := sidecarEndpoints(fixture.container, fixture.published, declarations)
			// Then
			require.Equal(t, fixture.want, actual)
		})
	}
}

func listingWithPorts(ports ...localinstall.ContainerPort) localinstall.ContainerListing {
	return localinstall.ContainerListing{State: "running", Ports: ports}
}

func boundPort(hostIP string, hostPort, containerPort int) localinstall.ContainerPort {
	return localinstall.ContainerPort{
		HostIP: hostIP, HostPort: hostPort, ContainerPort: containerPort,
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

func TestSidecarStatusRejectsUnknownName(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	// When
	err := operations.knownName("absent")
	// Then
	if err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSidecarListReportsCloudDeploymentsUnsupported(t *testing.T) {
	t.Parallel()
	// When
	entries, err := unsupportedAvailability(backendTypeTofu, sidecarTestCatalog(t))
	// Then
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, entry := range entries {
		if entry.Supported || !strings.Contains(entry.Unsupported, backendTypeTofu) {
			t.Fatalf("unexpected availability: %+v", entry)
		}
	}
}
