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

	// GetAccountInfo returns human-readable account information for the connected provider.
	// Returns empty string and error if not connected or unable to retrieve.
	GetAccountInfo(ctx context.Context) (string, error)

	// CollectDeploymentDetails retrieves detailed resource information for a specific deployment.
	CollectDeploymentDetails(ctx context.Context, deploymentID string) (*DeploymentDetails, error)

	// PlanActions creates an ordered list of cleanup actions for the given deployment.
	PlanActions(details *DeploymentDetails, typeFilter []ResourceType) ([]Action, error)

	// ExecuteActions runs the planned actions, either in dry-run or execute mode.
	ExecuteActions(ctx context.Context, actions []Action, execute bool) ([]Result, error)
}

// CollectAllProviders queries all available providers and merges results.
// It continues on errors from individual providers, collecting all available data.
// Returns an error if no providers are authenticated or if all authenticated providers fail.
func CollectAllProviders(
	ctx context.Context,
	collectors []ProviderCollector,
) ([]DeploymentSummary, error) {
	var allSummaries []DeploymentSummary
	availableCount := 0
	successCount := 0

	for _, collector := range collectors {
		// Skip providers without valid credentials
		if !collector.IsAvailable(ctx) {
			slog.Info("provider not authenticated, skipping", "provider", collector.Name())
			continue
		}

		availableCount++
		summaries, err := collector.CollectDeploymentSummaries(ctx)
		if err != nil {
			slog.Warn("provider discovery failed",
				"provider", collector.Name(),
				"error", err)
			continue // Continue with other providers
		}

		successCount++
		slog.Debug("provider discovery succeeded",
			"provider", collector.Name(),
			"count", len(summaries))

		allSummaries = append(allSummaries, summaries...)
	}

	if availableCount == 0 {
		return nil, fmt.Errorf("no providers are authenticated")
	}

	if successCount == 0 {
		return nil, fmt.Errorf("all provider discovery attempts failed")
	}

	return allSummaries, nil
}

// FindDeployment discovers which provider and region contains the deployment.
// Returns the matching collector, or error if not found.
// Returns an error if no providers are authenticated or if all authenticated providers fail.
func FindDeployment(
	ctx context.Context,
	collectors []ProviderCollector,
	deploymentID string,
) (ProviderCollector, error) {
	availableCount := 0
	successCount := 0

	for _, collector := range collectors {
		// Skip providers without valid credentials
		if !collector.IsAvailable(ctx) {
			slog.Info("provider not authenticated, skipping", "provider", collector.Name())
			continue
		}

		availableCount++
		summaries, err := collector.CollectDeploymentSummaries(ctx)
		if err != nil {
			slog.Warn("provider discovery failed during FindDeployment",
				"provider", collector.Name(),
				"error", err)
			continue
		}

		successCount++
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

	if availableCount == 0 {
		return nil, fmt.Errorf("no providers are authenticated")
	}

	if successCount == 0 {
		return nil, fmt.Errorf("all provider discovery attempts failed")
	}

	return nil, fmt.Errorf("deployment %s not found in any available provider", deploymentID)
}
