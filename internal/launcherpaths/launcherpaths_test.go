// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launcherpaths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPurposePaths_ResolveUnderTheLauncherNamespace(t *testing.T) {
	t.Parallel()

	for relative, resolve := range map[string]func() (string, error){
		"deployments":            DeploymentsRootPath,
		"history/exasol_history": HistoryFilePath,
		"resources.yaml":         ResourceConfigFilePath,
		"resources":              CacheRootPath,
		".migrated":              MigrationMarkerPath,
	} {
		// When
		got, err := resolve()

		// Then
		require.NoError(t, err)
		want := filepath.Join("launcher", filepath.FromSlash(relative))
		require.True(t, strings.HasSuffix(got, want), got)
	}
}

func TestLegacyPaths_ResolveWhereEarlierVersionsKeptThem(t *testing.T) {
	t.Parallel()

	// Given
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	cacheDir, err := os.UserCacheDir()
	require.NoError(t, err)

	legacyHome := filepath.Join(home, ".exasol", "personal")
	legacyCache := filepath.Join(cacheDir, ".exasol", "personal")

	for want, resolve := range map[string]func() (string, error){
		filepath.Join(legacyHome, "deployments"):            LegacyDeploymentsRootPath,
		filepath.Join(legacyHome, "runtime-artifacts.yaml"): LegacyResourceConfigFilePath,
		filepath.Join(legacyCache, "runtime-artifacts"):     LegacyCacheRootPath,
		filepath.Join(cacheDir, "exasol_history"):           LegacyHistoryFilePath,
	} {
		// When
		got, err := resolve()

		// Then
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
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
