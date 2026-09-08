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

func TestDirPath_JoinsRootAndPersonalUnderBaseDir(t *testing.T) {
	t.Parallel()

	got := DirPath("/home/user")
	want := filepath.Join("/home/user", ".exasol", "personal")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRootDirPath_ResolvesUnderTheCurrentUsersHomeDirectory(t *testing.T) {
	t.Parallel()

	got, err := RootDirPath()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if filepath.Base(got) != "personal" || filepath.Base(filepath.Dir(got)) != ".exasol" {
		t.Fatalf("expected a path ending in .exasol/personal, got %q", got)
	}
}

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
