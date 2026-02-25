// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const deployCmdShortDesc = `Deploy using an existing deployment directory`

const deployCmdLongDesc = deployCmdShortDesc + `

Once a deployment is complete, Terraform state files will be stored in the deployment directory.
Do not delete the deployment directory until the ` + "`destroy`" + ` command has been executed.
`

var deployCmd = &cobra.Command{
	Use:     "deploy",
	Short:   deployCmdShortDesc,
	Long:    deployCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupEssential,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		lockfileMode := deploy.TofuLockfileReadonly
		if commonFlags.DeployTofuUpdateLockfile {
			lockfileMode = deploy.TofuLockfileUpdate
		}

		if err := deploy.Deploy(
			cmd.Context(),
			commonFlags.DeploymentDir,
			commonFlags.DeployVerbose,
			lockfileMode,
		); err != nil {
			return err
		}

		return printConnectionInstructionsFromFile(commonFlags.DeploymentDir, os.Stdout)
	},
}

// nolint: gochecknoinits
func init() {
	registerDeploymentDirFlag(deployCmd, commonFlags)
	registerVerboseFlag(deployCmd, commonFlags)
	registerDeployFlags(deployCmd, commonFlags)
	rootCmd.AddCommand(deployCmd)
}
