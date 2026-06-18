// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/saas"
	"github.com/spf13/cobra"
)

const saasCmdShortDesc = "Migrate a local deployment to Exasol SaaS"

const saasCmdLongDesc = saasCmdShortDesc + `

These commands authenticate to an Exasol SaaS account, prepare connectivity, and
migrate the local deployment's schema, data, and database objects into a SaaS database.

Define an account token first with "exasol saas token <PAT>" (or "exasol saas login").
The allow-ip, test-connection, and migration commands all require a defined token.
`

// saasFlags holds the SaaS account context shared by the subcommands. When a
// flag is empty the value cached in deployment.json (from a prior command) is used.
var saasFlags = struct {
	Account string
	Region  string
}{}

var saasCmd = &cobra.Command{
	Use:   "saas",
	Short: saasCmdShortDesc,
	Long:  saasCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

// resolveAccountID returns the explicit --account flag or the cached account id.
func resolveAccountID(deployment config.DeploymentDir) string {
	if saasFlags.Account != "" {
		return saasFlags.Account
	}
	if target, err := saas.LoadTarget(deployment); err == nil && target != nil {
		return target.AccountId
	}

	return ""
}

// resolveRegion returns the explicit --region flag or the cached region.
func resolveRegion(deployment config.DeploymentDir) string {
	if saasFlags.Region != "" {
		return saasFlags.Region
	}
	if target, err := saas.LoadTarget(deployment); err == nil && target != nil {
		return target.Region
	}

	return ""
}

// saasContext is the resolved SaaS API client and account context for a
// token-gated subcommand. Token is the SaaS access token, also used as the
// database connection credential.
type saasContext struct {
	API     saas.API
	Account string
	Region  string
	Token   string
}

// saasClientForCommand enforces the token gate and builds an API client for a
// token-gated subcommand.
func saasClientForCommand(deployment config.DeploymentDir) (saasContext, error) {
	token, err := saas.RequireToken(deployment)
	if err != nil {
		return saasContext{}, err
	}
	account := resolveAccountID(deployment)
	region := resolveRegion(deployment)

	return saasContext{
		API:     saas.NewClient(account, token),
		Account: account,
		Region:  region,
		Token:   token,
	}, nil
}

// nolint: gochecknoinits
func init() {
	saasCmd.PersistentFlags().StringVar(&saasFlags.Account, "account", "", "SaaS account id")
	saasCmd.PersistentFlags().StringVar(&saasFlags.Region, "region", "", "SaaS region")

	registerSaasTokenCmd(saasCmd)
	registerSaasLoginCmd(saasCmd)
	registerSaasAllowIPCmd(saasCmd)
	registerSaasTestConnectionCmd(saasCmd)
	registerSaasMigrationCmd(saasCmd)

	rootCmd.AddCommand(saasCmd)
}
