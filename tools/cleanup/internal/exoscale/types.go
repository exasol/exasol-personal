// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Type aliases for shared types used throughout the exoscale package
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

// ResourceType is imported from shared package
type ResourceType = shared.ResourceType

// Exoscale-specific resource type constants
const (
	ResourceComputeInstance  ResourceType = "exoscale-compute-instance"
	ResourceBlockVolume      ResourceType = "exoscale-block-storage-volume"
	ResourcePrivateNetwork   ResourceType = "exoscale-private-network"
	ResourceSecurityGroup    ResourceType = "exoscale-security-group"
	ResourceSSHKey           ResourceType = "exoscale-ssh-key"
	ResourceIAMRole          ResourceType = "exoscale-iam-role"
	ResourceIAMAPIKey        ResourceType = "exoscale-iam-api-key"
	ResourceSOSBucket        ResourceType = "sos-bucket"
)

// Organization represents an Exoscale organization from the API
type Organization struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"city"`
	Country string `json:"country"`
}
