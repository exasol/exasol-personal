// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/spf13/cobra"
)

const unlockCmdShortDesc = `Clear a stale deployment lock.`

const unlockCmdLongDesc = unlockCmdShortDesc + `

	Launcher commands lock the deployment directory while they run, and the operating
	system releases that lock when the command exits, also after a crash.

	Older launcher versions locked it with a marker file instead, which stays behind when
	such a launcher is terminated. This command removes those marker files. It cannot tell
	whether an older launcher that created them is still running, so make sure none is.
	It fails while a running launcher command of this version holds the lock.
`

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: unlockCmdShortDesc,
	Long:  unlockCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		mutex, err := directorymutex.New(commonFlags.Deployment().Root())
		if err != nil {
			return err
		}

		return mutex.ClearLock()
	},
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(unlockCmd)
	requireInitializedDeploymentDir(unlockCmd)
	registerDeploymentDirFlag(unlockCmd, commonFlags)
	diagCmd.AddCommand(unlockCmd)
}
