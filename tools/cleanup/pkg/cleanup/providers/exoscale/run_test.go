// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import "testing"

func TestPlanActionsDeletesVPCResourcesAfterInstances(t *testing.T) {
	// Given a deployment with a VPC, its subnet, and an instance attached to it
	details := &DeploymentDetails{Resources: []ResourceMeta{
		{Ref: ResourceRef{Type: ResourceVPC, ID: "vpc"}},
		{Ref: ResourceRef{Type: ResourceVPCSubnet, ID: "vpc/subnet"}},
		{Ref: ResourceRef{Type: ResourceComputeInstance, ID: "instance"}},
	}}

	// When
	actions, err := PlanActions(details, nil)

	// Then the instance is deleted first, then the subnet, then the VPC
	if err != nil {
		t.Fatalf("PlanActions failed: %v", err)
	}
	var order []ResourceType
	for _, action := range actions {
		order = append(order, action.Ref.Type)
	}
	want := []ResourceType{ResourceComputeInstance, ResourceVPCSubnet, ResourceVPC}
	if len(order) != len(want) {
		t.Fatalf("planned %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("planned %v, want %v", order, want)
		}
	}
}
