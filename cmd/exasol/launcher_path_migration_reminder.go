// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	pathMigrationReminderPrefix = ".launcher-path-migration-reminder-"
	pathMigrationReminderPerm   = 0o600
)

type pathMigrationReason int

const (
	migrationRunning pathMigrationReason = iota
	migrationLocked
	migrationStateUnknown
)

type pathMigrationDeferralError struct {
	legacyPath string
	newPath    string
	reason     pathMigrationReason
	cause      error
}

func (migration *pathMigrationDeferralError) Error() string {
	return "deployment layout migration is deferred: " + migration.legacyPath
}

type pathMigrationContextKey struct{}

func runStartupPathMigration(ctx context.Context, out io.Writer) context.Context {
	return context.WithValue(ctx, pathMigrationContextKey{}, runStartupMigration(ctx, out))
}

func queueDeferredPathMigrationCTAs(ctx context.Context, jsonOutput bool) {
	if !callsToActionVisible(jsonOutput) {
		return
	}
	deferred, ok := ctx.Value(pathMigrationContextKey{}).([]pathMigrationDeferralError)
	if !ok {
		return
	}
	for _, migration := range deferred {
		claimed, err := claimPathMigrationReminder(migration.legacyPath, time.Now())
		if err != nil {
			slog.Debug(
				"could not save layout migration reminder",
				"path",
				migration.legacyPath,
				"error",
				err,
			)
		}
		if claimed {
			addTerminalCallToAction(pathMigrationCTA(migration))
		}
	}
}

func pathMigrationCTA(migration pathMigrationDeferralError) string {
	selection := config.NewDeploymentDir(migration.legacyPath).CLISelection()
	var reason, action string
	switch migration.reason {
	case migrationRunning:
		reason = "the local deployment is running"
		action = "Stop it with `exasol stop" + selection + "` when convenient."
	case migrationLocked:
		reason = "a launcher operation holds its lock"
		action = "Close every exasol shell or other launcher command using it. " +
			"If none is running, clear the stale lock with `exasol diag unlock" + selection + "`."
	case migrationStateUnknown:
		reason = "its running state could not be checked"
		action = "Check the reported state-check error and run " +
			"`exasol diag local" + selection + "`."
	default:
		reason = "its migration is deferred"
		action = "Check the deployment state before moving it."
	}

	return fmt.Sprintf(
		"Migration pending for %s: launcher-managed deployments now belong at %s, but %s. "+
			"%s This deployment remains usable; the next invocation retries the move. "+
			"Support for the old location is temporary.",
		migration.legacyPath, migration.newPath, reason, action,
	)
}

func claimPathMigrationReminder(directory string, now time.Time) (bool, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	today := now.Format(time.DateOnly)
	for _, entry := range entries {
		if date, valid := pathMigrationReminderDate(entry); valid && date >= today {
			return false, nil
		}
	}
	marker := filepath.Join(directory, pathMigrationReminderPrefix+today)
	//nolint:gosec // The legacy directory is launcher-managed; the suffix is a local date.
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, pathMigrationReminderPerm)
	if os.IsExist(err) {
		//nolint:gosec // Inspect only the generated reminder path, without following symlinks.
		info, statErr := os.Lstat(marker)
		if statErr == nil && info.Mode().IsRegular() {
			return false, nil
		}

		return true, fmt.Errorf("invalid reminder marker %s", marker)
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	for _, entry := range entries {
		if date, valid := pathMigrationReminderDate(entry); valid && date < today {
			removePathMigrationReminder(filepath.Join(directory, entry.Name()))
		}
	}

	return true, nil
}

func pathMigrationReminderDate(entry os.DirEntry) (string, bool) {
	if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), pathMigrationReminderPrefix) {
		return "", false
	}
	date := strings.TrimPrefix(entry.Name(), pathMigrationReminderPrefix)
	_, err := time.Parse(time.DateOnly, date)

	return date, err == nil
}

func removePathMigrationReminders(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		slog.Debug("could not inspect migrated reminder state", "path", directory, "error", err)
		return
	}
	for _, entry := range entries {
		if _, valid := pathMigrationReminderDate(entry); valid {
			removePathMigrationReminder(filepath.Join(directory, entry.Name()))
		}
	}
}

func removePathMigrationReminder(path string) {
	//nolint:gosec // Callers select only validated date markers in a managed deployment.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Debug("could not remove layout migration reminder", "path", path, "error", err)
	}
}
