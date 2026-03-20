// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"fmt"
	"log/slog"
)

// ProviderCollector encapsulates provider-specific discovery logic.
// Each collector is initialized with all necessary configuration,
// including provider-specific defaults for owner filtering.
type ProviderCollector interface {
	// Name returns the provider identifier (e.g., "aws", "exoscale")
	Name() string

	// CollectDeploymentSummaries discovers all deployments for this provider
	// using the configuration provided at initialization.
	CollectDeploymentSummaries(ctx context.Context) ([]DeploymentSummary, error)

	// IsAvailable checks if the provider has valid credentials/configuration.
	// Returns false if credentials are missing or invalid.
	IsAvailable(ctx context.Context) bool

	// CollectDeploymentDetails retrieves detailed resource information for a specific deployment.
	CollectDeploymentDetails(ctx context.Context, deploymentID string) (*DeploymentDetails, error)

	// PlanActions creates an ordered list of cleanup actions for the given deployment.
	PlanActions(details *DeploymentDetails, typeFilter []ResourceType) ([]Action, error)

	// ExecuteActions runs the planned actions, either in dry-run or execute mode.
	ExecuteActions(ctx context.Context, actions []Action, execute bool) ([]Result, error)
}

// CollectAllProviders queries all available providers and merges results.
// It continues on errors from individual providers, collecting all available data.
func CollectAllProviders(
	ctx context.Context,
	collectors []ProviderCollector,
) ([]DeploymentSummary, error) {
	var allSummaries []DeploymentSummary

	for _, collector := range collectors {
		// Skip providers without valid credentials
		if !collector.IsAvailable(ctx) {
			slog.Debug("provider unavailable, skipping", "provider", collector.Name())
			continue
		}

		summaries, err := collector.CollectDeploymentSummaries(ctx)
		if err != nil {
			slog.Warn("provider discovery failed",
				"provider", collector.Name(),
				"error", err)
			continue // Continue with other providers
		}

		slog.Debug("provider discovery succeeded",
			"provider", collector.Name(),
			"count", len(summaries))

		allSummaries = append(allSummaries, summaries...)
	}

	return allSummaries, nil
}

// FindDeployment discovers which provider and region contains the deployment.
// Returns the matching collector, or error if not found.
func FindDeployment(
	ctx context.Context,
	collectors []ProviderCollector,
	deploymentID string,
) (ProviderCollector, error) {
	for _, collector := range collectors {
		// Skip providers without valid credentials
		if !collector.IsAvailable(ctx) {
			slog.Debug("provider unavailable, skipping", "provider", collector.Name())
			continue
		}

		summaries, err := collector.CollectDeploymentSummaries(ctx)
		if err != nil {
			slog.Warn("provider discovery failed during FindDeployment",
				"provider", collector.Name(),
				"error", err)
			continue
		}

		for _, summary := range summaries {
			if summary.ID == deploymentID {
				slog.Debug("deployment found",
					"deployment", deploymentID,
					"provider", collector.Name(),
					"region", summary.Region)
				return collector, nil
			}
		}
	}

	return nil, fmt.Errorf("deployment %s not found in any available provider", deploymentID)
}
