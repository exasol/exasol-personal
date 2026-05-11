// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"
	"slices"

	"github.com/exasol/exasol-personal/internal/presets"
)

func ResolveDefaultInstallationPreset(infrastructurePreset PresetRef) (PresetRef, error) {
	infrastructureManifest, err := readInfrastructureManifestFromPreset(infrastructurePreset)
	if err != nil {
		return PresetRef{}, fmt.Errorf(
			"failed to load infrastructure preset %q: %w",
			presetLabel(infrastructurePreset),
			err,
		)
	}

	for _, installName := range compatibleDefaultInstallationCandidates() {
		installationPreset := PresetRef{Name: installName}
		installManifest, err := readInstallManifestFromPreset(installationPreset)
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

func compatibleDefaultInstallationCandidates() []string {
	candidates := []string{presets.DefaultInstallation}
	for _, name := range presets.ListEmbeddedInstallationsPresets() {
		if !slices.Contains(candidates, name) {
			candidates = append(candidates, name)
		}
	}

	return candidates
}
