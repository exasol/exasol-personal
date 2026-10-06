// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

type guestSidecarStub struct {
	localinstall.SidecarManager

	containers []localinstall.SidecarContainer
	removed    []string
}

func (stub *guestSidecarStub) List(context.Context) ([]localinstall.SidecarContainer, error) {
	return slices.Clone(stub.containers), nil
}

func (*guestSidecarStub) Ensure(context.Context, sidecar.Container, sidecar.Sources) error {
	return nil
}

func (stub *guestSidecarStub) Remove(_ context.Context, name string) error {
	stub.removed = append(stub.removed, name)
	return nil
}

type sidecarForwardStub struct {
	state map[string]runnerForwardState
	calls [][]string
	err   error
}

func (stub *sidecarForwardStub) manager(guest localinstall.SidecarManager) *macSidecars {
	return &macSidecars{
		SidecarManager: guest,
		readForwards: func() (map[string]runnerForwardState, error) {
			return maps.Clone(stub.state), nil
		},
		forward: func(_ context.Context, args ...string) error {
			stub.calls = append(stub.calls, args)
			if stub.err == nil && args[0] == "remove" {
				delete(stub.state, args[1])
			}

			return stub.err
		},
	}
}

func guestSidecar() localinstall.SidecarContainer {
	const name = "caddy"
	return localinstall.SidecarContainer{
		ID:     name,
		State:  "running",
		Labels: map[string]string{localinstall.SidecarNameLabel: name},
		Ports: []localinstall.SidecarPort{
			{HostIP: "0.0.0.0", HostPort: 30080, ContainerPort: 8080, Protocol: "tcp"},
		},
	}
}

func TestMacSidecarsPublishAndReuseHostForwards(t *testing.T) {
	t.Parallel()
	// Given
	guest := &guestSidecarStub{containers: []localinstall.SidecarContainer{guestSidecar()}}
	current := map[string]runnerForwardState{"db": {GuestPort: 8563, HostPort: 28563}}
	forwards := &sidecarForwardStub{state: current}
	manager := forwards.manager(guest)
	definition := sidecar.Container{
		Name:  "caddy",
		Image: "caddy",
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}},
	}
	// When
	err := manager.Ensure(t.Context(), definition, nil)
	// Then
	require.NoError(t, err)
	want := [][]string{{"add", "--host-ip", "127.0.0.1", "sidecar.caddy.0:30080:18080"}}
	require.Equal(t, want, forwards.calls)
	// When
	current["sidecar.caddy.0"] = runnerForwardState{
		GuestPort: 30080,
		HostPort:  18080,
		HostIP:    "127.0.0.1",
	}
	err = manager.Ensure(t.Context(), definition, nil)
	listed, listErr := manager.List(t.Context())
	// Then
	require.NoError(t, err)
	require.NoError(t, listErr)
	require.Equal(t, want, forwards.calls)
	if len(listed) != 1 || len(listed[0].Ports) != 1 || listed[0].Ports[0].HostPort != 18080 ||
		listed[0].Ports[0].HostIP != "127.0.0.1" {
		t.Fatalf("host endpoints = %+v", listed)
	}
}

func TestMacSidecarsReplaceChangedAndRemoveObsoleteForwards(t *testing.T) {
	t.Parallel()
	// Given
	guest := &guestSidecarStub{containers: []localinstall.SidecarContainer{guestSidecar()}}
	current := map[string]runnerForwardState{
		"db":                    {GuestPort: 8563, HostPort: 28563},
		"sidecar.caddy.0":       {GuestPort: 30079, HostPort: 18080, HostIP: "127.0.0.1"},
		"sidecar.caddy.1":       {GuestPort: 30081, HostPort: 18081, HostIP: "127.0.0.1"},
		"sidecar.caddy-other.0": {GuestPort: 30082, HostPort: 18082, HostIP: "127.0.0.1"},
	}
	forwards := &sidecarForwardStub{state: current}
	manager := forwards.manager(guest)
	definition := sidecar.Container{
		Name:  "caddy",
		Image: "caddy",
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}},
	}
	// When
	err := manager.Ensure(t.Context(), definition, nil)
	// Then
	require.NoError(t, err)
	want := [][]string{
		{"remove", "sidecar.caddy.0"},
		{"remove", "sidecar.caddy.1"},
		{"add", "--host-ip", "127.0.0.1", "sidecar.caddy.0:30080:18080"},
	}
	require.Equal(t, want, forwards.calls)
}

