// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"go.yaml.in/yaml/v3"
)

// Saved definitions carry no secrets, but they decide what runs beside the
// database, so only their owner may edit them.
const sidecarsFileMode = 0o600

func (d DeploymentDir) SidecarsPath() string {
	return d.Resolve("sidecars.yaml")
}

func ReadSidecars(deployment DeploymentDir) (sidecar.Document, error) {
	data, err := os.ReadFile(deployment.SidecarsPath())
	if errors.Is(err, os.ErrNotExist) {
		return sidecar.Document{Version: 1, Sidecars: map[string]sidecar.Definition{}}, nil
	}
	if err != nil {
		return sidecar.Document{}, err
	}
	document, err := sidecar.Parse(data)
	if err != nil {
		return sidecar.Document{}, fmt.Errorf("sidecars.yaml: %w", err)
	}

	return document, nil
}

// WriteSidecars requires the deployment lock across read, mutation, and replacement.
func WriteSidecars(deployment DeploymentDir, document sidecar.Document) error {
	if document.Sidecars == nil {
		document.Sidecars = map[string]sidecar.Definition{}
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	if _, err := sidecar.Parse(data); err != nil {
		return err
	}

	return WriteFileAtomically(deployment.SidecarsPath(), data, sidecarsFileMode)
}
