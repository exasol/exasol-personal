// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const destroyCmdShortDesc = `Destroy a deployment`

const destroyCmdLongDesc = destroyCmdShortDesc + `

Destroying a deployment deletes all resources - including all data.
If you want to retain any data, make sure you've created and moved backups to another safe location.
`

var destroyOpts = struct {
	AutoApprove bool
}{}

func registerDestroyFlags(cmd *cobra.Command) {
	registerVerboseFlag(cmd, commonFlags)
	cmd.Flags().BoolVar(&destroyOpts.AutoApprove,
		"auto-approve",
		false,
		"Force destroy without confirmation prompt")
}

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Short:   destroyCmdShortDesc,
	Long:    destroyCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		response := false
		if destroyOpts.AutoApprove {
			response = true
		} else {
			response = askForUserConfirmation("WARNING: Destroying a deployment " +
				"is an irreversible operation, " +
				"and removes all cloud resources " +
				"- including all data.\n\nProceed with destroy? [y/N]")
		}

		if response {
			return deploy.Destroy(
				cmd.Context(),
				commonFlags.DeploymentDir, commonFlags.DeployVerbose,
			)
		}

		return nil
	},
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(destroyCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(destroyCmd)
	registerDeploymentDirFlag(destroyCmd, commonFlags)
	registerDestroyFlags(destroyCmd)
	rootCmd.AddCommand(destroyCmd)
}
