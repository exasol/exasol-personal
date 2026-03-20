// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"os"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Collector implements shared.ProviderCollector for Exoscale.
type Collector struct {
	zone        string
	ownerFilter string
	legacy      bool
}

// NewCollector creates an Exoscale collector with the specified configuration.
// The ownerFilter should already have provider-specific defaults applied by the caller.
func NewCollector(zone, ownerFilter string, legacy bool) *Collector {
	return &Collector{
		zone:        zone,
		ownerFilter: ownerFilter,
		legacy:      legacy,
	}
}

func (c *Collector) Name() string {
	return "exoscale"
}

func (c *Collector) IsAvailable(ctx context.Context) bool {
	// Check if Exoscale credentials are set
	apiKey := os.Getenv("EXOSCALE_API_KEY")
	apiSecret := os.Getenv("EXOSCALE_API_SECRET")
	return apiKey != "" && apiSecret != ""
}

func (c *Collector) CollectDeploymentSummaries(ctx context.Context) ([]shared.DeploymentSummary, error) {
	return CollectDeploymentSummaries(ctx, c.zone, c.ownerFilter, c.legacy)
}

func (c *Collector) CollectDeploymentDetails(ctx context.Context, deploymentID string) (*shared.DeploymentDetails, error) {
	return CollectDeploymentDetails(ctx, c.zone, deploymentID)
}

func (c *Collector) PlanActions(details *shared.DeploymentDetails, typeFilter []shared.ResourceType) ([]shared.Action, error) {
	return PlanActions(details, typeFilter)
}

func (c *Collector) ExecuteActions(ctx context.Context, actions []shared.Action, execute bool) ([]shared.Result, error) {
	return ExecuteActions(ctx, c.zone, actions, execute)
}
