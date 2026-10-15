// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func TestSidecarsMissingAndExplicitEmpty(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	// When
	missing, err := config.ReadSidecars(deployment)
	// Then
	if err != nil || len(missing.Containers) != 0 {
		t.Fatalf("missing: %+v, %v", missing, err)
	}
	// When
	err = config.WriteSidecars(deployment, missing)
	// Then
	require.NoError(t, err)
	empty, err := config.ReadSidecars(deployment)
	if err != nil || len(empty.Containers) != 0 {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
}

func TestSidecarsInvalidWritePreservesSavedFile(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, config.WriteSidecars(deployment, sidecar.Document{Version: 1}))
	before, err := os.ReadFile(deployment.SidecarsPath())
	require.NoError(t, err)
	// When
	writeErr := config.WriteSidecars(deployment, sidecar.Document{Version: 42})
	// Then
	after, err := os.ReadFile(deployment.SidecarsPath())
	require.NoError(t, err)
	if writeErr == nil || string(before) != string(after) {
		t.Fatal("invalid write changed saved definition")
	}
}

func TestSidecarsTruncatedFileIsInvalid(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, os.WriteFile(deployment.SidecarsPath(), nil, 0o600))
	// When
	_, err := config.ReadSidecars(deployment)
	// Then
	require.Error(t, err, "expected malformed document error")
}

func TestSidecarsAreOwnerReadableOnly(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	//nolint:gosec // The loose mode is the precondition this test tightens.
	require.NoError(t, os.WriteFile(deployment.SidecarsPath(), nil, 0o644))
	// When
	require.NoError(t, config.WriteSidecars(deployment, sidecar.Document{
		Version: 1, Containers: []sidecar.Container{{Name: "example", Image: "caddy:2"}},
	}))
	// Then
	info, err := os.Stat(deployment.SidecarsPath())
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}
