// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package hetzner

import (
	"context"
	"fmt"
	"os"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ProviderName is the identifier for the Hetzner provider
const ProviderName = "hetzner"

// Collector implements shared.ProviderCollector for Hetzner Cloud.
type Collector struct {
	location    string
	ownerFilter string
	legacy      bool
}

// NewCollector creates a Hetzner collector with the specified configuration.
func NewCollector(location, ownerFilter string, legacy bool) *Collector {
	return &Collector{
		location:    location,
		ownerFilter: ownerFilter,
		legacy:      legacy,
	}
}

func (c *Collector) Name() string {
	return ProviderName
}

func (c *Collector) IsAvailable(_ context.Context) bool {
	token := os.Getenv("HCLOUD_TOKEN")
	return token != ""
}

func (c *Collector) GetAccountInfo(ctx context.Context) (string, error) {
	client := newHCloudClient()

	// Use a simple API call to verify connectivity and get project info
	// The hcloud client doesn't expose "whoami" directly, but listing servers
	// with a limit of 0 confirms the token is valid.
	_, resp, err := client.Server.List(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{PerPage: 1},
	})
	if err != nil {
		return "", fmt.Errorf("failed to verify Hetzner Cloud credentials: %w", err)
	}

	// Use the response to confirm we're connected; Hetzner tokens are project-scoped
	if resp != nil && resp.StatusCode == 200 {
		return "[project-scoped-token]", nil
	}

	return "[connected]", nil
}

func (c *Collector) CollectDeploymentSummaries(ctx context.Context) ([]shared.DeploymentSummary, error) {
	return CollectDeploymentSummaries(ctx, c.ownerFilter)
}

func (c *Collector) CollectDeploymentDetails(ctx context.Context, deploymentID string) (*shared.DeploymentDetails, error) {
	return CollectDeploymentDetails(ctx, deploymentID)
}

func (c *Collector) PlanActions(details *shared.DeploymentDetails, typeFilter []shared.ResourceType) ([]shared.Action, error) {
	return PlanActions(details, typeFilter)
}

func (c *Collector) ExecuteActions(ctx context.Context, actions []shared.Action, execute bool) ([]shared.Result, error) {
	return ExecuteActions(ctx, actions, execute)
}

func newHCloudClient() *hcloud.Client {
	token := os.Getenv("HCLOUD_TOKEN")
	return hcloud.NewClient(hcloud.WithToken(token))
}
