// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"
	"testing"

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

//nolint:paralleltest // overrides HOME to place the launcher-managed directory.
func TestDeploymentDir_CLISelectionFollowsTheDirectoryLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	deployments := filepath.Join(LauncherDirPath(home), deploymentsDirName)
	outside := filepath.Join(home, "work", "exasol")

	require.Empty(t, NewDeploymentDir(filepath.Join(deployments, "default")).CLISelection())
	require.Equal(t, " -d dev", NewDeploymentDir(filepath.Join(deployments, "dev")).CLISelection())
	require.Equal(t, " --deployment-dir "+outside, NewDeploymentDir(outside).CLISelection())
	require.Equal(t,
		" --deployment-dir '"+filepath.Join(home, "my deployment")+"'",
		NewDeploymentDir(filepath.Join(home, "my deployment")).CLISelection())
}
