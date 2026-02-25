// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"

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

// This variable must be named like this an remain in the main package,
// because we inject it's value during the build process with -ldflags.
var version = "0.0.0"

var versionCheckOpts = struct {
	Latest bool
}{}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: versionCmdShortDesc,
	Long:  versionCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		ctx := cmd.Context()

		if !versionCheckOpts.Latest {
			_, err := fmt.Fprintln(os.Stdout, semver.MustParse(version))
			return err
		}

		response, err := deploy.FetchLatestVersion(
			ctx,
			version,
			commonFlags.DeploymentDir,
		)
		if err != nil {
			return err
		}
		if commonFlags.OutputJson {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")

			return encoder.Encode(response)
		}

		return printLatestVersionText(version, response)
	},
}

//nolint:revive // Suppress warning about not handing Fprintf errors
func printLatestVersionText(
	currentVersion string,
	response *deploy.VersionCheckResponse,
) error {
	if response.LatestVersion.Version == currentVersion {
		fmt.Fprintf(
			os.Stdout,
			"You are using the latest version of Exasol Personal (%s).\n",
			currentVersion,
		)

		return nil
	}

	fmt.Fprintf(
		os.Stdout,
		"A new version of Exasol Personal is available: %s "+
			"(you are using %s)\n",
		response.LatestVersion.Version,
		currentVersion,
	)
	fmt.Fprintf(os.Stdout, "  Version: %s\n", response.LatestVersion.Version)
	fmt.Fprintf(
		os.Stdout,
		"  Operating System: %s\n",
		response.LatestVersion.OperatingSystem,
	)
	fmt.Fprintf(
		os.Stdout,
		"  Architecture: %s\n",
		response.LatestVersion.Architecture,
	)
	fmt.Fprintf(os.Stdout, "  Filename: %s\n", response.LatestVersion.Filename)
	fmt.Fprintf(os.Stdout, "  Size: %d bytes\n", response.LatestVersion.Size)
	fmt.Fprintf(os.Stdout, "  Download URL: %s\n", response.LatestVersion.URL)
	fmt.Fprintf(os.Stdout, "  SHA256: %s\n\n", response.LatestVersion.SHA256)

	return nil
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
