// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import "testing"

func TestClassifyARN(t *testing.T) {
	t.Parallel()
	cases := []struct {
		arn      string
		wantType ResourceType
		wantID   string
	}{
		{
			"arn:aws:ec2:eu-central-1:123456789012:instance/i-abc123",
			ResourceEC2Instance,
			"i-abc123",
		},
		{"arn:aws:ec2:eu-central-1:123456789012:volume/vol-xyz", ResourceEBSVolume, "vol-xyz"},
		{
			"arn:aws:ec2:eu-central-1:123456789012:internet-gateway/igw-1",
			ResourceInternetGW,
			"igw-1",
		},
		{"arn:aws:ec2:eu-central-1:123456789012:route-table/rtb-1", ResourceRouteTable, "rtb-1"},
		{"arn:aws:ec2:eu-central-1:123456789012:security-group/sg-1", ResourceSecurityGrp, "sg-1"},
		{"arn:aws:ec2:eu-central-1:123456789012:subnet/subnet-1", ResourceSubnet, "subnet-1"},
		{"arn:aws:ec2:eu-central-1:123456789012:vpc/vpc-1", ResourceVPC, "vpc-1"},
		{
			"arn:aws:ssm:eu-central-1:123456789012:parameter/deployment-param",
			ResourceSSMParam,
			"deployment-param",
		},
		{"invalid", "", ""},
	}
	for _, currentCase := range cases {
		gotType, gotID := classifyARN(currentCase.arn)
		if gotType != currentCase.wantType || gotID != currentCase.wantID {
			t.Errorf(
				"classifyARN(%s) = (%s,%s) want (%s,%s)",
				currentCase.arn,
				gotType,
				gotID,
				currentCase.wantType,
				currentCase.wantID,
			)
		}
	}
}
