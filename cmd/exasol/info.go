// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const deploymentInfoCmdShortDesc = "Prints a summary of your Exasol deployment."

const deploymentInfoCmdLongDesc = deploymentInfoCmdShortDesc + `

Shows key deployment attributes in plain text, including:

- Deployment name, Deployment State, Cluster size and Cluster State
- Node details (public IP, Admin UI, DB port, DNS, SSH info)

You can use the '--json' option to print the output in JSON format.

Example usage:
    exasol info
    exasol info --json
`

func fetchDeploymentInfoJSON(
	ctx context.Context,
	deploymentDir string,
	writer io.Writer,
) error {
	return deploy.PrintConnectionInsInJson(ctx, deploymentDir, writer)
}

func fetchDeploymentInfoText(
	ctx context.Context,
	deploymentDir string,
	writer io.Writer,
) error {
	content, err := deploy.GetConnectionInstructionsText(ctx, deploymentDir)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(writer, content)

	return err
}

func printConnectionInstructionsFromFile(deploymentDir string, writer io.Writer) error {
	content, err := os.ReadFile(filepath.Join(deploymentDir, config.ConnectionInstruction))
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
		if commonFlags.OutputJson {
			return fetchDeploymentInfoJSON(ctx, commonFlags.DeploymentDir, os.Stdout)
		}

		return fetchDeploymentInfoText(ctx, commonFlags.DeploymentDir, os.Stdout)
	},
}

// nolint: gochecknoinits
func init() {
	registerDeploymentDirFlag(deploymentInfoCmd, commonFlags)
	registerOutputFlags(deploymentInfoCmd, commonFlags)
	rootCmd.AddCommand(deploymentInfoCmd)
}
