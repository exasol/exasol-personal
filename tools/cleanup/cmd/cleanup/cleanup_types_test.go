// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

func TestFilterDeploymentDetailsByTypes(t *testing.T) {
	t.Parallel()

	input := &shared.DeploymentDetails{
		Summary: shared.DeploymentSummary{
			ID:        "exasol-12345678",
			Provider:  "aws",
			Region:    "eu-central-1",
			Owner:     "owner-1",
			State:     shared.StateStopped,
			Resources: 3,
		},
		Resources: []shared.ResourceMeta{
			{Ref: shared.ResourceRef{Type: shared.ResourceType("ec2-instance"), ID: "i-1"}},
			{Ref: shared.ResourceRef{Type: shared.ResourceType("ebs-volume"), ID: "vol-1"}},
			{Ref: shared.ResourceRef{Type: shared.ResourceType("ssm-parameter"), ID: "param-1"}},
		},
	}

	// Given deployment details with multiple resource types
	// When a type filter is applied
	filtered := filterDeploymentDetailsByTypes(input, []string{"ec2-instance", "ssm-parameter"})

	// Then only the matching resources remain and the summary count matches
	if filtered == input {
		t.Fatal("filtered details reused original pointer")
	}
	if len(filtered.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(filtered.Resources))
	}
	if filtered.Resources[0].Ref.ID != "i-1" || filtered.Resources[1].Ref.ID != "param-1" {
		t.Fatalf("filtered resources = %#v, want ec2-instance and ssm-parameter", filtered.Resources)
	}
	if filtered.Summary.Resources != 2 {
		t.Fatalf("summary resources = %d, want 2", filtered.Summary.Resources)
	}
	if input.Summary.Resources != 3 {
		t.Fatalf("original summary resources = %d, want 3", input.Summary.Resources)
	}
}
