// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import (
	"context"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Collector implements shared.ProviderCollector for Azure.
type Collector struct {
	subscriptionID string
	location       string
	ownerFilter    string
	legacy         bool
}

// NewCollector builds an Azure collector scoped to a subscription and location.
// The location may be the DefaultLocation sentinel ("all") to search every region.
func NewCollector(subscriptionID, location, ownerFilter string, legacy bool) *Collector {
	return &Collector{
		subscriptionID: subscriptionID,
		location:       location,
		ownerFilter:    ownerFilter,
		legacy:         legacy,
	}
}

func (c *Collector) Name() string {
	return ProviderName
}

func (c *Collector) IsAvailable(_ context.Context) bool {
	return c.subscriptionID != ""
}

func (c *Collector) GetAccountInfo(ctx context.Context) (string, error) {
	return GetAccountInfo(ctx, c.subscriptionID)
}

func (c *Collector) CollectDeploymentSummaries(ctx context.Context) ([]shared.DeploymentSummary, error) {
	return CollectDeploymentSummaries(ctx, c.subscriptionID, c.location, c.ownerFilter, c.legacy)
}

func (c *Collector) CollectDeploymentDetails(ctx context.Context, deploymentID string) (*shared.DeploymentDetails, error) {
	return CollectDeploymentDetails(ctx, c.subscriptionID, deploymentID)
}

func (c *Collector) PlanActions(details *shared.DeploymentDetails, typeFilter []shared.ResourceType) ([]shared.Action, error) {
	return PlanActions(details, typeFilter)
}

func (c *Collector) ExecuteActions(ctx context.Context, actions []shared.Action, execute bool) ([]shared.Result, error) {
	return ExecuteActions(ctx, c.subscriptionID, actions, execute)
}
