// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/assets"
	"gopkg.in/yaml.v3"
)

// InfrastructureTofu captures optional tofu-related configuration from the manifest.
type InfrastructureTofu struct {
	VariablesFile  string `yaml:"variablesFile"`
	VarsOutputFile string `yaml:"varsOutputFile"`
}

// InfrastructureManifest represents the infrastructure metadata and optional tofu configuration.
type InfrastructureManifest struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Tofu        *InfrastructureTofu `yaml:"tofu,omitempty"`
}

var (
	ErrMissingName        = errors.New("missing infrastructure name")
	ErrMissingDescription = errors.New("missing infrastructure description")
)

// ReadInfrastructureManifest loads and validates the infrastructure manifest from embedded assets.
func ReadInfrastructureManifest(infrastructureName string) (*InfrastructureManifest, error) {
	manifestRaw, err := assets.InfrastructureAssets.ReadFile(
		assets.InfrastructureAssetDir + "/" +
			infrastructureName + "/" +
			InfrastructureManifestFilename,
	)
	if err != nil {
		return nil, err
	}

	return parseInfrastructureManifest(manifestRaw)
}

// ReadInfrastructureManifestFromDir loads and validates the infrastructure manifest
// from a preset directory on disk.
func ReadInfrastructureManifestFromDir(dir string) (*InfrastructureManifest, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, InfrastructureManifestFilename))
	if err != nil {
		return nil, fmt.Errorf("failed to read infrastructure manifest from %q: %w", dir, err)
	}

	return parseInfrastructureManifest(manifestRaw)
}

func parseInfrastructureManifest(manifestRaw []byte) (*InfrastructureManifest, error) {
	var manifest InfrastructureManifest

	decoder := yaml.NewDecoder(bytes.NewReader(manifestRaw))
	// Do not enforce KnownFields to allow future/unknown keys.
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}

	// Validate minimal fields (new schema only)
	if manifest.Name == "" {
		return nil, ErrMissingName
	}
	if manifest.Description == "" {
		return nil, ErrMissingDescription
	}

	return &manifest, nil
}
