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

func (d DeploymentDir) SidecarsPath() string {
	return d.Resolve("sidecars.yaml")
}

func ReadSidecars(deployment DeploymentDir) (sidecar.Document, error) {
	data, err := os.ReadFile(deployment.SidecarsPath())
	if errors.Is(err, os.ErrNotExist) {
		return sidecar.Document{Version: 1, Containers: []sidecar.Container{}}, nil
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
	if document.Containers == nil {
		document.Containers = []sidecar.Container{}
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	if _, err := sidecar.Parse(data); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(deployment.Root(), ".sidecars-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), deployment.SidecarsPath()); err != nil {
		return fmt.Errorf("replace sidecars.yaml: %w", err)
	}

	return nil
}
