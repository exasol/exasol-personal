// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

type ConfigVariableType string

const (
	ConfigVariableTypeString ConfigVariableType = "string"
	ConfigVariableTypeBool   ConfigVariableType = "bool"
	ConfigVariableTypeNumber ConfigVariableType = "number"
)

type ConfigVariableDefinition struct {
	Name           string
	Description    string
	Type           ConfigVariableType
	DefaultDisplay string
	Required       bool
}

// ConfigVariableResolution bundles the configurable variables declared by an
// infrastructure backend with the human-readable label used to identify the
// preset they came from (either an embedded preset name or a preset path).
type ConfigVariableResolution struct {
	Variables   map[string]ConfigVariableDefinition
	PresetLabel string
}

func ResolveInfrastructureConfigVariables(
	ctx context.Context,
	preset PresetRef,
) (ConfigVariableResolution, error) {
	manifest, label, err := readInfrastructureManifestForPreset(ctx, preset)
	if err != nil {
		return ConfigVariableResolution{PresetLabel: label}, err
	}
	variables, err := readInfrastructurePresetConfigVariables(ctx, preset, manifest)
	if err != nil {
		return ConfigVariableResolution{PresetLabel: label}, err
	}

	return ConfigVariableResolution{Variables: variables, PresetLabel: label}, nil
}

func ResolveInfrastructureConfigVariablesFromDeployment(
	ctx context.Context,
	deployment config.DeploymentDir,
) (ConfigVariableResolution, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return ConfigVariableResolution{}, err
	}
	backend, err := newDeploymentBackend(ctx, deployment, manifest)
	if err != nil {
		return ConfigVariableResolution{PresetLabel: manifest.Name}, err
	}
	variables, err := backend.ReadDeploymentConfigVariables()
	if err != nil {
		return ConfigVariableResolution{PresetLabel: manifest.Name}, err
	}

	return ConfigVariableResolution{Variables: variables, PresetLabel: manifest.Name}, nil
}

func readInfrastructureManifestForPreset(
	ctx context.Context,
	preset PresetRef,
) (*presets.InfrastructureManifest, string, error) {
	if preset.IsPath() {
		manifest, err := presets.ReadInfrastructureManifestFromDir(preset.Path)
		if err != nil {
			return nil, preset.Path, fmt.Errorf(
				"failed to read manifest for infrastructure preset at %q: %w",
				preset.Path,
				err,
			)
		}

		return manifest, preset.Path, nil
	}

	manifest, err := presets.ReadInfrastructureManifest(ctx, preset.Name)
	if err != nil {
		return nil, preset.Name, fmt.Errorf(
			"failed to read manifest for infrastructure %q: %w",
			preset.Name,
			err,
		)
	}

	return manifest, preset.Name, nil
}
