// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/resource"
)

const (
	infrastructurePresetsResource = "infrastructure-presets"
	installationPresetsResource   = "installation-presets"
	sharedAssetsResource          = "shared-assets"
	resourceSpecFilename          = "resources.yaml"
)

func ResourceContext(
	ctx context.Context, kind PresetKind, preset PresetRef,
) (context.Context, string, error) {
	dir, err := presetDirectory(ctx, kind, preset)
	if err != nil {
		return nil, "", err
	}

	raw, err := os.ReadFile(filepath.Join(dir, resourceSpecFilename))
	if errors.Is(err, os.ErrNotExist) {
		return ctx, dir, nil
	}
	if err != nil {
		return nil, "", fmt.Errorf(
			"read resource specification for preset %q: %w", presetLabel(preset), err,
		)
	}

	spec, err := resource.ParseSpec(raw)
	if err != nil {
		return nil, "", fmt.Errorf(
			"invalid resource specification for preset %q: %w", presetLabel(preset), err,
		)
	}

	return resource.NewContext(ctx, resource.FromContext(ctx).Layer(spec)), dir, nil
}

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
	if preset.IsPath() {
		return resource.Copy(preset.Path, outDir)
	}

	resolver := resource.FromContext(ctx)
	resourceID := kind.resourceGroup + "/" + preset.Name
	path, err := resolver.Resolve(ctx, resourceID)
	if err != nil {
		if errors.Is(err, resource.ErrUnknownMember) {
			return fmt.Errorf("%w: %s", kind.unknownErr, preset.Name)
		}

		return err
	}

	return resource.Copy(path, outDir)
}

// WriteSharedDir materializes the deployment assets shared by every preset.
func WriteSharedDir(ctx context.Context, outDir string) error {
	path, err := resource.FromContext(ctx).Resolve(ctx, sharedAssetsResource)
	if err != nil {
		return err
	}

	return resource.Copy(path, outDir)
}

// ListEmbeddedPresets lists every built-in preset name of kind.
func ListEmbeddedPresets(ctx context.Context, kind PresetKind) []string {
	prefix := kind.resourceGroup + "/"
	resourceIDs := resource.FromContext(ctx).List(prefix)
	presets := make([]string, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		presets = append(presets, strings.TrimPrefix(resourceID, prefix))
	}

	return presets
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
	dir, err := resource.FromContext(ctx).Resolve(ctx, kind.resourceGroup+"/"+name)
	if err != nil {
		if errors.Is(err, resource.ErrUnknownMember) {
			return "", fmt.Errorf("%w: %s", kind.unknownErr, name)
		}

		return "", err
	}

	return dir, nil
}

func presetDirectory(ctx context.Context, kind PresetKind, preset PresetRef) (string, error) {
	if preset.IsPath() {
		return preset.Path, nil
	}

	return presetDir(ctx, kind, preset.Name)
}

func presetLabel(preset PresetRef) string {
	if preset.IsPath() {
		return preset.Path
	}

	return preset.Name
}
