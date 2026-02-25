// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

// DefaultInfrastructure and DefaultInstallation are the embedded preset IDs that appear in
// documentation/tests and CLI help examples.
//
// Note: The launcher no longer defaults the infrastructure preset at the CLI level; the
// infrastructure preset must be provided explicitly (e.g. `exasol install aws`).
// The installation preset may still be omitted and will default to DefaultInstallation.
//
// They intentionally live outside cmd/ so they can be referenced by internal packages and tests
// without introducing an import cycle (cmd imports internal, but internal must not import cmd).
const (
	DefaultInfrastructure = "aws"
	DefaultInstallation   = "rootless"

	InfrastructureManifestFilename = "infrastructure.yaml"
	InstallationManifestFilename   = "installation.yaml"
)
