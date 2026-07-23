// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const diagLocalCmdShortDesc = "Report local deployment runtime and reachability state"

const diagLocalCmdLongDesc = diagLocalCmdShortDesc + `

Reports local VM status, reported guest IP, bound host ports, per-port
forwarder reachability, database readiness, and local platform support.
Safe to run at any time, whether or not the deployment is currently running.

The output is formatted as JSON.
`

var diagLocalCmd = &cobra.Command{
	Use:   "local",
	Short: diagLocalCmdShortDesc,
	Long:  diagLocalCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		diagnostics, err := deploy.DiagnoseLocal(cmd.Context(), commonFlags.Deployment())
		if err != nil {
			return err
		}

		return addJSONTerminalOutput(diagnostics)
	},
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(diagLocalCmd)
	requireInitializedDeploymentDir(diagLocalCmd)
	registerDeploymentDirFlag(diagLocalCmd, commonFlags)
	diagCmd.AddCommand(diagLocalCmd)
}
