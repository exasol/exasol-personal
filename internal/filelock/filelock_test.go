// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package filelock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func openLockFile(t *testing.T, path string) *os.File {
	t.Helper()

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	return file
}

func TestTryLock_ExclusiveLockExcludesEveryOtherHandle(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "lock")
	holder := openLockFile(t, path)
	require.NoError(t, TryLock(holder, true))
	other := openLockFile(t, path)

	// When
	exclusiveErr := TryLock(other, true)
	sharedErr := TryLock(other, false)

	// Then
	require.ErrorIs(t, exclusiveErr, ErrWouldBlock)
	require.ErrorIs(t, sharedErr, ErrWouldBlock)
}

func TestTryLock_SharedLocksCoexistAndExcludeExclusive(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "lock")
	first := openLockFile(t, path)
	require.NoError(t, TryLock(first, false))

	// When
	sharedErr := TryLock(openLockFile(t, path), false)
	exclusiveErr := TryLock(openLockFile(t, path), true)

	// Then
	require.NoError(t, sharedErr)
	require.ErrorIs(t, exclusiveErr, ErrWouldBlock)
}

func TestUnlock_ReleasesTheLock(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "lock")
	holder := openLockFile(t, path)
	require.NoError(t, TryLock(holder, true))

	// When
	require.NoError(t, Unlock(holder))

	// Then
	require.NoError(t, TryLock(openLockFile(t, path), true))
}

func TestClose_ReleasesTheLock(t *testing.T) {
	t.Parallel()

	// Given
	path := filepath.Join(t.TempDir(), "lock")
	holder := openLockFile(t, path)
	require.NoError(t, TryLock(holder, true))

	// When
	require.NoError(t, holder.Close())

	// Then
	require.NoError(t, TryLock(openLockFile(t, path), true))
}
