// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
)

const podmanBindMountType = "bind"

type podmanMount struct {
	//nolint:tagliatelle // Podman inspect emits this exact field name.
	Type string `json:"Type"`
	//nolint:tagliatelle // Podman inspect emits this exact field name.
	Source string `json:"Source"`
	//nolint:tagliatelle // Podman inspect emits this exact field name.
	Destination string `json:"Destination"`
}

func (install *PodmanInstall) migrateOverlayDataIfNeeded(
	ctx context.Context,
	out, outErr io.Writer,
	containerName, dataDir string,
	containerStatus podmanContainerStatus,
) (bool, error) {
	if !containerStatus.Exists {
		return false, nil
	}
	persistent, err := install.containerHasPersistentExaMount(ctx, outErr, containerName)
	if err != nil {
		return false, install.failureWithDiagnostics(ctx, outErr, containerName, err)
	}
	if persistent {
		return false, nil
	}

	populated, err := install.directoryHasEntries(ctx, dataDir)
	if err != nil {
		return false, err
	}
	if populated {
		return false, fmt.Errorf(
			"refusing to overwrite populated persistent Nano data at %s while "+
				"migrating %s:/exa; inspect both locations and merge them manually "+
				"before removing the legacy container",
			dataDir,
			containerName,
		)
	}

	if err := install.runPodman(ctx, out, outErr, "stop", containerName); err != nil {
		return false, install.failureWithDiagnostics(
			ctx, outErr, containerName,
			fmt.Errorf(
				"failed to stop legacy Nano container %s before migration: %w",
				containerName, err,
			),
		)
	}
	if err := install.environment.MkdirAll(ctx, filepath.Dir(dataDir), dataDirMode); err != nil {
		return false, fmt.Errorf("failed to prepare Nano migration directory: %w", err)
	}
	stagingDir, err := install.environment.MkdirTemp(
		ctx,
		filepath.Dir(dataDir), "."+filepath.Base(dataDir)+".overlay-migration-*",
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to create Nano overlay migration staging directory: %w", err,
		)
	}

	if err := install.runPodman(
		ctx,
		out,
		outErr,
		"cp",
		containerName+":"+nanoDataMountPath+"/.",
		stagingDir,
	); err != nil {
		_ = install.environment.RemoveAll(ctx, stagingDir)

		return false, fmt.Errorf(
			"failed to copy legacy Nano data from %s:/exa; the stopped container "+
				"was retained. Retry with 'podman cp %s:/exa/. %s/': %w",
			containerName,
			containerName,
			dataDir,
			err,
		)
	}

	stagedData, err := install.directoryHasEntries(ctx, stagingDir)
	if err != nil {
		return false, fmt.Errorf(
			"failed to inspect copied Nano data in %s; the legacy container and "+
				"staging directory were retained: %w",
			stagingDir,
			err,
		)
	}
	if stagedData {
		if err := install.installOverlayDataAtomically(ctx, dataDir, stagingDir); err != nil {
			return false, fmt.Errorf(
				"failed to install migrated Nano data from %s; the legacy container "+
					"and staging directory were retained: %w",
				stagingDir,
				err,
			)
		}
	} else if err := install.environment.RemoveDir(ctx, stagingDir); err != nil {
		return false, fmt.Errorf("failed to remove empty Nano migration staging directory: %w", err)
	}

	if err := install.runPodman(ctx, out, outErr, "rm", containerName); err != nil {
		return false, install.failureWithDiagnostics(ctx, outErr, containerName,
			fmt.Errorf(
				"migrated Nano data is installed at %s, but the legacy container %s "+
					"could not be removed; verify the data and remove that container "+
					"before retrying: %w",
				dataDir,
				containerName,
				err,
			))
	}

	return true, nil
}

// containerHasPersistentExaMount reports whether the container keeps /exa on a
// bind mount, meaning its data already lives outside the container's overlay
// filesystem and needs no migration.
//
// It deliberately does not compare the mount source against the deployment's
// data directory. Podman reports the source as the container engine sees it,
// which on Windows is the path inside the Podman machine
// (/mnt/c/Users/... for C:\Users\...), so no amount of path cleaning makes
// the two comparable. Treating that mismatch as "not persistent" made every
// Windows container look like a legacy overlay one, and the migration then
// refused to overwrite the deployment's own data.
//
// A source that differs from the current data directory is not a problem
// either way: the container is recreated against the configured directory on
// every start, so the mount is corrected without copying anything.
func (install *PodmanInstall) containerHasPersistentExaMount(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	output, err := install.runPodmanOutput(
		ctx,
		nil,
		outErr,
		"container",
		"inspect",
		"--format",
		"{{json .Mounts}}",
		containerName,
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to inspect mounts for Nano container %s: %w", containerName, err,
		)
	}
	var mounts []podmanMount
	if err := json.Unmarshal([]byte(output), &mounts); err != nil {
		return false, fmt.Errorf(
			"failed to parse mounts for Nano container %s: %w", containerName, err,
		)
	}
	for _, mount := range mounts {
		if mount.Destination == nanoDataMountPath && mount.Type == podmanBindMountType {
			slog.Debug("Nano container has a bind-mounted /exa; skipping overlay migration",
				"container", containerName, "source", mount.Source)

			return true, nil
		}
	}

	return false, nil
}

func (install *PodmanInstall) directoryHasEntries(
	ctx context.Context,
	path string,
) (bool, error) {
	populated, err := install.environment.DirectoryHasEntries(ctx, path)
	if err != nil {
		return false, fmt.Errorf("failed to inspect directory %s: %w", path, err)
	}

	return populated, nil
}

func (install *PodmanInstall) installOverlayDataAtomically(
	ctx context.Context,
	dataDir, stagingDir string,
) error {
	populated, err := install.directoryHasEntries(ctx, dataDir)
	if err != nil {
		return err
	}
	if populated {
		return fmt.Errorf(
			"persistent Nano data directory %s became populated during migration", dataDir,
		)
	}
	if err := install.environment.RemoveDir(ctx, dataDir); err != nil {
		return fmt.Errorf("failed to remove empty Nano data directory %s: %w", dataDir, err)
	}
	if err := install.environment.Rename(ctx, stagingDir, dataDir); err != nil {
		if mkdirErr := install.environment.MkdirAll(ctx, dataDir, dataDirMode); mkdirErr != nil {
			return fmt.Errorf(
				"failed to atomically install Nano data: %w "+
					"(also failed to recreate %s: %w)",
				err,
				dataDir,
				mkdirErr,
			)
		}

		return fmt.Errorf("failed to atomically install Nano data: %w", err)
	}

	return nil
}
