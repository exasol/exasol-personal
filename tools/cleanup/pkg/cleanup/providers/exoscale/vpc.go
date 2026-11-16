// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	v3 "github.com/exoscale/egoscale/v3"
)

// deploymentVPC is a VPC that belongs to a deployment, with all its subnets.
type deploymentVPC struct {
	deploymentID string
	vpc          v3.ListVpcEntry
	subnets      []v3.ListSubnetEntry
}

// listDeploymentVPCs lists the VPCs of the zone that belong to a deployment.
func listDeploymentVPCs(ctx context.Context, client *v3.Client) ([]deploymentVPC, error) {
	resp, err := client.ListVpcs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list VPCs: %w", err)
	}
	if resp == nil {
		return nil, nil
	}

	var result []deploymentVPC
	for _, vpc := range resp.Vpcs {
		depID := vpcDeploymentID(vpc)
		if depID == "" {
			continue
		}

		entry := deploymentVPC{deploymentID: depID, vpc: vpc}
		subnets, err := client.ListSubnets(ctx, vpc.ID)
		if err != nil {
			slog.Warn("list VPC subnets failed", "vpc", vpc.ID, "error", err)
		} else if subnets != nil {
			entry.subnets = subnets.Subnets
		}
		result = append(result, entry)
	}

	return result, nil
}

// vpcDeploymentID returns the deployment a VPC belongs to, or "" for VPCs of
// no deployment, including the zone's default VPC.
func vpcDeploymentID(vpc v3.ListVpcEntry) string {
	if vpc.Default != nil && *vpc.Default {
		return ""
	}
	if depID := vpc.Labels["deployment_id"]; depID != "" {
		return depID
	}

	return deploymentIDFromResourceName(vpc.Name)
}

// discoverSummariesFromVPCs discovers deployments from VPCs and their subnets.
func discoverSummariesFromVPCs(
	ctx context.Context,
	client *v3.Client,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	vpcs, err := listDeploymentVPCs(ctx, client)
	if err != nil {
		slog.Warn("VPC discovery failed", "zone", zone, "error", err)
		return
	}

	addVPCsToSummaries(vpcs, zone, ownerFilter, summaries)
}

// addVPCsToSummaries counts each deployment VPC and its subnets, registering
// the deployment if the owner filter accepts the VPC's owner.
func addVPCsToSummaries(
	vpcs []deploymentVPC,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	for _, entry := range vpcs {
		owner := entry.vpc.Labels["owner"]
		if summaries[entry.deploymentID] == nil && !ownerMatchesFilter(owner, ownerFilter) {
			continue
		}

		sum := getOrCreateSummary(summaries, zone, entry.deploymentID, owner, entry.vpc.CreatedAT)
		sum.Resources += 1 + len(entry.subnets)
		applyEarlierCreatedAt(sum, entry.vpc.CreatedAT)
		for _, subnet := range entry.subnets {
			applyEarlierCreatedAt(sum, subnet.CreatedAT)
		}
	}
}

// collectVPCDetails records the deployment's VPCs and all their subnets.
func collectVPCDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	vpcs, err := listDeploymentVPCs(ctx, client)
	if err != nil {
		slog.Warn("VPC discovery failed", "zone", zone, "error", err)
		return
	}

	acc.details.Resources = append(acc.details.Resources, vpcResourceMetas(vpcs, zone, deploymentID)...)
	for _, entry := range vpcs {
		if entry.deploymentID != deploymentID {
			continue
		}
		acc.earliest = preferEarlier(acc.earliest, nonZeroTime(entry.vpc.CreatedAT))
		for _, subnet := range entry.subnets {
			acc.earliest = preferEarlier(acc.earliest, nonZeroTime(subnet.CreatedAT))
		}
	}
}

// vpcResourceMetas describes the VPCs and subnets of one deployment.
func vpcResourceMetas(vpcs []deploymentVPC, zone, deploymentID string) []ResourceMeta {
	var metas []ResourceMeta
	for _, entry := range vpcs {
		if entry.deploymentID != deploymentID {
			continue
		}

		vpcID := string(entry.vpc.ID)
		metas = append(metas, ResourceMeta{
			Ref: ResourceRef{
				ARN:    vpcARN(zone, vpcID),
				Type:   ResourceVPC,
				Region: zone,
				ID:     vpcID,
			},
			Tags: entry.vpc.Labels,
			Attr: map[string]any{"name": entry.vpc.Name},
		})

		for _, subnet := range entry.subnets {
			id := vpcSubnetID(vpcID, string(subnet.ID))
			metas = append(metas, ResourceMeta{
				Ref: ResourceRef{
					ARN:    vpcSubnetARN(zone, id),
					Type:   ResourceVPCSubnet,
					Region: zone,
					ID:     id,
				},
				Tags: subnet.Labels,
				Attr: map[string]any{
					"name":      subnet.Name,
					"ipv4Block": subnet.Ipv4Block,
				},
			})
		}
	}

	return metas
}

func nonZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}

// vpcSubnetID identifies a subnet together with its VPC, as the API needs
// both to delete it.
func vpcSubnetID(vpcID, subnetID string) string {
	return vpcID + "/" + subnetID
}

func parseVPCSubnetID(id string) (v3.UUID, v3.UUID, error) {
	vpcPart, subnetPart, ok := strings.Cut(id, "/")
	if !ok {
		return "", "", fmt.Errorf("invalid VPC subnet ID %q, want <vpc-id>/<subnet-id>", id)
	}
	vpcID, err := v3.ParseUUID(vpcPart)
	if err != nil {
		return "", "", fmt.Errorf("invalid VPC ID in %q: %w", id, err)
	}
	subnetID, err := v3.ParseUUID(subnetPart)
	if err != nil {
		return "", "", fmt.Errorf("invalid subnet ID in %q: %w", id, err)
	}

	return vpcID, subnetID, nil
}

func vpcARN(zone, id string) string {
	return fmt.Sprintf("exoscale:%s:vpc:%s", zone, id)
}

func vpcSubnetARN(zone, id string) string {
	return fmt.Sprintf("exoscale:%s:vpc-subnet:%s", zone, id)
}

// A subnet may still list instances shortly after their deletion, and a VPC
// its subnets, so deleting them is retried.
const (
	vpcDeleteAttempts = 10
	vpcDeleteInterval = 3 * time.Second
)

type vpcSubnetHandler struct {
	client *v3.Client
}

func (h *vpcSubnetHandler) Delete(ctx context.Context, ref ResourceRef) error {
	vpcID, subnetID, err := parseVPCSubnetID(ref.ID)
	if err != nil {
		return err
	}

	return retryVPCDelete(ctx, "subnet", func() error {
		return h.client.DeleteSubnet(ctx, vpcID, subnetID)
	})
}

type vpcHandler struct {
	client *v3.Client
}

func (h *vpcHandler) Delete(ctx context.Context, ref ResourceRef) error {
	vpcID, err := v3.ParseUUID(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid VPC ID: %w", err)
	}

	return retryVPCDelete(ctx, "VPC", func() error {
		return h.client.DeleteVpc(ctx, vpcID)
	})
}

func retryVPCDelete(ctx context.Context, kind string, deleteFn func() error) error {
	var err error
	for attempt := 1; attempt <= vpcDeleteAttempts; attempt++ {
		err = deleteFn()
		if err == nil || errors.Is(err, v3.ErrNotFound) {
			return nil
		}
		if errors.Is(err, v3.ErrForbidden) || attempt == vpcDeleteAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(vpcDeleteInterval):
		}
	}

	return fmt.Errorf("failed to delete %s: %w", kind, err)
}