func TestMacSidecarsRecoverOrphanedForwards(t *testing.T) {
	t.Parallel()
	// Given
	guest := &guestSidecarStub{}
	current := map[string]runnerForwardState{
		"db":              {GuestPort: 8563, HostPort: 28563},
		"sidecar.caddy.0": {GuestPort: 30080, HostPort: 18080, HostIP: "127.0.0.1"},
	}
	forwards := &sidecarForwardStub{state: current}
	manager := forwards.manager(guest)
	// When
	listed, err := manager.List(t.Context())
	// Then
	if err != nil || len(listed) != 1 || listed[0].Name() != "caddy" || listed[0].Running() {
		t.Fatalf("orphan listing = %+v, %v", listed, err)
	}
	// When
	for range 2 {
		require.NoError(t, manager.Remove(t.Context(), "caddy"))
	}
	// Then
	if len(current) != 1 || current["db"].HostPort != 28563 || len(forwards.calls) != 1 ||
		len(guest.removed) != 2 {
		t.Fatalf("cleanup: forwards=%v calls=%v removed=%v", current, forwards.calls, guest.removed)
	}
}

func TestMacSidecarsRetainContainerWhenForwardRemovalFails(t *testing.T) {
	t.Parallel()
	// Given
	guest := &guestSidecarStub{containers: []localinstall.SidecarContainer{guestSidecar()}}
	failure := errors.New("runner unavailable")
	forwards := &sidecarForwardStub{
		state: map[string]runnerForwardState{"sidecar.caddy.0": {GuestPort: 30080}},
		err:   failure,
	}
	manager := forwards.manager(guest)
	// When
	err := manager.Remove(t.Context(), "caddy")
	// Then
	require.ErrorIs(t, err, failure)
	require.Empty(t, guest.removed)
}

func TestMacSidecarsKeepInternalPortsUnpublished(t *testing.T) {
	t.Parallel()
	// Given
	guest := &guestSidecarStub{containers: []localinstall.SidecarContainer{guestSidecar()}}
	forwards := &sidecarForwardStub{}
	manager := forwards.manager(guest)
	// When
	err := manager.Ensure(
		t.Context(),
		sidecar.Container{Name: "caddy", Ports: []sidecar.Port{{ContainerPort: 8080}}},
		nil,
	)
	listed, listErr := manager.List(t.Context())
	// Then
	require.NoError(t, err)
	require.NoError(t, listErr)
	require.Len(t, listed, 1)
	require.Empty(t, listed[0].Ports)
	require.Empty(t, forwards.calls)
}

func TestMacSidecarInspectionChecksPublication(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"complete", "missing", "wrong guest", "extra", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given
			definition := sidecar.Container{
				Name: "caddy", Image: "caddy",
				Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}},
			}
			fingerprint, err := localinstall.SidecarDefinitionHash(definition)
			require.NoError(t, err)
			container := guestSidecar()
			container.Labels["com.exasol.launcher.sidecar-definition"] = fingerprint
			guest := &guestSidecarStub{containers: []localinstall.SidecarContainer{container}}
			forward := runnerForwardState{HostIP: "127.0.0.1", HostPort: 18080, GuestPort: 30080}
			forwards := &sidecarForwardStub{state: map[string]runnerForwardState{
				"sidecar.caddy.0": forward,
			}}
			switch name {
			case "missing":
				delete(forwards.state, "sidecar.caddy.0")
			case "wrong guest":
				forward.GuestPort++
				forwards.state["sidecar.caddy.0"] = forward
			case "extra":
				forwards.state["sidecar.caddy.1"] = runnerForwardState{
					HostIP: "127.0.0.1", HostPort: 18081, GuestPort: 30081,
				}
			case "duplicate":
				forwards.state["sidecar.caddy.1"] = forward
			default:
				require.Equal(t, "complete", name)
			}
			// When
			listed, err := forwards.manager(guest).List(t.Context())
			// Then
			require.NoError(t, err)
			require.Len(t, listed, 1)
			matches, err := listed[0].Matches(definition)
			require.NoError(t, err)
			require.Equal(t, name == "complete", matches)
			require.Empty(t, forwards.calls)
			require.Empty(t, guest.removed)
		})
	}
}
