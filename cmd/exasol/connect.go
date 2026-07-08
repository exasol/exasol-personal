// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"

	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const connectCmdShortDesc = "Open an SQL connection to a running database"

const connectCmdLongDesc = connectCmdShortDesc + `

Establish an SQL connection to the database instance in an active deployment.
When run non-interactively with --json, stdout contains one JSON document for
the full invocation. Statement records include statementType and rowsAffected;
result-set statements report rowsAffected as 0 when the driver does not expose
affected-row metadata. Interactive --json sessions continue to print one JSON
document per executed statement instead of an invocation envelope.
`

const connectCmdExample = `  exasol connect
  exasol connect --json
  exasol connect --csv -c "SELECT * FROM products" > products.csv
  exasol connect -c "SELECT 1; SELECT 2"
  exasol connect -f script.sql
  printf 'SELECT 1;\n' | exasol connect --json=compact`

var connectOpts = connect.Opts{
	ExecuteOnSemicolon: true,
	OutputFormat:       connect.OutputFormatTable,
	JSONFormat:         connect.JSONFormatPretty,
	MaxRows:            connect.MaxRowsUnset,
}

var connectCmd = &cobra.Command{
	Use:          "connect",
	Short:        connectCmdShortDesc,
	Long:         connectCmdLongDesc,
	Example:      connectCmdExample,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if cmd.Flags().Changed("command") && cmd.Flags().Changed("file") {
			return errors.New("--command and --file are mutually exclusive")
		}

		connectOpts.OutputFormat = selectedConnectOutputFormat(cmd)

		return deploy.Connect(cmd.Context(), &connectOpts, commonFlags.Deployment())
	},
}

func selectedConnectOutputFormat(cmd *cobra.Command) connect.OutputFormat {
	if cmd.Flags().Changed("csv") {
		return connect.OutputFormatCSV
	}
	if cmd.Flags().Changed("json") {
		return connect.OutputFormatJSON
	}

	return connect.OutputFormatTable
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

	connectCmd.Flags().Bool(
		"csv", false,
		"Output in CSV format",
	)

	connectCmd.Flags().StringVarP(
		&connectOpts.Command,
		"command", "c", "",
		"Execute the given semicolon-separated SQL statement(s) and exit",
	)

	connectCmd.Flags().StringVarP(
		&connectOpts.File,
		"file", "f", "",
		"Execute the semicolon-separated SQL statements from the given file and exit",
	)

	connectCmd.MarkFlagsMutuallyExclusive("command", "file")
	connectCmd.MarkFlagsMutuallyExclusive("json", "csv")

	connectCmd.Flags().IntVar(
		&connectOpts.MaxRows,
		"max-rows", connect.MaxRowsUnset,
		"Maximum rows to display per query (0 = unlimited; "+
			"default: 100 interactively, unlimited otherwise)",
	)
}

// nolint: gochecknoinits
func init() {
	requireDefaultDeploymentCompatibility(connectCmd)
	requireInitializedDeploymentDir(connectCmd)
	registerConnectFlags()
	registerDeploymentDirFlag(connectCmd, commonFlags)
	rootCmd.AddCommand(connectCmd)
}
