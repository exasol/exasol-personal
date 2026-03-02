// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// DeploymentVersionMarkerFileName is a plain-text marker file which contains the
// launcher version that created a deployment directory.
//
// Design decision: this file exists to provide a stable, low-coupling and
// human-auditable source for deployment compatibility checks.
const DeploymentVersionMarkerFileName = ".exasolLauncher.version"

const deploymentVersionMarkerFileMode = 0o600

// ReadDeploymentVersionMarker reads the deployment version marker from the deployment directory.
// It returns (version, true, nil) if the marker exists, ("", false, nil) if it does not exist.
func ReadDeploymentVersionMarker(deploymentDir string) (string, bool, error) {
	path := filepath.Join(deploymentDir, DeploymentVersionMarkerFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}

		return "", false, err
	}

	return strings.TrimSpace(string(data)), true, nil
}

// WriteDeploymentVersionMarker writes the plain-text deployment version marker.
func WriteDeploymentVersionMarker(deploymentDir string, deploymentVersion string) error {
	path := filepath.Join(deploymentDir, DeploymentVersionMarkerFileName)

	// Assumption: this file is written during init (under the deployment directory lock)
	// and only read afterwards. We keep the implementation intentionally simple.
	content := []byte(strings.TrimSpace(deploymentVersion))

	return os.WriteFile(path, content, deploymentVersionMarkerFileMode)
}
