// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launcherpaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
