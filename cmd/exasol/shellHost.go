// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const shellHostCmdShortDesc = "Establish a secure shell connection to a host node"

const shellHostCmdLongDesc = shellHostCmdShortDesc + `

Creates a secure host OS shell connection to a node in the active deployment.
If no specific node is specified, connects to the first node available.
`

var shellHostCmdOpts = struct {
	Node string
}{
	Node: "",
}

var shellHostCmd = &cobra.Command{
	Use:   "host",
	Short: shellHostCmdShortDesc,
	Long:  shellHostCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return deploy.OpenHostShell(cmd.Context(), commonFlags.DeploymentDir, shellHostCmdOpts.Node)
	},
}

func registerShellHostFlags() {
	shellHostCmd.Flags().StringVarP(
		&shellHostCmdOpts.Node, "node", "n", "",
		"Name of the node to connect to. Connects to the first available node if not specified",
	)
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(shellHostCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(shellHostCmd)
	registerShellHostFlags()
	registerDeploymentDirFlag(shellHostCmd, commonFlags)
	shellRootCmd.AddCommand(shellHostCmd)
}
