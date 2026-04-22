// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/spf13/cobra"
)

var localRunnerCmd = &cobra.Command{
	Use:    "local-runner",
	Short:  "",
	Long:   "",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		return localruntime.New(commonFlags.DeploymentDir).Run(cmd.Context())
	},
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(localRunnerCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(localRunnerCmd)
	registerDeploymentDirFlag(localRunnerCmd, commonFlags)
	rootCmd.AddCommand(localRunnerCmd)
}
