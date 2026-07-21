// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	_ "embed" // required for the go:embed directive below
	"encoding/json"
	"io"
	"os"
	"text/template"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const deploymentInfoCmdShortDesc = "Prints information about your Exasol deployment."

const deploymentInfoCmdLongDesc = deploymentInfoCmdShortDesc + `

You can use the '--json' option to print the output in JSON format.

Example usage:
    exasol info
    exasol info --json
`

//go:embed info_text.tmpl
var deploymentInfoTextTemplateSource string

var deploymentInfoTextTemplate = template.Must(
	template.New("deployment-info-text").Parse(deploymentInfoTextTemplateSource),
)

func fetchDeploymentInfoJSON(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	report, err := deploy.GetDeploymentInfoReport(ctx, deployment)
	if err != nil {
		return err
	}

	return renderDeploymentInfoJSON(writer, report)
}

func fetchDeploymentInfoText(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	report, err := deploy.GetDeploymentInfoReport(ctx, deployment)
	if err != nil {
		return err
	}

	return renderDeploymentInfoText(writer, report)
}

func renderDeploymentInfoJSON(
	writer io.Writer,
	report *deploy.DeploymentInfoReport,
) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(report)
}

func renderDeploymentInfoText(
	writer io.Writer,
	report *deploy.DeploymentInfoReport,
) error {
	return deploymentInfoTextTemplate.Execute(writer, report)
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
	requireDefaultDeploymentCompatibility(deploymentInfoCmd)
	registerDeploymentDirFlag(deploymentInfoCmd, commonFlags)
	registerOutputFlags(deploymentInfoCmd, commonFlags)
	rootCmd.AddCommand(deploymentInfoCmd)
}
