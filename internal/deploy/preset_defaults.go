// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"slices"

	"github.com/exasol/exasol-personal/internal/presets"
)

func ResolveDefaultInstallationPreset(
	ctx context.Context,
	infrastructurePreset PresetRef,
) (PresetRef, error) {
	if err := validateInfrastructurePreset(ctx, infrastructurePreset); err != nil {
		return PresetRef{}, err
	}

	infrastructureManifest, err := readInfrastructureManifestFromPreset(ctx, infrastructurePreset)
	if err != nil {
		return PresetRef{}, fmt.Errorf(
			"failed to load infrastructure preset %q: %w",
			presetLabel(infrastructurePreset),
			err,
		)
	}

	for _, installName := range compatibleDefaultInstallationCandidates(ctx) {
		installationPreset := PresetRef{Name: installName}
		installManifest, err := readInstallManifestFromPreset(ctx, installationPreset)
		if err != nil {
			continue
		}

		if err := validatePresetCompatibility(
			infrastructurePreset,
			infrastructureManifest,
			installationPreset,
			installManifest,
		); err == nil {
			return installationPreset, nil
		}
	}

	return PresetRef{}, fmt.Errorf(
		"no compatible default installation preset found for infrastructure preset %q",
		presetLabel(infrastructurePreset),
	)
}

func compatibleDefaultInstallationCandidates(ctx context.Context) []string {
	candidates := []string{presets.DefaultInstallation}
	for _, name := range presets.ListEmbeddedPresets(ctx, presets.Installation) {
		if !slices.Contains(candidates, name) {
			candidates = append(candidates, name)
		}
	}

	return candidates
}
