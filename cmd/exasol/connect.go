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

var connectOpts connect.Opts

var connectCmd = &cobra.Command{
	Use:          "connect",
	Short:        connectCmdShortDesc,
	Long:         connectCmdLongDesc,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return deploy.Connect(cmd.Context(), &connectOpts, commonFlags.DeploymentDir)
	},
}

func registerConnectFlags() {
	connectCmd.PersistentFlags().StringVarP(
		&connectOpts.Username,
		"username", "u", "sys",
		"Database username",
	)

	connectCmd.PersistentFlags().StringVarP(
		&connectOpts.Password,
		"password", "p", "",
		"Database password",
	)

	// This name is inspired by curl's --insecure flag.
	connectCmd.PersistentFlags().BoolVarP(
		&connectOpts.InsecureSkipCertValidation,
		"insecure", "k", false,
		"Skip server certificate verification",
	)
}

// nolint: gochecknoinits
func init() {
	registerConnectFlags()
	registerDeploymentDirFlag(connectCmd, commonFlags)
	rootCmd.AddCommand(connectCmd)
}
