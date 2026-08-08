// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadDeploymentVersionMarker_MissingFileReturnsFalseWithoutError(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())

	version, exists, err := ReadDeploymentVersionMarker(deployment)

	require.NoError(t, err)
	require.False(t, exists)
	require.Empty(t, version)
}

func TestWriteThenReadDeploymentVersionMarker_RoundTripsTrimmed(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())

	require.NoError(t, WriteDeploymentVersionMarker(deployment, "  1.2.3  \n"))

	version, exists, err := ReadDeploymentVersionMarker(deployment)

	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "1.2.3", version)
}
