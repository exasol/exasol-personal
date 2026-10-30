// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewContainerRuntime_DrivesThePodmanCLIForPodman(t *testing.T) {
	t.Parallel()

	runtime, err := NewContainerRuntime(EnginePodman, NewDirectExecutionEnvironment(nil))

	require.NoError(t, err)
	podman, ok := runtime.(podmanRuntime)
	require.True(t, ok, "expected the Podman runtime, got %T", runtime)
	require.Equal(t, podmanBinary, podman.binary)
}

func TestNewContainerRuntime_DrivesTheDockerCLIForDocker(t *testing.T) {
	t.Parallel()

	runtime, err := NewContainerRuntime(EngineDocker, NewDirectExecutionEnvironment(nil))

	require.NoError(t, err)
	docker, ok := runtime.(dockerRuntime)
	require.True(t, ok, "expected the Docker runtime, got %T", runtime)
	require.Equal(t, dockerBinary, docker.binary)
}

func TestNewContainerRuntime_RejectsUnknownEngine(t *testing.T) {
	t.Parallel()

	runtime, err := NewContainerRuntime("containerd", NewDirectExecutionEnvironment(nil))

	require.ErrorIs(t, err, ErrUnsupportedContainerEngine)
	require.ErrorContains(t, err, `"containerd"`)
	require.Nil(t, runtime)
}
