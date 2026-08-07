// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import (
	"strings"
)

// classifyARN inspects an AWS ARN and returns the internal ResourceType and native id.
// Expected EC2 style: arn:aws:ec2:region:account:resourceType/resourceId
// SSM parameter ARN: arn:aws:ssm:region:account:parameter/paramName.
func classifyARN(arn string) (ResourceType, string) {
	if !strings.HasPrefix(arn, "arn:") {
		return "", ""
	}
	parts := strings.SplitN(arn, ":", arnSplitParts)
	if len(parts) < arnSplitParts {
		return "", ""
	}
	service := parts[2]
	resource := parts[5]

	rtype, rid := splitResourceTypeAndID(resource)

	if resType, id, ok := classifyByService(service, rtype, rid, resource); ok {
		return resType, id
	}

	// Fallback: synthesize a display type from service and rtype so UI shows something meaningful
	if rtype != "" {
		return ResourceType(service + "-" + rtype), rid
	}

	return ResourceType(service), rid
}

// splitResourceTypeAndID splits an ARN resource segment on '/' to get type and
// id; some resources embed the type in a ':'-separated prefix instead.
func splitResourceTypeAndID(resource string) (string, string) {
	segs := strings.Split(resource, "/")
	if len(segs) < minSegs {
		alt := strings.Split(resource, ":")
		if len(alt) >= minSegs {
			segs = alt
		}
	}

	return segs[0], segs[len(segs)-1]
}

// classifyByService dispatches to the per-service classifier and reports
// whether the service/type combination was recognized.
func classifyByService(service, rtype, rid, resource string) (ResourceType, string, bool) {
	switch service {
	case "ec2":
		if resType, ok := classifyEC2Resource(rtype); ok {
			return resType, rid, true
		}
	case "s3":
		return classifyS3Resource(resource)
	case "ssm":
		return classifySSMResource(rtype, resource)
	case "iam":
		if resType, ok := classifyIAMResource(rtype); ok {
			return resType, rid, true
		}
	}

	return "", "", false
}

func classifyEC2Resource(rtype string) (ResourceType, bool) {
	switch rtype {
	case "instance":
		return ResourceEC2Instance, true
	case "volume":
		return ResourceEBSVolume, true
	case "vpc-endpoint":
		return ResourceVPCEndpoint, true
	case "internet-gateway":
		return ResourceInternetGW, true
	case "route-table":
		return ResourceRouteTable, true
	case "security-group":
		return ResourceSecurityGrp, true
	case "subnet":
		return ResourceSubnet, true
	case "vpc":
		return ResourceVPC, true
	case "key-pair":
		return ResourceEC2KeyPair, true
	default:
		return "", false
	}
}

// classifyS3Resource handles S3 bucket ARNs, which are of the form
// arn:aws:s3:::bucket-name and may parse with an empty type segment.
func classifyS3Resource(resource string) (ResourceType, string, bool) {
	if strings.HasPrefix(resource, ":::") {
		return ResourceS3Bucket, strings.TrimPrefix(resource, ":::"), true
	}
	if resource != "" {
		return ResourceS3Bucket, resource, true
	}

	return "", "", false
}

// classifySSMResource handles parameter ARNs, returning the full parameter
// name after the "parameter/" prefix rather than only the last segment.
func classifySSMResource(rtype, resource string) (ResourceType, string, bool) {
	if rtype != "parameter" {
		return "", "", false
	}

	return ResourceSSMParam, strings.TrimPrefix(resource, "parameter/"), true
}

func classifyIAMResource(rtype string) (ResourceType, bool) {
	switch rtype {
	case "user":
		return ResourceType("iam-user"), true
	case "role":
		return ResourceIAMRole, true
	case "policy":
		return ResourceType("iam-policy"), true
	case "instance-profile":
		return ResourceIAMInstProf, true
	default:
		return "", false
	}
}
