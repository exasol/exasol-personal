// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLinuxHostCommandStartFailure(t *testing.T) {
	t.Parallel()
	// Given
	runtime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	command := []string{filepath.Join(t.TempDir(), "missing-command")}
	// When
	err := runtime.OpenHostShell(context.Background(), command, nil, nil, nil)
	// Then
	require.Error(t, err)
	require.ErrorContains(t, err, "missing-command")
}

func TestLinuxHostCommandHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	// Given
	runtime := NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executable, err := os.Executable()
	require.NoError(t, err)
	// When
	err = runtime.OpenHostShell(ctx, []string{executable}, nil, nil, nil)
	// Then
	require.ErrorIs(t, err, context.Canceled)
}

func TestWindowsHostShellRemainsUnsupported(t *testing.T) {
	t.Parallel()
	// Given
	runtime := NewHostWindowsRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	// When
	err := runtime.OpenHostShell(context.Background(), []string{"unused-command"}, nil, nil, nil)
	// Then
	require.ErrorIs(t, err, ErrHostShellUnsupported)
}
