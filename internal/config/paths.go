// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"path"
	"path/filepath"
)

// All paths are relative to the deployment directory, which contains all configuration,
// state files, and credentials needed to manage a specific database instance.
// Remember to use filepath.FromSlash for windows support.

const (
	InfrastructureFilesDirectory = "infrastructure"
	InstallationFilesDirectory   = "installation"
	SharedFilesDirectory         = "."
	NodeAccessKeyFileName        = "node_access.pem"
)

// RelativeInfrastructureArtifactDir returns the path to the deployment root
// relative to the extracted infrastructure preset directory.
func RelativeInfrastructureArtifactDir() string {
	return ".."
}

// RelativeInstallationPresetDir returns the path to the extracted installation
// preset relative to the extracted infrastructure preset directory.
func RelativeInstallationPresetDir() string {
	return path.Join(RelativeInfrastructureArtifactDir(), InstallationFilesDirectory)
}

// ResolveDeploymentPath resolves a deployment-owned path against the deployment
// directory while preserving legacy absolute paths.
func ResolveDeploymentPath(pathValue string, deploymentDir string) string {
	if filepath.IsAbs(pathValue) {
		return pathValue
	}

	return filepath.Join(deploymentDir, pathValue)
}
