// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"time"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const startCmdShortDesc = `Start a deployment`

const startCmdLongDesc = startCmdShortDesc + `

Start a deployment after it has been stopped.
After the deployment starts, it will have a new public IP address and DNS name assigned.

This command now waits until the Exasol database becomes ready and can accept connections.
If the database does not become ready within the timeout, the command fails.`

var startOpts = struct {
	WaitTimeoutMin int
}{}

const stopCmdShortDesc = `Stop a deployment`

const stopCmdLongDesc = stopCmdShortDesc + `

Stop a deployment without removing its configuration, data, or infrastructure.
`

var startCmd = &cobra.Command{
	Use:     "start",
	Short:   startCmdShortDesc,
	Long:    startCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		waitTimeout := time.Duration(startOpts.WaitTimeoutMin) * time.Minute
		waitTimeoutSeconds := int(waitTimeout.Seconds())

		if err := deploy.Start(
			cmd.Context(),
			commonFlags.DeploymentDir,
			commonFlags.DeployVerbose,
			waitTimeoutSeconds,
		); err != nil {
			return err
		}

		return printConnectionInstructionsFromFile(commonFlags.DeploymentDir, os.Stdout)
	},
}

var stopCmd = &cobra.Command{
	Use:     "stop",
	Short:   stopCmdShortDesc,
	Long:    stopCmdLongDesc,
	Args:    cobra.NoArgs,
	GroupID: rootCmdGroupLifecycle,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return deploy.Stop(
			cmd.Context(),
			commonFlags.DeploymentDir,
			commonFlags.DeployVerbose,
		)
	},
}

func registerStartFlags() {
	startCmd.Flags().IntVar(
		&startOpts.WaitTimeoutMin,
		"wait-timeout-minutes",
		deploy.StartedDefaultTimeoutInMinutes,
		"Maximum minutes to wait for the database to become ready",
	)
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(startCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(startCmd)
	registerStartFlags()
	registerVerboseFlag(startCmd, commonFlags)
	registerDeploymentDirFlag(startCmd, commonFlags)
	rootCmd.AddCommand(startCmd)

	requireMinorVersionCompatibility(stopCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(stopCmd)
	registerVerboseFlag(stopCmd, commonFlags)
	registerDeploymentDirFlag(stopCmd, commonFlags)
	rootCmd.AddCommand(stopCmd)
}
