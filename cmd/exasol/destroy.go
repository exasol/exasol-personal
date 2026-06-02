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
By default, local deployment files are kept so the same deployment can be inspected or recreated.
Pass --remove to remove the local deployment directory after cloud resources have been destroyed.
`

var destroyOpts = struct {
	AutoApprove bool
	Remove      bool
}{}

func registerDestroyFlags(cmd *cobra.Command) {
	registerVerboseFlag(cmd, commonFlags)
	cmd.Flags().BoolVar(&destroyOpts.AutoApprove,
		"auto-approve",
		false,
		"Force destroy without confirmation prompt")
	cmd.Flags().BoolVar(&destroyOpts.Remove,
		"remove",
		false,
		"Remove the local deployment directory after cloud resources are successfully destroyed")
}

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Short:   destroyCmdShortDesc,
	Long:    destroyCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		deployment := commonFlags.Deployment()

		response := destroyOpts.AutoApprove
		if !response {
			removalTarget := ""
			if destroyOpts.Remove {
				removalTarget = deployment.Root()
			}
			response = askForUserConfirmation(destroyConfirmationPrompt(removalTarget))
		}
		if !response {
			return nil
		}

		if err := deploy.Destroy(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
		); err != nil {
			return err
		}
		if destroyOpts.Remove {
			return deploy.RemoveLocalDeploymentDirectory(cmd.Context(), deployment)
		}

		return nil
	},
}

func destroyConfirmationPrompt(localRemovalTarget string) string {
	prompt := "WARNING: Destroying a deployment " +
		"is an irreversible operation, " +
		"and removes all cloud resources " +
		"- including all data."
	if localRemovalTarget != "" {
		prompt += "\n\nLocal deployment directory to remove after destroy:\n" + localRemovalTarget
	}
	prompt += "\n\nProceed with destroy? [y/N]"

	return prompt
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(destroyCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(destroyCmd)
	requireDeploymentFileLogging(destroyCmd)
	registerDeploymentDirFlag(destroyCmd, commonFlags)
	registerDestroyFlags(destroyCmd)
	rootCmd.AddCommand(destroyCmd)
}
