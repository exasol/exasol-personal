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
	"syscall"

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
	if err := checkDeploymentNameCollisions(); err != nil {
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
// there, such as a symlink or a stray file, survives.
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
	names, err := deploymentNames(legacyRoot)
	if err != nil {
		return err
	}

	var actions []string
	for _, name := range names {
		path := filepath.Join(legacyRoot, name)
		mutex, err := directorymutex.New(path)
		if errors.Is(err, os.ErrNotExist) {
			// A concurrent invocation moved it after the listing.
			continue
		} else if err != nil {
			return err
		}
		status, err := mutex.Status()
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}

		if status.Locked {
			actions = append(actions, fmt.Sprintf(
				"close every exasol shell or other launcher command using %s, or run "+
					"`exasol diag unlock --deployment-dir %s` if none is running",
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

// checkDeploymentNameCollisions runs before any change, so a collision
// leaves every legacy item, including the cache, as found.
func checkDeploymentNameCollisions() error {
	newRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		return err
	}
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return err
	}
	// A concurrent rename adds a name to the new root only after removing it
	// from the legacy root, so listing the new root first never reports that
	// rename as a collision.
	newNames, err := deploymentNames(newRoot)
	if err != nil {
		return err
	}
	legacyNames, err := deploymentNames(legacyRoot)
	if err != nil {
		return err
	}

	var collisions []error
	for _, name := range legacyNames {
		if slices.Contains(newNames, name) {
			collisions = append(collisions, fmt.Errorf(
				"%w: %q (legacy: %s, new: %s); rename or remove one of them",
				errDeploymentNameCollision, name,
				filepath.Join(legacyRoot, name), filepath.Join(newRoot, name),
			))
		}
	}
	if err := errors.Join(collisions...); err != nil {
		return fmt.Errorf("migrate managed deployments: %w", err)
	}

	return nil
}

func deploymentNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("list deployments root %s: %w", root, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}

	return names, nil
}

// pathsExist checks paths in order. A concurrent rename creates its
// destination only after its source is gone, so listing the destination first
// never reports that rename as both paths existing.
func pathsExist(paths ...string) (bool, error) {
	for _, path := range paths {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return false, nil
		} else if err != nil {
			return false, fmt.Errorf("inspect %s: %w", path, err)
		}
	}

	return true, nil
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

func migrateLegacyDeployments() (string, error) {
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return "", err
	}
	names, err := deploymentNames(legacyRoot)
	if err != nil {
		return "", err
	}
	newRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		return "", err
	}

	message := ""
	for _, name := range names {
		moved, err := migrate(filepath.Join(legacyRoot, name), filepath.Join(newRoot, name))
		if err != nil {
			return message, fmt.Errorf("migrate managed deployments: deployment %q: %w", name, err)
		}
		if moved {
			message = "Migrated managed deployments to " + newRoot
		}
	}

	return message, nil
}

// migrate moves source to destination with a single rename, so an
// interruption leaves the data complete at exactly one of the two locations.
// It reports whether this call moved source.
func migrate(source, destination string) (bool, error) {
	if _, err := os.Lstat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("inspect migration source %s: %w", source, err)
	}

	if collides, err := pathsExist(destination, source); err != nil {
		return false, err
	} else if collides {
		return false, fmt.Errorf("%w: %s", errDestinationExists, destination)
	}

	if err := os.MkdirAll(filepath.Dir(destination), migrationDirPerm); err != nil {
		return false, fmt.Errorf("create parent directory for %s: %w", destination, err)
	}
	if err := os.Rename(source, destination); err != nil {
		if _, statErr := os.Lstat(source); errors.Is(statErr, os.ErrNotExist) {
			// A concurrent invocation moved it first.
			return false, nil
		}
		if errors.Is(err, syscall.EXDEV) {
			return false, fmt.Errorf(
				"move %s to %s: the locations are on different filesystems; "+
					"move it manually, then run the command again: %w",
				source, destination, err,
			)
		}

		return false, fmt.Errorf("move %s to %s: %w", source, destination, err)
	}

	return true, nil
}
