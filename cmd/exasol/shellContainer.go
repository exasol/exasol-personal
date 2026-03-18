// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const shellContainerCmdShortDesc = "Establish an interactive COS container shell connection"

const shellContainerCmdLongDesc = shellContainerCmdShortDesc + `

Creates an interactive COS container shell connection in the active deployment.
COS is cluster-scoped and uses the access node.
`

var shellContainerCmd = &cobra.Command{
	Use:   "container",
	Short: shellContainerCmdShortDesc,
	Long:  shellContainerCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return deploy.OpenCOSShell(cmd.Context(), commonFlags.DeploymentDir)
	},
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(shellContainerCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(shellContainerCmd)
	registerDeploymentDirFlag(shellContainerCmd, commonFlags)
	shellRootCmd.AddCommand(shellContainerCmd)
}
