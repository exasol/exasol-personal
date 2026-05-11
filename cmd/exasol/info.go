// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const deploymentInfoCmdShortDesc = "Prints a summary of your Exasol deployment."

const deploymentInfoCmdLongDesc = deploymentInfoCmdShortDesc + `

Shows key deployment attributes in plain text, including:

- Deployment name, Deployment State, Cluster size and Cluster State
- Connection details appropriate for the active deployment backend

You can use the '--json' option to print the output in JSON format.

Example usage:
    exasol info
    exasol info --json
`

func fetchDeploymentInfoJSON(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	return deploy.PrintConnectionInsInJson(ctx, deployment, writer)
}

func fetchDeploymentInfoText(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	content, err := deploy.GetConnectionInstructionsText(ctx, deployment)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(writer, content)

	return err
}

func printConnectionInstructionsFromFile(deployment config.DeploymentDir, writer io.Writer) error {
	content, err := os.ReadFile(deployment.ConnectionInstructionsPath())
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(writer, string(content))

	return err
}

var deploymentInfoCmd = &cobra.Command{
	Use:   "info",
	Short: deploymentInfoCmdShortDesc,
	Long:  deploymentInfoCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		ctx := cmd.Context()
		deployment := commonFlags.Deployment()
		if commonFlags.OutputJson {
			return fetchDeploymentInfoJSON(ctx, deployment, os.Stdout)
		}

		return fetchDeploymentInfoText(ctx, deployment, os.Stdout)
	},
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(deploymentInfoCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(deploymentInfoCmd)
	registerDeploymentDirFlag(deploymentInfoCmd, commonFlags)
	registerOutputFlags(deploymentInfoCmd, commonFlags)
	rootCmd.AddCommand(deploymentInfoCmd)
}
