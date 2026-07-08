// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import "github.com/exasol/exasol-personal/tools/cleanup/internal/shared"

// Type aliases for shared types used throughout the azure package.
type (
	DeploymentSummary = shared.DeploymentSummary
	DeploymentDetails = shared.DeploymentDetails
	ResourceRef       = shared.ResourceRef
	ResourceMeta      = shared.ResourceMeta
	Action            = shared.Action
	Result            = shared.Result
	CleanupPlan       = shared.CleanupPlan
	Phase             = shared.Phase
)

// ResourceType is imported from the shared package.
type ResourceType = shared.ResourceType

// Azure-specific resource type constants. The resource group is the unit of
// cleanup (deleting it cascades to everything it contains); the remaining types
// classify the inventory reported by `show` and covered by a `run` plan.
const (
	ResourceResourceGroup  ResourceType = "azure-resource-group"
	ResourceVirtualMachine ResourceType = "azure-vm"
	ResourceDisk           ResourceType = "azure-disk"
	ResourceNetworkIface   ResourceType = "azure-nic"
	ResourcePublicIP       ResourceType = "azure-public-ip"
	ResourceVirtualNetwork ResourceType = "azure-vnet"
	ResourceSubnet         ResourceType = "azure-subnet"
	ResourceSecurityGroup  ResourceType = "azure-nsg"
	ResourceStorageAccount ResourceType = "azure-storage-account"
	// ResourceGeneric is the fallback for any Azure resource type without a
	// dedicated classification.
	ResourceGeneric ResourceType = "azure-resource"
)
