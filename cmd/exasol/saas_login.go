// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSaasLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to Exasol SaaS interactively (work in progress)",
		Long: `Log in to Exasol SaaS using an interactive browser flow and store the
resulting account token.

This flow is a work in progress. Until it is available, define a token directly
with "exasol saas token <PAT>".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			_, _ = fmt.Fprintln(cmd.OutOrStdout(),
				"Interactive SaaS login is not available yet. "+
					"Define a token directly with 'exasol saas token <PAT>'.")

			return nil
		},
	}
}

func registerSaasLoginCmd(parent *cobra.Command) {
	cmd := newSaasLoginCmd()
	requireInitializedDeploymentDir(cmd)
	registerDeploymentDirFlag(cmd, commonFlags)
	parent.AddCommand(cmd)
}
