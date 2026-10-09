// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func TestSidecarCloudHostNetworking(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	runtime.DatabaseHost = "10.0.0.11"
	definition := sidecar.Container{
		Name: "example", Image: "caddy:2",
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 8080}},
	}
	// When
	err := runtime.Ensure(t.Context(), definition, nil)
	restart, attachErr := runtime.AttachDatabase(t.Context())
	// Then
	require.NoError(t, err)
	require.NoError(t, attachErr)
	require.False(t, restart)
	args := environment.createCalls[0]
	require.Equal(t, "host", args[slices.Index(args, "--network")+1])
	require.Contains(t, args, "database:10.0.0.11")
	require.NotContains(t, args, "--publish")
	require.NotContains(t, args, "--network-alias")
	// When
	labels := map[string]string{}
	for index, arg := range args {
		if arg == labelFlag {
			key, value, _ := strings.Cut(args[index+1], "=")
			labels[key] = value
		}
	}
	environment.containers = []SidecarContainer{{ID: "test", State: "running", Labels: labels}}
	containers, err := runtime.List(t.Context())
	// Then
	require.NoError(t, err)
	require.Equal(t, []SidecarPort{{
		HostIP: "127.0.0.1", HostPort: 8080,
		ContainerPort: 8080, Protocol: "tcp",
	}}, containers[0].Ports)
}

func TestSidecarCloudRejectsRemappingBeforeRuntimeChanges(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	runtime.DatabaseHost = "10.0.0.11"
	definition := sidecar.Container{
		Name: "example", Image: "caddy:2",
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}},
	}
	// When
	err := runtime.Ensure(t.Context(), definition, nil)
	// Then
	require.ErrorContains(t, err, "hostPort to equal containerPort")
	require.Empty(t, environment.calls)
	require.Empty(t, environment.createCalls)
}
