// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	v3 "github.com/exoscale/egoscale/v3"
)

// namedResource is a resource that identifies its deployment only by name.
type namedResource struct {
	name      string
	createdAt time.Time
}

// addNamedResourcesToSummaries counts each resource against its deployment and
// registers the deployment if no other resource did. Named resources carry no
// owner, so they only establish a deployment when the owner filter accepts an
// unknown owner.
func addNamedResourcesToSummaries(
	resources []namedResource,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	for _, resource := range resources {
		depID := deploymentIDFromResourceName(resource.name)
		if depID == "" {
			continue
		}

		sum, ok := summaries[depID]
		if !ok {
			if !ownerMatchesFilter("", ownerFilter) {
				continue
			}
			sum = getOrCreateSummary(summaries, zone, depID, "", resource.createdAt)
		}
		sum.Resources++
		applyEarlierCreatedAt(sum, resource.createdAt)
	}
}

// sosBucketsInZone converts the deployment buckets located in zone.
func sosBucketsInZone(buckets []s3types.Bucket, zone string) []namedResource {
	var resources []namedResource
	for _, bucket := range buckets {
		if !bucketInZone(bucket, zone) {
			continue
		}

		resource := namedResource{name: ptrString(bucket.Name)}
		if bucket.CreationDate != nil {
			resource.createdAt = *bucket.CreationDate
		}
		resources = append(resources, resource)
	}

	return resources
}

// listOrganizationResources lists security groups, SSH keys, IAM roles, and
// IAM API keys. They are not bound to a zone, so every zone lists them.
func listOrganizationResources(ctx context.Context, client *v3.Client) []namedResource {
	listers := []struct {
		kind string
		list func(context.Context, *v3.Client) ([]namedResource, error)
	}{
		{"security groups", listSecurityGroupResources},
		{"ssh keys", listSSHKeyResources},
		{"iam roles", listIAMRoleResources},
		{"iam api keys", listAPIKeyResources},
	}

	var resources []namedResource
	for _, lister := range listers {
		listed, err := lister.list(ctx, client)
		if err != nil {
			slog.Warn("list "+lister.kind+" failed", "error", err)
			continue
		}
		resources = append(resources, listed...)
	}

	return resources
}

func listSecurityGroupResources(ctx context.Context, client *v3.Client) ([]namedResource, error) {
	resp, err := client.ListSecurityGroups(ctx)
	if err != nil || resp == nil {
		return nil, err
	}

	resources := make([]namedResource, 0, len(resp.SecurityGroups))
	for _, sg := range resp.SecurityGroups {
		resources = append(resources, namedResource{name: sg.Name})
	}

	return resources, nil
}

func listSSHKeyResources(ctx context.Context, client *v3.Client) ([]namedResource, error) {
	resp, err := client.ListSSHKeys(ctx)
	if err != nil || resp == nil {
		return nil, err
	}

	resources := make([]namedResource, 0, len(resp.SSHKeys))
	for _, key := range resp.SSHKeys {
		resources = append(resources, namedResource{name: key.Name})
	}

	return resources, nil
}

func listIAMRoleResources(ctx context.Context, client *v3.Client) ([]namedResource, error) {
	resp, err := client.ListIAMRoles(ctx)
	if err != nil || resp == nil {
		return nil, err
	}

	resources := make([]namedResource, 0, len(resp.IAMRoles))
	for _, role := range resp.IAMRoles {
		resources = append(resources, namedResource{name: role.Name})
	}

	return resources, nil
}

func listAPIKeyResources(ctx context.Context, client *v3.Client) ([]namedResource, error) {
	resp, err := client.ListAPIKeys(ctx)
	if err != nil || resp == nil {
		return nil, err
	}

	resources := make([]namedResource, 0, len(resp.APIKeys))
	for _, key := range resp.APIKeys {
		resources = append(resources, namedResource{name: key.Name, createdAt: key.CreatedAT})
	}

	return resources, nil
}

