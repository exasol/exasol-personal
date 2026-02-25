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
		region := cleanupOpts.Region
		// Use shared collector for details
		details, err := cleanup.CollectDeploymentDetails(cmd.Context(), region, deploymentID)
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
			cleanup.RenderTable(
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
		regionDisplay := cleanupOpts.Region
		if regionDisplay == "" {
			regionDisplay = "(default)"
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Summary: deployment=%s, owner=%s, region=%s, created=%s, state=%s, resources=%d\n",
			details.Summary.ID,
			details.Summary.Owner,
			regionDisplay,
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
