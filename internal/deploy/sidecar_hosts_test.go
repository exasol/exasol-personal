// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
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
	require.Equal(t, "8563", second.manager.sources[sidecar.DatabaseSource]["port"])
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
