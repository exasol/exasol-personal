// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import (
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Type aliases for shared types used throughout the aws package
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

// AWS-specific resource type constants
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