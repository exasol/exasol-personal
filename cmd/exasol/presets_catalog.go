// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
)

// Preset describes an infrastructure/installation preset for help output.
//
// ID is the value used on the command line (e.g. `exasol init <id>`).
// Name/Description come from the preset manifest.
type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PresetCatalog struct {
	Infrastructures []Preset `json:"infrastructures"`
	Installations   []Preset `json:"installations"`
}

var (
	presetCatalogOnce sync.Once
	presetCatalog     PresetCatalog
)

func GetPresetCatalog(ctx context.Context) PresetCatalog {
	presetCatalogOnce.Do(func() {
		presetCatalog = PresetCatalog{
			Infrastructures: loadInfrastructurePresets(ctx),
			Installations:   loadInstallationPresets(ctx),
		}
	})

	return presetCatalog
}

func loadInfrastructurePresets(ctx context.Context) []Preset {
	ids := presets.ListEmbeddedPresets(ctx, presets.Infrastructure)
	presetList := make([]Preset, 0, len(ids))
	for _, presetId := range ids {
		info, err := deploy.GetInfrastructureInfo(ctx, presetId)
		if err != nil {
			// If a manifest cannot be read, skip it for help rendering.
			continue
		}
		presetList = append(
			presetList,
			Preset{ID: presetId, Name: info.ShortDescription, Description: info.LongDescription},
		)
	}
	slices.SortFunc(presetList, func(a, b Preset) int { return cmp.Compare(a.ID, b.ID) })

	return presetList
}

func loadInstallationPresets(ctx context.Context) []Preset {
	ids := presets.ListEmbeddedPresets(ctx, presets.Installation)
	presetList := make([]Preset, 0, len(ids))
	for _, presetId := range ids {
		manifest, err := presets.ReadInstallManifest(ctx, presetId)
		if err != nil {
			continue
		}
		presetList = append(
			presetList,
			Preset{ID: presetId, Name: manifest.Name, Description: manifest.Description},
		)
	}
	slices.SortFunc(presetList, func(a, b Preset) int { return cmp.Compare(a.ID, b.ID) })

	return presetList
}
