// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const connectCmdShortDesc = "Open an SQL connection to a running database"

const connectCmdLongDesc = connectCmdShortDesc + `

Establish an SQL connection to the database instance in an active deployment.
`

const connectCmdExample = `  exasol connect
  exasol connect --json
	printf 'SELECT 1;\n' | exasol connect --json=compact`

var connectOpts = connect.Opts{
	ExecuteOnSemicolon: true,
	JSONFormat:         connect.JSONFormatPretty,
}

var connectCmd = &cobra.Command{
	Use:          "connect",
	Short:        connectCmdShortDesc,
	Long:         connectCmdLongDesc,
	Example:      connectCmdExample,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		connectOpts.OutputJSON = cmd.Flags().Changed("json")

		return deploy.Connect(cmd.Context(), &connectOpts, commonFlags.Deployment())
	},
}

func registerConnectFlags() {
	connectCmd.Flags().StringVarP(
		&connectOpts.Username,
		"username", "u", "sys",
		"Database username",
	)

	connectCmd.Flags().StringVarP(
		&connectOpts.Password,
		"password", "p", "",
		"Database password",
	)

	// This name is inspired by curl's --insecure flag.
	connectCmd.Flags().BoolVarP(
		&connectOpts.InsecureSkipCertValidation,
		"insecure", "k", false,
		"Skip server certificate verification",
	)

	connectCmd.Flags().BoolVar(
		&connectOpts.ExecuteOnSemicolon,
		"execute-on-semicolon", true,
		"Execute SQL only after semicolon terminators are entered",
	)

	JSONFormatVarP(
		connectCmd.Flags(),
		&connectOpts.JSONFormat,
		"json",
		"j",
		connect.JSONFormatPretty,
		"Output in JSON format: pretty, compact",
	)
}

// nolint: gochecknoinits
func init() {
	requireMinorVersionCompatibility(connectCmd, CurrentLauncherVersion)
	requireInitializedDeploymentDir(connectCmd)
	registerConnectFlags()
	registerDeploymentDirFlag(connectCmd, commonFlags)
	rootCmd.AddCommand(connectCmd)
}
