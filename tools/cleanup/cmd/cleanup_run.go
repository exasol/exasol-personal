// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/cleanup"
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
		region := cleanupOpts.Region
		details, err := cleanup.CollectDeploymentDetails(cmd.Context(), region, deploymentID)
		if err != nil {
			return err
		}
		// Use resolved region from details to avoid empty display
		region = details.Summary.Region
		var typeFilter []cleanup.ResourceType
		for _, t := range cleanupRunOpts.Types {
			typeFilter = append(typeFilter, cleanup.ResourceType(t))
		}
		actions, err := cleanup.PlanActions(details, typeFilter)
		if err != nil {
			return err
		}
		results, err := cleanup.ExecuteActions(cmd.Context(), actions, cleanupRunOpts.Execute)
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
		cleanup.RenderTable(
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
