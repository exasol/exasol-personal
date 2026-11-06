// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func seedLegacyDeployment(t *testing.T, name string) string {
	t.Helper()

	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	require.NoError(t, err)
	path := filepath.Join(legacyRoot, name)
	seedFile(t, filepath.Join(path, "deployment.json"), "{}")

	return path
}

func seedFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func newDeploymentPath(t *testing.T, name string) string {
	t.Helper()

	path, err := launcherpaths.NamedDeploymentDirPath(name)
	require.NoError(t, err)

	return path
}

func requireFileContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func runStartupMigrationForTest(t *testing.T) string {
	t.Helper()

	var out strings.Builder
	runStartupMigration(context.Background(), &out)

	return out.String()
}

func fakeLegacyDeploymentRunning(t *testing.T, running bool, err error) {
	t.Helper()

	original := legacyDeploymentRunningFn
	legacyDeploymentRunningFn = func(context.Context, config.DeploymentDir) (bool, error) {
		return running, err
	}
	t.Cleanup(func() { legacyDeploymentRunningFn = original })
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_AppliesChangesAutomaticallyWithoutPrompting(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	seedLegacyDeployment(t, "default")

	// When nothing supplies interactive input
	runStartupMigrationForTest(t)

	// Then
	require.FileExists(t, filepath.Join(newDeploymentPath(t, "default"), "deployment.json"))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ReportsWhatItChanged(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	seedLegacyDeployment(t, "default")

	// When
	out := runStartupMigrationForTest(t)

	// Then
	newRoot := filepath.Dir(newDeploymentPath(t, "default"))
	require.Contains(t, out, "Migrated managed deployments to "+newRoot)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_StaysQuietWhenNothingToMigrate(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.Empty(t, out)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_MovesKeptDeploymentOnALaterStart(t *testing.T) {
	// Given a deployment kept at the legacy location while it ran
	setTestHome(t, t.TempDir())
	legacyPath := seedLegacyDeployment(t, "default")
	fakeLegacyDeploymentRunning(t, true, nil)
	runStartupMigrationForTest(t)
	require.DirExists(t, legacyPath)
	fakeLegacyDeploymentRunning(t, false, nil)

	// When
	runStartupMigrationForTest(t)

	// Then
	require.DirExists(t, newDeploymentPath(t, "default"))
	require.NoDirExists(t, legacyPath)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ResumesInterruptedMigration(t *testing.T) {
	// Given an earlier run moved "default" but stopped before "staging"
	setTestHome(t, t.TempDir())
	legacyPath := seedLegacyDeployment(t, "staging")
	require.NoError(t, os.MkdirAll(newDeploymentPath(t, "default"), 0o700))

	// When
	runStartupMigrationForTest(t)

	// Then
	require.DirExists(t, newDeploymentPath(t, "default"))
	require.DirExists(t, newDeploymentPath(t, "staging"))
	require.NoDirExists(t, legacyPath)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_MovesLegacyCacheConfig(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	legacyPath, err := launcherpaths.LegacyResourceConfigFilePath()
	require.NoError(t, err)
	seedFile(t, legacyPath, "retention_days: 45\n")

	// When
	runStartupMigrationForTest(t)

	// Then
	newPath, err := launcherpaths.ResourceConfigFilePath()
	require.NoError(t, err)
	requireFileContent(t, newPath, "retention_days: 45\n")
	require.NoFileExists(t, legacyPath)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_DeletesLegacyCache(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	legacyRoot, err := launcherpaths.LegacyCacheRootPath()
	require.NoError(t, err)
	seedFile(t, filepath.Join(legacyRoot, "artifacts", "nano.tar"), "image")

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.NoDirExists(t, legacyRoot)
	require.Contains(t, out, "Deleted legacy resource cache at "+legacyRoot)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_RemovesEmptiedLegacyDirectories(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	legacyDeployment := seedLegacyDeployment(t, "default")
	legacyCache, err := launcherpaths.LegacyCacheRootPath()
	require.NoError(t, err)
	seedFile(t, filepath.Join(legacyCache, "artifacts", "nano.tar"), "image")

	// When
	runStartupMigrationForTest(t)

	// Then
	require.NoDirExists(t, filepath.Dir(filepath.Dir(legacyDeployment)))
	require.NoDirExists(t, filepath.Dir(filepath.Dir(legacyCache)))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_KeepsLegacyDirectoryThatStillHoldsContent(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	require.NoError(t, err)
	notes := filepath.Join(legacyRoot, "notes.txt")
	seedFile(t, notes, "keep")

	// When
	runStartupMigrationForTest(t)

	// Then
	require.FileExists(t, notes)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_KeepsRunningLegacyDeploymentAndMigratesTheRest(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	fakeLegacyDeploymentRunning(t, true, nil)
	legacyPath := seedLegacyDeployment(t, "default")
	legacyCache, err := launcherpaths.LegacyCacheRootPath()
	require.NoError(t, err)
	seedFile(t, filepath.Join(legacyCache, "artifacts", "nano.tar"), "image")

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.Contains(t, out, "Warning: kept "+legacyPath)
	require.Contains(t, out, "exasol stop --deployment-dir "+legacyPath)
	require.DirExists(t, legacyPath)
	require.NoDirExists(t, legacyCache)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_KeepsLegacyDeploymentWhoseStateIsUnknown(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	fakeLegacyDeploymentRunning(t, false, errors.New("engine unavailable"))
	legacyPath := seedLegacyDeployment(t, "default")

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.Contains(t, out, "Warning: kept "+legacyPath)
	require.Contains(t, out, "engine unavailable")
	require.DirExists(t, legacyPath)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_KeepsLockedLegacyDeployment(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	legacyPath := seedLegacyDeployment(t, "default")
	mutex, err := directorymutex.New(legacyPath)
	require.NoError(t, err)
	require.NoError(t, mutex.AcquireExclusive(context.Background()))
	t.Cleanup(func() { _ = mutex.ReleaseExclusive(context.Background()) })

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.Contains(t, out, "Warning: kept "+legacyPath)
	require.Contains(t, out, "close every exasol shell or other launcher command using it")
	require.Contains(t, out, "exasol diag unlock --deployment-dir "+legacyPath)
	require.DirExists(t, legacyPath)
}

func TestRawArgsRequestShellCompletion_SkipsShellCompletion(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{cobra.ShellCompRequestCmd, "status", ""},
		{cobra.ShellCompNoDescRequestCmd, "status", ""},
		{"completion", "bash"},
	} {
		// When
		skipped := rawArgsRequestShellCompletion(args)

		// Then
		require.True(t, skipped, args)
	}
	require.False(t, rawArgsRequestShellCompletion([]string{"status"}))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_KeepsCollidingLegacyDeploymentAndMigratesTheRest(t *testing.T) {
	// Given "default" at both roots and "staging" only at the legacy root
	setTestHome(t, t.TempDir())
	legacyDefault := seedLegacyDeployment(t, "default")
	legacyStaging := seedLegacyDeployment(t, "staging")
	newDefault := newDeploymentPath(t, "default")
	seedFile(t, filepath.Join(newDefault, "deployment.json"), "new")

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.Contains(t, out, "Warning: kept "+legacyDefault)
	require.Contains(t, out, newDefault)
	requireFileContent(t, filepath.Join(legacyDefault, "deployment.json"), "{}")
	requireFileContent(t, filepath.Join(newDefault, "deployment.json"), "new")
	require.NoDirExists(t, legacyStaging)
	require.DirExists(t, newDeploymentPath(t, "staging"))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_WarnsAboutAFailedStepAndContinues(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("relies on POSIX directory permissions enforced for a non-root user")
	}

	// Given a legacy history file whose directory refuses the rename
	setTestHome(t, t.TempDir())
	legacyHistory, err := launcherpaths.LegacyHistoryFilePath()
	require.NoError(t, err)
	seedFile(t, legacyHistory, "SELECT 1;\n")
	//nolint:gosec // remove write bit to force the rename to fail
	require.NoError(t, os.Chmod(filepath.Dir(legacyHistory), 0o500))
	//nolint:gosec
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(legacyHistory), 0o700) })
	seedLegacyDeployment(t, "default")

	// When
	out := runStartupMigrationForTest(t)

	// Then
	require.Contains(t, out, "Warning: migrate SQL history")
	requireFileContent(t, legacyHistory, "SELECT 1;\n")
	require.DirExists(t, newDeploymentPath(t, "default"))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ConcurrentInvocationsBothSucceed(t *testing.T) {
	for range 20 {
		// Given
		setTestHome(t, t.TempDir())
		for i := range 30 {
			seedLegacyDeployment(t, fmt.Sprintf("d%d", i))
		}

		// When
		outs := make(chan string, 2)
		for range 2 {
			go func() {
				var out strings.Builder
				runStartupMigration(context.Background(), &out)
				outs <- out.String()
			}()
		}

		// Then
		require.NotContains(t, <-outs, "Warning")
		require.NotContains(t, <-outs, "Warning")
		for i := range 30 {
			require.DirExists(t, newDeploymentPath(t, fmt.Sprintf("d%d", i)))
		}
	}
}

func TestMigrate_FileMoves(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source", "history")
	destination := filepath.Join(dir, "destination", "history")
	seedFile(t, source, "some history")

	// When
	moved, err := migrate(source, destination)

	// Then
	require.NoError(t, err)
	require.True(t, moved)
	requireFileContent(t, destination, "some history")
	require.NoFileExists(t, source)
}

func TestMigrate_DirectoryMoves(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source", "deployment")
	destination := filepath.Join(dir, "destination", "deployment")
	seedFile(t, filepath.Join(source, "deployment.json"), "{}")

	// When
	moved, err := migrate(source, destination)

	// Then
	require.NoError(t, err)
	require.True(t, moved)
	requireFileContent(t, filepath.Join(destination, "deployment.json"), "{}")
	require.NoDirExists(t, source)
}

func TestMigrate_MissingSourceIsNoOp(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	destination := filepath.Join(dir, "destination")

	// When
	moved, err := migrate(filepath.Join(dir, "source"), destination)

	// Then
	require.NoError(t, err)
	require.False(t, moved)
	require.NoFileExists(t, destination)
}

func TestMigrate_DestinationAlreadyExistsIsRefused(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	seedFile(t, source, "content")
	seedFile(t, destination, "existing content")

	// When
	_, err := migrate(source, destination)

	// Then
	require.ErrorIs(t, err, errDestinationExists)
	requireFileContent(t, source, "content")
	requireFileContent(t, destination, "existing content")
}

func TestMigrate_RenameFailureNamesBothPathsAndKeepsSource(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("relies on POSIX directory permissions enforced for a non-root user")
	}

	// Given a source whose parent directory refuses the rename
	dir := t.TempDir()
	sourceParent := filepath.Join(dir, "legacy")
	source := filepath.Join(sourceParent, "history")
	destination := filepath.Join(dir, "new", "history")
	seedFile(t, source, "some history")
	//nolint:gosec // remove write bit to force the rename to fail
	require.NoError(t, os.Chmod(sourceParent, 0o500))
	//nolint:gosec
	t.Cleanup(func() { _ = os.Chmod(sourceParent, 0o700) })

	// When
	_, err := migrate(source, destination)

	// Then
	require.ErrorContains(t, err, source)
	require.ErrorContains(t, err, destination)
	requireFileContent(t, source, "some history")
	require.NoFileExists(t, destination)
}

func TestMigrate_CrossFilesystemMoveAsksUserToMoveManually(t *testing.T) {
	t.Parallel()

	// Given
	if runtime.GOOS != "linux" {
		t.Skip("relies on /dev/shm being a separate mount")
	}
	//nolint:usetesting // t.TempDir is on the same filesystem as the source
	otherFilesystem, err := os.MkdirTemp("/dev/shm", "migrate")
	if err != nil {
		t.Skip("no writable /dev/shm")
	}
	t.Cleanup(func() { _ = os.RemoveAll(otherFilesystem) })
	source := filepath.Join(t.TempDir(), "history")
	destination := filepath.Join(otherFilesystem, "history")
	seedFile(t, source, "some history")

	// When
	_, err = migrate(source, destination)

	// Then
	if err == nil {
		t.Skip("temporary directory shares a filesystem with /dev/shm")
	}
	require.ErrorContains(t, err, "move it manually")
	require.ErrorContains(t, err, source)
	require.ErrorContains(t, err, destination)
	requireFileContent(t, source, "some history")
}
