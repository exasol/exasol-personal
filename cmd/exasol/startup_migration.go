// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/spf13/cobra"
)

const (
	migrationDirPerm        = 0o700
	migrationMarkerFilePerm = 0o600
	// Both legacy roots sit two levels below a launcher-owned .exasol
	// directory: .exasol/personal/deployments and .exasol/personal/runtime-artifacts.
	legacyDirDepth = 3
)

var (
	errMigrationBlocked = errors.New(
		"launcher files move to their new locations only while no legacy deployment is in use",
	)
	errDestinationExists       = errors.New("migration destination already exists")
	errDeploymentNameCollision = errors.New(
		"deployment name exists at both the legacy and new managed-deployments root",
	)
)

var legacyDeploymentRunningFn = deploy.LocalDeploymentRunning

// runStartupMigration runs once, gated by a completion marker; a failure
// leaves the marker unset so the next invocation retries.
func runStartupMigration(ctx context.Context, out io.Writer) error {
	markerPath, err := launcherpaths.MigrationMarkerPath()
	if err != nil {
		return err
	}
	if done, err := pathExists(markerPath); err != nil || done {
		return err
	}

	if err := checkLegacyDeploymentsIdle(ctx); err != nil {
		return err
	}

	for _, step := range []func() (string, error){
		moveLegacyFileStep(
			"cache configuration",
			launcherpaths.LegacyResourceConfigFilePath,
			launcherpaths.ResourceConfigFilePath,
		),
		deleteLegacyCache,
		moveLegacyFileStep(
			"SQL history",
			launcherpaths.LegacyHistoryFilePath,
			launcherpaths.HistoryFilePath,
		),
		migrateLegacyDeployments,
	} {
		message, err := step()
		if message != "" {
			_, _ = fmt.Fprintln(out, message)
		}
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(markerPath), migrationDirPerm); err != nil {
		return fmt.Errorf("create directory for migration marker %s: %w", markerPath, err)
	}

	if err := os.WriteFile(markerPath, nil, migrationMarkerFilePerm); err != nil {
		return err
	}
	removeEmptyLegacyDirs()

	return nil
}

func rawArgsRequestShellCompletion(args []string) bool {
	return len(args) > 0 && slices.Contains(
		[]string{cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd, "completion"},
		args[0],
	)
}

// stop and diag unlock are how the user clears a migration block, so they run
// against the legacy directory without migrating.
func commandClearsMigrationBlock(args []string) bool {
	return preregisteredCommandIs(args, stopCmd) || preregisteredCommandIs(args, unlockCmd)
}

// removeEmptyLegacyDirs removes each legacy directory up to its .exasol
// ancestor. os.Remove refuses a non-empty directory, so anything still kept
// there, such as a colliding deployment, survives.
func removeEmptyLegacyDirs() {
	for _, legacyPath := range []func() (string, error){
		launcherpaths.LegacyDeploymentsRootPath,
		launcherpaths.LegacyCacheRootPath,
	} {
		path, err := legacyPath()
		if err != nil {
			continue
		}
		for range legacyDirDepth {
			_ = os.Remove(path)
			path = filepath.Dir(path)
		}
	}
}

// checkLegacyDeploymentsIdle runs before any move, because renaming a
// deployment that a running database or another launcher process uses leaves
// that process working against the legacy path.
func checkLegacyDeploymentsIdle(ctx context.Context) error {
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(legacyRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("list legacy deployments root %s: %w", legacyRoot, err)
	}

	var actions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(legacyRoot, entry.Name())
		mutex, err := directorymutex.New(path)
		if err != nil {
			return err
		}
		status, err := mutex.Status()
		if err != nil {
			return err
		}

		if status.Locked {
			actions = append(actions, fmt.Sprintf(
				"wait for the operation in %s to finish, or run "+
					"`exasol diag unlock --deployment-dir %s` if no launcher is running",
				path, path,
			))
		} else if legacyDeploymentRunningFn(ctx, config.NewDeploymentDir(path)) {
			actions = append(actions, "exasol stop --deployment-dir "+path)
		}
	}
	if len(actions) == 0 {
		return nil
	}

	return fmt.Errorf("%w; first:\n  %s", errMigrationBlocked, strings.Join(actions, "\n  "))
}

// moveLegacyFileStep keeps a file the current launcher already wrote at the new
// path over the legacy one.
func moveLegacyFileStep(
	what string,
	legacyPath, newPath func() (string, error),
) func() (string, error) {
	return func() (string, error) {
		legacy, err := legacyPath()
		if err != nil {
			return "", err
		}
		destination, err := newPath()
		if err != nil {
			return "", err
		}

		moved, err := migrate(legacy, destination)
		if errors.Is(err, errDestinationExists) || (err == nil && !moved) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("migrate %s: %w", what, err)
		}

		return fmt.Sprintf("Migrated %s to %s", what, destination), nil
	}
}

func deleteLegacyCache() (string, error) {
	legacyRoot, err := launcherpaths.LegacyCacheRootPath()
	if err != nil {
		return "", err
	}

	exists, err := pathExists(legacyRoot)
	if err != nil || !exists {
		return "", err
	}
	if err := os.RemoveAll(legacyRoot); err != nil {
		return "", fmt.Errorf("delete legacy resource cache %s: %w", legacyRoot, err)
	}

	return "Deleted legacy resource cache at " + legacyRoot, nil
}

// migrateLegacyDeployments reports each name collision but still migrates
// every other deployment in the same run.
func migrateLegacyDeployments() (string, error) {
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return "", err
	}
	newRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(legacyRoot)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("list legacy deployments root %s: %w", legacyRoot, err)
	}

	message := ""
	var collisions []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		legacyPath := filepath.Join(legacyRoot, name)
		newPath := filepath.Join(newRoot, name)
		_, err := migrate(legacyPath, newPath)
		if errors.Is(err, errDestinationExists) {
			collisions = append(collisions, fmt.Errorf(
				"%w: %q (legacy: %s, new: %s); rename or remove one of them",
				errDeploymentNameCollision, name, legacyPath, newPath,
			))

			continue
		} else if err != nil {
			return message, fmt.Errorf("migrate managed deployments: deployment %q: %w", name, err)
		}
		message = "Migrated managed deployments to " + newRoot
	}
	if err := errors.Join(collisions...); err != nil {
		return message, fmt.Errorf("migrate managed deployments: %w", err)
	}

	return message, nil
}

// migrate moves source to destination with a single rename, so an
// interruption leaves the data complete at exactly one of the two locations.
// It reports whether source existed and was moved.
func migrate(source, destination string) (bool, error) {
	if _, err := os.Lstat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("inspect migration source %s: %w", source, err)
	}

	if _, err := os.Lstat(destination); err == nil {
		return false, fmt.Errorf("%w: %s", errDestinationExists, destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect migration destination %s: %w", destination, err)
	}

	if err := os.MkdirAll(filepath.Dir(destination), migrationDirPerm); err != nil {
		return false, fmt.Errorf("create parent directory for %s: %w", destination, err)
	}
	if err := os.Rename(source, destination); err != nil {
		return false, fmt.Errorf("move %s to %s: %w", source, destination, err)
	}

	return true, nil
}
