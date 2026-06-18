// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/exasol/exasol-personal/internal/saas"
	"github.com/spf13/cobra"
)

const allowIPName = "exasol-personal"

func newSaasAllowIPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "allow-ip",
		Short: "Allow this machine's egress IP on the SaaS account",
		Long: `Detect this machine's public egress IP (as seen by SaaS) and add it to the
SaaS account's allowed-IP list.

The local source database connects outbound to SaaS during migration, so its
egress IP must be allowed. Requires only a SaaS token; the IP is detected
automatically and added as a /32. The operation is idempotent.`,
		Example: `  exasol saas allow-ip`,
		Args:    cobra.NoArgs,
		RunE:    runSaasAllowIP,
	}
}

func runSaasAllowIP(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()
	deployment := commonFlags.Deployment()

	sctx, err := saasClientForCommand(deployment)
	if err != nil {
		return err
	}

	detected, err := sctx.API.DetectEgressIP(ctx)
	if err != nil {
		return fmt.Errorf("detecting egress ip: %w", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Detected egress IP %s.\n", detected)

	cidr, err := saas.ToCIDR(detected)
	if err != nil {
		return err
	}

	existing, err := sctx.API.ListAllowedIPs(ctx)
	if err != nil {
		return fmt.Errorf("reading allowed-ip list: %w", err)
	}
	for _, entry := range existing {
		if entry.CidrIp == cidr {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s is already allowed.\n", cidr)
			return nil
		}
	}

	if err := sctx.API.AddAllowedIP(ctx, saas.AllowedIP{
		Name:   allowIPName,
		CidrIp: cidr,
	}); err != nil {
		return fmt.Errorf("adding allowed ip: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Added %s to the allowed-IP list.\n", cidr)

	return nil
}

func registerSaasAllowIPCmd(parent *cobra.Command) {
	cmd := newSaasAllowIPCmd()
	requireInitializedDeploymentDir(cmd)
	registerDeploymentDirFlag(cmd, commonFlags)
	parent.AddCommand(cmd)
}
