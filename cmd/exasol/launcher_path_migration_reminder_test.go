// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaimPathMigrationReminder_UsesLocalCalendarDate(t *testing.T) {
	t.Parallel()

	// Given a deployment used on both sides of midnight in its local timezone
	directory := t.TempDir()
	before := time.Date(2026, time.November, 17, 23, 59, 0, 0, time.FixedZone("local", 2*60*60))
	after := before.Add(2 * time.Minute)
	otherFile := filepath.Join(directory, ".keep")
	require.NoError(t, os.WriteFile(otherFile, nil, 0o600))

	// When separate invocations claim the same day, the next day and a rolled-back clock
	first, err := claimPathMigrationReminder(directory, before)
	require.NoError(t, err)
	repeated, err := claimPathMigrationReminder(directory, before)
	require.NoError(t, err)
	nextDay, err := claimPathMigrationReminder(directory, after)
	require.NoError(t, err)
	rolledBack, err := claimPathMigrationReminder(directory, before)
	require.NoError(t, err)

	// Then midnight permits another reminder while the saved future date suppresses rollback
	require.True(t, first)
	require.False(t, repeated)
	require.True(t, nextDay)
	require.False(t, rolledBack)
	require.FileExists(t, filepath.Join(directory, pathMigrationReminderPrefix+"2026-11-18"))
	require.NoFileExists(t, filepath.Join(directory, pathMigrationReminderPrefix+"2026-11-17"))
	require.FileExists(t, otherFile)
}

func TestClaimPathMigrationReminder_InvalidStateAndSeparateDeployments(t *testing.T) {
	t.Parallel()

	// Given malformed migration state and an unrelated file
	first, second := t.TempDir(), t.TempDir()
	now := time.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC)
	invalid := filepath.Join(first, pathMigrationReminderPrefix+"not-a-date")
	require.NoError(t, os.WriteFile(invalid, nil, 0o600))

	// When each deployment claims a reminder
	for _, directory := range []string{first, second} {
		claimed, err := claimPathMigrationReminder(directory, now)
		require.NoError(t, err)
		require.True(t, claimed)
	}

	// Then invalid state did not suppress guidance or cross deployment boundaries
	require.FileExists(t, invalid)
}

func TestClaimPathMigrationReminder_InvalidMarkerPermitsBestEffort(t *testing.T) {
	t.Parallel()

	// Given a directory where today's regular reminder file should be
	directory := t.TempDir()
	now := time.Now()
	require.NoError(
		t,
		os.Mkdir(
			filepath.Join(directory, pathMigrationReminderPrefix+now.Format(time.DateOnly)),
			0o700,
		),
	)

	// When an invocation tries to claim its daily reminder
	claimed, err := claimPathMigrationReminder(directory, now)

	// Then it permits best-effort guidance and reports the persistence problem to its caller
	require.True(t, claimed)
	require.Error(t, err)
}

//nolint:gosec // Exercise directory write permission failures.
func TestClaimPathMigrationReminder_UnwritableDirectoryPermitsBestEffort(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory write permissions do not enforce this scenario on this host")
	}

	// Given a readable deployment directory without write permission
	directory := t.TempDir()
	require.NoError(
		t,
		os.Chmod(directory, 0o500),
	) //nolint:gosec // Directory needs read and traverse permission.
	t.Cleanup(
		func() { _ = os.Chmod(directory, 0o700) },
	) //nolint:gosec // Restore directory permissions.

	// When a reminder is due
	claimed, err := claimPathMigrationReminder(directory, time.Now())

	// Then persistence failure does not suppress best-effort guidance
	require.True(t, claimed)
	require.Error(t, err)
}

func TestClaimPathMigrationReminder_MissingDeploymentStaysMissing(t *testing.T) {
	t.Parallel()

	// Given a legacy path another invocation has already moved
	directory := filepath.Join(t.TempDir(), "moved")

	// When reminder recording reaches that path
	claimed, err := claimPathMigrationReminder(directory, time.Now())

	// Then it neither claims guidance nor recreates the old deployment directory
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoDirExists(t, directory)
}

