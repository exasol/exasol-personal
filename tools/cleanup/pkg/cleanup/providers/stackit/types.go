// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
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

// STACKIT-specific resource type constants
const (
	ResourceServer                        ResourceType = "stackit-server"
	ResourceVolume                        ResourceType = "stackit-volume"
	ResourcePublicIP                      ResourceType = "stackit-public-ip"
	ResourceNetworkInterface              ResourceType = "stackit-network-interface"
	ResourceNetwork                       ResourceType = "stackit-network"
	ResourceSecurityGroup                 ResourceType = "stackit-security-group"
	ResourceObjectStorageBucket           ResourceType = "stackit-objectstorage-bucket"
	ResourceObjectStorageCredential       ResourceType = "stackit-objectstorage-credential"
	ResourceObjectStorageCredentialsGroup ResourceType = "stackit-objectstorage-credential-group"
)

// Organization represents a STACKIT organization from the API
type Organization struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"city"`
	Country string `json:"country"`
}
