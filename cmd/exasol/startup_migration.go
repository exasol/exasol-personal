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
	"syscall"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
	"github.com/spf13/cobra"
)

const (
	migrationDirPerm = 0o700
	// Both legacy roots sit two levels below a launcher-owned .exasol
	// directory: .exasol/personal/deployments and .exasol/personal/runtime-artifacts.
	legacyDirDepth = 3
)

var (
	errDestinationExists       = errors.New("migration destination already exists")
	errDeploymentNameCollision = errors.New("a deployment with the same name exists")
	errLegacyDeploymentInUse   = errors.New("the deployment is in use")
)

var legacyDeploymentRunningFn = deploy.LocalDeploymentRunning

// runStartupMigration runs on every start, so an item it warns about and
// keeps at its legacy location moves on a later start.
func runStartupMigration(ctx context.Context, out io.Writer) {
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
	} {
		message, err := step()
		if message != "" {
			_, _ = fmt.Fprintln(out, message)
		}
		if err != nil {
			warnMigration(out, err)
		}
	}
	migrateLegacyDeployments(ctx, out)
	removeEmptyLegacyDirs()
}

func warnMigration(out io.Writer, err error) {
	_, _ = fmt.Fprintf(out, "Warning: %v\n", err)
}

func rawArgsRequestShellCompletion(args []string) bool {
	return len(args) > 0 && slices.Contains(
		[]string{cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd, "completion"},
		args[0],
	)
}

// removeEmptyLegacyDirs removes each legacy directory up to its .exasol
// ancestor. os.Remove refuses a non-empty directory, so anything still kept
// there, such as a deployment in use or a stray file, survives. The working
// directory is kept because removing it breaks relative paths and the user's
// shell.
func removeEmptyLegacyDirs() {
	cwd, cwdErr := os.Getwd()
	for _, legacyPath := range []func() (string, error){
		launcherpaths.LegacyDeploymentsRootPath,
		launcherpaths.LegacyCacheRootPath,
	} {
		path, err := legacyPath()
		if err != nil {
			continue
		}
		for range legacyDirDepth {
			if cwdErr != nil || !isWithin(cwd, path) {
				_ = os.Remove(path)
			}
			path = filepath.Dir(path)
		}
	}
}

func migrateLegacyDeployments(ctx context.Context, out io.Writer) {
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		warnMigration(out, err)

		return
	}
	names, err := deploymentNames(legacyRoot)
	if err != nil {
		warnMigration(out, err)

		return
	}
	newRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		warnMigration(out, err)

		return
	}

	var kept []error
	movedAny := false
	for _, name := range names {
		legacyPath := filepath.Join(legacyRoot, name)
		moved, err := migrateLegacyDeployment(ctx, legacyPath, filepath.Join(newRoot, name))
		if err != nil {
			kept = append(kept, fmt.Errorf("kept %s at its legacy location: %w", legacyPath, err))
		}
		movedAny = movedAny || moved
	}
	if movedAny {
		_, _ = fmt.Fprintln(out, "Migrated managed deployments to "+newRoot)
	}
	for _, err := range kept {
		warnMigration(out, err)
	}
}

// migrateLegacyDeployment leaves a deployment in use where it is, because
// renaming it under a running database or another launcher process leaves
// that process working against the legacy path.
func migrateLegacyDeployment(ctx context.Context, legacyPath, newPath string) (bool, error) {
	if collides, err := pathsExist(newPath, legacyPath); err != nil {
		return false, err
	} else if collides {
		return false, fmt.Errorf(
			"%w at %s; rename or remove one of them",
			errDeploymentNameCollision, newPath,
		)
	}

	mutex, err := directorymutex.New(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		// A concurrent invocation moved it after the listing.
		return false, nil
	} else if err != nil {
		return false, err
	}
	status, err := mutex.Status()
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if status.Locked {
		return false, fmt.Errorf(
			"%w; close every exasol shell or other launcher command using it, or run "+
				"`exasol diag unlock --deployment-dir %s` if none is running",
			errLegacyDeploymentInUse, legacyPath,
		)
	}

	running, err := legacyDeploymentRunningFn(ctx, config.NewDeploymentDir(legacyPath))
	if err != nil {
		return false, fmt.Errorf("check whether the deployment is running: %w", err)
	}
	if running {
		return false, fmt.Errorf(
			"%w; stop it with `exasol stop --deployment-dir %s`",
			errLegacyDeploymentInUse, legacyPath,
		)
	}

	return migrate(legacyPath, newPath)
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
					"move it manually: %w",
				source, destination, err,
			)
		}

		return false, fmt.Errorf("move %s to %s: %w", source, destination, err)
	}

	return true, nil
}
