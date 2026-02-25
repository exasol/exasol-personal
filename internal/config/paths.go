// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

// All paths are relative to the deployment directory, which contains all configuration,
// state files, and credentials needed to manage a specific database instance.
// Remember to use filepath.FromSlash for windows support.

const (
	InfrastructureFilesDirectory = "infrastructure"
	InstallationFilesDirectory   = "installation"
	SharedFilesDirectory         = "."
)
