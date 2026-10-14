// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

type forwardStub struct {
	state map[string]runnerForwardState
	calls [][]string
	err   error
}

func (stub *forwardStub) forwards() macForwards {
	if stub.state == nil {
		stub.state = map[string]runnerForwardState{}
	}

	return macForwards{
		read: func() (map[string]runnerForwardState, error) {
			return maps.Clone(stub.state), nil
		},
		update: func(_ context.Context, args ...string) error {
			stub.calls = append(stub.calls, args)
			if stub.err == nil && args[0] == "remove" {
				delete(stub.state, args[1])
			}

			return stub.err
		},
	}
}

func caddyEndpoint() sidecar.PublishedPort {
	return sidecar.PublishedPort{
		Sidecar: "caddy", HostIP: "127.0.0.1", HostPort: 18080, RuntimePort: 30080,
	}
}

func TestMacForwardsPublishAndReuseHostEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	stub := &forwardStub{
		state: map[string]runnerForwardState{"db": {GuestPort: 8563, HostPort: 28563}},
	}
	endpoints := []sidecar.PublishedPort{caddyEndpoint()}
	// When
	err := stub.forwards().open(t.Context(), endpoints)
	// Then
	require.NoError(t, err)
	want := [][]string{{"add", "--host-ip", "127.0.0.1", "sidecar.caddy.0:30080:18080"}}
	require.Equal(t, want, stub.calls)
	// When
	stub.state["sidecar.caddy.0"] = runnerForwardState{
		HostIP: "127.0.0.1", HostPort: 18080, GuestPort: 30080,
	}
	err = stub.forwards().open(t.Context(), endpoints)
	opened, openedErr := stub.forwards().opened()
	// Then
	require.NoError(t, err)
	require.NoError(t, openedErr)
	require.Equal(t, want, stub.calls)
	require.Equal(t, endpoints, opened)
}

func TestMacForwardsReplaceChangedAndRemoveObsoleteEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	stub := &forwardStub{state: map[string]runnerForwardState{
		"db":                    {GuestPort: 8563, HostPort: 28563},
		"sidecar.caddy.0":       {GuestPort: 30079, HostPort: 18080, HostIP: "127.0.0.1"},
		"sidecar.caddy.1":       {GuestPort: 30081, HostPort: 18081, HostIP: "127.0.0.1"},
		"sidecar.caddy-other.0": {GuestPort: 30082, HostPort: 18082, HostIP: "127.0.0.1"},
	}}
	// When
	err := stub.forwards().open(t.Context(), []sidecar.PublishedPort{caddyEndpoint()})
	// Then
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"remove", "sidecar.caddy-other.0"},
		{"remove", "sidecar.caddy.0"},
		{"remove", "sidecar.caddy.1"},
		{"add", "--host-ip", "127.0.0.1", "sidecar.caddy.0:30080:18080"},
	}, stub.calls)
	require.Equal(t, map[string]runnerForwardState{
		"db": {GuestPort: 8563, HostPort: 28563},
	}, stub.state)
}

func TestMacForwardsReportAndWithdrawOrphanedEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	stub := &forwardStub{state: map[string]runnerForwardState{
		"db":              {GuestPort: 8563, HostPort: 28563},
		"sidecar.caddy.0": {GuestPort: 30080, HostPort: 18080, HostIP: "127.0.0.1"},
	}}
	// When
	opened, err := stub.forwards().opened()
	// Then
	require.NoError(t, err)
	require.Equal(t, []sidecar.PublishedPort{caddyEndpoint()}, opened)
	// When
	for range 2 {
		require.NoError(t, stub.forwards().open(t.Context(), nil))
	}
	// Then
	require.Equal(t, [][]string{{"remove", "sidecar.caddy.0"}}, stub.calls)
	require.Equal(t, map[string]runnerForwardState{
		"db": {GuestPort: 8563, HostPort: 28563},
	}, stub.state)
}

func TestMacForwardsRetainEndpointsWhenRunnerFails(t *testing.T) {
	t.Parallel()
	// Given
	failure := errors.New("runner unavailable")
	stub := &forwardStub{
		state: map[string]runnerForwardState{"sidecar.caddy.0": {GuestPort: 30080}},
		err:   failure,
	}
	// When
	err := stub.forwards().open(t.Context(), nil)
	// Then
	require.ErrorIs(t, err, failure)
	require.Contains(t, stub.state, "sidecar.caddy.0")
}

func TestMacRuntimeReportsNoEndpointsBeforeTheVMStarts(t *testing.T) {
	t.Parallel()
	// Given
	runtime := NewMacVMRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	declared := caddyEndpoint()
	declared.RuntimePort = 0
	// When
	opened, err := runtime.OpenedPorts(t.Context())
	publishErr := runtime.OpenPorts(t.Context(), []sidecar.PublishedPort{declared})
	// Then
	require.NoError(t, err)
	require.Empty(t, opened)
	require.NoError(t, publishErr)
}

func TestMacForwardsSkipUnboundEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	stub := &forwardStub{}
	stopped := caddyEndpoint()
	stopped.RuntimePort = 0
	// When
	err := stub.forwards().open(t.Context(), []sidecar.PublishedPort{stopped})
	opened, openedErr := stub.forwards().opened()
	// Then
	require.NoError(t, err)
	require.NoError(t, openedErr)
	require.Empty(t, stub.calls)
	require.Empty(t, opened)
}
