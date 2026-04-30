// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	tarXzSuffix       = ".tar.xz"
	extractedSubdir   = "extracted"
	diskImageSuffix   = ".img"
	extractDirMode    = 0o700
	extractMarkerMode = 0o600
	extractMarkerExt  = ".sha256"
)

var extractTarXzCommand = func(archivePath string, outDir string) *exec.Cmd {
	return exec.CommandContext(
		context.Background(),
		"tar", "-xJf", archivePath, "-C", outDir,
	)
}

// resolveDiskImagePath inspects the wire path returned by EnsureCached. If
// the file is a `.tar.xz` archive, it is extracted (once) into a sibling
// directory and the first `*.img` entry inside is returned. Otherwise the
// wire path is returned as-is.
func resolveDiskImagePath(wirePath string) (string, error) {
	if !strings.HasSuffix(wirePath, tarXzSuffix) {
		return wirePath, nil
	}

	parentDir := filepath.Dir(wirePath)
	extractDir := filepath.Join(parentDir, extractedSubdir)
	markerPath := filepath.Join(extractDir, filepath.Base(wirePath)+extractMarkerExt)

	if existing, ok := findCachedImage(extractDir, markerPath); ok {
		return existing, nil
	}

	if err := resetExtractDir(extractDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(extractDir, extractDirMode); err != nil {
		return "", fmt.Errorf("failed to create extract dir: %w", err)
	}

	cmd := extractTarXzCommand(wirePath, extractDir)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to extract disk archive %s: %w", wirePath, err)
	}

	imgPath, err := findFirstImage(extractDir)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(markerPath, []byte(""), extractMarkerMode); err != nil {
		return "", fmt.Errorf("failed to write extract marker: %w", err)
	}

	return imgPath, nil
}

func findCachedImage(extractDir string, markerPath string) (string, bool) {
	if _, err := os.Stat(markerPath); err != nil {
		return "", false
	}
	imgPath, err := findFirstImage(extractDir)
	if err != nil {
		return "", false
	}

	return imgPath, true
}

func resetExtractDir(extractDir string) error {
	if err := os.RemoveAll(extractDir); err != nil {
		return fmt.Errorf("failed to reset extract dir: %w", err)
	}

	return nil
}

func findFirstImage(root string) (string, error) {
	var imgPath string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, diskImageSuffix) {
			imgPath = path
			return filepath.SkipAll
		}

		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return "", fmt.Errorf("failed to inspect extract dir: %w", err)
	}
	if imgPath == "" {
		return "", fmt.Errorf("no %s file found in %s", diskImageSuffix, root)
	}

	return imgPath, nil
}
