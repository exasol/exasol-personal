// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"path"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/stretchr/testify/require"
)

func TestDeploymentDir_LayoutPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)

	require.Equal(t, root, deployment.Root())
	require.Equal(
		t,
		filepath.Join(root, InfrastructureFilesDirectory),
		deployment.InfrastructureDir(),
	)
	require.Equal(
		t,
		filepath.Join(root, InstallationFilesDirectory),
		deployment.InstallationDir(),
	)
	require.Equal(
		t,
		filepath.Join(root, ConnectionInstruction),
		deployment.ConnectionInstructionsPath(),
	)
}

func TestDeploymentDir_ArtifactPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)

	require.Equal(t, filepath.Join(root, nodeDetailsFileName), deployment.NodeDetailsPath())
	require.Equal(
		t,
		filepath.Join(root, DeploymentVersionMarkerFileName),
		deployment.DeploymentVersionMarkerPath(),
	)
	require.Equal(t, filepath.Join(root, secretsFileName), deployment.SecretsPath())
	require.Equal(
		t,
		filepath.Join(deployment.InfrastructureDir(), presets.InfrastructureManifestFilename),
		deployment.InfrastructureManifestPath(),
	)
	require.Equal(
		t,
		filepath.Join(deployment.InstallationDir(), presets.InstallationManifestFilename),
		deployment.InstallManifestPath(),
	)
	require.Equal(t, "..", deployment.RelativeInfrastructureArtifactDir())
	require.Equal(
		t,
		path.Join("..", InstallationFilesDirectory),
		deployment.RelativeInstallationPresetDir(),
	)
}

func TestDeploymentsRootPath_IsParentOfDefaultDeploymentDir(t *testing.T) {
	t.Parallel()

	rootsPath, err := DeploymentsRootPath()
	require.NoError(t, err)

	defaultPath, err := DefaultDeploymentDirPath()
	require.NoError(t, err)

	require.Equal(t, rootsPath, filepath.Dir(defaultPath))
}

func TestNamedDeploymentDirPath_SameParentAsDefault(t *testing.T) {
	t.Parallel()

	defaultPath, err := DefaultDeploymentDirPath()
	require.NoError(t, err)

	namedPath, err := NamedDeploymentDirPath("staging")
	require.NoError(t, err)

	require.Equal(t, filepath.Dir(defaultPath), filepath.Dir(namedPath))
	require.Equal(t, "staging", filepath.Base(namedPath))
}

func TestNamedDeploymentDirPath_DefaultNameMatchesDefaultPath(t *testing.T) {
	t.Parallel()

	defaultPath, err := DefaultDeploymentDirPath()
	require.NoError(t, err)

	namedPath, err := NamedDeploymentDirPath(defaultDeploymentDirName)
	require.NoError(t, err)

	require.Equal(t, defaultPath, namedPath)
}

func TestDeploymentDir_Resolve(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)
	relPath := "ssh/private-key.pem"
	absPath := filepath.Join(t.TempDir(), "external-key.pem")

	require.Equal(t, filepath.Join(root, relPath), deployment.Resolve(relPath))
	require.Equal(t, absPath, deployment.Resolve(absPath))
}
