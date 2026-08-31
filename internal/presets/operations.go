// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/assets"
	"github.com/exasol/exasol-personal/internal/resource"
)

const (
	infrastructurePresetsResource = "infrastructure-presets"
	installationPresetsResource   = "installation-presets"
)

var (
	ErrUnknownInfrastructure = errors.New("the specified infrastructure preset does not exist")
	ErrUnknownInstallation   = errors.New("the specified installation preset does not exist")
)

// PresetKind identifies which preset catalog (infrastructure or
// installation) an operation applies to.
type PresetKind struct {
	resourceGroup string
	unknownErr    error
}

var (
	Infrastructure = PresetKind{infrastructurePresetsResource, ErrUnknownInfrastructure}
	Installation   = PresetKind{installationPresetsResource, ErrUnknownInstallation}
)

// WriteDir materializes preset into outDir.
func WriteDir(ctx context.Context, kind PresetKind, preset PresetRef, outDir string) error {
	manager := resource.FromContext(ctx)
	if preset.IsPath() {
		def := resource.ResourceDefinition{
			Artifact: map[string]resource.ArtifactSpec{"any": {URL: preset.Path}},
		}

		return manager.GetCopy(ctx, def, kind.resourceGroup, outDir)
	}

	if err := manager.RequestMemberCopy(ctx, kind.resourceGroup, preset.Name, outDir); err != nil {
		if errors.Is(err, resource.ErrUnknownMember) {
			return fmt.Errorf("%w: %s", kind.unknownErr, preset.Name)
		}

		return err
	}

	return nil
}

func WriteSharedDir(outDir string) error {
	return writeEmbeddedDir(assets.SharedAssets, assets.SharedAssetDir, outDir)
}

// ListEmbeddedPresets lists every built-in preset name of kind.
func ListEmbeddedPresets(ctx context.Context, kind PresetKind) []string {
	return resource.FromContext(ctx).GroupMembers(kind.resourceGroup)
}

// ReadInfrastructureFile reads a file from the named embedded infrastructure
// preset directory (resolved through the resource cache). relPath is
// relative to the infrastructure preset directory.
func ReadInfrastructureFile(
	ctx context.Context,
	infrastructureName, relPath string,
) ([]byte, error) {
	dir, err := presetDir(ctx, Infrastructure, infrastructureName)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(relPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return io.ReadAll(file)
}

func presetDir(ctx context.Context, kind PresetKind, name string) (string, error) {
	dir, err := resource.FromContext(ctx).RequestMember(ctx, kind.resourceGroup, name)
	if err != nil {
		if errors.Is(err, resource.ErrUnknownMember) {
			return "", fmt.Errorf("%w: %s", kind.unknownErr, name)
		}

		return "", err
	}

	return dir, nil
}

// Write an embedded directory to the filesystem.
func writeEmbeddedDir(filesys embed.FS, embeddedDirPath string, outputDir string) error {
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
			if err := writeEmbeddedDir(
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
