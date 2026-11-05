// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"context"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

// Collector implements shared.ProviderCollector for IONOS Cloud.
type Collector struct {
	location    string
	ownerFilter string
}

// NewCollector builds an IONOS collector. The location may be the
// DefaultLocation sentinel ("all") to search every location.
func NewCollector(location, ownerFilter string) *Collector {
	return &Collector{location: location, ownerFilter: ownerFilter}
}

func (c *Collector) Name() string {
	return ProviderName
}

func (c *Collector) IsAvailable(_ context.Context) bool {
	return hasCredentials()
}

func (c *Collector) GetAccountInfo(ctx context.Context) (string, error) {
	return GetAccountInfo(ctx)
}

func (c *Collector) CollectDeploymentSummaries(ctx context.Context) ([]shared.DeploymentSummary, error) {
	return CollectDeploymentSummaries(ctx, c.location, c.ownerFilter)
}

func (c *Collector) CollectDeploymentDetails(ctx context.Context, deploymentID string) (*shared.DeploymentDetails, error) {
	return CollectDeploymentDetails(ctx, c.location, deploymentID)
}

func (c *Collector) PlanActions(details *shared.DeploymentDetails, typeFilter []shared.ResourceType) ([]shared.Action, error) {
	return PlanActions(details, typeFilter)
}

func (c *Collector) ExecuteActions(ctx context.Context, actions []shared.Action, execute bool) ([]shared.Result, error) {
	return ExecuteActions(ctx, actions, execute)
}
