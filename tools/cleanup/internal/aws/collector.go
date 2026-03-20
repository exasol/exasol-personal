// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Collector implements shared.ProviderCollector for AWS.
type Collector struct {
	region      string
	ownerFilter string
	legacy      bool
}

// NewCollector creates an AWS collector with the specified configuration.
// The ownerFilter should already have provider-specific defaults applied by the caller.
func NewCollector(region, ownerFilter string, legacy bool) *Collector {
	return &Collector{
		region:      region,
		ownerFilter: ownerFilter,
		legacy:      legacy,
	}
}

func (c *Collector) Name() string {
	return "aws"
}

func (c *Collector) IsAvailable(ctx context.Context) bool {
	// Check if AWS credentials are configured
	_, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.region))
	return err == nil
}

func (c *Collector) CollectDeploymentSummaries(ctx context.Context) ([]shared.DeploymentSummary, error) {
	return CollectDeploymentSummaries(ctx, c.region, c.ownerFilter, c.legacy)
}

func (c *Collector) CollectDeploymentDetails(ctx context.Context, deploymentID string) (*shared.DeploymentDetails, error) {
	return CollectDeploymentDetails(ctx, c.region, deploymentID)
}

func (c *Collector) PlanActions(details *shared.DeploymentDetails, typeFilter []shared.ResourceType) ([]shared.Action, error) {
	return PlanActions(details, typeFilter)
}

func (c *Collector) ExecuteActions(ctx context.Context, actions []shared.Action, execute bool) ([]shared.Result, error) {
	return ExecuteActions(ctx, actions, execute)
}
