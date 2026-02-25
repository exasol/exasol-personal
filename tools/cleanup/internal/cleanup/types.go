// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package cleanup

import "time"

// ResourceType enumerates supported AWS resource classes for cleanup ordering.
type ResourceType string

const (
	ResourceEC2Instance ResourceType = "ec2-instance"
	ResourceEBSVolume   ResourceType = "ebs-volume"
	ResourceEC2KeyPair  ResourceType = "ec2-key-pair"
	ResourceVPCEndpoint ResourceType = "vpc-endpoint"
	ResourceInternetGW  ResourceType = "internet-gateway"
	ResourceRouteTable  ResourceType = "route-table"
	ResourceSecurityGrp ResourceType = "security-group"
	ResourceSubnet      ResourceType = "subnet"
	ResourceVPC         ResourceType = "vpc"
	ResourceSSMParam    ResourceType = "ssm-parameter"
	ResourceS3Bucket    ResourceType = "s3-bucket"
	ResourceIAMRole     ResourceType = "iam-role"
	ResourceIAMInstProf ResourceType = "iam-instance-profile"
)

// DeploymentSummary is a high-level view of a tagged deployment.
type DeploymentSummary struct {
	ID        string    `json:"id"`
	Region    string    `json:"region"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
	State     string    `json:"state"` // e.g. active|partial|unknown
	Resources int       `json:"resources"`
}

// ResourceRef identifies a resource found via tagging or relationship.
type ResourceRef struct {
	ARN    string       `json:"arn"`
	Type   ResourceType `json:"type"`
	Region string       `json:"region"`
	ID     string       `json:"id"` // native ID (e.g., i-123, vol-abc)
}

// ResourceMeta adds descriptive metadata used for planning & output.
type ResourceMeta struct {
	Ref       ResourceRef       `json:"ref"`
	Tags      map[string]string `json:"tags"`
	Attr      map[string]any    `json:"attributes"`
	Protected bool              `json:"protected"` // default or cannot delete
}

// Phase groups homogeneous resource types for ordered deletion.
type Phase struct {
	Name  string         `json:"name"`
	Types []ResourceType `json:"types"`
}

// CleanupPlan is the ordered execution structure.
type CleanupPlan struct {
	Phases []Phase `json:"phases"`
}

// Action represents an individual planned step.
type Action struct {
	Ref    ResourceRef `json:"ref"`
	Op     string      `json:"op"`     // delete|detach|skip
	Reason string      `json:"reason"` // why skip or additional context
}

// Result captures execution outcome of an action.
type Result struct {
	Action Action `json:"action"`
	Status string `json:"status"` // planned|success|failed|skipped
	Error  string `json:"error,omitempty"`
}

// DeploymentDetails holds all resources for a deployment.
type DeploymentDetails struct {
	Summary   DeploymentSummary `json:"summary"`
	Resources []ResourceMeta    `json:"resources"`
}

// Common errors.
var (
	ErrDeploymentIDRequired = CleanupError("deployment id is required")
	ErrRegionRequired       = CleanupError("region is required for discovery")
)

// CleanupErr is a lightweight string error helper.
type CleanupError string

func (e CleanupError) Error() string { return string(e) }

// DiscoverOptions for discovery behavior.
type DiscoverOptions struct {
	Region      string
	OwnerFilter string // optional owner ARN/substring with '*' wildcard; '*' means any
	// Future: filtering options
}

// RunOptions for cleanup run behavior.
type RunOptions struct {
	Region     string
	Execute    bool
	TypeFilter []ResourceType // optional narrowing
}

// Common string constants to satisfy linter and avoid duplication.
const (
	StateActive       = "active"
	StateProvisioning = "provisioning"
	StateStopped      = "stopped"
	StateTerminated   = "terminated"
	StateUnknown      = "unknown"
	OpSkip            = "skip"
)
