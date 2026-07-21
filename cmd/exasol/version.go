// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	_ "embed" // required for the go:embed directive below
	"encoding/json"
	"strings"
	"text/template"

	// Legacy v3 variants of semver are found in this repo due indirect tflint dependencies.
	// We should use the modern v4 within the our code though.
	"github.com/blang/semver/v4"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const versionCmdShortDesc = "Print the program version and exit"

const versionCmdLongDesc = versionCmdShortDesc + `

Example usage:
    exasol version
    exasol version --latest --json
`

// This variable must be named like this and remain in the main package,
// because we inject it's true value during the build process with -ldflags.
var CurrentLauncherVersion = "0.0.0"

var versionCheckOpts = struct {
	Latest bool
}{}

//go:embed version_latest_text.tmpl
var latestVersionTextTemplateSource string

var latestVersionTextTemplate = template.Must(
	template.New("version-latest-text").Parse(latestVersionTextTemplateSource),
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: versionCmdShortDesc,
	Long:  versionCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		ctx := cmd.Context()

		if !versionCheckOpts.Latest {
			if commonFlags.OutputJson {
				output, err := formatCurrentVersionJSON(CurrentLauncherVersion)
				if err != nil {
					return err
				}
				addTerminalOutput(output)

				return nil
			}
			addTerminalOutput(formatCurrentVersionText(CurrentLauncherVersion))

			return nil
		}

		response, err := deploy.FetchLatestVersion(
			ctx,
			CurrentLauncherVersion,
			commonFlags.Deployment(),
		)
		if err != nil {
			return err
		}
		if commonFlags.OutputJson {
			content, marshalErr := json.MarshalIndent(response, "", "  ")
			if marshalErr != nil {
				return marshalErr
			}
			addTerminalOutput(string(content))

			return nil
		}

		text, err := formatLatestVersionText(CurrentLauncherVersion, response)
		if err != nil {
			return err
		}
		addTerminalOutput(text)

		return nil
	},
}

type currentVersionOutput struct {
	Version string `json:"version"`
}

func formatCurrentVersionText(currentVersion string) string {
	return semver.MustParse(currentVersion).String()
}

func formatCurrentVersionJSON(currentVersion string) (string, error) {
	version, err := semver.Parse(currentVersion)
	if err != nil {
		return "", err
	}
	content, err := json.MarshalIndent(currentVersionOutput{Version: version.String()}, "", "  ")
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func formatLatestVersionText(
	currentVersion string,
	response *deploy.VersionCheckResponse,
) (string, error) {
	latestVersion := response.LatestVersion.Version
	updateAvailable, err := deploy.IsVersionUpdateAvailable(currentVersion, latestVersion)
	if err != nil {
		return "", err
	}
	state := "newer"
	if !updateAvailable && latestVersion == currentVersion {
		state = "equal"
	} else if !updateAvailable {
		state = "older"
	}
	var builder strings.Builder
	err = latestVersionTextTemplate.Execute(&builder, struct {
		CurrentVersion string
		LatestVersion  deploy.LatestVersionInfo
		State          string
	}{
		CurrentVersion: currentVersion,
		LatestVersion:  response.LatestVersion,
		State:          state,
	})
	if err != nil {
		return "", err
	}

	return builder.String(), nil
}

func registerVersionCheckFlags() {
	versionCmd.Flags().BoolVarP(
		&versionCheckOpts.Latest,
		"latest", "l", false,
		"Get information about the latest available version",
	)
}

// nolint: gochecknoinits
func init() {
	registerVersionCheckFlags()
	registerOutputFlags(versionCmd, commonFlags)
	rootCmd.AddCommand(versionCmd)
}
