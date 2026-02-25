// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/spf13/cobra"
)

// CommonFlags contains CLI flag values.
//
// Keeping state and registration separate (via pointers to this struct) makes it easy
// to write unit tests in parallel: tests can create a fresh CommonFlags and register
// flags against test commands without touching shared globals.
type CommonFlags struct {
	// Global flags
	LogLevel string

	// Common flags for commands that interact with a deployment directory
	DeploymentDir string

	// Common flags for initialization (init and install)
	NoLauncherVersionCheck bool

	// Common flags for commands that could produced JSON
	OutputJson bool

	// Common flags for deploy-like commands (deploy + install).
	DeployVerbose            bool
	DeployTofuUpdateLockfile bool
}

// commonFlags is the default runtime instance used by the actual CLI commands.
// Unit tests should generally create their own FlagState instead of mutating this.
var commonFlags = &CommonFlags{}

func registerLogLevelFlag(root *cobra.Command, state *CommonFlags) {
	root.PersistentFlags().StringVar(
		&state.LogLevel,
		"log-level",
		"",
		"Set log level: debug, info, warn, error",
	)
}

func registerVerboseFlag(cmd *cobra.Command, state *CommonFlags) {
	cmd.Flags().BoolVarP(
		&state.DeployVerbose,
		"verbose", "v", false,
		"Enable verbose output for deployment actions",
	)
}

func registerDeploymentDirFlag(cmd *cobra.Command, state *CommonFlags) {
	AbsDirVar(
		cmd.Flags(),
		&state.DeploymentDir,
		"deployment-dir",
		"",
		".",
		"The directory to store deployment files",
	)
}

func registerDeployFlags(cmd *cobra.Command, state *CommonFlags) {
	cmd.Flags().BoolVar(
		&state.DeployTofuUpdateLockfile,
		"tofu-update-lockfile",
		false,
		"Allow OpenTofu to update .terraform.lock.hcl during init",
	)
}

func registerOutputFlags(cmd *cobra.Command, state *CommonFlags) {
	cmd.Flags().BoolVarP(
		&state.OutputJson,
		"json", "j", false,
		"Output in JSON format",
	)
}

func registerInitFlags(cmd *cobra.Command, state *CommonFlags) {
	cmd.PersistentFlags().BoolVar(
		&state.NoLauncherVersionCheck,
		"no-launcher-version-check",
		false,
		"Disable automatic version checking for new launcher releases",
	)
}
