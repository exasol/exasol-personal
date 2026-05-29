// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import "github.com/exasol/exasol-personal/tools/cleanup/internal/shared"

func resourceTypeFilter(typeNames []string) []shared.ResourceType {
	if len(typeNames) == 0 {
		return nil
	}

	typeFilter := make([]shared.ResourceType, 0, len(typeNames))
	for _, typeName := range typeNames {
		typeFilter = append(typeFilter, shared.ResourceType(typeName))
	}

	return typeFilter
}

func filterDeploymentDetailsByTypes(
	details *shared.DeploymentDetails,
	typeNames []string,
) *shared.DeploymentDetails {
	if len(typeNames) == 0 {
		return details
	}

	allowedTypes := make(map[shared.ResourceType]struct{}, len(typeNames))
	for _, typeName := range typeNames {
		allowedTypes[shared.ResourceType(typeName)] = struct{}{}
	}

	filteredResources := make([]shared.ResourceMeta, 0, len(details.Resources))
	for _, resource := range details.Resources {
		if _, ok := allowedTypes[resource.Ref.Type]; ok {
			filteredResources = append(filteredResources, resource)
		}
	}

	filteredDetails := *details
	filteredDetails.Resources = filteredResources
	filteredDetails.Summary = details.Summary
	filteredDetails.Summary.Resources = len(filteredResources)

	return &filteredDetails
}
