// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"sort"
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

func GetPresetCatalog() PresetCatalog {
	presetCatalogOnce.Do(func() {
		presetCatalog = PresetCatalog{
			Infrastructures: loadInfrastructurePresets(),
			Installations:   loadInstallationPresets(),
		}
	})

	return presetCatalog
}

func loadInfrastructurePresets() []Preset {
	ids := presets.ListEmbeddedInfrastructuresPresets()
	presetList := make([]Preset, 0, len(ids))
	for _, presetId := range ids {
		info, err := deploy.GetInfrastructureInfo(presetId)
		if err != nil {
			// If a manifest cannot be read, skip it for help rendering.
			continue
		}
		presetList = append(
			presetList,
			Preset{ID: presetId, Name: info.ShortDescription, Description: info.LongDescription},
		)
	}
	sort.Slice(presetList, func(i, j int) bool { return presetList[i].ID < presetList[j].ID })

	return presetList
}

func loadInstallationPresets() []Preset {
	ids := presets.ListEmbeddedInstallationsPresets()
	presetList := make([]Preset, 0, len(ids))
	for _, presetId := range ids {
		manifest, err := presets.ReadInstallManifest(presetId)
		if err != nil {
			continue
		}
		presetList = append(
			presetList,
			Preset{ID: presetId, Name: manifest.Name, Description: manifest.Description},
		)
	}
	sort.Slice(presetList, func(i, j int) bool { return presetList[i].ID < presetList[j].ID })

	return presetList
}
