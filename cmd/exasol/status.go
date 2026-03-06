// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const statusCmdShortDesc = `Get the status a deployment`

const statusCmdLongDesc = statusCmdShortDesc + `

Display the status of the current deployment.

	The possible values for the ` + "`status`" + ` field are:

	- ` + deploy.StatusNotInitialized + `
	- ` + deploy.StatusInitialized + `
	- ` + deploy.StatusOperationInProgress + `
	- ` + deploy.StatusInterrupted + `
	- ` + deploy.StatusDeploymentFailed + `
	- ` + deploy.StatusDatabaseConnectionFailed + `
	- ` + deploy.StatusDatabaseReady + `
`

var statusOpts = struct {
	unsafe bool
}{}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: statusCmdShortDesc,
	Long:  statusCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		var output string
		var err error
		formatter := deploy.StatusTextFormatter
		if commonFlags.OutputJson {
			formatter = deploy.StatusJSONFormatter
		}

		if statusOpts.unsafe {
			slog.Debug("acquiring deployment status without lock")
			output, err = deploy.StatusUnsafe(cmd.Context(), commonFlags.DeploymentDir, formatter)
		} else {
			slog.Debug("acquiring deployment status with lock")
			output, err = deploy.Status(cmd.Context(), commonFlags.DeploymentDir, formatter)
		}
		if err != nil {
			return err
		}

		safePrint(output)

		return nil
	},
}

func registerStatusFlags() {
	statusCmd.Flags().BoolVar(
		&statusOpts.unsafe,
		"unsafe", false,
		"Try to read the deployment folder state even it is locked. May fail.",
	)
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(statusCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(statusCmd)
	registerStatusFlags()
	registerDeploymentDirFlag(statusCmd, commonFlags)
	registerOutputFlags(statusCmd, commonFlags)
	rootCmd.AddCommand(statusCmd)
}
