// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
	"github.com/spf13/cobra"
)

var cleanupRunOpts = struct {
	Execute bool
	All     bool
	Types   []string
}{}

const cleanupRunShort = "Run ordered cleanup (dry-run by default) for one or more deployment ids"

var cleanupRunCmd = &cobra.Command{
	Use:    "run <deployment-id>...",
	Short:  cleanupRunShort,
	Args:   cobra.ArbitraryArgs,
	PreRun: func(_ *cobra.Command, _ []string) { configureLogger() },
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateCleanupRunSelection(args); err != nil {
			return err
		}
		cmd.SilenceUsage = true

		plan := buildCleanupPlan(cmd.Context(), false)
		lookupIndex := collectLookupIndex(cmd.Context(), &plan)

		if !cleanupOpts.JSON {
			renderCleanupScope(cmd.OutOrStdout(), plan.Scope)
		}

		execution := cleanupExecutionInfo{Mode: runMode(cleanupRunOpts.Execute), Types: cleanupRunOpts.Types}

		switch {
		case len(plan.Collectors) == 0:
			err := fmt.Errorf("no searchable providers available")
			if cleanupOpts.JSON {
				if encodeErr := encodeJSONOutput(cmd.OutOrStdout(), cleanupRunJSONOutput{
					Scope:       plan.Scope,
					Execution:   execution,
					Deployments: []cleanupRunDeploymentJSONOutput{},
					Error:       commandError("no_searchable_providers", err),
				}); encodeErr != nil {
					return encodeErr
				}
			}

		case lookupIndex.SuccessCount == 0:
			err := fmt.Errorf("all selected provider lookup attempts failed")
			if cleanupOpts.JSON {
				if encodeErr := encodeJSONOutput(cmd.OutOrStdout(), cleanupRunJSONOutput{
					Scope:       plan.Scope,
					Execution:   execution,
					Deployments: []cleanupRunDeploymentJSONOutput{},
					Error:       commandError("provider_lookup_failed", err),
				}); encodeErr != nil {
					return encodeErr
				}
			}

			return err
		}

		deploymentIDs := selectCleanupRunDeployments(args, lookupIndex)
		if !cleanupOpts.JSON && len(deploymentIDs) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No deployments selected for cleanup.")
		}

		results := make([]cleanupRunDeploymentJSONOutput, 0, len(deploymentIDs))
		failureCount := 0
		resourceFailures := 0
		for i, deploymentID := range deploymentIDs {
			if !cleanupOpts.JSON && i > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}

			result, err := loadCleanupRunResult(cmd.Context(), lookupIndex, deploymentID)
			results = append(results, result)
			if err != nil {
				failureCount++
				if !cleanupOpts.JSON {
					renderRequestedDeployment(cmd.OutOrStdout(), deploymentID)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", err)
				}
				continue
			}

			resourceFailures += countFailedResults(result.Results)
			if !cleanupOpts.JSON {
				renderCleanupRunResult(cmd, *result.Resolved, result.Results, execution)
			}
			slog.Info(
				"run complete",
				"deployment",
				deploymentID,
				"actions",
				result.Summary.Actions,
				"execute",
				cleanupRunOpts.Execute,
			)
		}

		outcomeCode, outcomeErr := cleanupRunOutcome(failureCount, resourceFailures)
		if cleanupOpts.JSON {
			payload := cleanupRunJSONOutput{
				Scope:       plan.Scope,
				Execution:   execution,
				Deployments: results,
			}
			if outcomeErr != nil {
				payload.Error = commandError(outcomeCode, outcomeErr)
			}
			if encodeErr := encodeJSONOutput(cmd.OutOrStdout(), payload); encodeErr != nil {
				return encodeErr
			}
		}

		return outcomeErr
	},
}

// validateCleanupRunSelection rejects ambiguous targeting: ids and --all are
// alternatives, and --older-than does nothing without --all.
func validateCleanupRunSelection(args []string) error {
	switch {
	case cleanupRunOpts.All && len(args) > 0:
		return fmt.Errorf("--all cannot be combined with deployment ids")
	case !cleanupRunOpts.All && len(args) == 0:
		return fmt.Errorf("provide at least one deployment id, or --all to select every discovered deployment")
	case !cleanupRunOpts.All && cleanupOpts.OlderThan > 0:
		return fmt.Errorf("--older-than only applies together with --all")
	}

	return nil
}

