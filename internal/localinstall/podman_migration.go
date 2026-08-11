// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type podmanMount struct {
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
	hasMount, err := install.containerHasDataMount(ctx, outErr, containerName)
	if err != nil {
		return false, install.failureWithDiagnostics(ctx, outErr, containerName, err)
	}
	if hasMount {
		return false, nil
	}

	populated, err := directoryHasEntries(dataDir)
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

	if err := install.runCmd(ctx, out, outErr, "stop", containerName); err != nil {
		return false, install.failureWithDiagnostics(
			ctx, outErr, containerName,
			fmt.Errorf(
				"failed to stop legacy Nano container %s before migration: %w",
				containerName, err,
			),
		)
	}
	if err := os.MkdirAll(filepath.Dir(dataDir), dataDirMode); err != nil {
		return false, fmt.Errorf("failed to prepare Nano migration directory: %w", err)
	}
	stagingDir, err := os.MkdirTemp(
		filepath.Dir(dataDir), "."+filepath.Base(dataDir)+".overlay-migration-*",
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to create Nano overlay migration staging directory: %w", err,
		)
	}

	if err := install.runCmd(
		ctx,
		out,
		outErr,
		"cp",
		containerName+":/exa/.",
		stagingDir,
	); err != nil {
		_ = os.RemoveAll(stagingDir)

		return false, fmt.Errorf(
			"failed to copy legacy Nano data from %s:/exa; the stopped container "+
				"was retained. Retry with 'podman cp %s:/exa/. %s/': %w",
			containerName,
			containerName,
			dataDir,
			err,
		)
	}

	stagedData, err := directoryHasEntries(stagingDir)
	if err != nil {
		return false, fmt.Errorf(
			"failed to inspect copied Nano data in %s; the legacy container and "+
				"staging directory were retained: %w",
			stagingDir,
			err,
		)
	}
	if stagedData {
		if err := installOverlayDataAtomically(dataDir, stagingDir); err != nil {
			return false, fmt.Errorf(
				"failed to install migrated Nano data from %s; the legacy container "+
					"and staging directory were retained: %w",
				stagingDir,
				err,
			)
		}
	} else if err := os.Remove(stagingDir); err != nil {
		return false, fmt.Errorf("failed to remove empty Nano migration staging directory: %w", err)
	}

	if err := install.runCmd(ctx, out, outErr, "rm", containerName); err != nil {
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

func (install *PodmanInstall) containerHasDataMount(
	ctx context.Context,
	outErr io.Writer,
	containerName string,
) (bool, error) {
	output, err := install.runCmdOutput(
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
		if mount.Destination == "/exa" {
			return true, nil
		}
	}

	return false, nil
}

func directoryHasEntries(path string) (bool, error) {
	directory, err := os.Open(path) //nolint:gosec // launcher-owned deployment path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("failed to inspect directory %s: %w", path, err)
	}
	defer func() { _ = directory.Close() }()

	_, err = directory.Readdirnames(1)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}

	return false, fmt.Errorf("failed to inspect directory %s: %w", path, err)
}

func installOverlayDataAtomically(dataDir, stagingDir string) error {
	populated, err := directoryHasEntries(dataDir)
	if err != nil {
		return err
	}
	if populated {
		return fmt.Errorf(
			"persistent Nano data directory %s became populated during migration", dataDir,
		)
	}
	if err := os.Remove(dataDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove empty Nano data directory %s: %w", dataDir, err)
	}
	if err := os.Rename(stagingDir, dataDir); err != nil {
		if mkdirErr := os.MkdirAll(dataDir, dataDirMode); mkdirErr != nil {
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
