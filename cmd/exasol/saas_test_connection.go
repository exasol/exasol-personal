// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/exasol/exasol-personal/internal/saas"
	"github.com/spf13/cobra"
)

var saasTestConnectionFlags = struct {
	DB   string
	User string
}{}

func newSaasTestConnectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test-connection",
		Short: "Dry-run connectivity test to a SaaS database",
		Long: `Verify connectivity to a target SaaS database without transferring data or
mutating either database.

Authentication uses the SaaS token (the token determines the session user). Runs an
ordered, fail-fast checklist: token validity, database status, egress-IP allowlist,
endpoint reachability, authentication, and SELECT 1. When --db-user is given, the
session user (CURRENT_USER) is verified against it.`,
		Example: `  exasol saas test-connection --db <db_uuid>
  exasol saas test-connection --db <db_uuid> --db-user <user>`,
		Args: cobra.NoArgs,
		RunE: runSaasTestConnection,
	}

	cmd.Flags().StringVar(&saasTestConnectionFlags.DB, "db", "", "Target SaaS database UUID")
	cmd.Flags().StringVar(&saasTestConnectionFlags.User, "db-user", "",
		"Expected SaaS database user; verified against CURRENT_USER")
	_ = cmd.MarkFlagRequired("db")

	return cmd
}

func runSaasTestConnection(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()
	deployment := commonFlags.Deployment()

	sctx, err := saasClientForCommand(deployment)
	if err != nil {
		return err
	}

	input := saas.ConnTestInput{
		DBUUID:   saasTestConnectionFlags.DB,
		Username: saasTestConnectionFlags.User,
		Token:    sctx.Token,
		EgressIP: detectEgressBestEffort(ctx, sctx.API),
	}

	checks, resolved, allOK := saas.RunConnectionTest(
		ctx,
		sctx.API,
		saas.DefaultDBFactory,
		sctx.Account,
		sctx.Region,
		input,
	)
	printChecks(cmd, checks)

	// Print the resolved endpoint whenever it is known — including on failure —
	// so the connection target can be verified.
	if resolved != nil {
		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "Endpoint: %s:%d\n", resolved.Host, resolved.Port)
		_, _ = fmt.Fprintf(out, "Port: %d\n", resolved.Port)
		if resolved.JDBC != "" {
			_, _ = fmt.Fprintf(out, "Connection string: %s\n", resolved.JDBC)
		}
	}

	if !allOK {
		return errors.New("connection test failed")
	}

	return nil
}

// detectEgressBestEffort returns the egress IP, or "" when detection fails (the
// allowlist check is then skipped rather than failing the whole test).
func detectEgressBestEffort(ctx context.Context, api saas.API) string {
	ip, err := api.DetectEgressIP(ctx)
	if err != nil {
		return ""
	}

	return ip
}

func printChecks(cmd *cobra.Command, checks []saas.Check) {
	for _, check := range checks {
		var mark string
		switch {
		case check.OK:
			mark = "✓"
		case check.Warn:
			mark = "!"
		default:
			mark = "x"
		}
		line := fmt.Sprintf("%s %s", mark, check.Name)
		if check.Detail != "" {
			line += " (" + check.Detail + ")"
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
	}
}

func registerSaasTestConnectionCmd(parent *cobra.Command) {
	cmd := newSaasTestConnectionCmd()
	requireInitializedDeploymentDir(cmd)
	registerDeploymentDirFlag(cmd, commonFlags)
	parent.AddCommand(cmd)
}
