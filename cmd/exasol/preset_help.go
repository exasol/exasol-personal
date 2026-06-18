// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
)

func embeddedPresetCompatibilityMatrix() string {
	infraIDs := presets.ListEmbeddedInfrastructuresPresets()
	installIDs := presets.ListEmbeddedInstallationsPresets()
	if len(infraIDs) == 0 || len(installIDs) == 0 {
		return ""
	}

	infraManifests := map[string]*presets.InfrastructureManifest{}
	for _, infraID := range infraIDs {
		manifest, err := presets.ReadInfrastructureManifest(infraID)
		if err != nil {
			continue
		}
		infraManifests[infraID] = manifest
	}

	installManifests := map[string]*presets.InstallManifest{}
	for _, installID := range installIDs {
		manifest, err := presets.ReadInstallManifest(installID)
		if err != nil {
			continue
		}
		installManifests[installID] = manifest
	}

	if len(infraManifests) == 0 || len(installManifests) == 0 {
		return ""
	}

	firstColumnWidth := len("infrastructure")
	for infraID := range infraManifests {
		if len(infraID) > firstColumnWidth {
			firstColumnWidth = len(infraID)
		}
	}

	columnWidths := map[string]int{}
	for installID := range installManifests {
		width := max(len(installID), len("yes"))
		columnWidths[installID] = width
	}

	var builder strings.Builder
	_, _ = builder.WriteString("Compatibility matrix (embedded presets):\n")
	_, _ = fmt.Fprintf(&builder, "  %-*s", firstColumnWidth, "infrastructure")
	for _, installID := range installIDs {
		if _, ok := installManifests[installID]; !ok {
			continue
		}
		_, _ = fmt.Fprintf(&builder, "  %-*s", columnWidths[installID], installID)
	}
	_ = builder.WriteByte('\n')

	for _, infraID := range infraIDs {
		infraManifest, ok := infraManifests[infraID]
		if !ok {
			continue
		}
		_, _ = fmt.Fprintf(&builder, "  %-*s", firstColumnWidth, infraID)
		for _, installID := range installIDs {
			installManifest, ok := installManifests[installID]
			if !ok {
				continue
			}
			cell := "no"
			if embeddedPresetPairCompatible(infraManifest, installManifest) {
				cell = "yes"
			}
			_, _ = fmt.Fprintf(&builder, "  %-*s", columnWidths[installID], cell)
		}
		_ = builder.WriteByte('\n')
	}

	return strings.TrimRight(builder.String(), "\n")
}

func embeddedPresetPairCompatible(
	infrastructureManifest *presets.InfrastructureManifest,
	installationManifest *presets.InstallManifest,
) bool {
	if infrastructureManifest == nil || installationManifest == nil {
		return false
	}

	required := installationManifest.RequiredCapabilities()
	if len(required) == 0 {
		return true
	}

	providedSet := map[string]struct{}{}
	for _, capability := range infrastructureManifest.ProvidedCapabilities() {
		providedSet[capability] = struct{}{}
	}

	for _, capability := range required {
		if _, ok := providedSet[capability]; !ok {
			return false
		}
	}

	return true
}
