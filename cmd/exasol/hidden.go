// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
)

// These commands are only for internal purposes / debugging and are not visible to users directly

func addConnectionInstructionsTerminalOutput(deployment config.DeploymentDir) error {
	content, err := os.ReadFile(deployment.ConnectionInstructionsPath())
	if err != nil {
		return err
	}

	addTerminalOutput(string(content))

	return nil
}

var printConnectionInstructions = &cobra.Command{
	Use:    "print-connection-instructions",
	Short:  "",
	Long:   "",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		return addConnectionInstructionsTerminalOutput(commonFlags.Deployment())
	},
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(
		printConnectionInstructions,
		minSupportedDeploymentVersionBaseline,
	)
	requireInitializedDeploymentDir(printConnectionInstructions)
	registerDeploymentDirFlag(printConnectionInstructions, commonFlags)
	rootCmd.AddCommand(printConnectionInstructions)
}
