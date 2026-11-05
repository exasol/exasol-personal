// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"time"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

// Type aliases for shared types used throughout the ionos package.
type (
	DeploymentSummary = shared.DeploymentSummary
	DeploymentDetails = shared.DeploymentDetails
	ResourceRef       = shared.ResourceRef
	ResourceMeta      = shared.ResourceMeta
	Action            = shared.Action
	Result            = shared.Result
)

// ResourceType is imported from the shared package.
type ResourceType = shared.ResourceType

// IONOS-specific resource type constants.
const (
	ResourceDatacenter ResourceType = "ionos-datacenter"
	ResourceServer     ResourceType = "ionos-server"
	ResourceVolume     ResourceType = "ionos-volume"
	ResourceLAN        ResourceType = "ionos-lan"
	ResourceIPBlock    ResourceType = "ionos-ipblock"
)

// resource is the provider-neutral view of an IONOS resource used for grouping
// and planning.
type resource struct {
	kind      ResourceType
	id        string
	name      string
	location  string
	parentID  string
	createdAt time.Time
	state     string
}

type datacenterInventory struct {
	datacenter resource
	children   []resource
}

type inventory struct {
	datacenters []datacenterInventory
	ipBlocks    []resource
}