// selectCleanupRunDeployments resolves the run targets: the given ids, or the
// stale deployments already in the index when --all is set.
func selectCleanupRunDeployments(args []string, lookupIndex cleanupLookupIndex) []string {
	if !cleanupRunOpts.All {
		return args
	}

	selected := shared.StaleDeploymentIDs(lookupIndex, cleanupOpts.OlderThan, time.Now())
	slog.Info("selected deployments", "count", len(selected), "older_than", cleanupOpts.OlderThan.String())

	return selected
}

// countFailedResults counts failed actions. Skipped is a normal outcome for
// protected resources, not a failure.
func countFailedResults(results []shared.Result) int {
	failed := 0
	for _, result := range results {
		if result.Status == shared.ResultStatusFailed {
			failed++
		}
	}

	return failed
}

// cleanupRunOutcome maps run tallies onto an error code and error. Failed
// resource actions count too, so callers can rely on the exit code alone.
func cleanupRunOutcome(deploymentFailures, resourceFailures int) (string, error) {
	switch {
	case deploymentFailures > 0 && resourceFailures > 0:
		return "run_failed", fmt.Errorf(
			"%d deployment requests failed, %d resource actions failed",
			deploymentFailures,
			resourceFailures,
		)
	case deploymentFailures > 0:
		return "deployment_requests_failed", fmt.Errorf("%d deployment requests failed", deploymentFailures)
	case resourceFailures > 0:
		return "resource_actions_failed", fmt.Errorf("%d resource actions failed", resourceFailures)
	}

	return "", nil
}

func loadCleanupRunResult(
	ctx context.Context,
	lookupIndex cleanupLookupIndex,
	deploymentID string,
) (cleanupRunDeploymentJSONOutput, error) {
	result := cleanupRunDeploymentJSONOutput{Requested: deploymentID}
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

	typeFilter := resourceTypeFilter(cleanupRunOpts.Types)
	actions, err := collector.PlanActions(details, typeFilter)
	if err != nil {
		result.Resolved = &resolved
		result.Error = commandError("cleanup_plan_failed", err)

		return result, err
	}

	results, err := collector.ExecuteActions(ctx, actions, cleanupRunOpts.Execute)
	if err != nil {
		result.Resolved = &resolved
		result.Error = commandError("cleanup_execution_failed", err)

		return result, err
	}

	result.Resolved = &resolved
	result.Results = results
	result.Summary = &cleanupExecutionSummary{Actions: len(results)}
	slog.Info(
		"cleanup run",
		"deployment",
		deploymentID,
		"region",
		details.Summary.Region,
		"execute",
		cleanupRunOpts.Execute,
		"filtered_types",
		cleanupRunOpts.Types,
	)

	return result, nil
}

func renderCleanupRunResult(
	cmd *cobra.Command,
	resolved cleanupResolved,
	results []shared.Result,
	execution cleanupExecutionInfo,
) {
	renderResolved(cmd.OutOrStdout(), resolved)
	renderExecutionInfo(cmd.OutOrStdout(), execution)
	mode := strings.ToUpper(execution.Mode)
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"Cleanup %s Deployment %s Region %s Actions %d\n\n",
		mode,
		resolved.Deployment,
		resolved.Location,
		len(results),
	); err != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "write output failed:", err)
	}
	rows := make([][]string, 0, len(results))
	for _, result := range results {
		rows = append(rows, []string{
			string(result.Action.Ref.Type),
			result.Action.Ref.ID,
			result.Action.Op,
			result.Status,
			result.Action.Reason,
		})
	}
	renderTable(
		cmd.OutOrStdout(),
		[]string{"type", "id", "op", "status", "reason"},
		[]int{20, 25, 8, 10, 20},
		rows,
	)
}

func runMode(execute bool) string {
	if execute {
		return "execute"
	}

	return "dry-run"
}

func registerCleanupRunFlags(cmd *cobra.Command) {
	cmd.Flags().
		BoolVar(&cleanupRunOpts.Execute, "execute", false, "Execute deletions instead of dry-run")
	cmd.Flags().
		BoolVar(&cleanupRunOpts.All, "all", false,
			"Clean up every discovered deployment instead of named ids (combine with --older-than)")
	registerAgeFilterFlag(cmd)
	cmd.Flags().
		StringSliceVar(&cleanupRunOpts.Types, "types", nil,
			"Optional comma-separated resource types to limit (e.g. ec2-instance,ebs-volume)")
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupRunCmd)
	registerCommonFlags(cleanupRunCmd, cleanupFlagOptions{includeOwner: true, includeJSON: true})
	registerCleanupRunFlags(cleanupRunCmd)
}
