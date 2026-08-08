// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadInfrastructureManifest_MissingFileReturnsWrappedError(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())

	_, err := ReadInfrastructureManifest(deployment)

	require.ErrorIs(t, err, ErrMissingInfrastructureManifest)
}

func TestReadInfrastructureManifest_ParsesBackendAndName(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())
	require.NoError(t, os.MkdirAll(deployment.InfrastructureDir(), 0o750))
	manifestYAML := "name: Test\ndescription: test infra\nbackend: local\n"
	require.NoError(t, os.WriteFile(
		deployment.InfrastructureManifestPath(),
		[]byte(manifestYAML),
		0o600,
	))

	manifest, err := ReadInfrastructureManifest(deployment)

	require.NoError(t, err)
	require.Equal(t, "Test", manifest.Name)
	require.Equal(t, "local", manifest.Backend)
}

func TestReadInstallManifest_MissingFileReturnsWrappedError(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())

	_, err := ReadInstallManifest(deployment)

	require.ErrorIs(t, err, ErrMissingInstallManifest)
}

func TestReadInstallManifest_ParsesName(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())
	require.NoError(t, os.MkdirAll(deployment.InstallationDir(), 0o750))
	manifestYAML := "name: Test Install\ndescription: test install\n"
	require.NoError(t, os.WriteFile(
		deployment.InstallManifestPath(),
		[]byte(manifestYAML),
		0o600,
	))

	manifest, err := ReadInstallManifest(deployment)

	require.NoError(t, err)
	require.Equal(t, "Test Install", manifest.Name)
}
