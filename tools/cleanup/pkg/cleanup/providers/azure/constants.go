// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import "regexp"

// ProviderName is the identifier for the Azure provider.
const ProviderName = "azure"

// DefaultLocation is the sentinel location meaning "all locations in the
// subscription". Azure discovery is subscription-wide (a single list call
// returns resource groups across every region), so unlike AWS/Exoscale/STACKIT
// a location is a client-side filter rather than a separate query target.
const DefaultLocation = "all"

// Tag keys the launcher sets on every Azure resource and resource group
// (see assets/infrastructure/azure/main.tf, local.common_tags).
const (
	tagProject    = "Project"
	tagDeployment = "Deployment"
	tagOwner      = "Owner"
	tagCreatedAt  = "CreatedAt"

	projectValue = "exasol-personal"
)

// deploymentIDRegex matches the strict deployment identifier format
// (exasol-<8 hex>), matching the AWS provider's discovery filter.
var deploymentIDRegex = regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)
