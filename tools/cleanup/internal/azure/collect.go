// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// CollectDeploymentSummaries discovers Exasol Personal deployments in the
// subscription. Each deployment maps to a single resource group tagged by the
// launcher; resources within the group are counted to derive state.
func CollectDeploymentSummaries(
	ctx context.Context,
	subscriptionID string,
	location string,
	ownerFilter string,
	legacy bool,
) ([]DeploymentSummary, error) {
	cl, err := newClients(subscriptionID)
	if err != nil {
		return nil, err
	}

	summaries := make([]DeploymentSummary, 0)
	pager := cl.groups.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list Azure resource groups: %w", err)
		}
		for _, group := range page.Value {
			if group == nil || group.Name == nil {
				continue
			}
			if !matchesDeploymentTags(group.Tags, legacy) {
				continue
			}
			rgLocation := valueOrEmpty(group.Location)
			if !matchesLocation(location, rgLocation) {
				continue
			}
			owner := tagValue(group.Tags, tagOwner)
			if !ownerMatchesFilter(owner, ownerFilter) {
				continue
			}

			resources, err := listResourcesInGroup(ctx, cl, *group.Name)
			if err != nil {
				return nil, err
			}

			summaries = append(summaries, DeploymentSummary{
				ID:        tagValue(group.Tags, tagDeployment),
				Provider:  ProviderName,
				Region:    rgLocation,
				Owner:     ownerOrDash(owner),
				CreatedAt: parseCreatedAt(tagValue(group.Tags, tagCreatedAt)),
				State:     deriveState(resources),
				// Count the resource group itself alongside the resources it holds.
				Resources: len(resources) + 1,
			})
		}
	}

	return summaries, nil
}

// CollectDeploymentDetails returns the resource group for a deployment together
// with a typed inventory of every resource it contains.
func CollectDeploymentDetails(
	ctx context.Context,
	subscriptionID string,
	deploymentID string,
) (*DeploymentDetails, error) {
	cl, err := newClients(subscriptionID)
	if err != nil {
		return nil, err
	}

	group, err := findDeploymentGroup(ctx, cl, deploymentID)
	if err != nil {
		return nil, err
	}

	resourceGroupName := *group.Name
	details := &DeploymentDetails{
		Summary: DeploymentSummary{
			ID:        deploymentID,
			Provider:  ProviderName,
			Region:    valueOrEmpty(group.Location),
			Owner:     ownerOrDash(tagValue(group.Tags, tagOwner)),
			CreatedAt: parseCreatedAt(tagValue(group.Tags, tagCreatedAt)),
			State:     shared.StateUnknown,
		},
	}

	// The resource group is itself a cleanup target (deleting it cascades).
	details.Resources = append(details.Resources, ResourceMeta{
		Ref: ResourceRef{
			ARN:    valueOrEmpty(group.ID),
			Type:   ResourceResourceGroup,
			Region: details.Summary.Region,
			ID:     resourceGroupName,
		},
		Tags: tagsToMap(group.Tags),
		Attr: map[string]any{"name": resourceGroupName},
	})

	resources, err := listResourcesInGroup(ctx, cl, resourceGroupName)
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		details.Resources = append(details.Resources, resourceMetaFrom(resource, details.Summary.Region))
	}

	details.Summary.Resources = len(details.Resources)
	details.Summary.State = deriveState(resources)

	return details, nil
}

// findDeploymentGroup locates the resource group whose Deployment tag matches
// the requested deployment id.
func findDeploymentGroup(
	ctx context.Context,
	cl *clients,
	deploymentID string,
) (*armresources.ResourceGroup, error) {
	pager := cl.groups.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list Azure resource groups: %w", err)
		}
		for _, group := range page.Value {
			if group == nil || group.Name == nil {
				continue
			}
			if tagValue(group.Tags, tagDeployment) == deploymentID {
				return group, nil
			}
		}
	}

	return nil, fmt.Errorf("deployment %s not found in Azure subscription", deploymentID)
}

// listResourcesInGroup returns every resource contained in a resource group.
func listResourcesInGroup(
	ctx context.Context,
	cl *clients,
	resourceGroup string,
) ([]*armresources.GenericResourceExpanded, error) {
	var resources []*armresources.GenericResourceExpanded
	pager := cl.resources.NewListByResourceGroupPager(resourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list resources in resource group %s: %w", resourceGroup, err)
		}
		resources = append(resources, page.Value...)
	}

	return resources, nil
}

// resourceMetaFrom builds a typed ResourceMeta from an Azure resource.
func resourceMetaFrom(resource *armresources.GenericResourceExpanded, region string) ResourceMeta {
	azureType := valueOrEmpty(resource.Type)
	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    valueOrEmpty(resource.ID),
			Type:   classifyAzureType(azureType),
			Region: region,
			ID:     valueOrEmpty(resource.Name),
		},
		Tags: tagsToMap(resource.Tags),
		Attr: map[string]any{
			"name":      valueOrEmpty(resource.Name),
			"azureType": azureType,
		},
	}
	if resource.Location != nil {
		meta.Attr["location"] = *resource.Location
	}

	return meta
}

// deriveState reports "active" when a deployment still has virtual machines and
// "orphaned" when only leftover resources remain.
func deriveState(resources []*armresources.GenericResourceExpanded) string {
	for _, resource := range resources {
		if classifyAzureType(valueOrEmpty(resource.Type)) == ResourceVirtualMachine {
			return shared.StateActive
		}
	}
	if len(resources) > 0 {
		return "orphaned"
	}

	return shared.StateUnknown
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func ownerOrDash(owner string) string {
	if owner == "" {
		return "-"
	}

	return owner
}
