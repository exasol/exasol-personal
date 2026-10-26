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
	"strings"
	"time"
)

const (
	initialCreateMarkerName = ".exanano-initial-create-in-progress"
	quarantineSuffix        = ".failed-"
	quarantineTimeFormat    = "20060102150405"
)

var nanoTLSFiles = []string{
	filepath.Join("certificates", "fullchain.pem"),
	filepath.Join("certificates", "privkey.pem"),
}

func (install *NanoInstall) recoverIncompleteInitialCreate(
	ctx context.Context,
	out, outErr io.Writer,
	containerName, dataDir string,
	containerStatus podmanContainerStatus,
) (bool, error) {
	markerPath := filepath.Join(dataDir, initialCreateMarkerName)
	markerExists, err := install.environment.PathExists(ctx, markerPath)
	if err != nil {
		return false, fmt.Errorf(
			"failed to inspect Nano initial-create marker %s: %w", markerPath, err,
		)
	}
	if !markerExists {
		return false, nil
	}

	if containerStatus.Exists {
		if err := install.client.Stop(ctx, containerName, out, outErr); err != nil {
			return false, install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to stop incomplete Nano container %s: %w", containerName, err))
		}
		if err := install.client.Remove(
			ctx, containerName, true, out, outErr,
		); err != nil {
			return false, install.failureWithDiagnostics(ctx, outErr, containerName,
				fmt.Errorf("failed to remove incomplete Nano container %s: %w", containerName, err))
		}
	}

	quarantinePath, err := install.nextQuarantinePath(ctx, dataDir, time.Now())
	if err != nil {
		return false, err
	}
	if err := install.environment.Rename(ctx, dataDir, quarantinePath); err != nil {
		return false, fmt.Errorf(
			"failed to quarantine incomplete Nano data at %s: %w", quarantinePath, err,
		)
	}
	if err := install.environment.MkdirAll(ctx, dataDir, dataDirMode); err != nil {
		return false, fmt.Errorf("failed to recreate Nano data directory %s: %w", dataDir, err)
	}

	return true, nil
}

// HostQuarantinedDataDirs lists the data that recovery set aside next to
// dataDir, reading the host filesystem directly.
func HostQuarantinedDataDirs(dataDir string) ([]string, error) {
	parent := filepath.Dir(dataDir)
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list quarantined Nano data in %s: %w", parent, err)
	}
	prefix := filepath.Base(dataDir) + quarantineSuffix
	var quarantined []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			quarantined = append(quarantined, filepath.Join(parent, entry.Name()))
		}
	}

	return quarantined, nil
}

func (install *NanoInstall) nextQuarantinePath(
	ctx context.Context,
	dataDir string,
	now time.Time,
) (string, error) {
	base := dataDir + quarantineSuffix + now.Format(quarantineTimeFormat)
	candidate := base
	for suffix := 1; ; suffix++ {
		exists, err := install.environment.PathExists(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("failed to inspect Nano quarantine path %s: %w", candidate, err)
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func (install *NanoInstall) removeStaleNanoTLSFiles(
	ctx context.Context,
	dataDir string,
) error {
	for _, relativePath := range nanoTLSFiles {
		path := filepath.Join(dataDir, relativePath)
		if err := install.environment.RemoveFile(ctx, path); err != nil {
			return fmt.Errorf("failed to remove stale Nano TLS file %s: %w", path, err)
		}
	}

	return nil
}
