// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

// CollectDeploymentSummaries discovers Exasol Personal deployments across the
// contract, optionally restricted to one location.
func CollectDeploymentSummaries(
	ctx context.Context,
	location string,
	ownerFilter string,
) ([]DeploymentSummary, error) {
	if !ownerFilterMatchesUnowned(ownerFilter) {
		return []DeploymentSummary{}, nil
	}

	api, err := newClient()
	if err != nil {
		return nil, err
	}

	inv, err := listInventory(ctx, api, location)
	if err != nil {
		return nil, err
	}

	deployments := groupDeployments(inv, location)
	summaries := make([]DeploymentSummary, 0, len(deployments))
	for _, details := range deployments {
		summaries = append(summaries, details.Summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })

	return summaries, nil
}

// CollectDeploymentDetails returns every IONOS resource of a deployment.
func CollectDeploymentDetails(
	ctx context.Context,
	location string,
	deploymentID string,
) (*DeploymentDetails, error) {
	api, err := newClient()
	if err != nil {
		return nil, err
	}

	inv, err := listInventory(ctx, api, location)
	if err != nil {
		return nil, err
	}

	details, ok := groupDeployments(inv, location)[deploymentID]
	if !ok {
		return nil, fmt.Errorf("deployment %s not found in IONOS Cloud", deploymentID)
	}

	return details, nil
}

type deploymentSet map[string]*DeploymentDetails

// groupDeployments assigns resources to deployments. A datacenter named after a
// deployment was created by the launcher and owns everything inside it, unless
// another deployment was installed into it with --datacenter-id. In that case,
// as in any other datacenter, only resources carrying a deployment name prefix
// belong to that deployment, and the datacenter itself is kept.
func groupDeployments(inv inventory, location string) map[string]*DeploymentDetails {
	deployments := deploymentSet{}
	for _, dc := range inv.datacenters {
		if matchesLocation(location, dc.datacenter.location) {
			deployments.addDatacenter(dc)
		}
	}
	for _, block := range inv.ipBlocks {
		if matchesLocation(location, block.location) {
			deployments.addByName(block)
		}
	}
	for _, details := range deployments {
		details.Summary.Resources = len(details.Resources)
		details.Summary.CreatedAt = earliestCreatedAt(details.Resources)
		details.Summary.State = deriveState(details.Resources)
	}

	return deployments
}

func (deployments deploymentSet) addDatacenter(dc datacenterInventory) {
	if !deploymentIDRegex.MatchString(dc.datacenter.name) || hostsOtherDeployment(dc) {
		for _, child := range dc.children {
			deployments.addByName(child)
		}

		return
	}

	deployments.add(dc.datacenter.name, dc.datacenter)
	for _, child := range dc.children {
		deployments.add(dc.datacenter.name, child)
	}
}

func hostsOtherDeployment(dc datacenterInventory) bool {
	for _, child := range dc.children {
		if deploymentID, ok := deploymentFromName(child.name); ok && deploymentID != dc.datacenter.name {
			return true
		}
	}

	return false
}

func (deployments deploymentSet) addByName(res resource) {
	if deploymentID, ok := deploymentFromName(res.name); ok {
		deployments.add(deploymentID, res)
	}
}

func (deployments deploymentSet) add(deploymentID string, res resource) {
	details, ok := deployments[deploymentID]
	if !ok {
		details = &DeploymentDetails{Summary: DeploymentSummary{
			ID:       deploymentID,
			Provider: ProviderName,
			Region:   res.location,
			Owner:    "-",
		}}
		deployments[deploymentID] = details
	}
	details.Resources = append(details.Resources, resourceMeta(res))
}

func resourceMeta(res resource) ResourceMeta {
	attr := map[string]any{"name": res.name}
	if res.state != "" {
		attr["state"] = res.state
	}
	if !res.createdAt.IsZero() {
		attr["createdAt"] = res.createdAt
	}

	return ResourceMeta{
		Ref: ResourceRef{
			Type:     res.kind,
			Region:   res.location,
			ParentID: res.parentID,
			ID:       res.id,
		},
		Attr: attr,
	}
}

func deploymentFromName(name string) (string, bool) {
	match := resourceNamePattern.FindStringSubmatch(name)
	if match == nil {
		return "", false
	}

	return match[1], true
}

// earliestCreatedAt uses the API creation time because the launcher's
// createdat label exists only on servers.
func earliestCreatedAt(resources []ResourceMeta) time.Time {
	var earliest time.Time
	for _, res := range resources {
		createdAt, ok := res.Attr["createdAt"].(time.Time)
		if !ok {
			continue
		}
		if earliest.IsZero() || createdAt.Before(earliest) {
			earliest = createdAt
		}
	}

	return earliest
}

// deriveState reports "active" while servers exist and "orphaned" when only
// leftover resources remain.
func deriveState(resources []ResourceMeta) string {
	for _, res := range resources {
		if res.Ref.Type == ResourceServer {
			return shared.StateActive
		}
	}
	if len(resources) > 0 {
		return "orphaned"
	}

	return shared.StateUnknown
}

// matchesLocation reports whether a resource in resourceLocation should be
// included for a collector scoped to collectorLocation.
func matchesLocation(collectorLocation, resourceLocation string) bool {
	if collectorLocation == "" || collectorLocation == DefaultLocation {
		return true
	}

	return strings.EqualFold(collectorLocation, resourceLocation)
}
