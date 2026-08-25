// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"

	"github.com/exasol/exasol-personal/internal/presets"
)

type InfrastructureInfo struct {
	Name             string
	ShortDescription string
	LongDescription  string
}

func GetInfrastructureInfo(
	ctx context.Context,
	infrastructureName string,
) (*InfrastructureInfo, error) {
	info := InfrastructureInfo{}

	manifest, err := presets.ReadInfrastructureManifest(ctx, infrastructureName)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read manifest for infrastructure %q: %w",
			infrastructureName,
			err,
		)
	}

	info.Name = manifest.Name
	info.ShortDescription = manifest.Name
	info.LongDescription = manifest.Description

	return &info, nil
}

func GetInfrastructureInfoFromDir(dir string) (*InfrastructureInfo, error) {
	info := InfrastructureInfo{}

	manifest, err := presets.ReadInfrastructureManifestFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read manifest for infrastructure preset at %q: %w",
			dir,
			err,
		)
	}

	info.Name = manifest.Name
	info.ShortDescription = manifest.Name
	info.LongDescription = manifest.Description

	return &info, nil
}

// GetInfrastructureInfoFromPreset returns infrastructure info for either an embedded preset
// (by name) or a filesystem preset (by path). This keeps the selection logic in one place.
func GetInfrastructureInfoFromPreset(
	ctx context.Context,
	preset PresetRef,
) (*InfrastructureInfo, error) {
	if preset.IsPath() {
		return GetInfrastructureInfoFromDir(preset.Path)
	}

	return GetInfrastructureInfo(ctx, preset.Name)
}
