// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const infoCmdShortDesc = "Print all deployment info"

const infoCmdLongDesc = infoCmdShortDesc + `

The output is formatted as JSON.
`

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: infoCmdShortDesc,
	Long:  infoCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		details, err := deploy.GetDiagnosticDeploymentInfo(
			cmd.Context(),
			commonFlags.Deployment(),
		)
		if err != nil {
			return err
		}

		return addJSONTerminalOutput(details)
	},
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(infoCmd)
	requireInitializedDeploymentDir(infoCmd)
	registerDeploymentDirFlag(infoCmd, commonFlags)
	diagCmd.AddCommand(infoCmd)
}
