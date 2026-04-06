// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/presets"
	"gopkg.in/yaml.v3"
)

var (
	ErrMissingInfrastructureManifest = errors.New(
		"missing infrastructure.yaml; this directory may not be initialized",
	)
	ErrMissingInstallManifest = errors.New(
		"missing installation.yaml; this directory may not be initialized",
	)
)

func ReadInfrastructureManifest(deployment DeploymentDir) (*presets.InfrastructureManifest, error) {
	path := deployment.InfrastructureManifestPath()

	slog.Debug("reading infrastructure manifest", "path", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrMissingInfrastructureManifest, path)
		}

		return nil, err
	}

	var manifest presets.InfrastructureManifest
	dec := yaml.NewDecoder(bytesNewReaderNoEscape(data))
	// Do not enforce known fields to allow forward-compatible keys.
	if err := dec.Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func ReadInstallManifest(deployment DeploymentDir) (*presets.InstallManifest, error) {
	path := deployment.InstallManifestPath()

	slog.Debug("reading installation manifest", "path", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrMissingInstallManifest, path)
		}

		return nil, err
	}

	var manifest presets.InstallManifest
	dec := yaml.NewDecoder(bytesNewReaderNoEscape(data))
	if err := dec.Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// bytesNewReaderNoEscape returns a yaml-decoder friendly reader for raw bytes.
// Using a tiny wrapper avoids importing bytes directly in multiple places and keeps
// linters happy if we later customize the decoder.
func bytesNewReaderNoEscape(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
