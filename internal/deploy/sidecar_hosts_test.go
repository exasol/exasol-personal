// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/remote"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

//nolint:nonamedreturns // Names distinguish the two host fixtures.
func twoSidecarHosts(t *testing.T) (
	operations *sidecarOperations, firstHost, secondHost *sidecarRuntimeStub,
) {
	t.Helper()
	ops, first := newSidecarOperationsFixture(t)
	_, second := newSidecarOperationsFixture(t)
	firstAdapter, secondAdapter := stubSidecarHost(first), stubSidecarHost(second)
	firstAdapter.name, secondAdapter.name = "n11", "n12"
	firstAdapter.databasePort, secondAdapter.databasePort = "8563", "9563"
	ops.hosts = []sidecarHost{firstAdapter, secondAdapter}

	return ops, first, second
}

func TestSidecarEveryHostReconcilesIndependently(t *testing.T) {
	t.Parallel()
	// Given
	ops, first, second := twoSidecarHosts(t)
	first.manager.startError = errors.New("unavailable image")
	// When
	result, err := ops.enable(t.Context(), "example")
	// Then
	require.ErrorContains(t, err, "on host n11")
	require.True(t, result.Enabled)
	require.Len(t, result.Hosts, 2)
	require.False(t, result.Hosts[0].Running)
	require.Contains(t, result.Hosts[0].LastOperationError, "unavailable image")
	require.True(t, result.Hosts[1].Running)
	require.Empty(t, result.Hosts[1].LastOperationError)
	require.Equal(t, "9563", second.manager.sources[sidecar.DatabaseSource]["port"])
	// When
	first.manager.startError = nil
	require.NoError(t, ops.reconcile(t.Context(), false))
	result, err = ops.status(t.Context(), "example")
	// Then
	require.NoError(t, err)
	for _, host := range result.Hosts {
		require.True(t, host.Running)
		require.Empty(t, host.LastOperationError)
	}
}

func TestSidecarMultiHostDisableRetriesFailedHost(t *testing.T) {
	t.Parallel()
	// Given
	ops, first, second := twoSidecarHosts(t)
	_, err := ops.enable(t.Context(), "example")
	require.NoError(t, err)
	first.manager.removeError = errors.New("busy")
	// When
	result, err := ops.disable(t.Context(), "example")
	// Then
	require.ErrorContains(t, err, "on host n11")
	require.False(t, result.Enabled)
	require.True(t, result.Hosts[0].Running)
	require.False(t, result.Hosts[1].Running)
	require.Empty(t, second.manager.definitions)
	// When
	first.manager.removeError = nil
	result, err = ops.disable(t.Context(), "example")
	// Then
	require.NoError(t, err)
	for _, host := range result.Hosts {
		require.False(t, host.Running)
		require.Empty(t, host.LastOperationError)
	}
}

func TestSidecarHostHooksRetainIntentAndClearObservedState(t *testing.T) {
	t.Parallel()
	// Given
	ops, first, second := twoSidecarHosts(t)
	materializeTestSidecar(t, ops.deployment, ops.catalog)
	hooks := sidecarHooks{
		deployment: ops.deployment,
		hosts: func(context.Context) ([]sidecarHost, error) {
			return ops.hosts, nil
		},
		ports: ops.ports,
	}
	// When
	require.NoError(t, hooks.HostsReady(t.Context()))
	first.manager.removeError = errors.New("busy")
	err := hooks.BeforeStop(t.Context())
	// Then
	require.ErrorContains(t, err, "on host n11")
	require.Empty(t, second.manager.definitions)
	document, err := config.ReadSidecars(ops.deployment)
	require.NoError(t, err)
	require.Len(t, document.Containers, 1)
	// When
	require.NoError(t, hooks.AfterDestroy())
	failures, err := config.ReadSidecarState(ops.deployment)
	// Then
	require.NoError(t, err)
	require.Empty(t, failures)
}

func TestCloudSidecarHostsUseNodeIdentityAndConnection(t *testing.T) {
	t.Parallel()
	// Given
	directory := newTestDeploymentWithState(t)
	backend := &tofuBackend{deployment: directory}
	state, err := config.ReadExasolPersonalState(directory)
	require.NoError(t, err)
	require.NoError(t, state.SetWorkflowStateAndWrite(&config.WorkflowStateRunning{}, directory))
	require.NoError(t, config.WriteDeploymentInfo(directory.Root(), &config.DeploymentInfo{
		Nodes: map[string]config.DeploymentNode{
			"n12": {PrivateIp: "10.0.0.12", Database: config.DeploymentDatabase{DbPort: "9563"}},
			"n11": {PrivateIp: "10.0.0.11", Database: config.DeploymentDatabase{DbPort: "8563"}},
		},
	}))
	// When
	hosts, err := backend.SidecarHosts(t.Context())
	// Then
	require.NoError(t, err)
	require.Len(t, hosts, 2)
	require.Equal(t, "n11", hosts[0].name)
	require.Equal(t, "n12", hosts[1].name)
	require.Equal(t, "amd64", hosts[0].architecture)
	require.Equal(t, "9563", hosts[1].databasePort)
	provider, ok := hosts[1].provider.(*cloudSidecarProvider)
	require.True(t, ok)
	require.Equal(t, "10.0.0.12", provider.address)
	// When
	require.NoError(
		t,
		state.SetWorkflowStateAndWrite(&config.WorkflowStateInitialized{}, directory),
	)
	hosts, err = backend.SidecarHosts(t.Context())
	// Then
	require.NoError(t, err)
	require.Empty(t, hosts)
}

type sidecarSSHStub struct {
	command  []string
	input    string
	failure  error
	attempts int
}

func (connection *sidecarSSHStub) RunCommand(
	_ context.Context,
	command []string,
	input io.Reader,
	_, _ io.Writer,
) error {
	connection.attempts++
	if connection.failure != nil {
		err := connection.failure
		connection.failure = nil

		return err
	}
	connection.command = command
	data, err := io.ReadAll(input)
	connection.input = string(data)

	return err
}

func TestCloudSidecarEnvironmentUsesPrivateInput(t *testing.T) {
	t.Parallel()
	// Given
	connection := &sidecarSSHStub{}
	environment := &sidecarSSHEnvironment{connection: connection}
	// When
	err := environment.Run(t.Context(), map[string]string{"PASSWORD": "disposable test value"},
		nil, io.Discard, io.Discard, "podman", "run", "--env", "PASSWORD", "caddy:2")
	// Then
	require.NoError(t, err)
	require.NotContains(t, strings.Join(connection.command, " "), "disposable test value")
	require.Contains(t, connection.input, "PASSWORD='disposable test value'")
}

func TestCloudSidecarWaitsForSSHWithoutReplayingCommands(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{remote.ErrFailedToConnect, errors.New("command failed")} {
		t.Run(failure.Error(), func(t *testing.T) {
			t.Parallel()
			// Given
			connection := &sidecarSSHStub{failure: failure}
			environment := &sidecarSSHEnvironment{connection: connection}
			// When
			err := environment.Run(t.Context(), map[string]string{"PASSWORD": "disposable"},
				nil, io.Discard, io.Discard, "podman", "run", "--env", "PASSWORD", "caddy:2")
			// Then
			if errors.Is(failure, remote.ErrFailedToConnect) {
				require.NoError(t, err)
				require.Equal(t, 2, connection.attempts)
				require.Contains(t, connection.input, "PASSWORD='disposable'")
			} else {
				require.ErrorIs(t, err, failure)
				require.Equal(t, 1, connection.attempts)
			}
		})
	}
}
