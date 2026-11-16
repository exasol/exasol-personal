// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"fmt"
	"testing"
	"time"

	v3 "github.com/exoscale/egoscale/v3"
)

const (
	testVPCID    = "11111111-1111-4111-8111-111111111111"
	testSubnetID = "22222222-2222-4222-8222-222222222222"
)

func TestVPCDeploymentID(t *testing.T) {
	// Given VPCs labeled, named, unrelated to, and defaulted for a deployment
	isDefault := true
	cases := []struct {
		name string
		vpc  v3.ListVpcEntry
		want string
	}{
		{"label", v3.ListVpcEntry{Name: "custom", Labels: v3.Labels{"deployment_id": "exasol-e5b1becf"}}, "exasol-e5b1becf"},
		{"name", v3.ListVpcEntry{Name: "exasol-e5b1becf-vpc"}, "exasol-e5b1becf"},
		{"unrelated", v3.ListVpcEntry{Name: "team-network"}, ""},
		{"default", v3.ListVpcEntry{Name: "exasol-e5b1becf-vpc", Default: &isDefault}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// When
			got := vpcDeploymentID(tc.vpc)

			// Then
			if got != tc.want {
				t.Errorf("vpcDeploymentID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAddVPCsToSummariesDiscoversVPCOnlyDeployments(t *testing.T) {
	// Given a deployment VPC with one subnet created after it
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	vpcs := []deploymentVPC{{
		deploymentID: "exasol-e5b1becf",
		vpc:          v3.ListVpcEntry{ID: testVPCID, Name: "exasol-e5b1becf-vpc", CreatedAT: created},
		subnets:      []v3.ListSubnetEntry{{ID: testSubnetID, CreatedAT: created.Add(time.Second)}},
	}}
	summaries := map[string]*DeploymentSummary{}

	// When
	addVPCsToSummaries(vpcs, "ch-gva-2", "*", summaries)

	// Then the deployment is found with both resources, dated by the VPC
	sum, ok := summaries["exasol-e5b1becf"]
	if !ok {
		t.Fatalf("deployment not discovered: %v", summaries)
	}
	if sum.Resources != 2 {
		t.Errorf("Resources = %d, want 2", sum.Resources)
	}
	if !sum.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", sum.CreatedAt, created)
	}
}

func TestVPCResourceMetasIdentifySubnetsWithTheirVPC(t *testing.T) {
	// Given VPCs of two deployments
	vpcs := []deploymentVPC{
		{
			deploymentID: "exasol-e5b1becf",
			vpc:          v3.ListVpcEntry{ID: testVPCID, Name: "exasol-e5b1becf-vpc"},
			subnets:      []v3.ListSubnetEntry{{ID: testSubnetID, Name: "exasol-e5b1becf-subnet"}},
		},
		{deploymentID: "exasol-0badc0de", vpc: v3.ListVpcEntry{ID: testSubnetID}},
	}

	// When
	metas := vpcResourceMetas(vpcs, "ch-gva-2", "exasol-e5b1becf")

	// Then the deployment's VPC and subnet are described, and the subnet ID can be parsed back
	if len(metas) != 2 {
		t.Fatalf("got %d resources, want 2: %v", len(metas), metas)
	}
	if metas[0].Ref.Type != ResourceVPC || metas[0].Ref.ID != testVPCID {
		t.Errorf("first resource = %+v, want the VPC", metas[0].Ref)
	}
	if metas[1].Ref.Type != ResourceVPCSubnet {
		t.Fatalf("second resource type = %s, want %s", metas[1].Ref.Type, ResourceVPCSubnet)
	}
	vpcID, subnetID, err := parseVPCSubnetID(metas[1].Ref.ID)
	if err != nil {
		t.Fatalf("parseVPCSubnetID(%q) failed: %v", metas[1].Ref.ID, err)
	}
	if vpcID != testVPCID || subnetID != testSubnetID {
		t.Errorf("parsed (%s, %s), want (%s, %s)", vpcID, subnetID, testVPCID, testSubnetID)
	}
}

func TestParseVPCSubnetIDRejectsMalformedIDs(t *testing.T) {
	// Given IDs without a VPC part or with an invalid UUID
	for _, id := range []string{testSubnetID, "not-a-uuid/" + testSubnetID, testVPCID + "/not-a-uuid"} {
		// When
		_, _, err := parseVPCSubnetID(id)

		// Then
		if err == nil {
			t.Errorf("parseVPCSubnetID(%q) succeeded, want an error", id)
		}
	}
}

func TestAddVPCsToSummariesCountsOwnerlessVPCsOfKnownDeployments(t *testing.T) {
	// Given a deployment that alice owns and an ownerless VPC of an unknown deployment
	vpcs := []deploymentVPC{
		{deploymentID: "exasol-0badc0de", vpc: v3.ListVpcEntry{ID: testVPCID}},
		{deploymentID: "exasol-e5b1becf", vpc: v3.ListVpcEntry{ID: testSubnetID}},
	}
	summaries := map[string]*DeploymentSummary{
		"exasol-0badc0de": {ID: "exasol-0badc0de", Owner: "alice", Resources: 1},
	}

	// When the owner filter names alice
	addVPCsToSummaries(vpcs, "ch-gva-2", "alice", summaries)

	// Then the VPC counts toward alice's deployment and establishes no other one
	if got := summaries["exasol-0badc0de"].Resources; got != 2 {
		t.Errorf("Resources = %d, want 2", got)
	}
	if len(summaries) != 1 {
		t.Errorf("got deployments %v, want only exasol-0badc0de", summaries)
	}
}

func TestRetryVPCDeleteTreatsMissingResourcesAsDeleted(t *testing.T) {
	// Given a delete call that reports the resource as not found
	calls := 0
	deleteFn := func() error {
		calls++
		return fmt.Errorf("DeleteVpc: http response: %w", v3.ErrNotFound)
	}

	// When
	err := retryVPCDelete(context.Background(), "VPC", deleteFn)

	// Then
	if err != nil || calls != 1 {
		t.Errorf("err = %v after %d calls, want success after 1 call", err, calls)
	}
}

func TestRetryVPCDeleteStopsOnMissingPermission(t *testing.T) {
	// Given a delete call that the IAM role forbids
	calls := 0
	deleteFn := func() error {
		calls++
		return fmt.Errorf("DeleteVpc: http response: %w", v3.ErrForbidden)
	}

	// When
	err := retryVPCDelete(context.Background(), "VPC", deleteFn)

	// Then it fails without retrying
	if err == nil || calls != 1 {
		t.Errorf("err = %v after %d calls, want a failure after 1 call", err, calls)
	}
}
