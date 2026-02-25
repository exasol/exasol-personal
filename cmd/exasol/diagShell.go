// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const shellCmdShortDesc = "Establish a secure shell connection to a node"

const shellCmdLongDesc = shellCmdShortDesc + `

Creates a secure shell connection to a node in the active deployment.
If no specific node is specified, connects to the first node available.
`

var shellCmdOpts = struct {
	Node string
}{
	Node: "",
}

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: shellCmdShortDesc,
	Long:  shellCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return deploy.Shell(cmd.Context(), commonFlags.DeploymentDir, shellCmdOpts.Node)
	},
}

func registerShellFlags() {
	shellCmd.Flags().StringVarP(
		&shellCmdOpts.Node, "node", "n", "",
		"Name of the node to connect to. Connects to the first available node if not specified",
	)
}

// nolint: gochecknoinits
func init() {
	registerShellFlags()
	registerDeploymentDirFlag(shellCmd, commonFlags)
	diagCmd.AddCommand(shellCmd)
}
