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

var cleanupRunOpts = struct {
	JSON    bool
	Execute bool
	Types   []string
}{}

const cleanupRunShort = "Run ordered cleanup (dry-run by default) for a deployment id"

var cleanupRunCmd = &cobra.Command{
	Use:    "run <deployment-id>",
	Short:  cleanupRunShort,
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

		// Use resolved region from details to avoid empty display
		region := details.Summary.Region
		var typeFilter []shared.ResourceType
		for _, t := range cleanupRunOpts.Types {
			typeFilter = append(typeFilter, shared.ResourceType(t))
		}
		actions, err := collector.PlanActions(details, typeFilter)
		if err != nil {
			return err
		}
		results, err := collector.ExecuteActions(cmd.Context(), actions, cleanupRunOpts.Execute)
		if err != nil {
			return err
		}
		slog.Info(
			"cleanup run",
			"deployment",
			deploymentID,
			"region",
			region,
			"execute",
			cleanupRunOpts.Execute,
			"filtered_types",
			cleanupRunOpts.Types,
		)
		if cleanupRunOpts.JSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")

			return enc.Encode(results)
		}
		mode := "DRY-RUN"
		if cleanupRunOpts.Execute {
			mode = "EXECUTE"
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Cleanup %s Deployment %s Region %s Actions %d\n\n",
			mode,
			deploymentID,
			region,
			len(results),
		); err != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "write output failed:", err)
		}
		rows := make([][]string, 0, len(results))
		for _, result := range results {
			rows = append(
				rows,
				[]string{
					string(result.Action.Ref.Type),
					result.Action.Ref.ID,
					result.Action.Op,
					result.Status,
					result.Action.Reason,
				},
			)
		}
		shared.RenderTable(
			cmd.OutOrStdout(),
			[]string{"type", "id", "op", "status", "reason"},
			[]int{20, 25, 8, 10, 20},
			rows,
		)
		slog.Info(
			"run complete",
			"deployment",
			deploymentID,
			"actions",
			len(results),
			"execute",
			cleanupRunOpts.Execute,
		)

		return nil
	},
}

func registerCleanupRunFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&cleanupRunOpts.JSON, "json", false, "Output JSON")
	cmd.Flags().
		BoolVar(&cleanupRunOpts.Execute, "execute", false, "Execute deletions instead of dry-run")
	cmd.Flags().
		StringSliceVar(&cleanupRunOpts.Types, "types", nil,
			"Optional comma-separated resource types to limit (e.g. ec2-instance,ebs-volume)")
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupRunCmd)
	registerCleanupRunFlags(cleanupRunCmd)
}
