// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func savedHostPort(t *testing.T, deployment config.DeploymentDir) int {
	t.Helper()
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)

	return document.Sidecars["example"].Container.Ports[0].HostPort
}

func TestSidecarEnableSavesAvailableTemplatePort(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	// When
	_, err := operations.enable(t.Context(), "example", "")
	// Then
	require.NoError(t, err)
	require.Equal(t, 18080, savedHostPort(t, operations.deployment))
}

func TestSidecarEnableAdvancesPastHeldTemplatePort(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	operations.ports = testLocalPortAllocator(
		t, localMinimumAutomaticPort, localMaximumPort,
		func(_ string, port int) bool { return port != 18080 },
	)
	// When
	result, err := operations.enable(t.Context(), "example", "")
	// Then
	require.NoError(t, err)
	require.Equal(t, 18081, savedHostPort(t, operations.deployment))
	require.Equal(t, []SidecarEndpoint{{IP: "127.0.0.1", Port: 18081}}, result.Endpoints)
}

func TestSidecarEnableSkipsPortsSavedForOtherServices(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name          string
		databasePorts string
		others        map[string]sidecar.Definition
	}{
		{name: "database", databasePorts: "db:18080"},
		{name: "sidecar", others: map[string]sidecar.Definition{"other": {
			Container: sidecar.Container{Image: "caddy:2", Ports: []sidecar.Port{
				{Name: "http", ContainerPort: 8080, HostPort: 18080},
			}},
		}}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			operations, _ := newSidecarOperationsFixture(t)
			operations.templates = sidecarTestPublishedCatalog(t)
			operations.databasePorts = fixture.databasePorts
			if fixture.others != nil {
				require.NoError(t, config.WriteSidecars(operations.deployment, sidecar.Document{
					Version: 1, Sidecars: fixture.others,
				}))
			}
			// When
			_, err := operations.enable(t.Context(), "example", "")
			// Then
			require.NoError(t, err)
			require.Equal(t, 18081, savedHostPort(t, operations.deployment))
		})
	}
}

func TestSidecarEnableWithPortsMovesRunningSidecar(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	_, err := operations.enable(t.Context(), "example", "")
	require.NoError(t, err)
	// When
	result, err := operations.enable(t.Context(), "example", "http:18090")
	// Then
	require.NoError(t, err)
	require.Equal(t, sidecarReconciliationComplete, result.Reconciliation)
	require.Equal(t, []SidecarEndpoint{{IP: "127.0.0.1", Port: 18090}}, result.Endpoints)
}

func TestSidecarPortsRejectHostPortOfUnlistedOwnPort(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	_, err := operations.enable(t.Context(), "example", "")
	require.NoError(t, err)
	document, err := config.ReadSidecars(operations.deployment)
	require.NoError(t, err)
	definition := document.Sidecars["example"]
	definition.Container.Ports = append(definition.Container.Ports, sidecar.Port{
		Name: "admin", ContainerPort: 9090, HostPort: 18081,
	})
	document.Sidecars["example"] = definition
	require.NoError(t, config.WriteSidecars(operations.deployment, document))
	// When
	_, err = operations.enable(t.Context(), "example", "http:18081")
	// Then
	require.ErrorContains(t, err, `host port 18081 is saved for sidecar "example" port "admin"`)
	require.Equal(t, 18080, savedHostPort(t, operations.deployment))
}

func TestSidecarEnableWithoutFreePortSavesNothing(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	operations.ports = testLocalPortAllocator(
		t, localMinimumAutomaticPort, localMaximumPort,
		func(string, int) bool { return false },
	)
	// When
	_, err := operations.enable(t.Context(), "example", "")
	// Then
	require.ErrorContains(t, err, `sidecar "example"`)
	require.ErrorContains(t, err, `"http"`)
	document, readErr := config.ReadSidecars(operations.deployment)
	require.NoError(t, readErr)
	require.NotContains(t, document.Sidecars, "example")
}

func enabledPublishedFixture(
	t *testing.T, ports string,
) (*deploymentSidecars, *sidecarRuntimeStub) {
	t.Helper()
	operations, selected := newSidecarOperationsFixture(t)
	operations.templates = sidecarTestPublishedCatalog(t)
	_, err := operations.enable(t.Context(), "example", ports)
	require.NoError(t, err)

	return operations, selected
}

func TestSidecarConfigSetMovesRunningSidecar(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := enabledPublishedFixture(t, "")
	// When
	result, err := operations.configurePorts(t.Context(), "example", "http:18090")
	// Then
	require.NoError(t, err)
	require.Equal(t,
		SidecarConfiguration{Name: "example", Ports: "http:18090", Applied: true}, result)
	status, err := operations.status(t.Context(), "example")
	require.NoError(t, err)
	require.Equal(t, sidecarReconciliationComplete, status.Reconciliation)
	require.Equal(t, []SidecarEndpoint{{IP: "127.0.0.1", Port: 18090}}, status.Endpoints)
}

func TestSidecarConfigSetOnStoppedDeploymentAppliesOnStart(t *testing.T) {
	t.Parallel()
	// Given
	operations, selected := enabledPublishedFixture(t, "")
	writeFakeWorkflowState(t, operations.deployment, &config.WorkflowStateStopped{})
	selected.publishCalls = 0
	// When
	_, err := operations.configurePorts(t.Context(), "example", "http:18090")
	// Then
	require.NoError(t, err)
	require.Zero(t, selected.publishCalls)
	require.Equal(t, 18090, savedHostPort(t, operations.deployment))
	// When
	writeFakeWorkflowState(t, operations.deployment, &config.WorkflowStateRunning{})
	_, err = operations.reconcile(t.Context(), false)
	// Then
	require.NoError(t, err)
	status, err := operations.status(t.Context(), "example")
	require.NoError(t, err)
	require.Equal(t, []SidecarEndpoint{{IP: "127.0.0.1", Port: 18090}}, status.Endpoints)
}

func TestSidecarConfigResetStartsAtCatalogPort(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := enabledPublishedFixture(t, "http:18090")
	// When
	result, err := operations.configurePorts(t.Context(), "example", "")
	// Then
	require.NoError(t, err)
	require.Equal(t, "http:18080", result.Ports)
	require.Equal(t, 18080, savedHostPort(t, operations.deployment))
}

func TestSidecarConfigResetStartsAtSavedPortWithoutCatalogEntry(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := enabledPublishedFixture(t, "http:18090")
	delete(operations.templates.Entries(), "example")
	// When
	result, err := operations.configurePorts(t.Context(), "example", "")
	// Then
	require.NoError(t, err)
	require.Equal(t, "http:18090", result.Ports)
}

func TestSidecarConfigResetKeepsPortItsRunningContainerHolds(t *testing.T) {
	t.Parallel()
	// Given
	operations, _ := enabledPublishedFixture(t, "")
	operations.ports = testLocalPortAllocator(
		t, localMinimumAutomaticPort, localMaximumPort,
		func(_ string, port int) bool { return port != 18080 },
	)
	// When
	result, err := operations.configurePorts(t.Context(), "example", "")
	// Then
	require.NoError(t, err)
	require.Equal(t, "http:18080", result.Ports)
}
