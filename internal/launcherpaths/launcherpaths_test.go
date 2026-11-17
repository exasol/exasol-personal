// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launcherpaths

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPurposePaths_ResolveUnderPlatformRoots(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprintf("override=%t", override), func(t *testing.T) {
			// Given
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("XDG_CACHE_HOME", "")
			configRoot := filepath.Join(home, ".config", "exasol", "launcher")
			cacheRoot := filepath.Join(home, ".cache", "exasol", "launcher")
			durableRoot := filepath.Join(home, ".exasol", "launcher")
			roaming := filepath.Join(home, "AppData", "Roaming")
			local := filepath.Join(home, "AppData", "Local")
			if override {
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))
				t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "custom-cache"))
				configRoot = filepath.Join(home, "custom-config", "exasol", "launcher")
				cacheRoot = filepath.Join(home, "custom-cache", "exasol", "launcher")
				roaming = filepath.Join(home, "custom-roaming")
				local = filepath.Join(home, "custom-local")
			}
			t.Setenv("APPDATA", roaming)
			t.Setenv("LOCALAPPDATA", local)
			switch runtime.GOOS {
			case "darwin":
				configRoot = durableRoot
				cacheRoot = filepath.Join(home, "Library", "Caches", "exasol", "launcher")
			case "windows":
				durableRoot = filepath.Join(local, "exasol", "launcher")
				configRoot = filepath.Join(roaming, "exasol", "launcher")
				cacheRoot = durableRoot
			case "linux":
			default:
				t.Skip("platform-specific path contract covers Linux, macOS and Windows")
			}

			for want, resolve := range map[string]func() (string, error){
				filepath.Join(durableRoot, "deployments"):               DeploymentsRootPath,
				filepath.Join(durableRoot, "history", "exasol_history"): HistoryFilePath,
				filepath.Join(configRoot, "resources.yaml"):             ResourceConfigFilePath,
				filepath.Join(cacheRoot, "resources"):                   CacheRootPath,
			} {
				// When
				got, err := resolve()

				// Then
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		})
	}
}

func TestMacOSPaths_UseHomeDirectory(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))

	// When
	durableRoot, durableErr := durableRootPathForOS("darwin")
	configPath, configErr := resourceConfigFilePathForOS("darwin")

	// Then
	require.NoError(t, durableErr)
	require.NoError(t, configErr)
	require.Equal(t, filepath.Join(home, ".exasol", "launcher"), durableRoot)
	require.Equal(t, filepath.Join(home, ".exasol", "launcher", "resources.yaml"), configPath)
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
