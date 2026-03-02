// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

// These commands are only for internal purposes / debugging and are not visible to users directly

var printConnectionInstructions = &cobra.Command{
	Use:    "print-connection-instructions",
	Short:  "",
	Long:   "",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		content, err := deploy.GetConnectionInstructionsText(
			cmd.Context(),
			commonFlags.DeploymentDir,
		)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(os.Stdout, content)

		return err
	},
}

// nolint: gochecknoinits
func init() {
	requireDeploymentCompatibility(
		printConnectionInstructions,
		minSupportedDeploymentVersionBaseline,
	)
	requireInitializedDeploymentDir(printConnectionInstructions)
	registerDeploymentDirFlag(printConnectionInstructions, commonFlags)
	rootCmd.AddCommand(printConnectionInstructions)
}
