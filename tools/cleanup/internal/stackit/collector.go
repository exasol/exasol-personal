// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"os"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// ProviderName is the identifier for the STACKIT provider
const ProviderName = "stackit"

// Collector implements shared.ProviderCollector for STACKIT.
type Collector struct {
	projectId   string
	region      string
	ownerFilter string
}

func NewCollector(projectId, region, ownerFilter string) *Collector {
	return &Collector{
		projectId:   projectId,
		region:      region,
		ownerFilter: ownerFilter,
	}
}

func (c *Collector) Name() string {
	return ProviderName
}

func (c *Collector) IsAvailable(ctx context.Context) bool {
	// Check if STACKIT credentials are set
	keyPath := os.Getenv("STACKIT_SERVICE_ACCOUNT_KEY_PATH")
	return keyPath != ""
}

func (c *Collector) GetAccountInfo(ctx context.Context) (string, error) {
	return GetAccountInfo(ctx, c.projectId)
}

func (c *Collector) CollectDeploymentSummaries(ctx context.Context) ([]shared.DeploymentSummary, error) {
	return CollectDeploymentSummaries(ctx, c.projectId, c.region, c.ownerFilter)
}

func (c *Collector) CollectDeploymentDetails(ctx context.Context, deploymentId string) (*shared.DeploymentDetails, error) {
	return CollectDeploymentDetails(ctx, c.projectId, c.region, deploymentId)
}

func (c *Collector) PlanActions(details *shared.DeploymentDetails, typeFilter []shared.ResourceType) ([]shared.Action, error) {
	return PlanActions(details, typeFilter)
}

func (c *Collector) ExecuteActions(ctx context.Context, actions []shared.Action, execute bool) ([]shared.Result, error) {
	return ExecuteActions(ctx, c.projectId, c.region, actions, execute)
}
