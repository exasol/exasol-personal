// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
)

// presetTypeHandler centralizes per-type behavior so list/export logic can be generic.
//
// If more preset types are added, extend this registry rather than duplicating switch logic.
type presetTypeHandler struct {
	Type string

	// Header is used for human-readable list output.
	Header string

	// Kind selects which preset catalog (used by presets list, export, and
	// JSON output) this handler's ListEmbedded/WriteDir calls apply to.
	Kind presets.PresetKind

	// Catalog accessors (used by presets list and JSON output).
	GetFromCatalog func(PresetCatalog) []Preset
	SetOnCatalog   func(*PresetCatalog, []Preset)
}

var presetTypeHandlers = []presetTypeHandler{
	{
		Type:   presets.PresetTypeInfrastructure,
		Header: "Infrastructure presets:",
		Kind:   presets.Infrastructure,
		GetFromCatalog: func(cat PresetCatalog) []Preset {
			return cat.Infrastructures
		},
		SetOnCatalog: func(cat *PresetCatalog, list []Preset) {
			cat.Infrastructures = list
		},
	},
	{
		Type:   presets.PresetTypeInstallation,
		Header: "Installation presets:",
		Kind:   presets.Installation,
		GetFromCatalog: func(cat PresetCatalog) []Preset {
			return cat.Installations
		},
		SetOnCatalog: func(cat *PresetCatalog, list []Preset) {
			cat.Installations = list
		},
	},
}

func allowedPresetTypes() string {
	parts := make([]string, 0, len(presetTypeHandlers))
	for _, h := range presetTypeHandlers {
		parts = append(parts, h.Type)
	}

	return strings.Join(parts, ", ")
}

func findPresetTypeHandler(presetType string) (*presetTypeHandler, bool) {
	for i := range presetTypeHandlers {
		if presetTypeHandlers[i].Type == presetType {
			return &presetTypeHandlers[i], true
		}
	}

	return nil, false
}
