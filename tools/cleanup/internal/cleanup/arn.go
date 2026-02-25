// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package cleanup

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

	// Split resource on '/' to get type and id; some resources embed type in prefix.
	segs := strings.Split(resource, "/")
	if len(segs) < minSegs {
		// Some ARNs might use ':' separators; try that.
		alt := strings.Split(resource, ":")
		if len(alt) >= minSegs {
			segs = alt
		}
	}
	rtype := segs[0]
	rid := segs[len(segs)-1]

	switch service {
	case "ec2":
		switch rtype {
		case "instance":
			return ResourceEC2Instance, rid
		case "volume":
			return ResourceEBSVolume, rid
		case "vpc-endpoint":
			return ResourceVPCEndpoint, rid
		case "internet-gateway":
			return ResourceInternetGW, rid
		case "route-table":
			return ResourceRouteTable, rid
		case "security-group":
			return ResourceSecurityGrp, rid
		case "subnet":
			return ResourceSubnet, rid
		case "vpc":
			return ResourceVPC, rid
		case "key-pair":
			// EC2 key pairs: classify for display and cleanup
			return ResourceEC2KeyPair, rid
		default:
			// fallthrough to synthesized type below
		}
	case "s3":
		// S3 bucket ARNs are of the form arn:aws:s3:::bucket-name
		// For S3, resource part can be like ':::bucket-name' without type segment
		if strings.HasPrefix(resource, ":::") {
			return ResourceS3Bucket, strings.TrimPrefix(resource, ":::")
		}
		// Some ARN parsers split arn:aws:s3:::bucket-name into parts where
		// resource == "bucket-name" and region/account are empty.
		if resource != "" {
			return ResourceS3Bucket, resource
		}
	case "ssm":
		// parameter ARNs start with parameter/<full path>
		if rtype == "parameter" {
			// Return the full parameter name after the "parameter/" prefix
			// instead of only the last segment
			full := strings.TrimPrefix(resource, "parameter/")
			return ResourceSSMParam, full
		}
	case "iam":
		// Classify common IAM resources for better display
		switch rtype {
		case "user":
			return ResourceType("iam-user"), rid
		case "role":
			return ResourceIAMRole, rid
		case "policy":
			return ResourceType("iam-policy"), rid
		case "instance-profile":
			return ResourceIAMInstProf, rid
		default:
			// fallthrough
		}
	default:
		// fallthrough to synthesized type below
	}
	// Fallback: synthesize a display type from service and rtype so UI shows something meaningful
	if rtype != "" {
		return ResourceType(service + "-" + rtype), rid
	}

	return ResourceType(service), rid
}
