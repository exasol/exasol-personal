// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
)

func ValidatePresetSelection(
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
) error {
	if err := validateInfrastructurePreset(infrastructurePreset); err != nil {
		return err
	}
	if err := validateInstallationPreset(installationPreset); err != nil {
		return err
	}

	infrastructureManifest, err := readInfrastructureManifestFromPreset(infrastructurePreset)
	if err != nil {
		return fmt.Errorf(
			"failed to load infrastructure preset %q: %w",
			presetLabel(infrastructurePreset),
			err,
		)
	}
	backend, err := resolveBackendForManifest(infrastructureManifest)
	if err != nil {
		return err
	}
	if err := backend.ValidateEnvironment(); err != nil {
		return err
	}

	installManifest, err := readInstallManifestFromPreset(installationPreset)
	if err != nil {
		return fmt.Errorf(
			"failed to load installation preset %q: %w",
			presetLabel(installationPreset),
			err,
		)
	}

	return validatePresetCompatibility(
		infrastructurePreset,
		infrastructureManifest,
		installationPreset,
		installManifest,
	)
}

func readInfrastructureManifestFromPreset(
	infrastructurePreset PresetRef,
) (*presets.InfrastructureManifest, error) {
	if infrastructurePreset.IsPath() {
		return presets.ReadInfrastructureManifestFromDir(infrastructurePreset.Path)
	}

	return presets.ReadInfrastructureManifest(infrastructurePreset.Name)
}

func readInstallManifestFromPreset(
	installationPreset PresetRef,
) (*presets.InstallManifest, error) {
	if installationPreset.IsPath() {
		return presets.ReadInstallManifestFromDir(installationPreset.Path)
	}

	return presets.ReadInstallManifest(installationPreset.Name)
}

func validatePresetCompatibility(
	infrastructurePreset PresetRef,
	infrastructureManifest *presets.InfrastructureManifest,
	installationPreset PresetRef,
	installManifest *presets.InstallManifest,
) error {
	required := installManifest.RequiredCapabilities()
	if len(required) == 0 {
		return nil
	}

	provided := infrastructureManifest.ProvidedCapabilities()
	providedSet := make(map[string]struct{}, len(provided))
	for _, capability := range provided {
		providedSet[capability] = struct{}{}
	}

	missing := make([]string, 0, len(required))
	for _, capability := range required {
		if _, exists := providedSet[capability]; exists {
			continue
		}
		missing = append(missing, capability)
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf(
		"installation preset %q is incompatible with infrastructure preset %q:"+
			" missing capabilities [%s] (provided [%s])",
		presetLabel(installationPreset),
		presetLabel(infrastructurePreset),
		strings.Join(missing, ", "),
		strings.Join(provided, ", "),
	)
}