// addOrganizationResourcesToSummaries counts organization-wide resources of
// known deployments. A deployment that only such resources reveal is
// registered unless it still has resources in another zone, whose resources
// these may be.
func addOrganizationResourcesToSummaries(
	ctx context.Context,
	resources []namedResource,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
	deploymentsInOtherZones func(context.Context) (map[string]bool, error),
) {
	var known, unknown []namedResource
	for _, resource := range resources {
		depID := deploymentIDFromResourceName(resource.name)
		switch {
		case depID == "":
		case summaries[depID] != nil:
			known = append(known, resource)
		default:
			unknown = append(unknown, resource)
		}
	}
	addNamedResourcesToSummaries(known, zone, ownerFilter, summaries)

	if len(unknown) == 0 || !ownerMatchesFilter("", ownerFilter) {
		return
	}

	elsewhere, err := deploymentsInOtherZones(ctx)
	if err != nil {
		slog.Warn("skipping deployments found only from organization-wide resources", "zone", zone, "error", err)
		return
	}

	var orphaned []namedResource
	for _, resource := range unknown {
		if !elsewhere[deploymentIDFromResourceName(resource.name)] {
			orphaned = append(orphaned, resource)
		}
	}
	addNamedResourcesToSummaries(orphaned, zone, ownerFilter, summaries)
	for _, resource := range orphaned {
		if sum := summaries[deploymentIDFromResourceName(resource.name)]; sum != nil {
			sum.LocationIndependent = true
		}
	}
}

// deploymentsInZonesOtherThan returns the ids of deployments with compute
// instances, block storage volumes, private networks, VPCs, or SOS buckets in
// any zone of the organization other than zone.
func deploymentsInZonesOtherThan(client *v3.Client, zone string) func(context.Context) (map[string]bool, error) {
	return func(ctx context.Context) (map[string]bool, error) {
		zones, err := listZoneNames(ctx, client)
		if err != nil {
			return nil, err
		}

		ids := map[string]bool{}
		for _, other := range zones {
			if other == zone {
				continue
			}
			if err := addZoneDeploymentIDs(ctx, other, ids); err != nil {
				return nil, err
			}
		}

		buckets, err := listDeploymentSOSBuckets(ctx, zone)
		if err != nil {
			return nil, err
		}
		for _, bucket := range buckets {
			if !bucketInZone(bucket, zone) {
				ids[deploymentIDFromResourceName(ptrString(bucket.Name))] = true
			}
		}

		return ids, nil
	}
}

func addZoneDeploymentIDs(ctx context.Context, zone string, ids map[string]bool) error {
	client, err := createExoscaleClient(ctx, zone)
	if err != nil {
		return err
	}

	instances, err := client.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances in %s: %w", zone, err)
	}
	if instances != nil {
		for _, inst := range instances.Instances {
			addLabeledDeploymentID(ids, inst.Labels)
		}
	}

	volumes, err := client.ListBlockStorageVolumes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list block storage volumes in %s: %w", zone, err)
	}
	if volumes != nil {
		for _, vol := range volumes.BlockStorageVolumes {
			addLabeledDeploymentID(ids, vol.Labels)
		}
	}

	networks, err := client.ListPrivateNetworks(ctx)
	if err != nil {
		return fmt.Errorf("failed to list private networks in %s: %w", zone, err)
	}
	if networks != nil {
		for _, network := range networks.PrivateNetworks {
			addLabeledDeploymentID(ids, network.Labels)
		}
	}

	// VPCs need the networking service, which a key may lack.
	vpcs, err := listDeploymentVPCs(ctx, client)
	if err != nil {
		slog.Warn("VPC discovery failed", "zone", zone, "error", err)
	}
	for _, entry := range vpcs {
		ids[entry.deploymentID] = true
	}

	return nil
}

func addLabeledDeploymentID(ids map[string]bool, labels map[string]string) {
	if depID := labels["deployment_id"]; depID != "" {
		ids[depID] = true
	}
}
