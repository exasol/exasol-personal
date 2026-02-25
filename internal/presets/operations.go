// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/exasol/exasol-personal/assets"
)

var (
	ErrUnknownInfrastructure = errors.New("the specified infrastructure preset does not exist")
	ErrUnknownInstallation   = errors.New("the specified installation preset does not exist")
)

func WriteInfrastructureDir(infrastructureName string, outDir string) error {
	entries, err := assets.InfrastructureAssets.ReadDir(assets.InfrastructureAssetDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == infrastructureName {
			return writeDir(
				assets.InfrastructureAssets,
				assets.InfrastructureAssetDir+"/"+infrastructureName,
				outDir)
		}
	}

	return ErrUnknownInfrastructure
}

func WriteInstallDir(installName string, outDir string) error {
	entries, err := assets.InstallationAssets.ReadDir(assets.InstallationAssetDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == installName {
			return writeDir(
				assets.InstallationAssets,
				assets.InstallationAssetDir+"/"+installName,
				outDir)
		}
	}

	return ErrUnknownInstallation
}

func WriteSharedDir(outDir string) error {
	return writeDir(assets.SharedAssets, assets.SharedAssetDir, outDir)
}

func ListEmbeddedInfrastructuresPresets() []string {
	entries, err := assets.InfrastructureAssets.ReadDir(assets.InfrastructureAssetDir)
	if err != nil {
		// If assets are not available, return an empty list instead of panicking.
		// Callers can handle the absence of configs and surface a friendly error.
		return []string{}
	}

	var infrastructures []string

	for _, entry := range entries {
		if entry.IsDir() {
			infrastructures = append(infrastructures, entry.Name())
		}
	}
	sort.Strings(infrastructures)

	return infrastructures
}

func ListEmbeddedInstallationsPresets() []string {
	entries, err := assets.InstallationAssets.ReadDir(assets.InstallationAssetDir)
	if err != nil {
		// If assets are not available, return an empty list instead of panicking.
		// Callers can handle the absence of configs and surface a friendly error.
		return []string{}
	}

	var installs []string

	for _, entry := range entries {
		if entry.IsDir() {
			installs = append(installs, entry.Name())
		}
	}

	sort.Strings(installs)

	return installs
}

// ReadInfrastructureFile reads a file from the embedded infrastructure assets.
// relPath is relative to the infrastructure preset directory.
func ReadInfrastructureFile(infrastructureName, relPath string) ([]byte, error) {
	return assets.InfrastructureAssets.ReadFile(
		assets.InfrastructureAssetDir + "/" + infrastructureName + "/" + relPath,
	)
}

// Write an embedded directory to the filesystem.
func writeDir(filesys embed.FS, embeddedDirPath string, outputDir string) error {
	slog.Debug("writing directory", "path", embeddedDirPath)

	entries, err := filesys.ReadDir(embeddedDirPath)
	if err != nil {
		return err
	}

	const perm = 0o700

	for _, entry := range entries {
		// Use path.Join for embedded asset paths ('/'), not OS paths.
		if entry.IsDir() {
			embeddedSubDir := embeddedDirPath + "/" + entry.Name()
			physicalSubDir := filepath.Join(outputDir, entry.Name())
			if err := writeDir(
				filesys,
				embeddedSubDir,
				physicalSubDir,
			); err != nil {
				return err
			}

			continue
		}

		/* Use path.Join (not filepath.Join) here because embedded asset
		paths always use '/' as a separator, regardless of OS. filepath.Join
		inserts OS separators (e.g., '\' on Windows), which caused issues
		accessing embedded binaries. path.Join ensures consistent asset paths. */
		embeddedFilePath := embeddedDirPath + "/" + entry.Name()
		outputFilePath := filepath.Join(outputDir, entry.Name())

		data, err := filesys.ReadFile(embeddedFilePath)
		if err != nil {
			return fmt.Errorf("%w: reading file: %s", err, embeddedFilePath)
		}

		err = os.MkdirAll(filepath.FromSlash(outputDir), perm) // nolint:gosec
		if err != nil {
			return fmt.Errorf("%w: creating directory: %s", err, outputDir)
		}

		slog.Debug("writing file", "path", outputFilePath)

		err = os.WriteFile(filepath.FromSlash(outputFilePath), data, perm) // nolint:gosec
		if err != nil {
			return fmt.Errorf("%w: writing file: %s", err, outputFilePath)
		}
	}

	return nil
}
