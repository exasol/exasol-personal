// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/exasol/exasol-personal/internal/resource"
)

const (
	infrastructurePresetsResource = "infrastructure-presets"
	installationPresetsResource   = "installation-presets"
	sharedAssetsResource          = "shared-assets"
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

// WriteSharedDir materializes the deployment assets shared by every preset.
func WriteSharedDir(ctx context.Context, outDir string) error {
	return resource.FromContext(ctx).RequestCopy(ctx, sharedAssetsResource, outDir)
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
