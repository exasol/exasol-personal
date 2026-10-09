// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func publishingSidecarFixture(t *testing.T) (*sidecarOperations, *sidecarRuntimeStub) {
	t.Helper()
	ops, selected := newSidecarOperationsFixture(t)
	entry := ops.catalog.Entries()["example"]
	entry.Container.Ports = []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}}
	ops.catalog.Entries()["example"] = entry

	return ops, selected
}

func guestPublication(t *testing.T, guestPort int) []localinstall.SidecarContainer {
	t.Helper()

	return []localinstall.SidecarContainer{{
		ID: "example", State: "running",
		Labels: map[string]string{localinstall.SidecarNameLabel: "example"},
		Ports: []localinstall.SidecarPort{
			{HostIP: "0.0.0.0", HostPort: guestPort, ContainerPort: 8080, Protocol: "tcp"},
		},
	}}
}

func TestSidecarBackendOpensDeclaredEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	ops, selected := publishingSidecarFixture(t)
	// When
	_, err := ops.enable(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Equal(t, []sidecar.PublishedPort{{
		Sidecar: "example", Index: 0, HostIP: "127.0.0.1", HostPort: 18080, RuntimePort: 18080,
	}}, selected.publishedPorts)
	// When
	require.NoError(t, ops.reconcile(t.Context(), true))
	// Then
	require.Equal(t, []sidecar.PublishedPort{{
		Sidecar: "example", Index: 0, HostIP: "127.0.0.1", HostPort: 18080,
	}}, selected.publishedPorts)
	document, err := config.ReadSidecars(ops.deployment)
	require.NoError(t, err)
	require.True(t, containsSidecar(document, "example"))
}

func TestSidecarBackendWithdrawsDisabledEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	ops, selected := publishingSidecarFixture(t)
	_, err := ops.enable(t.Context(), "example")
	require.NoError(t, err)
	// When
	_, err = ops.disable(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Empty(t, selected.publishedPorts)
}

func TestSidecarStatusReportsOpenedEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	ops, selected := publishingSidecarFixture(t)
	selected.forwarding = true
	selected.manager.observed = guestPublication(t, 30080)
	// When
	result, err := ops.enable(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Equal(t, []sidecar.PublishedPort{{
		Sidecar: "example", Index: 0, HostIP: "127.0.0.1", HostPort: 18080, RuntimePort: 30080,
	}}, selected.publishedPorts)
	require.Equal(
		t,
		[]SidecarEndpoint{{IP: "127.0.0.1", Port: 18080}},
		result.Hosts[0].Endpoints,
	)
}

func TestSidecarReconciliationChecksOpenedEndpoints(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name  string
		guest int
		want  string
	}{
		{"published endpoint", 30080, sidecarReconciliationComplete},
		{"guest publication moved", 30081, sidecarReconciliationPending},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// Given
			ops, selected := publishingSidecarFixture(t)
			selected.forwarding = true
			selected.manager.observed = guestPublication(t, 30080)
			_, err := ops.enable(t.Context(), "example")
			require.NoError(t, err)
			document, err := config.ReadSidecars(ops.deployment)
			require.NoError(t, err)
			fingerprint, err := localinstall.SidecarDefinitionHash(document.Containers[0])
			require.NoError(t, err)
			observed := guestPublication(t, scenario.guest)
			observed[0].Labels["com.exasol.launcher.sidecar-definition"] = fingerprint
			selected.manager.observed = observed
			// When
			result, err := ops.status(t.Context(), "example")
			// Then
			require.NoError(t, err)
			require.Equal(t, scenario.want, result.Hosts[0].Reconciliation)
		})
	}
}

func TestSidecarReconciliationChecksDirectPublication(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name string
		host int
		want string
	}{
		{"requested binding", 18080, sidecarReconciliationComplete},
		{"binding moved", 18081, sidecarReconciliationPending},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// Given
			ops, selected := publishingSidecarFixture(t)
			_, err := ops.enable(t.Context(), "example")
			require.NoError(t, err)
			document, err := config.ReadSidecars(ops.deployment)
			require.NoError(t, err)
			fingerprint, err := localinstall.SidecarDefinitionHash(document.Containers[0])
			require.NoError(t, err)
			observed := guestPublication(t, scenario.host)
			observed[0].Ports[0].HostIP = "127.0.0.1"
			observed[0].Labels["com.exasol.launcher.sidecar-definition"] = fingerprint
			selected.manager.observed = observed
			// When
			result, err := ops.status(t.Context(), "example")
			// Then
			require.NoError(t, err)
			require.Equal(t, scenario.want, result.Hosts[0].Reconciliation)
		})
	}
}

func cloudPortFixture(t *testing.T, ports string) *tofuBackend {
	t.Helper()
	deployment := config.NewDeploymentDir(t.TempDir())
	workspace := filepath.Join(deployment.Root(), config.InfrastructureFilesDirectory)
	require.NoError(t, os.MkdirAll(workspace, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(workspace, "variables_public.tf"),
		[]byte("variable \"cluster_size\" {\n  type = \"number\"\n  default = 1\n}\n"),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workspace, "vars.tfvars"),
		[]byte("cluster_size = 1\nsidecar_ports = \""+ports+"\"\n"),
		0o600,
	))

	return newTofuBackend(deployment, &presets.InfrastructureManifest{
		Backend: backendTypeTofu, Tofu: &presets.InfrastructureTofu{},
	}, nil)
}

func TestCloudSidecarPortsEnterProviderConfiguration(t *testing.T) {
	t.Parallel()
	// Given
	backend := cloudPortFixture(t, "")
	// When
	err := backend.OpenPorts(t.Context(), []sidecar.PublishedPort{
		{Sidecar: "example", HostPort: 18081},
		{Sidecar: "other", HostPort: 18080},
	})
	// Then
	require.Error(t, err, "applying the provider configuration needs the tofu binary")
	opened, portsErr := backend.ingressPorts()
	require.NoError(t, portsErr)
	require.Equal(t, []int{18080, 18081}, opened)
}

func TestCloudSidecarPortsStayUnchangedWhenAlreadyOpen(t *testing.T) {
	t.Parallel()
	// Given
	backend := cloudPortFixture(t, "18080")
	// When
	err := backend.OpenPorts(t.Context(), []sidecar.PublishedPort{
		{Sidecar: "example", HostPort: 18080, RuntimePort: 18080},
	})
	// Then
	require.NoError(t, err)
	opened, portsErr := backend.OpenedPorts(t.Context())
	require.NoError(t, portsErr)
	require.Empty(t, opened, "cloud containers bind the node address themselves")
}

func TestCloudSidecarPortsSurviveReconfiguration(t *testing.T) {
	t.Parallel()
	// Given
	backend := cloudPortFixture(t, "18080,18081")
	// When
	err := backend.Configure(
		t.Context(),
		map[string]string{"cluster_size": "2"},
		DeploymentMetadata{ID: "test"},
		DeploymentLayout{},
	)
	// Then
	require.NoError(t, err)
	opened, portsErr := backend.ingressPorts()
	require.NoError(t, portsErr)
	require.Equal(t, []int{18080, 18081}, opened)
}
