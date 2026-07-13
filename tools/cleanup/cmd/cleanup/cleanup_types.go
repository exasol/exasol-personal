// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"

func resourceTypeFilter(typeNames []string) []shared.ResourceType {
	return shared.ResourceTypeFilter(typeNames)
}

func filterDeploymentDetailsByTypes(
	details *shared.DeploymentDetails,
	typeNames []string,
) *shared.DeploymentDetails {
	return shared.FilterDeploymentDetailsByTypes(details, resourceTypeFilter(typeNames))
}
