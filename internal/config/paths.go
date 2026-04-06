// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import "path"

// All paths are relative to the deployment directory, which contains all configuration,
// state files, and credentials needed to manage a specific database instance.
// Remember to use filepath.FromSlash for windows support.

const (
	InfrastructureFilesDirectory = "infrastructure"
	InstallationFilesDirectory   = "installation"
	SharedFilesDirectory         = "."
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
