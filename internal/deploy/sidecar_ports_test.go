// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func savedHostPort(t *testing.T, deployment config.DeploymentDir, name string) int {
	t.Helper()
	document, err := config.ReadSidecars(deployment)
	require.NoError(t, err)

	return document.Sidecars[name].Container.Ports[0].HostPort
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
	require.Equal(t, 18080, savedHostPort(t, operations.deployment, "example"))
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
	require.Equal(t, 18081, savedHostPort(t, operations.deployment, "example"))
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
			require.Equal(t, 18081, savedHostPort(t, operations.deployment, "example"))
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
	require.Equal(t, 18080, savedHostPort(t, operations.deployment, "example"))
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
