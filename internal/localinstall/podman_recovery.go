// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	initialCreateMarkerName = ".exanano-initial-create-in-progress"
	quarantineTimeFormat    = "20060102150405"
)

var nanoTLSFiles = []string{
	filepath.Join("certificates", "fullchain.pem"),
	filepath.Join("certificates", "privkey.pem"),
}

func (install *PodmanInstall) recoverIncompleteInitialCreate(
	ctx context.Context,
	out, outErr io.Writer,
	containerName, dataDir string,
) error {
	markerPath := filepath.Join(dataDir, initialCreateMarkerName)
	if _, err := os.Stat(markerPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to inspect Nano initial-create marker %s: %w", markerPath, err)
	}

	exists, err := install.containerExists(ctx, outErr, containerName)
	if err != nil {
		return err
	}
	if exists {
		if err := install.runCmd(ctx, out, outErr, "podman", "stop", containerName); err != nil {
			return install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to stop incomplete Nano container %s: %w", containerName, err))
		}
		if err := install.runCmd(
			ctx, out, outErr, "podman", "rm", "--force", "--ignore", containerName,
		); err != nil {
			return install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to remove incomplete Nano container %s: %w", containerName, err))
		}
	}

	quarantinePath, err := nextQuarantinePath(dataDir, time.Now())
	if err != nil {
		return err
	}
	if err := os.Rename(dataDir, quarantinePath); err != nil {
		return fmt.Errorf(
			"failed to quarantine incomplete Nano data at %s: %w",
			quarantinePath,
			err,
		)
	}
	if err := os.MkdirAll(dataDir, dataDirMode); err != nil {
		return fmt.Errorf("failed to recreate Nano data directory %s: %w", dataDir, err)
	}

	return nil
}

func nextQuarantinePath(dataDir string, now time.Time) (string, error) {
	base := dataDir + ".failed-" + now.Format(quarantineTimeFormat)
	candidate := base
	for suffix := 1; ; suffix++ {
		_, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("failed to inspect Nano quarantine path %s: %w", candidate, err)
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func removeStaleNanoTLSFiles(dataDir string) error {
	for _, relativePath := range nanoTLSFiles {
		path := filepath.Join(dataDir, relativePath)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove stale Nano TLS file %s: %w", path, err)
		}
	}

	return nil
}