func TestClaimPathMigrationReminder_ConcurrentProcesses(t *testing.T) {
	t.Parallel()

	// Given independent launcher processes sharing one deployment directory
	directory := t.TempDir()
	executable, err := os.Executable()
	require.NoError(t, err)
	commands := make([]*exec.Cmd, 8)
	outputs := make([]strings.Builder, len(commands))
	for index := range commands {
		command := exec.CommandContext(
			t.Context(),
			executable,
			"-test.run=^TestPathMigrationReminderProcess$",
		)
		command.Env = append(os.Environ(), "EXASOL_TEST_REMINDER_DIRECTORY="+directory)
		command.Stdout = &outputs[index]
		command.Stderr = &outputs[index]
		commands[index] = command
	}

	// When all processes claim the same date
	for _, command := range commands {
		require.NoError(t, command.Start())
	}
	claimed := 0
	for index, command := range commands {
		require.NoError(t, command.Wait(), outputs[index].String())
		if strings.Contains(outputs[index].String(), "claimed reminder") {
			claimed++
		}
	}

	// Then exactly one process presents the daily reminder
	require.Equal(t, 1, claimed)
}

func TestPathMigrationReminderProcess(t *testing.T) {
	t.Parallel()
	directory := os.Getenv("EXASOL_TEST_REMINDER_DIRECTORY")
	if directory == "" {
		return
	}
	// Given the shared directory provided by the parent process
	now := time.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC)

	// When this independent process claims the day's reminder
	claimed, err := claimPathMigrationReminder(directory, now)

	// Then the parent can count successful claims
	require.NoError(t, err)
	if claimed {
		_, err = fmt.Fprintln(os.Stdout, "claimed reminder")
		require.NoError(t, err)
	}
}

//nolint:paralleltest // terminal messages are process-global.
func TestQueueDeferredPathMigrationCTAs_JSONPreservesTextReminder(t *testing.T) {
	// Given a deployment with a deferred migration and no daily claim
	resetTerminalMessages()
	t.Cleanup(resetTerminalMessages)
	directory := t.TempDir()
	pending := pathMigrationDeferralError{
		legacyPath: directory,
		newPath:    directory + "-new",
		reason:     migrationRunning,
	}
	ctx := context.WithValue(
		context.Background(),
		pathMigrationContextKey{},
		[]pathMigrationDeferralError{pending},
	)

	// When a JSON invocation runs before text
	queueDeferredPathMigrationCTAs(ctx, true)
	require.Empty(t, terminalCallsToAction)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries)
	queueDeferredPathMigrationCTAs(ctx, false)

	// Then text still receives one CTA and a repeated invocation receives none
	require.Len(t, terminalCallsToAction, 1)
	require.Contains(t, terminalCallsToAction[0], "Support for the old location is temporary")
	resetTerminalMessages()
	queueDeferredPathMigrationCTAs(ctx, false)
	require.Empty(t, terminalCallsToAction)
}

func TestPathMigrationCTA_QuotesPathsAndConditionsUnlocking(t *testing.T) {
	t.Parallel()

	// Given a locked deployment whose path requires shell quoting
	pending := pathMigrationDeferralError{
		legacyPath: "/home/user's deployments/prod",
		newPath:    "/home/current/prod",
		reason:     migrationLocked,
	}

	// When its migration guidance is rendered
	message := pathMigrationCTA(pending)

	// Then it names both paths and offers unlock only after operations have stopped
	require.Contains(t, message, pending.legacyPath)
	require.Contains(t, message, pending.newPath)
	require.Contains(t, message, "If no exasol launcher of any version is running")
	require.Contains(t, message, "--deployment-dir '/home/user'\\''s deployments/prod'")
}

//nolint:paralleltest // running-state hook and home-directory environment are process-global.
func TestRunStartupMigration_RetiresReminderAfterMoving(t *testing.T) {
	// Given a stopped legacy deployment with reminder and unrelated state
	setTestHome(t, t.TempDir())
	fakeLegacyDeploymentRunning(t, false, nil)
	legacy := seedLegacyDeployment(t, "prod")
	claimed, err := claimPathMigrationReminder(legacy, time.Now())
	require.NoError(t, err)
	require.True(t, claimed)
	seedFile(t, filepath.Join(legacy, ".keep"), "unrelated")

	// When migration runs despite today's reminder
	runStartupMigrationForTest(t)

	// Then only this migration's valid reminder state is retired
	current := newDeploymentPath(t, "prod")
	entries, err := os.ReadDir(current)
	require.NoError(t, err)
	for _, entry := range entries {
		_, marker := pathMigrationReminderDate(entry)
		require.False(t, marker)
	}
	requireFileContent(t, filepath.Join(current, ".keep"), "unrelated")
	require.FileExists(t, filepath.Join(current, "deployment.json"))
}
