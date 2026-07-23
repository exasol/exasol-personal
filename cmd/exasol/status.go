// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

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
		deployment := commonFlags.Deployment()
		var status *deploy.StatusOutput
		var err error

		if statusOpts.unsafe {
			slog.Debug("acquiring deployment status without lock")
			status, err = deploy.StatusUnsafe(cmd.Context(), deployment)
		} else {
			slog.Debug("acquiring deployment status with lock")
			status, err = deploy.Status(cmd.Context(), deployment)
		}
		if err != nil {
			return err
		}

		var output string
		if commonFlags.OutputJson {
			output, err = formatStatusJSON(*status)
			if err != nil {
				return err
			}
		} else {
			output = formatStatusText(*status)
		}
		addTerminalOutput(output)

		return nil
	},
}

func formatStatusJSON(status deploy.StatusOutput) (string, error) {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func formatStatusText(status deploy.StatusOutput) string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "Deployment directory: %s\n", status.DeploymentDir)
	_, _ = fmt.Fprintf(&builder, "Status: %s\n", status.Status)
	if status.Message != "" {
		_, _ = fmt.Fprintf(&builder, "Message: %s\n", status.Message)
	}

	return strings.TrimRight(builder.String(), "\n")
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
	requireDefaultDeploymentCompatibility(statusCmd)
	registerStatusFlags()
	registerDeploymentDirFlag(statusCmd, commonFlags)
	registerOutputFlags(statusCmd, commonFlags)
	rootCmd.AddCommand(statusCmd)
}
