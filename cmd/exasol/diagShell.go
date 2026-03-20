// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const diagShellCmdShortDesc = "Establish a secure shell connection to a node"

const diagShellCmdLongDesc = diagShellCmdShortDesc + `

Creates a secure shell connection to a node in the active deployment.
If no specific node is specified, connects to the first node available.
`

var diagShellCmdOpts = struct {
	Node string
}{
	Node: "",
}

var diagShellCmd = &cobra.Command{
	Use:        "shell",
	Short:      diagShellCmdShortDesc,
	Long:       diagShellCmdLongDesc,
	Args:       cobra.NoArgs,
	Hidden:     true,
	Deprecated: "use 'exasol shell host' instead",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return deploy.OpenHostShell(
			cmd.Context(),
			commonFlags.DeploymentDir,
			diagShellCmdOpts.Node,
		)
	},
}

func registerDiagShellFlags() {
	diagShellCmd.Flags().StringVarP(
		&diagShellCmdOpts.Node, "node", "n", "",
		"Name of the node to connect to. Connects to the first available node if not specified",
	)
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(diagShellCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(diagShellCmd)
	registerDiagShellFlags()
	registerDeploymentDirFlag(diagShellCmd, commonFlags)
	diagCmd.AddCommand(diagShellCmd)
}
