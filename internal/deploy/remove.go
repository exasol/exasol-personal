// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
)

var ErrNotExasolPersonalDeploymentDirectory = errors.New(
	"not an Exasol Personal deployment directory",
)

var ErrDeploymentDirectoryRemovalUnsafe = errors.New(
	"deployment directory cannot be removed safely",
)

func RemoveLocalDeploymentDirectory(ctx context.Context, deployment config.DeploymentDir) error {
	if err := ensureDeploymentDirectoryRemovalIsSafe(deployment); err != nil {
		return err
	}

	slog.Info("removing local deployment directory", "path", deployment.Root())

	removeDeploymentDir := false
	if err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			if err := prepareDeploymentDirectoryRemoval(deployment); err != nil {
				return err
			}
			removeDeploymentDir = true

			return nil
		}); err != nil {
		return err
	}
	if removeDeploymentDir {
		if err := removeDeploymentDirectoryRoot(deployment); err != nil {
			return err
		}
	}

	slog.Info("removed local deployment directory", "path", deployment.Root())

	return nil
}

func prepareDeploymentDirectoryRemoval(deployment config.DeploymentDir) error {
	if err := ensureRemovableDeploymentDirectory(deployment); err != nil {
		return err
	}

	entries, err := os.ReadDir(deployment.Root())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if directorymutex.IsMarkerName(name) {
			continue
		}
		path := filepath.Join(deployment.Root(), name)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove local deployment path %q: %w", path, err)
		}
	}

	return nil
}

func ensureDeploymentDirectoryRemovalIsSafe(deployment config.DeploymentDir) error {
	if err := ensureRemovableDeploymentDirectory(deployment); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine current directory before local removal: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine launcher binary path before local removal: %w", err)
	}

	return validateDeploymentDirectoryRemovalContext(deployment, cwd, executable)
}

func validateDeploymentDirectoryRemovalContext(
	deployment config.DeploymentDir,
	cwd string,
	executable string,
) error {
	if pathIsInside(deployment.Root(), cwd) {
		return fmt.Errorf(
			"%w: current directory %q is inside deployment directory %q; "+
				"change to another directory and rerun",
			ErrDeploymentDirectoryRemovalUnsafe,
			cwd,
			deployment.Root(),
		)
	}
	if pathIsInside(deployment.Root(), executable) {
		return fmt.Errorf(
			"%w: launcher binary %q is inside deployment directory %q; "+
				"move the launcher binary and rerun",
			ErrDeploymentDirectoryRemovalUnsafe,
			executable,
			deployment.Root(),
		)
	}

	return nil
}

func pathIsInside(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)

	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}

func removeDeploymentDirectoryRoot(deployment config.DeploymentDir) error {
	if err := os.Remove(deployment.Root()); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf(
			"failed to remove local deployment directory %q: %w",
			deployment.Root(),
			err,
		)
	}

	return nil
}

func ensureRemovableDeploymentDirectory(deployment config.DeploymentDir) error {
	info, err := os.Stat(deployment.Root())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"%w: %q does not exist",
				ErrNotExasolPersonalDeploymentDirectory,
				deployment.Root(),
			)
		}

		return err
	}
	if !info.IsDir() {
		return fmt.Errorf(
			"%w: %q is not a directory",
			ErrNotExasolPersonalDeploymentDirectory,
			deployment.Root(),
		)
	}

	for _, path := range []string{
		deployment.ExasolPersonalStatePath(),
		deployment.DeploymentVersionMarkerPath(),
		deployment.Resolve(".workflowState.json"),
	} {
		if exists, err := regularFileExists(path); err != nil {
			return err
		} else if exists {
			return nil
		}
	}

	hasInfrastructureManifest, err := regularFileExists(deployment.InfrastructureManifestPath())
	if err != nil {
		return err
	}
	hasInstallManifest, err := regularFileExists(deployment.InstallManifestPath())
	if err != nil {
		return err
	}
	if hasInfrastructureManifest && hasInstallManifest {
		return nil
	}

	return fmt.Errorf(
		"%w: refusing to remove %q because it does not contain launcher state, "+
			"a launcher version marker, a legacy workflow marker, or extracted preset manifests",
		ErrNotExasolPersonalDeploymentDirectory,
		deployment.Root(),
	)
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return !info.IsDir(), nil
}
