// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/aws"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/exoscale"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/spf13/cobra"
)

var cleanupShowOpts = struct {
	JSON bool
}{}

const cleanupShowShort = "Show resources for a deployment id"

var cleanupShowCmd = &cobra.Command{
	Use:    "show <deployment-id>",
	Short:  cleanupShowShort,
	Args:   cobra.ExactArgs(1),
	PreRun: func(_ *cobra.Command, _ []string) { configureLogger() },
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		deploymentID := args[0]

		var collectors []shared.ProviderCollector

		// AWS Collector - default owner to caller identity
		if cleanupOpts.Region != "" {
			awsOwnerFilter := ""
			cfg, err := config.LoadDefaultConfig(cmd.Context())
			if err == nil {
				stsClient := sts.NewFromConfig(cfg)
				idOut, err := stsClient.GetCallerIdentity(cmd.Context(), &sts.GetCallerIdentityInput{})
				if err == nil && idOut.Arn != nil && *idOut.Arn != "" {
					awsOwnerFilter = *idOut.Arn
				}
			}

			collectors = append(collectors,
				aws.NewCollector(cleanupOpts.Region, awsOwnerFilter, false))
		}

		// Exoscale Collector
		if cleanupOpts.ExoscaleZone != "" {
			collectors = append(collectors,
				exoscale.NewCollector(cleanupOpts.ExoscaleZone, "", false))
		}

		// Find which provider has this deployment
		collector, err := shared.FindDeployment(cmd.Context(), collectors, deploymentID)
		if err != nil {
			return err
		}

		// Use the found collector to get details
		details, err := collector.CollectDeploymentDetails(cmd.Context(), deploymentID)
		if err != nil {
			return err
		}

		if cleanupShowOpts.JSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")

			return enc.Encode(details)
		}
		// Header removed; footer provides concise key=value summary instead
		rows := make([][]string, 0, len(details.Resources))
		for _, resource := range details.Resources {
			owner := resource.Tags["Owner"]
			if owner == "" {
				owner = "-"
			}
			when := "-"
			if v, ok := resource.Attr["launchTime"]; ok {
				when = fmt.Sprintf("%v", v)
			}
			if when == "-" {
				if v, ok := resource.Attr["createTime"]; ok {
					when = fmt.Sprintf("%v", v)
				}
			}
			if when == "-" {
				if v, ok := resource.Attr["lastModified"]; ok {
					when = fmt.Sprintf("%v", v)
				}
			}
			state := "-"
			if v, ok := resource.Attr["state"]; ok {
				state = fmt.Sprintf("%v", v)
			}
			rows = append(
				rows,
				[]string{
					string(resource.Ref.Type),
					resource.Ref.ID,
					owner,
					when,
					state,
					resource.Ref.ARN,
				},
			)
		}
		if len(rows) > 0 {
			shared.RenderTable(
				cmd.OutOrStdout(),
				[]string{"type", "id", "owner", "created", "state", "arn"},
				[]int{15, 25, 50, 22, 10, 90},
				rows,
			)
			fmt.Fprintf(cmd.OutOrStdout(), "\n")
		}
		// Footer summary in key=value style
		created := "-"
		if !details.Summary.CreatedAt.IsZero() {
			created = details.Summary.CreatedAt.Format("2006-01-02 15:04")
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Summary: deployment=%s, owner=%s, provider=%s, region=%s, created=%s, state=%s, resources=%d\n",
			details.Summary.ID,
			details.Summary.Owner,
			details.Summary.Provider,
			details.Summary.Region,
			created,
			details.Summary.State,
			details.Summary.Resources,
		); err != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "write output failed:", err)
		}

		slog.Info("show complete", "deployment", deploymentID)

		return nil
	},
}

func registerCleanupShowFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&cleanupShowOpts.JSON, "json", false, "Output JSON")
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupShowCmd)
	registerCleanupShowFlags(cleanupShowCmd)
}
