// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	_ "embed" // required for the go:embed directive below
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

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

//go:embed info_notice.tmpl
var deploymentInfoNoticeTemplateSource string

var deploymentInfoTextTemplate = template.Must(
	template.New("deployment-info-text").Parse(deploymentInfoTextTemplateSource),
)

var deploymentInfoNoticeTemplate = template.Must(
	template.New("deployment-info-notice").
		Funcs(template.FuncMap{"nextSteps": formatNextSteps}).
		Parse(deploymentInfoNoticeTemplateSource),
)

// formatNextSteps takes command and comment pairs and aligns the comments, so
// a long deployment selection keeps the block readable.
func formatNextSteps(pairs ...string) string {
	const pairSize = 2
	width := 0
	for index := 0; index < len(pairs); index += pairSize {
		width = max(width, len(pairs[index]))
	}
	lines := make([]string, 0, len(pairs)/pairSize)
	for index := 0; index+1 < len(pairs); index += pairSize {
		lines = append(lines, fmt.Sprintf("  %-*s   // %s", width, pairs[index], pairs[index+1]))
	}

	return strings.Join(lines, "\n")
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
	Use:   commandInfo,
	Short: deploymentInfoCmdShortDesc,
	Long:  deploymentInfoCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		report, err := deploy.GetDeploymentInfoReport(cmd.Context(), commonFlags.Deployment())
		if err != nil {
			return err
		}
		if commonFlags.OutputJson {
			return addRenderedDeploymentInfoJSON(report)
		}

		return addRenderedDeploymentInfoTextAndNotice(report)
	},
}

func addRenderedDeploymentInfoJSON(report *deploy.DeploymentInfoReport) error {
	return addRenderedTerminalOutput(func(writer io.Writer) error {
		return renderDeploymentInfoJSON(writer, report)
	})
}

func addRenderedDeploymentInfoTextAndNotice(report *deploy.DeploymentInfoReport) error {
	guidance, err := formatDeploymentInfoNotice(report)
	if err != nil {
		return err
	}
	if err := addRenderedTerminalOutput(func(writer io.Writer) error {
		return renderDeploymentInfoText(writer, report)
	}); err != nil {
		return err
	}
	addTerminalCallToAction(guidance)

	return nil
}

func formatDeploymentInfoNotice(report *deploy.DeploymentInfoReport) (string, error) {
	var output bytes.Buffer
	if err := deploymentInfoNoticeTemplate.Execute(&output, report); err != nil {
		return "", err
	}

	return strings.TrimRight(output.String(), "\n"), nil
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(deploymentInfoCmd)
	registerDeploymentDirFlag(deploymentInfoCmd, commonFlags)
	registerOutputFlags(deploymentInfoCmd, commonFlags)
	rootCmd.AddCommand(deploymentInfoCmd)
}
