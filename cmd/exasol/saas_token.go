// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"

	"github.com/exasol/exasol-personal/internal/saas"
	"github.com/spf13/cobra"
)

var saasTokenFlags = struct {
	Show  bool
	Clear bool
}{}

func newSaasTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token [PAT]",
		Short: "Define the SaaS account access token",
		Long: `Define, show, or clear the Exasol SaaS account access token.

The token is validated against the SaaS API and stored only when valid. It is
masked in all output. Defining a token is the prerequisite for the allow-ip,
test-connection, and migration commands.`,
		Example: `  exasol saas token exa_pat_xxx --account ORG-123 --region eu-central-1
  exasol saas token --show
  exasol saas token --clear`,
		Args: cobra.MaximumNArgs(1),
		RunE: runSaasToken,
	}

	cmd.Flags().
		BoolVar(&saasTokenFlags.Show, "show", false, "Show the stored (masked) token and exit")
	cmd.Flags().BoolVar(&saasTokenFlags.Clear, "clear", false, "Remove the stored token and exit")

	return cmd
}

func runSaasToken(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	deployment := commonFlags.Deployment()

	switch {
	case saasTokenFlags.Show && saasTokenFlags.Clear:
		return errors.New("--show and --clear are mutually exclusive")
	case saasTokenFlags.Clear:
		if err := saas.ClearToken(deployment); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "SaaS token cleared.")

		return nil
	case saasTokenFlags.Show:
		token, err := saas.LoadToken(deployment)
		if errors.Is(err, saas.ErrNoToken) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No SaaS token defined.")
			return nil
		}
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "SaaS token: %s\n", saas.MaskToken(token))

		return nil
	}

	if len(args) == 0 {
		return errors.New("provide the token as an argument, or use --show/--clear")
	}

	token := args[0]
	account := resolveAccountID(deployment)
	if account == "" {
		return errors.New("a SaaS account id is required: pass --account <id>")
	}

	client := saas.NewClient(account, token)
	if _, err := client.ValidateToken(cmd.Context()); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	if err := saas.SaveToken(deployment, token); err != nil {
		return err
	}
	if err := saas.SaveAccount(deployment, account, resolveRegion(deployment)); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(
		cmd.OutOrStdout(),
		"SaaS token stored for account %s (%s).\n",
		account,
		saas.MaskToken(token),
	)

	return nil
}

func registerSaasTokenCmd(parent *cobra.Command) {
	cmd := newSaasTokenCmd()
	requireInitializedDeploymentDir(cmd)
	registerDeploymentDirFlag(cmd, commonFlags)
	parent.AddCommand(cmd)
}
