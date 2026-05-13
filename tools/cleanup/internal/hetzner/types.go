// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package hetzner

import (
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Type aliases for shared types used throughout the hetzner package
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

// Hetzner-specific resource type constants
const (
	ResourceServer     ResourceType = "hetzner-server"
	ResourceVolume     ResourceType = "hetzner-volume"
	ResourceNetwork    ResourceType = "hetzner-network"
	ResourceFirewall   ResourceType = "hetzner-firewall"
	ResourceSSHKey     ResourceType = "hetzner-ssh-key"
)
