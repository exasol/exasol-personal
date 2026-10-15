// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const shellHostCmdShortDesc = "Open a shell on a deployment host"

const shellHostCmdLongDesc = shellHostCmdShortDesc + `

Opens a host OS shell on a node in the active deployment.
If no specific node is specified, connects to the first node available.
Linux local deployments use the caller's shell, environment, and working directory.
Pass a command after -- to run it without opening an interactive shell.
`

var shellHostCmdOpts = struct {
	Node string
}{
	Node: "",
}

var shellHostCmd = &cobra.Command{
	Use:   "host [-- command...]",
	Short: shellHostCmdShortDesc,
	Long:  shellHostCmdLongDesc,
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return deploy.OpenHostShell(
			cmd.Context(), commonFlags.Deployment(), shellHostCmdOpts.Node, args,
		)
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
	requireDefaultDeploymentCompatibility(shellHostCmd)
	requireInitializedDeploymentDir(shellHostCmd)
	registerShellHostFlags()
	registerDeploymentDirFlag(shellHostCmd, commonFlags)
	shellRootCmd.AddCommand(shellHostCmd)
}
