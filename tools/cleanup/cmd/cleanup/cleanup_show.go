// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/spf13/cobra"
)

var cleanupShowOpts = struct {
	Types []string
}{}

const cleanupShowShort = "Show resources for one or more deployment ids"

var cleanupShowCmd = &cobra.Command{
	Use:    "show <deployment-id>...",
	Short:  cleanupShowShort,
	Args:   cobra.MinimumNArgs(1),
	PreRun: func(_ *cobra.Command, _ []string) { configureLogger() },
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		plan := buildCleanupPlan(cmd.Context(), false)
		lookupIndex := collectLookupIndex(cmd.Context(), &plan)

		if !cleanupOpts.JSON {
			renderCleanupScope(cmd.OutOrStdout(), plan.Scope)
		}

		switch {
		case len(plan.Collectors) == 0:
			err := fmt.Errorf("no searchable providers available")
			if cleanupOpts.JSON {
				if encodeErr := encodeJSONOutput(cmd.OutOrStdout(), cleanupShowJSONOutput{
					Scope:       plan.Scope,
					Deployments: []cleanupShowDeploymentJSONOutput{},
					Error:       commandError("no_searchable_providers", err),
				}); encodeErr != nil {
					return encodeErr
				}
			}

			return err
		case lookupIndex.SuccessCount == 0:
			err := fmt.Errorf("all selected provider lookup attempts failed")
			if cleanupOpts.JSON {
				if encodeErr := encodeJSONOutput(cmd.OutOrStdout(), cleanupShowJSONOutput{
					Scope:       plan.Scope,
					Deployments: []cleanupShowDeploymentJSONOutput{},
					Error:       commandError("provider_lookup_failed", err),
				}); encodeErr != nil {
					return encodeErr
				}
			}

			return err
		}

		results := make([]cleanupShowDeploymentJSONOutput, 0, len(args))
		failureCount := 0
		for i, deploymentID := range args {
			if !cleanupOpts.JSON && i > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}

			result, err := loadCleanupShowResult(cmd.Context(), lookupIndex, deploymentID)
			results = append(results, result)
			if err != nil {
				failureCount++
				if !cleanupOpts.JSON {
					renderRequestedDeployment(cmd.OutOrStdout(), deploymentID)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", err)
				}
				continue
			}

			if !cleanupOpts.JSON {
				renderCleanupShowResult(cmd, *result.Resolved, result.Details)
			}
			slog.Info("show complete", "deployment", deploymentID)
		}

		if cleanupOpts.JSON {
			payload := cleanupShowJSONOutput{
				Scope:       plan.Scope,
				Deployments: results,
			}
			if failureCount > 0 {
				payload.Error = commandError(
					"deployment_requests_failed",
					fmt.Errorf("%d deployment requests failed", failureCount),
				)
			}
			if encodeErr := encodeJSONOutput(cmd.OutOrStdout(), payload); encodeErr != nil {
				return encodeErr
			}
		}

		if failureCount > 0 {
			return fmt.Errorf("%d deployment requests failed", failureCount)
		}

		return nil
	},
}

func loadCleanupShowResult(
	ctx context.Context,
	lookupIndex cleanupLookupIndex,
	deploymentID string,
) (cleanupShowDeploymentJSONOutput, error) {
	result := cleanupShowDeploymentJSONOutput{Requested: deploymentID}
	matches := lookupIndex.Matches[deploymentID]

	switch {
	case len(matches) == 0:
		err := fmt.Errorf("deployment %s not found in searched providers", deploymentID)
		result.Error = commandError("deployment_not_found", err)

		return result, err
	case len(matches) > 1:
		err := fmt.Errorf(
			"deployment %s matched multiple search targets: %s",
			deploymentID,
			lookupMatchSummary(matches),
		)
		result.Error = commandError("deployment_ambiguous", err)

		return result, err
	}

	collector := matches[0].Collector
	resolved := matches[0].Resolved
	details, err := collector.CollectDeploymentDetails(ctx, deploymentID)
	if err != nil {
		result.Resolved = &resolved
		result.Error = commandError("deployment_details_failed", err)

		return result, err
	}

	result.Resolved = &resolved
	result.Details = filterDeploymentDetailsByTypes(details, cleanupShowOpts.Types)

	return result, nil
}

func renderCleanupShowResult(cmd *cobra.Command, resolved cleanupResolved, details *shared.DeploymentDetails) {
	renderResolved(cmd.OutOrStdout(), resolved)
	renderTypesFilter(cmd.OutOrStdout(), cleanupShowOpts.Types)
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
		rows = append(rows, []string{
			string(resource.Ref.Type),
			resource.Ref.ID,
			owner,
			when,
			state,
			resource.Ref.ARN,
		})
	}
	if len(rows) > 0 {
		shared.RenderTable(
			cmd.OutOrStdout(),
			[]string{"type", "id", "owner", "created", "state", "arn"},
			[]int{15, 25, 50, 22, 10, 90},
			rows,
		)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}
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
}

func registerCleanupShowFlags(cmd *cobra.Command) {
	cmd.Flags().
		StringSliceVar(&cleanupShowOpts.Types, "types", nil,
			"Optional comma-separated resource types to limit (e.g. ec2-instance,ebs-volume)")
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupShowCmd)
	registerCommonFlags(cleanupShowCmd, cleanupFlagOptions{includeOwner: true, includeJSON: true})
	registerCleanupShowFlags(cleanupShowCmd)
}
