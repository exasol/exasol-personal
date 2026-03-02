// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/spf13/cobra"
)

const unlockCmdShortDesc = `Unlock a deployment.`

const unlockCmdLongDesc = statusCmdShortDesc + `

	The lock file will be removed.

	The lock file prevents multiple instances of the exasol launcher running simultaneously.
	Unlocking the directory may cause inconsistent file states in the deployment directory.
`

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: unlockCmdShortDesc,
	Long:  unlockCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		mutex, err := directorymutex.New(commonFlags.DeploymentDir)
		if err != nil {
			return err
		}

		return mutex.ClearLock()
	},
}

// nolint: gochecknoinits
func init() {
	requireDeploymentCompatibility(unlockCmd, minSupportedDeploymentVersionBaseline)
	requireInitializedDeploymentDir(unlockCmd)
	registerDeploymentDirFlag(unlockCmd, commonFlags)
	diagCmd.AddCommand(unlockCmd)
}
