// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func runStartupMigrationForTest(t *testing.T) (string, error) {
	t.Helper()

	var out strings.Builder
	err := runStartupMigration(context.Background(), &out)

	return out.String(), err
}

func fakeLegacyDeploymentRunning(t *testing.T, running bool) {
	t.Helper()

	original := legacyDeploymentRunningFn
	legacyDeploymentRunningFn = func(context.Context, config.DeploymentDir) bool { return running }
	t.Cleanup(func() { legacyDeploymentRunningFn = original })
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_AppliesChangesAutomaticallyWithoutPrompting(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	seedLegacyDeployment(t, "default")

	// When nothing supplies interactive input
	_, err := runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(newDeploymentPath(t, "default"), "deployment.json"))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ReportsWhatItChanged(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	seedLegacyDeployment(t, "default")

	// When
	out, err := runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
	newRoot := filepath.Dir(newDeploymentPath(t, "default"))
	require.Contains(t, out, "Migrated managed deployments to "+newRoot)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_StaysQuietWhenNothingToMigrate(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())

	// When
	out, err := runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
	require.Empty(t, out)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_SkipsAlreadyCompletedLocation(t *testing.T) {
	// Given a completed migration and a deployment that reappears at the legacy root
	setTestHome(t, t.TempDir())
	seedLegacyDeployment(t, "default")
	_, err := runStartupMigrationForTest(t)
	require.NoError(t, err)
	legacyPath := seedLegacyDeployment(t, "staging")

	// When
	out, err := runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
	require.Empty(t, out)
	require.DirExists(t, legacyPath)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_ResumesInterruptedMigration(t *testing.T) {
	// Given an earlier run moved "default" but stopped before "staging"
	setTestHome(t, t.TempDir())
	legacyPath := seedLegacyDeployment(t, "staging")
	require.NoError(t, os.MkdirAll(newDeploymentPath(t, "default"), 0o700))

	// When
	_, err := runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
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
	_, err = runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
	newPath, err := launcherpaths.ResourceConfigFilePath()
	require.NoError(t, err)
	content, err := os.ReadFile(newPath)
	require.NoError(t, err)
	require.Equal(t, "retention_days: 45\n", string(content))
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
	out, err := runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
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
	_, err = runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
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
	_, err = runStartupMigrationForTest(t)

	// Then
	require.NoError(t, err)
	require.FileExists(t, notes)
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_BlocksWhileLegacyLocalDeploymentRuns(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	fakeLegacyDeploymentRunning(t, true)
	legacyPath := seedLegacyDeployment(t, "default")

	// When
	_, err := runStartupMigrationForTest(t)

	// Then
	require.ErrorIs(t, err, errMigrationBlocked)
	require.ErrorContains(t, err, "exasol stop --deployment-dir "+legacyPath)
	require.DirExists(t, legacyPath)
	require.False(t, commandClearsMigrationBlock([]string{"status"}))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_LetsStopAndUnlockRunWhileBlocked(t *testing.T) {
	for _, command := range [][]string{{"stop"}, {"diag", "unlock"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			// Given
			setTestHome(t, t.TempDir())
			fakeLegacyDeploymentRunning(t, true)
			legacyPath := seedLegacyDeployment(t, "default")

			args := slices.Concat(command, []string{"--deployment-dir", legacyPath})

			// When
			_, err := runStartupMigrationForTest(t)

			// Then
			require.ErrorIs(t, err, errMigrationBlocked)
			require.True(t, commandClearsMigrationBlock(args))
			require.DirExists(t, legacyPath)
		})
	}
}

//nolint:paralleltest // home-directory environment is process-global.
func TestRunStartupMigration_BlocksWhileLegacyDeploymentIsLocked(t *testing.T) {
	// Given
	setTestHome(t, t.TempDir())
	legacyPath := seedLegacyDeployment(t, "default")
	mutex, err := directorymutex.New(legacyPath)
	require.NoError(t, err)
	require.NoError(t, mutex.AcquireExclusive(context.Background()))
	t.Cleanup(func() { _ = mutex.ReleaseExclusive(context.Background()) })

	// When
	_, err = runStartupMigrationForTest(t)

	// Then
	require.ErrorIs(t, err, errMigrationBlocked)
	require.ErrorContains(t, err, "exasol diag unlock --deployment-dir "+legacyPath)
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
func TestRunStartupMigration_KeepsBothDeploymentsOnNameCollision(t *testing.T) {
	// Given "default" at both roots and "staging" only at the legacy root
	setTestHome(t, t.TempDir())
	legacyDefault := seedLegacyDeployment(t, "default")
	legacyStaging := seedLegacyDeployment(t, "staging")
	newDefault := newDeploymentPath(t, "default")
	seedFile(t, filepath.Join(newDefault, "deployment.json"), "new")

	// When
	_, err := runStartupMigrationForTest(t)

	// Then
	require.ErrorIs(t, err, errDeploymentNameCollision)
	require.ErrorContains(t, err, legacyDefault)
	require.ErrorContains(t, err, newDefault)
	requireFileContent(t, filepath.Join(legacyDefault, "deployment.json"), "{}")
	requireFileContent(t, filepath.Join(newDefault, "deployment.json"), "new")
	require.NoDirExists(t, legacyStaging)
	require.DirExists(t, newDeploymentPath(t, "staging"))
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
